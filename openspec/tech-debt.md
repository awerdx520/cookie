# 技术债记录

> 条目格式：现象 / 影响 / 建议修复 / 优先级 / 来源。登记后不擅自修复，修复完成时移入「已解决」并注明日期。

## 未解决

### TD-1 cookie-get-interactive 把 Cookie 值写入系统剪贴板并回显

- **来源**：pre-existing，2026-11-05 安全审查发现，非本次改动引入
- **现象**：`cookie-get-interactive`（`elisp/cookie.el` 交互命令分支）对取到的值执行 `(kill-new value)`，随后 `message` 回显该值（长度 > 60 时取 `(substring value 0 57)` + `"..."`）。
- **影响**：① WSL 下 `kill-new` 与 Windows 剪贴板同步，Cookie 值进入 Windows「剪贴板历史」（Win+V 可回溯，可能再随云端同步扩散）；② `*Messages*` 可持久化到磁盘（`message-log-max` 非 nil 时），前 57 字符的会话凭据留在日志里；③ 屏幕回显在共享屏幕/录屏场景直接泄露。
- **建议修复**：复制时绑定 `(let ((interprogram-cut-function nil)) (kill-new value))` 阻断跨进程剪贴板同步；回显改为只报长度与域名（如 `已复制 128 字符 → example.com`），不输出任何值片段。
- **优先级**：低-中

### TD-2 cookie--http-request 不回收响应 buffer，且未抑制 cookie 存储

- **来源**：pre-existing，2026-11-05 安全审查发现，非本次改动引入
- **现象**：`cookie--http-request` 用 `(url-retrieve-synchronously url t nil 3)` 取响应，`buf` 用完从不 `kill-buffer`；第三个参数 INHIBIT-COOKIES 为 `nil`。
- **影响**：① 响应 buffer（含完整 Cookie JSON 明文，`cookie-list-domains` 场景含全部域名）驻留内存直至 Emacs 退出，`*http*` 类临时 buffer 还会出现在 buffer 列表里，任何 buffer 列举/会话恢复都可能暴露；② `nil` 使 `url` 库把 Bridge 响应中的 `Set-Cookie` 写入 `url-cookie` 存储并落盘（`~/.emacs.d/url/cookies`），与「只读取浏览器 Cookie」的设计意图相反，形成第二处持久化副本。
- **建议修复**：`unwind-protect` 包裹，`when (buffer-live-p buf) (kill-buffer buf)`；第三参改 `t`（抑制 cookie 存储）。
- **优先级**：低-中

### TD-3 cookie-bridge-url 不做传输层校验

- **来源**：pre-existing，2026-11-05 安全审查发现，非本次改动引入
- **现象**：`cookie-bridge-url` 默认 `http://127.0.0.1:8008`，但取值只经 `(concat cookie-bridge-url path query)` 拼接，`defcustom` 与运行期都不校验 host/scheme。
- **影响**：用户被诱导或误配成 `http://<外网地址>:8008`（或 https 之外的自定义 scheme）时，含 Cookie 值的请求会明文发往远端；配合 TD-2 的第二参 `t`（SILENT）与 `condition-case … (error nil)`，失败静默、外传成功也无迹可循。
- **建议修复**：在 `cookie--http-request` 入口校验解析出的 host ∈ {`127.0.0.1`, `localhost`, `::1`}，或 scheme 为 `https`；不满足时 `user-error` 并给出提示（`cookie-bridge-url` 的 `:set` 里做同样校验，让配置阶段就失败）。
- **优先级**：低-中

### TD-4 cookie-request-hook 只按 host 匹配，忽略 scheme/port

- **来源**：pre-existing，2026-11-05 安全审查发现，非本次改动引入
- **现象**：`cookie-request-hook` 从 `(url-host (url-generic-parse-url url-str))` 取 host，直接 `cookie-header` 注入 `Cookie` 头，不区分目标请求是 `https` 还是 `http`。
- **影响**：浏览器侧标记 `Secure` 的 Cookie 会被注入到明文 `http://` 请求（verb/restclient 里写 `http://` 即可），在链路上可被嗅探；也不会因 port 不同而拒绝（同一 host 的测试端口/其他服务同样收到凭据）。
- **建议修复**：新增 `cookie-require-https`（默认 `t`），hook 内解析 scheme 非 `https` 时跳过注入并 `message` 说明；需要时可按 host+scheme+port 精确匹配。
- **优先级**：低-中

### TD-5 cookie--get-via-cli 对以 `-` 开头的 domain/name 无校验

- **来源**：pre-existing，2026-11-05 安全审查发现，非本次改动引入
- **现象**：`cookie--get-via-cli` 以 `("get" "-domain" domain "-browser" br ["-name" name])` 组装 argv，交给 `call-process`（无 shell，故**不存在**命令注入），但 domain/name 未校验是否以 `-` 开头。
- **影响**：值形如 `-browser` / `-name` 时会被 cookie-cli 的 flag parser 误吞，造成参数错位——「取单个 Cookie」可能退化为「返回该域名全部 Cookie」（多值写进 `Cookie` 头或变量），或取值串到其他 flag 上；结果错误但不报错（`未找到` 才返回 nil）。
- **建议修复**：入口校验 `(string-prefix-p "-" domain)` / 同理 name，命中即 `user-error`；或改用 `--domain=<value>` 形式传参。
- **优先级**：低-中

### TD-6 cookie--cache 无容量上限，过期项只在读取时清除

- **来源**：pre-existing，2026-11-05 安全审查发现，非本次改动引入
- **现象**：`cookie--cache` 是 `equal` 测试的全局 hash table，只有 `cookie--cache-get` 命中该键时才 `remhash` 过期项；键形如 `(method browser domain name)`，不含后端标识（bridge / cli），`cookie-prefer-bridge` 切换后仍复用旧值。
- **影响**：① 长时间会话里每个「域名 × 名称 × browser × 入口」组合都留一条永不读到的条目（含 Cookie 值与时间戳），内存与敏感数据驻留随时间单调增长；② 切换后端（bridge ↔ cli）后至多 300s 内继续返回旧后端的值，排障时表现为「改了配置没生效」。
- **建议修复**：`cookie--cache-put` 时顺带清扫过期项（或达阈值时 `maphash` 全扫一遍）；key 加入后端标识（`(cons backend (list method browser domain name))`）；可再加 `cookie-cache-max-entries` 兜底。
- **优先级**：低-中

### TD-7 cookie--call-cli-once 在 cookie-cli 退出码非零时静默返回 nil

- **来源**：2026-11-05 code-reviewer N2（本次公共 API 补 autoload 修复的代码审查发现）
- **现象**：`cookie--call-cli-once` 在 `cookie-cli` 可执行文件存在但退出码非零时**静默返回 nil**（`(if (zerop exit) … nil)` 分支无 `message`），只有 `condition-case` 捕获到的 error 才打印一条 `message`；`cookie--call-cli` 的失败重试（最多两次调用）同样无任何提示。
- **影响**：CLI 内部失败（未登录浏览器、bridge 端口占用、cookie-cli 版本不兼容等）对用户不可见，表现为「该域名没有 Cookie」而非「CLI 报错」；叠加同文件 `condition-case … (error nil)` 的既有静默路径后，整条取数链路上没有任何可观测点，排障只能靠手工复跑 CLI。
- **建议修复**：`(exit-code)` 非零时 `message` 一次，只报退出码与子命令（如 `cookie-cli get 退出码 3`），**不输出** stdout/stderr 片段以免泄露 Cookie 值。
- **优先级**：低-中

### TD-8 负缓存是 per-key，失败场景下每个 (domain, name) 各自承担一次外部调用

- **来源**：2026-11-05 安全复审残余项
- **现象**：负缓存（`cookie-negative-cache-expire`）的键为 `(method browser domain name)`，粒度是 per-key；一次发送里出现多个不同 `(domain, name)` 且都取不到值时，每个组合各自触发一次外部调用（实测单次约 4.3s）。
- **影响**：失败场景（未登录、Bridge 未启动、cookie-cli 不可用等）下总耗时随变量数量线性放大，同一批请求可能累计数十秒；而这些失败往往源自**同一个进程级原因**，重复调用并无新信息。
- **建议修复**：让 `cookie--call-cli-once` 区分「进程级错误」（非零退出码 / 可执行文件缺失 / 连接失败）与「未找到」（退出码 0 但无值），对进程级错误做**全局短期熔断**（例如若干秒内直接返回 nil，不再起子进程）；`not-found` 必须保持 per-key 语义，不得一并熔断。
- **优先级**：低-中

## 已解决

2026-08-02 — cookie.el 取代 restclient-cookie.el（elisp 双轨包收敛为单一包）
2026-08-02 — cookie--call-cli stderr 合并污染输出（回归，参考 0f11f8f 修复方案）
2026-08-02 — cookie-get-interactive 对 nil 崩溃（string-empty-p nil）
