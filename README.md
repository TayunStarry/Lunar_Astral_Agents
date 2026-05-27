# 星月智能（Lunar Astral Agents）

基于 **Go + TypeScript + C/C++** 的纯本地化桌面 AI 智能体平台，集成文本推理、图像生成、语音识别、语音合成等功能，采用纯客户端部署方案，无需任何 Python 环境。

---

## 目录

- [项目结构](#项目结构)
- [环境要求](#环境要求)
- [编译流程](#编译流程)
- [系统架构](#系统架构)
- [子系统导航](#子系统导航)
- [常见问题](#常见问题)

---

## 项目结构

```
Lunar_Astral_Agents/
│
├── README.md                           ← 项目主文档（本文件）
├── image/                              ← 项目图片资源目录
│   ├── 月华-主页面.webp                ← 月华系统主界面截图
│   ├── 月华-主界面-手机端.webp         ← 月华系统移动端界面
│   ├── 月华-聊天记录.webp              ← 月华系统聊天记录界面
│   ├── 星图-月华-人设图-1.webp         ← 月华角色人设图
│   ├── 琉璃-主页面.webp                ← 琉璃系统主界面截图
│   ├── 琉璃-参数管理-配置预览.webp     ← 琉璃配置预览界面
│   ├── 琉璃-图像生成-参数配置.webp     ← 琉璃图像生成参数配置
│   ├── 琉璃-图像生成-图片预览.webp     ← 琉璃图像生成预览
│   ├── 琉璃-截图标注.webp              ← 琉璃截图标注界面
│   ├── 琉璃-数据管理-主页面.webp       ← 琉璃数据管理界面
│   ├── 琉璃-数据管理-配置说明.webp     ← 琉璃数据配置说明
│   ├── 琉璃-文件管理-主页面.webp       ← 琉璃文件管理界面
│   ├── 琉璃-文件管理-文本编辑.webp     ← 琉璃文本编辑界面
│   ├── 琉璃-消息渲染.webp              ← 琉璃消息渲染界面
│   ├── 星图-琉璃-人设图-0.webp         ← 琉璃角色人设图
│   ├── 多媒体预览-图片0.webp           ← 多媒体图片预览
│   ├── 多媒体预览-图片1.webp           ← 多媒体图片预览
│   ├── 多媒体预览-视频.webp            ← 多媒体视频预览
│   ├── 独立模块-语音合成-0.webp        ← 语音合成独立界面
│   ├── 独立模块-语音合成-1.webp        ← 语音合成独立界面
│   ├── 独立模块-语音识别-0.webp        ← 语音识别独立界面
│   ├── 独立模块-语音识别-1.webp        ← 语音识别独立界面
│   └── 旧版宣传图.jpg                  ← 旧版宣传图片
│
├── lunar_astral/                       ← 核心系统：星图·月华
│   ├── README.md                       ← 月华系统文档
│   ├── main.go                         ← 程序入口
│   ├── go.mod                          ← Go 模块定义
│   ├── build.ps1                       ← 编译脚本
│   ├── icon.ico                        ← 应用图标
│   ├── package.json                    ← Node.js 前端构建配置
│   ├── rollup.config.js                ← 前端打包配置
│   ├── tsconfig.json                   ← TypeScript 配置
│   ├── removeExport.cjs                ← 构建后处理脚本
│   ├── adapters/                       ← Go↔JS 适配器层（CGO 桥接）
│   │   ├── type.go                     ← 类型定义
│   │   ├── create.go                   ← JS 运行时创建
│   │   ├── database.go                 ← 数据库适配
│   │   ├── file.go                     ← 文件系统适配
│   │   ├── message.go                  ← 消息处理适配
│   │   ├── network.go                  ← 网络请求适配
│   │   └── vision.go                   ← 视觉处理适配
│   ├── model/                          ← 模型服务层
│   │   ├── type.go                     ← 模型类型定义
│   │   ├── core.go                     ← 核心模型逻辑
│   │   ├── variable.go                 ← 模型变量
│   │   ├── llama/                ← llama.cpp 代理
│   │   │   └── proxy.go                ← 代理核心实现
│   │   └── tts/                        ← TTS 语音合成引擎
│   │       ├── type.go                 ← TTS 类型定义
│   │       ├── entry.go                ← TTS 入口
│   │       ├── cache.go                ← 音频缓存
│   │       ├── capture.go              ← 音频捕获
│   │       ├── variable.go             ← TTS 变量
│   │       ├── wrapper.go              ← TTS 封装
│   │       └── writer.go               ← 音频写入
│   ├── server/                         ← HTTP 服务器层
│   │   ├── type.go                     ← 服务器类型定义
│   │   ├── create.go                   ← 服务器创建与启动
│   │   ├── manage.go                   ← 服务器管理
│   │   ├── variable.go                 ← 端点与变量
│   │   └── handlers/                   ← HTTP 请求处理器
│   │       ├── type.go                 ← 处理器类型
│   │       ├── generate.go             ← 图像生成处理
│   │       ├── message.go              ← 消息处理
│   │       ├── proxy.go                ← 代理转发处理
│   │       └── video.go                ← 视频处理
│   ├── release/                        ← 进程/端口管理
│   │   ├── execute.go                  ← 命令执行
│   │   ├── kill.go                     ← 进程终止
│   │   ├── network_status.go           ← 网络状态监控
│   │   ├── processes.go                ← 进程列表
│   │   └── query.go                    ← 查询功能
│   ├── hierarchy/                      ← 前端资源与脚本
│   │   ├── embedded.go                 ← Go embed 资源嵌入
│   │   ├── image/                      ← 图像生成模块
│   │   │   ├── generate/               ← 图像生成
│   │   │   │   ├── generate.go         ← 生成逻辑
│   │   │   │   └── type.go             ← 生成类型
│   │   │   └── video.go                ← 视频工具
│   │   └── assets/                     ← 前端资源
│   │       ├── agentSystem.js          ← 智能体系统核心 JS
│   │       ├── prompts/                ← AI 提示词模板
│   │       │   ├── chatRole.md         ← 聊天角色设定
│   │       │   ├── descriptionRole.md  ← 描述角色设定
│   │       │   ├── emotionManager.md   ← 情绪管理设定
│   │       │   ├── imagePrompt.md      ← 图像生成提示
│   │       │   ├── painterRole.md      ← 画师角色设定
│   │       │   ├── queryKeywords.md    ← 关键词查询
│   │       │   ├── recorderRole.md     ← 记录角色设定
│   │       │   ├── selfAppearance.md   ← 角色外观设定
│   │       │   └── summaryRole.md      ← 摘要角色设定
│   │       └── client/                 ← 前端客户端
│   │           ├── index.html          ← 主页面
│   │           ├── app.js              ← 主应用逻辑
│   │           ├── chat.js             ← 聊天模块
│   │           ├── fetch.js            ← 网络请求
│   │           ├── file.js             ← 文件处理
│   │           ├── live2d.js           ← Live2D 角色渲染
│   │           ├── socket.js           ← WebSocket 通信
│   │           ├── style.css           ← 样式表
│   │           ├── tts.js              ← 语音合成前端
│   │           ├── util.js             ← 工具函数
│   │           └── favicon.ico         ← 网站图标
│   ├── websocket/                      ← WebSocket 通信层
│   │   ├── type.go                     ← WebSocket 类型
│   │   ├── variable.go                 ← WebSocket 变量
│   │   └── websocket.go                ← WebSocket 核心
│   └── model/                          ←（同上，模型服务层）
│
├── crystal_astral/                     ← 扩展系统：星图·琉璃
│   ├── README.md                       ← 琉璃系统文档
│   ├── main.go                         ← 程序入口
│   ├── go.mod                          ← Go 模块定义
│   ├── create.go                       ← 服务器创建
│   ├── embedded.go                     ← 资源嵌入
│   ├── endpoint.go                     ← API 端点定义
│   ├── handler.go                      ← 请求处理
│   ├── type.go                         ← 类型定义
│   ├── build.ps1                       ← 编译脚本
│   ├── icon.ico                        ← 应用图标
│   └── assets/                         ← 前端资源
│       ├── index.html                  ← 主页面
│       ├── script.js                   ← 应用逻辑
│       ├── style.css                   ← 样式表
│       └── favicon.ico                 ← 网站图标
│
├── subsystem/                          ← 可复用子系统模块
│   ├── config/                         ← 子系统：配置管理
│   │   ├── README.md                   ← 配置模块文档
│   │   ├── go.mod                      ← 模块定义
│   │   ├── init.go                     ← 配置初始化入口
│   │   ├── allow.go                    ← 功能开关
│   │   ├── engine.go                   ← 外部引擎配置
│   │   ├── image.go                    ← 图像参数
│   │   ├── model.go                    ← 模型路径
│   │   ├── path.go                     ← 路径配置
│   │   ├── port.go                     ← 端口配置
│   │   ├── system.go                   ← 运行时状态
│   │   └── webview.go                  ← WebView 窗口配置
│   │
│   ├── browser/                        ← 子系统：网页前端启动
│   │   ├── README.md                   ← 浏览器模块文档
│   │   ├── go.mod                      ← 模块定义
│   │   ├── execute.go                  ← IP 发现与启动
│   │   ├── type.go                     ← 类型与状态
│   │   └── webView.go                  ← WebView 窗口管理
│   │
│   ├── storage/                        ← 子系统：文件管理
│   │   ├── README.md                   ← 存储模块文档
│   │   ├── go.mod                      ← 模块定义
│   │   ├── module/                     ← 核心逻辑层
│   │   │   ├── type.go                 ← 数据结构
│   │   │   ├── save.go                 ← 文件保存
│   │   │   ├── read.go                 ← 文件读取
│   │   │   ├── delete.go               ← 文件删除
│   │   │   ├── download.go             ← 文件下载
│   │   │   ├── filelist.go             ← 文件列表
│   │   │   ├── archive.go              ← ZIP 压缩/解压
│   │   │   ├── background.go           ← 随机背景图
│   │   │   └── database.go             ← SQLite 数据库
│   │   └── server/                     ← HTTP 服务层
│   │       ├── save.go                 ← 保存接口
│   │       ├── read.go                 ← 读取接口
│   │       ├── delete.go               ← 删除接口
│   │       ├── download.go             ← 下载接口
│   │       ├── filelist.go             ← 文件列表接口
│   │       ├── archive.go              ← 归档接口
│   │       ├── background.go           ← 背景图接口
│   │       └── database.go             ← 数据库接口
│   │
│   ├── screenshot/                     ← 子系统：屏幕截图
│   │   ├── README.md                   ← 截图模块文档
│   │   ├── go.mod                      ← 模块定义
│   │   ├── type.go                     ← 类型定义
│   │   ├── module.go                   ← 核心逻辑
│   │   └── server.go                   ← HTTP 服务
│   │
│   ├── qwen3_tts_lunar/               ← 独立系统：语音合成
│   │   ├── README.md                   ← TTS 模块文档
│   │   ├── main.go                     ← 程序入口
│   │   ├── go.mod                      ← 模块定义
│   │   ├── server.go                   ← HTTP 服务
│   │   ├── build.ps1                   ← 编译脚本
│   │   ├── build_cpp.ps1               ← C++ 编译脚本
│   │   ├── build_ggml.ps1              ← GGML 编译脚本
│   │   ├── icon.ico                    ← 应用图标
│   │   ├── module/                     ← Go 逻辑层
│   │   │   ├── generate.go             ← 语音生成
│   │   │   ├── variable.go             ← 变量定义
│   │   │   └── stream.go               ← 流式处理
│   │   ├── client/                     ← 前端界面
│   │   │   ├── index.html              ← 主页面
│   │   │   ├── app.js                  ← 应用逻辑
│   │   │   ├── style.css               ← 样式表
│   │   │   ├── picture.webp            ← 背景图
│   │   │   └── favicon.ico             ← 图标
│   │   └── cpp/                        ← C++ 推理引擎
│   │       ├── CMakeLists.txt          ← CMake 构建
│   │       ├── src/                    ← 引擎源码
│   │       │   ├── qwen3_tts.cpp/h     ← TTS 主引擎
│   │       │   ├── qwen3tts_c_api.cpp/h ← C API 接口
│   │       │   ├── tts_transformer.cpp/h ← Transformer 层
│   │       │   ├── audio_tokenizer_*.cpp/h ← 音频分词器
│   │       │   ├── gguf_loader.cpp/h   ← GGUF 模型加载
│   │       │   ├── text_tokenizer.cpp/h ← 文本分词
│   │       │   ├── main.cpp            ← 独立可执行文件入口
│   │       │   ├── coreml_*.cpp/h      ← Apple CoreML 加速
│   │       │   └── qwen3tts.def        ← Windows DLL 导出
│   │       └── ggml/                   ← GGML 张量计算库
│   │
│   └── qwen_asr_lunar/                ← 独立系统：语音识别
│       ├── README.md                   ← ASR 模块文档
│       ├── main.go                     ← 程序入口
│       ├── go.mod                      ← 模块定义
│       ├── asr.go                      ← Go↔C 桥接层
│       ├── handler.go                  ← HTTP 处理
│       ├── build.ps1                   ← 编译脚本
│       ├── icon.ico                    ← 应用图标
│       ├── static/                     ← 前端界面
│       │   ├── index.html              ← 主页面
│       │   ├── app.js                  ← 应用逻辑
│       │   ├── style.css               ← 样式表
│       │   ├── picture.webp            ← 背景图
│       │   └── favicon.ico             ← 图标
│       ├── openblas/                   ← OpenBLAS 线性代数库
│       │   └── include/                ← C 头文件
│       └── C 推理源码                   ← 纯 C 推理引擎
│           ├── qwen_asr.h/c            ← 主入口与管线
│           ├── qwen_asr_audio.h/c      ← 音频预处理
│           ├── qwen_asr_encoder.c      ← 编码器实现
│           ├── qwen_asr_decoder.c      ← 解码器实现
│           ├── qwen_asr_tokenizer.h/c  ← GPT-2 BPE 分词
│           ├── qwen_asr_safetensors.h/c ← SafeTensors 加载
│           ├── qwen_asr_kernels.h/c    ← 数学核心分发
│           ├── qwen_asr_kernels_avx.c  ← x86 SIMD 优化
│           ├── qwen_asr_kernels_neon.c ← ARM NEON 优化
│           └── qwen_asr_kernels_generic.c ← 通用实现
│
└── .trae/                              ← 项目规则配置
    └── rules/                          ← 代码规范
        └── git-commit-message.md       ← Git 提交规范
```

### 层级关系说明

```
星月智能平台 (Lunar Astral Agents)
│
├── 核心系统: 星图·月华 (lunar_astral)
│   ├── 依赖: config, browser, storage, screenshot, qwen3_tts_lunar
│   ├── 功能: AI 对话、Live2D 角色、TTS 语音、图像生成
│   └── 入口: Lunar_Astral.exe
│
├── 扩展系统: 星图·琉璃 (crystal_astral)
│   ├── 依赖: config, browser, storage, screenshot
│   ├── 功能: 文件管理、数据库管理、截图标注、AI 代理
│   └── 入口: Crystal_Astral.exe
│
├── 独立系统: 语音合成 (qwen3_tts_lunar)
│   ├── 依赖: C++ GGML 引擎
│   ├── 功能: Qwen3-TTS 文本转语音
│   └── 入口: Qwen3_TTS_Lunar.exe
│
├── 独立系统: 语音识别 (qwen_asr_lunar)
│   ├── 依赖: 纯 C 引擎 + OpenBLAS
│   ├── 功能: Qwen3-ASR 语音转文本
│   └── 入口: Qwen_ASR_Lunar.exe
│
└── 公共子系统 (subsystem/)
    ├── config      → 全局配置中枢
    ├── browser     → WebView 窗口 + 本地 IP 发现
    ├── storage     → 文件存储 + SQLite 数据库
    └── screenshot  → 屏幕截图 + 图片缩放
```

---

## 环境要求

### 操作系统支持

| 系统 | 版本 | 架构 | 状态 |
|------|------|------|------|
| Windows 10 | 21H2 及以上 | x64 | ✅ 支持 |
| Windows 11 | 所有版本 | x64 | ✅ 支持 |
| Windows 10/11 | 32 位 | x86 | ❌ 不支持 |
| Linux | 任意版本 | 任意 | ❌ 不支持 |
| macOS | 任意版本 | 任意 | ❌ 不支持 |

### 开发环境

| 工具 | 最低版本 | 用途 | 安装指南 |
|------|---------|------|---------|
| Go | ≥ 1.25.0 | Go 后端编译 | [go.dev/dl](https://go.dev/dl/) |
| Node.js | ≥ 20.x | TypeScript 前端编译 | [nodejs.org](https://nodejs.org/en/download/) |
| GCC (MinGW-w64) | ≥ 8.1.0 | C/C++ 编译（ASR/TTS） | [mingw-w64.org](https://www.mingw.org/mingw64) |
| CMake | ≥ 3.29.0 | C++ 项目构建（TTS） | [cmake.org](https://cmake.org/download/) |

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
| WebView2 Runtime | ≥ 109.0 | 桌面嵌入式浏览器 | [developer.microsoft.com/en-us/microsoft-edge/webview2/](https://developer.microsoft.com/en-us/microsoft-edge/webview2/) |
| FFmpeg | ≥ 5.0.0 | 音频/视频格式转换 | [ffmpeg.org/download.html](https://ffmpeg.org/download.html) |
| Vulkan SDK | ≥ 1.3 | GPU 推理加速 | [lunarg.com/sdk-downloads/vulkan-sdk](https://www.lunarg.com/sdk-downloads/vulkan-sdk) |

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
月华 → 琉璃 → 桥接适配器 → 卷归档 → 语音合成。每个子系统的 `build.ps1` 均为自包含脚本，内部已处理所有前置步骤（前端编译、GGML 库构建、C++ 引擎编译等）。

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
┌─────────────────────────────────────────────────────────────┐
│                      星月智能平台                             │
│                                                              │
│  ┌─────────────────┐  ┌─────────────────┐                   │
│  │  星图·月华        │  │  星图·琉璃        │                  │
│  │  (AI 桌面智能体)   │  │  (工具集扩展程序)  │                 │
│  │                  │  │                  │                  │
│  │  · AI 对话角色   │  │  · 文件管理      │                  │
│  │  · Live2D 展示   │  │  · 数据库管理    │                  │
│  │  · TTS 语音合成  │  │  · 截图标注      │                  │
│  │  · 图像生成     │  │  · AI 代理转发   │                  │
│  │  · WebSocket    │  │  · 应用加载器    │                  │
│  └────────┬────────┘  └────────┬────────┘                   │
│           │                    │                             │
│           └────────┬───────────┘                             │
│                    │                                         │
│  ┌─────────────────┼─────────────────────────────┐          │
│  │          公共子系统 (subsystem)                 │          │
│  │                                                │          │
│  │  ┌──────────┐ ┌─────────┐ ┌──────────┐        │          │
│  │  │ config   │ │ browser │ │ storage  │        │          │
│  │  │ 配置管理  │ │ 网页前端 │ │ 文件管理  │        │          │
│  │  └──────────┘ └─────────┘ └──────────┘        │          │
│  │  ┌──────────┐ ┌──────────────────┐            │          │
│  │  │screenshot│ │ 独立 AI 引擎      │            │          │
│  │  │屏幕截图   │ │ TTS · ASR        │            │          │
│  │  └──────────┘ └──────────────────┘            │          │
│  └───────────────────────────────────────────────┘          │
│                                                              │
│  ┌──────────────────────────────────────────────────┐      │
│  │              外部推理引擎                          │      │
│  │  ┌──────────────┐ ┌──────────────┐                │      │
│  │  │ llama.cpp    │ │ stable-      │                │      │
│  │  │ (GGUF 推理)   │ │ diffusion.cpp│               │      │
│  │  └──────────────┘ └──────────────┘                │      │
│  └──────────────────────────────────────────────────┘      │
└─────────────────────────────────────────────────────────────┘
```

### 数据流概要

```
用户输入 (前端界面)
    │
    ├─→ HTTP API → llm_proxy → llama-server.exe → GGUF 模型推理
    │                                          ↓
    │                                    推理结果返回
    │                                          ↓
    ├─→ JS 智能体 (goja 运行时) → 角色逻辑处理 → 生成回复
    │                                          ↓
    ├─→ TTS 引擎 → WAV 音频合成
    │
    └─→ WebSocket 推送 → 前端实时渲染 (Markdown/Mermaid/ECharts/Live2D)
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
| 语音合成 | [subsystem/qwen3_tts_lunar/README.md](subsystem/qwen3_tts_lunar/README.md) | Qwen3-TTS 文本转语音引擎 |
| 语音识别 | [subsystem/qwen_asr_lunar/README.md](subsystem/qwen_asr_lunar/README.md) | Qwen3-ASR 语音转文本引擎 |

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