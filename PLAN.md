# Cookie 提取与 Restclient 集成项目计划书

## 项目概述

开发一个工具，用于从 Chrome 浏览器中提取指定域名的 Cookie 信息，并与 Emacs 的 restclient.el 插件集成，实现本地开发时自动携带云端服务的认证 Token。

## 核心需求

1. **Cookie 提取功能**：
   - 支持从 Chrome 浏览器（包括 Windows、Linux、macOS）读取 Cookie
   - 特别支持 WSL2 环境下读取 Windows 系统中 Chrome 的 Cookie
   - 能够按域名过滤和提取特定的 Cookie（如认证 Token）

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
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   Chrome        │    │   cookie-cli    │    │   Emacs         │
│   Cookie 数据库  │───▶│   (Go 工具)     │───▶│   restclient.el │
└─────────────────┘    └─────────────────┘    └─────────────────┘
         │                        │                        │
         │ 读取 SQLite            │ 命令行输出            │ 变量注入
         ▼                        ▼                        ▼
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│  平台适配层      │    │  格式化输出      │    │  HTTP 请求      │
│  - Windows路径   │    │  - 纯文本       │    │  携带 Cookie    │
│  - Linux路径     │    │  - JSON         │    │                 │
│  - macOS路径     │    │  - 环境变量      │    │                 │
│  - WSL2 特殊处理 │    └─────────────────┘    └─────────────────┘
└─────────────────┘
```

### 模块划分

#### 1. cookie-cli (Go 程序)

**功能**：
- `cookie-cli get <domain>`：获取指定域名的所有 Cookie
- `cookie-cli get <domain> <name>`：获取特定名称的 Cookie 值
- `cookie-cli list`：列出所有可用的域名
- `cookie-cli serve`：启动 HTTP 服务，提供 REST API

**实现要点**：
- **平台检测**：自动识别操作系统和 Chrome 数据目录
- **WSL2 支持**：检测 WSL2 环境，使用 `/mnt/c/` 路径访问 Windows Chrome 数据
- **SQLite 操作**：使用 `github.com/mattn/go-sqlite3` 读取 Chrome 的 `Cookies` 数据库文件
- **加密处理**：Chrome 的 Cookie 值可能加密（Windows 使用 DPAPI，macOS 使用 Keychain，Linux 使用 GNOME Keyring 或 KWallet）。初始版本可先支持未加密的 Cookie，或依赖系统工具解密。

#### 2. cookie-el (Emacs Lisp 包)

**功能**：
- 提供函数 `cookie-get` 调用 `cookie-cli` 获取 Cookie 值
- 定义 restclient 变量语法糖：`{{cookie:example.com}}` 或 `{{cookie:example.com token}}`
- 可选：提供 minor mode，自动为所有 restclient 请求注入 Cookie

**实现要点**：
- 使用 `(shell-command-to-string "cookie-cli get example.com token")` 获取值
- 处理可能的多行输出和错误情况
- 缓存机制避免频繁调用命令行工具

#### 3. 配置系统

- 环境变量 `COOKIE_BROWSER` 指定浏览器类型（chrome, edge, firefox 等）
- 环境变量 `COOKIE_PROFILE` 指定 Chrome 用户配置目录
- 配置文件 `~/.cookie/config.yaml` 支持更复杂的多环境配置

## 开发计划

### 第一阶段：核心功能（1-2 周）
1. 实现 `cookie-cli` 基础框架和命令行解析
2. 实现 Chrome Cookie 文件定位（各平台）
3. 实现 SQLite 查询基础功能
4. 测试基本 Cookie 读取功能

### 第二阶段：平台适配与解密（1-2 周）
1. 实现 Windows DPAPI 解密（使用 `go-dpapi` 或调用系统命令）
2. 实现 macOS Keychain 集成
3. 实现 Linux 密钥环集成
4. 完善 WSL2 支持

### 第三阶段：Emacs 集成（1 周）
1. 编写 `cookie.el` 基础函数
2. 实现 restclient.el 集成语法
3. 编写使用文档和示例

### 第四阶段：高级功能（可选）
1. HTTP 服务模式，支持多客户端同时使用
2. 多浏览器支持（Firefox, Edge, Safari）
3. Cookie 监控和自动刷新
4. 图形化配置工具

## 集成示例

### Restclient 使用示例

```restclient
# 定义变量，从 Chrome 获取 example.com 的 token Cookie
:token = {{(cookie-get "example.com" "token")}}

# 使用变量
GET https://api.example.com/user
Authorization: Bearer :token

POST https://api.example.com/data
Content-Type: application/json
Cookie: session={{cookie-get "example.com" "sessionid"}}

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
- **SQLite3**：Chrome Cookie 数据库操作
- **Emacs Lisp**：集成层实现
- **Restclient.el**：HTTP 客户端

## 风险与挑战

1. **Cookie 加密**：不同平台加密方式不同，需要处理系统级密钥存储
2. **Chrome 文件锁**：Chrome 运行时 Cookie 文件被锁定，可能需要复制或只读打开
3. **WSL2 文件权限**：访问 Windows 文件系统可能需要特殊权限处理
4. **多用户环境**：需要正确处理多 Chrome 用户配置

## 扩展性考虑

1. **插件架构**：支持其他浏览器（Firefox, Edge, Safari）
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
