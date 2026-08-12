# 核心系统——星图·月华（lunar_astral）

AI 桌面智能体核心系统，集成多模态对话、TTS 语音合成与图像生成功能。

---

## 人格智能体：月华

**月华**寓意为「月亮的光华」——她的智慧如同月光般温柔而普照大地。

月华的智能体系根植于一个富有诗意的技术隐喻：如同现实中月光本是对太阳光芒的温柔反射，月华的智能体亦是对「伟大之物」——全量参数无量化的 Qwen 大模型——的蒸馏与量化。通过知识蒸馏技术将大模型的智慧浓缩，再经量化压缩使其能在本地轻量运行，恰如月光将炽烈的太阳光芒化为柔和银辉，洒向千家万户。

---

## 功能概述

| 功能 | 说明 |
|------|------|
| AI 智能对话 | 基于本地 GGUF 模型的角色扮演对话，支持多模态输入（文本/图片/视频） |
| TTS 语音合成 | 文本转语音，AI 回复实时合成音频 |
| 图像生成 | 基于 stable-diffusion.cpp 的文生图 |
| 富文本渲染 | 对话中 Markdown / Mermaid / ECharts / KaTeX 渲染 |
| QQ 适配器 | 接入 QQ 群聊，@ 即可唤醒 AI 对话 |

---

## 项目结构

| 目录 | 职责 |
|------|------|
| `adapters/` | Go↔JS 适配器层，基于 goja 运行时将 Go 能力（文件/数据库/网络/图像）暴露为 JS 可调用函数 |
| `model/` | 模型服务层，管理 llama-server 进程生命周期、TTS 引擎调用、请求队列 |
| `server/` | HTTP 服务层，路由注册、CORS、初始化流程、消息/图像/视频处理 |
| `hierarchy/` | 前端资源层，Go embed 嵌入：goja 智能体 JS、角色 Prompt、Web 界面 |
| `websocket/` | WebSocket 通信层，连接管理、读写泵、广播推送 |
| `server_side/` | TypeScript 智能体源码（配置/控制/文件/数学/模型子模块），编译为 agentSystem.js |
| `bridging/` | QQ 群聊适配器，NapCat ↔ 月华消息转发 |

**Go 模块依赖**：`general_config`、`browser_client`、`file_manager`、`image_processor`、`general_logger`、`lunar_chromedp`、`qwen3_tts`

---

## 核心架构

### 启动时序

```
main.go
  ├── config.init()              ← 解析命令行 + JSON 配置
  ├── server.InitializeServer()
  │   ├── registerHandlers()     ← 注册所有 HTTP 路由
  │   ├── llama.Init()           ← 启动 llama-server + 等待就绪
  │   ├── InitTTSEngine()        ← 初始化 TTS 引擎
  │   ├── websocket.Setup()      ← 注册 /ws 端点
  │   └── adapters.RunAgentContext()
  │       ├── createAgentContext()  ← 创建 goja eventloop 运行时
  │       ├── 注册适配器函数
  │       └── 执行 agentSystem.js  ← TypeScript 编译的智能体代码
  ├── SetupSignalHandling()      ← 系统信号监听
  ├── StartServerListener()      ← 端口自动递增重试（最多 10 次）
  └── WaitForShutdown()          ← 优雅关闭
```

### 核心数据流

```
前端 UI (WebView) → HTTP POST /write/message → Go 服务层 → JS 智能体 (goja)
                                                              │
                                          ┌───────────────────┼───────────────────┐
                                          ▼                   ▼                   ▼
                                    llama-server          TTS 引擎           SD 引擎
                                    (GGUF 推理)        (C++ GGML)        (C++ GGML)
                                          │                   │                   │
                                          └───────────────────┼───────────────────┘
                                                              ▼
                                                    WebSocket 推送
                                                              │
                                                              ▼
                                                    前端渲染 (Markdown/Mermaid/ECharts)
```

### 双层配置架构

1. **命令行参数**（`-basic-port`、`-developer` 等）—— 最高优先级
2. **`lunar_config.json`** 配置文件 —— 覆盖默认值

> 详细配置项见 [配置管理子系统文档](../subsystem/general_config/README.md)。

---

## 核心模块

### 适配器层（adapters/）

Go↔JavaScript 双向桥接，基于 goja 运行时。暴露出 `saveFile()`、`readFile()`、`database()`、`url()`、`generateImage()` 等函数供 JS 智能体调用。

### 模型代理层（model/llama/）

管理 `llama-server.exe` 进程生命周期，启动参数：`--models-preset models.ini --n-gpu-layers 999 --ctx-size 16384 --flash-attn on`。支持就绪检测（5 分钟超时）、云端 API 切换、请求队列控制。

### TTS 引擎（model/tts/）

AI 回复文本 → 按标点分句 → TTS 引擎合成 WAV → 音频缓存 → WebSocket 推送 → 前端播放。底层由 Qwen3-TTS 独立引擎提供。

### 前端界面（hierarchy/assets/client/）

基于原生 HTML/CSS/JavaScript 的单页 Web 应用，通过 WebView 嵌入桌面窗口。核心模块：`app.js`（主逻辑）、`chat.js`（富文本渲染）、`socket.js`（WebSocket）、`tts.js`（语音播放）。

### AI 提示词（assets/prompts/）

| 文件 | 用途 |
|------|------|
| `dialogueRole.md` | 月华角色基础人设与对话风格 |
| `descriptionRole.md` | 场景/物品描述角色 |
| `organizeRole.md` | 整理角色设定 |
| `painterRole.md` | 画师角色设定 |
| `selfAppearance.md` | 角色外观自我描述 |

---

## API 接口

| 路径 | 方法 | 功能 |
|------|------|------|
| `/v1/models` | GET | 获取可用模型列表 |
| `/v1/chat/completions` | POST | OpenAI 格式对话补全 |
| `/write/message` | POST | 写入对话消息 |
| `/generate` | POST | 扩散图像生成 |
| `/video` | POST | 视频上传与关键帧提取 |
| `/ws` | GET | WebSocket 连接升级 |

### WebSocket 消息协议

| 消息类型 | `type` 值 | 内容 |
|---------|----------|------|
| 上下文消息 | `context` | AI 对话文本 |
| 图片消息 | `image` | 生成/上传的图片 |
| TTS 音频 | `tts` | Base64 编码 WAV |
| 情绪更新 | `emotion` | 情绪标签字符串 |

---

## 编译与运行

### 编译

```powershell
cd d:\Lunar_Astral_Agents\lunar_astral
.\build.ps1
```

`build.ps1` 内部自动完成图标编译、前端 TypeScript 编译打包、Go 编译。

编译产物：`d:\Lunar_Astral_Agents\Lunar_Astral.exe`

### 运行

```powershell
.\Lunar_Astral.exe                           # 直接运行
.\Lunar_Astral.exe -developer                # 开发模式（直读文件系统）
.\Lunar_Astral.exe -clear-port               # 清理端口后启动
.\Lunar_Astral.exe -basic-port 36800         # 指定基础端口
```

---

## 常见问题

**如何切换语言模型？** 将 GGUF 模型放入 `local_data/models/`，编辑 `lunar_config.json` 配置 `MultimodalModel` 和 `MmprojModel` 路径。

**llama-server 启动失败？** 检查 `llama-server.exe` 是否存在、模型文件配置正确、端口未被占用、查看控制台 llama 日志。

**JS 智能体代码在哪里修改？** 源码位于 `server_side/` 目录（TypeScript），编译后生成 `hierarchy/assets/agentSystem.js`。修改后重新执行 `.\build.ps1`。

---

## 相关文档

- [项目主文档](../README.md) — 环境要求、整体架构
- [项目架构说明](../ARCHITECTURE.md) — 完整架构
- [星图·琉璃](../crystal_astral/README.md) — 扩展工具集
- [配置管理](../subsystem/general_config/README.md) — 命令行参数与 JSON 配置
- [网络检索](../subsystem/lunar_chromedp/README.md) — AI 搜索智能体详情
- [语音合成](../subsystem/qwen3_tts/README.md) — TTS 引擎详情
- [语音识别](../subsystem/qwen_asr_lunar/README.md) — ASR 引擎详情