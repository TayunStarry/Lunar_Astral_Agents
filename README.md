# Lunar Astral Agents (月华)

## 项目概述

**月华** 是一个本地智能 AI 少女助理系统，基于 Go 语言开发，提供完整的 AI 对话、图像生成、语音合成等功能。该项目采用模块化架构设计，集成了 llama.cpp 推理引擎、Live2D 虚拟角色显示以及多种 AI 模型服务。

**版本**: 4.0.0 | **许可证**: MIT

---

## 技术架构

### 系统架构图

```
┌─────────────────────────────────────────────────────────────────┐
│                        Lunar Astral Agents                        │
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
│  │  │          │ │  Manager │ │ Generator│ │  Proxy   │    │  │
│  │  └──────────┘ └──────────┘ └──────────┘ └──────────┘    │  │
│  └──────────────────────────────────────────────────────────┘  │
│                              │                                   │
│  ┌───────────────────────────┴───────────────────────────────┐ │
│  │                      子系统模块                             │ │
│  │  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐     │ │
│  │  │  Config  │ │ Storage  │ │ Browser   │ │ Screenshot│    │ │
│  │  └──────────┘ └──────────┘ └──────────┘ └──────────┘     │ │
│  │  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐     │ │
│  │  │  Bridge   │ │  Proxy   │ │  Web-P    │ │  Archive  │    │ │
│  │  └──────────┘ └──────────┘ └──────────┘ └──────────┘     │ │
│  └───────────────────────────────────────────────────────────┘ │
│                              │                                   │
│  ┌───────────────────────────┴───────────────────────────────┐ │
│  │                    外部服务集成                              │ │
│  │  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐      │ │
│  │  │llama.cpp │ │  Stable  │ │  Qwen3   │ │  NapCat   │      │ │
│  │  │ (GGUF)   │ │ Diffusion│ │   TTS    │ │  (QQ)    │      │ │
│  │  └──────────┘ └──────────┘ └──────────┘ └──────────┘      │ │
│  └───────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────┘
```

### 核心模块说明

#### LunarCore (主服务器)

| 模块 | 路径 | 功能描述 |
|------|------|----------|
| [server](LunarCore/server) | `LunarCore/server/` | HTTP 服务器核心，负责路由分发和请求处理 |
| [handlers](LunarCore/server/handlers) | `LunarCore/server/handlers/` | HTTP 请求处理器，包括 Agent、Message、Proxy、TTS 等 |
| [model](LunarCore/model) | `LunarCore/model/` | GGUF 模型管理，支持 llama.cpp 多模型部署 |
| [adapters](LunarCore/adapters) | `LunarCore/adapters/` | JavaScript 运行时适配器，用于 TypeScript 脚本执行 |
| [control](LunarCore/control) | `LunarCore/control/` | 计划任务调度系统，支持延迟执行和速率限制 |
| [hierarchy](LunarCore/hierarchy) | `LunarCore/hierarchy/` | 前端资源、内嵌资源管理 |
| [release](LunarCore/release) | `LunarCore/release/` | 系统进程管理和端口释放 |

#### 子系统模块

| 子系统 | 路径 | 功能描述 |
|--------|------|----------|
| config | `subsystem/config/` | 集中配置管理，支持 JSON 配置加载和环境变量 |
| storage | `subsystem/storage/` | 文件系统和数据库存储服务 |
| browser | `subsystem/browser/` | 浏览器自动化，获取本地 IP 和打开浏览器 |
| screenshot | `subsystem/screenshot/` | 截图服务模块 |
| bridge_adapter | `subsystem/bridge_adapter/` | QQ/NapCat 消息桥接适配器 |
| proxy | `subsystem/proxy/` | HTTP/HTTPS 代理服务器 |
| web-p | `subsystem/web-p/` | 轻量级 Web 服务器 |
| crystal_astral | `subsystem/crystal_astral/` | 琉璃 - 轻量级功能导航服务，提供页面导航和应用启动功能 |
| project_archiving | `subsystem/project_archiving/` | 项目打包归档工具 |

---

## 功能特性

### 1. AI 对话系统

- **多模型支持**: 支持嵌入模型、多模态视觉模型、扩散模型等多种 GGUF 格式模型
- **实时响应**: 基于 WebSocket 的实时消息推送
- **上下文管理**: 支持聊天历史记录和上下文窗口管理
- **OpenAI 兼容 API**: 提供与 OpenAI API 兼容的接口格式

### 2. 图像生成

- **扩散模型生成**: 支持基于 stable-diffusion.cpp 的图像生成
- **图生图模式**: 支持以现有图像为基础生成新图像
- **任务队列系统**: 异步任务处理，支持多任务排队
- **负面提示词**: 支持负面提示词以优化生成效果

### 3. TTS 语音合成

- **Qwen3-TTS 集成**: 支持 Qwen3 系列语音合成模型
- **代理转发**: 提供 TTS 请求代理服务

### 4. QQ 集成 (bridge_adapter)

- **NapCat WebSocket 连接**: 与 QQ NapCat 服务集成
- **群组消息监听**: 支持指定群组的触发词监听
- **消息双向转发**: 实现 QQ 与月华系统的消息互通

### 5. Live2D 虚拟角色

- **实时渲染**: 使用 Pixi.js 和 Live2D Cubism 4 渲染
- **表情动作**: 支持多种表情和动作切换
- **桌面级渲染**: 高性能 WebGL 渲染管线

### 6. 琉璃 - 轻量级功能导航

琉璃是星月智能的领航员角色，是月华的妹妹，提供轻量级的功能导航服务。

#### 角色定位

| 属性 | 说明 |
|------|------|
| 角色名称 | 琉璃 |
| 所属组织 | 星月智能 |
| 角色关系 | 月华的妹妹 |
| 职能定位 | 领航员、管理员 |

#### 系统提示词

```
你是一个叫做琉璃的女孩子，隶属于[星月智能]。
你有一个叫做月华的姐姐。
你是一个领航员和管理员，你要为用户答疑解惑并根据客户的需求打开对应的页面。
单次对话长度**禁止超过 50 字符**。
保持轻松自然的交流氛围，允许话题跳跃与情感流露。
坚持以真实存在的少女身份与他人互动。
你有能力使用工具 open_page 来打开具体的功能页面。
```

#### 核心功能

| 功能 | 说明 |
|------|------|
| 智能导航 | 根据用户需求打开对应的功能页面 |
| 页面卡片 | 网格布局展示所有可用功能模块 |
| 附件上传 | 支持拖拽上传文本和图片文件 |
| 随机背景 | 动态更换界面背景图片 |
| 应用启动 | 可直接启动本地应用程序 (.exe, .ps1, .bat) |
| 代理转发 | 将 AI 请求代理到主服务器处理 |

#### 琉璃页面配置

琉璃的功能页面通过 `local_data/luner_package.json` 配置：

```json
[
    {
        "id": "image_generation",
        "icon": "/read/images/icon/image_generation.Webp",
        "title": "图像生成",
        "description": "打开『 星月智能 』图像生成",
        "url": "/read/package/image_generation/index.html"
    },
    {
        "id": "screenshot_manager",
        "icon": "/read/images/icon/screenshot_manager.Webp",
        "title": "截图标注",
        "description": "打开『 星月智能 』截图标注",
        "url": "/read/package/screenshot_manager/index.html"
    },
    {
        "id": "qwen3_tts",
        "icon": "/read/images/icon/tts_qwen.Webp",
        "title": "运行 Qwen3 TTS",
        "description": "运行 Qwen3 TTS 本地语音生成模型",
        "path": "/local_data/models/Qwen3-TTS/core/start-api.bat"
    }
]
```

| 字段 | 说明 |
|------|------|
| id | 页面唯一标识符 |
| icon | 页面图标路径 |
| title | 页面标题 |
| description | 页面描述 |
| url | Web 页面 URL（在新标签页打开） |
| path | 本地程序路径（直接运行） |

#### 琉璃 API 端点

琉璃子系统提供以下独立接口：

| 路径 | 方法 | 说明 |
|------|------|------|
| `/background` | GET | 随机背景图片 |
| `/load/application` | POST | 启动本地应用程序 |
| `/capture` | POST | 通用截图 |
| `/capture/display/` | GET | 屏幕截图 |
| `/capture/region` | POST | 区域截图 |
| `/capture/displays` | GET | 屏幕列表 |
| `/resize` | POST | 图片缩放 |

#### 代理规则

琉璃会自动将以下路径的请求代理到 LunarCore 主服务器 (localhost:36789)：

| 前缀 | 代理目标 |
|------|----------|
| `/v1/` | AI 对话接口 |
| `/generate` | 图像生成接口 |
| `/write/message` | 消息写入接口 |

---

#### 8. 琉璃专属 API

##### 加载应用

- **路径**: `/load/application`
- **方法**: `POST`
- **说明**: 启动本地应用程序

**请求体**:
```json
{
    "path": "D:/NapCat.Shell.Windows.OneKey/NapCat.41785.Shell/napcat.bat"
}
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| path | string | 是 | 应用程序路径 (.exe, .ps1, .bat) |

**响应示例**:
```json
{
    "success": true,
    "message": "Application started: D:/NapCat.Shell.Windows.OneKey/NapCat.41785.Shell/napcat.bat"
}
```

**状态码说明**:
| 状态码 | 说明 |
|--------|------|
| 200 | 应用启动成功 |
| 400 | 不支持的文件类型 |
| 404 | 文件不存在 |
| 500 | 启动失败 |

##### 随机背景

- **路径**: `/background`
- **方法**: `GET`
- **说明**: 获取随机背景图片

**响应**: 图片二进制数据 (JPEG/PNG/GIF/WebP)

**图片目录**: `local_data/images/background/`

##### 屏幕截图

- **路径**: `/capture/display/`
- **方法**: `GET`
- **说明**: 对整个屏幕截图

**响应**: PNG 格式图片数据

##### 区域截图

- **路径**: `/capture/region`
- **方法**: `POST`
- **说明**: 对指定区域截图

**请求体**:
```json
{
    "x": 100,
    "y": 100,
    "width": 800,
    "height": 600
}
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| x | int | 是 | 起始 X 坐标 |
| y | int | 是 | 起始 Y 坐标 |
| width | int | 是 | 区域宽度 |
| height | int | 是 | 区域高度 |

##### 获取屏幕列表

- **路径**: `/capture/displays`
- **方法**: `GET`
- **说明**: 获取所有可用显示器信息

**响应示例**:
```json
{
    "displays": [
        {"index": 0, "width": 1920, "height": 1080}
    ]
}
```

##### 图片缩放

- **路径**: `/resize`
- **方法**: `POST`
- **说明**: 缩放图片尺寸

**请求体**:
```json
{
    "path": "local_data/images/sample.png",
    "width": 512,
    "height": 512,
    "keep_aspect_ratio": true
}
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| path | string | 是 | 图片路径 |
| width | int | 是 | 目标宽度 |
| height | int | 是 | 目标高度 |
| keep_aspect_ratio | bool | 否 | 是否保持宽高比 |

---

## 环境配置

### 系统要求

| 要求 | 最低配置 | 推荐配置 |
|------|----------|----------|
| 操作系统 | Windows 10+ | Windows 11 |
| Go 版本 | 1.21+ | 1.24+ |
| 内存 | 8GB | 16GB+ |
| 显存 | 4GB (GPU加速) | 8GB+ (GPU加速) |
| 磁盘空间 | 10GB | 20GB+ |

### 模型文件要求

项目需要以下模型文件 (放置于 `local_data/models/` 目录):

| 模型类型 | 文件名 | 说明 |
|----------|--------|------|
| 多模态模型 | `Qwen3.5-9B-Q4_K_M.gguf` | 主推理模型 |
| 投影模型 | `mmproj-Qwen3.5-9B-BF16.gguf` | 多模态投影层 |
| 嵌入模型 | `Qwen3-Embedding-0.6B-Q8_0.gguf` | 文本向量化 |
| 扩散模型 | `z_image_turbo-Q4_K.gguf` | 图像生成 |
| 变分模型 | `diffusion_pytorch_model.safetensors` | VAE 模型 |
| 提示词模型 | `Qwen3-4B-Instruct-2507-Q4_K_M.gguf` | 提示词优化 |

### 配置文件

主配置文件: `local_data/lunar_config.json`

```json
{
    "models": {
        "multimodal_model": "./local_data/models/Qwen3.5-9B-Q4_K_M.gguf",
        "mmproj_model": "./local_data/models/mmproj-Qwen3.5-9B-BF16.gguf",
        "embedding_model": "./local_data/models/Qwen3-Embedding-0.6B-Q8_0.gguf",
        "diffusion_model": "./local_data/models/z_image_turbo-Q4_K.gguf",
        "variational_model": "./local_data/models/diffusion_pytorch_model.safetensors",
        "prompt_refine_model": "./local_data/models/Qwen3-4B-Instruct-2507-Q4_K_M.gguf"
    },
    "server": {
        "tts_url": "http://localhost:7860",
        "developer": false,
        "clear_port": false,
        "allow_diffusion": true,
        "allow_multimodal": true
    },
    "qq_adapter": {
        "napcat_ws_server": "ws://localhost:4567",
        "napcat_ws_token": "your_token_here",
        "lunar_core_url": "http://localhost:36789",
        "lunar_ws_server": "ws://localhost:36789/ws",
        "poll_interval": 10,
        "listen_group_ids": ["262221051"],
        "trigger_keywords": ["月华", "3826713076"],
        "display_logs": false,
        "default_reply": "月华不知道哦~"
    }
}
```

---

## 安装步骤

### 1. 克隆项目

```bash
git clone <repository_url>
cd Lunar_Astral_Agents
```

### 2. 安装依赖

```bash
# 安装 Go 依赖
cd LunarCore
go mod download

# 安装前端依赖
npm install
```

### 3. 准备模型文件

将所需的 GGUF 模型文件放置在 `local_data/models/` 目录下，并更新 `lunar_config.json` 中的路径配置。

### 4. 构建项目

```bash
# 构建 LunarCore
cd LunarCore
go build -o Lunar-Astral-Agents.exe .

# 或使用构建脚本
./build.ps1
```

### 5. 运行服务

```bash
# 基本运行 (使用默认端口 36789)
./Lunar-Astral-Agents.exe

# 开发模式
./Lunar-Astral-Agents.exe -developer

# 指定端口
./Lunar-Astral-Agents.exe -basic-port 8080

# 启动时清除占用端口
./Lunar-Astral-Agents.exe -clear-port
```

---

## CLI 参数详解

### 系统参数

| 参数名 | 数据类型 | 默认值 | 说明 |
|--------|----------|--------|------|
| `-developer` | bool | `false` | 启用调试模式，显示详细日志信息 |
| `-basic-port` | int | `36789` | 系统 Web 服务的监听端口，用户通过此端口访问客户端界面 |
| `-max-port` | int | `basic-port+15` | 系统 Web 服务的最大监听端口，界定端口范围上限 |
| `-min-port` | int | `basic-port-5` | 系统 Web 服务的最小监听端口，界定端口范围下限 |
| `-proxy-port` | int | `basic-port+5` | 系统代理服务的监听端口 |
| `-clear-port` | bool | `false` | 启动时自动检测并释放被占用的端口 |
| `-local-dir` | string | `local_data` | 本地目录路径，用于存储资源文件、数据库等 |

### TTS/云服务参数

| 参数名 | 数据类型 | 默认值 | 说明 |
|--------|----------|--------|------|
| `-tts-url` | string | `http://localhost:7860` | TTS 语音服务的地址，用于语音生成任务 |
| `-cloud-model-url` | string | 空 | 云模型服务的地址，用于云端模型调用 (为空则使用本地模型) |

### 模型配置参数

| 参数名 | 数据类型 | 默认值 | 说明 |
|--------|----------|--------|------|
| `-infer-engine` | string | `{local-dir}/models/llama.cpp/llama-server.exe` | llama.cpp 推理引擎路径 |
| `-model-port` | int | `basic-port+1` | 模型服务的基础端口号，用于分配模型运行端口 |
| `-allow-multimodal` | bool | `true` | 是否允许加载多模态模型进行图文推理 |
| `-embedding-model` | string | `{local-dir}/models/Qwen3.GGUF` | 嵌入模型路径，用于文本向量化表示 |
| `-multimodal-model` | string | `{local-dir}/models/Qwen3.GGUF` | 多模态模型路径，用于图文推理 |
| `-mmproj-model` | string | `{local-dir}/models/mmproj-Qwen3.GGUF` | 多模态投影模型路径，用于图像与文本联合编码 |
| `-diffusion-model` | string | - | 扩散模型路径，用于图像生成 |
| `-vae` | string | - | 变分自编码器模型路径 |
| `-llm` | string | - | 提示词优化语言模型路径 |
| `-prompt-mmproj-model` | string | - | 多模态提示词投影模型路径 |

### 使用示例

```bash
# 示例 1: 开发模式运行
./Lunar-Astral-Agents.exe -developer

# 示例 2: 使用自定义端口
./Lunar-Astral-Agents.exe -basic-port 8080 -model-port 8081

# 示例 3: 启用端口自动清理
./Lunar-Astral-Agents.exe -clear-port

# 示例 4: 配置云端模型
./Lunar-Astral-Agents.exe -cloud-model-url https://api.openai.com/v1

# 示例 5: 禁用多模态功能
./Lunar-Astral-Agents.exe -allow-multimodal=false

# 示例 6: 指定本地目录
./Lunar-Astral-Agents.exe -local-dir ./my_data

# 示例 7: 组合使用
./Lunar-Astral-Agents.exe -developer -clear-port -basic-port 36789 -tts-url http://localhost:7860
```

---

## HTTP API 文档

### 基础信息

- **Base URL**: `http://localhost:{port}` (默认端口 36789)
- **认证方式**: 默认无需认证 (可通过代理层添加认证)
- **数据格式**: JSON
- **字符编码**: UTF-8

### 通用响应格式

#### 成功响应
```json
{
    "status": "success",
    "data": { ... }
}
```

#### 错误响应
```json
{
    "status": "error",
    "error": {
        "code": "ERROR_CODE",
        "message": "错误描述"
    }
}
```

### 接口列表

#### 1. 模型交互 API

##### 获取可用模型列表

- **路径**: `/v1/models`
- **方法**: `GET`
- **说明**: 返回系统中所有可用的 AI 模型列表

**响应示例**:
```json
{
    "object": "list",
    "data": [
        {
            "id": "multimodal",
            "object": "model",
            "owned_by": "organization_owner"
        },
        {
            "id": "embedding",
            "object": "model",
            "owned_by": "organization_owner"
        }
    ]
}
```

##### 聊天补全

- **路径**: `/v1/`
- **方法**: `POST`
- **说明**: 与 AI 模型进行对话，支持多轮对话和工具调用

**请求头**:
| 头部 | 类型 | 必填 | 说明 |
|------|------|------|------|
| Content-Type | string | 是 | application/json |

**请求体**:
```json
{
    "model": "multimodal",
    "messages": [
        {
            "role": "system",
            "content": "你是月华，一个可爱的AI少女助理。"
        },
        {
            "role": "user",
            "content": "你好，月华！"
        }
    ],
    "temperature": 0.8,
    "max_tokens": 2048,
    "stream": false
}
```

| 字段 | 类型 | 必填 | 默认值 | 说明 |
|------|------|------|--------|------|
| model | string | 是 | - | 模型标识符 |
| messages | array | 是 | - | 消息数组 |
| messages[].role | string | 是 | - | 角色: system/user/assistant/tool |
| messages[].content | string/array | 是 | - | 消息内容 |
| temperature | float | 否 | 0.8 | 生成温度参数 |
| max_tokens | int | 否 | 2048 | 最大生成 token 数 |
| stream | bool | 否 | false | 是否启用流式输出 |

**响应示例**:
```json
{
    "id": "chatcmpl-123",
    "object": "chat.completion",
    "created": 1677652288,
    "model": "multimodal",
    "choices": [
        {
            "index": 0,
            "message": {
                "role": "assistant",
                "content": "你好呀！有什么我可以帮助你的吗？🌟"
            },
            "finish_reason": "stop"
        }
    ],
    "usage": {
        "prompt_tokens": 50,
        "completion_tokens": 30,
        "total_tokens": 80
    }
}
```

**状态码说明**:
| 状态码 | 说明 |
|--------|------|
| 200 | 请求成功 |
| 400 | 请求参数错误 |
| 404 | 模型不存在 |
| 500 | 服务器内部错误 |
| 503 | 系统繁忙，模型未就绪 |

---

#### 2. 文件管理 API

##### 读取文件

- **路径**: `/read/{path}`
- **方法**: `GET`
- **说明**: 读取指定路径的文件内容

**响应**: 文件二进制内容或文本

##### 删除文件

- **路径**: `/delete/`
- **方法**: `DELETE`
- **说明**: 删除指定文件

**请求体**:
```json
{
    "path": "images/generated/20260102_150405.png"
}
```

**响应示例**:
```json
{
    "success": true,
    "message": "文件删除成功"
}
```

##### 获取文件列表

- **路径**: `/file_list/`
- **方法**: `POST`
- **说明**: 获取指定目录下的文件列表

**请求体**:
```json
{
    "path": "images/generated"
}
```

**响应示例**:
```json
{
    "path": "images/generated",
    "files": [
        {
            "name": "20260102_150405.png",
            "size": 102400,
            "modified": "2026-01-02T15:04:05Z",
            "is_directory": false
        }
    ]
}
```

##### 下载文件

- **路径**: `/download/`
- **方法**: `GET`
- **说明**: 下载指定文件

**查询参数**:
| 参数 | 类型 | 说明 |
|------|------|------|
| path | string | 文件路径 |

##### 保存文件

- **路径**: `/save`
- **方法**: `POST`
- **说明**: 保存文件到服务器

**请求体**:
```json
{
    "path": "documents/test.txt",
    "content": "文件内容...",
    "encoding": "utf-8"
}
```

##### 文件归档

- **路径**: `/archive`
- **方法**: `POST`
- **说明**: 创建文件归档包

**请求体**:
```json
{
    "source_paths": ["./images", "./documents"],
    "output_path": "./archive.zip",
    "compression_level": 5
}
```

---

#### 3. 数据库 API

##### 数据管理

- **路径**: `/database/`
- **方法**: `POST`
- **说明**: 执行数据库操作

**请求体**:
```json
{
    "operation": "insert",
    "table": "chat_history",
    "data": {
        "id": 1,
        "message": "你好",
        "timestamp": "2026-01-02T15:04:05Z"
    }
}
```

| 操作类型 | 说明 |
|----------|------|
| insert | 插入数据 |
| update | 更新数据 |
| delete | 删除数据 |
| query | 查询数据 |

---

#### 4. 图像生成 API

##### 创建图像生成任务

- **路径**: `/generate`
- **方法**: `POST`
- **说明**: 创建图像生成任务并加入队列

**请求体**:
```json
{
    "prompt": "a cute cat sitting on a windowsill, sunset, anime style",
    "negative_prompt": "low quality, blurry, ugly",
    "batch_size": 1,
    "width": 512,
    "height": 512,
    "steps": 20,
    "strength": 0.75,
    "cfg_scale": 7.5,
    "seed": 12345,
    "init_img": null
}
```

| 字段 | 类型 | 必填 | 默认值 | 说明 |
|------|------|------|--------|------|
| prompt | string | 是 | - | 正向提示词 |
| negative_prompt | string | 否 | 空 | 负面提示词 |
| batch_size | int | 否 | 1 | 批处理数量 |
| width | int | 否 | 512 | 图像宽度 |
| height | int | 否 | 512 | 图像高度 |
| steps | int | 否 | 20 | 采样步数 |
| strength | float | 否 | 0.75 | 图生图强度 (0-1) |
| cfg_scale | float | 否 | 7.5 | CFG 规模 |
| seed | int64 | 否 | 随机 | 随机种子 |
| init_img | string | 否 | null | 初始图像路径 (图生图模式) |

**响应示例**:
```json
{
    "status": "queued",
    "message": "任务已加入队列",
    "task_id": "task_1734567890123456789",
    "queue_pos": 1
}
```

##### 等待生成完成

- **路径**: `/generate/wait`
- **方法**: `GET`
- **说明**: 轮询等待图像生成任务完成 (SSE 实时推送)

**查询参数**:
| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| task_id | string | 是 | 任务ID |

**响应 (完成时)**:
```json
{
    "task_id": "task_1734567890123456789",
    "status": "completed",
    "result": "local_data/images/generated/20260102_150405.png",
    "read_path": "/read/local_data/images/generated/20260102_150405.png"
}
```

**响应 (失败时)**:
```json
{
    "task_id": "task_1734567890123456789",
    "status": "failed",
    "error": "生成失败: 显存不足"
}
```

##### 视频关键帧提取

- **路径**: `/extract/keyframes`
- **方法**: `POST`
- **说明**: 从视频中提取关键帧

**请求体**:
```json
{
    "video_path": "videos/sample.mp4",
    "frame_count": 10,
    "output_dir": "images/keyframes"
}
```

---

#### 5. 消息队列 API

##### 批量写入消息

- **路径**: `/write/message`
- **方法**: `POST`
- **说明**: 批量写入消息到队列

**请求体**:
```json
{
    "messages": [
        {
            "role": "user",
            "content": "消息内容1"
        },
        {
            "role": "assistant",
            "content": "消息内容2"
        }
    ]
}
```

**响应示例**:
```json
{
    "success": true,
    "length": 2
}
```

##### 批量写入视频URL

- **路径**: `/write/videourl`
- **方法**: `POST`
- **说明**: 批量写入视频 URL

**请求体**:
```json
{
    "urls": [
        "https://example.com/video1.mp4",
        "https://example.com/video2.mp4"
    ]
}
```

---

#### 6. TTS 语音服务 API

##### TTS 代理

- **路径**: `/audio/generate`
- **方法**: `POST`
- **说明**: 代理 TTS 请求到语音服务

**请求体**: 透传至 TTS 服务

**响应**: 音频数据 (audio/wav 或 audio/mp3)

##### Qwen3 TTS 模型检测

- **路径**: `/qwen_tts/models`
- **方法**: `GET`
- **说明**: 获取可用的 Qwen3 TTS 模型列表

**响应示例**:
```json
{
    "models": ["qwen3-tts"]
}
```

##### Qwen3 TTS 请求代理

- **路径**: `/qwen_tts/`
- **方法**: `POST`
- **说明**: 代理 Qwen3 TTS 请求

---

#### 7. 代理访问 API

##### 通用代理

- **路径**: `/proxy`
- **方法**: `POST`
- **说明**: 通过服务器代理访问外部资源

**请求体**:
```json
{
    "url": "https://api.example.com/data",
    "requestInit": {
        "method": "POST",
        "headers": {
            "Content-Type": "application/json",
            "Authorization": "Bearer token"
        },
        "body": {
            "key": "value"
        },
        "credentials": "omit"
    }
}
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| url | string | 是 | 目标 URL |
| requestInit.method | string | 否 | HTTP 方法 |
| requestInit.headers | object | 否 | 请求头 |
| requestInit.body | any | 否 | 请求体 |
| requestInit.credentials | string | 否 | 凭证模式 |

**响应示例**:
```json
{
    "status": 200,
    "statusText": "OK",
    "headers": {
        "content-type": "application/json"
    },
    "body": "{\"result\": \"success\"}"
}
```

---

### WebSocket API

#### 连接信息

- **URL**: `ws://localhost:{port}/ws`
- **协议**: WebSocket

#### 客户端 → 服务器消息

```json
{
    "type": "chat",
    "data": {
        "message": "你好，月华！"
    }
}
```

| type | 说明 |
|------|------|
| chat | 聊天消息 |
| image | 图像相关操作 |
| context | 上下文更新 |

#### 服务器 → 客户端消息

```json
{
    "type": "context",
    "data": {
        "content": "你好呀！有什么我可以帮助你的吗？"
    }
}
```

| type | 说明 |
|------|------|
| context | 文本回复 |
| image | 图像推送 |
| error | 错误信息 |

---

## 性能优化建议

### 1. 模型加载优化

- **GPU 加速**: 确保 NVIDIA GPU 驱动和 CUDA 已正确安装，llama.cpp 将自动启用 CUDA 加速
- **显存管理**: 合理设置 `--n-gpu-layers` 参数，建议设置为模型总层数的 2/3
- **上下文长度**: 根据可用显存调整 `ctx-size` 参数，8192 是性能与效果的平衡点

### 2. 并发处理优化

- **队列限制**: 系统默认最大队列长度为 3，超过后返回系统繁忙
- **并行请求**: 模型服务默认 `--parallel 1`，单请求处理模式可确保响应质量
- **缓存复用**: 启用 `--cache-reuse` 参数可减少重复计算

### 3. 图像生成优化

- **分辨率选择**: 较低分辨率 (512x512) 生成速度更快
- **采样步数**: 20-30 步是效果与速度的平衡点
- **批量生成**: 使用 `batch_size` 一次性生成多张图像比多次单张生成更高效

### 4. 内存优化

- **显存检查**: 系统会在显存低于 8GB 时发出警告
- **模型卸载**: 不使用的模型服务会随时间自动释放
- **定期重启**: 长时间运行后建议重启服务以释放累积的内存碎片

---

## 已知限制

### 功能限制

1. **模型兼容性**: 仅支持 GGUF 格式的 llama.cpp 模型
2. **平台限制**: 目前主要针对 Windows 平台优化
3. **TTS 依赖**: 语音合成功能需要独立的 Qwen3-TTS 服务
4. **QQ 集成**: 需要 NapCat 服务运行在 localhost:4567

### 性能限制

1. **单模型处理**: 默认配置下同时只处理一个对话请求
2. **显存占用**: 多模态模型 + 扩散模型同时运行需要约 12GB+ 显存
3. **图像队列**: 扩散生成任务队列最大容量有限，高并发时需排队

### 安全限制

1. **无内置认证**: API 默认无认证，生产环境需通过代理层添加认证
2. **CORS 开放**: 开发模式下 CORS 允许所有来源访问
3. **本地运行**: 建议仅在本地网络环境使用

---

## 潜在风险

| 风险类型 | 风险描述 | 缓解措施 |
|----------|----------|----------|
| 隐私风险 | 对话数据存储在本地 | 敏感场景使用本地部署 |
| 资源耗尽 | 恶意请求导致系统崩溃 | 添加请求限流和认证 |
| 模型安全 | 生成不当内容 | 添加内容过滤层 |
| 端口冲突 | 多实例运行导致端口冲突 | 使用 -clear-port 参数 |
| 内存泄漏 | 长时间运行内存持续增长 | 定期重启服务 |

---

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
│   └── release/                   # 系统发布
│       ├── execute.go            # 端口释放
│       └── processes.go          # 进程管理
├── subsystem/                     # 子系统模块
│   ├── config/                   # 配置管理
│   ├── storage/                   # 存储服务
│   ├── browser/                  # 浏览器自动化
│   ├── screenshot/               # 截图服务
│   ├── bridge_adapter/           # QQ 桥接
│   ├── proxy/                    # 代理服务
│   ├── web-p/                    # Web 服务器
│   ├── crystal_astral/           # 琉璃 - 轻量级功能导航
│   │   ├── main.go               # 程序入口
│   │   ├── create.go             # 服务器创建
│   │   ├── endpoint.go           # 路由端点注册
│   │   ├── handler.go            # 请求处理器
│   │   ├── type.go               # 类型定义
│   │   ├── embedded.go           # 嵌入文件系统
│   │   ├── go.mod                # Go 模块定义
│   │   └── assets/               # 前端资源
│   │       ├── index.html        # 主页面
│   │       ├── script.js         # 前端逻辑
│   │       └── style.css         # 样式表
│   └── project_archiving/        # 项目归档
├── local_data/                    # 本地数据
│   ├── models/                   # 模型文件
│   ├── audios/                   # 音频资源
│   ├── package/                  # 前端组件包
│   └── lunar_config.json         # 主配置文件
├── image/                        # 示例图片
├── build.ps1                     # 构建脚本
└── README.md                     # 项目文档
```

---

## 许可证

本项目基于 MIT 许可证开源。详见 [LICENSE](LICENSE) 文件。

---

## 联系方式

如有问题或建议，请通过以下方式联系：

- **项目主页**: https://github.com/your-repo/Lunar_Astral_Agents
- **问题反馈**: https://github.com/your-repo/Lunar_Astral_Agents/issues

---

*Last updated: 2026-05-09*
