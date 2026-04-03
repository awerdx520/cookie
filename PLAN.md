# Cookie 提取与 Restclient 集成项目计划书

## 项目概述

开发一个工具，用于从浏览器（Chrome、Firefox、Edge）中提取指定域名的 Cookie 信息，并与 Emacs 的 restclient.el 插件集成，实现本地开发时自动携带云端服务的认证 Token。

## 核心需求

1. **Cookie 提取功能**：
   - 支持从 Chrome、Firefox、Edge 浏览器（Windows、Linux）读取 Cookie
   - 特别支持 WSL2 环境下读取 Windows 系统中浏览器的 Cookie
   - 能够按域名过滤和提取特定的 Cookie（如认证 Token）
   - Firefox Cookie 为明文存储，无需解密，开箱即用
   - Edge 基于 Chromium，复用 Chrome 的解密逻辑

2. **Restclient 集成**：
   - 在 restclient.el 发起的 HTTP 请求中自动注入 Cookie 值
   - 支持动态获取最新 Cookie，避免 Token 过期
   - 提供简洁的配置方式

3. **开发体验**：
   - 本地开发时无需手动复制粘贴 Token
   - 支持多环境切换（开发、测试、生产）
   - 良好的错误处理和日志输出

## 技术方案

### 方案选择：Go + Emacs Lisp 混合方案

采用 Go 语言实现核心的 Cookie 提取逻辑，通过命令行工具提供接口；使用 Emacs Lisp 编写 restclient.el 的集成插件。

**优势**：
- Go 语言编译为单文件二进制，无需运行时依赖
- Go 的 SQLite 驱动成熟稳定，适合操作 Chrome 的 Cookie 数据库
- Emacs Lisp 直接集成到 restclient 工作流，用户体验无缝

### 架构设计

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
       ├──▶ CLI 输出 (cookie-cli get/list)
       ├──▶ HTTP JSON API (curl/脚本)
       └──▶ Emacs restclient.el 集成
```

Chrome/Edge 推荐使用扩展模式（通过 WebSocket 桥接，直接获取明文 Cookie，不受加密版本影响）；
Firefox Cookie 为明文存储，直接读取数据库即可。

### 模块划分

#### 1. cookie-cli (Go 程序)

**功能**：
- `cookie-cli get -domain <domain>`：获取指定域名的所有 Cookie
- `cookie-cli get -domain <domain> -name <name>`：获取特定名称的 Cookie 值
- `cookie-cli list`：列出所有可用的域名
- `cookie-cli serve`：启动 Cookie Bridge HTTP + WebSocket 服务

**实现要点**：
- **Chrome 扩展桥接**（推荐）：`serve` 启动本地 HTTP + WebSocket 服务，Chrome 扩展通过 WebSocket 连接，`get`/`list` 命令自动检测 Bridge 服务并优先使用，直接获取明文 Cookie，不需要解密，不需要关闭浏览器
- **数据库直读**（回退）：Bridge 不可用时回退到读取 SQLite 数据库，受浏览器文件锁和加密限制
- **平台检测**：自动识别操作系统和浏览器数据目录
- **WSL2 支持**：检测 WSL2 环境，使用 `/mnt/c/` 路径访问 Windows 浏览器数据

#### 1.5 Cookie Bridge 扩展 (Chrome MV3)

**功能**：
- 通过 `chrome.cookies.getAll()` API 获取明文 Cookie
- 通过 Offscreen Document 维持 WebSocket 长连接到本地 Bridge 服务
- 自动重连机制

**实现要点**：
- MV3 Service Worker 不支持 WebSocket，通过 `chrome.offscreen` API 创建 Offscreen Document 持有连接
- Offscreen Document 收到请求后通过 `chrome.runtime.sendMessage` 转发给 Service Worker 处理 `chrome.cookies` API 调用

#### 2. restclient-cookie.el (Emacs Lisp 包)

**功能**：
- 提供函数 `restclient-cookie-get` 调用 `cookie-cli` 获取 Cookie 值
- 与 restclient.el 原生 `:=` elisp 求值机制集成
- 提供 `restclient-cookie-header` 返回 HTTP Cookie 头格式字符串

**实现要点**：
- 使用 `(shell-command-to-string "cookie-cli get ...")` 获取值
- 支持 Bridge HTTP API 和 CLI 双后端，可配置优先级
- 缓存机制避免频繁调用命令行工具

#### 3. 配置系统

- 环境变量 `COOKIE_BROWSER` 指定浏览器类型（chrome, edge, firefox 等）
- 环境变量 `COOKIE_PROFILE` 指定 Chrome 用户配置目录
- 配置文件 `~/.cookie/config.yaml` 支持更复杂的多环境配置

## 开发计划

### 第一阶段：核心功能 ✅
1. 实现 `cookie-cli` 基础框架和命令行解析
2. 实现 Chrome/Firefox/Edge Cookie 文件定位（各平台）
3. 实现 SQLite 查询基础功能

### 第二阶段：平台适配与解密 ✅
1. 实现 Windows DPAPI 解密（v10/v11）
2. 完善 WSL2 支持（Windows copy 命令绕过文件锁）
3. v20 解密支持（flag 0x01/0x02，flag 0x03 受 CNG/KSP 限制）

### 第三阶段：Emacs 集成 ✅
1. 编写 `restclient-cookie.el` 基础函数
2. 实现 restclient.el 集成语法

### 第四阶段：Chrome 扩展桥接 ✅（2026-04-03）
1. Chrome MV3 扩展 + Offscreen WebSocket 客户端
2. Go HTTP + WebSocket Bridge 服务 (`internal/bridge`)
3. CLI 自动检测 Bridge 服务，优先使用扩展获取明文 Cookie
4. 彻底绕过 Chrome v20 加密和浏览器文件锁问题

### 第五阶段：完善 Restclient Cookie 支持（2026-04-03）
1. 重写为 `restclient-cookie.el`：移除有问题的 `{{cookie:...}}` 文本替换方式和不存在的 `restclient-request-hook`
2. 改用 restclient.el 原生 `:=` elisp 求值机制：`restclient-cookie-get`、`restclient-cookie-http-get`、`restclient-cookie-header`
3. 新增 `restclient-cookie-header` 函数，返回 `name1=val1; name2=val2` 格式
4. Bridge HTTP API 新增 `format` 参数（`header`/`raw`）
5. CLI `get` 命令新增 `-format` 参数（`header`/`json`）
6. 单个 Cookie 输出改为纯值无换行（适配 elisp `shell-command-to-string`）
7. 新增 `restclient-cookie-list-domains` 交互命令

### 第六阶段：Native Messaging + 文件导出（2026-04-03）
1. 新增 `internal/native` 包：NM 协议编解码、Host、Client、文件导出
2. Native Messaging Host 通过 unix domain socket 桥接 CLI 与 Chrome 扩展
3. Chrome 扩展同时支持 WebSocket（Offscreen）和 Native Messaging 两种通道
4. CLI 四级回退链：Bridge HTTP → Native Messaging socket → 导出文件 → SQLite
5. 新增 `cookie-cli native-messaging-host` / `export` 子命令
6. `make native-install` / `native-uninstall` 支持 WSL2 和原生 Linux
7. 文件导出格式：`~/.cookie/export.json`，带 timestamp，支持过期检查

### 后续（可选）
1. Cookie 监控和自动刷新
2. 图形化配置工具

## 集成示例

### Restclient 使用示例

```restclient
# 定义变量，从 Chrome 获取 example.com 的 token Cookie
:token := (restclient-cookie-get "example.com" "token")

# 使用变量
GET https://api.example.com/user
Authorization: Bearer :token

###

:session := (restclient-cookie-get "example.com" "sessionid")

POST https://api.example.com/data
Content-Type: application/json
Cookie: session=:session

{
  "data": "test"
}
```

### 命令行使用示例

```bash
# 获取 example.com 的所有 Cookie
$ cookie-cli get example.com

# 获取特定 Cookie 值
$ cookie-cli get example.com token
eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...

# 启动 HTTP 服务（默认端口 8080）
$ cookie-cli serve
```

## 技术栈

- **Go 1.26+**：核心逻辑实现
- **SQLite3**：浏览器 Cookie 数据库操作
- **Emacs Lisp**：集成层实现
- **Restclient.el**：HTTP 客户端

## 风险与挑战

1. **Cookie 加密**：~~Chrome/Edge 不同平台加密方式不同~~ → 已通过 Chrome 扩展桥接方案彻底解决
2. **浏览器文件锁**：~~浏览器运行时 Cookie 文件被锁定~~ → 扩展模式无需读取文件；回退模式下 WSL2 使用 Windows copy 命令
3. **WSL2 文件权限**：访问 Windows 文件系统可能需要特殊权限处理
4. **MV3 Service Worker 限制**：Service Worker 不支持 WebSocket，通过 Offscreen Document 解决

## 扩展性考虑

1. **插件架构**：已支持 Chrome、Firefox 和 Edge（Edge 复用 Chromium 架构）
2. **输出格式**：支持 JSON、YAML、环境变量等多种输出格式
3. **缓存机制**：避免频繁读取数据库，提高性能
4. **监控模式**：监听 Cookie 变化，自动更新

## 后续优化

1. 性能优化：使用内存缓存，减少数据库读取次数
2. 安全性：避免敏感信息泄露，支持模糊化日志
3. 用户体验：提供安装脚本和配置向导
4. 测试覆盖：增加单元测试和集成测试

---

## 立即行动

1. 创建项目结构
2. 实现基础 Cookie 读取功能
3. 编写 Emacs Lisp 集成函数
4. 提供完整的使用示例

通过本方案，开发者可以在本地开发环境中无缝使用云端服务的认证 Token，大幅提升开发效率。
