# 子系统——配置管理（general_config）

全局配置中枢模块，负责聚合所有命令行参数与 JSON 配置文件，为其他子系统提供统一的配置访问点。

---

## 目录

- [功能概述](#功能概述)
- [配置体系](#配置体系)
- [核心模块说明](#核心模块说明)
- [配置项参考](#配置项参考)
- [使用示例](#使用示例)
- [常见问题](#常见问题)

---

## 功能概述

`general_config` 模块是星月智能平台的**基础设施层**，不实现任何业务逻辑，仅负责：

1. 定义并聚合所有**命令行参数**（Go `flag` 包）
2. 通过 **JSON 配置文件** (`lunar_config.json`) 覆盖默认值
3. 维护**运行时共享状态**（模型就绪状态、端口映射表、MIME 类型映射等）

### 模块特点

| 特点 | 说明 |
|------|------|
| 零外部依赖 | 仅依赖 Go 标准库（`encoding/json`、`flag`、`log`、`os`、`sync`） |
| 纯数据提供者 | 不 import 任何项目内部模块，其他模块通过 `import "LunarSubsystem/GeneralConfig"` 读取 |
| 双层配置 | 命令行参数 + JSON 配置文件增量覆盖 |
| 并发安全 | 端口映射表使用 `sync.RWMutex` 保护 |

---

## 配置体系

### 双层架构

```
┌──────────────────────────────────────┐
│  第 1 层：命令行参数（flag 包）        │
│  所有配置项均有默认值                  │
│  示例: -basic-port 36800             │
├──────────────────────────────────────┤
│  第 2 层：lunar_config.json 覆盖      │
│  仅非空字段才覆盖（增量式更新）         │
│  示例: {"agent": {"embedding_model": "system-embedding"}}    │
└──────────────────────────────────────┘
```

### 启动流程

```
程序启动
    │
    ▼
① init() 函数执行（Go runtime 自动调用）
    │  过滤 -test.* 标志 → flag.Parse() 解析命令行参数，设置全局 flag 变量
    ▼
② 获取 exe 所在目录 → 拼接 {exeDir}/{LocalDir}/lunar_config.json
    │  读取并解析 JSON → 按字段覆盖对应的全局配置变量
    ▼
③ 配置就绪，供其他子系统读取
```

---

## 核心模块说明

### 文件职责

| 文件 | 核心变量 | 职责 |
|------|---------|------|
| [init.go](init.go) | `ModelConfig` 结构体 | JSON 配置文件加载 + `init()` 入口 |
| [port.go](port.go) | `BasicPort`、`ModelPort`、`ProxyPort` 等 | 网络端口体系 |
| [allow.go](allow.go) | `AllowDiffusion`、`AllowBrowser` | 功能开关控制 |
| [engine.go](engine.go) | `InferEngine`、`VisualEngine` | 外部引擎可执行文件路径 |
| [image.go](image.go) | `MaxWidth`、`MaxHeight`、`JPEGQuality`、`Format`、`FfmpegPath` | 图像处理参数 |
| [model.go](model.go) | `Agent/Memory/Search` 三组模型配置 + 引擎模型路径 | AI 模型名称与 API 地址（每组含嵌入/多模态 Model+URL+Key） |
| [path.go](path.go) | `LocalDir`、`CertFile`、`KeyFile`、`KnowledgeDBPath`、`MemoryDBDir` | 本地资源目录、TLS 证书路径、数据库路径 |
| [system.go](system.go) | `Developer`、`ModelReady`、`ModelPortMap`、`MimeMap` | 运行时共享状态 |
| [webview.go](webview.go) | `WebViewTitle`、`WebViewWidth` 等 6 项 | 桌面 WebView 窗口尺寸与行为 |

### 端口体系

端口以 `BasicPort`（默认 36789）为锚点，构建递增范围体系：

```
MinPort          ModelPort         ProxyPort        MaxPort
  │                  │                  │               │
36784              36790              36794           36804
  │                  │                  │               │
  ├── 端口区间下界    ├── 模型服务起始    ├── 代理端口      ├── 端口区间上界
```

- `ModelPortMap`（`map[string]int`）运行时动态维护 `模型名 → 端口号` 的映射
- 支持在该端口范围内同时运行多个模型服务实例

### 模型配置矩阵

模型配置分为**三类角色**（agent 核心智能体 / memory 记忆库 / search 智能搜索），每类均含嵌入与多模态两组服务配置，每组由 `Model`（模型名）+ `URL`（API 地址）+ `Key`（密钥）组成：

| 角色 | 配置组 | 嵌入服务（文本向量化） | 多模态服务（图文推理） |
|------|--------|----------------------|----------------------|
| 核心智能体 | `Agent*` | `AgentEmbeddingModel/URL/Key` | `AgentMultimodalModel/URL/Key` |
| 记忆库 | `Memory*` | `MemoryEmbeddingModel/URL/Key` | `MemoryMultimodalModel/URL/Key` |
| 智能搜索 | `Search*` | `SearchEmbeddingModel/URL/Key` | `SearchMultimodalModel/URL/Key` |

默认模型名为 `system-embedding`（嵌入）与 `system-multimodal`（多模态），默认 API 地址为 `http://127.0.0.1:36789/v1`（同源 v1 端点）。

此外包含引擎级模型路径：

| 模型变量 | 用途 |
|---------|------|
| `MmprojModel` | 图像-文本联合编码（视觉投影层） |
| `AsrModel` | 自动语音识别（语音转文本） |
| `DiffusionModel` | 扩散模型（文生图） |
| `VariationalModel` | VAE 编解码（图像编解码） |
| `PromptAnalysisModel` | 提示词精炼（图像生成提示优化） |
| `PromptMmprojModel` | 提示词分析投影模型（因版本原因暂不可用） |
| `RealESRGANModel` | 图像超分辨率 |

---

## 配置项参考

### 端口配置（port.go）

| 变量 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `BasicPort` | `*int` | `36789` | 基础 HTTP 服务端口 |
| `ModelPort` | `*int` | `36790` | llama.cpp 模型服务端口 |
| `ProxyPort` | `*int` | `36794` | 代理转发端口 |
| `MinPort` | `*int` | `36784` | 端口范围下界 |
| `MaxPort` | `*int` | `36804` | 端口范围上界 |

### 功能开关（allow.go）

| 变量 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `AllowDiffusion` | `*bool` | `true` | 是否启用扩散图像生成 |
| `AllowBrowser` | `*bool` | `true` | 是否允许打开系统浏览器 |

### 模型名称与 API 地址（model.go）

三类角色（agent / memory / search）各含嵌入与多模态两组服务，默认模型名 `system-embedding` / `system-multimodal`，默认 API 地址 `http://127.0.0.1:36789/v1`：

| 变量 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `AgentEmbeddingModel/URL/Key` | `*string` | `system-embedding` / `http://127.0.0.1:36789/v1` / `""` | 核心智能体嵌入服务 |
| `AgentMultimodalModel/URL/Key` | `*string` | `system-multimodal` / `http://127.0.0.1:36789/v1` / `""` | 核心智能体多模态服务 |
| `MemoryEmbeddingModel/URL/Key` | `*string` | `system-embedding` / `http://127.0.0.1:36789/v1` / `""` | 记忆库嵌入服务 |
| `MemoryMultimodalModel/URL/Key` | `*string` | `system-multimodal` / `http://127.0.0.1:36789/v1` / `""` | 记忆库多模态服务 |
| `SearchEmbeddingModel/URL/Key` | `*string` | `system-embedding` / `http://127.0.0.1:36789/v1` / `""` | 智能搜索嵌入服务 |
| `SearchMultimodalModel/URL/Key` | `*string` | `system-multimodal` / `http://127.0.0.1:36789/v1` / `""` | 智能搜索多模态服务 |

引擎级模型路径（默认指向 `{LocalDir}/models/` 下的文件）：

| 变量 | 默认值 |
|------|---------|
| `MmprojModel` | `{LocalDir}/models/mmproj-Qwen3.GGUF` |
| `AsrModel` | `{LocalDir}/models/Qwen3-ASR-0.6B` |
| `DiffusionModel` | `{LocalDir}/models/Qwen3.GGUF` |
| `VariationalModel` | `{LocalDir}/models/Qwen3.GGUF` |
| `PromptAnalysisModel` | `{LocalDir}/models/Qwen3.GGUF` |
| `PromptMmprojModel` | `""`（暂不可用） |
| `RealESRGANModel` | `{LocalDir}/models/Qwen3.pth` |

### 引擎路径（engine.go）

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `InferEngine` | `{LocalDir}/models/llama.cpp/llama-server.exe` | llama.cpp 推理服务器 |
| `VisualEngine` | `{LocalDir}/models/stable_diffusion.cpp/sd-cli.exe` | stable-diffusion 命令行 |

### 图像参数（image.go）

| 变量 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `MaxWidth` | `*int` | `1920` | 图像最大宽度 |
| `MaxHeight` | `*int` | `1080` | 图像最大高度 |
| `JPEGQuality` | `*int` | `90` | JPEG 压缩质量 (1-100) |
| `Format` | `*string` | `"jpg"` | 默认图片格式 |
| `FfmpegPath` | `*string` | `""` | FFmpeg 可执行文件路径（空则使用系统 PATH） |

### WebView 窗口（webview.go）

| 变量 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `WebViewTitle` | `*string` | `"星月智能 - 月之华"` | 窗口标题 |
| `WebViewWidth` | `*int` | `345` | 窗口宽度 |
| `WebViewHeight` | `*int` | `500` | 窗口高度 |
| `WebViewMinWidth` | `*int` | `260` | 最小宽度 |
| `WebViewMinHeight` | `*int` | `260` | 最小高度 |
| `WebViewResizable` | `*bool` | `true` | 是否可调整大小 |

### 运行时状态（system.go）

| 变量 | 类型 | 说明 |
|------|------|------|
| `Developer` | `*bool` | 开发模式开关，启用调试模式显示详细日志 |
| `ModelReady` | `int` | 模型是否就绪的状态标识（0 表示未就绪） |
| `MaxModelAmount` | `int` | 系统支持的最大模型数量（默认 0，初始未设置） |
| `ServerAddress` | `[]string` | 服务器对外 IP 地址列表 |
| `ModelPortMap` | `map[string]int` | 运行时模型名→端口号映射 |
| `ModelMapMutex` | `sync.RWMutex` | 保护 `ModelPortMap` 并发读写的互斥锁 |
| `MimeMap` | `map[string]string` | 扩展名→MIME 类型映射表 |

---

## 使用示例

### 命令行使用

```powershell
# 指定基础端口启动
.\Lunar_Astral.exe -basic-port 36800

# 开发模式（启用调试日志）
.\Lunar_Astral.exe -developer

# 禁用扩散图像生成
.\Lunar_Astral.exe -allow-diffusion=false

# 覆盖核心智能体多模态模型服务地址
.\Lunar_Astral.exe -agent-multimodal-url http://127.0.0.1:36789/v1
```

### lunar_config.json 示例

配置文件按 `models` / `server` / `agent` / `memory` / `search` 五组组织：

```json
{
  "models": {
    "diffusion_model": "./local_data/models/z_image_turbo/z-image-turbo-Q4_K_M-UD.gguf",
    "variational_model": "./local_data/models/z_image_turbo/diffusion_pytorch_model.safetensors",
    "prompt_analysis_model": "./local_data/models/z_image_turbo/Qwen3-4B-Instruct-2507-Q4_K_M.gguf",
    "real_esrgan_model": "./local_data/models/z_image_turbo/4x-AnimeSharp.pth",
    "asr_model": "./local_data/models/Qwen3-ASR-0.6B"
  },
  "server": {
    "user_name": null,
    "developer": false,
    "allow_diffusion": true,
    "bridging_type": "napcat",
    "bridging_path": "ws://localhost:4567",
    "bridging_token": "",
    "bridging_target": 906314036,
    "bridging_keywords": ["月华", "3826713076"]
  },
  "agent": {
    "embedding_model": "system-embedding",
    "embedding_url": "http://127.0.0.1:36789/v1",
    "embedding_key": "",
    "multimodal_model": "system-multimodal",
    "multimodal_url": "http://127.0.0.1:36789/v1",
    "multimodal_key": ""
  },
  "memory": {
    "embedding_model": "system-embedding",
    "embedding_url": "http://127.0.0.1:36789/v1",
    "embedding_key": "",
    "multimodal_model": "system-multimodal",
    "multimodal_url": "http://127.0.0.1:36789/v1",
    "multimodal_key": ""
  },
  "search": {
    "embedding_model": "system-embedding",
    "embedding_url": "http://127.0.0.1:36789/v1",
    "embedding_key": "",
    "multimodal_model": "system-multimodal",
    "multimodal_url": "http://127.0.0.1:36789/v1",
    "multimodal_key": ""
  }
}
```

> **注意**：`lunar_config.json` 应放置在 `{LocalDir}/` 目录下（默认可执行文件同目录的 `local_data/`）。`server` 分组的 `developer` / `allow_diffusion` 为布尔覆盖；其余字段仅在非空时覆盖对应配置。`bridging_*` 等扩展字段由月华主程序读取，配置模块不处理。

---

## 常见问题

### Q: lunar_config.json 放在哪里？

放置在 `{LocalDir}/` 目录下。默认情况下 `LocalDir` 值为 `local_data`，所以完整路径为 `<exe所在目录>/local_data/lunar_config.json`。

### Q: 哪些配置只能在命令行中设置？

端口号（`BasicPort`、`ModelPort` 等）、功能开关（`AllowBrowser` 等）、图像参数（`MaxWidth`、`MaxHeight` 等）和 WebView 参数只能通过命令行设置。`lunar_config.json` 中 `models` 分组覆盖模型路径，`server` 分组的 `developer` / `allow_diffusion` 覆盖对应开关，`agent` / `memory` / `search` 分组覆盖各自的模型名与 API 地址。

### Q: 如何查看当前所有配置值？

开发模式下运行程序，启动日志会打印大部分配置参数。也可以在各模块的代码中通过 `fmt.Println(*config.BasicPort)` 自行打印。

### Q: 配置修改后需要重新编译吗？

不需要。`lunar_config.json` 在每次程序启动时重新读取，修改后重启程序即可生效。命令行参数也无需重新编译。

---

## 相关文档

- [项目主文档](../../README.md) —— 环境要求与编译流程
- [星图·月华](../../lunar_astral/README.md) —— 配置使用方
- [星图·琉璃](../../crystal_astral/README.md) —— 配置使用方
- [网页前端子系统](../browser_client/README.md) —— WebView 窗口参数使用