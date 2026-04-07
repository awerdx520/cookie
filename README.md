# Cookie — 浏览器 Cookie 提取工具

从浏览器提取 Cookie，方便本地开发时自动携带云端服务的认证 Token。

## 功能特性

- **Chrome 扩展桥接**：通过 Cookie Bridge 扩展直接获取明文 Cookie，无需解密，无需关闭浏览器
- **多种通信模式**：Serve（HTTP+WebSocket）、Native Messaging（无需常驻进程）、文件导出
- **多浏览器支持**：Chrome、Firefox、Edge
- **跨平台**：Windows、Linux、WSL2
- **HTTP API**：提供本地 REST API，方便任意工具调用

## 架构

```
                              Chrome 浏览器
                              ┌──────────────────┐
curl / 脚本 / 编辑器          │  Cookie Bridge    │
       │                      │  扩展 (MV3)       │
       ▼                      │  chrome.cookies   │
┌──────────────┐              │  API              │
│ cookie-cli   │ ←─ WebSocket ┤                   │
│              │ ←─ NativeMsg ─┤                   │
│              │              └──────────────────┘
└──────┬───────┘
       │
       ├──▶ 模式 1: Serve（HTTP + WebSocket，需启动常驻服务）
       ├──▶ 模式 2: Native Messaging（扩展自动启动，无需 serve）
       ├──▶ 模式 3: 文件导出（读取 ~/.cookie/export.json）
       └──▶ 模式 4: SQLite 直读（回退，需关闭浏览器）
```

CLI 自动按优先级尝试所有模式，优先使用最便捷的方式获取 Cookie。

## 快速开始

### 1. 编译

```bash
make build
# 或
go build -o cookie-cli ./cmd/cookie-cli
```

### 2. 安装 Chrome 扩展

```bash
# WSL2 用户: 复制扩展到 Windows 目录
make ext-copy
```

然后在 Chrome 中加载扩展：

1. 打开 `chrome://extensions/`
2. 开启右上角 **开发者模式**
3. 点击 **加载已解压的扩展程序**
4. 选择 `C:\Users\<用户名>\cookie-bridge-extension` 目录

### 3. 选择通信模式

**方式 A：Native Messaging（推荐，无需常驻进程）**

```bash
make native-install
# 按提示将扩展 ID 填入 manifest 文件
# 然后重新加载 Chrome 扩展
```

安装后，扩展会自动启动 native-messaging-host，`cookie-cli get` 直接可用。

**方式 B：Serve 模式（需启动常驻服务）**

```bash
cookie-cli serve
# 默认监听 127.0.0.1:8008
```

**方式 C：文件导出（离线使用）**

```bash
# 通过 Native Messaging 导出 Cookie 到本地文件
cookie-cli export -domain example.com
```

### 4. 获取 Cookie

```bash
# CLI 方式（自动按优先级选择通信模式）
cookie-cli get -domain example.com
cookie-cli get -domain example.com -name sessionid
cookie-cli get -domain example.com -format header

# HTTP API 方式（仅 serve 模式）
curl 'http://127.0.0.1:8008/cookies?domain=example.com'
curl 'http://127.0.0.1:8008/cookies?domain=example.com&name=sessionid'
curl 'http://127.0.0.1:8008/domains'
curl 'http://127.0.0.1:8008/health'
```

## 命令行参考

```bash
cookie-cli get -domain <域名> [-name <名称>] [-browser <浏览器>] [-format <格式>] [-cache-expire <秒>]
cookie-cli list [-browser <浏览器>] [-cache-expire <秒>]
cookie-cli serve [-port <端口>]
cookie-cli export [-domain <域名>]
cookie-cli native-messaging-host
```

| 子命令 | 说明 |
|--------|------|
| `get` | 获取指定域名的 Cookie |
| `list` | 列出所有包含 Cookie 的域名 |
| `serve` | 启动 Cookie Bridge HTTP + WebSocket 服务 |
| `export` | 通过 Native Messaging 导出 Cookie 到本地文件 |
| `native-messaging-host` | 作为 Chrome Native Messaging Host 运行（由扩展自动启动） |

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `-domain` | — | 目标域名 |
| `-name` | — | Cookie 名称（省略则返回该域名所有 Cookie） |
| `-browser` | `chrome` | 浏览器类型：`chrome`、`firefox`、`edge` |
| `-format` | — | 输出格式：`header`（Cookie 头格式）、`json`（JSON 数组） |
| `-cache-expire` | `-1`（未传参） | 仅 `get` / `list`：使用 `~/.cookie/export.json` 回退时的最大文件年龄（秒）。`-1` 表示沿用 `COOKIE_CACHE_EXPIRE` 环境变量或默认 `300`；`0` 表示不限制。**优先级：命令行高于环境变量** |
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
| `COOKIE_CACHE_EXPIRE` | 使用 `~/.cookie/export.json` 回退时的最大文件年龄（秒），默认 `300`；设为 `0` 表示不限制。若未传 `-cache-expire`，则使用该变量 |

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

项目提供 `elisp/restclient-cookie.el`，可与 [restclient.el](https://github.com/pashky/restclient.el) 集成，在 HTTP 请求中自动注入 Cookie。详细配置和用法参见该文件头部注释。

重放请求前若需最新 Cookie，可 `M-x restclient-cookie-refresh-cache`（或 `restclient-cookie-clear-cache`）清除 Emacs 缓存；`restclient-cookie-cache-expire` 大于 0 时，包内调用 `cookie-cli get` 会自动带上 `-cache-expire`，与 CLI 行为对齐。

## Cookie 获取策略

Chrome/Edge 的获取优先级（自动选择）：

| 优先级 | 方式 | 条件 | 说明 |
|--------|------|------|------|
| 1 | Native Messaging | 已执行 `make native-install` | 扩展自动启动 host 进程，无需 serve |
| 2 | Bridge HTTP | `cookie-cli serve` 运行中 | 通过 WebSocket 桥接获取明文 Cookie |
| 3 | 文件导出 | `~/.cookie/export.json` 存在且未过期 | 读取之前导出的 Cookie |
| 4 | SQLite 直读 | 回退 | 需关闭浏览器，受加密限制 |

Firefox 直接读取 SQLite 数据库（明文存储，无需解密）。

## WSL2 说明

工具自动检测 WSL2 环境并访问 Windows 浏览器数据。

**浏览器数据路径：**
- Chrome: `/mnt/c/Users/<用户名>/AppData/Local/Google/Chrome/User Data/`
- Firefox: `/mnt/c/Users/<用户名>/AppData/Roaming/Mozilla/Firefox/Profiles/`
- Edge: `/mnt/c/Users/<用户名>/AppData/Local/Microsoft/Edge/User Data/`

## Makefile 目标

```bash
make build             # 编译
make serve             # 编译并启动 Bridge 服务
make ext-copy          # 复制扩展到 Windows 用户目录
make native-install    # 注册 Native Messaging Host
make native-uninstall  # 移除 Native Messaging Host 注册
make install           # 安装到 GOPATH/bin
make fmt               # 格式化代码
make vet               # 静态检查
make test              # 运行测试
make clean             # 清理构建产物
make help              # 显示帮助
```

## 故障排除

### Bridge 服务显示 `extension: false`

- 确认 Chrome 已启动且扩展已加载
- 在 Chrome 扩展页面检查 Service Worker 是否有错误
- 确认端口一致（默认 8008）

### Native Messaging 不工作

- 确认已运行 `make native-install`
- 确认 manifest 中的 `allowed_origins` 包含正确的扩展 ID
- 在 Chrome 扩展页面查看 Service Worker 日志中是否有 "Native Messaging" 相关错误
- WSL2 用户确认 `wsl.exe` 可以正常调用 `cookie-cli`

### Firefox 找不到 Profile

工具自动查找 `.default-release`（Firefox 67+）或 `.default` profile。多个 profile 时默认选择第一个匹配的。

### WSL2 下 "permission denied"

浏览器运行时会锁定 Cookie 文件。Chrome/Edge 建议使用扩展模式（Serve 或 Native Messaging）；Firefox 可尝试以只读模式打开。

## 许可证

MIT License
