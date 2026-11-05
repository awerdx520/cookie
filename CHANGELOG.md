# Changelog

## [未发布]

### Added

- 增强 verb.el 集成：新增 `cookie-request-hook`（`:Verb-Map-Request:` 自动注入 Cookie 头）与 `cookie-json-block-body` / `cookie-json-block-to-alist` / `cookie-json-block-to-string`（Org 源块 JSON helpers）
- 更新 README.md 与 examples/verb-example.org：补充自动注入钩子与 JSON helpers 用法示例
- 修复：`cookie-json-block-to-alist` 数组解析为 list（json-array-type 'list），与项目惯例一致

### Fixed

- 修复：restclient 中 `:x := (cookie-get-value ...)` 首次求值报 `void-function cookie-get-value`。根因是公共 API 未带包级 autoload，`(use-package cookie ...)` 延迟加载时只有 `:general` 绑定的命令有隐式 autoload，restclient 里以普通函数调用形式出现的符号无任何入口。现为 `cookie-get` / `cookie-get-value` / `cookie-header` / `cookie-http-get` / `cookie-clear-cache` / `cookie-request-hook` / `cookie-json-block-*` 补 `;;;###autoload`，`(use-package cookie)` 后直接 `C-c C-c` 即可，无需先手动 `M-x cookie-refresh-cache`
- 修复：`cookie--cache-put` 对空值（nil / 空字符串）改为按 `cookie-negative-cache-expire`（默认 10 秒）**短缓存**，不再按 `cookie-cache-expire` 固化 300 秒。此前一次冷启动失败（CLI/bridge 不可用、未登录）会被固化为 300 秒空 Cookie，下一次请求拿不到正确值也不重试；而完全不缓存又会让同一次发送内 N 个变量各触发一次 CLI（CLI 不可用时单次约 4 秒）。缓存条目改为 `(value timestamp . ttl)` 支持 per-entry TTL，`cookie--cache-get` 命中过期条目时顺手 `remhash`
- 修复：`cookie--call-cli` 失败（nil）时 `sleep-for` 0.2s 后重试一次，但仅在 `cookie-cli-path` 可执行时重试（新增 `cookie--cli-executable-p`；主体逻辑保持不变，改名为 `cookie--call-cli-once` 并保留 `-cache-expire` 追加与 stderr 丢弃）
- 修复：Firefox WAL 模式下 Cookie 读取不全（缺失 `token`/`authority_token`）。根因是 Firefox 的 `cookies.sqlite` 采用 WAL 存储，最近写入的 Cookie 停留在 `-wal` 日志未合并进主文件，而旧逻辑用 `immutable=1` 直读且回退只复制主文件，丢弃了 WAL 数据。现改为复制三件套（主文件 + `-wal` + `-shm`）到临时目录后 `mode=ro` 打开，复用 `store.go` 的 WSL2 跨边界复制能力绕过 Firefox 独占锁。

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
