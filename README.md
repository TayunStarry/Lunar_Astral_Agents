# 星月智能（Lunar Astral Agents）

基于 **Go + TypeScript + C/C++** 的纯本地化桌面 AI 智能体平台，集成文本推理、图像生成、语音识别、语音合成等功能，采用纯客户端部署方案，无需任何 Python 环境。

---

## 人格智能体

星月智能平台承载了两位具有独特个性与智慧体系的 AI 人格智能体：

### 月华 — 月亮的光华

**月华**之名，寓意为「月亮的光华」——她的智慧如同月光般温柔而普照大地。

月华的智慧体系根植于一个富有诗意的隐喻：如同现实中月光本是对太阳光芒的温柔反射，月华的智能体亦是对「伟大之物」——全量参数无量化的 Qwen 大模型——的蒸馏与量化。通过知识蒸馏技术将大模型的智慧浓缩，再经量化压缩使其能在本地轻量运行，恰如月光将炽烈的太阳光芒化为柔和银辉，洒向千家万户。

月华是平台的**核心灵魂**，掌管 AI 角色对话、Live2D 角色展示与 TTS 语音表达，以温柔知性的语言风格与用户交互，兼具感性关怀与理性思辨。

### 琉璃 — 如水晶般澄澈

**琉璃**之名，寓意为「如水晶般澄澈」——她代表着透明、轻盈与纯粹的本真。

琉璃的性格如同水晶一般清透明澈、轻盈灵动，专注于工具的纯粹性与操作的直观性。她不做无谓的修饰，而是以最简洁直接的方式完成每一项任务。琉璃是平台的**扩展工具集**的化身，掌管文件管理、数据库操作、截图标注、AI 代理转发等实用工具集，以高效精准的操作风格服务于系统的底层能力需求。

月华与琉璃两位人格智能体如同星与月——月华以温柔智慧照亮对话空间，琉璃以澄澈纯粹夯实工具基石，二者相辅相成，共同构建起星月智能平台的完整智能生态。

---

## 目录

- [人格智能体](#人格智能体)
- [项目结构](#项目结构)
- [环境要求](#环境要求)
- [编译流程](#编译流程)
- [系统架构](#系统架构)
- [子系统导航](#子系统导航)
- [常见问题](#常见问题)

---

## 项目结构

> 完整的目录树与逐目录职责说明请参见 **[ARCHITECTURE.md](ARCHITECTURE.md)**。

### 层级关系说明

<div style="font-family: 'Cascadia Code', 'SF Mono', Consolas, monospace; font-size: 0.9em; line-height: 1.6;">
  <ul style="list-style-type: none; padding-left: 0;">
    <li><strong>星月智能平台 (Lunar Astral Agents)</strong></li>
    <li style="padding-left: 1.5em;"><strong>核心系统: 星图·月华</strong> <span style="color: #6a737d;">(lunar_astral)</span>
      <ul style="list-style-type: none; padding-left: 1.5em;">
        <li><span style="color: #6a737d;">依赖: config, browser, storage, screenshot, qwen3_tts_lunar</span></li>
        <li><span style="color: #6a737d;">功能: AI 对话、Live2D 角色、TTS 语音、图像生成</span></li>
        <li><span style="color: #6a737d;">入口: <code>Lunar_Astral.exe</code></span></li>
      </ul>
    </li>
    <li style="padding-left: 1.5em;"><strong>扩展系统: 星图·琉璃</strong> <span style="color: #6a737d;">(crystal_astral)</span>
      <ul style="list-style-type: none; padding-left: 1.5em;">
        <li><span style="color: #6a737d;">依赖: config, browser, storage, screenshot</span></li>
        <li><span style="color: #6a737d;">功能: 文件管理、数据库管理、截图标注、AI 代理</span></li>
        <li><span style="color: #6a737d;">入口: <code>Crystal_Astral.exe</code></span></li>
      </ul>
    </li>
    <li style="padding-left: 1.5em;"><strong>独立系统: 语音合成</strong> <span style="color: #6a737d;">(qwen3_tts_lunar)</span>
      <ul style="list-style-type: none; padding-left: 1.5em;">
        <li><span style="color: #6a737d;">依赖: C++ GGML 引擎</span></li>
        <li><span style="color: #6a737d;">功能: Qwen3-TTS 文本转语音</span></li>
        <li><span style="color: #6a737d;">入口: <code>Qwen3_TTS_Lunar.exe</code></span></li>
      </ul>
    </li>
    <li style="padding-left: 1.5em;"><strong>独立系统: 语音识别</strong> <span style="color: #6a737d;">(qwen_asr_lunar)</span>
      <ul style="list-style-type: none; padding-left: 1.5em;">
        <li><span style="color: #6a737d;">依赖: 纯 C 引擎 + OpenBLAS</span></li>
        <li><span style="color: #6a737d;">功能: Qwen3-ASR 语音转文本</span></li>
        <li><span style="color: #6a737d;">入口: <code>Qwen_ASR_Lunar.exe</code></span></li>
      </ul>
    </li>
    <li style="padding-left: 1.5em;"><strong>公共子系统</strong> <span style="color: #6a737d;">(subsystem/)</span>
      <ul style="list-style-type: none; padding-left: 1.5em;">
        <li><code>config</code> <span style="color: #6a737d;">— 全局配置中枢</span></li>
        <li><code>browser</code> <span style="color: #6a737d;">— WebView 窗口 + 本地 IP 发现</span></li>
        <li><code>storage</code> <span style="color: #6a737d;">— 文件存储 + SQLite 数据库</span></li>
        <li><code>screenshot</code> <span style="color: #6a737d;">— 屏幕截图 + 图片缩放</span></li>
        <li><code>image</code> <span style="color: #6a737d;">— 图像生成 + 视频关键帧提取</span></li>
        <li><code>logger</code> <span style="color: #6a737d;">— 彩色终端日志</span></li>
      </ul>
    </li>
    <li style="padding-left: 1.5em;"><strong>扩展子系统</strong> <span style="color: #6a737d;">(subsystem/)</span>
      <ul style="list-style-type: none; padding-left: 1.5em;">
        <li><code>LunarTick</code> <span style="color: #6a737d;">— 通用程序执行引擎（tick 驱动）</span></li>
        <li><code>bridge_adapter</code> <span style="color: #6a737d;">— QQ 群聊适配器（NapCat ↔ 月华）</span></li>
        <li><code>gguf_metadata_viewer</code> <span style="color: #6a737d;">— GGUF 模型元数据查看器</span></li>
        <li><code>proxy</code> <span style="color: #6a737d;">— HTTPS 代理服务器</span></li>
        <li><code>sd_lunar</code> <span style="color: #6a737d;">— Stable Diffusion 图像生成引擎</span></li>
        <li><code>volume_archive</code> <span style="color: #6a737d;">— 卷归档管理</span></li>
        <li><code>websearch</code> <span style="color: #6a737d;">— 智能网络检索（三级搜索策略）</span></li>
      </ul>
    </li>
  </ul>
</div>

---

## 环境要求

### 操作系统支持

| 系统 | 版本 | 架构 | 状态 |
|------|------|------|------|
| Windows 10 | 21H2 及以上 | x64 | 支持 |
| Windows 11 | 所有版本 | x64 | 支持 |
| Windows 10/11 | 32 位 | x86 | 不支持 |
| Linux | 任意版本 | 任意 | 不支持 |
| macOS | 任意版本 | 任意 | 不支持 |

### 开发环境

| 工具 | 最低版本 | 用途 | 安装指南 |
|------|---------|------|---------|
| Go | >= 1.25.0 | Go 后端编译 | [go.dev/dl](https://go.dev/dl/) |
| Node.js | >= 20.x | TypeScript 前端编译 | [nodejs.org](https://nodejs.org/en/download/) |
| GCC (MinGW-w64) | >= 8.1.0 | C/C++ 编译（ASR/TTS） | [mingw-w64.org](https://www.mingw.org/mingw64) |
| CMake | >= 3.29.0 | C++ 项目构建（TTS） | [cmake.org](https://cmake.org/download/) |

#### 验证方法

```powershell
# Go 版本验证
go version
# 期望输出: go version go1.25.x windows/amd64

# Node.js 版本验证
node --version
# 期望输出: v20.x.x 或更高

# GCC 版本验证
gcc --version
# 期望输出: gcc (MinGW-W64 ...) 8.1.0 或更高

# CMake 版本验证
cmake --version
# 期望输出: cmake version 3.29.0 或更高
```

### 运行时依赖

| 依赖项 | 版本要求 | 用途 | 下载链接 |
|--------|---------|------|---------|
| CUDA Toolkit | 12.x 或 13.x | GPU 加速推理 | [developer.nvidia.com/cuda-downloads](https://developer.nvidia.com/cuda-downloads) |
| NVIDIA CUDA 驱动 | 与 CUDA 版本匹配 | GPU 驱动支持 | [developer.nvidia.com/cuda-downloads](https://developer.nvidia.com/cuda-downloads) |
| WebView2 Runtime | >= 109.0 | 桌面嵌入式浏览器 | [developer.microsoft.com/en-us/microsoft-edge/webview2/](https://developer.microsoft.com/en-us/microsoft-edge/webview2/) |
| FFmpeg | >= 5.0.0 | 音频/视频格式转换 | [ffmpeg.org/download.html](https://ffmpeg.org/download.html) |
| Vulkan SDK | >= 1.3 | GPU 推理加速 | [lunarg.com/sdk-downloads/vulkan-sdk](https://www.lunarg.com/sdk-downloads/vulkan-sdk) |

#### 验证方法

```powershell
# CUDA 验证
nvidia-smi
# 期望输出: 显示 CUDA 版本 12.x 或 13.x

# WebView2 验证
reg query "HKLM\SOFTWARE\WOW6432Node\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}" /v pv 2>nul
# 有输出表示已安装

# FFmpeg 验证
ffmpeg -version
# 期望输出: ffmpeg version 5.0.0 或更高

# Vulkan 验证
vulkaninfo --summary
# 期望输出: 显示 Vulkan Instance Version 1.3 或更高
```

> **注意**：FFmpeg 需要添加到系统环境变量 `PATH` 中，或在 `lunar_config.json` 中配置自定义路径。

---

## 编译流程

### 前置准备

1. 确保已安装所有[开发环境](#开发环境)中的工具
2. 确保已安装所有[运行时依赖](#运行时依赖)
3. 将本仓库克隆到本地（避免路径中包含中文或空格）

### 编译步骤

#### 方式一：一键编译全部（推荐）

```powershell
cd d:\Lunar_Astral_Agents
.\build.ps1
```

根目录的 `build.ps1` 是**统一构建入口**，自动完成环境检查后按顺序编译所有子系统：
月华 琉璃 桥接适配器 卷归档 语音合成。每个子系统的 `build.ps1` 均为自包含脚本，内部已处理所有前置步骤（前端编译、GGML 库构建、C++ 引擎编译等）。

#### 方式二：单独编译某个子系统

```powershell
# 编译核心系统——月华（含前端 TypeScript 编译）
cd d:\Lunar_Astral_Agents\lunar_astral
.\build.ps1

# 编译扩展系统——琉璃
cd d:\Lunar_Astral_Agents\crystal_astral
.\build.ps1

# 编译语音识别
cd d:\Lunar_Astral_Agents\subsystem\qwen_asr_lunar
.\build.ps1

# 编译语音合成（含 GGML + C++ 引擎 + Go 服务）
cd d:\Lunar_Astral_Agents\subsystem\qwen3_tts_lunar
.\build.ps1
```

> 各子系统的 `build.ps1` 均为自包含脚本，无需手动执行 `npm install`、`build_ggml.ps1`、`build_cpp.ps1` 等前置步骤，它们已在脚本内部自动处理。

### 编译输出

所有编译产物默认输出到项目根目录：

| 文件 | 所属系统 | 说明 |
|------|---------|------|
| `Lunar_Astral.exe` | 星图·月华 | AI 桌面智能体主程序 |
| `Crystal_Astral.exe` | 星图·琉璃 | 工具集扩展程序 |
| `Qwen_ASR_Lunar.exe` | 语音识别 | 独立语音识别程序 |
| `Qwen3_TTS_Lunar.exe` | 语音合成 | 独立语音合成程序 |

### 编译参数说明

各模块的编译参数已内置于各自的 `build.ps1` 脚本中，无需手动设置。常见编译标志如下（供高级用户参考）：

| 参数 | 适用模块 | 说明 |
|------|---------|------|
| `CGO_ENABLED=1` | ASR、TTS、月华、琉璃 | 启用 CGO（调用 C/C++ 推理引擎） |
| `GOARCH=amd64` | 全部 | 指定目标架构为 64 位 |
| `-tags webview` | 月华、琉璃 | 启用 WebView 桌面窗口支持 |
| `-ldflags="-s -w"` | 全部 | 去除调试符号和 DWARF 信息以减小体积 |
| `-O3 -march=native` | ASR | GCC 最高优化 + 本机指令集 |
| `-DUSE_BLAS` | ASR（可选） | 启用 OpenBLAS 加速 |

### 编译验证

```powershell
# 检查编译产物是否存在
Test-Path d:\Lunar_Astral_Agents\Lunar_Astral.exe   # 应返回 True
Test-Path d:\Lunar_Astral_Agents\Crystal_Astral.exe # 应返回 True
Test-Path d:\Lunar_Astral_Agents\Qwen_ASR_Lunar.exe  # 应返回 True

# 检查文件大小（应大于 10MB）
Get-Item d:\Lunar_Astral_Agents\Lunar_Astral.exe | Select-Object Length
```

### 常见编译错误

| 错误信息 | 原因 | 解决方案 |
|---------|------|---------|
| `go: go.mod file indicates go 1.25, but maximum version is 1.xx` | Go 版本过低 | 升级 Go 到 1.25 或更高版本 |
| `cannot find module for path config` | 未设置 Go workspace | 在项目根目录执行 `go work init` 并添加各个模块 |
| `gcc: command not found` | GCC 未安装 | 安装 MinGW-w64 并确保 `gcc` 在 PATH 中 |
| `CMake Error: Could not find cmake version 3.29` | CMake 版本过低 | 升级 CMake 到 3.29 以上 |
| `undefined reference to cblas_sgemm` | OpenBLAS 未正确链接 | 检查 OpenBLAS 头文件和库文件路径 |
| `CGO_ENABLED=0` 时 C 代码编译失败 | CGO 未启用 | 确保执行 `$env:CGO_ENABLED=1` |

---

## 系统架构

### 整体架构图

```
+-------------------------------------------------------------+
|                      星月智能平台                             |
|                                                              |
|  +-----------------+  +-----------------+                   |
|  |  星图·月华        |  |  星图·琉璃        |                  |
|  |  (AI 桌面智能体)   |  |  (工具集扩展程序)  |                 |
|  |                  |  |                  |                  |
|  |  . AI 对话角色   |  |  . 文件管理      |                  |
|  |  . Live2D 展示   |  |  . 数据库管理    |                  |
|  |  . TTS 语音合成  |  |  . 截图标注      |                  |
|  |  . 图像生成     |  |  . AI 代理转发   |                  |
|  |  . WebSocket    |  |  . 应用加载器    |                  |
|  +--------+--------+  +--------+--------+                   |
|           |                    |                             |
|           +----------+---------+                             |
|                    |                                         |
|  +-----------------+-----------------------------+          |
|  |          公共子系统 (subsystem)                 |          |
|  |                                                |          |
|  |  +----------+ +---------+ +----------+        |          |
|  |  | config   | | browser | | storage  |        |          |
|  |  | 配置管理  | | 网页前端 | | 文件管理  |        |          |
|  |  +----------+ +---------+ +----------+        |          |
|  |  +----------+ +------------------+            |          |
|  |  |screenshot| | 独立 AI 引擎      |            |          |
|  |  |屏幕截图   | | TTS . ASR        |            |          |
|  |  +----------+ +------------------+            |          |
|  +-----------------------------------------------+          |
|                                                              |
|  +--------------------------------------------------+      |
|  |              外部推理引擎                          |      |
|  |  +--------------+ +--------------+                |      |
|  |  | llama.cpp    | | stable-      |                |      |
|  |  | (GGUF 推理)   | | diffusion.cpp|               |      |
|  |  +--------------+ +--------------+                |      |
|  +--------------------------------------------------+      |
+-------------------------------------------------------------+
```

### 数据流概要

```
用户输入 (前端界面)
    |
    +-- HTTP API -- llm_proxy -- llama-server.exe -- GGUF 模型推理
    |                                          |
    |                                    推理结果返回
    |                                          |
    +-- JS 智能体 (goja 运行时) -- 角色逻辑处理 -- 生成回复
    |                                          |
    +-- TTS 引擎 -- WAV 音频合成
    |
    +-- WebSocket 推送 -- 前端实时渲染 (Markdown/Mermaid/ECharts/Live2D)
```

### 技术栈总览

| 层级 | 技术 | 说明 |
|------|------|------|
| 前端 UI | HTML5 + CSS3 + JavaScript | 嵌入式 WebView 桌面界面 |
| AI 智能体 | TypeScript (goja 运行时) | 在 Go 进程中运行的 JS 智能体 |
| 后端服务 | Go 1.25 | HTTP API + WebSocket + 业务逻辑 |
| 图像生成 | stable-diffusion.cpp | 外部 SD 推理引擎 |
| 文本推理 | llama.cpp (llama-server) | 外部 GGUF 模型推理 |
| 语音合成 | C++ GGML 引擎 | Qwen3-TTS 模型推理 |
| 语音识别 | 纯 C 引擎 + OpenBLAS | Qwen3-ASR 模型推理 |
| 数据存储 | SQLite (go-sqlite3) | 本地嵌入式数据库 |

---

## 子系统导航

| 子系统 | 文档链接 | 功能概要 |
|--------|---------|---------|
| 星图·月华 | [lunar_astral/README.md](lunar_astral/README.md) | AI 桌面智能体核心系统，含对话角色、Live2D、TTS |
| 星图·琉璃 | [crystal_astral/README.md](crystal_astral/README.md) | 工具集扩展系统，含文件/数据库管理、截图标注 |
| 配置管理 | [subsystem/config/README.md](subsystem/config/README.md) | 全局配置中枢，命令行参数 + JSON 双层配置 |
| 网页前端 | [subsystem/browser/README.md](subsystem/browser/README.md) | WebView 窗口管理 + 本地 IP 自动发现 |
| 文件管理 | [subsystem/storage/README.md](subsystem/storage/README.md) | 文件 CRUD + SQLite 数据库 + ZIP 归档 |
| 屏幕截图 | [subsystem/screenshot/README.md](subsystem/screenshot/README.md) | 多显示器截图 + 区域截图 + 图片缩放 |
| 图像处理 | [subsystem/image/README.md](subsystem/image/README.md) | 扩散图像生成 + 视频关键帧提取 |
| 语音合成 | [subsystem/qwen3_tts_lunar/README.md](subsystem/qwen3_tts_lunar/README.md) | Qwen3-TTS 文本转语音引擎 |
| 语音识别 | [subsystem/qwen_asr_lunar/README.md](subsystem/qwen_asr_lunar/README.md) | Qwen3-ASR 语音转文本引擎 |
| 月球节拍 | [subsystem/LunarTick/README.md](subsystem/LunarTick/README.md) | tick 驱动的通用程序执行引擎 |
| QQ 适配器 | [subsystem/bridge_adapter/DEVELOPMENT_GUIDE.md](subsystem/bridge_adapter/DEVELOPMENT_GUIDE.md) | NapCat <-> 月华 QQ 群聊消息转发 |
| GGUF 查看器 | [subsystem/gguf_metadata_viewer/README.md](subsystem/gguf_metadata_viewer/README.md) | GGUF 模型文件元数据查看工具 |
| HTTPS 代理 | [subsystem/proxy/](subsystem/proxy/) | HTTPS 代理服务器 + 证书管理 |
| SD 图像生成 | [subsystem/sd_lunar/](subsystem/sd_lunar/) | Stable Diffusion C++ GGML 推理引擎 |
| 卷归档 | [subsystem/volume_archive/](subsystem/volume_archive/) | 卷归档管理工具 |
| 项目架构 | [ARCHITECTURE.md](ARCHITECTURE.md) | 项目架构说明（文件夹层级 + 功能描述） |

---

## 常见问题

### Q: 项目需要 Python 环境吗？

不需要。本项目的设计理念是「零 Python 依赖」。所有 AI 模型推理均由纯 C/C++ 或 Go 实现的本地引擎完成。

### Q: 可以离线使用吗？

完全支持离线使用。所有模型文件均为本地 GGUF 格式，推理过程不需要网络连接。

### Q: 支持哪些 GPU？

通过 llama.cpp 和 stable-diffusion.cpp 支持 NVIDIA CUDA GPU。Vulkan 后端也可用于兼容的 GPU。

### Q: 前端如何修改？

月华系统的前端位于 `lunar_astral/hierarchy/assets/client/` 和 `lunar_astral/server_side/`，修改后重新执行 `lunar_astral\build.ps1` 即可（脚本内部自动处理 TypeScript 编译与打包）。

### Q: 如何添加新的 AI 模型？

将 GGUF 格式的模型文件放入 `{LocalDir}/models/` 目录，并在 `lunar_config.json` 中配置模型路径即可。

---

## 许可证

本项目仅限个人学习与研究使用，未经授权不得用于商业用途。