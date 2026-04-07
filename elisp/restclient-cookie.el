;;; restclient-cookie.el --- 从浏览器提取 Cookie 并与 restclient.el 集成 -*- lexical-binding: t; -*-

;; Author:
;; Version: 0.5.2
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
;;    :token := (restclient-cookie-get "api.example.com" "auth_token")
;;
;; 2. HTTP 模式 — 调用 cookie-cli serve 的 HTTP API：
;;    :token := (restclient-cookie-http-get "api.example.com" "auth_token")
;;
;; restclient.el 集成示例:
;;
;;   :token := (restclient-cookie-get "api.example.com" "auth_token")
;;   GET https://api.example.com/user
;;   Authorization: Bearer :token
;;
;;   # 获取所有 Cookie 并以 header 格式注入
;;   :cookies := (restclient-cookie-header "api.example.com")
;;   GET https://api.example.com/data
;;   Cookie: :cookies

;;; Code:

(require 'json)
(require 'url)

;;; — Customization ———————————————————————————————————

(defgroup restclient-cookie nil
  "从浏览器提取 Cookie 并与 restclient.el 集成"
  :group 'tools
  :group 'convenience)

(defcustom restclient-cookie-cli-path "cookie-cli"
  "cookie-cli 可执行文件的路径。"
  :type 'string
  :group 'restclient-cookie)

(defcustom restclient-cookie-default-browser "chrome"
  "默认使用的浏览器类型。"
  :type '(choice (const "chrome") (const "firefox") (const "edge"))
  :group 'restclient-cookie)

(defcustom restclient-cookie-bridge-url "http://127.0.0.1:8008"
  "Cookie Bridge 服务的 URL。"
  :type 'string
  :group 'restclient-cookie)

(defcustom restclient-cookie-cache-expire 300
  "Cookie 缓存过期时间（秒）。设为 0 禁用 Emacs 侧缓存。

与 cookie-cli 对齐：`restclient-cookie--call-cli' 在调用 `cookie-cli get' 时，若本值大于 0，
会追加命令行参数 `-cache-expire'，用于限制 ~/.cookie/export.json 回退文件的最大年龄（秒）；
若为 0，则不传该参数，cookie-cli 沿用其默认（环境变量 COOKIE_CACHE_EXPIRE 或 300 秒）。"
  :type 'integer
  :group 'restclient-cookie)

(defcustom restclient-cookie-prefer-bridge t
  "非 nil 时优先通过 Bridge HTTP API 获取 Cookie，失败再回退到 CLI。"
  :type 'boolean
  :group 'restclient-cookie)

;;; — Cache ——————————————————————————————————————————

(defvar restclient-cookie--cache (make-hash-table :test 'equal)
  "Cookie 缓存。键为 (method browser domain name)，值为 (value . timestamp)。")

(defun restclient-cookie--cache-get (key)
  "从缓存获取 KEY 对应的值，过期则返回 nil。"
  (when (> restclient-cookie-cache-expire 0)
    (let ((entry (gethash key restclient-cookie--cache)))
      (when (and entry
                 (< (- (float-time) (cdr entry)) restclient-cookie-cache-expire))
        (car entry)))))

(defun restclient-cookie--cache-put (key value)
  "将 VALUE 写入缓存 KEY。"
  (when (> restclient-cookie-cache-expire 0)
    (puthash key (cons value (float-time)) restclient-cookie--cache))
  value)

(defun restclient-cookie--cache-clear (&optional msg)
  "清空哈希表 `restclient-cookie--cache'；若 MSG 非空则 `message' 显示。"
  (clrhash restclient-cookie--cache)
  (when msg (message "%s" msg)))

(defun restclient-cookie-clear-cache ()
  "清除所有 Cookie 缓存。"
  (interactive)
  (restclient-cookie--cache-clear "Cookie 缓存已清除"))

;;;###autoload
(defun restclient-cookie-refresh-cache ()
  "丢弃全部缓存，下次获取将重新从浏览器拉取。
适合在重新执行 restclient 请求前调用，以使用最新登录态。"
  (interactive)
  (restclient-cookie--cache-clear "Cookie 缓存已刷新，下次将从浏览器重新获取"))

;;; — CLI backend ————————————————————————————————————

(defun restclient-cookie--call-cli (&rest args)
  "调用 cookie-cli，传递 ARGS。返回 stdout（去除尾部换行）。出错时返回 nil。
对 `get' 子命令且 `restclient-cookie-cache-expire' 大于 0 时，自动追加 `-cache-expire'。"
  (let* ((args (if (and (> restclient-cookie-cache-expire 0)
                        (equal (car args) "get"))
                   (append args
                           (list "-cache-expire"
                                 (number-to-string restclient-cookie-cache-expire)))
                 args))
         (cmd (mapconcat #'shell-quote-argument
                         (cons restclient-cookie-cli-path args) " ")))
    (condition-case err
        (let ((output (string-trim-right (shell-command-to-string cmd))))
          (if (string-prefix-p "未找到" output)
              nil
            output))
      (error
       (message "cookie-cli 调用失败: %s" (error-message-string err))
       nil))))

;;; — HTTP backend (Bridge) —————————————————————————

(defun restclient-cookie--http-request (path &optional params)
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
         (url (concat restclient-cookie-bridge-url path query))
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

(defun restclient-cookie--bridge-available-p ()
  "检查 Bridge 服务是否可用。"
  (let ((result (restclient-cookie--http-request "/health")))
    (and result (alist-get 'extension result))))

;;; — Public API —————————————————————————————————————

(defun restclient-cookie-get (domain &optional name browser)
  "获取指定 DOMAIN 的 Cookie 值（字符串）。

如果提供了 NAME，返回该特定 Cookie 的值。
否则返回该域名所有 Cookie（每行一个 name=value）。
BROWSER 指定浏览器类型，nil 使用 `restclient-cookie-default-browser'。

当 `restclient-cookie-prefer-bridge' 非 nil 时，优先尝试 Bridge HTTP API，
失败则回退到 CLI 调用。"
  (let* ((br (or browser restclient-cookie-default-browser))
         (cache-key (list 'get br domain name))
         (cached (restclient-cookie--cache-get cache-key)))
    (or cached
        (restclient-cookie--cache-put
         cache-key
         (or (and restclient-cookie-prefer-bridge
                  (restclient-cookie--get-via-bridge domain name))
             (restclient-cookie--get-via-cli domain name br))))))

(defun restclient-cookie-http-get (domain &optional name)
  "通过 Bridge HTTP API 获取 Cookie 值。不回退到 CLI。"
  (let* ((cache-key (list 'http-get nil domain name))
         (cached (restclient-cookie--cache-get cache-key)))
    (or cached
        (restclient-cookie--cache-put cache-key
                           (or (restclient-cookie--get-via-bridge domain name) "")))))

(defun restclient-cookie-header (domain &optional browser)
  "获取 DOMAIN 的所有 Cookie，以 HTTP Cookie 头格式返回。

返回格式如：\"name1=val1; name2=val2\"。
可直接用于 restclient 的 Cookie 头。"
  (let* ((br (or browser restclient-cookie-default-browser))
         (cache-key (list 'header br domain nil))
         (cached (restclient-cookie--cache-get cache-key)))
    (or cached
        (restclient-cookie--cache-put
         cache-key
         (or (restclient-cookie--header-via-bridge domain)
             (restclient-cookie--header-via-cli domain br))))))

(defun restclient-cookie-get-value (domain name &optional browser)
  "获取指定 DOMAIN 和 NAME 的 Cookie 值。`restclient-cookie-get' 的便捷包装。"
  (restclient-cookie-get domain name browser))

;;; — Internal: Bridge ——————————————————————————————

(defun restclient-cookie--get-via-bridge (domain &optional name)
  "通过 Bridge API 获取 Cookie。返回值字符串或 nil。"
  (let* ((params (list (cons "domain" domain)))
         (_ (when name (setq params (append params (list (cons "name" name))))))
         (result (restclient-cookie--http-request "/cookies" params)))
    (when (and result (alist-get 'ok result))
      (let ((cookies (alist-get 'cookies result)))
        (if name
            (let ((found (seq-find (lambda (c) (equal (alist-get 'name c) name))
                                   cookies)))
              (when found (format "%s" (alist-get 'value found))))
          (mapconcat (lambda (c)
                       (format "%s=%s" (alist-get 'name c) (alist-get 'value c)))
                     cookies "\n"))))))

(defun restclient-cookie--header-via-bridge (domain)
  "通过 Bridge API 获取 Cookie 头格式字符串。"
  (let* ((params (list (cons "domain" domain)
                       (cons "format" "header")))
         (result (restclient-cookie--http-request "/cookies" params)))
    (when (and result (alist-get 'ok result))
      (let ((header (alist-get 'header result)))
        (if header
            (format "%s" header)
          (let ((cookies (alist-get 'cookies result)))
            (mapconcat (lambda (c)
                         (format "%s=%s" (alist-get 'name c) (alist-get 'value c)))
                       cookies "; ")))))))

;;; — Internal: CLI —————————————————————————————————

(defun restclient-cookie--get-via-cli (domain &optional name browser)
  "通过 cookie-cli 获取 Cookie。返回值字符串或空字符串。"
  (let* ((br (or browser restclient-cookie-default-browser))
         (args (list "get" "-domain" domain "-browser" br)))
    (when name (setq args (append args (list "-name" name))))
    (or (apply #'restclient-cookie--call-cli args) "")))

(defun restclient-cookie--header-via-cli (domain &optional browser)
  "通过 cookie-cli 获取 Cookie 头格式字符串。"
  (let* ((br (or browser restclient-cookie-default-browser))
         (args (list "get" "-domain" domain "-browser" br "-format" "header")))
    (or (apply #'restclient-cookie--call-cli args) "")))

;;; — Interactive commands —————————————————————————

;;;###autoload
(defun restclient-cookie-get-interactive (domain name browser)
  "交互式获取 Cookie 值并复制到剪贴板。"
  (interactive
   (list (read-string "域名: ")
         (read-string "Cookie 名称 (留空获取全部): ")
         (completing-read "浏览器: " '("chrome" "firefox" "edge")
                          nil nil nil nil restclient-cookie-default-browser)))
  (let* ((cookie-name (if (string-empty-p name) nil name))
         (value (restclient-cookie-get domain cookie-name browser)))
    (if (or (null value) (string-empty-p value))
        (message "未找到 Cookie: %s@%s [%s]" (or name "*") domain browser)
      (kill-new value)
      (message "Cookie 值已复制到剪贴板: %s"
               (if (> (length value) 60)
                   (concat (substring value 0 57) "...")
                 value)))))

;;;###autoload
(defun restclient-cookie-list-domains ()
  "列出 Bridge 服务已知的所有域名。"
  (interactive)
  (let ((result (restclient-cookie--http-request "/domains")))
    (if (and result (alist-get 'ok result))
        (let ((domains (alist-get 'domains result)))
          (with-current-buffer (get-buffer-create "*Cookie Domains*")
            (erase-buffer)
            (dolist (d domains) (insert d "\n"))
            (goto-char (point-min))
            (display-buffer (current-buffer))))
      (message "无法获取域名列表（Bridge 服务可能未运行）"))))

(provide 'restclient-cookie)
;;; restclient-cookie.el ends here
