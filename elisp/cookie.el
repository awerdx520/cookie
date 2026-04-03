;;; cookie.el --- 从浏览器提取 Cookie 并与 restclient.el 集成

;; Author:
;; Version: 0.2.0
;; Keywords: restclient, chrome, firefox, cookie, authentication
;; URL: https://github.com/thomas/cookie

;;; Commentary:
;; 该包提供了从浏览器（Chrome、Firefox）提取 Cookie 的功能，并与 restclient.el 集成，
;; 方便在本地开发时自动携带云端服务的认证 Token。

;; 主要功能：
;; 1. 通过 cookie-cli 工具获取浏览器中的 Cookie 值
;; 2. 支持 Chrome 和 Firefox
;; 3. 为 restclient.el 提供便捷的变量定义语法
;; 4. 支持缓存和错误处理

;;; Code:

(defgroup cookie nil
  "从浏览器提取 Cookie 并与 restclient.el 集成"
  :group 'tools
  :group 'convenience)

(defcustom cookie-cli-path "cookie-cli"
  "cookie-cli 可执行文件的路径。"
  :type 'string
  :group 'cookie)

(defcustom cookie-default-browser "chrome"
  "默认使用的浏览器类型。
可选值: \"chrome\", \"firefox\"。"
  :type '(choice (const "chrome") (const "firefox"))
  :group 'cookie)

(defcustom cookie-cache-expire 300
  "Cookie 缓存过期时间（秒）。"
  :type 'integer
  :group 'cookie)

(defvar cookie--cache (make-hash-table :test 'equal)
  "Cookie 缓存，键为 (domain . name)，值为 (value . timestamp)。")

(defun cookie--call-cli (&rest args)
  "调用 cookie-cli 工具，传递 ARGS 参数。

返回标准输出的字符串（去除末尾换行符）。"
  (let ((cmd (concat cookie-cli-path " " (mapconcat 'identity args " "))))
    (message "执行命令: %s" cmd)
    (let ((output (shell-command-to-string cmd)))
      (if (string-match "\n+$" output)
          (replace-match "" t t output)
        output))))

(defun cookie-get (domain &optional name browser)
  "获取指定 DOMAIN 的 Cookie 值。

如果提供了 NAME，则返回该特定 Cookie 的值；
否则返回该域名的所有 Cookie（每行一个 name=value）。
BROWSER 指定浏览器类型，nil 则使用 `cookie-default-browser'。

优先从缓存中读取，缓存过期时重新调用 cookie-cli。"
  (let* ((br (or browser cookie-default-browser))
         (key (list br domain name))
         (cached (gethash key cookie--cache))
         (now (time-to-seconds (current-time))))
    (if (and cached (< (- now (cdr cached)) cookie-cache-expire))
        (car cached)
      (let ((args (list "get" "-domain" domain "-browser" br))
            (value nil))
        (when name
          (setq args (append args (list "-name" name))))
        (setq value (apply 'cookie--call-cli args))
        (puthash key (cons value now) cookie--cache)
        value))))

(defun cookie-get-value (domain name &optional browser)
  "获取指定 DOMAIN 和 NAME 的 Cookie 值。

BROWSER 指定浏览器类型，nil 则使用 `cookie-default-browser'。
这是 cookie-get 的便捷包装函数，专门用于获取单个 Cookie 值。"
  (cookie-get domain name browser))

(defun cookie-clear-cache ()
  "清除所有 Cookie 缓存。"
  (interactive)
  (clrhash cookie--cache)
  (message "Cookie 缓存已清除"))

;;;###autoload
(defun cookie-setup-restclient ()
  "设置 restclient.el 集成。

提供以下便利功能：
1. 在 restclient 缓冲区中，可以使用 `{{cookie:domain}}` 或 `{{cookie:domain name}}` 语法
2. 添加 `cookie-update-variables` 命令，用于手动更新 Cookie 变量"
  (interactive)
  (add-hook 'restclient-mode-hook #'cookie--restclient-init))

(defun cookie--restclient-init ()
  "初始化 restclient 缓冲区的 Cookie 相关功能。"
  (when (boundp 'restclient-mode)
    (make-local-variable 'cookie--restclient-vars)
    (setq cookie--restclient-vars nil)
    (add-hook 'before-save-hook #'cookie--update-restclient-vars nil t)))

(defun cookie--update-restclient-vars ()
  "更新当前 restclient 缓冲区中的所有 Cookie 变量。

此函数会查找所有 `{{cookie:...}}` 模式，并用实际值替换。"
  (interactive)
  (save-excursion
    (goto-char (point-min))
    (while (re-search-forward "{{\s*cookie:\s*\\([^ }]+\\)\\(?:\s+\\([^ }]+\\)\\)?\s*}}" nil t)
      (let* ((domain (match-string 1))
             (name (match-string 2))
             (value (cookie-get domain name))
             (start (match-beginning 0))
             (end (match-end 0)))
        (when value
          (delete-region start end)
          (insert value))))))

;; 提供简单的交互命令
;;;###autoload
(defun cookie-get-interactive (domain name browser)
  "交互式获取 Cookie 值。
BROWSER 为浏览器类型（chrome 或 firefox），默认使用 `cookie-default-browser'。"
  (interactive
   (list (read-string "域名: ")
         (read-string "Cookie 名称: ")
         (completing-read "浏览器 (默认 chrome): " '("chrome" "firefox") nil nil nil nil cookie-default-browser)))
  (let ((value (cookie-get domain name browser)))
    (if (string-empty-p value)
        (message "未找到 Cookie: %s@%s [%s]" name domain browser)
      (message "Cookie 值: %s" value)
      (kill-new value)
      (message "值已复制到剪贴板"))))

;; 提供 minor mode
(define-minor-mode cookie-auto-mode
  "自动从 Chrome 获取 Cookie 的 minor mode。

启用后，会在 restclient 请求中自动注入 Cookie 值。"
  :global nil
  :lighter " Cookie"
  (if cookie-auto-mode
      (progn
        (add-hook 'restclient-request-hook #'cookie--inject-cookies)
        (message "Cookie auto mode 已启用"))
    (remove-hook 'restclient-request-hook #'cookie--inject-cookies)
    (message "Cookie auto mode 已禁用")))

(defun cookie--inject-cookies ()
  "在发送 restclient 请求前注入 Cookie。

此函数会查找请求头中的 :cookie 变量，并用实际值替换。"
  (save-excursion
    (goto-char (point-min))
    (while (re-search-forward "^:cookie\s*=\s*\\(.*\\)$" nil t)
      (let ((line (match-string 0)))
        (when (string-match "{{\s*cookie:\s*\\([^ }]+\\)\\(?:\s+\\([^ }]+\\)\\)?\s*}}" line)
          (let* ((domain (match-string 1 line))
                 (name (match-string 2 line))
                 (value (cookie-get domain name))
                 (start (match-beginning 0))
                 (end (match-end 0)))
            (when value
              (delete-region start end)
              (insert (format ":cookie = %s" value)))))))))

(provide 'cookie)
;;; cookie.el ends here
