# 扩展系统——星图·琉璃（crystal_astral）

工具集扩展程序，提供文件管理、数据库管理、截图标注、AI 代理转发与应用加载等综合功能。

---

## 人格智能体：琉璃

**琉璃**寓意为「如水晶般澄澈」——她代表着透明、轻盈与纯粹的本真。

琉璃的性格如同水晶一般清透明澈、轻盈灵动。她不做无谓的修饰，而是以最简洁直接的方式完成每一项任务。在技术上，琉璃追求操作的纯粹性与工具的直观性——每一个功能都经过精心打磨，如同水晶的每一面都折射出清晰的光芒。

如果说月华是平台温柔的「面容」，那么琉璃便是平台坚实的「双手」——她以水晶般的澄澈透明，承载起文件管理、数据库操作、截图标注、AI 代理转发等每一项实用工具。

---

## 功能概述

| 功能 | 说明 |
|------|------|
| 文件管理 | 本地文件浏览、上传、下载、删除、文本编辑 |
| 数据库管理 | SQLite 数据库可视化操作（CRUD/建表/查询） |
| 截图标注 | 多显示器截图 + 区域截图 + 图片缩放标注 |
| AI 代理转发 | OpenAI 格式 API 代理，支持外部模型接入 |
| 应用加载器 | 从界面启动外部 .exe / .ps1 / .bat 程序 |
| 随机背景图 | 本地图片库随机选取桌面背景 |

---

## 项目结构

| 文件 | 职责 |
|------|------|
| `main.go` | 程序入口，随机端口（10000~40000）+ 启动服务器 |
| `create.go` | 服务器创建、代理感知路由、应用启动 |
| `handler.go` | 代理转发处理器（模型列表/对话/completions） |
| `type.go` | 请求/响应类型定义 |
| `variable.go` | 全局变量与常量 |
| `ws.go` | WebSocket 通信 |
| `assets/` | 前端静态资源（index.html + script.js + style.css） |

**Go 模块依赖**：`config`、`browser`、`storage`、`image`、`logger`

---

## 核心架构

### 启动时序

```
main.go
  ├── flag.Parse()
  ├── rand.Intn(30001)+10000     ← 随机端口（10000~40000）
  └── StartServer(port, fs, name)
      ├── 注册 SystemEndpoints 路由
      ├── 创建代理感知 Handler (/v1/ → 月华 llama-proxy)
      ├── reloadPageParameters() ← 窗口尺寸 1500×1050
      ├── browser.OpenBrowser()  ← 自动打开 WebView
      ├── http.ListenAndServe()  ← 异步启动 HTTP
      └── 等待信号优雅退出
```

### 代理感知路由

琉璃通过智能路由根据请求路径自动分发：

```
请求 → proxyAwareHandler
  ├── /v1/ 开头      → 代理到月华后端
  ├── /generate 开头  → 同上
  ├── /write/message  → 同上
  └── 其他路径        → 直接服务静态文件
```

---

## API 接口

### 文件管理

| 方法 | 端点 | 说明 |
|------|------|------|
| POST | `/file/write` | 保存文件（Header: `X-File-Name`） |
| GET | `/file/read/{path}` | 读取文件内容 |
| DELETE | `/file/delete/{path}` | 删除文件或目录 |
| GET | `/file/download/{path}` | 下载文件 |
| POST | `/file/list/{path}` | 列出目录内容 |
| POST | `/file/archive` | 创建或解压 ZIP 归档 |
| GET | `/background` | 获取随机背景图片 |

### 数据库管理

| 方法 | 端点 | 说明 |
|------|------|------|
| POST | `/database/` | 批量数据库操作（insert/update/delete/select/create/drop） |

### 截图

| 方法 | 端点 | 说明 |
|------|------|------|
| POST | `/capture` | 通用截图（区域/显示器/全屏） |
| GET | `/capture/display/{index}` | 指定显示器截图 |
| POST | `/capture/region` | 区域截图 |
| GET | `/capture/displays` | 获取显示器列表 |
| POST | `/resize` | 图片缩放到 1080p |

### 应用管理

| 方法 | 端点 | 说明 |
|------|------|------|
| POST | `/load/application` | 启动外部应用程序 |

支持的启动方式：`.exe` 直接执行、`.ps1` PowerShell 执行、`.bat` CMD 窗口执行。

### AI 代理

| 方法 | 端点 | 说明 |
|------|------|------|
| POST | `/proxy/chat` | 代理 OpenAI 对话请求 |
| POST | `/proxy/models` | 代理查询模型列表 |

---

## 编译与运行

### 编译

```powershell
cd d:\Lunar_Astral_Agents\crystal_astral
.\build.ps1
```

编译产物：`d:\Lunar_Astral_Agents\Crystal_Astral.exe`

### 运行

```powershell
.\Crystal_Astral.exe
```

琉璃在 10000~40000 之间随机选择端口，窗口自动以 WebView 形式打开（1500×1050）。

---

## 常见问题

**琉璃和月华有什么区别？** 月华是 AI 智能体核心系统（角色对话、TTS），琉璃是工具集扩展系统（文件管理、数据库、截图、AI 代理）。琉璃启动时自动连接到月华后端进行 AI 请求代理。

**端口为什么是随机的？** 琉璃设计为可多实例并行运行，随机端口避免端口冲突。

**如何调用外部模型 API？** 使用 `/proxy/chat` 端点，传入 `base_url`、`api_key`、`model` 和 `messages` 即可代理转发到任意兼容 OpenAI 格式的 API 服务。

---

## 相关文档

- [项目主文档](../README.md) — 环境要求、整体架构
- [项目架构说明](../ARCHITECTURE.md) — 完整架构
- [星图·月华](../lunar_astral/README.md) — AI 智能体核心系统
- [配置管理](../subsystem/config/README.md) — 全局配置
- [文件管理](../subsystem/storage/README.md) — 文件与数据库详情
- [图像处理](../subsystem/image/README.md) — 截图与图像生成详情