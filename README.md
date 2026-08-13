# 星月智能（Lunar Astral Agents）

## 项目简介

**星月智能**是一个纯本地化的桌面 AI 智能体平台，基于 **Go + TypeScript + C/C++** 构建，集成多模态对话、文本推理、图像生成、语音识别与语音合成等前沿 AI 能力。项目完全零 Python 依赖，所有推理均在本地完成，无需云端服务支持。

项目的灵魂由两位人格智能体驱动——**月华**与**琉璃**。月华是温柔的 AI 对话核心，她的智慧源于对 Qwen 大模型的蒸馏与量化，通过 GGUF 格式在本地轻量运行，为用户提供自然流畅的多模态交互体验。琉璃则是工具集的化身，以水晶般的澄澈纯粹承载文件管理、数据库操作、截图标注、AI 代理转发等实用功能。二者相辅相成，如同星与月，共同构筑起一个完整的本地 AI 工作生态。

技术上，星月智能依托 llama.cpp 进行文本推理、stable-diffusion.cpp 进行图像生成、Qwen3-TTS 与 Qwen3-ASR 分别实现语音合成与识别，前端采用 WebView2 嵌入玻璃拟态风格的 Web 界面，实现了从底层推理引擎到上层交互界面的全链路本地化。

---

## 人格智能体

- **月华** — 「月亮的光华」，AI 桌面智能体核心。她的智慧根植于对 Qwen 大模型的蒸馏与量化，掌管多模态对话与 TTS 语音表达。
- **琉璃** — 「如水晶般澄澈」，工具集扩展系统。她专注于工具的纯粹性与操作的直观性，掌管文件管理、知识库/记忆库、截图标注等实用工具集。

月华与琉璃如同星与月——月华以温柔智慧照亮对话空间，琉璃以澄澈纯粹夯实工具基石。

---

## 项目结构

```
Lunar_Astral_Agents/
├── lunar_astral/          # 核心系统：星图·月华（AI 桌面智能体）
├── crystal_astral/        # 扩展系统：星图·琉璃（工具集扩展）
├── subsystem/             # 公共子系统 + 独立 AI 引擎 + 运维工具
│   ├── general_config/     # 全局配置中枢
│   ├── browser_client/     # WebView 窗口管理
│   ├── file_manager/       # 文件管理 + 知识库/记忆库 + 扩展包
│   ├── image_processor/    # 图像生成 + 截图 + 视频关键帧
│   ├── qwen3_tts/          # 语音合成（C++ GGML 引擎）
│   ├── qwen_asr/           # 语音识别（纯 C 引擎）
│   ├── agent_search/       # 智能网络检索
│   └── environment_repair/ # 运维工具箱
├── local_data/            # 本地数据（模型文件 + 前端资源）
└── image/                 # 项目文档配图
```

> 详细架构参见 [ARCHITECTURE.md](ARCHITECTURE.md)。

---

## 环境要求

### 操作系统

| 系统 | 版本 | 架构 | 状态 |
|------|------|------|------|
| Windows 10 | 21H2 及以上 | x64 | ✅ 支持 |
| Windows 11 | 所有版本 | x64 | ✅ 支持 |
| 32 位 / Linux / macOS | 任意 | 任意 | ❌ 不支持 |

### 开发环境

| 工具 | 最低版本 | 用途 |
|------|---------|------|
| Go | ≥ 1.25.0 | Go 后端编译 |
| Node.js | ≥ 20.x | TypeScript 前端编译 |
| GCC (MinGW-w64) | ≥ 8.1.0 | C/C++ 编译（ASR/TTS） |
| CMake | ≥ 3.29.0 | C++ 项目构建（TTS） |

### 运行时依赖

| 依赖 | 版本 | 用途 |
|------|------|------|
| WebView2 Runtime | ≥ 109.0 | 桌面嵌入式浏览器（Win11 已预装） |
| CUDA Toolkit | 12.x/13.x | GPU 加速推理（可选） |
| FFmpeg | ≥ 5.0 | 音视频格式转换（可选） |
| Vulkan SDK | ≥ 1.3 | GPU 推理加速（可选） |

---

## 编译流程

### 一键编译全部

```powershell
cd d:\Lunar_Astral_Agents
.\build.ps1
```

### 单独编译

```powershell
# 核心系统——月华（含前端 TypeScript 编译）
cd d:\Lunar_Astral_Agents\lunar_astral
.\build.ps1

# 扩展系统——琉璃
cd d:\Lunar_Astral_Agents\crystal_astral
.\build.ps1

# 语音识别
cd d:\Lunar_Astral_Agents\subsystem\qwen_asr
.\build.ps1

# 语音合成（含 GGML + C++ 引擎 + Go 服务）
cd d:\Lunar_Astral_Agents\subsystem\qwen3_tts
.\build.ps1
```

各 `build.ps1` 均为自包含脚本，内部自动处理所有前置步骤。

### 编译产物

| 文件 | 系统 | 说明 |
|------|------|------|
| `Lunar_Astral.exe` | 星图·月华 | AI 桌面智能体主程序 |
| `Crystal_Astral.exe` | 星图·琉璃 | 工具集扩展程序 |
| `Qwen_ASR_Lunar.exe` | 语音识别 | 独立语音识别程序 |
| `Qwen3_TTS_Lunar.exe` | 语音合成 | 独立语音合成程序 |

---

## 系统架构

### 数据流

```
用户输入 → HTTP API → Go 服务层 → JS 智能体 (goja) → llama-server (GGUF 推理)
                                                    → TTS 引擎 (C++ GGML)
                                                    → SD 引擎 (C++ GGML)
         → WebSocket 推送 → 前端渲染 (Markdown/Mermaid/ECharts)
```

### 技术栈

| 层级 | 技术 |
|------|------|
| 前端 UI | HTML5 + CSS3 + JavaScript，WebView 嵌入 |
| AI 智能体 | TypeScript（goja 运行时） |
| 后端服务 | Go 1.25，HTTP API + WebSocket |
| 文本推理 | llama.cpp，GGUF 格式 |
| 图像生成 | stable-diffusion.cpp |
| 语音合成 | Qwen3-TTS，C++ GGML 引擎 |
| 语音识别 | Qwen3-ASR，纯 C 引擎 + OpenBLAS |
| 数据存储 | SQLite，本地嵌入式 |

---

## 子系统导航

| 子系统 | 文档 | 功能 |
|--------|------|------|
| 星图·月华 | [lunar_astral/README.md](lunar_astral/README.md) | AI 桌面智能体核心 |
| 星图·琉璃 | [crystal_astral/README.md](crystal_astral/README.md) | 工具集扩展系统 |
| 配置管理 | [subsystem/general_config/](subsystem/general_config/) | 全局配置中枢 |
| 文件管理 | [subsystem/file_manager/](subsystem/file_manager/) | 文件管理 + 知识库/记忆库 + 扩展包 |
| 图像处理 | [subsystem/image_processor/](subsystem/image_processor/README.md) | 图像生成 + 截图 + 视频关键帧 |
| 网络检索 | [subsystem/agent_search/](subsystem/agent_search/README.md) | AI 多引擎搜索智能体 |
| 语音合成 | [subsystem/qwen3_tts/](subsystem/qwen3_tts/README.md) | Qwen3-TTS |
| 语音识别 | [subsystem/qwen_asr/](subsystem/qwen_asr/README.md) | Qwen3-ASR |
| 项目架构 | [ARCHITECTURE.md](ARCHITECTURE.md) | 完整架构说明 |

---

## 常见问题

**需要 Python 吗？** 不需要。所有 AI 推理均由纯 C/C++ 或 Go 实现的本地引擎完成。

**可以离线使用吗？** 完全支持。所有模型为本地 GGUF 格式，推理无需网络。

**支持哪些 GPU？** 通过 llama.cpp 和 stable-diffusion.cpp 支持 NVIDIA CUDA GPU，Vulkan 后端也可用于兼容 GPU。

**如何添加 AI 模型？** 将 GGUF 模型放入 `local_data/models/`：语言模型（多模态/嵌入）编辑 `local_data/models/models.ini` 配置路径，ASR/扩散等辅助模型编辑 `lunar_config.json` 的 `models` 分组。修改后重启生效。

---

## 许可证

本项目仅限个人学习与研究使用，未经授权不得用于商业用途。