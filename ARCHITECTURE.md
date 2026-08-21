# 星月智能（Lunar Astral Agents）—— 项目架构

基于 **Go + TypeScript + C/C++** 的纯本地化桌面 AI 智能体平台，零 Python 依赖。

> 📚 **代码级细节**：本文从架构层面概述系统；要深入具体类与函数，请配合 [Code Wiki 综合文档门户](docs/code-wiki/README.md)（尤其 [01 项目架构总览](docs/code-wiki/01-项目架构总览.md) 与 [07 依赖关系](docs/code-wiki/07-依赖关系.md)）阅读，两套文档互相超链接、查漏补缺。

---

## 人格智能体

- **月华** — AI 桌面智能体核心，掌管多模态对话、TTS 语音合成与图像生成
- **琉璃** — 工具集扩展系统，掌管文件管理、知识库/记忆库、截图标注、AI 代理转发

---

## 整体架构

```
┌──────────────────────────────────────────────────────────────┐
│                     星月智能平台                              │
│                                                              │
│  ┌───────────────────────┐  ┌───────────────────────┐        │
│  │  星图·月华              │  │  星图·琉璃              │        │
│  │  AI 桌面智能体核心       │  │  工具集扩展系统         │        │
│  │  adapters/server/model  │  │  handler/endpoint     │        │
│  │  websocket/TypeScript   │  │  assets/              │        │
│  └───────────┬────────────┘  └───────────┬────────────┘        │
│              └────────────┬─────────────┘                      │
│                           │                                    │
│  ┌────────────────────────┼────────────────────────────────┐   │
│  │           公共子系统 (subsystem/)                         │   │
│  │  general_config · browser_client · file_manager · image_processor · logger_general             │   │
│  └─────────────────────────────────────────────────────────┘   │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │           独立 AI 引擎 (subsystem/)                      │   │
│  │  qwen3_tts (C++ GGML) · qwen_asr (纯C)      │   │
│  │  agent_search (网络检索)                               │   │
│  └─────────────────────────────────────────────────────────┘   │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │           运维工具 (subsystem/)                           │   │
│  │  environment_repair（资源补全/端口释放/HTTPS代理/打包）    │   │
│  └─────────────────────────────────────────────────────────┘   │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │           本地数据 (local_data/)                          │   │
│  │  models/ (GGUF 模型) · package/ (前端共享资源库)          │   │
│  └─────────────────────────────────────────────────────────┘   │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │           外部推理引擎                                    │   │
│  │  llama.cpp (GGUF 文本) · stable-diffusion.cpp (图像)     │   │
│  └─────────────────────────────────────────────────────────┘   │
└──────────────────────────────────────────────────────────────┘
```

---

## 文件夹结构

```
Lunar_Astral_Agents/
├── lunar_astral/          # 核心系统：星图·月华
│   ├── adapters/          # Go↔JS 桥接层（goja 运行时）
│   ├── hierarchy/         # 前端资源（embed 嵌入）
│   │   └── assets/
│   │       ├── client/    # WebView 前端 UI
│   │       ├── prompts/   # AI 角色提示词模板
│   │       └── agentSystem.js  # TypeScript 编译产物
│   ├── model/             # 模型服务层（llama 代理 + TTS 引擎）
│   ├── server/            # HTTP 服务层 + 请求处理器
│   ├── TypeScript/        # TypeScript 智能体源码
│   ├── websocket/         # WebSocket 实时通信
│   └── bridging/          # QQ 群聊适配器（NapCat）
│
├── crystal_astral/        # 扩展系统：星图·琉璃
│   └── assets/            # 前端静态资源（embed 嵌入）
│
├── subsystem/             # 可复用子系统
│   ├── general_config/     # 全局配置中枢
│   ├── browser_client/     # WebView 窗口 + IP 发现
│   ├── file_manager/       # 文件管理 + 知识库/记忆库 + 扩展包
│   ├── image_processor/    # 图像生成 + 截图 + 视频关键帧
│   ├── logger_general/     # 彩色终端日志
│   ├── agent_search/    # 智能网络检索（Chromedp）
│   ├── qwen3_tts/   # 语音合成（C++ GGML 引擎）
│   ├── qwen_asr/    # 语音识别（纯 C 引擎）
│   └── environment_repair/ # 运维工具箱（资源补全/端口释放/HTTPS代理/打包）
│
├── local_data/            # 本地运行时数据
│   ├── models/            # AI 模型文件（GGUF/SafeTensors）
│   └── package/           # 前端共享资源库（UI 模块 + 第三方库）
```

---

## 核心系统：lunar_astral

| 模块 | 职责 |
|------|------|
| `adapters/` | Go↔JS 双向桥接，基于 goja 运行时将 Go 能力暴露为 JS 可调用函数 |
| `hierarchy/` | 前端资源容器，Go embed 嵌入，含角色 Prompt、Web 界面 |
| `model/` | 模型服务层，管理 llama-server 进程生命周期与请求队列（TTS 引擎由独立子系统 qwen3_tts 提供） |
| `server/` | HTTP 服务入口，路由注册、CORS、初始化编排 |
| `TypeScript/` | TypeScript 智能体源码，编译为 agentSystem.js 在 goja 中执行 |
| `websocket/` | 实时双向通信，连接管理、读写泵、广播推送 |
| `bridging/` | QQ 群聊适配器，NapCat ↔ 月华消息转发 |
| `learner/` | 学习者智能体，自主搜索学习并沉淀为记忆 |

**Go 模块依赖**：`general_config`、`browser_client`、`file_manager`、`image_processor`、`logger_general`、`agent_search`、`qwen3_tts`

**数据流**：前端 UI → HTTP POST `/write/message` → Go 服务层 → goja JS 智能体 → llama-server (GGUF 推理) → WebSocket 推送 → 前端渲染

---

## 扩展系统：crystal_astral

| 模块 | 职责 |
|------|------|
| `main.go` | 程序入口，随机端口 + 启动服务器 |
| `create.go` | 服务器创建、代理感知路由（`/v1/` 请求转发至月华 llama-proxy） |
| `handler.go` | 代理转发、GGUF 元数据解析、图片转换、月华服务启停 |
| `gguf.go` | GGUF 模型文件解析 |
| `convert.go` | 图片格式转换（单张/批量/列表） |
| `ws.go` | StudioHub WebSocket 集线器 + 引擎命令桥接 |
| `type.go` | 请求/响应类型定义 |
| `variable.go` | 全局变量与 SystemEndpoints 路由表 |
| `assets/` | 前端静态资源（毛玻璃风格 UI） |

**Go 模块依赖**：`general_config`、`browser_client`、`file_manager`、`image_processor`、`logger_general`

琉璃通过智能路由将 AI 请求代理到月华后端（localhost:36789），同时直接服务自己的文件管理、知识库/记忆库、截图等工具界面。

---

## 子系统模块

### 公共基础设施

| 子系统 | 功能 | 依赖方 |
|--------|------|--------|
| `general_config` | 全局配置中枢，命令行参数 + JSON 双层配置 | 所有 Go 模块 |
| `browser_client` | WebView 窗口管理 + 本地 IP 自动发现 | 月华、琉璃 |
| `file_manager` | 文件 CRUD + 知识库/记忆库 + ZIP 归档 + 扩展包管理 | 月华、琉璃 |
| `image_processor` | 扩散图像生成 + 截图（多显示器/区域）+ 视频关键帧提取 | 月华、琉璃 |
| `logger_general` | 彩色终端日志输出 | 所有 Go 模块 |

### 独立 AI 引擎

| 子系统 | 功能 | 技术栈 |
|--------|------|--------|
| `qwen3_tts` | Qwen3-TTS 文本转语音，支持音色克隆与 LRU 缓存 | C++ GGML 引擎 + Go HTTP 服务 |
| `qwen_asr` | Qwen3-ASR 语音转文本，30 种语言，BF16 推理 | 纯 C 引擎 + OpenBLAS + Go HTTP |
| `agent_search` | 智能网络检索：多引擎搜索 → 页面提取 → AI 摘要 → 记忆存储 | Chromedp + LLM API |

### 运维工具

| 子系统 | 功能 |
|--------|------|
| `environment_repair` | 资源补全修复、端口占用释放、HTTPS 代理（WSS→WS + CORS）、分卷打包归档 |

---

## 跨模块依赖

### Go Module 依赖图

```
lunar_astral → general_config, browser_client, file_manager, image_processor, logger_general, agent_search, qwen3_tts
crystal_astral → general_config, browser_client, file_manager, image_processor, logger_general
environment_repair → general_config
agent_search → file_manager, general_config, logger_general
```

### 数据流

```
用户输入 → HTTP API → Go 服务层 → JS 智能体 (goja) → llama-server (GGUF)
                                                    → TTS 引擎 (C++ GGML)
                                                    → SD 引擎 (C++ GGML)
         → WebSocket 推送 → 前端渲染 (Markdown/Mermaid/ECharts)
```

### 前端资源引用

所有前端 UI 统一引用 `local_data/package/standard_dependency/` 获取基础样式与脚本，通过 `/file/read/package/` 路径加载第三方库（ECharts、marked、mermaid、KaTeX 等）。

---

## 技术栈总览

| 层级 | 技术 |
|------|------|
| 前端 UI | HTML5 + CSS3 + Vanilla JS (ES6+)，玻璃拟态风格，WebView 嵌入 |
| AI 智能体 | TypeScript → goja 运行时（Go 进程中执行） |
| 后端服务 | Go 1.26，HTTP API + WebSocket |
| 文本推理 | llama.cpp (llama-server)，GGUF 格式 |
| 图像生成 | stable-diffusion.cpp，GGUF 格式 |
| 语音合成 | Qwen3-TTS，C++ GGML 引擎 |
| 语音识别 | Qwen3-ASR，纯 C 引擎 + OpenBLAS |
| 数据存储 | SQLite (go-sqlite3)，本地嵌入式 |