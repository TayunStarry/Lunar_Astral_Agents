# 扩展系统——星图·琉璃（crystal_astral）

> 📚 代码级文档参见 [Code Wiki 03·扩展系统-琉璃](../docs/code-wiki/03-扩展系统-星图琉璃.md)，综合入口 [Code Wiki 门户](../docs/code-wiki/README.md)。

工具集扩展程序，提供文件管理、知识库/记忆库、截图标注、图片处理、AI 代理转发、模型元数据解析与引擎命令桥接等综合功能。

---

## 人格智能体：琉璃

**琉璃**寓意为「如水晶般澄澈」——她代表着透明、轻盈与纯粹的本真。

琉璃的性格如同水晶一般清透明澈、轻盈灵动。她不做无谓的修饰，而是以最简洁直接的方式完成每一项任务。在技术上，琉璃追求操作的纯粹性与工具的直观性——每一个功能都经过精心打磨，如同水晶的每一面都折射出清晰的光芒。

如果说月华是平台温柔的「面容」，那么琉璃便是平台坚实的「双手」——她以水晶般的澄澈透明，承载起文件管理、知识库操作、截图标注、AI 代理转发等每一项实用工具。

---

## 功能概述

| 功能 | 说明 |
|------|------|
| 文件管理 | 本地文件浏览、上传、下载、删除、编辑、预览、归档 |
| 扩展包管理 | 扩展包的安装、导出、删除与包目录扫描 |
| 知识库 | SQLite 知识库管理（CRUD/建表/查询） |
| 记忆库 | 向量记忆库（实例初始化/集合管理/消息增删查/文档列表/重建） |
| 文件整理 | 批量文件整理操作 |
| 截图与图像处理 | 多显示器截图 + 区域截图 + 图片缩放 + 关键帧提取 + 图片格式转换 |
| AI 代理转发 | OpenAI 格式 API 代理，支持外部模型接入 |
| GGUF 元数据解析 | 解析 GGUF 模型文件头信息与元数据 |
| 引擎命令桥接 | StudioHub WebSocket 集线器，桥接智能体引擎命令与动画动作 |
| 月华服务管理 | 检测月华服务状态、一键启动 |
| 应用加载器 | 从界面启动外部 .exe / .ps1 / .bat 程序 |
| 随机背景图 | 本地图片库随机选取桌面背景 |

---

## 项目结构

| 文件 | 职责 |
|------|------|
| `main.go` | 程序入口，随机端口（10000~40000）+ 启动服务器 |
| `create.go` | 服务器创建、代理感知路由、应用启动、StudioHub 初始化 |
| `handler.go` | 代理转发（模型列表/对话）、GGUF 元数据、图片转换、月华服务启停 |
| `gguf.go` | GGUF 模型文件解析 |
| `convert.go` | 图片格式转换（单张/批量/列表） |
| `ws.go` | StudioHub WebSocket 集线器 + 引擎命令桥接 |
| `type.go` | 请求/响应类型定义 |
| `variable.go` | 全局变量、常量与 SystemEndpoints 路由表 |
| `assets/` | 前端静态资源（index.html + script.js + style.css） |

**Go 模块依赖**：`general_config`、`browser_client`、`file_manager`、`image_processor`、`logger_general`（另含 `gorilla/websocket`、`chai2010/webp`）

---

## 核心架构

### 启动时序

启动调用链与各步骤对应实现，见 [Code Wiki 03 §2 启动与路由](../docs/code-wiki/03-扩展系统-星图琉璃.md)，此处不重复。

### 代理感知路由

琉璃通过智能路由根据请求路径自动分发，代理目标统一为月华后端（`http://localhost:36789`）：

```
请求 → proxyAwareHandler
  ├── /v1/ 开头          → 代理到月华后端
  ├── /tts 开头          → 代理到月华后端（由其转发至 TTS 服务）
  ├── /write/message 开头 → 代理到月华后端
  ├── /ltpx/ 开头        → 代理到月华后端
  └── 其他路径            → 直接服务静态文件
```

### StudioHub 引擎桥接

`/ws/studio` 是工作室 WebSocket 集线器端点：

- 前端组件（如动作控制面板）通过 WebSocket 实时收发消息
- 集线器拦截 `animation_list` 消息并缓存动作定义（`animCache`），供智能体查询
- `/api/engine/command` 接收智能体后端的引擎命令并转发到 StudioHub 广播
- `/api/engine/animations` 返回缓存的可用动作列表

---

## API 接口

端点由 `SystemEndpoints` 路由表统一注册，按功能域分组：

### 应用与资源

| 方法 | 端点 | 说明 |
|------|------|------|
| POST | `/load/application` | 加载外部应用程序（.exe / .ps1 / .bat） |
| GET | `/background` | 获取随机背景图片 |

### 文件操作

| 方法 | 端点 | 说明 |
|------|------|------|
| GET | `/file/read/{path}` | 读取文件内容 |
| POST | `/file/write` | 保存文件（Header: `X-File-Name`） |
| DELETE | `/file/delete/{path}` | 删除文件或目录 |
| POST | `/file/list/{path}` | 列出目录内容 |
| GET | `/file/download/{path}` | 下载文件 |
| GET | `/file/preview` | 全局文件预览（图片/视频/文本） |
| POST | `/file/archive` | 文件归档（ZIP） |

### 扩展包管理

| 方法 | 端点 | 说明 |
|------|------|------|
| POST | `/file/package/install` | 安装扩展包 |
| POST | `/file/package/export` | 导出扩展包 |
| POST | `/file/package/delete` | 删除扩展包 |
| GET | `/api/packages` | 扫描包目录 |

### 知识库与记忆库

| 方法 | 端点 | 说明 |
|------|------|------|
| POST | `/knowledge/` | 知识库管理（insert/update/delete/select/create/drop） |
| ANY | `/memory/` | 记忆库（实例初始化/集合管理/消息增删查/文档列表/重建） |

### 文件整理

| 方法 | 端点 | 说明 |
|------|------|------|
| POST | `/file/organize` | 批量文件整理操作 |

### 截图与图像处理

| 方法 | 端点 | 说明 |
|------|------|------|
| POST/GET | `/capture` | 统一截图（auto/window/fullscreen/display/region 五种模式） |
| GET | `/capture/displays` | 获取显示器列表 |
| POST | `/resize` | 图片缩放 |
| POST | `/keyframe` | 视频关键帧提取 |
| POST | `/convert/image` | 单张图片格式转换 |
| POST | `/convert/batch` | 批量图片格式转换 |
| POST | `/convert/list` | 列出文件夹中的图片文件 |

### AI 模型与推理

| 方法 | 端点 | 说明 |
|------|------|------|
| POST | `/proxy/models` | 代理查询模型列表 |
| POST | `/proxy/chat` | 代理 OpenAI 对话请求 |
| POST | `/gguf/metadata` | GGUF 模型元数据解析 |
| POST | `/generate` | 图像生成 |
| GET | `/generate/wait` | 图像生成任务等待 |

### 月华服务

| 方法 | 端点 | 说明 |
|------|------|------|
| GET | `/lunar/check` | 检测月华服务状态（端口 36789） |
| POST | `/lunar/start` | 启动月华服务（Lunar_Astral.exe） |

### 引擎命令桥接

| 方法 | 端点 | 说明 |
|------|------|------|
| POST | `/api/engine/command` | 智能体引擎命令转发 |
| GET | `/api/engine/animations` | 查询引擎可用动作列表 |

### WebSocket

| 端点 | 说明 |
|------|------|
| `/ws/studio` | StudioHub 工作室实时通信（广播引擎命令、缓存动画动作） |

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

**琉璃和月华有什么区别？** 月华是 AI 智能体核心系统（角色对话、TTS），琉璃是工具集扩展系统（文件管理、知识库/记忆库、截图、图片处理、AI 代理）。琉璃启动时自动连接到月华后端进行 AI 请求代理。

**端口为什么是随机的？** 琉璃设计为可多实例并行运行，随机端口避免端口冲突。

**如何调用外部模型 API？** 使用 `/proxy/chat` 端点，传入 `base_url`、`api_key`、`model` 和 `messages` 即可代理转发到任意兼容 OpenAI 格式的 API 服务。

---

## 相关文档

- [项目主文档](../README.md) — 环境要求、整体架构
- [项目架构说明](../ARCHITECTURE.md) — 完整架构
- [星图·月华](../lunar_astral/README.md) — AI 智能体核心系统
- [配置管理](../subsystem/general_config/README.md) — 全局配置
- [文件管理](../subsystem/file_manager/README.md) — 文件、知识库、记忆库与扩展包详情
- [图像处理](../subsystem/image_processor/README.md) — 截图、关键帧与图像生成详情
