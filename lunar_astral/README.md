# 核心系统——星图·月华（lunar_astral）

AI 桌面智能体核心系统，集成多模态对话、Live2D 角色展示、TTS 语音合成与图像生成功能。

---

![月华主页面](../image/月华-主页面.webp)

*图：星图·月华主界面（桌面端）*

---

## 目录

- [功能概述](#功能概述)
- [项目结构](#项目结构)
- [核心架构](#核心架构)
- [核心模块说明](#核心模块说明)
- [API 接口定义](#api-接口定义)
- [编译与运行](#编译与运行)
- [配置说明](#配置说明)
- [常见问题](#常见问题)

---

## 功能概述

星图·月华是星月智能平台的**核心系统**，提供以下主要功能：

| 功能 | 说明 |
|------|------|
| AI 智能对话 | 基于本地 GGUF 模型的角色扮演对话，支持多模态输入 |
| Live2D 角色 | 实时 Live2D 模型渲染，角色表情与动作展示 |
| TTS 语音合成 | 文本转语音，支持将 AI 回复实时合成音频 |
| 图像生成 | 基于 stable-diffusion.cpp 的文生图功能 |
| 文件管理 | 对话上下文中的文件上传、读取与管理 |
| 视频处理 | 视频关键帧提取与多媒体预览 |
| Mermaid/ECharts/KaTeX | 对话内容中的图表、流程图与数学公式渲染 |

### 界面展示

![月华手机端](../image/月华-主界面-手机端.webp)

*图：星图·月华移动端界面*

![月华聊天记录](../image/月华-聊天记录.webp)

*图：星图·月华聊天记录界面*

![星图-月华-人设图-1](../image/星图-月华-人设图-1.webp)

*图：月华角色人设*

---

## 项目结构

```
lunar_astral/
├── main.go                  ← 程序入口，启动 HTTP 服务器
├── go.mod                   ← Go 模块声明（依赖 config/browser/storage/screenshot/tts）
├── build.ps1                ← 编译脚本
├── icon.ico                 ← 应用程序图标
├── package.json             ← Node.js 前端构建
├── rollup.config.js         ← 前端打包配置
├── tsconfig.json            ← TypeScript 配置
├── removeExport.cjs         ← 构建后处理（移除 ES export）
│
├── adapters/                ← Go↔JS 适配器层
│   ├── create.go            ← JS 运行时（goja eventloop）创建与生命周期
│   ├── database.go          ← SQLite 数据库操作适配
│   ├── file.go              ← 文件系统操作适配
│   ├── message.go           ← 上下文消息推拉适配
│   ├── network.go           ← 网络请求与 IP 定位适配
│   ├── type.go              ← 共享类型定义
│   └── vision.go            ← 视觉/图像处理适配
│
├── server/                  ← HTTP 服务层
│   ├── create.go            ← 服务器启动器、CORS 中间件、端口重试
│   ├── manage.go            ← 初始化流程（flag、MIME、路由、llama、TTS、WebSocket、JS 智能体）
│   ├── type.go              ← 系统端点与服务类型
│   ├── variable.go          ← 全局路由表 SystemEndpoints
│   └── handlers/            ← HTTP 请求处理器
│       ├── generate.go      ← 图像生成处理（sd-cli 调用）
│       ├── message.go       ← 消息收发处理
│       ├── proxy.go         ← llama.cpp 代理转发
│       ├── type.go          ← 处理器类型
│       └── video.go         ← 视频关键帧提取
│
├── model/                   ← 模型服务层
│   ├── type.go              ← 模型/任务结构体
│   ├── core.go              ← 模型列表、端口映射、请求队列
│   ├── variable.go          ← 并发控制变量
│   ├── llama/         ← llama.cpp 代理
│   │   └── proxy.go         ← llama-server 启动/监控/代理/云服务回退
│   └── tts/                 ← TTS 语音合成引擎
│       ├── entry.go         ← TTS 合成入口
│       ├── cache.go         ← WAV 音频缓存
│       ├── capture.go       ← 音频分句捕获
│       ├── type.go          ← TTS 类型定义
│       ├── variable.go      ← TTS 运行时状态
│       ├── wrapper.go       ← WAV 封装
│       └── writer.go        ← 脉冲编码调制写入
│
├── hierarchy/               ← 前端资源层
│   ├── embedded.go          ← Go embed.FS 资源嵌入
│   ├── image/               ← 图像生成
│   │   ├── generate/        ← 生成逻辑与类型
│   │   └── video.go         ← 视频工具
│   └── assets/              ← 前端静态资源
│       ├── agentSystem.js   ← 智能体核心 JS（goja 运行时执行）
│       ├── prompts/         ← AI 角色提示词模板（9 个 .md）
│       └── client/          ← 前端 Web 界面（HTML/CSS/JS）
│
├── websocket/               ← WebSocket 通信层
│   ├── websocket.go         ← 连接管理、读写泵、广播
│   ├── type.go              ← WebSocket 类型定义
│   └── variable.go          ← WebSocket 全局变量
│
├── release/                 ← 进程/端口管理
│   ├── execute.go           ← 命令执行
│   ├── kill.go              ← 进程终止
│   ├── network_status.go    ← 网络状态监控
│   ├── processes.go         ← 进程列表
│   └── query.go             ← 查询功能
│
└── server_side/             ← TypeScript 智能体源码（编译→ agentSystem.js）
    ├── index.ts             ← 智能体入口
    ├── config/              ← 全局配置（模型/数据库/历史/图像/工具）
    ├── control/             ← 对话控制（延迟/限制/计划）
    ├── file/                ← 文件处理（数据库/编码/读取/分割）
    ├── math/                ← 数学工具（向量/基向量）
    └── model/               ← AI 模型（智能体/构建器/对话/画师/定义）
```

---

## 核心架构

### 启动时序

```
main.go
  │
  ├── config.init()              ← 解析命令行 + JSON 配置
  ├── server.InitializeServer()
  │   ├── flag.Parse()
  │   ├── 按 ext→MIME 注册映射
  │   ├── 创建本地数据目录
  │   ├── registerHandlers()     ← 注册所有 HTTP 路由
  │   ├── llama.Init()     ← 启动 llama-server + 等待就绪
  │   ├── module.InitTTSEngine() ← 初始化 TTS 引擎
  │   ├── websocket.Setup...()   ← 注册 /ws 端点
  │   └── adapters.RunAgentContext()
  │       ├── createAgentContext()  ← 创建 goja eventloop 运行时
  │       ├── 注册适配器函数（文件/数据库/网络/图像/消息）
  │       └── 执行 agentSystem.js  ← TypeScript 编译的智能体代码
  │
  ├── SetupSignalHandling()      ← 系统信号监听
  ├── StartServerListener()      ← 端口自动递增重试（最多 10 次）
  │   ├── CORSMiddleware
  │   └── browser.OpenBrowser()  ← 自动打开 WebView/浏览器
  │
  └── WaitForShutdown()          ← 优雅关闭
```

### 核心数据流

```
┌──────────┐     HTTP POST     ┌──────────┐     推入队列     ┌───────────┐
│ 前端 UI  │ ──── /write/ ───→ │ Go 服务层 │ ──────────────→ │ JS 智能体 │
│ (WebView)│     /message      │          │                 │ (goja)    │
└────┬─────┘                   └────┬─────┘                 └─────┬─────┘
     ▲                              │                             │
     │                              │ 拉取上下文    调用 LLM API   │
     │                    ┌─────────▼────────┐         ┌──────────▼──────┐
     │                    │   模型代理层       │ ←────── │  llama-server  │
     │                    │  llama      │         │  (GGUF 推理)    │
     │                    └─────────┬────────┘         └─────────────────┘
     │                              │
     │    WebSocket 推送            │ TTS 合成
     └──────────────────────────────┼──────────────────┐
                                    │                  │
                          ┌─────────▼────────┐  ┌──────▼──────┐
                          │  WebSocket 广播   │  │  TTS 引擎   │
                          │  (gorilla/ws)    │  │  (多引擎)   │
                          └──────────────────┘  └─────────────┘
```

### 双层配置架构

```
┌─────────────────────────────────────┐
│  第 1 层：命令行参数（flag 包）       │
│  所有配置项均有默认值                 │
├─────────────────────────────────────┤
│  第 2 层：lunar_config.json 覆盖     │
│  仅非空字段才覆盖命令行默认值          │
└─────────────────────────────────────┘
```

详见 [配置管理子系统文档](../subsystem/config/README.md)。

---

## 核心模块说明

### 1. 适配器层（adapters/）

Go↔JavaScript 双向桥接层，基于 [goja](https://github.com/dop251/goja) 运行时实现。

| 适配函数 | JS 调用名 | 功能 |
|---------|----------|------|
| `saveFile` | `saveFile()` | 保存文件到本地存储 |
| `readFile` | `readFile()` | 读取本地文件 |
| `fileView` | `fileView()` | 浏览文件目录 |
| `fileList` | `fileList()` | 获取文件列表 |
| `database` | `database()` | SQLite 数据库操作 |
| `url` | `url()` | 发起 HTTP 请求 |
| `address` | `address()` | IP 定位查询 |
| `syncFetch` | `syncFetch()` | 同步 HTTP 请求 |
| `keyframe` | `keyframe()` | 视频关键帧提取 |
| `resizeImage` | `resizeImage()` | 图片缩放 |
| `generateImage` | `generateImage()` | 图像生成 |
| `atob` | `atob()` | Base64 编解码 |
| `pullContext` | `pullContext()` | 拉取新上下文消息 |
| `pushContext` | `pushContext()` | 推送上下文消息 |
| `pushImage` | `pushImage()` | 推送生成图片 |
| `pullVideoUrl` | `pullVideoUrl()` | 拉取待处理视频 |

**JS 智能体核心**：`agentSystem.js` 由 `server_side/` 目录下的 TypeScript 代码编译而来，包含 AI 角色管理、对话流控制、工具调用等功能。

### 2. 模型代理层（model/llama/）

管理 `llama-server.exe` 进程的生命周期。

- **启动参数**：`--models-preset models.ini --n-gpu-layers 999 --ctx-size 16384 --flash-attn on`
- **就绪检测**：监控 stdout/stderr 输出中的关键信号词（如 `server is listening`），支持 5 分钟超时
- **模式切换**：可配置 `CloudModelUrl` 切换到云端 API 代理
- **请求队列**（[model/core.go](model/core.go)）：控制并发请求数量，超出队列容量时返回「系统繁忙」响应

### 3. TTS 引擎（model/tts/）

文本转语音合成模块的 Go 端封装。

核心流程：

```
AI 回复文本 → 按标点分句 → TTS 引擎合成 WAV → 音频缓存 → WebSocket 推送 → 前端播放
```

TTS 底层由 [Qwen3-TTS 独立引擎](../subsystem/qwen3_tts_lunar/README.md) 提供，月华模块通过 Go 封装的 CGO 接口调用。

### 4. 前端界面（hierarchy/assets/client/）

基于原生 HTML/CSS/JavaScript 的单页 Web 应用，通过 WebView 嵌入桌面窗口。

核心 JS 模块：

| 文件 | 功能 |
|------|------|
| `app.js` | 主应用逻辑、Live2D 交互 |
| `chat.js` | Markdown/Mermaid/ECharts/KaTeX 富文本渲染 |
| `socket.js` | WebSocket 连接管理与消息分发 |
| `live2d.js` | Live2D 模型加载与动作控制 |
| `tts.js` | 语音合成前端播放控制 |
| `fetch.js` | HTTP 请求封装 |
| `file.js` | 文件上传与预览 |
| `util.js` | 通用工具函数 |

### 5. AI 提示词（assets/prompts/）

| 文件 | 用途 |
|------|------|
| `chatRole.md` | 月华角色基础人设与对话风格 |
| `descriptionRole.md` | 场景/物品描述角色 |
| `emotionManager.md` | 情绪状态管理 |
| `imagePrompt.md` | 图像生成提示词扩写 |
| `painterRole.md` | 画师角色设定 |
| `queryKeywords.md` | 搜索关键词提取 |
| `recorderRole.md` | 对话记录整理角色 |
| `selfAppearance.md` | 角色外观自我描述 |
| `summaryRole.md` | 对话摘要整理角色 |

---

## API 接口定义

### HTTP 端点

所有端点由 [server/variable.go](server/variable.go) 中的 `SystemEndpoints` 定义，通过 `registerHandlers()` 自动注册。主要端点包括：

| 路径 | 方法 | 功能 |
|------|------|------|
| `/v1/models` | GET | 获取可用模型列表 |
| `/v1/chat/completions` | POST | OpenAI 格式对话补全 |
| `/write/message` | POST | 写入对话消息 |
| `/generate` | POST | 扩散图像生成 |
| `/video` | POST | 视频上传与关键帧提取 |
| `/ws` | GET | WebSocket 连接升级 |

> 完整的 Storage/Screenshot 子系统端点列表参见 [琉璃系统文档](../crystal_astral/README.md)。

### WebSocket 消息协议

**下行消息格式**（服务端 → 客户端）：

```json
{
  "type": "context | image | tts | emotion",
  "data": { ... }
}
```

| 消息类型 | `type` 值 | `data` 内容 |
|---------|----------|------------|
| 上下文消息 | `context` | `{ type, content }` |
| 图片消息 | `image` | `{ type, images: [] }` |
| TTS 音频 | `tts` | Base64 编码的 WAV 数据 |
| 情绪更新 | `emotion` | 情绪标签字符串 |

**上行消息**（客户端 → 服务端）：由 `readPump()` 持续监听，解析 JSON 后通过 `wsBroadcaster` 通道分发处理。

---

## 编译与运行

### 编译

```powershell
cd d:\Lunar_Astral_Agents\lunar_astral

# 安装前端依赖并编译（仅开发模式）
npm install
npx tsc
npx rollup -c

# 编译 Go 程序
.\build.ps1
```

编译产物：`d:\Lunar_Astral_Agents\LunarAgent.exe`

### 运行

```powershell
# 直接运行
.\LunarAgent.exe

# 可选命令行参数
.\LunarAgent.exe -developer           # 开发模式（直接读取文件系统）
.\LunarAgent.exe -clear-port          # 清理端口后启动
.\LunarAgent.exe -basic-port 36800    # 指定基础端口
```

---

## 配置说明

核心配置由以下来源按优先级合并：

1. **命令行参数**（`-basic-port`、`-developer` 等）—— 最高优先级
2. **`lunar_config.json`** 配置文件（位于可执行文件同目录）—— 覆盖默认值
3. **代码内默认值**（[config 子系统](../subsystem/config/README.md)）—— 兜底值

> 详细配置项说明见 [配置管理子系统文档](../subsystem/config/README.md)。

---

## 常见问题

### Q: JS 智能体代码在哪里修改？

智能体源代码位于 `server_side/` 目录（TypeScript），编译后生成 `hierarchy/assets/agentSystem.js`。修改流程：

```powershell
cd d:\Lunar_Astral_Agents\lunar_astral
npx tsc                         # 编译 TypeScript
npx rollup -c                   # 打包为单文件
```

### Q: 如何切换语言模型？

将 GGUF 格式的模型文件放入 `{LocalDir}/models/` 目录，编辑 `lunar_config.json` 配置 `MultimodalModel` 和 `MmprojModel` 路径即可。

### Q: llama-server 启动失败怎么办？

1. 检查 `llama-server.exe` 文件是否存在（默认路径：`{LocalDir}/models/llama.cpp/llama-server.exe`）
2. 确认模型文件（`models.ini`）配置正确
3. 检查端口是否被占用（默认 36790）
4. 查看控制台输出中的 `llama` 日志

### Q: 如何在浏览器中打开而非 WebView？

设置命令行参数可强制使用系统浏览器。WebView 是默认行为，提供沉浸式桌面体验。

---

## 相关文档

- [项目主文档](../README.md) —— 环境要求、整体架构
- [配置管理子系统](../subsystem/config/README.md) —— 命令行参数与 JSON 配置
- [网页前端子系统](../subsystem/browser/README.md) —— WebView 窗口管理
- [文件管理子系统](../subsystem/storage/README.md) —— 文件与数据库 API
- [屏幕截图子系统](../subsystem/screenshot/README.md) —— 截图服务
- [语音合成独立系统](../subsystem/qwen3_tts_lunar/README.md) —— TTS 引擎详情
- [星图·琉璃](../crystal_astral/README.md) —— 扩展工具集系统