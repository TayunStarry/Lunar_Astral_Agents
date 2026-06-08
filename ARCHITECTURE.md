# 星月智能（Lunar Astral Agents）—— 项目架构说明

> 基于 **Go + TypeScript + C/C++** 的纯本地化桌面 AI 智能体平台，零 Python 依赖。

---

## 目录

- [整体架构图](#整体架构图)
- [文件夹结构总览](#文件夹结构总览)
- [顶层目录详解](#顶层目录详解)
- [核心系统：lunar_astral](#核心系统lunar_astral)
- [扩展系统：crystal_astral](#扩展系统crystal_astral)
- [子系统模块：subsystem](#子系统模块subsystem)
- [数据与资源：local_data](#数据与资源local_data)
- [跨模块依赖关系](#跨模块依赖关系)

---

## 整体架构图

```
┌──────────────────────────────────────────────────────────────────────────┐
│                           星月智能平台                                    │
│                        Lunar Astral Agents                              │
│                                                                         │
│  ┌───────────────────────────────┐  ┌───────────────────────────────┐   │
│  │     星图·月华 (lunar_astral)   │  │    星图·琉璃 (crystal_astral)  │   │
│  │       AI 桌面智能体核心         │  │        工具集扩展系统          │   │
│  │                               │  │                               │   │
│  │  ┌─────────┐  ┌───────────┐  │  │  ┌─────────┐  ┌───────────┐  │   │
│  │  │adapters │  │  server   │  │  │  │handler  │  │ endpoint  │  │   │
│  │  │Go↔JS桥接│  │ HTTP 服务 │  │  │  │代理转发  │  │ API 路由  │  │   │
│  │  └─────────┘  └───────────┘  │  │  └─────────┘  └───────────┘  │   │
│  │  ┌─────────┐  ┌───────────┐  │  │  ┌─────────────────────────┐  │   │
│  │  │  model  │  │ websocket │  │  │  │      assets/           │  │   │
│  │  │模型服务 │  │  实时通信  │  │  │  │   前端静态资源         │  │   │
│  │  └─────────┘  └───────────┘  │  │  └─────────────────────────┘  │   │
│  │  ┌─────────────────────────┐  │  │                               │   │
│  │  │  server_side (TS 智能体) │  │  │                               │   │
│  │  │  → 编译为 agentSystem.js │  │  │                               │   │
│  │  └─────────────────────────┘  │  │                               │   │
│  └──────────────┬────────────────┘  └──────────────┬────────────────┘   │
│                 │                                    │                    │
│                 └──────────────┬─────────────────────┘                    │
│                                │                                         │
│  ┌─────────────────────────────┼─────────────────────────────────────┐  │
│  │                    公共子系统 (subsystem/)                          │  │
│  │                             │                                      │  │
│  │  ┌──────────┐ ┌─────────┐ ┌──────────┐ ┌──────────┐            │  │
│  │  │  config  │ │ browser │ │ storage  │ │screenshot │            │  │
│  │  │ 配置中枢 │ │WebView  │ │文件+SQLite│ │ 屏幕截图  │            │  │
│  │  └──────────┘ └─────────┘ └──────────┘ └──────────┘            │  │
│  │  ┌──────────┐                                                    │  │
│  │  │  logger  │                                                    │  │
│  │  │ 彩色日志 │                                                    │  │
│  │  └──────────┘                                                    │  │
│  └──────────────────────────────────────────────────────────────────┘  │
│                                                                         │
│  ┌──────────────────────────────────────────────────────────────────┐  │
│  │                    扩展子系统 (subsystem/)                        │  │
│  │                                                                  │  │
│  │  ┌────────────┐ ┌──────────────┐ ┌────────────────┐             │  │
│  │  │ LunarTick  │ │bridge_adapter│ │gguf_metadata   │             │  │
│  │  │ tick执行引擎│ │ QQ群聊适配器  │ │ GGUF元数据查看 │             │  │
│  │  └────────────┘ └──────────────┘ └────────────────┘             │  │
│  │  ┌────────────┐ ┌──────────────┐                                │  │
│  │  │   proxy    │ │volume_archive│                                │  │
│  │  │HTTPS代理服务│ │  卷归档管理   │                                │  │
│  │  └────────────┘ └──────────────┘                                │  │
│  └──────────────────────────────────────────────────────────────────┘  │
│                                                                         │
│  ┌──────────────────────────────────────────────────────────────────┐  │
│  │                    独立 AI 引擎 (subsystem/)                      │  │
│  │                                                                  │  │
│  │  ┌──────────────────────┐  ┌──────────────────────┐             │  │
│  │  │  qwen3_tts_lunar     │  │  qwen_asr_lunar      │             │  │
│  │  │  语音合成 (C++ GGML) │  │  语音识别 (纯C+BLAS) │             │  │
│  │  └──────────────────────┘  └──────────────────────┘             │  │
│  │  ┌──────────────────────┐                                       │  │
│  │  │     sd_lunar         │                                       │  │
│  │  │ 图像生成 (C++ GGML)  │                                       │  │
│  │  └──────────────────────┘                                       │  │
│  └──────────────────────────────────────────────────────────────────┘  │
│                                                                         │
│  ┌──────────────────────────────────────────────────────────────────┐  │
│  │                    本地数据与前端资源 (local_data/)                │  │
│  │                                                                  │  │
│  │  ┌──────────────────────┐  ┌──────────────────────┐             │  │
│  │  │      models/         │  │      package/        │             │  │
│  │  │  AI 模型文件 (GGUF)  │  │  前端共享资源库       │             │  │
│  │  └──────────────────────┘  └──────────────────────┘             │  │
│  └──────────────────────────────────────────────────────────────────┘  │
│                                                                         │
│  ┌──────────────────────────────────────────────────────────────────┐  │
│  │                    外部推理引擎                                   │  │
│  │  ┌──────────────┐  ┌──────────────────┐                          │  │
│  │  │  llama.cpp   │  │stable-diffusion  │                          │  │
│  │  │ (GGUF 文本)  │  │  .cpp (图像)     │                          │  │
│  │  └──────────────┘  └──────────────────┘                          │  │
│  └──────────────────────────────────────────────────────────────────┘  │
└──────────────────────────────────────────────────────────────────────────┘
```

---

## 文件夹结构总览

```
Lunar_Astral_Agents/
├── image/                        # 项目图片资源
├── local_data/                   # 本地数据与前端资源
│   ├── models/                   # AI 模型文件
│   │   ├── Qwen3-ASR-0.6B/       # ASR 模型
│   │   ├── document/             # 参数文档
│   │   ├── live2d/              # Live2D 模型配置
│   │   └── stable_diffusion.cpp/ # SD 参数文档
│   └── package/                  # 前端共享资源库
│       ├── archive/             # 归档与许可
│       ├── database_manager/    # 数据库管理 UI
│       ├── different_lunar/     # 月华前端主界面
│       ├── file_explorer/       # 文件浏览器 UI
│       ├── fontAwesome/         # Font Awesome 6.4.0
│       ├── highlight/           # highlight.js
│       ├── image_generation/    # 图像生成 UI
│       ├── katex/               # KaTeX 数学库
│       ├── message_rendering/   # 消息渲染 UI
│       ├── model_query/         # 模型查询 UI
│       ├── multimedia_preview/   # 多媒体预览 UI
│       ├── parameter_assistant/ # 参数助手 UI
│       ├── qwen3_tts/           # TTS 语音合成 UI
│       ├── screenshot_manager/  # 截图管理 UI
│       ├── standard_dependency/ # 全局标准依赖
│       └── vector_db_manager/   # 向量数据库管理 UI
├── lunar_astral/                # 核心系统：星图·月华
│   ├── adapters/                # Go↔JS 适配器层
│   ├── hierarchy/               # 前端资源与脚本
│   │   ├── assets/             # 前端静态资源
│   │   │   ├── client/         # 前端 Web UI
│   │   │   └── prompts/        # AI 角色提示词模板
│   │   └── image/              # 图像生成模块
│   │       └── generate/       # 生成逻辑与类型
│   ├── model/                  # 模型服务层
│   │   ├── llama/              # llama.cpp 代理
│   │   └── tts/                # TTS 语音合成引擎
│   ├── release/                # 进程/端口管理
│   ├── server/                 # HTTP 服务器层
│   │   └── handlers/           # HTTP 请求处理器
│   ├── server_side/            # TypeScript 智能体源码
│   │   ├── config/             # 全局配置
│   │   ├── control/            # 对话控制
│   │   ├── file/               # 文件处理
│   │   ├── math/               # 数学工具
│   │   └── model/              # AI 模型逻辑
│   └── websocket/              # WebSocket 通信层
├── crystal_astral/             # 扩展系统：星图·琉璃
│   └── assets/                 # 前端静态资源
└── subsystem/                  # 可复用子系统模块
    ├── bridge_adapter/         # QQ 群聊适配器
    │   ├── pkg/                # 核心包
    │   └── template/           # 消息模板
    ├── browser/                # WebView 窗口管理
    ├── config/                 # 全局配置中枢
    ├── gguf_metadata_viewer/   # GGUF 元数据查看器
    │   ├── gguf/               # GGUF 二进制解析
    │   └── server/             # HTTP 服务 + 前端
    │       └── static/        # 前端静态文件
    ├── logger/                 # 彩色终端日志
    ├── LunarTick/              # tick 驱动执行引擎
    │   ├── api/                # HTTP API 服务
    │   ├── cmd/lunartick/      # CLI 入口
    │   └── engine/             # 核心引擎
    ├── proxy/                  # HTTPS 代理服务器
    │   ├── cmd/                # CLI 入口
    │   └── frontend/proxy_ui/  # 代理管理 UI
    ├── qwen3_tts_lunar/        # 语音合成（Qwen3-TTS）
    │   ├── client/             # 前端 UI
    │   ├── cpp/                # C++ 推理引擎
    │   │   ├── ggml/           # GGML 张量计算库
    │   │   └── src/            # TTS 引擎源码
    │   └── module/             # Go 逻辑层
    ├── qwen_asr_lunar/         # 语音识别（Qwen3-ASR）
    │   ├── openblas/           # OpenBLAS 线性代数库
    │   └── static/             # 前端 UI
    ├── screenshot/             # 屏幕截图模块
    ├── sd_lunar/               # Stable Diffusion 图像生成
    │   ├── assets/             # 前端 UI
    │   └── cpp/                # C++ GGML 推理引擎
    │       └── ggml/           # GGML 张量计算库
    ├── storage/                # 文件管理 + SQLite
    │   ├── module/             # 核心业务逻辑
    │   └── server/             # HTTP 服务层
    └── volume_archive/         # 卷归档管理
        └── component/          # 核心组件
```

---

## 顶层目录详解

### `image/`

| 维度 | 说明 |
|------|------|
| **主要职责** | 存放项目展示用的图片资源，包括系统截图、角色人设图、功能界面截图等 |
| **核心模块类型** | WebP/JPG 图片文件 |
| **关联关系** | 被 `README.md`、`lunar_astral/README.md`、`crystal_astral/README.md` 引用作为文档配图 |
| **架构角色** | 项目文档与展示层，不参与运行时逻辑 |

### `local_data/`

| 维度 | 说明 |
|------|------|
| **主要职责** | 承载本地运行时数据（AI 模型文件）与前端共享资源库（UI 组件、第三方库、各功能模块界面） |
| **核心模块类型** | GGUF/SafeTensors 模型文件、HTML/CSS/JS 前端界面、第三方 JS 库 |
| **关联关系** | `lunar_astral` 和 `crystal_astral` 通过 HTTP 静态文件服务（`/file/read/package/`）读取此目录的前端资源；`local_data/models/` 为 `model/llama/` 代理层提供 GGUF 模型文件 |
| **架构角色** | 数据持久层与前端资源中心，是所有前端 UI 的物理存储位置 |

### `lunar_astral/`

| 维度 | 说明 |
|------|------|
| **主要职责** | AI 桌面智能体核心系统（星图·月华），集成多模态对话、Live2D 角色展示、TTS 语音合成、图像生成、WebSocket 实时通信 |
| **核心模块类型** | Go 后端服务、TypeScript 智能体、HTML/CSS/JS 前端、AI 提示词模板 |
| **关联关系** | 依赖 `subsystem/config`（配置）、`subsystem/browser`（WebView）、`subsystem/storage`（文件+数据库）、`subsystem/screenshot`（截图）、`subsystem/qwen3_tts_lunar`（TTS 引擎）；通过代理转发为 `crystal_astral` 提供 AI 推理能力 |
| **架构角色** | 平台核心，所有 AI 能力的入口与调度中心。详见 [lunar_astral/README.md](lunar_astral/README.md) |

### `crystal_astral/`

| 维度 | 说明 |
|------|------|
| **主要职责** | 工具集扩展系统（星图·琉璃），提供文件管理、数据库管理、截图标注、AI 代理转发、应用加载器等功能 |
| **核心模块类型** | Go 后端服务、HTML/CSS/JS 前端 |
| **关联关系** | 依赖 `subsystem/config`、`subsystem/browser`、`subsystem/storage`、`subsystem/screenshot`；通过代理感知路由将 `/v1/` 请求转发至 `lunar_astral` 的 llama-proxy |
| **架构角色** | 平台扩展层，为月华核心系统提供辅助工具集。详见 [crystal_astral/README.md](crystal_astral/README.md) |

### `subsystem/`

| 维度 | 说明 |
|------|------|
| **主要职责** | 承载所有可复用的子系统模块，包括公共基础设施（配置、浏览器、存储、截图、日志）和扩展功能（TTS、ASR、SD、代理、QQ 适配等） |
| **核心模块类型** | Go 库模块、C/C++ 推理引擎、HTML/CSS/JS 前端界面 |
| **关联关系** | 被 `lunar_astral` 和 `crystal_astral` 以 Go module 方式引用；部分子系统（TTS、ASR、SD）可独立编译为可执行文件 |
| **架构角色** | 平台基础设施层，为核心系统和扩展系统提供可复用的能力支撑 |

---

## 核心系统：lunar_astral

> 📖 详细文档：[lunar_astral/README.md](lunar_astral/README.md)

### `adapters/`

| 维度 | 说明 |
|------|------|
| **主要职责** | Go↔JavaScript 双向桥接层，基于 goja 运行时实现，将 Go 的文件系统、数据库、网络、视觉等能力暴露为 JS 可调用函数 |
| **核心模块类型** | CGO 桥接适配器、goja 运行时管理、类型定义 |
| **关联关系** | 被 `server/manage.go` 在初始化阶段调用创建 goja 运行时；适配器函数供 `hierarchy/assets/agentSystem.js`（即 `server_side/` 编译产物）调用 |
| **架构角色** | Go 后端与 JS 智能体之间的胶水层，是 TypeScript 智能体能够操作本地资源的唯一通道 |

### `hierarchy/`

| 维度 | 说明 |
|------|------|
| **主要职责** | 前端资源与脚本的容器，通过 Go `embed.FS` 嵌入到编译产物中，实现单文件分发 |
| **核心模块类型** | Go embed 嵌入声明、前端 HTML/CSS/JS、AI 提示词 Markdown 模板、图像/视频处理 Go 代码 |
| **关联关系** | `assets/client/` 为 WebView 提供前端界面；`assets/prompts/` 为 JS 智能体提供角色设定；`assets/agentSystem.js` 由 `server_side/` 编译生成；`image/generate/` 被 `server/handlers/generate.go` 调用 |
| **架构角色** | 前端资源打包层，确保所有静态资源编译进单个可执行文件 |

#### `hierarchy/assets/client/`

| 维度 | 说明 |
|------|------|
| **主要职责** | 月华系统的前端 Web UI，基于原生 HTML/CSS/JavaScript 的单页应用，通过 WebView 嵌入桌面窗口 |
| **核心模块类型** | HTML 页面、CSS 样式、ES Module JS 脚本 |
| **关联关系** | 通过 WebSocket 与 `websocket/` 层通信；通过 Fetch API 与 `server/handlers/` 交互；引用 `local_data/package/` 下的共享资源 |
| **架构角色** | 用户交互层，承载 Live2D 角色渲染、Markdown/Mermaid/ECharts 富文本、TTS 播放等前端能力 |

#### `hierarchy/assets/prompts/`

| 维度 | 说明 |
|------|------|
| **主要职责** | AI 角色提示词模板库，定义月华的角色人设、对话风格、情绪管理、图像生成提示等 |
| **核心模块类型** | Markdown 提示词文件（10 个） |
| **关联关系** | 被 `server_side/` TypeScript 智能体代码读取并注入到 LLM 上下文中 |
| **架构角色** | AI 角色行为定义层，决定了月华的性格、对话风格与多模态行为 |

#### `hierarchy/image/generate/`

| 维度 | 说明 |
|------|------|
| **主要职责** | 图像生成的 Go 端逻辑与类型定义，封装 stable-diffusion.cpp 的调用流程 |
| **核心模块类型** | Go 生成逻辑、类型定义 |
| **关联关系** | 被 `server/handlers/generate.go` 调用；底层调用 `subsystem/sd_lunar` 的 C++ 推理引擎 |
| **架构角色** | 图像生成能力的 Go 端封装层 |

### `model/`

| 维度 | 说明 |
|------|------|
| **主要职责** | 模型服务层，管理 llama.cpp 推理代理、TTS 语音合成引擎、模型列表维护与请求队列控制 |
| **核心模块类型** | Go 模型管理逻辑、进程生命周期管理、音频缓存 |
| **关联关系** | `llama/` 启动并监控 llama-server 进程，为 `server/handlers/proxy.go` 提供代理目标；`tts/` 封装 TTS 引擎调用，为 WebSocket 推送音频数据 |
| **架构角色** | AI 推理调度层，是所有模型推理请求的统一入口 |

#### `model/llama/`

| 维度 | 说明 |
|------|------|
| **主要职责** | llama.cpp 代理，管理 llama-server 进程的启动、就绪检测、请求转发与云服务回退 |
| **核心模块类型** | Go 进程管理、HTTP 代理转发 |
| **关联关系** | 读取 `local_data/models/` 中的 GGUF 模型文件；为 `server/handlers/proxy.go` 提供代理目标 |
| **架构角色** | 文本推理的进程管理层，桥接 Go 服务与 llama.cpp 推理引擎 |

#### `model/tts/`

| 维度 | 说明 |
|------|------|
| **主要职责** | TTS 语音合成引擎的 Go 端封装，处理文本分句、引擎调用、WAV 音频缓存 |
| **核心模块类型** | Go TTS 请求处理、音频缓存管理 |
| **关联关系** | 底层通过 CGO 调用 `subsystem/qwen3_tts_lunar` 的 C++ 推理引擎；合成结果通过 `websocket/` 推送到前端 |
| **架构角色** | 语音合成能力的 Go 端封装与缓存层 |

### `server/`

| 维度 | 说明 |
|------|------|
| **主要职责** | HTTP 服务器层，负责服务启动、CORS 中间件、路由注册、端口自动递增重试 |
| **核心模块类型** | Go HTTP 服务器、路由管理、初始化编排 |
| **关联关系** | `manage.go` 编排整个初始化流程（flag 解析→路由注册→llama 启动→TTS 初始化→WebSocket→JS 智能体）；`handlers/` 处理具体的 HTTP 请求 |
| **架构角色** | 服务入口与请求分发层，是所有外部请求的第一个 Go 层接触点 |

#### `server/handlers/`

| 维度 | 说明 |
|------|------|
| **主要职责** | HTTP 请求处理器集合，处理消息收发、图像生成代理、llama.cpp 代理转发、视频关键帧提取 |
| **核心模块类型** | Go HTTP handler 函数 |
| **关联关系** | `proxy.go` 将请求转发至 `model/llama/`；`generate.go` 调用 `hierarchy/image/generate/`；`message.go` 与 `adapters/` 交互 |
| **架构角色** | HTTP 请求的业务处理层 |

### `server_side/`

| 维度 | 说明 |
|------|------|
| **主要职责** | TypeScript 智能体源码，编译后生成 `hierarchy/assets/agentSystem.js`，在 goja 运行时中执行，实现 AI 角色管理、对话流控制、工具调用 |
| **核心模块类型** | TypeScript 源码（config/control/file/math/model 五大域） |
| **关联关系** | 编译产物由 `adapters/` 加载到 goja 运行时；通过 `adapters/` 暴露的桥接函数调用 Go 后端能力；读取 `hierarchy/assets/prompts/` 的提示词模板 |
| **架构角色** | AI 智能体逻辑层，定义了月华的对话策略、角色切换、工具调用等核心智能行为 |

#### `server_side/config/`

| 维度 | 说明 |
|------|------|
| **主要职责** | 智能体全局配置，包括模型参数、数据库路径、历史记录策略、图像生成参数、工具定义 |
| **核心模块类型** | TypeScript 配置定义 |
| **关联关系** | 被 `server_side/` 其他模块引用 |
| **架构角色** | 智能体配置中心 |

#### `server_side/control/`

| 维度 | 说明 |
|------|------|
| **主要职责** | 对话流控制，包括响应延迟、消息长度限制、对话计划编排 |
| **核心模块类型** | TypeScript 控制逻辑 |
| **关联关系** | 被 `server_side/model/` 中的对话模块调用 |
| **架构角色** | 对话节奏与质量控制层 |

#### `server_side/file/`

| 维度 | 说明 |
|------|------|
| **主要职责** | 文件处理工具集，包括数据库操作封装、文本编码、文件读取、文本分割 |
| **核心模块类型** | TypeScript 文件处理工具 |
| **关联关系** | 通过 `adapters/` 暴露的 `database()`、`readFile()` 等桥接函数操作本地资源 |
| **架构角色** | 智能体的文件操作能力层 |

#### `server_side/math/`

| 维度 | 说明 |
|------|------|
| **主要职责** | 数学工具集，提供向量运算与基向量计算，支撑向量数据库的嵌入检索 |
| **核心模块类型** | TypeScript 数学工具 |
| **关联关系** | 被 `server_side/model/` 中的检索逻辑调用 |
| **架构角色** | 智能体的数学计算能力层 |

#### `server_side/model/`

| 维度 | 说明 |
|------|------|
| **主要职责** | AI 模型交互逻辑，包括智能体定义、对话构建、角色叙述、画师角色、对话整理等 |
| **核心模块类型** | TypeScript AI 交互模块（agent/builder/define/dialogue/narrator/organize/painter） |
| **关联关系** | 通过 `adapters/` 暴露的 `syncFetch()` 调用 LLM API；读取 `hierarchy/assets/prompts/` 注入角色设定 |
| **架构角色** | AI 模型交互的核心逻辑层，定义了所有与 LLM 的交互模式 |

### `websocket/`

| 维度 | 说明 |
|------|------|
| **主要职责** | WebSocket 通信层，基于 gorilla/websocket 实现实时双向通信，支持连接管理、读写泵、广播推送 |
| **核心模块类型** | Go WebSocket 服务、连接池管理、消息广播 |
| **关联关系** | 被 `server/manage.go` 注册到 `/ws` 端点；为 `hierarchy/assets/client/` 前端提供实时消息推送（上下文、图片、TTS 音频、情绪更新） |
| **架构角色** | 实时通信层，是流式 AI 响应、TTS 音频推送、情绪更新的传输通道 |

### `release/`

| 维度 | 说明 |
|------|------|
| **主要职责** | 进程与端口管理，包括命令执行、进程终止、网络状态监控、进程列表查询 |
| **核心模块类型** | Go 进程管理、网络监控 |
| **关联关系** | 被 `server/` 和 `model/llama/` 调用进行进程生命周期管理 |
| **架构角色** | 运行时进程管理层，保障外部引擎（llama-server 等）的可靠运行 |

---

## 扩展系统：crystal_astral

> 📖 详细文档：[crystal_astral/README.md](crystal_astral/README.md)

### `assets/`

| 维度 | 说明 |
|------|------|
| **主要职责** | 琉璃系统的前端静态资源，包含主页面 HTML、应用逻辑 JS、样式表 CSS |
| **核心模块类型** | HTML/CSS/JS 前端文件 |
| **关联关系** | 通过 Go `embed.FS` 嵌入编译产物；前端通过 Fetch API 与 `crystal_astral` 后端交互；引用 `local_data/package/` 下的共享资源 |
| **架构角色** | 琉璃系统的用户交互层，提供文件管理、数据库操作、截图标注等工具界面 |

---

## 子系统模块：subsystem

### 公共基础设施

#### `config/`

| 维度 | 说明 |
|------|------|
| **主要职责** | 全局配置中枢，实现命令行参数（flag 包）+ JSON 配置文件双层配置架构，覆盖端口、路径、模型、引擎、图像参数等 |
| **核心模块类型** | Go 配置管理库 |
| **关联关系** | 被所有 Go 模块（`lunar_astral`、`crystal_astral`、各子系统）以 Go module 方式引用 |
| **架构角色** | 平台配置基础设施，是所有模块获取运行参数的唯一来源。详见 [subsystem/config/README.md](subsystem/config/README.md) |

#### `browser/`

| 维度 | 说明 |
|------|------|
| **主要职责** | WebView 窗口管理与本地 IP 自动发现，支持桌面嵌入式浏览器窗口创建与局域网访问 |
| **核心模块类型** | Go WebView 窗口管理、IP 发现 |
| **关联关系** | 被 `lunar_astral` 和 `crystal_astral` 在启动时调用打开 WebView 窗口 |
| **架构角色** | 桌面窗口基础设施，将 Web UI 嵌入原生桌面体验。详见 [subsystem/browser/README.md](subsystem/browser/README.md) |

#### `storage/`

| 维度 | 说明 |
|------|------|
| **主要职责** | 文件管理（CRUD、上传、下载、归档）与 SQLite 数据库操作，提供完整的 HTTP API |
| **核心模块类型** | Go 文件操作库、SQLite 数据库操作库、HTTP 服务 |
| **关联关系** | 被 `lunar_astral` 和 `crystal_astral` 直接复用 HTTP handler；`module/` 层提供业务逻辑，`server/` 层提供 HTTP 接口 |
| **架构角色** | 数据持久化基础设施，是所有文件与数据库操作的统一后端。详见 [subsystem/storage/README.md](subsystem/storage/README.md) |

##### `storage/module/`

| 维度 | 说明 |
|------|------|
| **主要职责** | 核心业务逻辑层，实现文件读写删除、目录列表、ZIP 归档、SQLite 数据库批量操作 |
| **核心模块类型** | Go 业务逻辑函数 |
| **关联关系** | 被 `storage/server/` 的 HTTP handler 调用 |
| **架构角色** | 存储子系统的逻辑核心 |

##### `storage/server/`

| 维度 | 说明 |
|------|------|
| **主要职责** | HTTP 服务层，将 `module/` 的业务逻辑暴露为 RESTful API |
| **核心模块类型** | Go HTTP handler |
| **关联关系** | 调用 `storage/module/` 的业务函数 |
| **架构角色** | 存储子系统的 API 暴露层 |

#### `screenshot/`

| 维度 | 说明 |
|------|------|
| **主要职责** | 屏幕截图模块，支持多显示器截图、区域截图、图片缩放 |
| **核心模块类型** | Go 截图库、图片处理 |
| **关联关系** | 被 `lunar_astral` 和 `crystal_astral` 复用 HTTP handler |
| **架构角色** | 截图基础设施。详见 [subsystem/screenshot/README.md](subsystem/screenshot/README.md) |

#### `logger/`

| 维度 | 说明 |
|------|------|
| **主要职责** | 彩色终端日志输出库，提供分级、带颜色的控制台日志 |
| **核心模块类型** | Go 日志库 |
| **关联关系** | 被所有 Go 模块引用 |
| **架构角色** | 日志基础设施，统一全平台的日志输出格式 |

### 扩展功能子系统

#### `LunarTick/`

| 维度 | 说明 |
|------|------|
| **主要职责** | tick 驱动的响应式程序执行引擎，支持定时调度、变量管理、指针操作与指令执行 |
| **核心模块类型** | Go 调度引擎、HTTP API、CLI 入口 |
| **关联关系** | 可独立运行或被其他模块调用 |
| **架构角色** | 通用任务调度引擎。详见 [subsystem/LunarTick/README.md](subsystem/LunarTick/README.md) |

##### `LunarTick/api/`

| 维度 | 说明 |
|------|------|
| **主要职责** | HTTP API 服务层，暴露引擎的调度与查询接口 |
| **核心模块类型** | Go HTTP 服务 |
| **关联关系** | 调用 `LunarTick/engine/` 的核心功能 |
| **架构角色** | LunarTick 的外部接口层 |

##### `LunarTick/cmd/lunartick/`

| 维度 | 说明 |
|------|------|
| **主要职责** | CLI 入口，提供命令行启动方式 |
| **核心模块类型** | Go main 包 |
| **关联关系** | 引用 `LunarTick/engine/` 和 `LunarTick/api/` |
| **架构角色** | LunarTick 的独立可执行文件入口 |

##### `LunarTick/engine/`

| 维度 | 说明 |
|------|------|
| **主要职责** | 核心引擎，实现 tick 调度器、变量系统、指针操作与指令执行 |
| **核心模块类型** | Go 调度引擎核心 |
| **关联关系** | 被 `api/` 和 `cmd/` 调用 |
| **架构角色** | LunarTick 的逻辑核心 |

#### `bridge_adapter/`

| 维度 | 说明 |
|------|------|
| **主要职责** | QQ 群聊适配器，桥接 NapCat QQ 协议与月华核心系统，实现 QQ 群内 AI 对话 |
| **核心模块类型** | Go QQ 协议适配、消息转换 |
| **关联关系** | 通过 HTTP 调用 `lunar_astral` 的 API 发送/接收消息 |
| **架构角色** | 外部平台接入层，扩展月华的对话渠道 |

##### `bridge_adapter/pkg/`

| 维度 | 说明 |
|------|------|
| **主要职责** | 核心包集合，包含配置管理、日志、月华通信、消息格式转换、NapCat 协议、类型定义 |
| **核心模块类型** | Go 库包 |
| **关联关系** | 被 `bridge_adapter/` 主程序引用 |
| **架构角色** | 桥接适配器的逻辑核心 |

##### `bridge_adapter/template/`

| 维度 | 说明 |
|------|------|
| **主要职责** | 消息模板，定义 QQ 消息与月华消息之间的格式转换规则 |
| **核心模块类型** | 消息模板定义 |
| **关联关系** | 被 `bridge_adapter/pkg/` 的消息模块引用 |
| **架构角色** | 消息格式转换层 |

#### `gguf_metadata_viewer/`

| 维度 | 说明 |
|------|------|
| **主要职责** | GGUF 模型文件元数据查看工具，解析 GGUF 二进制格式并以 Web UI 展示模型信息 |
| **核心模块类型** | Go GGUF 解析器、HTTP 服务、前端 UI |
| **关联关系** | 读取 `local_data/models/` 中的 GGUF 文件 |
| **架构角色** | 模型管理辅助工具。详见 [subsystem/gguf_metadata_viewer/README.md](subsystem/gguf_metadata_viewer/README.md) |

##### `gguf_metadata_viewer/gguf/`

| 维度 | 说明 |
|------|------|
| **主要职责** | GGUF 二进制格式解析器，读取模型元数据（架构、参数量、量化方式等） |
| **核心模块类型** | Go 二进制解析库 |
| **关联关系** | 被 `gguf_metadata_viewer/server/` 调用 |
| **架构角色** | GGUF 格式的解析核心 |

##### `gguf_metadata_viewer/server/`

| 维度 | 说明 |
|------|------|
| **主要职责** | HTTP 服务层与前端 UI，将解析结果以 Web 界面展示 |
| **核心模块类型** | Go HTTP 服务、前端静态文件 |
| **关联关系** | 调用 `gguf/` 解析库；`static/` 提供前端界面 |
| **架构角色** | GGUF 查看器的服务与展示层 |

#### `proxy/`

| 维度 | 说明 |
|------|------|
| **主要职责** | HTTPS 代理服务器，支持 TLS 证书管理与请求代理转发 |
| **核心模块类型** | Go 代理服务、TLS 证书管理、CLI 入口、前端管理 UI |
| **关联关系** | 可独立运行，为其他模块提供网络代理能力 |
| **架构角色** | 网络代理基础设施 |

##### `proxy/cmd/`

| 维度 | 说明 |
|------|------|
| **主要职责** | CLI 入口，提供命令行启动代理服务 |
| **核心模块类型** | Go main 包 |
| **关联关系** | 引用 `proxy/` 核心逻辑 |
| **架构角色** | 代理服务的独立启动入口 |

##### `proxy/frontend/proxy_ui/`

| 维度 | 说明 |
|------|------|
| **主要职责** | 代理管理前端 UI，提供代理配置与状态监控界面 |
| **核心模块类型** | HTML/CSS/JS 前端 |
| **关联关系** | 通过 API 与 `proxy/` 后端交互 |
| **架构角色** | 代理服务的用户交互层 |

#### `volume_archive/`

| 维度 | 说明 |
|------|------|
| **主要职责** | 卷归档管理工具，支持配置检查、归档创建、执行与清理 |
| **核心模块类型** | Go 归档管理 |
| **关联关系** | 可独立运行 |
| **架构角色** | 数据归档辅助工具 |

##### `volume_archive/component/`

| 维度 | 说明 |
|------|------|
| **主要职责** | 核心组件集合，包含配置、检查、创建、执行、清理等归档流程组件 |
| **核心模块类型** | Go 业务组件 |
| **关联关系** | 被 `volume_archive/` 主程序编排调用 |
| **架构角色** | 卷归档的逻辑核心 |

### 独立 AI 引擎

#### `qwen3_tts_lunar/`

| 维度 | 说明 |
|------|------|
| **主要职责** | 独立语音合成系统，基于 Qwen3-TTS 模型实现文本转语音，包含 C++ GGML 推理引擎与 Go 服务封装 |
| **核心模块类型** | C++ GGML 推理引擎、Go HTTP 服务、前端 UI |
| **关联关系** | `lunar_astral/model/tts/` 通过 CGO 调用其 C++ 引擎；也可独立编译为 `Qwen3_TTS_Lunar.exe` |
| **架构角色** | 语音合成能力引擎。详见 [subsystem/qwen3_tts_lunar/README.md](subsystem/qwen3_tts_lunar/README.md) |

##### `qwen3_tts_lunar/client/`

| 维度 | 说明 |
|------|------|
| **主要职责** | TTS 独立运行时的前端 UI，提供文本输入与音频播放界面 |
| **核心模块类型** | HTML/CSS/JS 前端 |
| **关联关系** | 通过 HTTP API 与 Go 服务层交互 |
| **架构角色** | TTS 独立模式的用户交互层 |

##### `qwen3_tts_lunar/cpp/`

| 维度 | 说明 |
|------|------|
| **主要职责** | C++ 推理引擎，实现 Qwen3-TTS 模型的完整推理管线（文本分词→Transformer→音频分词→WAV 输出） |
| **核心模块类型** | C++ 推理引擎源码、C API 接口、CMake 构建配置 |
| **关联关系** | `ggml/` 提供张量计算基础；`src/` 实现业务逻辑；通过 C API 被 Go 的 CGO 调用 |
| **架构角色** | TTS 推理的计算核心 |

##### `qwen3_tts_lunar/cpp/ggml/`

| 维度 | 说明 |
|------|------|
| **主要职责** | GGML 张量计算库，提供 CPU/GPU 加速的矩阵运算基础 |
| **核心模块类型** | C/C++ 张量计算库 |
| **关联关系** | 被 `cpp/src/` 的推理代码调用 |
| **架构角色** | 底层张量计算基础设施 |

##### `qwen3_tts_lunar/cpp/src/`

| 维度 | 说明 |
|------|------|
| **主要职责** | TTS 引擎源码，包含主引擎、Transformer 层、音频分词器、文本分词器、GGUF 加载器等 |
| **核心模块类型** | C++ 推理源码 |
| **关联关系** | 调用 `cpp/ggml/` 进行张量计算 |
| **架构角色** | TTS 推理的业务逻辑核心 |

##### `qwen3_tts_lunar/module/`

| 维度 | 说明 |
|------|------|
| **主要职责** | Go 逻辑层，封装语音生成、流式处理与变量管理 |
| **核心模块类型** | Go 业务逻辑 |
| **关联关系** | 通过 CGO 调用 `cpp/` 的 C API；被 `qwen3_tts_lunar/server.go` 的 HTTP handler 调用 |
| **架构角色** | TTS 的 Go 端业务封装层 |

#### `qwen_asr_lunar/`

| 维度 | 说明 |
|------|------|
| **主要职责** | 独立语音识别系统，基于 Qwen3-ASR 模型实现语音转文本，采用纯 C 实现 + OpenBLAS 加速 |
| **核心模块类型** | 纯 C 推理引擎、Go HTTP 服务（CGO 桥接）、前端 UI |
| **关联关系** | 可独立编译为 `Qwen_ASR_Lunar.exe`；`lunar_astral` 可通过 HTTP API 调用其识别能力 |
| **架构角色** | 语音识别能力引擎。详见 [subsystem/qwen_asr_lunar/README.md](subsystem/qwen_asr_lunar/README.md) |

##### `qwen_asr_lunar/openblas/`

| 维度 | 说明 |
|------|------|
| **主要职责** | OpenBLAS 线性代数库，为 ASR 推理提供优化的矩阵运算加速 |
| **核心模块类型** | C 头文件与预编译库 |
| **关联关系** | 被 ASR 的 C 推理源码链接 |
| **架构角色** | ASR 推理的线性代数加速基础设施 |

##### `qwen_asr_lunar/static/`

| 维度 | 说明 |
|------|------|
| **主要职责** | ASR 独立运行时的前端 UI，提供音频录制/上传与识别结果展示界面 |
| **核心模块类型** | HTML/CSS/JS 前端 |
| **关联关系** | 通过 HTTP API 与 Go 服务层交互 |
| **架构角色** | ASR 独立模式的用户交互层 |

#### `sd_lunar/`

| 维度 | 说明 |
|------|------|
| **主要职责** | Stable Diffusion 图像生成引擎，基于 C++ GGML 实现 SD 模型的本地推理 |
| **核心模块类型** | C++ GGML 推理引擎、前端 UI |
| **关联关系** | 被 `lunar_astral` 的图像生成 handler 调用；也可独立运行 |
| **架构角色** | 图像生成能力引擎 |

##### `sd_lunar/assets/`

| 维度 | 说明 |
|------|------|
| **主要职责** | SD 图像生成的前端 UI，提供参数配置与图片预览界面 |
| **核心模块类型** | HTML/CSS/JS 前端 |
| **关联关系** | 通过 HTTP API 与后端交互 |
| **架构角色** | SD 图像生成的用户交互层 |

##### `sd_lunar/cpp/`

| 维度 | 说明 |
|------|------|
| **主要职责** | C++ GGML 推理引擎，实现 Stable Diffusion 模型的完整推理管线 |
| **核心模块类型** | C++ 推理引擎源码 |
| **关联关系** | `ggml/` 提供张量计算基础 |
| **架构角色** | SD 推理的计算核心 |

##### `sd_lunar/cpp/ggml/`

| 维度 | 说明 |
|------|------|
| **主要职责** | GGML 张量计算库，为 SD 推理提供 CPU/GPU 加速的矩阵运算 |
| **核心模块类型** | C/C++ 张量计算库 |
| **关联关系** | 被 `sd_lunar/cpp/` 的推理代码调用 |
| **架构角色** | SD 推理的底层张量计算基础设施 |

---

## 数据与资源：local_data

### `local_data/models/`

| 维度 | 说明 |
|------|------|
| **主要职责** | 存放所有 AI 模型文件（GGUF/SafeTensors 格式），包括文本推理模型、ASR 模型、TTS 模型、视觉模型等 |
| **核心模块类型** | GGUF/SafeTensors 模型文件、参数文档、Live2D 模型配置 |
| **关联关系** | `lunar_astral/model/llama/` 读取此目录的 GGUF 文件启动推理；`qwen3_tts_lunar` 和 `qwen_asr_lunar` 读取各自的模型文件 |
| **架构角色** | AI 模型文件的物理存储层，是所有推理引擎的数据来源 |

#### `local_data/models/Qwen3-ASR-0.6B/`

| 维度 | 说明 |
|------|------|
| **主要职责** | Qwen3-ASR 语音识别模型文件存储 |
| **核心模块类型** | SafeTensors 模型文件 |
| **关联关系** | 被 `subsystem/qwen_asr_lunar/` 的推理引擎加载 |
| **架构角色** | ASR 推理的模型数据 |

#### `local_data/models/document/`

| 维度 | 说明 |
|------|------|
| **主要职责** | 模型参数文档，记录各模型的配置参数与使用说明 |
| **核心模块类型** | 文档文件 |
| **关联关系** | 供开发者和用户参考 |
| **架构角色** | 模型配置参考 |

#### `local_data/models/live2d/`

| 维度 | 说明 |
|------|------|
| **主要职责** | Live2D 模型配置文件，定义角色模型的动作、表情与渲染参数 |
| **核心模块类型** | Live2D 模型配置 |
| **关联关系** | 被 `hierarchy/assets/client/live2d.js` 加载渲染 |
| **架构角色** | Live2D 角色的模型数据 |

#### `local_data/models/stable_diffusion.cpp/`

| 维度 | 说明 |
|------|------|
| **主要职责** | Stable Diffusion 参数文档，记录 SD 模型的配置与使用参数 |
| **核心模块类型** | 文档文件 |
| **关联关系** | 供 `subsystem/sd_lunar/` 参考 |
| **架构角色** | SD 模型配置参考 |

### `local_data/package/`

| 维度 | 说明 |
|------|------|
| **主要职责** | 前端共享资源库，存放所有前端 UI 模块与第三方 JS/CSS 库，通过 HTTP 静态文件服务供各系统前端引用 |
| **核心模块类型** | HTML/CSS/JS 前端模块、第三方库（ECharts、marked、mermaid、Live2D、PixiJS、QRCode 等） |
| **关联关系** | 被 `lunar_astral/hierarchy/assets/client/` 和 `crystal_astral/assets/` 通过 `/file/read/package/` 路径引用 |
| **架构角色** | 前端资源共享中心，避免各系统重复打包相同的前端依赖 |

#### `local_data/package/standard_dependency/`

| 维度 | 说明 |
|------|------|
| **主要职责** | 全局标准依赖，提供统一的 CSS 变量定义、玻璃拟态样式、主题切换、通用 JS 工具函数 |
| **核心模块类型** | CSS 标准样式、JS 标准脚本 |
| **关联关系** | 被所有前端 UI 模块引用为全局基础样式与脚本 |
| **架构角色** | 前端设计系统的核心，定义了全平台的视觉规范（玻璃拟态、配色、字体等） |

#### `local_data/package/different_lunar/`

| 维度 | 说明 |
|------|------|
| **主要职责** | 月华前端主界面组件库，包含 Live2D 渲染、Markdown/Mermaid 渲染、提示词管理、包管理等核心前端组件 |
| **核心模块类型** | HTML/CSS/JS 前端组件 |
| **关联关系** | 被 `lunar_astral/hierarchy/assets/client/` 引用 |
| **架构角色** | 月华系统的前端组件库 |

#### `local_data/package/archive/`

| 维度 | 说明 |
|------|------|
| **主要职责** | 归档与许可文件，存放第三方库的许可证与归档资料 |
| **核心模块类型** | 许可证文件、归档资料 |
| **关联关系** | 无运行时依赖 |
| **架构角色** | 法律合规与历史归档 |

#### 各功能 UI 模块

以下模块均为独立的前端功能界面，通过 `standard_dependency/` 获取基础样式，通过 HTTP API 与后端交互：

| 目录 | 功能 | 关联后端 |
|------|------|---------|
| `database_manager/` | 数据库可视化管理 | `subsystem/storage/` |
| `file_explorer/` | 文件浏览器 | `subsystem/storage/` |
| `image_generation/` | 图像生成参数配置与预览 | `lunar_astral/server/handlers/generate.go` |
| `message_rendering/` | 消息渲染（Markdown/Mermaid/ECharts/KaTeX） | `lunar_astral/websocket/` |
| `model_query/` | 模型查询与参数查看 | `lunar_astral/model/` |
| `multimedia_preview/` | 多媒体预览（图片/视频/音频） | `lunar_astral/server/handlers/video.go` |
| `parameter_assistant/` | 参数助手 | `subsystem/config/` |
| `qwen3_tts/` | TTS 语音合成界面 | `subsystem/qwen3_tts_lunar/` |
| `screenshot_manager/` | 截图管理界面 | `subsystem/screenshot/` |
| `vector_db_manager/` | 向量数据库管理界面 | `subsystem/storage/` |

---

## 跨模块依赖关系

### Go Module 依赖图

```
lunar_astral
  ├── subsystem/config          (配置管理)
  ├── subsystem/browser         (WebView 窗口)
  ├── subsystem/storage         (文件 + 数据库)
  ├── subsystem/screenshot      (屏幕截图)
  └── subsystem/qwen3_tts_lunar (TTS 引擎，CGO 调用)

crystal_astral
  ├── subsystem/config          (配置管理)
  ├── subsystem/browser         (WebView 窗口)
  ├── subsystem/storage         (文件 + 数据库)
  └── subsystem/screenshot      (屏幕截图)

subsystem/bridge_adapter
  └── subsystem/config          (配置管理)

subsystem/LunarTick
  └── subsystem/logger          (日志)

subsystem/gguf_metadata_viewer
  └── subsystem/logger          (日志)

subsystem/volume_archive
  └── subsystem/logger          (日志)

subsystem/proxy
  └── subsystem/logger          (日志)
```

### 数据流依赖图

```
用户输入
  │
  ├──→ lunar_astral/server/handlers/     (HTTP 请求入口)
  │       │
  │       ├──→ model/llama/              (文本推理代理)
  │       │       └──→ llama-server.exe   (GGUF 推理)
  │       │               └──→ local_data/models/  (模型文件)
  │       │
  │       ├──→ adapters/                  (Go↔JS 桥接)
  │       │       └──→ agentSystem.js     (TS 智能体)
  │       │               ├──→ server_side/model/   (AI 交互逻辑)
  │       │               └──→ hierarchy/assets/prompts/ (角色设定)
  │       │
  │       ├──→ model/tts/                  (语音合成)
  │       │       └──→ qwen3_tts_lunar/cpp/ (C++ GGML 引擎)
  │       │
  │       └──→ websocket/                  (实时推送)
  │               └──→ hierarchy/assets/client/ (前端渲染)
  │
  ├──→ crystal_astral/                    (工具集)
  │       ├──→ subsystem/storage/         (文件 + 数据库)
  │       ├──→ subsystem/screenshot/      (截图)
  │       └──→ lunar_astral/model/llama/  (AI 代理转发)
  │
  └──→ 独立子系统
          ├──→ qwen_asr_lunar/            (语音识别)
          ├──→ qwen3_tts_lunar/           (语音合成)
          └──→ sd_lunar/                  (图像生成)
```

### 前端资源引用关系

```
所有前端 UI
  ├──→ local_data/package/standard_dependency/  (全局样式 + 脚本)
  ├──→ local_data/package/fontAwesome/          (图标库)
  ├──→ local_data/package/highlight/           (代码高亮)
  ├──→ local_data/package/katex/               (数学公式)
  └──→ local_data/package/*.min.js             (ECharts/marked/mermaid/Live2D/PixiJS/QRCode)
```
