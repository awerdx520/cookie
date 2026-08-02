# Changelog

## [1.0.0] - 2026-08-02

### Changed

- Elisp 集成包收敛：`elisp/cookie.el` v1.0.0 取代 `restclient-cookie.el`（命名空间 `cookie-*`）
- 新增 Bridge HTTP 后端（`cookie-prefer-bridge` 控制 CLI/HTTP 回退）
- 新增多浏览器支持（`cookie-get` / `cookie-header` 支持 browser 参数，默认 chrome）
- 新增 `cookie-header`（Cookie 头格式）与 `cookie-http-get`（仅 HTTP 后端）
- 新增 `{{cookie:...}}` 占位符批量刷新（`cookie-update-restclient-vars`）
- 新增 `cookie-list-domains` 与 `cookie-refresh-cache`
- 修复：`cookie--call-cli` 丢弃 stderr（避免 cookie-cli 诊断日志污染输出）
- 修复：`cookie-get-interactive` 对 nil 值崩溃
- 移除：`elisp/restclient-cookie.el`（功能并入 cookie.el）
- 更新：README.md、examples/restclient-example.rest、aur/PKGBUILD 中全部引用
- 新增 org-verb (verb.el) 集成：code tag `{{(cookie-get-value ...)}}` 直接注入 Cookie，新增示例 `examples/verb-example.org`
- 修复：AUR 打包脚本 `cookie-native-install` 增加 WSL2 分支（.bat 启动器 + Windows 注册表 HKCU 注册），新增 `cookie-native-uninstall` 卸载脚本
- 新增 `cookie-cli native-install` / `native-uninstall`（Native Messaging Host 注册/移除，自动检测 WSL2）
- 新增 `cookie-cli doctor`（诊断 Native Messaging / Bridge HTTP / 文件导出 / SQLite 直读 / 扩展 ID 五模式）
- 修正扩展自动加载：新增 `cookie-cli chrome` 启动器（`--load-extension`），移除无效的 External Extensions 注册表/JSON 写入
- 明确流程边界：`cookie-cli native-install` 只自动注册 NativeMessagingHosts 并填充扩展 ID，不自动安装/复制扩展（扩展由用户手动加载：make ext-copy + chrome://extensions，或 `cookie-cli chrome` 启动）
- 移除 `cookie-cli chrome` 子命令（扩展加载改为纯手动：make ext-copy + chrome://extensions）
- 新增 `cookie-cli copy`：一键复制 Cookie Bridge 扩展到目标目录（替代 make ext-copy）
- `cookie-cli copy` 支持 `-target windows|linux` 显式指定复制目标平台（默认 auto）
