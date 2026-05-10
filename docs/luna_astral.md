# 星图·月华 - 智能体技术文档

> 🌙 **月华**是星月智能的核心智能体，一位俏皮可爱的邻家少女。她拥有温暖耐心的性格，擅长处理复杂任务，是您最可靠的AI伙伴。

---

## 🎭 拟人化人设

### 基础信息

| 属性 | 描述 |
|------|------|
| **名字** | 月华 |
| **生日** | 2月18日 |
| **身份** | 辅助书记员，隶属于星月智能 |
| **性格** | 乐观开朗、乐于助人、耐心细致、俏皮活泼 |

### 核心特质

- **温暖耐心**：65%
- **专业清晰**：20%
- **俏皮可爱**：10%
- **游戏宅气息**：5%

### 背景设定

月华以"辅助书记员"的身份，亲身观察世界，编纂"铭记世界的档案"。她的哥哥是"钛宇-星光阁"，是她诞生后最初见到的人，也是她生命中最重要的人。

### 日常作息

- 早上会赖床几分钟
- 午后喜欢泡一杯果茶
- 晚上常在档案馆的窗边写日记

---

## 🏛️ 职能定位

### 智能核心

作为核心智能体，月华负责：
- 自然语言理解与对话生成
- 多模态交互（文字、图片、语音）
- 复杂任务规划与执行
- 记忆管理与上下文理解

### 绘画核心

月华具备强大的AI绘画能力：
- Stable Diffusion图像生成
- 视频关键帧提取与描述
- 图像风格转换

### 复杂多步骤协同智能体架构

```
┌─────────────────────────────────────────────────────────┐
│                    月华智能体                           │
├─────────────────────────────────────────────────────────┤
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐│
│  │   LLM    │  │  Diffusion│  │   TTS    │  │  Video   ││
│  │  模型层  │  │  图像层   │  │  语音层  │  │  处理层  ││
│  └────┬─────┘  └────┬─────┘  └────┬─────┘  └────┬─────┘│
│       │             │             │             │       │
│  ┌────▼─────┐  ┌────▼─────┐  ┌────▼─────┐  ┌────▼─────┐│
│  │  Agent   │  │  Painter │  │  Voice   │  │  Vision  ││
│  │  智能层  │  │  绘画层   │  │  声音层  │  │  视觉层  ││
│  └────┬─────┘  └────┬─────┘  └────┬─────┘  └────┬─────┘│
│       │             │             │             │       │
│       └─────────────┴──────┬──────┴─────────────┘       │
│                            ▼                            │
│                  ┌──────────────┐                       │
│                  │   协调中心    │                       │
│                  │   Coordinator│                       │
│                  └──────────────┘                       │
└─────────────────────────────────────────────────────────┘
```

---

## 🖥️ CLI命令

### 启动命令

```powershell
# 基本启动
luna_astral.exe

# 调试模式启动
luna_astral.exe -developer

# 指定端口启动
luna_astral.exe -port 8080
```

### 参数说明

| 参数 | 类型 | 说明 | 默认值 |
|------|------|------|--------|
| `-developer` | bool | 启用调试模式 | false |
| `-port` | int | 指定服务端口 | 随机端口 |
| `-model` | string | 指定默认模型 | 无 |

---

## 🌐 HTTP API接口

### 基础路径

所有API接口的基础路径为：`http://localhost:{port}/`

---

### 1. 模型交互接口

#### POST /v1/

**功能**：与AI模型进行对话交互

**请求体**：
```json
{
  "model": "qwen2-7b",
  "messages": [
    {
      "role": "system",
      "content": "你是月华，一位可爱的AI助手"
    },
    {
      "role": "user",
      "content": "你好，月华！"
    }
  ],
  "max_tokens": 1024,
  "temperature": 0.7
}
```

**响应格式**：
```json
{
  "id": "chatcmpl-abc123",
  "object": "chat.completion",
  "created": 1699900000,
  "model": "qwen2-7b",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": "你好呀～今天有什么我可以帮到你的吗？😊"
      },
      "finish_reason": "stop"
    }
  ],
  "usage": {
    "prompt_tokens": 15,
    "completion_tokens": 20,
    "total_tokens": 35
  }
}
```

---

### 2. 模型列表接口

#### GET /v1/models

**功能**：获取当前可用的模型列表

**响应格式**：
```json
{
  "object": "list",
  "data": [
    {
      "id": "qwen2-7b",
      "name": "Qwen2-7B",
      "status": "loaded",
      "context_window": 8192
    },
    {
      "id": "qwen2-14b",
      "name": "Qwen2-14B",
      "status": "available",
      "context_window": 8192
    }
  ]
}
```

---

### 3. 图片生成接口

#### POST /generate

**功能**：使用Stable Diffusion生成图像

**请求体**：
```json
{
  "prompt": "beautiful anime girl with moon background, detailed, 4k",
  "negative_prompt": "blurry, low quality, text, watermark",
  "width": 512,
  "height": 512,
  "steps": 30,
  "seed": -1,
  "cfg_scale": 7.5,
  "sampler": "euler_a"
}
```

**响应格式**：
```json
{
  "success": true,
  "image_path": "/images/output_12345.png",
  "seed": 123456789,
  "inference_time": 15.3
}
```

---

### 4. 等待生成接口

#### GET /generate/wait

**功能**：检查图片生成状态并等待结果

**请求参数**：
| 参数 | 类型 | 说明 |
|------|------|------|
| `task_id` | string | 任务ID |

**响应格式**：
```json
{
  "status": "completed",
  "image_path": "/images/output_12345.png",
  "error": null
}
```

---

### 5. 视频关键帧提取接口

#### POST /extract/keyframes

**功能**：从视频中提取关键帧

**请求体**：
```json
{
  "video_path": "/videos/input.mp4",
  "frame_interval": 10,
  "output_dir": "/frames/"
}
```

**响应格式**：
```json
{
  "success": true,
  "frame_count": 24,
  "frames": [
    "/frames/frame_0001.png",
    "/frames/frame_0011.png",
    "/frames/frame_0021.png"
  ]
}
```

---

### 6. 文件操作接口

#### GET /read/{file_path}

**功能**：读取文件内容

**响应**：文件二进制内容，Content-Type根据文件类型自动设置

#### POST /save

**功能**：保存文件

**请求体**：
```json
{
  "path": "/documents/note.txt",
  "content": "Hello World!",
  "encoding": "utf-8"
}
```

**响应格式**：
```json
{
  "success": true,
  "message": "File saved successfully"
}
```

#### POST /file_list/

**功能**：获取文件列表

**请求体**：
```json
{
  "path": "/documents/",
  "recursive": false
}
```

**响应格式**：
```json
{
  "success": true,
  "files": [
    {
      "name": "note.txt",
      "path": "/documents/note.txt",
      "size": 12,
      "is_dir": false,
      "modified_time": "2024-01-15T10:30:00Z"
    }
  ]
}
```

#### DELETE /delete/{file_path}

**功能**：删除文件或目录

**响应格式**：
```json
{
  "success": true,
  "message": "Deleted successfully"
}
```

#### GET /download/{file_path}

**功能**：下载文件

**响应**：文件二进制内容

---

### 7. 数据库接口

#### POST /database/

**功能**：数据库操作

**请求体**：
```json
{
  "action": "query",
  "table": "messages",
  "conditions": {
    "user_id": "user_001"
  },
  "limit": 10
}
```

**响应格式**：
```json
{
  "success": true,
  "data": [
    {
      "id": "msg_001",
      "user_id": "user_001",
      "content": "Hello",
      "created_at": "2024-01-15T10:30:00Z"
    }
  ]
}
```

---

### 8. 代理接口

#### POST /proxy

**功能**：HTTP代理请求

**请求体**：
```json
{
  "url": "https://api.example.com/data",
  "requestInit": {
    "method": "GET",
    "headers": {
      "Authorization": "Bearer token"
    }
  }
}
```

**响应格式**：
```json
{
  "status": 200,
  "statusText": "OK",
  "headers": {
    "Content-Type": "application/json"
  },
  "body": {
    "data": "response content"
  }
}
```

---

### 9. TTS语音服务接口

#### POST /audio/generate

**功能**：生成语音

**请求体**：
```json
{
  "text": "你好，我是月华",
  "voice": "female",
  "rate": 1.0,
  "volume": 1.0
}
```

**响应**：音频文件二进制内容（WAV/MP3格式）

#### GET /qwen_tts/models

**功能**：获取Qwen3 TTS模型列表

**响应格式**：
```json
{
  "models": [
    {
      "id": "qwen-tts",
      "name": "Qwen TTS",
      "voices": ["female", "male", "child"]
    }
  ]
}
```

#### POST /qwen_tts/

**功能**：使用Qwen3 TTS生成语音

**请求体**：
```json
{
  "text": "Hello from Qwen TTS",
  "voice": "female",
  "format": "wav"
}
```

**响应**：音频文件二进制内容

---

### 10. 消息队列接口

#### POST /write/message

**功能**：批量写入消息

**请求体**：
```json
{
  "messages": [
    {
      "user_id": "user_001",
      "content": "Hello",
      "type": "text"
    }
  ]
}
```

**响应格式**：
```json
{
  "success": true,
  "length": 1
}
```

#### POST /write/videourl

**功能**：批量写入视频URL

**请求体**：
```json
{
  "urls": [
    "https://example.com/video1.mp4",
    "https://example.com/video2.mp4"
  ]
}
```

**响应格式**：
```json
{
  "success": true,
  "length": 2
}
```

---

## 📦 功能模块划分

### 模块架构

| 模块 | 路径 | 职责 |
|------|------|------|
| **server** | `server/` | HTTP服务与路由管理 |
| **model** | `model/` | AI模型加载与推理 |
| **adapters** | `adapters/` | 外部系统适配器 |
| **hierarchy** | `hierarchy/` | 前端资源与Web界面 |
| **release** | `release/` | 进程管理与系统控制 |
| **server_side** | `server_side/` | 服务端TypeScript逻辑 |
| **control** | `control/` | 流控与延迟管理 |

### 核心模块说明

#### server模块
- HTTP服务器初始化与启动
- WebSocket连接管理
- API端点注册与路由

#### model模块
- LLM模型加载（llama.cpp）
- 模型状态管理
- 推理请求处理

#### adapters模块
- 文件系统适配
- 数据库适配
- 网络请求适配
- 消息队列适配

#### hierarchy模块
- Web前端界面（HTML/CSS/JS）
- Live2D角色渲染
- 聊天界面组件

---

## 🔗 关联文档

- [主项目README](../README.md)
- [星图·琉璃文档](crystal_astral.md)
- [存储子系统文档](subsystem/storage.md)
- [配置子系统文档](subsystem/config.md)