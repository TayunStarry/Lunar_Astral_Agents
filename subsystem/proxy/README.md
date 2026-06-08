# 子系统——HTTPS 代理（proxy）

本地 HTTPS 代理服务器模块，提供 TLS 终止代理、自签名证书自动生成与持久化、请求转发与拦截功能，使平台的 HTTP 服务具备 HTTPS 访问能力。

---

## 目录

- [功能概述](#功能概述)
- [项目结构](#项目结构)
- [核心架构](#核心架构)
- [核心模块说明](#核心模块说明)
- [API 接口定义](#api-接口定义)
- [编译与运行](#编译与运行)
- [常见问题](#常见问题)

---

## 功能概述

`proxy` 模块为星月智能平台提供 HTTPS 代理能力，解决本地 HTTP 服务无法直接通过 HTTPS 访问的问题。

| 功能 | 说明 |
|------|------|
| **TLS 终止代理** | 接收外部 HTTPS 请求，解密后转发给内部 HTTP 服务器 |
| **自签名证书** | 自动生成 RSA 2048 位自签名 TLS 证书，含 SAN 扩展 |
| **证书持久化** | 证书保存到磁盘，重启时复用，过期前 7 天自动刷新 |
| **WebSocket 代理** | 支持 WSS → WS 的 WebSocket 连接代理 |
| **嵌入式前端** | Go embed 内嵌代理管理 UI，提供可视化操作界面 |
| **CORS 支持** | 自动处理跨域请求预检，支持平台内部来源 |

### 两种运行模式

| 模式 | 入口函数 | 端口 | 说明 |
|------|---------|------|------|
| **独立代理服务** | `Run()` | 36369 | 完整代理服务 + 前端管理界面 |
| **TLS 终止代理** | `BuildTLSTerminationProxy()` | `config.ProxyPort` | 纯 TLS 终止转发，无前端界面 |

---

## 项目结构

<div style="font-family: 'Cascadia Code', 'SF Mono', Consolas, monospace; font-size: 0.9em; line-height: 1.6;">
  <ul style="list-style-type: none; padding-left: 0;">
    <li><strong>proxy/</strong></li>
    <li style="padding-left: 1.5em;"><code>proxy.go</code> <span style="color: #6a737d;">— 核心代理逻辑（HTTP/HTTPS 转发、请求拦截、WebSocket 代理、前端资源服务）</span></li>
    <li style="padding-left: 1.5em;"><code>certs.go</code> <span style="color: #6a737d;">— TLS 证书管理（自签名证书生成、磁盘加载、过期刷新、持久化存储）</span></li>
    <li style="padding-left: 1.5em;"><code>go.mod</code> / <code>go.sum</code> <span style="color: #6a737d;">— Go 模块定义</span></li>
    <li style="padding-left: 1.5em;"><code>build.ps1</code> <span style="color: #6a737d;">— 构建脚本</span></li>
    <li style="padding-left: 1.5em;">
      <strong>cmd/</strong>
      <ul style="list-style-type: none; padding-left: 1.5em;">
        <li><code>main.go</code> <span style="color: #6a737d;">— CLI 入口（启动代理服务、信号监听、优雅关闭）</span></li>
      </ul>
    </li>
    <li style="padding-left: 1.5em;">
      <strong>frontend/</strong> <span style="color: #6a737d;">— 前端资源（Go embed 嵌入）</span>
      <ul style="list-style-type: none; padding-left: 1.5em;">
        <li>
          <strong>proxy_ui/</strong>
          <ul style="list-style-type: none; padding-left: 1.5em;">
            <li><code>index.html</code> <span style="color: #6a737d;">— 代理管理界面</span></li>
            <li><code>script.js</code> <span style="color: #6a737d;">— 前端交互逻辑</span></li>
            <li><code>styles.css</code> <span style="color: #6a737d;">— 样式表</span></li>
          </ul>
        </li>
      </ul>
    </li>
  </ul>
</div>

### 依赖关系

<div style="font-family: 'Cascadia Code', 'SF Mono', Consolas, monospace; font-size: 0.9em; line-height: 1.6;">
  <ul style="list-style-type: none; padding-left: 0;">
    <li><code>proxy</code></li>
    <li style="padding-left: 1.5em;"><code>config</code> <span style="color: #6a737d;">(../config) — 配置管理（端口、证书路径、开发模式等）</span></li>
    <li style="padding-left: 1.5em;"><code>logger</code> <span style="color: #6a737d;">(../logger) — 彩色终端日志</span></li>
    <li style="padding-left: 1.5em;"><code>browser</code> <span style="color: #6a737d;">(../browser) — 嵌入式 WebView 窗口 + 本地 IP 发现</span></li>
  </ul>
</div>

---

## 核心架构

### 请求处理流程

```
客户端 HTTPS 请求
    │
    ▼
handleRequest() 统一请求处理器
    │
    ├── /proxy_ui/*  ──────────→ serveFrontend()     ← 嵌入式前端资源
    ├── /health      ──────────→ handleHealthCheck()  ← 健康检查
    ├── /api/server-info ──────→ handleServerInfo()   ← 服务器信息
    ├── Upgrade: websocket ───→ handleWebSocketProxy() ← WSS→WS 代理
    └── 其余路径    ──────────→ handleReverseProxy()  ← HTTPS→HTTP 反向代理
                                    │
                                    ▼
                              http://localhost:{BasicPort}
                              （内部 HTTP 服务）
```

### TLS 证书生命周期

```
程序启动
    │
    ▼
generateSelfSignedCert()
    │
    ├── 尝试 loadCertFromDisk()
    │   │
    │   ├── 读取 cert.pem + key.pem
    │   ├── 解析证书检查有效期
    │   │   ├── 未过期（预留7天）→ 直接使用 ✓
    │   │   └── 即将过期/已过期 → 重新生成
    │   └── 文件不存在 → 重新生成
    │
    └── generateAndSaveCert()
        ├── 生成 RSA 2048 位私钥
        ├── 收集 IP 地址（127.0.0.1 + ::1 + 局域网 IP）
        ├── 创建 X.509 证书（有效期 365 天，含 SAN）
        ├── saveCertToDisk() → 持久化到 {LocalDir}/certs/
        └── 加载 TLS 证书 ✓
```

### WebSocket 代理机制

```
客户端 WSS 连接
    │
    ▼
handleWebSocketProxy()
    │
    ├── Hijack 劫持客户端连接
    ├── 建立到后端的 TCP 连接 (localhost:BasicPort)
    ├── 将原始 HTTP 升级请求转发给后端
    │
    └── 双向数据转发（2 个 goroutine）
        ├── goroutine 1: Client → Backend
        └── goroutine 2: Backend → Client
```

---

## 核心模块说明

### proxy.go — 核心代理逻辑

| 函数 | 说明 |
|------|------|
| `Run()` | 启动代理服务完整流程：获取 IP → 启动 HTTPS 服务 → 打开浏览器 |
| `StartProxyServer()` | 创建并启动 HTTPS 代理服务器（端口 36369） |
| `BuildTLSTerminationProxy()` | 构建 TLS 终止代理服务器（端口 `config.ProxyPort`） |
| `handleRequest()` | 统一请求路由：前端资源 / 健康检查 / API / WebSocket / 反向代理 |
| `handleReverseProxy()` | HTTPS→HTTP 反向代理，设置 `X-Forwarded-Proto` / `X-Forwarded-Port` 头 |
| `handleWebSocketProxy()` | WebSocket 代理，Hijack + 双向 TCP 转发 |
| `handleHealthCheck()` | 健康检查，返回状态、待处理请求数、时间戳 |
| `handleServerInfo()` | 服务器信息 API，返回 IP、端口、URL |
| `serveFrontend()` | 嵌入式前端资源服务（Go embed） |
| `ReadCertFile()` / `ReadKeyFile()` | 读取当前证书/私钥 PEM 数据 |

### certs.go — TLS 证书管理

| 函数 | 说明 |
|------|------|
| `generateSelfSignedCert()` | 生成或加载自签名 TLS 证书（优先磁盘加载） |
| `loadCertFromDisk()` | 从磁盘加载证书并验证有效期（预留 7 天刷新） |
| `generateAndSaveCert()` | 生成新证书：RSA 2048 + SAN（localhost + 127.0.0.1 + ::1 + 局域网 IP） |
| `saveCertToDisk()` | 将证书和私钥写入磁盘（私钥权限 0600） |

---

## API 接口定义

### 端点列表

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/proxy_ui` | 代理管理前端界面 |
| GET | `/proxy_ui/*` | 前端静态资源（JS/CSS） |
| GET | `/health` | 健康检查 |
| GET | `/api/server-info` | 服务器信息 |
| ANY | `/*` | 反向代理转发至内部 HTTP 服务 |

### 健康检查响应

```json
// GET /health
{
  "status": "healthy",
  "timestamp": "2026-06-08T12:00:00Z",
  "pending_requests": 0,
  "port": 36369
}
```

### 服务器信息响应

```json
// GET /api/server-info
{
  "ip": "192.168.1.100",
  "port": 36369,
  "url": "https://192.168.1.100:36369",
  "basic_port": 36789
}
```

### 代理错误响应

```json
{
  "error": "无法连接到后端服务器",
  "message": "具体错误信息"
}
```

---

## 编译与运行

### 编译

```powershell
cd d:\Lunar_Astral_Agents\subsystem\proxy

# 一键构建（推荐）
.\build.ps1

# 交叉编译 Linux 版本
.\build.ps1 -TargetOS linux -TargetArch amd64
```

构建脚本会自动：
1. 设置 `CGO_ENABLED=1`（webview 依赖）
2. 编译带 `-tags webview` 的可执行文件
3. 使用 `-s -w -H windowsgui` 剥离符号并隐藏控制台窗口

编译产物：`d:\Lunar_Astral_Agents\proxy_server.exe`

### 手动构建

```powershell
cd d:\Lunar_Astral_Agents\subsystem\proxy

$env:CGO_ENABLED = "1"
$env:CC = "gcc"

go build -tags webview -ldflags="-s -w -H windowsgui" -trimpath -o ../../proxy_server.exe ./cmd/
```

### 运行

```powershell
# 直接运行（使用默认配置）
.\proxy_server.exe

# 默认信息：
#   HTTPS 代理端口: 36369
#   后端 HTTP 端口: 36789 (config.BasicPort)
#   前端界面: https://localhost:36369/proxy_ui
```

### 环境要求

| 组件 | 版本/说明 |
|------|----------|
| **Go** | ≥ 1.24 |
| **CGO** | 需启用（webview 依赖） |
| **GCC** | Windows 下推荐 MinGW-w64 |
| **WebView2** | Windows 10/11 通常已预装 |

---

## 常见问题

### Q: 浏览器提示证书不安全怎么办？

这是自签名证书的正常行为。在浏览器中点击"高级"→"继续访问"即可。如需消除警告，可将证书导入系统受信任根证书存储。

### Q: 证书文件存储在哪里？

证书和私钥存储在 `config.CertFile` 和 `config.KeyFile` 指定的路径，默认为 `{LocalDir}/certs/` 目录下。程序首次运行时自动生成，后续启动自动复用。

### Q: 证书过期后如何处理？

程序启动时会自动检查证书有效期。若证书将在 7 天内过期，会自动重新生成并保存到磁盘，无需手动干预。

### Q: WebSocket 代理不工作怎么办？

1. 确认后端 HTTP 服务已正常运行
2. 检查 `config.BasicPort` 端口是否正确
3. 确认后端支持 WebSocket 升级
4. 查看终端日志中的具体错误信息

### Q: 如何修改代理端口？

独立代理服务端口 `ProxyServerPort`（36369）为代码常量。TLS 终止代理端口通过 [config 子系统](../config/README.md) 的 `ProxyPort`（默认 36794）配置。

---

## 相关文档

- [项目主文档](../../README.md) —— 环境要求与整体架构
- [配置管理子系统](../config/README.md) —— 端口与证书路径配置
- [网页前端子系统](../browser/README.md) —— 嵌入式浏览器模块
- [日志子系统](../logger/README.md) —— 彩色终端日志
- [星图·月华](../../lunar_astral/README.md) —— proxy 的主要使用方
