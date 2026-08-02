;;; cookie.el --- 从浏览器提取 Cookie 并与 restclient.el 集成 -*- lexical-binding: t; -*-

;; Author:
;; Version: 1.0.0
;; Keywords: restclient, chrome, firefox, edge, cookie, authentication
;; URL: https://github.com/thomas/cookie
;; Package-Requires: ((emacs "26.1"))

;;; Commentary:
;;
;; 该包提供了从浏览器（Chrome、Firefox、Edge）提取 Cookie 的功能，
;; 并与 restclient.el 集成，方便在本地开发时自动携带云端服务的认证 Token。
;;
;; 两种使用方式：
;;
;; 1. CLI 模式 — 调用 cookie-cli 命令行工具：
;;    :token := (cookie-get-value "api.example.com" "auth_token")
;;
;; 2. HTTP 模式 — 调用 cookie-cli serve 的 HTTP API：
;;    :token := (cookie-http-get "api.example.com" "auth_token")
;;
;; restclient.el 集成示例:
;;
;;   :token := (cookie-get-value "api.example.com" "auth_token")
;;   GET https://api.example.com/user
;;   Authorization: Bearer :token
;;
;;   # 获取所有 Cookie 并以 header 格式注入
;;   :cookies := (cookie-header "api.example.com")
;;   GET https://api.example.com/data
;;   Cookie: :cookies
;;
;; 辅助方式 — {{cookie:...}} 占位符批量刷新：
;;   在 restclient 文件中写 {{cookie:domain}} 或 {{cookie:domain name}}，
;;   发送前调用 M-x cookie-update-restclient-vars 批量刷新为变量。
;;   注意：占位符仅用于请求头/请求体展开，不能作为 :var 定义行。
;;
;; org-verb (verb.el) 集成：
;;   verb 的 code tag 语法 `{{...}}' 会对其中内容做 elisp 求值，
;;   可直接调用本包函数，无需额外配置：
;;
;;     :GET https://api.example.com/user
;;     :header Authorization: Bearer {{(cookie-get-value "api.example.com" "auth_token")}}
;;
;;   cookie-request-hook 可挂载到 :Verb-Map-Request: 属性，请求发送时
;;   自动从 URL 提取 domain 并注入 Cookie 头（无需手动调用）：
;;
;;     :Verb-Map-Request: cookie-request-hook
;;
;;   cookie-json-block-* 用于读取 Org 源块中的 JSON（构造请求体）：
;;
;;     #+name: payload
;;     #+begin_src json
;;     {"a": 1}
;;     #+end_src
;;
;;     :POST https://api.example.com/data
;;     :Content-Type: application/json
;;     :body {{(cookie-json-block-to-string "payload")}}
;;
;;   verb 中不要使用 `{{cookie:...}}' 占位符语法（那是 restclient 专用），
;;   verb 会把 `{{...}}' 当作 elisp 表达式求值而报错。

;;; Code:

(require 'json)
(require 'url)

;;; — Customization ———————————————————————————————————

(defgroup cookie nil
  "从浏览器提取 Cookie 并与 restclient.el 集成"
  :group 'tools
  :group 'convenience)

(defcustom cookie-cli-path "cookie-cli"
  "cookie-cli 可执行文件的路径。"
  :type 'string
  :group 'cookie)

(defcustom cookie-default-browser "chrome"
  "默认使用的浏览器类型。"
  :type '(choice (const "chrome") (const "firefox") (const "edge"))
  :group 'cookie)

(defcustom cookie-bridge-url "http://127.0.0.1:8008"
  "Cookie Bridge 服务的 URL。"
  :type 'string
  :group 'cookie)

(defcustom cookie-cache-expire 300
  "Cookie 缓存过期时间（秒）。设为 0 禁用 Emacs 侧缓存。

大于 0 时，调用 `cookie-cli get' 会追加 `-cache-expire' 参数，
限制 ~/.cookie/export.json 回退文件的最大年龄（秒）；为 0 则不传
该参数，cookie-cli 沿用其默认值（环境变量 COOKIE_CACHE_EXPIRE
或 300 秒）。"
  :type 'integer
  :group 'cookie)

(defcustom cookie-prefer-bridge t
  "非 nil 时优先通过 Bridge HTTP API 获取 Cookie，失败再回退到 CLI。"
  :type 'boolean
  :group 'cookie)

;;; — Cache ——————————————————————————————————————————

(defvar cookie--cache (make-hash-table :test 'equal)
  "Cookie 缓存。键为 (method browser domain name)，值为 (value . timestamp)。")

(defun cookie--cache-get (key)
  "从缓存获取 KEY 对应的值，过期则返回 nil。"
  (when (> cookie-cache-expire 0)
    (let ((entry (gethash key cookie--cache)))
      (when (and entry
                 (< (- (float-time) (cdr entry)) cookie-cache-expire))
        (car entry)))))

(defun cookie--cache-put (key value)
  "将 VALUE 写入缓存 KEY。"
  (when (> cookie-cache-expire 0)
    (puthash key (cons value (float-time)) cookie--cache))
  value)

(defun cookie--cache-clear (&optional msg)
  "清空哈希表 `cookie--cache'；若 MSG 非空则 `message' 显示。"
  (clrhash cookie--cache)
  (when msg (message "%s" msg)))

;;;###autoload
(defun cookie-refresh-cache ()
  "丢弃全部缓存，下次获取将重新从浏览器拉取。
适合在重新执行 restclient 请求前调用，以使用最新登录态。"
  (interactive)
  (cookie--cache-clear "Cookie 缓存已刷新，下次将从浏览器重新获取"))

(defun cookie-clear-cache ()
  "清除所有 Cookie 缓存。"
  (interactive)
  (cookie--cache-clear "Cookie 缓存已清除"))

;;; — CLI backend ————————————————————————————————————

(defun cookie--call-cli (&rest args)
  "调用 cookie-cli，传递 ARGS。返回 stdout（去除尾部换行）。出错时返回 nil。
对 `get' 子命令且 `cookie-cache-expire' 大于 0 时，自动追加 `-cache-expire'。

使用 `call-process' 且丢弃 stderr：避免 stderr 合并进输出时，
cookie-cli 的诊断日志（如 `log.Printf'）污染 Cookie 头内容。"
  (let* ((args (if (and (> cookie-cache-expire 0)
                        (equal (car args) "get"))
                   (append args
                           (list "-cache-expire"
                                 (number-to-string cookie-cache-expire)))
                 args))
         (program (if (file-name-absolute-p cookie-cli-path)
                      cookie-cli-path
                    (or (executable-find cookie-cli-path)
                        cookie-cli-path))))
    (condition-case err
        (with-temp-buffer
          (let ((exit (apply #'call-process
                             program nil
                             (list (current-buffer) nil) nil args)))
            (if (zerop exit)
                (let ((output (string-trim-right (buffer-string))))
                  (if (string-prefix-p "未找到" output)
                      nil
                    output))
              nil)))
      (error
       (message "cookie-cli 调用失败: %s" (error-message-string err))
       nil))))

;;; — HTTP backend (Bridge) —————————————————————————

(defun cookie--http-request (path &optional params)
  "向 Bridge 服务发送同步 GET 请求。
PATH 为路径（如 \"/cookies\"），PARAMS 为 alist 查询参数。
成功返回 parsed JSON，失败返回 nil。"
  (let* ((query (if params
                    (concat "?" (mapconcat
                                 (lambda (p)
                                   (concat (url-hexify-string (car p))
                                           "="
                                           (url-hexify-string (cdr p))))
                                 params "&"))
                  ""))
         (url (concat cookie-bridge-url path query))
         (url-request-method "GET")
         (url-show-status nil)
         (buf nil))
    (condition-case nil
        (progn
          (setq buf (url-retrieve-synchronously url t nil 3))
          (when buf
            (with-current-buffer buf
              (goto-char (point-min))
              (when (re-search-forward "\n\n" nil t)
                (let ((json-object-type 'alist)
                      (json-array-type 'list)
                      (json-key-type 'symbol))
                  (json-read))))))
      (error nil))))

(defun cookie--bridge-available-p ()
  "检查 Bridge 服务是否可用。"
  (let ((result (cookie--http-request "/health")))
    (and result (alist-get 'extension result))))

;;; — Public API —————————————————————————————————————

(defun cookie-get (domain &optional name browser)
  "获取指定 DOMAIN 的 Cookie 值（字符串）。

如果提供了 NAME，返回该特定 Cookie 的值。
否则返回该域名所有 Cookie（每行一个 name=value）。
BROWSER 指定浏览器类型，nil 使用 `cookie-default-browser'。

当 `cookie-prefer-bridge' 非 nil 时，优先尝试 Bridge HTTP API，
失败则回退到 CLI 调用。"
  (let* ((br (or browser cookie-default-browser))
         (cache-key (list 'get br domain name))
         (cached (cookie--cache-get cache-key)))
    (or cached
        (cookie--cache-put
         cache-key
         (or (and cookie-prefer-bridge
                  (cookie--get-via-bridge domain name))
             (cookie--get-via-cli domain name br))))))

(defun cookie-get-value (domain name &optional browser)
  "获取指定 DOMAIN 和 NAME 的 Cookie 值。
`cookie-get' 的便捷包装。"
  (cookie-get domain name browser))

(defun cookie-header (domain &optional browser)
  "获取 DOMAIN 的所有 Cookie，以 HTTP Cookie 头格式返回。

返回格式如：\"name1=val1; name2=val2\"。
可直接用于 restclient 的 Cookie 头。"
  (let* ((br (or browser cookie-default-browser))
         (cache-key (list 'header br domain nil))
         (cached (cookie--cache-get cache-key)))
    (or cached
        (cookie--cache-put
         cache-key
         (or (cookie--header-via-bridge domain)
             (cookie--header-via-cli domain br))))))

(defun cookie-http-get (domain &optional name)
  "通过 Bridge HTTP API 获取 Cookie 值。不回退到 CLI。
失败时返回空字符串。"
  (let* ((cache-key (list 'http-get nil domain name))
         (cached (cookie--cache-get cache-key)))
    (or cached
        (cookie--cache-put cache-key
                           (or (cookie--get-via-bridge domain name) "")))))

;;; — Internal: Bridge ——————————————————————————————

(defun cookie--get-via-bridge (domain &optional name)
  "通过 Bridge API 获取 Cookie。返回值字符串或 nil。"
  (let* ((params (list (cons "domain" domain)))
         (_ (when name
              (setq params (append params (list (cons "name" name))))))
         (result (cookie--http-request "/cookies" params)))
    (when (and result (alist-get 'ok result))
      (let ((cookies (alist-get 'cookies result)))
        (if name
            (let ((found (seq-find (lambda (c) (equal (alist-get 'name c) name))
                                   cookies)))
              (when found (format "%s" (alist-get 'value found))))
          (mapconcat
           (lambda (c)
             (format "%s=%s" (alist-get 'name c) (alist-get 'value c)))
           cookies "\n"))))))

(defun cookie--header-via-bridge (domain)
  "通过 Bridge API 获取 Cookie 头格式字符串。"
  (let* ((params (list (cons "domain" domain)
                       (cons "format" "header")))
         (result (cookie--http-request "/cookies" params)))
    (when (and result (alist-get 'ok result))
      (let ((header (alist-get 'header result)))
        (if header
            (format "%s" header)
          (let ((cookies (alist-get 'cookies result)))
            (mapconcat
             (lambda (c)
               (format "%s=%s" (alist-get 'name c) (alist-get 'value c)))
             cookies "; ")))))))

;;; — Internal: CLI —————————————————————————————————

(defun cookie--get-via-cli (domain &optional name browser)
  "通过 cookie-cli 获取 Cookie。返回值字符串或空字符串。"
  (let* ((br (or browser cookie-default-browser))
         (args (list "get" "-domain" domain "-browser" br)))
    (when name (setq args (append args (list "-name" name))))
    (or (apply #'cookie--call-cli args) "")))

(defun cookie--header-via-cli (domain &optional browser)
  "通过 cookie-cli 获取 Cookie 头格式字符串。"
  (let* ((br (or browser cookie-default-browser))
         (args (list "get" "-domain" domain "-browser" br "-format" "header")))
    (or (apply #'cookie--call-cli args) "")))

;;; — {{cookie:...}} 占位符 ————————————————————————

(defconst cookie--restclient-syntax-regexp
  (concat "{{[[:space:]]*cookie:[[:space:]]*"
          "\\([^ }]+\\)\\(?:[[:space:]]+\\([^ }]+\\)\\)?"
          "[[:space:]]*}}")
  "匹配 {{cookie:domain}} 或 {{cookie:domain name}} 占位符的正则。")

;;;###autoload
(defun cookie-update-restclient-vars ()
  "扫描当前 restclient buffer 中的 {{cookie:...}} 占位符，批量刷新为变量。

占位符形式：{{cookie:domain}} 或 {{cookie:domain name}}。
刷新后的变量名形如 cookie:domain 或 cookie:domain name。
若获取失败（值为空），清除对应的旧变量，避免残留过期值。"
  (interactive)
  (require 'restclient)
  (save-excursion
    (goto-char (point-min))
    (while (re-search-forward cookie--restclient-syntax-regexp nil t)
      (let* ((domain (match-string-no-properties 1))
             (name (match-string-no-properties 2))
             (var-name (if name
                           (format "cookie:%s %s" domain name)
                         (format "cookie:%s" domain)))
             (value (cookie-get domain name)))
        (if (or (null value) (string-empty-p value))
            (restclient-remove-var var-name)
          (restclient-set-var var-name value)))))
  (message "Cookie 变量已更新"))

;;; — verb.el 集成 —————————————————————————————————

(defun cookie-request-hook (rs)
  "Verb-Map-Request 钩子函数：从请求 URL 提取 DOMAIN，自动注入 Cookie 头。
适用于 :Verb-Map-Request: 属性，自动完成浏览器 Cookie 注入。"
  (require 'url-parse)
  (let* ((url-str (oref rs url))
         (url-obj (url-generic-parse-url url-str))
         (host (url-host url-obj))
         (cookie-val (cookie-header host)))
    (unless (string-empty-p cookie-val)
      (oset rs headers
            (append (oref rs headers)
                    (list (cons "Cookie" cookie-val)))))
    rs))

(defun cookie-json-block-body (src-block-name)
  "返回当前 buffer 中 #+name: SRC-BLOCK-NAME 对应源块的正文字符串。
找不到块时 signal `error'。"
  (require 'ob-core)
  (let ((loc (org-babel-find-named-block src-block-name)))
    (unless loc
      (error "cookie: 未找到名为 %s 的 Org 源块" src-block-name))
    (save-excursion
      (goto-char loc)
      (org-element-property :value (org-element-at-point)))))

(defun cookie-json-block-to-alist (src-block-name)
  "将 #+name: SRC-BLOCK-NAME 的 json 源块解析为 alist（不执行 Babel）。
JSON 对象解析为 alist，数组解析为 list。"
  (let ((json-object-type 'alist)
        (json-array-type 'list))
    (json-read-from-string (cookie-json-block-body src-block-name))))

(defun cookie-json-block-to-string (src-block-name)
  "将 #+name: SRC-BLOCK-NAME 的 json 源块序列化为 JSON 字符串字面量。"
  (prin1-to-string (json-encode (cookie-json-block-to-alist src-block-name))))

;;; — Interactive commands —————————————————————————

;;;###autoload
(defun cookie-get-interactive (domain name browser)
  "交互式获取 Cookie 值并复制到剪贴板。"
  (interactive
   (list (read-string "域名: ")
         (read-string "Cookie 名称 (留空获取全部): ")
         (completing-read "浏览器: " '("chrome" "firefox" "edge")
                          nil nil nil nil cookie-default-browser)))
  (let* ((cookie-name (if (string-empty-p name) nil name))
         (value (cookie-get domain cookie-name browser)))
    (if (or (null value) (string-empty-p value))
        (message "未找到 Cookie: %s@%s [%s]" (or name "*") domain browser)
      (kill-new value)
      (message "Cookie 值已复制到剪贴板: %s"
               (if (> (length value) 60)
                   (concat (substring value 0 57) "...")
                 value)))))

;;;###autoload
(defun cookie-list-domains ()
  "列出 Bridge 服务已知的所有域名。"
  (interactive)
  (let ((result (cookie--http-request "/domains")))
    (if (and result (alist-get 'ok result))
        (let ((domains (alist-get 'domains result)))
          (with-current-buffer (get-buffer-create "*Cookie Domains*")
            (erase-buffer)
            (dolist (d domains) (insert d "\n"))
            (goto-char (point-min))
            (display-buffer (current-buffer))))
      (message "无法获取域名列表（Bridge 服务可能未运行）"))))

(provide 'cookie)
;;; cookie.el ends here
