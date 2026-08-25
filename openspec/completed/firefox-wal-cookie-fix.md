# 修复 Firefox WAL 模式下 Cookie 读取不全（缺失 token）

## 当前状态
- 阶段：Phase 4（Review & Document）· 已完成
- 已确认：方案 ✅（用户 2026-08-25 确认）、计划 ✅（用户 2026-08-25 确认）
- 已完成：实现 + L1/L2/L3 验证 + 二次验证均通过

## 目标

修复 `cookie-cli get -domain kso.net -browser firefox` 返回的 Cookie 缺少 `token` / `authority_token` 的问题。根因是 Firefox 使用 SQLite WAL 模式，最近写入的 Cookie 停留在 `cookies.sqlite-wal` 日志文件中未合并进主文件，而当前读取逻辑丢弃了 WAL 数据。

## 设计

### 根因

1. `FirefoxStore.openDB` 首次尝试 `sql.Open("sqlite3", dbPath+"?mode=ro&immutable=1")` —— `immutable=1` 会明确让 SQLite 忽略 WAL 日志文件。
2. 回退路径 `copyToTemp` 只复制主文件 `cookies.sqlite`，不复制 `-wal` / `-shm` 伴生文件，SQLite 打开时无 WAL 可应用。

实测验证（sqlite3）：
- 只复制主文件（当前行为）：读不到 `token` / `authority_token`
- 主文件 + WAL + SHM 一起复制到同一目录保持同名，`mode=ro` 打开：能读到完整数据
- 直接 `mode=ro` 读原文件（跨 WSL2 运行时）：报 `disk I/O error (10)`

### 修复方案

重写 `FirefoxStore.openDB`：
- 去掉 `immutable=1` 直读原文件的优化路径（在 WAL 模式下有害，且实测直读原文件报 I/O 错误）。
- 新增复制三件套（`cookies.sqlite` + `cookies.sqlite-wal` + `cookies.sqlite-shm`）到独立临时目录并保持同名。
- 用 `mode=ro`（不带 immutable）打开临时目录中的主文件，让 SQLite 自动应用 WAL。
- `cleanup` 改为删除整个临时目录。

### WSL2 跨边界复制（关键）

Firefox 运行时通过 `/mnt/c/` 访问 Windows 侧 Cookie 文件会被独占锁定，Linux 端 `os.Open` + `io.Copy` 会失败。因此 `copyFirefoxDB` 在 WSL2 场景（源路径以 `/mnt/` 开头）下必须复用 `store.go` 中已有的 Windows 端复制能力：
- `wslPathToWindows`（已存在）：`/mnt/c/...` → `C:\...`
- `copyViaCreateFileW`（已存在）：PowerShell `CreateFileW` 以共享读方式复制被锁文件
- `copyViaCmdCopy`（已存在）：回退用 `cmd.exe copy`
- `moveFromWindows`（已存在）：把 Windows 临时文件搬回 Linux 侧

`firefox.go` 与 `store.go` 同属 `cookie` 包，可直接复用上述函数。`copyFirefoxDB` 对三件套中的每个文件：源以 `/mnt/` 开头时，先经 `wslPathToWindows` 转出 Windows 路径，用 `copyViaCreateFileW` 复制到 Windows 临时路径（失败则回退 `copyViaCmdCopy`），再用 `moveFromWindows` 把复制好的文件搬回 Linux 侧并放入目标目录；非 WSL2 时走本地 `os.Open` + `io.Copy`。

### 架构取舍

- 仅改 `internal/cookie/firefox.go`，不动 ChromeStore / EdgeStore（两者 Cookie 值本身加密，SQLite 直读只是回退，WAL 应用收益有限，避免回归）。
- 不新增对外接口，不改变 CLI 行为。
- 复用 `store.go` 现有 WSL2 复制函数，不重复实现跨边界复制逻辑。

## 任务

- [x] 任务 1：在 `internal/cookie/firefox.go` 新增 `copyFirefoxDB` 函数与 `copyFileTo` 辅助函数，并补充 `io` import。`copyFileTo(dst, src)` 复制单个文件到指定目标路径；当 `src` 以 `/mnt/` 开头（WSL2）时，经 `wslPathToWindows` 转换路径后，用 `copyViaCreateFileW` 复制到 Windows 临时路径（失败回退 `copyViaCmdCopy`），再用 `moveFromWindows` 搬回 Linux 侧并放入 `dst`，用完删除 Windows 临时文件；否则用本地 `os.Open` + `io.Copy`；对 `-shm` 文件不存在时跳过且不报错。`copyFirefoxDB` 创建独立临时目录（`os.MkdirTemp`），把 `cookies.sqlite`、`cookies.sqlite-wal`、`cookies.sqlite-shm` 三件套复制进去并保持 `cookies.sqlite` 原名，返回临时目录路径（依赖：无）
- [x] 任务 2：重写 `FirefoxStore.openDB`，去掉 `immutable=1` 直读分支，改为调用 `copyFirefoxDB` 得到临时目录，用 `sql.Open("sqlite3", 目录/cookies.sqlite+"?mode=ro")` 打开；`cleanup` 用 `os.RemoveAll` 删除整个临时目录（依赖：任务 1）
- [x] 任务 3：L1 验证 —— `go build ./...` 编译通过 + `gofmt -l` 无输出 + `go vet ./...` 通过（依赖：任务 2）
- [x] 任务 4：L2 验证 —— 运行 `./cookie-cli get -domain kso.net -browser firefox`，确认返回包含 `token=` 与 `authority_token=` 两行（依赖：任务 3）

## 文档清单

| 文档 | 处理 |
|------|------|
| README.md（"Cookie 获取策略" Firefox 段） | 补充 WAL 三件套说明 |
| CHANGELOG.md | 记录本次修复 |

（本项目无 API 文档 / 错误码 / 配置文档）

## 验证标准

- L1：`go build ./...` 通过，`gofmt -l` 无输出，`go vet ./...` 通过
- L2：`./cookie-cli get -domain kso.net -browser firefox` 输出包含 `token=` 与 `authority_token=` 两行
- L3：改动仅限 `internal/cookie/firefox.go`；不引入循环依赖；复用而非重复实现 WSL2 复制逻辑

## 注意事项

- 复制三件套必须放入同一临时目录且主文件名保持 `cookies.sqlite`（不含随机前缀），否则 SQLite 无法按约定名找到 WAL。
- `-shm` 文件可能不存在（Firefox 未运行时），复制时忽略其不存在错误。
- `-wal` 文件也可能不存在（Firefox 正常关闭并 checkpoint 后），同样忽略不存在错误；只有主文件 `cookies.sqlite` 必须存在（缺失时报错）。
- 临时目录在 `cleanup` 时用 `os.RemoveAll` 删除。
- 复用 `store.go` 的 `copyViaCreateFileW` / `copyViaCmdCopy` / `moveFromWindows` / `wslPathToWindows`，不要在 firefox.go 里重复实现。
- WSL2 分支生成的 Windows 临时文件（`C:\Windows\Temp\...`）需在使用后删除，参考 `store.go` 中 `copyToTempViaWindows` 的 `defer os.Remove` 模式。
- 不修改 `store.go`、`edge.go`、ChromeStore 相关逻辑。
