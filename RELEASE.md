# 星月智能（Lunar Astral Agents）—— 发行版说明

**版本**：v2026.05.26  
**发布日期**：2026-05-26  
**适用平台**：Windows 10/11（64 位）

---

## 项目简介

星月智能是一个**纯本地化**的桌面 AI 智能体平台，将多模态大语言模型、图像生成、语音识别与合成等多种 AI 能力整合于一体，无需任何 Python 环境或云端依赖，所有模型推理均在本地完成。

---

## 核心亮点

### 🔮 真·本地运行
所有 AI 能力均在本机执行，无需联网、无需注册、无需 API Key。数据完全私有，始终掌握在您手中。

### 🧠 多模态 AI 对话
基于 Qwen3 系列模型的角色扮演对话，支持文本、图片、视频等多媒体输入，具备上下文记忆与情绪感知能力。

### 🎭 Live2D 角色互动
集成 Live2D 渲染引擎，虚拟角色「月华」支持实时表情变化和动作展示，带来沉浸式交互体验。

### 🎙️ 语音全链路
- **语音识别**（Qwen3-ASR）：支持中英粤日韩等 30 种语言，纯 C 推理引擎，BF16 高精度
- **语音合成**（Qwen3-TTS）：支持文本转自然语音，可定制音色，C++ GGML 引擎驱动

### 🎨 图像生成
搭载 stable-diffusion.cpp 引擎，支持文生图与提示词精炼，输出高质量图像。

### 🪟 桌面原生体验
基于 WebView2 的嵌入式桌面窗口，无需浏览器即可流畅运行，支持窗口自定义与多端访问。

---

## 系统架构

```
┌────────────────────────────────────────────┐
│              星月智能平台                    │
│                                            │
│  ┌──────────────┐  ┌──────────────────┐    │
│  │  星图·月华     │  │  星图·琉璃         │   │
│  │  AI 桌面智能体  │  │  工具集扩展程序      │   │
│  │              │  │  · 文件管理         │   │
│  │  · AI 对话   │  │  · 数据库管理       │   │
│  │  · Live2D   │  │  · 截图标注         │   │
│  │  · TTS 语音  │  │  · AI 代理转发     │   │
│  │  · 图像生成  │  │  · 应用加载器       │   │
│  └──────┬───────┘  └────────┬─────────┘   │
│         │                   │              │
│         └───────┬───────────┘              │
│                 │                          │
│  ┌──────────────┼──────────────────┐       │
│  │        公共子系统层              │       │
│  │  配置 · WebView · 存储 · 截图    │       │
│  └──────────────┼──────────────────┘       │
│                 │                          │
│  ┌──────────────┼──────────────────┐       │
│  │        推理引擎层                │       │
│  │  llama.cpp · stable-diffusion    │       │
│  │  Qwen3-TTS · Qwen3-ASR          │       │
│  └─────────────────────────────────┘       │
└────────────────────────────────────────────┘
```

---

## 功能一览

| 模块 | 功能 | 说明 |
|------|------|------|
| **星图·月华** | AI 角色对话 | 基于 Qwen3 的多模态智能体，支持角色扮演 |
| | Live2D 展示 | 实时角色渲染，表情与动作联动 |
| | 语音合成 | 月华语音输出，支持音色定制 |
| | 图像生成 | 文生图 + 提示词优化 |
| | 富文本渲染 | Markdown / Mermaid / ECharts / KaTeX |
| | QQ 适配器 | 接入 QQ 群聊，@ 即可唤醒 |
| **星图·琉璃** | 文件管理 | 本地文件浏览、上传、下载、编辑 |
| | 数据库管理 | SQLite 可视化 CRUD，批量操作 |
| | 屏幕截图 | 多显示器截图、区域截图、标注 |
| | AI 代理转发 | 代理 OpenAI 格式 API，接入外部模型 |
| | 应用加载器 | 一键启动外部 .exe / .ps1 / .bat |
| **独立模块** | 语音识别 | Qwen3-ASR-0.6B/1.7B，30 种语言 |
| | 语音合成 | Qwen3-TTS-0.6B，中文语音合成 |

---

## 运行环境

### 操作系统
- ✅ Windows 10 21H2+（64 位）
- ✅ Windows 11（64 位）
- ❌ 32 位系统不支持
- ❌ Linux / macOS 不支持

### 硬件建议

| 配置项 | 最低要求 | 推荐配置 |
|--------|---------|---------|
| CPU | x86_64，支持 AVX2 | 4 核以上 |
| 内存 | 8 GB | 16 GB |
| 显卡 | 无要求（纯 CPU 可运行） | NVIDIA CUDA 12.x（显存 4GB+） |
| 磁盘 | 10 GB 可用空间 | SSD 20 GB+ |

### 运行时依赖

| 依赖 | 版本 | 下载 |
|------|------|------|
| WebView2 Runtime | ≥ 109.0 | [微软官网](https://developer.microsoft.com/en-us/microsoft-edge/webview2/)（Win11 已预装） |
| NVIDIA CUDA | 12.x 或 13.x（可选） | [NVIDIA 官网](https://developer.nvidia.com/cuda-downloads) |
| FFmpeg | ≥ 5.0（可选） | [ffmpeg.org](https://ffmpeg.org/download.html) |

---

## 快速开始

### 第一步：解压发行包

将下载的压缩包解压到**不包含中文和空格**的路径下，例如 `D:\Lunar_Astral\`。

### 第二步：配置模型

将 GGUF 格式的模型文件放入 `local_data\models\` 目录，编辑 `local_data\lunar_config.json` 配置模型路径。

支持模型：

| 模型类型 | 推荐模型 | 格式 |
|---------|---------|------|
| 多模态对话 | Qwen3-1.7B / Qwen3-4B | GGUF |
| 视觉投影 | mmproj-Qwen3-F16 | GGUF |
| 语音识别 | Qwen3-ASR-0.6B / 1.7B | SafeTensors |
| 语音合成 | qwen3-tts-0.6b | GGUF |
| 图像生成 | z_image_turbo | GGUF |

### 第三步：启动

```powershell
# 启动核心智能体（月华）
.\Lunar_Astral.exe

# 启动扩展工具集（琉璃）
.\Crystal_Astral.exe

# 启动语音识别（独立模块）
.\Qwen_ASR_Lunar.exe

# 启动语音合成（独立模块）
.\Qwen3_TTS_Lunar.exe
```

首次启动后程序自动打开 WebView 桌面窗口，即可开始使用。

### 命令行参数

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `-basic-port` | HTTP 服务端口 | 36789 |
| `-developer` | 开发模式（直读文件系统） | false |
| `-clear-port` | 启动前清理端口 | false |
| `-allow-diffusion` | 启用图像生成 | true |
| `-allow-multimodal` | 启用多模态模型 | true |

---

## 发行包内容

本发行版（`plan-3` 完整包）包含以下文件：

```
Lunar_Astral_Agents-v2026.05.26/
│
├── Lunar_Astral.exe          ← 核心智能体主程序
├── Crystal_Astral.exe        ← 扩展工具集主程序
├── Qwen_ASR_Lunar.exe        ← 语音识别独立程序
├── Qwen3_TTS_Lunar.exe       ← 语音合成独立程序
│
├── qwen3tts.dll              ← TTS 推理引擎 DLL
├── libdl.dll                 ← 运行时依赖库
├── libgcc_s_seh-1.dll        ← GCC 运行时
├── libgomp-1.dll             ← OpenMP 运行时
├── libstdc++-6.dll           ← C++ 标准库
├── libwinpthread-1.dll       ← POSIX 线程库
├── vulkan-1.dll              ← Vulkan 运行时
│
├── local_data/               ← 本地数据目录
│   ├── lunar_config.json     ← 主配置文件
│   ├── lunar_package.json    ← 前端包配置
│   ├── package/              ← 前端渲染库
│   ├── audios/               ← 参考音频
│   ├── images/               ← 图片资源
│   └── embed_cache/          ← 嵌入缓存
│
├── crystal_astral/           ← 琉璃源码（嵌入式资源）
├── lunar_astral/             ← 月华源码（嵌入式资源）
└── subsystem/                ← 公共子系统源码
```

---

## 配置说明

`lunar_config.json` 核心配置项：

```json
{
  "models": {
    "asr_model": "./local_data/models/Qwen3-ASR-0.6B",
    "diffusion_model": "./local_data/models/z_image_turbo-Q4_K.gguf",
    "variational_model": "./local_data/models/diffusion_pytorch_model.safetensors",
    "prompt_refine_model": "./local_data/models/Qwen3-4B-Instruct-Q4_K_M.gguf"
  },
  "server": {
    "developer": false,
    "allow_diffusion": true,
    "allow_multimodal": true
  },
  "cloud": {
    "cloud_model_url": "",
    "multimodal_model_name": ""
  }
}
```

| 配置项 | 说明 |
|--------|------|
| `models.*` | 各 AI 模型文件路径（GGUF/SafeTensors 格式） |
| `server.developer` | `true` 启用开发模式（直接读文件系统，不读 embed） |
| `server.allow_diffusion` | `false` 禁用图像生成功能 |
| `server.allow_multimodal` | `false` 禁用多模态（仅文本对话） |
| `cloud.cloud_model_url` | 云端模型 API（留空则纯本地运行） |

---

## QQ 群聊适配器

星月智能内置了 **NapCat QQ 适配器**，可以让月华接入 QQ 群聊。

### 配置方法

1. 部署 [NapCatQQ](https://github.com/NapNeko/NapCatQQ) 并启动 WebSocket 服务
2. 在 `lunar_config.json` 中配置：

```json
"qq_adapter": {
  "napcat_ws_server": "ws://localhost:4567",
  "napcat_ws_token": "your_token_here",
  "listen_group_ids": ["群号1", "群号2"],
  "trigger_keywords": ["月华", "@机器人"],
  "poll_interval": 10
}
```

### 使用方式

在配置的 QQ 群中发送包含关键词（如 `@月华`）的消息即可唤醒 AI 对话。

| 配置项 | 说明 |
|--------|------|
| `napcat_ws_server` | NapCat WebSocket 服务地址 |
| `listen_group_ids` | 监听的 QQ 群号列表 |
| `trigger_keywords` | 触发词列表（消息包含任一即唤醒） |
| `poll_interval` | 消息轮询间隔（秒） |
| `default_reply` | 无法回答时的默认回复 |

---

## 编译指南

如果您需要从源码自行编译，请参考以下步骤。

### 开发环境

| 工具 | 版本 | 用途 |
|------|------|------|
| Go | ≥ 1.25.0 | 后端编译 |
| Node.js | ≥ 20.x | 前端编译 |
| GCC (MinGW-w64) | ≥ 8.1.0 | C/C++ 编译 |
| CMake | ≥ 3.29.0 | C++ 项目构建 |

### 编译命令

#### 一键编译全部（推荐）

```powershell
cd d:\Lunar_Astral_Agents
.\build.ps1
```

根目录的 `build.ps1` 是统一构建入口，自动检查环境（Go / Node.js / npm / GCC / rsrc）后按顺序编译所有子系统。每个子系统的 `build.ps1` 均为自包含脚本，内部已处理所有前置步骤。

#### 单独编译某个子系统

```powershell
# 编译月华核心系统（含前端 TypeScript 编译）
cd lunar_astral
.\build.ps1

# 编译琉璃扩展系统
cd crystal_astral
.\build.ps1

# 编译语音识别
cd subsystem\qwen_asr_lunar
.\build.ps1

# 编译语音合成（含 GGML + C++ 引擎 + Go 服务）
cd subsystem\qwen3_tts_lunar
.\build.ps1
```

编译产物见[发行包内容](#发行包内容)。

---

## 常见问题

### 启动失败怎么办？

1. 确认 WebView2 Runtime 已安装（Win11 预装，Win10 需手动安装）
2. 确认没有中文路径
3. 检查 `local_data\lunar_config.json` 配置是否正确
4. 查看控制台输出的错误日志

### 如何添加新的 AI 模型？

将 GGUF 格式模型放入 `local_data\models\`，在 `lunar_config.json` 中配置路径后重启。

### 内存不足怎么办？

- 使用量化模型（Q4_K_M 等，而非 F16）
- 关闭不需要的功能（`allow_diffusion: false`）
- 使用较小模型（1.7B 替代 4B）

### 支持云端模型吗？

支持。在 `lunar_config.json` 中配置 `cloud.cloud_model_url` 为兼容 OpenAI 格式的 API 地址（如阿里云 DashScope、LM Studio 等），系统将自动切换到云端推理。

---

## 许可证

本项目**仅供个人学习与研究使用**，未经授权不得用于商业用途。

AI 模型权属归相应模型提供方所有。使用前请确认遵守对应模型的开源协议。

---

## 致谢

本项目使用了以下开源项目，特此致谢：

| 项目 | 用途 | 许可 |
|------|------|------|
| [llama.cpp](https://github.com/ggerganov/llama.cpp) | GGUF 模型推理引擎 | MIT |
| [stable-diffusion.cpp](https://github.com/leejet/stable-diffusion.cpp) | 扩散模型推理 | MIT |
| [qwen3-tts.cpp](https://github.com/predict-woo/qwen3-tts.cpp) | 文本转语音模型 | MIT |
| [qwen3-asr](https://github.com/antirez/qwen-asr) | 语音识别模型 | MIT |
| [GGML](https://github.com/ggerganov/ggml) | 张量计算库 | MIT |
| [WebView2](https://developer.microsoft.com/en-us/microsoft-edge/webview2/) | 嵌入式浏览器 | Microsoft |
| [Live2D](https://www.live2d.com/) | 角色渲染引擎 | Live2D |

---

> 🌟 月华和琉璃永远陪伴着大家 ~ 一起探索 AI 的无限可能吧！