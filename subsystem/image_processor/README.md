# 子系统——图像处理（image_processor）

图像生成、视频关键帧提取与屏幕截图的共享库模块，封装 stable-diffusion.cpp 调用流程、视频帧截取逻辑与跨平台截图能力，采用 Module（逻辑层）+ Server（HTTP 层）二层架构。

---

## 目录

- [功能概述](#功能概述)
- [项目结构](#项目结构)
- [核心架构](#核心架构)
- [核心模块说明](#核心模块说明)
- [API 接口定义](#api-接口定义)
- [使用示例](#使用示例)
- [常见问题](#常见问题)

---

## 功能概述

`image_processor` 子系统是图像处理能力的共享库，为星图·月华提供文生图、视频关键帧提取与屏幕截图能力。

| 功能 | 说明 |
|------|------|
| 扩散图像生成 | 调用 stable-diffusion.cpp 命令行引擎，支持文生图 / 图生图 / 超分，异步任务队列 |
| 视频关键帧提取 | 从视频文件中智能提取关键帧图片 |
| 屏幕截图 | 跨平台屏幕捕获（全屏 / 指定显示器 / 区域截图），支持缩放与格式转换 |
| 图像格式处理 | Base64 编解码、像素格式转换、缩放裁剪、着色处理 |

---

## 项目结构

<div style="font-family: 'Cascadia Code', 'SF Mono', Consolas, monospace; font-size: 0.9em; line-height: 1.6;">
  <ul style="list-style-type: none; padding-left: 0;">
    <li><strong>image_processor/</strong></li>
    <li style="padding-left: 1.5em;"><code>go.mod</code> / <code>go.sum</code> <span style="color: #6a737d;">— Go 模块定义</span></li>
    <li style="padding-left: 1.5em;"><strong>module/</strong> <span style="color: #6a737d;">— 核心业务逻辑层</span>
      <ul style="list-style-type: none; padding-left: 1.5em;">
        <li><code>generate.go</code> <span style="color: #6a737d;">— 扩散图像生成逻辑（异步任务队列 + sd-cli 引擎）</span></li>
        <li><code>keyframe.go</code> <span style="color: #6a737d;">— 视频关键帧提取（FFmpeg 辅助）</span></li>
        <li><code>screenshot.go</code> <span style="color: #6a737d;">— 屏幕截图与图片缩放（kbinani/screenshot）</span></li>
        <li><code>type.go</code> <span style="color: #6a737d;">— 数据类型定义</span></li>
        <li><code>variable.go</code> <span style="color: #6a737d;">— 全局变量与初始化</span></li>
      </ul>
    </li>
    <li style="padding-left: 1.5em;"><strong>server/</strong> <span style="color: #6a737d;">— HTTP 服务层</span>
      <ul style="list-style-type: none; padding-left: 1.5em;">
        <li><code>generate.go</code> <span style="color: #6a737d;">— 图像生成 HTTP 处理器</span></li>
        <li><code>keyframe.go</code> <span style="color: #6a737d;">— 关键帧提取 HTTP 处理器</span></li>
        <li><code>screenshot.go</code> <span style="color: #6a737d;">— 截图与缩放 HTTP 处理器</span></li>
        <li><code>type.go</code> <span style="color: #6a737d;">— HTTP 请求/响应类型定义</span></li>
      </ul>
    </li>
  </ul>
</div>

### 依赖关系

<div style="font-family: 'Cascadia Code', 'SF Mono', Consolas, monospace; font-size: 0.9em; line-height: 1.6;">
  <ul style="list-style-type: none; padding-left: 0;">
    <li><code>image_processor</code></li>
    <li style="padding-left: 1.5em;"><code>general_config</code> <span style="color: #6a737d;">(../general_config) — 配置管理（扩散模型路径、图像参数）</span></li>
    <li style="padding-left: 1.5em;"><code>logger_general</code> <span style="color: #6a737d;">(../logger_general) — 彩色终端日志</span></li>
    <li style="padding-left: 1.5em;"><strong>外部依赖</strong>
      <ul style="list-style-type: none; padding-left: 1.5em;">
        <li><code>sd-cli.exe</code> <span style="color: #6a737d;">— stable-diffusion.cpp 命令行引擎</span></li>
        <li><code>ffmpeg</code> <span style="color: #6a737d;">— 视频解码与帧提取（ffmpeg-go 封装）</span></li>
        <li><code>kbinani/screenshot</code> <span style="color: #6a737d;">— 跨平台屏幕捕获</span></li>
      </ul>
    </li>
  </ul>
</div>

---

## 核心架构

### 二层分离设计

```
┌─────────────────────────────────────────────────┐
│            HTTP 层（server/）                     │
│  GenerateHandler  ExtractKeyFramesHandler  ...   │
│  HandleScreenshot  HandleScreenshotRegion  ...   │
│  解析 HTTP 请求 → 调用 module 函数 → 写 HTTP 响应  │
└──────────────────────┬──────────────────────────┘
                       │
┌──────────────────────▼──────────────────────────┐
│          业务逻辑层（module/）                     │
│  图像生成: prompt → sd-cli 引擎 → JPEG 输出        │
│  关键帧提取: 视频 → FFmpeg 解码 → 帧图片           │
│  屏幕截图: 显示器 → kbinani/screenshot → 图像     │
│  安全机制: 路径校验 / 格式验证 / 超时控制           │
└──────────────────────┬──────────────────────────┘
                       │
          ┌────────────┼────────────┐
          ▼            ▼            ▼
     ┌─────────┐  ┌──────────┐  ┌──────────┐
     │ sd-cli  │  │  ffmpeg  │  │screenshot│
     │外部引擎 │  │外部工具   │  │ 截图库   │
     └─────────┘  └──────────┘  └──────────┘
```

### 图像生成流水线

```
用户请求
    │  (prompt, negative_prompt, batch_size, width, height, steps, seed, ...)
    ▼
module.CreateGenerateTask()
    │  创建异步任务（任务队列，避免阻塞 HTTP 请求）
    ▼
module.ProcessTask()  →  module.GenerateImage()
    │
    ├── ① 参数校验（提示词非空、步数范围、种子范围）
    ├── ② 构建 sd-cli 命令行参数
    │    └── sd-cli -m {diffusion_model} --vae {variational_model}
    │              -p "{prompt}" -n "{negative_prompt}"
    │              --steps {steps} --seed {seed}
    │
    ├── ③ exec.Command 启动外部进程
    │    └── 带超时控制（避免长时间阻塞）
    │
    ├── ④ 读取 stdout/stderr → 捕获输出
    │
    ├── ⑤ 读取生成的图片文件 → Base64 编码
    │
    └── ⑥ 更新任务状态，通知等待客户端
```

### 视频关键帧提取流程

```
视频文件
    │
    ▼
module.VideoKeyframeExtraction()
    │
    ├── ffprobe 获取视频时长与关键帧信息
    ├── ffmpeg 按时间间隔提取帧 → 帧图像数组
    ├── CalculateImageDifference 计算相邻帧差异，过滤相似帧
    ├── 生成关键帧文件 → KeyFrame（文件名/时间戳/帧数据）
    └── 返回关键帧数组
```

### 屏幕截图流程

```
HTTP 请求（ScreenshotRequest）
    │  (display_index, region, scale, format, quality)
    ▼
module.Screenshot()
    │
    ├── kbinani/screenshot 捕获显示器画面（-1 表示所有显示器）
    ├── 按 region / scale 裁剪缩放
    ├── 编码为指定格式（png / jpg / jpeg）
    └── 返回图像字节 + 格式 + 尺寸
```

---

## 核心模块说明

### module/generate.go — 图像生成（异步任务队列）

| 函数 | 说明 |
|------|------|
| `StartTaskProcessor()` | 启动后台任务处理协程 |
| `CreateGenerateTask(prompt, negativePrompt, batchSize, width, height, steps, strength, cfgScale, seed, initImg, allowSuperResolution) (*GenerateTask, int)` | 创建生成任务并入队，返回任务与 HTTP 状态码 |
| `ProcessTask(task GenerateTask)` | 消费任务：调用 sd-cli 引擎生成图片 |
| `GenerateImage(...) (map[string]any, error)` | 主生成函数：构建参数 → 调用引擎 → 返回结果 |
| `GetTaskStatus(taskID) (*GenerateTask, bool)` | 查询任务状态 |
| `RegisterWaitClient(taskID) chan *GenerateTask` | 注册任务完成通知等待通道 |
| `NotifyWaitClients(taskID, task)` | 任务完成后通知所有等待客户端 |

### module/keyframe.go — 关键帧提取

| 函数 | 说明 |
|------|------|
| `VideoKeyframeExtraction(inputFile) ([]KeyFrame, error)` | 从视频智能提取关键帧（差异过滤） |
| `GetVideoDuration(inputFile) (float64, error)` | 获取视频时长 |
| `CalculateImageDifference(img1, img2) float64` | 计算两帧图像差异度 |
| `CreateKeyframeFile(currImage, keyFrames) (string, []byte, error)` | 生成关键帧文件 |
| `IsSupportedVideoFormat(filename) bool` | 校验视频格式支持 |

### module/screenshot.go — 屏幕截图

| 函数 | 说明 |
|------|------|
| `Screenshot(req ScreenshotRequest) ([]byte, string, string, error)` | 核心截图函数：返回（图像字节、格式、尺寸） |
| `GetDisplays() []map[string]int` | 枚举显示器列表 |
| `ResizeImage(imgData) ([]map[string]any, error)` | 图片缩放 |
| `ToRGBA(img) *image.RGBA` | 任意图像转 RGBA |
| `ResizeToFit(img, maxWidth, maxHeight) *image.RGBA` | 等比缩放至指定尺寸 |

`ScreenshotRequest` 字段：`display_index`（显示器索引，-1 全部）、`region`（区域 `x,y,width,height`）、`scale`（缩放 `width,height` 或 `0.5`）、`format`（png/jpg/jpeg）、`quality`（JPEG 质量 1-100）

---

## API 接口定义

### HTTP 端点

由宿主程序（星图·月华 crystal_astral）注册，路径如下：

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/generate` | 扩散图像生成（异步任务） |
| GET | `/generate/wait` | 轮询 / 等待生成任务完成 |
| POST | `/keyframe` | 视频关键帧提取 |
| POST | `/capture` | 通用截图（按 ScreenshotRequest 参数） |
| GET | `/capture/display/` | 指定显示器全屏截图 |
| POST | `/capture/region` | 区域截图 |
| GET | `/capture/displays` | 屏幕列表 |
| POST | `/resize` | 图片缩放 |

### 图像生成请求

```json
// POST /generate
{
  "prompt": "一座月光下的湖泊，波光粼粼",
  "negative_prompt": "模糊，低质量",
  "width": 512,
  "height": 512,
  "steps": 20,
  "seed": 42,
  "cfg_scale": 7.5
}
```

### 图像生成响应

```json
// 成功
{
  "success": true,
  "image": "data:image/jpeg;base64,/9j/4AAQSkZJRg...",
  "seed": 42,
  "steps": 20
}

// 失败
{
  "success": false,
  "error": "扩散引擎执行失败：模型文件不存在"
}
```

### 视频关键帧请求

```
POST /keyframe
Content-Type: multipart/form-data

# 表单字段：video = <视频文件>
```

### 视频关键帧响应

```json
{
  "success": true,
  "frames": [
    { "filePath": "keyframe_1.jpg", "timestamp": "00:00:03", "frameNum": 1, "data": "base64..." },
    { "filePath": "keyframe_2.jpg", "timestamp": "00:00:08", "frameNum": 2, "data": "base64..." }
  ],
  "duration": 30.5,
  "frame_count": 2
}
```

### 截图请求

```json
// POST /capture
{
  "display_index": -1,      // -1 全部显示器，0/1/2 指定显示器
  "region": "",             // 可选，"x,y,width,height" 区域截图
  "scale": "",              // 可选，"width,height" 或 "0.5" 缩放
  "format": "png",          // png / jpg / jpeg
  "quality": 90             // JPEG 质量 1-100
}
```

### 截图响应

```json
{
  "success": true,
  "image": "data:image/png;base64,...",
  "format": "png",
  "size": "1920x1080"
}
```

---

## 使用示例

### Go 代码中使用

```go
package main

import (
    "fmt"
    "LunarSubsystem/ImageProcessor/module"
)

func main() {
    // 创建生成任务（异步队列）
    task, code := module.CreateGenerateTask(
        "月光下的樱花树",      // prompt
        "模糊，畸形",          // negative_prompt
        1,                     // batch_size
        512, 512,              // width, height
        20,                    // steps
        0.8, 7.5,              // strength, cfg_scale
        42,                    // seed
        "",                    // init_img（空=文生图）
        false,                 // allow_super_resolution
    )
    fmt.Println(task.ID, code)

    // 关键帧提取
    frames, err := module.VideoKeyframeExtraction("video.mp4")
    if err != nil {
        panic(err)
    }
    fmt.Println(len(frames))

    // 屏幕截图（所有显示器）
    imgBytes, format, size, err := module.Screenshot(module.ScreenshotRequest{
        DisplayIndex: -1,
        Format:       "png",
    })
    if err != nil {
        panic(err)
    }
    fmt.Println(format, size, len(imgBytes))
}
```

---

## 常见问题

### Q: sd-cli.exe 在哪里？

sd-cli.exe 是 stable-diffusion.cpp 的命令行引擎，需自行编译或获取。图像生成模块通过 [general_config 子系统](../general_config/README.md) 中的 `VisualEngine` 路径配置找到它。

### Q: 图像生成速度慢怎么办？

1. 启用 GPU 加速（Vulkan/CUDA）
2. 使用量化模型减少推理计算量
3. 降低采样步数（`steps` 参数）
4. 减小输出分辨率（`width`/`height`）

### Q: 支持哪些图像生成模型？

支持 stable-diffusion.cpp 兼容的 SafeTensors/GGUF 格式模型，包括 SD 1.x / SD 2.x / SDXL / SD3 等架构。模型路径在 config 子系统中配置。

### Q: 视频关键帧提取失败怎么办？

1. 确认 FFmpeg 已安装并添加到系统 PATH
2. 或在 `config` 子系统中的 `FfmpegPath` 配置自定义路径
3. 确认视频文件格式兼容（支持 MP4、AVI、MOV、WebM 等主流格式）

### Q: 截图功能在哪些平台可用？

截图基于 `kbinani/screenshot` 库，支持 Windows、macOS、Linux（X11）等主流桌面平台；无显示器环境（如远程会话）可能无法捕获画面。

---

## 相关文档

- [项目主文档](../../README.md) —— 环境要求与整体架构
- [配置管理子系统](../general_config/README.md) —— `DiffusionModel`、`VisualEngine`、`FfmpegPath` 配置
- [星图·月华](../../lunar_astral/README.md) —— image_processor 子系统的主要集成使用方