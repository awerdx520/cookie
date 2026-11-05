# restclient 发送请求报 void-function cookie-get-value 修复

## 当前状态
- 阶段：已完成（2026-11-05 归档）
- 已确认：方案 ✅、计划 ✅
- 审查：plan-reviewer 2 轮（blocking 已消解）、code-reviewer 2 轮（M1–M4/N1 闭环，新增 major `elisp-check` 退出码问题已修）、security-reviewer 2 轮（blocking 失败放大已修）
- 结果：autoloads 4 → 13；`(featurep 'cookie)` 仍 nil（延迟加载）而 `(fboundp 'cookie-get-value)` 为 t；负缓存 10 秒短 TTL（第二次取值 2.6µs）；`make elisp-check` 与新增 2 条 ERT 均红/绿双向验证
- 遗留：cookie/openspec/tech-debt.md TD-1…TD-8；thomas-emacs/openspec/tech-debt.md 2 条（build-31.1 陈旧产物、cookie 测试正则边界）

## 目标
**修复项**（本计划必须完成）：
1. 打开 .http/.rest 文件后直接 `C-c C-c`（restclient-http-send-current）即可发送含 Cookie 变量的请求，不再需要先手动 `M-x cookie-refresh-cache`。
2. cookie 包公共 API 具备 autoload 入口；`use-package cookie` 显式声明延迟加载语义。
3. 失败/空 Cookie 不再被缓存 300 秒；cookie-cli 冷启动失败自动重试一次。

**增强项**（顺带，非修复必需，Phase 4 复盘时区分）：
4. 新增 3 个操作键位（`oxcu` / `oxcg` / `oxcl`）。

## 根因（已核实）
| 证据 | 内容 |
|---|---|
| 运行态 | 主实例 `(featurep 'cookie)` = nil（包未加载），同期 `restclient` = t |
| autoloads | `thomas-emacs/.local/straight/build-31.1/cookie/cookie-autoloads.el` 仅 4 条：cookie-refresh-cache / cookie-update-restclient-vars / cookie-get-interactive / cookie-list-domains |
| 源码 | `cookie-get` / `cookie-get-value` / `cookie-header` / `cookie-http-get` 均无 `;;;###autoload` |
| 配置 | `lisp/init-project.el` 的 `(use-package cookie ... :general ... :init ...)` 未声明 `:defer`/`:hook`/`:demand`；general 的 `:general` 关键字使包延迟加载，且只对 `:general` 中绑定的符号注册隐式 autoload（`cookie-refresh-cache` / `cookie-clear-cache`），`cookie-get-value` 无任何 autoload 入口 |
| 用户症状 | 打开 .http → `C-c C-c` 报 `void-function cookie-get-value` → `M-x cookie-refresh-cache`（其 autoload 顺带 require 'cookie）后正常 |

因果链：包未加载 → restclient 解析 `:x := (cookie-get-value ...)` 时符号未定义 → 报错；手动调用任一有 autoload 的命令触发整包加载 → 此后正常，故每次重启 Emacs 都需重跑一次。

两层 autoload 需区分：`:general` 绑定只解决「绑定符号自身的 autoload」，不解决「restclient 里以普通函数调用形式出现的符号」；包级 `;;;###autoload` + straight autoloads 才能解决后者。

次级缺陷（事实✓）：`cookie-get` / `cookie-header` / `cookie-http-get` 对空结果也调用 `cookie--cache-put`，一次冷启动失败会被固化为 300 秒空值。

## 设计
1. **包侧根治**（`/home/thomas/Workspace/cookie/elisp/cookie.el`）：给公共 API 补 `;;;###autoload` cookie——`cookie-get`、`cookie-get-value`、`cookie-header`、`cookie-http-get`、`cookie-clear-cache`、`cookie-request-hook`、`cookie-json-block-body`、`cookie-json-block-to-alist`、`cookie-json-block-to-string`（已有的 cookie-refresh-cache / cookie-update-restclient-vars / cookie-get-interactive / cookie-list-domains 保持不动）。
2. **负缓存修复**：`cookie--cache-put` 仅在 value 为非 nil 非空字符串时写缓存；空值照常返回但不落盘，下次调用自动重试。
3. **CLI 重试**：把现有 `cookie--call-cli` 主体改名为 `cookie--call-cli-once`（完整保留 `-cache-expire` 追加逻辑与 stderr 丢弃逻辑），新增 `cookie--call-cli` 包装：调用 once 失败（nil）时 `(sleep-for 0.2)` 后重试一次；两次都失败返回 nil。包装层无法区分「未找到」与真正失败（`once` 对两者都返回 nil），故「未找到」也会被重试一次——仅多 0.2s 与一次多余 CLI 调用，无正确性影响；负缓存修复已保证空值不落盘。
4. **use-package 适配**（`/home/thomas/thomas-emacs/lisp/init-project.el`）：该块加 `:defer t` 显式声明延迟加载；`:general` 增补 `oxcu` → `cookie-update-restclient-vars`、`oxcg` → `cookie-get-interactive`、`oxcl` → `cookie-list-domains`。
5. **重生成 autoloads**：执行 `M-x straight-rebuild-package RET cookie`（或 `(straight-rebuild-package "cookie")`），使 `build-31.1/cookie/cookie.el` 副本与 `cookie-autoloads.el` 更新。

**宿主不写 `:commands`** 的依据：援引项目约定的**精神**——包内 `;;;###autoload` + straight autoloads 是标准机制，宿主不重复补偿（`lisp/init-tools.el:456` 的原文理由是 eimg 多文件包「符号名≠库名」的猜名风险，cookie 是单文件包，该字面理由不适用，但结论一致：9 个公共 API 逐个写 `:commands` 冗长且易漏）。

## 任务
- [x] 任务1：修改 `elisp/cookie.el`——补 9 处 `;;;###autoload`、负缓存修复、CLI 重试（依赖：无）
  验收：① `check-parens` 通过；② `grep -c ';;;###autoload' elisp/cookie.el` ≥ 13；③ `cookie--cache-put` 函数体内含 `string-empty-p` 判断；④ 存在 `cookie--call-cli-once` 且 `cookie--call-cli` 函数体内**调用**了它（grep 定位调用点）；⑤ `-cache-expire` 的追加逻辑出现在 `cookie--call-cli-once` 函数体内，未丢失
- [x] 任务2：修改 `lisp/init-project.el` 的 cookie 块——`:defer t` + 3 个键位（依赖：无）
  验收：`check-parens` 通过；cookie 的 use-package 块内出现 `:defer t`、`"oxcu"`、`"oxcg"`、`"oxcl"`；该块内不出现 `:commands`
- [x] 任务3：重生成 autoloads 与 build 副本（依赖：任务1）
  验收：① `grep -q "(autoload 'cookie-get-value \"cookie\"" build-32.0.50/cookie/cookie-autoloads.el`；② `grep -q "(autoload 'cookie-header \"cookie\"" build-32.0.50/cookie/cookie-autoloads.el`；③ `diff /home/thomas/Workspace/cookie/elisp/cookie.el /home/thomas/thomas-emacs/.local/straight/build-32.0.50/cookie/cookie.el` 无输出（退出码 0）
  注：`build-31.1` 为旧记录中的目录名，Emacs 版本升级后 `build-*` 目录名随之变化，以实际 `build-*` 为准（本次实测 `.local/straight/build-32.0.50/cookie/`，autoloads 4 → 13 条；`build-31.1/cookie/` 保留旧快照 4 条，属历史残留，不参与判定）
- [x] 任务4：L2 语义验证（依赖：任务3）
  **前置（必须按序）**：先 `elisp_mcp_instance(action=stop, instance=verify)`，再 `elisp_mcp_instance(action=start, instance=verify)`，轮询 `elisp_mcp_instance_status(instance=verify)` 的 `alive` 为 true（最多 10 次 × 1s）；随后 `elisp_mcp_set_task(task=verify)`。verify 不支持 `action=reset`（reset 仅对 `pure` 有效，对 verify 返回 error）；若 stop+start 后 `autoloadp` 仍为 nil，再次 stop+start 并核对 `build-31.1/cookie/cookie-autoloads.el` 已更新，禁止反复 start（按 rules/elisp-mcp.md 失败码处置表）
  验收（fresh 实例内）：① `(featurep 'cookie)` = nil（证明仍延迟）；② `(let ((def (symbol-function 'cookie-get-value))) (and (autoloadp def) (equal (nth 1 def) "cookie")))` = t；③ 调用 `(cookie-get-value "example.com" "x")` 后 `(featurep 'cookie)` = t 且无 void-function；④ `(cookie--cache-put 'k "")` 后 `(cookie--cache-get 'k)` 为 nil（空值不落盘）；⑤ `(cookie--cache-put 'k "v")` 后 `(cookie--cache-get 'k)` 为 "v"（非空仍缓存）
- [x] 任务5：L3 端到端验证（依赖：任务4）
  验收（verify 实例内）：① 构造临时 buffer 写入 `:tok := (cookie-get-value "example.com" "x")` 与一行 `GET https://example.com/`，用 `(restclient-find-vars-before-point)` 触发变量求值（更贴真实发送路径），或最小化用 `(restclient-eval-var "(cookie-get-value \"example.com\" \"x\")")`，确认不报 void-function（这是用户目标的直接模拟）；② `cookie-get-value` 对真实域名返回非空，或 CLI 不可用时记录降级结论；③ 连续调用两次结果一致、无报错
- [x] 任务6：文档与回归测试（依赖：任务1、2）
  验收：① `cookie/README.md` Emacs 集成节补充「公共 API 已带 autoload，打开 .http 直接 `C-c C-c` 即可，无需先手动刷新；仅在需要强制拿最新登录态时用 refresh-cache」（保留原有「若需最新 Cookie」的条件语义，不做绝对化删除）；② `cookie/CHANGELOG.md` 与 `thomas-emacs/CHANGELOG.md` 各新增一条记录；③ `thomas-emacs/test/init-smoke-test.el` 新增 ERT 断言，写法沿用既有 helper（参照第 4086-4114 行 `test-restclient-case-fold-search` 与第 1176 行 `:defer t` 断言）：cookie 块含 `:defer t`、含 `"oxcu"`、`(should-not (string-match-p ":commands" cookie-form))`；④ `make -C /home/thomas/thomas-emacs test` 通过

## 文档清单
| 文档 | 触发点 | 动作 |
|---|---|---|
| `cookie/README.md` | Emacs 集成节可补充「无需手动加载」 | 补充 autoload 说明；保留 refresh-cache 的「可选/需最新登录态」条件语义 |
| `cookie/CHANGELOG.md` | 包行为修复 | 新增条目 |
| `thomas-emacs/CHANGELOG.md` | 配置变更 | 新增条目 |
| `thomas-emacs/test/init-smoke-test.el` | 配置回归 | 新增 ERT 断言 |
| `cookie/openspec/decisions.md` | 无架构决策 | 不更新（autoload 补齐属缺陷修复；若 Phase 4 判定「包级 autoload 优先于宿主 :commands」为新不变量，则改记 rules/AGENTS，不记 decisions） |

## 验证标准
- L1：`check-parens` 对两个改动文件通过；`make -C /home/thomas/thomas-emacs check-parens` 通过
- L2：任务4 的 5 条 fresh 实例断言全部成立
- L3：任务5 端到端 + `make -C /home/thomas/thomas-emacs test` 全绿
- 生效验证（须用户显式要求才可写主实例，I2）：重启 daemon 后打开 .http 直接 `C-c C-c` 成功

## 注意事项
- 长耗时验证（ERT / flymake / rebuild）只允许在 `verify` 实例执行；`config` 任务下禁止
- verify 实例是持久 daemon，rebuild 后必须先 stop+start（或显式 load 新 autoloads）才是 fresh 状态，否则 `autoloadp` 假阴性
- 空值不缓存后，`cookie-http-get` 在 bridge 不可用时每次约 3 秒（`url-retrieve-synchronously` 超时）才返回 ""，属预期权衡；`cookie-get` 走 CLI 时由 0.2s 重试兜底
- 修改 cookie.el 后必须 `straight-rebuild-package`，否则 build 副本与 autoloads 不更新，修复不生效
- 不要在宿主 use-package 块写 `:commands`（结论正确；对 cookie 单文件包的理由是「易漏且 redundant」，不援引 eimg 的猜名理由）
- 不涉及 Go 代码，`make test`（Go）无需运行
- straight-cache-autoloads=t（init-package.el:18）下，straight-rebuild-package 只更新内存缓存；必须显式 (straight--save-build-cache) 落盘，否则重启后仍 eval 旧的 autoloads 快照（本次实测：fresh 实例 fboundp=cookie-get-value 为 nil，落盘后为 t）
- 审查修订（安全）：失败结果改为按 cookie-negative-cache-expire（默认 10 秒）短缓存——既避免 300 秒固化空值，又避免同一次发送内 N 个变量重复触发 CLI（实测 CLI 不可用时单次约 4.3 秒）；cookie-cli 不可执行时不再重试。
- 安全提示（security-reviewer #5）：autoload 补齐扩大了凭据读取入口的可达面（打开任意 .http/.org 即可能触发 `cookie-get-value` / `cookie-request-hook`）。仅在可信文件中使用；不加 :commands 也不会缩小该面。
