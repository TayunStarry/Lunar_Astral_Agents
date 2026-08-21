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
| `model/` | 模型服务层，管理 llama-server 进程生命周期与请求队列（TTS 引擎由独立子系统 qwen3_tts 提供） |
| `server/` | HTTP 服务层，路由注册、CORS、初始化流程、消息/图像/视频处理 |
| `hierarchy/` | 前端资源层，Go embed 嵌入：goja 智能体 JS、角色 Prompt、Web 界面 |
| `websocket/` | WebSocket 通信层，连接管理、读写泵、广播推送 |
| `TypeScript/` | TypeScript 智能体源码（配置/控制/文件/数学/模型子模块），编译为 agentSystem.js |
| `learner/` | 学习者智能体，自主搜索学习并沉淀为记忆 |
| `bridging/` | QQ 群聊适配器，NapCat ↔ 月华消息转发 |

**Go 子系统依赖**（以 `LunarSubsystem/` 前缀引入）：`GeneralConfig`、`BrowserClient`、`FileManager`、`LoggerGeneral`、`ImageProcessor`、`AgentSearch`、`Qwen3-TTS`

---

## 核心架构

### 启动时序

启动调用链与各步骤对应实现，见 [Code Wiki 02 §2 启动时序](../docs/code-wiki/02-核心系统-星图月华.md)，此处不重复。

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

Go↔JavaScript 双向桥接，基于 goja 运行时。暴露出 `saveFile()`、`readFile()`、`knowledge()`、`memoryQuery()`、`url()`、`generateImage()`、`keyframe()`、`resizeImage()`、`tts()`、`screenshotCapture()` 等函数供 JS 智能体调用。

### 模型代理层（model/llama/）

管理 `llama-server.exe` 进程生命周期，启动参数：`--models-preset local_data/models/models.ini --flash-attn on --port <ModelPort> --parallel 1 --batch-size 2048 --ubatch-size 1024 --cache-type-k q8_0 --cache-type-v q8_0 --no-ui --sleep-idle-seconds 900`（含温度/采样等生成参数）。模型由 `models.ini` 预设文件管理（`[system-multimodal]` 多模态、`[system-embedding]` 嵌入）。支持就绪检测（5 分钟超时）、embedding 请求本地路由、多模态请求云端代理切换。

### TTS 引擎（subsystem/qwen3_tts）

AI 回复文本 → 按标点分句 → TTS 引擎合成 WAV → 音频缓存 → WebSocket 推送 → 前端播放。TTS 引擎由独立子系统 `LunarSubsystem/Qwen3-TTS/module` 提供（`server/variable.go` 引入该模块并注册 `/tts` 端点）。

### 前端界面（hierarchy/assets/client/）

基于原生 HTML/CSS/JavaScript 的单页 Web 应用，通过 WebView 嵌入桌面窗口。核心模块：`app.js`（主逻辑）、`chat.js`（富文本渲染）、`socket.js`（WebSocket）、`tts.js`（语音播放）。

### AI 提示词（assets/prompts/）

| 文件 | 用途 |
|------|------|
| `dialogueRole.md` | 月华角色基础人设与对话风格 |
| `descriptionRole.md` | 视频关键帧描述指南 |
| `painterRole.md` | 画师角色设定 |
| `selfAppearance.md` | 角色外观自我描述 |
| `viewerRole.md` | 观影者角色 |
| `actorRole.md` | 行动者角色 |
| `musicianRole.md` | 音乐作曲家角色 |
| `learner*.md` | 学习者智能体系列（研究报告 / 查询推理完善 / 策略评估 / 搜索内容评估 / 记忆更新） |

---

## API 接口

| 路径 | 方法 | 功能 |
|------|------|------|
| `/v1/` | ANY | 智能体代理接口（转发到 llama.cpp，支持所有 HTTP 方法） |
| `/write/message` | POST | 消息写入队列 |
| `/write/videourl` | POST | 视频 URL 写入 |
| `/generate` / `/generate/wait` | POST / GET | 图像生成 / 等待生成结果 |
| `/extract/keyframes` | POST | 视频关键帧提取 |
| `/tts` | POST | TTS 语音合成 |
| `/memory/` | ANY | 记忆库管理（实例初始化/集合管理/消息增删查） |
| `/knowledge/` | POST | 知识库管理 |
| `/file/...` | 多种 | 文件读写/列表/下载/归档/扩展包管理 |
| `/api/engine/command` | POST | 智能体引擎命令转发 |
| `/proxy` | POST | 代理访问服务 |
| `/ltpx/load` `/ltpx/unload` `/ltpx/status` | POST/POST/GET | LTPX 工具包动态管理 |
| `/ws` | GET | WebSocket 连接升级 |

### WebSocket 消息协议

| 消息类型 | `type` 值 | 内容 |
|---------|----------|------|
| 上下文消息 | `context` | AI 对话文本（`type`/`content`/`audio` 字段；子类型含 thinking、response、music、music_audio 等） |
| 图片消息 | `image` | 生成/上传的图片（`images` 数组） |

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
.\Lunar_Astral.exe -developer                # 调试模式（显示详细日志）
.\Lunar_Astral.exe -basic-port 36800         # 指定基础端口
```

---

## 常见问题

**如何切换语言模型？** 将 GGUF 模型放入 `local_data/models/`，编辑 `local_data/models/models.ini`：`[system-multimodal]` 段配置多模态模型的 `model` / `mmproj` 路径，`[system-embedding]` 段配置嵌入模型的 `model` 路径。修改后重启程序生效。

**llama-server 启动失败？** 检查 `llama-server.exe` 是否存在、`models.ini` 中模型路径配置正确、端口未被占用、查看控制台 llama 日志。

**JS 智能体代码在哪里修改？** 源码位于 `TypeScript/` 目录，编译后生成 `hierarchy/assets/agentSystem.js`。修改后重新执行 `.\build.ps1`。

---

## 相关文档

> 📚 **代码级文档**：见 [Code Wiki 02·核心系统-月华](../docs/code-wiki/02-核心系统-星图月华.md)，综合入口 [Code Wiki 门户](../docs/code-wiki/README.md)。

- [项目主文档](../README.md) — 环境要求、整体架构
- [项目架构说明](../ARCHITECTURE.md) — 完整架构
- [星图·琉璃](../crystal_astral/README.md) — 扩展工具集
- [配置管理](../subsystem/general_config/README.md) — 命令行参数与 JSON 配置
- [网络检索](../subsystem/agent_search/README.md) — AI 搜索智能体详情
- [语音合成](../subsystem/qwen3_tts/README.md) — TTS 引擎详情
- [语音识别](../subsystem/qwen_asr/README.md) — ASR 引擎详情