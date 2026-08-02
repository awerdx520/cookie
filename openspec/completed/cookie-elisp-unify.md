# cookie Elisp 包收敛：cookie.el v1.0.0 取代 restclient-cookie.el

## 目标
项目 Elisp 层存在双轨包：已提交的 restclient-cookie.el（v0.5.2，功能完整但命名冗余）
与正在编辑的 cookie.el（v0.1.0，轻量但缺功能、含 2 个回归 bug）。本次收敛为单一包：
以 cookie.el 为基座补齐全部能力（双后端/多浏览器/header/cache-expire/占位符），
修复回归 bug，删除 restclient-cookie.el 并更新全部引用。

## 设计
- 命名：cookie-* 前缀（反转 b245f28 决策，见 decisions.md 2026-08-02 条目）
- 新 cookie.el v1.0.0 功能全景：
  - Customization：cookie-cli-path / cookie-default-browser / cookie-bridge-url /
    cookie-cache-expire / cookie-prefer-bridge
  - 缓存：键含 (method browser domain name)，cookie-refresh-cache / cookie-clear-cache
  - CLI 后端：cookie--call-cli 丢弃 stderr（用 (list (current-buffer) nil)），
    自动追加 -cache-expire（对齐 COOKIE_CACHE_EXPIRE 语义）
  - HTTP 后端：cookie--http-request / cookie--bridge-available-p，prefer-bridge 控制回退
  - 公共 API：cookie-get（+browser）/ cookie-get-value / cookie-header / cookie-http-get
  - 占位符：cookie-update-restclient-vars 保留 {{cookie:...}} 语法，
    修复失败时旧 override 残留问题（失败清除旧值）
  - 交互：cookie-get-interactive（nil 安全 + name 可留空）、cookie-list-domains
  - 文档：lexical-binding: t、Package-Requires、checkdoc 清洁
- 迁移：删除 elisp/restclient-cookie.el，更新 README.md / examples/restclient-example.rest /
  aur/PKGBUILD 中全部 restclient-cookie 引用

## 任务
- [x] 任务1：重建 openspec 基础设施（active/completed/decisions.md/tech-debt.md）
      验收：目录与文件存在，decisions.md 含 2026-08-02 反转决策（依赖：无）
- [x] 任务2：实现 elisp/cookie.el v1.0.0 完整包
      验收：check-parens 通过、flymake 无 error、函数清单齐全、含双后端/占位符/
      list-domains/refresh-cache/header（依赖：任务1）
- [x] 任务3：删除 restclient-cookie.el 并更新全部引用
      验收：grep 验证仓库内无 restclient-cookie 残留（依赖：任务2）
- [x] 任务4：验证 + 文档沉淀 + 计划归档
      验收：L1/L2/L3 全过、CHANGELOG 更新、计划移至 completed/（依赖：任务3）

## 文档清单
| 文档类型 | 路径 | 本次是否命中 |
|----------|------|-------------|
| README | README.md | ✅ 命中（Emacs 集成章节，2 处 restclient-cookie 引用） |
| 打包脚本 | aur/PKGBUILD | ✅ 命中（restclient-cookie.el 安装路径 1 处） |
| 示例 | examples/restclient-example.rest | ✅ 命中（14 处函数名） |
| 变更日志 | CHANGELOG.md | ✅ 命中（用户可感知变更） |

## 验证标准
- L1 语法：elisp_check_parens(cookie.el) 通过；flymake 无 error
- L2 功能：cookie--call-cli 用 (list (current-buffer) nil) 丢弃 stderr；
  cookie-get-interactive 对 nil/空值不崩溃；:var := (cookie-get-value ...) 语法可求值
- L3 不变量：仓库内 grep 无 restclient-cookie 残留；函数前缀统一 cookie-*；
  decisions.md 记录命名反转

## 注意事项
- cookie.el 当前只在 Emacs buffer 中未保存，执行任务2 前先让用户保存
- 禁止修改 Go 侧代码（internal/、cmd/）— 本计划只涉及 Elisp 层与文档
- restclient 占位符 {{...}} 不允许空格（restclient-use-var-regexp），
  占位符仅用于 header/body 展开，不能作为 :var 定义行
- 保持 restclient.el 原生 :var := (elisp) 语法为主推方式，占位符为辅助
