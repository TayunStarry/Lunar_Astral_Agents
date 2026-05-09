# 技术架构

本文档介绍 Lunar Astral Agents 项目的技术架构和系统设计。

## 系统架构图

```
┌─────────────────────────────────────────────────────────────────┐
│                    Lunar Astral Agents                          │
├─────────────────────────────────────────────────────────────────┤
│  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐ │
│  │   Live2D 前端    │  │   HTTP Server   │  │  WebSocket      │ │
│  │   (TypeScript)   │  │    (Go)         │  │   (Go)          │ │
│  └────────┬────────┘  └────────┬────────┘  └────────┬────────┘ │
│           │                   │                    │           │
│  ┌────────┴───────────────────┴────────────────────┴────────┐  │
│  │                    LunarCore Server                      │  │
│  │  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐    │  │
│  │  │ Handlers │ │  Model   │ │  Image   │ │   TTS    │    │  │
│  │  └──────────┘ └──────────┘ └──────────┘ └──────────┘    │  │
│  └──────────────────────────────────────────────────────────┘  │
│                              │                                   │
│  ┌───────────────────────────┴───────────────────────────────┐ │
│  │                      子系统模块                             │ │
│  │  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐     │ │
│  │  │  Config  │ │ Storage  │ │ Browser  │ │ Screenshot│    │ │
│  │  └──────────┘ └──────────┘ └──────────┘ └──────────┘     │ │
│  │  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐     │ │
│  │  │  Bridge   │ │  Proxy   │ │  Web-P   │ │  Archive  │    │ │
│  │  └──────────┘ └──────────┘ └──────────┘ └──────────┘     │ │
│  └───────────────────────────────────────────────────────────┘ │
│                              │                                   │
│  ┌───────────────────────────┴───────────────────────────────┐ │
│  │                    外部服务集成                             │ │
│  │  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐      │ │
│  │  │llama.cpp │ │  Stable  │ │  Qwen3   │ │  NapCat  │      │ │
│  │  │ (GGUF)   │ │ Diffusion│ │   TTS    │ │  (QQ)    │      │ │
│  │  └──────────┘ └──────────┘ └──────────┘ └──────────┘      │ │
│  └───────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────┘
```

## 核心模块说明

### LunarCore（主服务器）

LunarCore 是整个系统的核心服务器，负责处理主要的业务逻辑。

| 模块 | 路径 | 功能描述 |
|------|------|----------|
| [server](../LunarCore/server) | `LunarCore/server/` | HTTP 服务器核心，负责路由分发和请求处理 |
| [handlers](../LunarCore/server/handlers) | `LunarCore/server/handlers/` | HTTP 请求处理器，包括 Agent、Message、Proxy、TTS 等 |
| [model](../LunarCore/model) | `LunarCore/model/` | GGUF 模型管理，支持 llama.cpp 多模型部署 |
| [adapters](../LunarCore/adapters) | `LunarCore/adapters/` | JavaScript 运行时适配器，用于 TypeScript 脚本执行 |
| [control](../LunarCore/control) | `LunarCore/control/` | 计划任务调度系统，支持延迟执行和速率限制 |
| [hierarchy](../LunarCore/hierarchy) | `LunarCore/hierarchy/` | 前端资源、内嵌资源管理 |
| [release](../LunarCore/release) | `LunarCore/release/` | 系统进程管理和端口释放 |

### 子系统模块

系统包含多个独立的子系统模块，每个模块负责特定的功能。

| 子系统 | 路径 | 功能描述 |
|--------|------|----------|
| config | `subsystem/config/` | 集中配置管理，支持 JSON 配置加载和环境变量 |
| storage | `subsystem/storage/` | 文件系统和数据库存储服务 |
| browser | `subsystem/browser/` | 浏览器自动化，获取本地 IP 和打开浏览器 |
| screenshot | `subsystem/screenshot/` | 截图服务模块 |
| bridge_adapter | `subsystem/bridge_adapter/` | QQ/NapCat 消息桥接适配器 |
| proxy | `subsystem/proxy/` | HTTP/HTTPS 代理服务器 |
| web-p | `subsystem/web-p/` | PNG to WebP 图片格式转换器 |
| crystal_astral | `subsystem/crystal_astral/` | 琉璃 - 轻量级功能导航服务 |
| project_archiving | `subsystem/project_archiving/` | 项目打包归档工具 |

## 目录结构

```
Lunar_Astral_Agents/
├── LunarCore/                    # 主程序目录
│   ├── main.go                   # 程序入口
│   ├── go.mod                    # Go 模块定义
│   ├── package.json              # Node.js 依赖
│   ├── server/                   # HTTP 服务器
│   │   ├── create.go             # 服务器初始化
│   │   ├── endpoint.go           # 路由端点注册
│   │   ├── manage.go             # 服务器管理
│   │   ├── websocket.go          # WebSocket 处理
│   │   ├── type.go               # 类型定义
│   │   └── handlers/             # 请求处理器
│   │       ├── agent.go          # AI 模型交互
│   │       ├── message.go        # 消息队列
│   │       ├── proxy.go          # 代理请求
│   │       ├── tts.go            # TTS 代理
│   │       └── image/            # 图像生成
│   ├── model/                    # 模型管理
│   │   ├── core.go               # 核心接口
│   │   ├── type.go               # 类型定义
│   │   └── llama/                # llama.cpp 集成
│   ├── adapters/                 # 运行时适配器
│   ├── control/                  # 任务调度
│   │   ├── index.ts              # 控制模块入口
│   │   ├── plan.ts               # 计划任务系统
│   │   ├── delay.ts              # 延迟执行
│   │   └── limit.ts              # 速率限制
│   ├── hierarchy/                # 资源管理
│   │   ├── assets/client/        # 前端资源
│   │   ├── assets/prompts/       # 提示词模板
│   │   └── image/generate/       # 图像生成
│   └── release/                  # 系统发布
│       ├── execute.go            # 端口释放
│       └── processes.go          # 进程管理
├── subsystem/                     # 子系统模块
│   ├── config/                   # 配置管理
│   ├── storage/                  # 存储服务
│   ├── browser/                  # 浏览器自动化
│   ├── screenshot/               # 截图服务
│   ├── bridge_adapter/           # QQ 桥接
│   ├── proxy/                    # 代理服务
│   ├── web-p/                    # Web 服务器
│   ├── crystal_astral/           # 琉璃 - 轻量级功能导航
│   └── project_archiving/        # 项目归档
├── local_data/                    # 本地数据
│   ├── models/                   # 模型文件
│   ├── audios/                   # 音频资源
│   ├── package/                  # 前端组件包
│   └── lunar_config.json         # 主配置文件
├── docs/                          # 文档目录
│   ├── characters/               # 人物设定
│   ├── architecture.md           # 技术架构（本文档）
│   ├── setup.md                  # 安装配置
│   ├── api/                      # API 文档
│   └── performance.md            # 性能优化
├── image/                        # 示例图片
├── build.ps1                     # 构建脚本
└── README.md                     # 项目入口文档
```

## 技术栈

### 后端技术

- **语言**: Go 1.21+
- **Web 框架**: 原生 net/http
- **WebSocket**: 原生 net/http 包
- **AI 推理**: llama.cpp (GGUF 格式)
- **图像生成**: stable-diffusion.cpp

### 前端技术

- **语言**: TypeScript / JavaScript
- **UI 框架**: 原生 HTML/CSS/JavaScript
- **Live2D**: Cubism 4 SDK + Pixi.js
- **实时通信**: WebSocket

### 外部集成

- **语音合成**: Qwen3-TTS
- **QQ 集成**: NapCat
- **数据库**: 本地文件存储

---

*文档版本：1.0 | 最后更新：2026-05-09*

[返回主页](./README.md) | [查看人物设定](./characters/index.md)
