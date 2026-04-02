# Cookie 提取与 Restclient 集成工具

一个从 Chrome 浏览器提取 Cookie 并与 Emacs restclient.el 集成的工具，方便本地开发时自动携带云端服务的认证 Token。

## 功能特性

- **跨平台支持**：Windows、macOS、Linux（包括 WSL2）
- **多浏览器支持**：目前支持 Chrome，可扩展支持其他浏览器
- **无缝集成**：与 Emacs restclient.el 深度集成
- **命令行工具**：提供 `cookie-cli` 命令行工具，方便脚本调用
- **缓存机制**：减少频繁读取数据库，提升性能
- **错误处理**：完善的错误提示和降级机制

## 当前限制

⚠️ **Cookie 加密问题**：Chrome 默认加密所有 Cookie 值，当前版本尚未实现解密功能。

- 获取的加密 Cookie 值将显示为 `[ENCRYPTED_V20]`、`[ENCRYPTED_DPAPI]` 或 `[ENCRYPTED_AES]`
- 需要实现对应平台的解密逻辑（Windows DPAPI、macOS Keychain、Linux 密钥环）
- 解密功能计划在后续版本中实现

**临时解决方案**：
1. 使用 Chrome 扩展（如 "EditThisCookie"）导出 Cookie 为 JSON，然后使用其他工具解析
2. 暂时使用未加密的 Cookie（不推荐，需修改 Chrome 配置）
3. 贡献代码实现解密功能

## 安装

### 1. 安装 Go 工具

```bash
go install ./cmd/cookie-cli
```

或从源码编译：

```bash
git clone <仓库地址>
cd cookie
go build -o cookie-cli ./cmd/cookie-cli
sudo mv cookie-cli /usr/local/bin/
```

### 2. 配置 Emacs

将 `elisp/cookie.el` 添加到 Emacs 加载路径：

```elisp
(add-to-list 'load-path "/path/to/cookie/elisp")
(require 'cookie)
```

或使用 `use-package`：

```elisp
(use-package cookie
  :load-path "/path/to/cookie/elisp"
  :config
  (cookie-setup-restclient))
```

## 使用说明

### 命令行工具

```bash
# 获取指定域名的所有 Cookie
cookie-cli get -domain example.com

# 获取特定 Cookie 值
cookie-cli get -domain example.com -name sessionid

# 列出所有包含 Cookie 的域名
cookie-cli list

# 启动 HTTP 服务（默认端口 8080）
cookie-cli serve -port 8080
```

### Restclient 集成

在 restclient 文件中使用以下语法：

```restclient
# 基本用法
:token = {{cookie:api.example.com auth_token}}

GET https://api.example.com/user
Authorization: Bearer :token

# 直接使用语法糖
GET https://api.example.com/data
Authorization: Bearer {{cookie:api.example.com auth_token}}
```

### 高级配置

#### 环境变量

```bash
# 指定 Chrome 用户配置目录
export COOKIE_CHROME_PROFILE="Profile 1"

# 指定浏览器类型（未来支持）
export COOKIE_BROWSER="chrome"
```

#### Emacs 配置

```elisp
;; 自定义 cookie-cli 路径
(setq cookie-cli-path "/usr/local/bin/cookie-cli")

;; 设置缓存过期时间（秒）
(setq cookie-cache-expire 600)

;; 启用自动模式
(cookie-auto-mode 1)

;; 手动更新所有 Cookie 变量
M-x cookie-update-restclient-vars

;; 交互式获取 Cookie 值
M-x cookie-get-interactive
```

## WSL2 特别说明

在 WSL2 环境中，工具会自动检测并访问 Windows 系统中的 Chrome Cookie 文件。

**前提条件**：
1. Windows Chrome 至少运行过一次（生成 Cookie 文件）
2. WSL2 可以访问 `/mnt/c/` 挂载点

**手动指定路径**（如果需要）：
```bash
export COOKIE_CHROME_PATH="/mnt/c/Users/<用户名>/AppData/Local/Google/Chrome/User Data/Default/Cookies"
```

## 故障排除

### 1. "未找到 Chrome Cookie 文件" 错误

- 确保 Chrome 已安装并至少运行过一次
- 检查路径权限，确保可以读取 Cookie 文件
- 在 WSL2 中，确保可以访问 Windows 文件系统

### 2. Cookie 值为 "[ENCRYPTED]"

Chrome 会加密存储敏感 Cookie 值。当前版本尚未实现自动解密，需要根据平台手动处理：

- **Windows**：需要处理 DPAPI 加密
- **macOS**：需要访问 Keychain
- **Linux**：需要访问 GNOME Keyring 或 KWallet

**临时解决方案**：使用 Chrome 扩展导出 Cookie，或暂时使用未加密的 Cookie。

### 3. Emacs 集成不工作

- 检查 `cookie-cli` 是否在 PATH 中
- 检查 `cookie.el` 是否正确加载
- 查看 `*Messages*` 缓冲区获取错误信息

## 开发计划

- [x] 基础 Cookie 读取功能
- [x] 命令行工具
- [x] Emacs restclient 集成
- [ ] Cookie 值解密（各平台）
- [ ] 多浏览器支持（Firefox, Edge, Safari）
- [ ] HTTP 服务模式
- [ ] 图形化配置界面

## 贡献指南

欢迎提交 Issue 和 Pull Request！

1. Fork 仓库
2. 创建功能分支
3. 提交更改
4. 推送到分支
5. 创建 Pull Request

## 许可证

MIT License
