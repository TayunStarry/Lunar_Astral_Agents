# 独立系统——图像生成（sd_lunar）

基于 Stable Diffusion 的本地图像生成引擎，采用 C++ GGML 推理后端 + Go HTTP 服务的混合架构，支持 Vulkan GPU 加速。

---

## 目录

- [功能概述](#功能概述)
- [项目结构](#项目结构)
- [核心架构](#核心架构)
- [核心模块说明](#核心模块说明)
- [编译与运行](#编译与运行)
- [常见问题](#常见问题)

---

## 功能概述

SD Lunar 是一个全本地化的 Stable Diffusion 图像生成引擎，为星月智能平台提供文生图能力。

| 特性 | 说明 |
|------|------|
| 文生图 | 根据文本提示词生成图像 |
| GPU 加速 | 通过 Vulkan 后端加速推理，支持多种 GPU 厂商 |
| 本地运行 | 纯 C++/Go 实现，无需 Python 环境 |
| 嵌入式界面 | Go 内嵌 Web UI，WebView 桌面窗口 |
| 多后端支持 | GGML 支持 CUDA、Vulkan、Metal、SYCL 等加速后端 |

---

## 项目结构

<div style="font-family: 'Cascadia Code', 'SF Mono', Consolas, monospace; font-size: 0.9em; line-height: 1.6;">
  <ul style="list-style-type: none; padding-left: 0;">
    <li><strong>sd_lunar/</strong></li>
    <li style="padding-left: 1.5em;"><code>build.ps1</code> <span style="color: #6a737d;">— 一站式构建脚本（GGML + C++ + Go 三阶段）</span></li>
    <li style="padding-left: 1.5em;"><code>build_ggml.ps1</code> <span style="color: #6a737d;">— GGML 张量计算库预编译脚本</span></li>
    <li style="padding-left: 1.5em;">
      <strong>assets/</strong> <span style="color: #6a737d;">— 前端界面</span>
      <ul style="list-style-type: none; padding-left: 1.5em;">
        <li><code>index.html</code> <span style="color: #6a737d;">— 主页面（玻璃拟态风格）</span></li>
        <li><code>script.js</code> <span style="color: #6a737d;">— 前端逻辑（参数配置、图像预览）</span></li>
        <li><code>style.css</code> <span style="color: #6a737d;">— 样式表</span></li>
      </ul>
    </li>
    <li style="padding-left: 1.5em;">
      <strong>cpp/</strong> <span style="color: #6a737d;">— C++ 推理引擎</span>
      <ul style="list-style-type: none; padding-left: 1.5em;">
        <li><code>CMakeLists.txt</code> <span style="color: #6a737d;">— CMake 构建配置（C++17, Vulkan 后端, 预编译 GGML）</span></li>
        <li>
          <strong>ggml/</strong> <span style="color: #6a737d;">— GGML 张量计算库（子模块）</span>
          <ul style="list-style-type: none; padding-left: 1.5em;">
            <li><strong>include/</strong> <span style="color: #6a737d;">— 头文件（ggml.h, gguf.h, ggml-cuda.h, ggml-vulkan.h 等）</span></li>
            <li><strong>src/</strong> <span style="color: #6a737d;">— GGML 核心源码 + 多后端（CUDA/Vulkan/Metal/SYCL/OpenCL/BLAS 等）</span></li>
            <li><strong>cmake/</strong> <span style="color: #6a737d;">— CMake 辅助脚本</span></li>
            <li><strong>scripts/</strong> <span style="color: #6a737d;">— 发布与同步脚本</span></li>
            <li><code>CMakeLists.txt</code> <span style="color: #6a737d;">— GGML 顶层 CMake 配置</span></li>
          </ul>
        </li>
        <li>
          <strong>src/</strong> <span style="color: #6a737d;">— Stable Diffusion 引擎源码</span>
          <ul style="list-style-type: none; padding-left: 1.5em;">
            <li><code>main.cpp</code> <span style="color: #6a737d;">— CLI 可执行文件入口（sd-cli）</span></li>
            <li><strong>model_io/</strong> <span style="color: #6a737d;">— 模型文件加载（SafeTensors/GGUF）</span></li>
            <li><strong>tokenizers/</strong> <span style="color: #6a737d;">— 文本分词器（CLIP T5 等）</span></li>
          </ul>
        </li>
        <li>
          <strong>thirdparty/</strong> <span style="color: #6a737d;">— 第三方依赖</span>
        </li>
        <li>
          <strong>include/</strong> <span style="color: #6a737d;">— 公共头文件</span>
          <ul style="list-style-type: none; padding-left: 1.5em;">
            <li><code>stable-diffusion.h</code> <span style="color: #6a737d;">— Stable Diffusion 公共 API</span></li>
          </ul>
        </li>
      </ul>
    </li>
  </ul>
</div>

---

## 核心架构

### 三层异构架构

```
┌──────────────────────────────────────────────┐
│                Go 服务层                       │
│  HTTP API → 图像生成请求处理                    │
│  前端资源服务（assets/ 嵌入）                    │
└────────────────────┬─────────────────────────┘
                     │ 调用 sd-cli 可执行文件
┌────────────────────▼─────────────────────────┐
│           C++ 推理引擎（sd-cli）               │
│  Stable Diffusion 模型加载与推理               │
│  文本编码器 + 扩散采样 + VAE 解码              │
│  模型格式: SafeTensors / GGUF                 │
└────────────────────┬─────────────────────────┘
                     │
┌────────────────────▼─────────────────────────┐
│              GGML 张量计算库                   │
│  底层张量运算 + 多 GPU 后端加速                 │
│  CUDA · Vulkan · Metal · SYCL · OpenCL · BLAS │
└──────────────────────────────────────────────┘
```

### 图像生成推理流程

```
文本提示词（Prompt）
    │
    ▼
文本编码器（CLIP/T5 Tokenizer + Encoder）
    │
    ▼
条件嵌入（Conditioning Embeddings）
    │
    ▼
扩散采样器（DPM++ / Euler / 等）
    ├── 从随机噪声开始
    ├── 多步迭代去噪
    └── 以文本嵌入为条件引导
    │
    ▼
VAE 解码器
    ├── Latent 空间 → 像素空间
    └── 输出 RGB 图像
    │
    ▼
图像后处理 → PNG/JPEG 输出
```

### 构建流程

```
build.ps1（一站式构建入口）
    │
    ├── Stage 1: build_ggml.ps1
    │   ├── CMake 配置 GGML（Vulkan ON, CUDA OFF）
    │   ├── MinGW Makefiles 构建
    │   └── 产物: ggml.a, ggml-cpu.a, ggml-vulkan.a
    │
    ├── Stage 2: C++ 构建
    │   ├── CMake 配置 sd_lunar（链接预编译 GGML）
    │   ├── MinGW Makefiles 构建 sd-cli
    │   └── 产物: cpp/build/bin/sd-cli.exe
    │
    └── Stage 3: Go 编译
        ├── go mod tidy
        ├── CGO_ENABLED=1 编译
        └── 产物: SD_Lunar.exe
```

---

## 核心模块说明

### GGML 后端支持

GGML 库通过条件编译支持多种 GPU 加速后端：

| 后端 | CMake 标志 | 适用平台 |
|------|-----------|---------|
| Vulkan | `GGML_VULKAN=ON` | 跨平台 GPU（默认启用） |
| CUDA | `GGML_CUDA=ON` | NVIDIA GPU |
| Metal | `GGML_METAL=ON` | Apple Silicon |
| SYCL | `GGML_SYCL=ON` | Intel GPU |
| OpenCL | `GGML_OPENCL=ON` | 通用 GPU |
| BLAS | `GGML_BLAS=ON` | CPU 加速 |

### C++ 引擎 (cpp/)

| 目录/文件 | 说明 |
|----------|------|
| `src/main.cpp` | CLI 入口（sd-cli 可执行文件） |
| `src/model_io/` | 模型文件加载器（SafeTensors/GGUF 格式） |
| `src/tokenizers/` | 文本分词器（CLIP/T5 等） |
| `include/stable-diffusion.h` | 公共 API 头文件 |
| `thirdparty/` | 第三方依赖库 |

---

## 编译与运行

### 一键构建（推荐）

```powershell
cd d:\Lunar_Astral_Agents\subsystem\sd_lunar
.\build.ps1
```

`build.ps1` 是**一站式构建入口**，自动按顺序完成三个阶段：

| 阶段 | 内容 | 内部脚本 |
|------|------|---------|
| Stage 1 | 编译 GGML 张量计算库 | `build_ggml.ps1` |
| Stage 2 | 编译 Stable Diffusion C++ 引擎 | CMake + MinGW |
| Stage 3 | 编译 Go 服务层 | `go build` |

> `build_ggml.ps1` 是内部实现细节，由 `build.ps1` 自动调用，无需手动执行。

可选参数：

```powershell
.\build.ps1 -TargetOS windows -TargetArch amd64  # 默认配置
```

编译产物：`d:\Lunar_Astral_Agents\SD_Lunar.exe`

### GGML 预编译（单独执行）

```powershell
.\build_ggml.ps1

# 可选参数
.\build_ggml.ps1 -BuildType Debug    # Debug 模式
.\build_ggml.ps1 -Clean              # 清理后重新编译
.\build_ggml.ps1 -ParallelJobs 8     # 指定并行编译数
```

### 运行要求

1. 确保模型文件放置在 `{LocalDir}/models/` 目录
2. Stable Diffusion 模型文件（SafeTensors 格式）：`sd3_medium.safetensors`
3. VAE 模型文件：`sd3_vae.safetensors`
4. 如需 GPU 加速，确保已安装 Vulkan SDK 或 CUDA Toolkit

### 运行

```powershell
.\SD_Lunar.exe
```

程序自动打开 WebView 窗口，提供可视化文本输入与图像生成界面。

### 环境要求

| 组件 | 版本/说明 |
|------|----------|
| **Go** | ≥ 1.24 |
| **CMake** | ≥ 3.12 |
| **GCC/G++** | MinGW-w64（支持 C++17） |
| **Vulkan SDK** | 推荐 1.4.350.0+（GPU 加速） |
| **CGO** | 需启用（webview 依赖） |

---

## 常见问题

### Q: 构建时 GGML 预编译失败怎么办？

1. 确认已安装 CMake（`cmake --version`）
2. 确认 MinGW-w64 GCC 可用（`gcc --version`）
3. 如 Vulkan SDK 未安装，GGML 会以纯 CPU 模式编译
4. 尝试清理后重新编译：`.\build_ggml.ps1 -Clean`

### Q: 如何启用 CUDA 加速？

修改 `build_ggml.ps1` 中的 CMake 参数，将 `-DGGML_CUDA=OFF` 改为 `-DGGML_CUDA=ON`，并确保系统已安装 CUDA Toolkit 12.x 或 13.x。

### Q: 图像生成速度慢怎么办？

1. 启用 GPU 加速（Vulkan 或 CUDA）
2. 使用量化模型减少推理计算量
3. 降低采样步数或图像分辨率
4. 确认 GGML 编译时启用了 OpenMP（`-DGGML_OPENMP=ON`）

### Q: sd-cli.exe 在哪里？

编译后位于 `cpp/build/bin/sd-cli.exe`，由 `build.ps1` Stage 2 自动生成。Go 服务层通过调用此可执行文件执行图像生成。

### Q: 支持哪些 Stable Diffusion 模型？

支持 SD 1.x / SD 2.x / SDXL / SD3 等主流架构的 SafeTensors 格式模型。模型文件需放置在 [config 子系统](../config/README.md) 中 `DiffusionModel` 和 `VariationalModel` 指定的路径。

---

## 相关文档

- [项目主文档](../../README.md) —— 环境要求与编译流程
- [配置管理子系统](../config/README.md) —— 模型路径与引擎配置
- [语音合成独立系统](../qwen3_tts_lunar/README.md) —— 类似 C++ GGML 架构的 TTS 引擎
- [星图·月华](../../lunar_astral/README.md) —— 图像生成引擎的集成使用方
