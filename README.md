# Cookie — 浏览器 Cookie 提取与 Restclient 集成工具

从浏览器提取 Cookie 并与 Emacs restclient.el 集成，方便本地开发时自动携带云端服务的认证 Token。

## 功能特性

- **Chrome 扩展桥接**：通过 Cookie Bridge 扩展直接获取明文 Cookie，无需解密，无需关闭浏览器
- **多浏览器支持**：Chrome、Firefox、Edge
- **跨平台**：Windows、Linux、WSL2
- **Emacs 集成**：与 restclient.el 深度集成
- **HTTP API**：提供本地 REST API，方便任意工具调用

## 架构

```
                              Chrome 浏览器
curl / Emacs / 脚本            ┌──────────────────┐
       │                       │  Cookie Bridge    │
       ▼                       │  扩展 (MV3)       │
┌──────────────┐               │  chrome.cookies   │
│ cookie-cli   │◄── WebSocket ─┤  API              │
│ serve        │               └──────────────────┘
│ 127.0.0.1    │
│ :8008        │   Firefox / Edge (回退)
└──────┬───────┘   直接读取 SQLite 数据库
       │
  HTTP JSON API
```

Chrome/Edge 推荐使用**扩展模式**（不受加密版本变化影响）；Firefox Cookie 为明文存储，直接读取数据库即可。

## 快速开始

### 1. 编译

```bash
make build
# 或
go build -o cookie-cli ./cmd/cookie-cli
```

### 2. 启动 Bridge 服务

```bash
cookie-cli serve
# 默认监听 127.0.0.1:8008（仅本地访问）
```

### 3. 安装 Chrome 扩展

```bash
# WSL2 用户: 复制扩展到 Windows 目录
make ext-copy
```

然后在 Chrome 中加载扩展：

1. 打开 `chrome://extensions/`
2. 开启右上角 **开发者模式**
3. 点击 **加载已解压的扩展程序**
4. 选择 `C:\Users\<用户名>\cookie-bridge-extension` 目录

扩展加载后会自动连接 Bridge 服务。

### 4. 获取 Cookie

```bash
# CLI 方式（自动检测 Bridge 服务，不可用时回退到数据库读取）
cookie-cli get -domain example.com
cookie-cli get -domain example.com -name sessionid

# HTTP API 方式
curl 'http://127.0.0.1:8008/cookies?domain=example.com'
curl 'http://127.0.0.1:8008/cookies?domain=example.com&name=sessionid'
curl 'http://127.0.0.1:8008/domains'
curl 'http://127.0.0.1:8008/health'
```

## 命令行参考

```bash
cookie-cli get -domain <域名> [-name <名称>] [-browser <浏览器>] [-format <格式>]
cookie-cli list [-browser <浏览器>]
cookie-cli serve [-port <端口>]
```

| 子命令 | 说明 |
|--------|------|
| `get` | 获取指定域名的 Cookie |
| `list` | 列出所有包含 Cookie 的域名 |
| `serve` | 启动 Cookie Bridge 服务 |

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `-domain` | — | 目标域名 |
| `-name` | — | Cookie 名称（省略则返回该域名所有 Cookie） |
| `-browser` | `chrome` | 浏览器类型：`chrome`、`firefox`、`edge` |
| `-format` | — | 输出格式：`header`（Cookie 头格式）、`json`（JSON 数组） |
| `-port` | `8008` | Bridge 服务监听端口 |

### 输出格式

```bash
# 默认：每行 name=value
cookie-cli get -domain example.com
# sessionid=abc123
# csrftoken=xyz789

# header 格式：直接可用作 Cookie 请求头
cookie-cli get -domain example.com -format header
# sessionid=abc123; csrftoken=xyz789

# JSON 格式
cookie-cli get -domain example.com -format json
# [{"name":"sessionid","value":"abc123"}, ...]

# 获取单个 Cookie 值（纯值输出，无换行）
cookie-cli get -domain example.com -name sessionid
# abc123
```

### 环境变量

| 变量 | 说明 |
|------|------|
| `COOKIE_BROWSER` | 默认浏览器类型 |
| `COOKIE_PORT` | Bridge 服务端口（CLI 连接时使用） |

## HTTP API

Bridge 服务仅监听 `127.0.0.1`，外部无法访问。

### GET /cookies

获取指定域名的 Cookie。

```
GET /cookies?domain=example.com
GET /cookies?domain=example.com&name=sessionid
```

响应：

```json
{
  "ok": true,
  "cookies": [
    {
      "name": "sessionid",
      "value": "abc123",
      "domain": ".example.com",
      "path": "/",
      "secure": true,
      "httpOnly": true,
      "expirationDate": 1800000000,
      "sameSite": "lax"
    }
  ]
}
```

#### format 参数

| 值 | 说明 |
|------|------|
| （默认） | 返回 JSON 对象，含 `cookies` 数组 |
| `header` | JSON 响应中额外包含 `header` 字段（`"name1=val1; name2=val2"` 格式） |
| `raw` | 直接返回纯文本的 Cookie 头字符串（`text/plain`） |

```bash
# header 格式 — JSON 响应中包含 header 字段
curl 'http://127.0.0.1:8008/cookies?domain=example.com&format=header'
# {"ok":true,"header":"sessionid=abc123; csrftoken=xyz","cookies":[...]}

# raw 格式 — 直接返回纯文本，方便脚本使用
curl 'http://127.0.0.1:8008/cookies?domain=example.com&format=raw'
# sessionid=abc123; csrftoken=xyz
```

### GET /domains

列出所有域名。

```json
{ "ok": true, "domains": ["example.com", "github.com"] }
```

### GET /health

健康检查。

```json
{ "service": "cookie-bridge", "extension": true }
```

## Emacs 集成

### 配置

```elisp
(add-to-list 'load-path "/path/to/cookie/elisp")
(require 'cookie)

;; 可选配置
(setq cookie-default-browser "chrome")  ; "chrome", "firefox", "edge"
(setq cookie-bridge-url "http://127.0.0.1:8008") ; Bridge 服务地址
(setq cookie-cache-expire 300)          ; 缓存过期秒数，0 禁用缓存
(setq cookie-prefer-bridge t)           ; 优先走 Bridge HTTP API
```

### Restclient 使用

restclient.el 使用 `:=` 操作符求值 elisp 表达式。cookie.el 提供的函数可直接用于变量定义：

```restclient
# 获取单个 Cookie 值
:token := (cookie-get "api.example.com" "auth_token")

GET https://api.example.com/user
Authorization: Bearer :token

###

# 获取所有 Cookie 并以 header 格式注入
:cookies := (cookie-header "api.example.com")

GET https://api.example.com/data
Cookie: :cookies

###

# 仅通过 HTTP API 获取（不回退 CLI，速度更快）
:session := (cookie-http-get "api.example.com" "sessionid")

POST https://api.example.com/submit
Cookie: session=:session
```

### 可用函数

| 函数 | 说明 |
|------|------|
| `(cookie-get DOMAIN NAME)` | 获取指定 Cookie 值（优先 Bridge，回退 CLI） |
| `(cookie-http-get DOMAIN NAME)` | 仅通过 Bridge HTTP API 获取 |
| `(cookie-header DOMAIN)` | 获取所有 Cookie，返回 `name1=val1; name2=val2` 格式 |
| `(cookie-get-value DOMAIN NAME)` | `cookie-get` 的别名 |

### 交互命令

| 命令 | 说明 |
|------|------|
| `M-x cookie-get-interactive` | 交互式获取 Cookie 值并复制到剪贴板 |
| `M-x cookie-list-domains` | 列出 Bridge 服务已知的所有域名 |
| `M-x cookie-clear-cache` | 清除 Cookie 缓存 |

## Cookie 获取策略

| 浏览器 | Bridge 服务可用 | Bridge 不可用 |
|--------|----------------|---------------|
| Chrome | 通过扩展获取明文 Cookie | 读取数据库（需关闭浏览器，受加密限制） |
| Edge | 通过扩展获取明文 Cookie | 读取数据库（需关闭浏览器，受加密限制） |
| Firefox | — | 直接读取数据库（明文存储，无需解密） |

## WSL2 说明

工具自动检测 WSL2 环境并访问 Windows 浏览器数据。

**浏览器数据路径：**
- Chrome: `/mnt/c/Users/<用户名>/AppData/Local/Google/Chrome/User Data/`
- Firefox: `/mnt/c/Users/<用户名>/AppData/Roaming/Mozilla/Firefox/Profiles/`
- Edge: `/mnt/c/Users/<用户名>/AppData/Local/Microsoft/Edge/User Data/`

## Makefile 目标

```bash
make build       # 编译
make serve       # 编译并启动 Bridge 服务
make ext-copy    # 复制扩展到 Windows 用户目录
make install     # 安装到 GOPATH/bin
make fmt         # 格式化代码
make vet         # 静态检查
make test        # 运行测试
make clean       # 清理构建产物
make help        # 显示帮助
```

## 故障排除

### Bridge 服务显示 `extension: false`

- 确认 Chrome 已启动且扩展已加载
- 在 Chrome 扩展页面检查 Service Worker 是否有错误
- 确认端口一致（默认 8008）

### Firefox 找不到 Profile

工具自动查找 `.default-release`（Firefox 67+）或 `.default` profile。多个 profile 时默认选择第一个匹配的。

### WSL2 下 "permission denied"

浏览器运行时会锁定 Cookie 文件。Chrome/Edge 建议使用 Bridge 扩展模式；Firefox 可尝试以只读模式打开。

## 许可证

MIT License
