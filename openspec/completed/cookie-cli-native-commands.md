# cookie-cli 新增 native-install / native-uninstall / doctor 子命令

## 目标
AUR 安装后用户只有 cookie-cli 二进制，native messaging 安装功能分散在 shell 脚本且无诊断手段。
本次将安装/卸载内置为 cookie-cli 子命令（安装即用），并新增 doctor 诊断命令一键检查所有获取模式。

## 设计
- 三个新子命令：
  - `cookie-cli native-install`：注册 Native Messaging Host（自动检测 WSL2）
  - `cookie-cli native-uninstall`：移除注册
  - `cookie-cli doctor [-browser X]`：诊断 4 模式 + 扩展 ID 检查
- 文件改动：
  - internal/native/install.go（新）：InstallHost()/UninstallHost() 移植 scripts/native-install.sh 逻辑
    - WSL2 分支：cmd.exe 取 %USERPROFILE% → wslpath 转路径 → 写 .bat（wsl.exe -- cookie-cli native-messaging-host）→ 生成 manifest（path 指向 .bat）→ reg.exe 注册 HKCU
    - Linux 分支：写 ~/.config/google-chrome/NativeMessagingHosts + ~/.config/chromium/NativeMessagingHosts
    - binary 路径用 os.Executable()
  - internal/native/doctor.go（新）：4 模式诊断
    - Native Messaging：net.DialTimeout 检查 unix socket 可达性
    - Bridge HTTP：GET /health 检查服务 + extension 连接状态
    - 文件导出：~/.cookie/export.json 存在性 + 年龄
    - SQLite 直读：NewStore(browser) 只读查询
    - 附加：manifest 中 EXTENSION_ID_HERE 未替换检测
  - cmd/cookie-cli/main.go（改）：注册 3 子命令
  - README.md（改）：命令行参考更新

## 任务
- [x] 任务1：写入本计划文件
      验收：6 节完整、checkbox 未勾选（依赖：无）
- [x] 任务2：实现 internal/native/install.go
      验收：go build 通过、WSL2/Linux 分支逻辑完整（依赖：任务1）
- [x] 任务3：实现 internal/native/doctor.go
      验收：go build 通过、诊断逻辑完整（依赖：任务1）
- [x] 任务4：main.go 注册 3 子命令
      验收：go build 通过、子命令可运行（依赖：任务2,3）
- [x] 任务5：更新 README.md 命令行参考
      验收：README 与实现一致（依赖：任务4）
- [x] 任务6：验证 + 文档沉淀 + 归档
      验收：make build/vet/test 全绿、doctor 实际输出正常、CHANGELOG 追加、计划归档（依赖：任务5）

## 文档清单
| 文档类型 | 路径 | 本次是否命中 |
|----------|------|-------------|
| README | README.md | ✅ 命中（命令行参考 + 使用说明） |
| 变更日志 | CHANGELOG.md | ✅ 命中（用户可感知新命令） |
| 打包脚本 | aur/PKGBUILD | ⏭ 未命中（子命令内置后 /usr/bin/cookie-native-install 脚本仍保留，不冲突） |

## 验证标准
- L1 语法：go build ./... 通过
- L2 功能：doctor 实际运行输出 4 模式状态；native-install 在 WSL2/Linux 分支逻辑正确（真实 WSL2 需实机验证）
- L3 不变量：Makefile vet/test 通过；README 与实现一致

## 注意事项
- WSL2 检测函数（isWSL2/windowsPathToWSL/wslPathToWindows）在 internal/cookie 包未导出，在 native/install.go 内复制小量实现，不跨包导出
- 移植 shell 逻辑时注意：reg.exe 的 HKCU 路径双反斜杠转义、.bat 用 CRLF（\r\n）、manifest path 用 wslpath -w
- doctor 的 SQLite 检查用只读模式，禁止写浏览器数据库
- /usr/bin/cookie-native-install 脚本保留（兼容），新子命令与其并存
