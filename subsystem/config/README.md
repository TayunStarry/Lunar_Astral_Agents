# 子系统——配置管理（config）

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

`config` 模块是星月智能平台的**基础设施层**，不实现任何业务逻辑，仅负责：

1. 定义并聚合所有**命令行参数**（Go `flag` 包）
2. 通过 **JSON 配置文件** (`lunar_config.json`) 覆盖默认值
3. 维护**运行时共享状态**（模型就绪状态、端口映射表、MIME 类型映射等）

### 模块特点

| 特点 | 说明 |
|------|------|
| 零外部依赖 | 仅依赖 Go 标准库（`encoding/json`、`flag`、`log`、`os`、`sync`） |
| 纯数据提供者 | 不 import 任何项目内部模块，其他模块通过 `import "config"` 读取 |
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
│  示例: {"MultimodalModel": "..."}    │
└──────────────────────────────────────┘
```

### 启动流程

```
程序启动
    │
    ▼
① flag.Parse()（由 Go runtime 隐式调用）
    │  解析所有命令行参数 → 设置全局 flag 变量默认值
    ▼
② init() 函数执行
    │  获取 exe 所在目录 → 拼接 {exeDir}/{LocalDir}/lunar_config.json
    │  读取并解析 JSON → 将非空字段覆盖到对应的全局 flag 变量
    ▼
③ 配置就绪，供其他子系统读取
```

---

## 核心模块说明

### 文件职责

| 文件 | 核心变量 | 职责 |
|------|---------|------|
| [init.go](init.go) | `ModelConfig` 结构体 | JSON 配置文件加载 + `init()` 入口 |
| [port.go](port.go) | `BasicPort`、`ModelPort`、`ProxyPort` 等 | 网络端口体系与云服务地址 |
| [allow.go](allow.go) | `AllowDiffusion`、`AllowBrowser` | 功能开关控制 |
| [engine.go](engine.go) | `InferEngine`、`VisualEngine` | 外部引擎可执行文件路径 |
| [image.go](image.go) | `MaxWidth`、`MaxHeight`、`JPEGQuality`、`Format`、`FfmpegPath` | 图像处理参数 |
| [model.go](model.go) | `EmbeddingModel` 等 8 个模型路径 | 全部 AI 模型文件路径（GGUF 格式） |
| [path.go](path.go) | `LocalDir`、`CertFile`、`KeyFile`、`Database` | 本地资源目录、TLS 证书路径 |
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

### AI 模型矩阵

系统预设了 8 个模型路径，全部采用 GGUF 格式：

| 模型变量 | 用途 | 说明 |
|---------|------|------|
| `EmbeddingModel` | 文本向量化 | 文本嵌入模型 |
| `MultimodalModel` | 图文多模态推理 | 主对话模型 |
| `MmprojModel` | 图像-文本联合编码 | 视觉投影层 |
| `AsrModel` | 自动语音识别 | 语音转文本 |
| `DiffusionModel` | 扩散模型 | 文生图 |
| `VariationalModel` | VAE 编解码 | 图像编解码 |
| `PromptAnalysisModel` | 提示词精炼 | 图像生成提示优化 |
| `PromptMmprojModel` | 提示词多模态投影 | 暂不可用 |

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
| `CloudModelUrl` | `*string` | `""` | 云端模型服务地址（非空时优先使用） |

### 功能开关（allow.go）

| 变量 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `AllowDiffusion` | `*bool` | `true` | 是否启用扩散图像生成 |
| `AllowBrowser` | `*bool` | `true` | 是否允许打开系统浏览器 |
| `ClearPort` | `*bool` | `false` | 是否在启动时清理端口 |

### 模型路径（model.go）

所有模型路径默认指向 `{LocalDir}/models/` 下的 GGUF 文件。

| 变量 | 默认值示例 |
|------|---------|
| `MultimodalModel` | `{LocalDir}/models/Qwen3-1.7B-Q4_K_M.gguf` |
| `MmprojModel` | `{LocalDir}/models/mmproj-Qwen3-1.7B-F16.gguf` |
| `EmbeddingModel` | `{LocalDir}/models/Qwen3-Embedding-0.6B-Q4_K_M.gguf` |
| `AsrModel` | `{LocalDir}/models/Qwen3-ASR-0.6B-Q4_K_M.gguf` |
| `DiffusionModel` | `{LocalDir}/models/sd3_medium.safetensors` |
| `VariationalModel` | `{LocalDir}/models/sd3_vae.safetensors` |

### 引擎路径（engine.go）

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `InferEngine` | `{LocalDir}/models/llama.cpp/llama-server.exe` | llama.cpp 推理服务器 |
| `VisualEngine` | `./sd-cli.exe` | stable-diffusion 命令行 |

### 图像参数（image.go）

| 变量 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `MaxWidth` | `*int` | `768` | 图像最大宽度 |
| `MaxHeight` | `*int` | `768` | 图像最大高度 |
| `JPEGQuality` | `*int` | `85` | JPEG 压缩质量 (1-100) |
| `Format` | `*string` | `"jpg"` | 默认图片格式 |
| `FfmpegPath` | `*string` | `"ffmpeg"` | FFmpeg 可执行文件路径 |

### WebView 窗口（webview.go）

| 变量 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `WebViewTitle` | `*string` | `"星月智能"` | 窗口标题 |
| `WebViewWidth` | `*int` | `648` | 窗口宽度 |
| `WebViewHeight` | `*int` | `960` | 窗口高度 |
| `WebViewMinWidth` | `*int` | `400` | 最小宽度 |
| `WebViewMinHeight` | `*int` | `400` | 最小高度 |
| `WebViewResizable` | `*int` | `1` | 是否可调整大小 (1=是) |

### 运行时状态（system.go）

| 变量 | 类型 | 说明 |
|------|------|------|
| `Developer` | `*bool` | 开发模式开关（直接读取文件系统而非 embed） |
| `ModelReady` | `int` | 已就绪的模型数量（原子操作） |
| `MaxModelAmount` | `int` | `1`，最大可并发处理的模型数 |
| `ModelPortMap` | `map[string]int` | 运行时模型→端口映射（RWMutex 保护） |
| `MimeMap` | `map[string]string` | 扩展名→MIME 类型映射表 |
| `ServerAddress` | `*string` | 外部访问地址 |

---

## 使用示例

### 命令行使用

```powershell
# 指定基础端口启动
.\Lunar_Astral.exe -basic-port 36800

# 开发模式（直接读取文件系统）
.\Lunar_Astral.exe -developer

# 禁用多模态模型
.\Lunar_Astral.exe -allow-multimodal=false

# 清理端口后启动
.\Lunar_Astral.exe -clear-port
```

### lunar_config.json 示例

```json
{
  "MultimodalModel": "D:/models/Qwen3-4B-Q4_K_M.gguf",
  "MmprojModel": "D:/models/mmproj-Qwen3-4B-F16.gguf",
  "EmbeddingModel": "D:/models/Qwen3-Embedding-0.6B-Q4_K_M.gguf",
  "DiffusionModel": "",
  "InferEngine": "D:/models/llama.cpp/llama-server.exe",
  "VisualEngine": "",
  "CloudModelUrl": ""
}
```

> **注意**：`lunar_config.json` 应放置在 `{LocalDir}/` 目录下（默认可执行文件同目录的 `local_data/`）。JSON 中的空字符串字段不会覆盖命令行默认值。

---

## 常见问题

### Q: lunar_config.json 放在哪里？

放置在 `{LocalDir}/` 目录下。默认情况下 `LocalDir` 值为 `local_data`，所以完整路径为 `<exe所在目录>/local_data/lunar_config.json`。

### Q: 哪些配置只能在命令行中设置？

端口号（`BasicPort`、`ModelPort` 等）、功能开关（`AllowDiffusion`、`AllowBrowser` 等）、图像参数（`MaxWidth`、`MaxHeight` 等）和 WebView 参数都只能通过命令行设置。`lunar_config.json` 仅覆盖模型路径和引擎路径。

### Q: 如何查看当前所有配置值？

开发模式下运行程序，启动日志会打印大部分配置参数。也可以在各模块的代码中通过 `fmt.Println(*config.BasicPort)` 自行打印。

### Q: 配置修改后需要重新编译吗？

不需要。`lunar_config.json` 在每次程序启动时重新读取，修改后重启程序即可生效。命令行参数也无需重新编译。

---

## 相关文档

- [项目主文档](../../README.md) —— 环境要求与编译流程
- [星图·月华](../../lunar_astral/README.md) —— 配置使用方
- [星图·琉璃](../../crystal_astral/README.md) —— 配置使用方
- [网页前端子系统](../browser/README.md) —— WebView 窗口参数使用