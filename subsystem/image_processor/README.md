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
| 屏幕截图 | 统一截图接口（auto/window/fullscreen/display/region 五种模式），焦点窗口优先、失败降级全屏，支持窗口相对精准区域与缩放格式转换 |
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
        <li><code>capture.go</code> <span style="color: #6a737d;">— 统一截图入口与优先级路由</span></li>
        <li><code>capture_windows.go</code> <span style="color: #6a737d;">— 焦点窗口捕获（Windows 专用，build tag 隔离）</span></li>
        <li><code>capture_stub.go</code> <span style="color: #6a737d;">— 非 Windows 平台窗口捕获占位（返回明确错误）</span></li>
        <li><code>screenshot.go</code> <span style="color: #6a737d;">— 显示器/全屏/区域捕获底层实现与图片缩放（kbinani/screenshot）</span></li>
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
        <li><code>kbinani/screenshot</code> <span style="color: #6a737d;">— 跨平台屏幕捕获（显示器/全屏/区域）</span></li>
        <li><code>lxn/win</code> <span style="color: #6a737d;">— Windows Win32 API 封装（焦点窗口捕获）</span></li>
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
│  HandleCapture  HandleGetDisplays  HandleResize  │
│  解析 HTTP 请求 → 调用 module 函数 → 写 HTTP 响应  │
└──────────────────────┬──────────────────────────┘
                       │
┌──────────────────────▼──────────────────────────┐
│          业务逻辑层（module/）                     │
│  图像生成: prompt → sd-cli 引擎 → JPEG 输出        │
│  关键帧提取: 视频 → FFmpeg 解码 → 帧图片           │
│  屏幕截图: CaptureRequest → 优先级路由 → 图像      │
│  安全机制: 路径校验 / 格式验证 / 超时控制           │
└──────────────────────┬──────────────────────────┘
                       │
          ┌────────────┼────────────┬────────────┐
          ▼            ▼            ▼            ▼
     ┌─────────┐  ┌──────────┐  ┌──────────┐ ┌──────────┐
     │ sd-cli  │  │  ffmpeg  │  │screenshot│ │ lxn/win  │
     │外部引擎 │  │外部工具   │  │ 截图库   │ │窗口捕获  │
     └─────────┘  └──────────┘  └──────────┘ └──────────┘
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
HTTP 请求（CaptureRequest）
    │  (mode, display_index, offset_x/y, width/height, region_x/y/w/h, format, quality, scale)
    ▼
module.Capture()
    │
    ├── ① 频率限制 + 默认值填充（format/quality/mode）
    ├── ② 按 mode 优先级路由：
    │      auto    → 焦点窗口优先，失败降级为多屏拼接全屏
    │      window  → 强制焦点窗口，失败直接报错
    │      fullscreen → 多屏拼接全屏
    │      display → 指定显示器
    │      region  → 绝对屏幕坐标区域
    ├── ③ 窗口相对精准区域覆盖（mode=auto/window 且 width>0 && height>0）
    ├── ④ 按 scale 缩放
    ├── ⑤ 编码为指定格式（png / jpeg）
    └── 返回 CaptureResult（图像字节 + 格式 + 尺寸 + 模式 + 窗口标题）
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

### module/capture.go — 统一截图入口

| 函数 | 说明 |
|------|------|
| `Capture(req CaptureRequest) (CaptureResult, error)` | 统一截图入口：按 mode 优先级路由并返回结构化结果 |
| `captureFocusedWindow(req CaptureRequest) (*image.RGBA, string, error)` | 捕获焦点窗口，含窗口相对精准区域覆盖 |
| `captureDisplay(index int) (*image.RGBA, error)` | 捕获指定显示器 |
| `captureRegion(req CaptureRequest) (*image.RGBA, error)` | 捕获绝对屏幕坐标区域 |
| `normalizeFormat(format string) string` | 归一化格式名称（jpg/jpeg → jpeg） |

### module/capture_windows.go — 焦点窗口捕获（`//go:build windows`）

| 函数 | 说明 |
|------|------|
| `captureForegroundWindow() (*image.RGBA, string, error)` | 捕获焦点窗口整窗（Win32 GetWindowDC + BitBlt） |
| `captureForegroundWindowRegion(offsetX, offsetY, width, height int) (*image.RGBA, string, error)` | 捕获焦点窗口相对精准子区域 |

### module/capture_stub.go — 非 Windows 占位（`//go:build !windows`）

| 函数 | 说明 |
|------|------|
| `captureForegroundWindow()` / `captureForegroundWindowRegion(...)` | 返回「窗口截图仅在 Windows 平台可用」明确错误 |

### module/screenshot.go — 底层捕获与缩放

| 函数 | 说明 |
|------|------|
| `GetDisplays() []map[string]int` | 枚举显示器列表 |
| `screenshotAllDisplaysOptimized() (*image.RGBA, error)` | 多屏拼接全屏截图 |
| `ResizeImage(imgData) ([]map[string]any, error)` | 图片缩放（模型输入路径，≤1024px） |
| `ToRGBA(img) *image.RGBA` | 任意图像转 RGBA |
| `ResizeToFit(img, maxWidth, maxHeight) *image.RGBA` | 等比缩放至指定尺寸 |

`CaptureRequest` 字段：`mode`（auto/window/fullscreen/display/region）、`display_index`、`offset_x/offset_y/width/height`（窗口相对精准区域）、`region_x/region_y/region_w/region_h`（绝对屏幕区域）、`format`、`quality`、`scale`

---

## API 接口定义

### HTTP 端点

由宿主程序（星图·月华 crystal_astral）注册，路径如下：

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/generate` | 扩散图像生成（异步任务） |
| GET | `/generate/wait` | 轮询 / 等待生成任务完成 |
| POST | `/keyframe` | 视频关键帧提取 |
| POST | `/capture` | 统一截图（auto/window/fullscreen/display/region） |
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

`/capture` 支持 `POST`（JSON 请求体）与 `GET`（查询参数）两种方式，字段对齐 `module.CaptureRequest`。

```json
// POST /capture（焦点窗口优先，失败自动降级全屏）
{
  "mode": "auto",           // auto / window / fullscreen / display / region，缺省 auto
  "display_index": 0,       // mode=display 时生效，-1 表示全部
  "offset_x": 0,            // 窗口相对 X 偏移（mode=auto/window，配合 width/height）
  "offset_y": 0,            // 窗口相对 Y 偏移
  "width": 800,             // 窗口相对区域宽度（>0 且 height>0 时启用精准区域）
  "height": 600,            // 窗口相对区域高度
  "region_x": 100,          // 绝对屏幕区域 X（mode=region）
  "region_y": 100,          // 绝对屏幕区域 Y
  "region_w": 400,          // 绝对屏幕区域宽度
  "region_h": 300,          // 绝对屏幕区域高度
  "format": "png",          // png / jpg，缺省取 general_config
  "quality": 90,            // JPEG 质量 1-100，缺省取 general_config
  "scale": ""               // 可选缩放："0.5" 或 "800,600"
}
```

### 截图响应

`/capture` 直接返回图片二进制（`Content-Type` 为 `image/png` 或 `image/jpeg`），文件名按实际模式生成（`screenshot.png` / `screenshot_window.png` / `screenshot_d0.png`）。Go 层 `module.Capture` 则返回结构化 `CaptureResult`：

```go
type CaptureResult struct {
    Image        []byte      // 原始图像字节
    Format       string      // png / jpeg
    ContentType  string      // image/png / image/jpeg
    Width        int         // 最终宽度（缩放后）
    Height       int         // 最终高度（缩放后）
    Mode         CaptureMode // 实际采用的模式（含降级后）
    DisplayIndex int         // display 模式下的显示器索引
    WindowTitle  string      // 焦点窗口标题（window 模式）
}
```

### 接口迁移指南

本次改造已统一截图入口并移除旧接口，旧字段 / 旧端点映射如下：

| 旧接口 | 新接口 |
|--------|--------|
| `module.Screenshot(ScreenshotRequest)` | `module.Capture(CaptureRequest)` |
| `ScreenshotRequest.display_index = -1`（全部显示器） | `mode = "fullscreen"` |
| `ScreenshotRequest.display_index = N`（指定显示器） | `mode = "display"` + `display_index = N` |
| `ScreenshotRequest.region = "x,y,w,h"` | `mode = "region"` + `region_x/region_y/region_w/region_h` |
| `GET /capture/display/{index}` | `GET /capture?mode=display&display_index={index}` |
| `POST /capture/region` | `POST /capture`（`mode = "region"` + 区域字段） |
| 无窗口能力 | `mode = "auto"`（焦点窗口优先）或 `"window"`（强制窗口） |

`scale`、`format`、`quality` 字段语义保持不变；`module.Screenshot` 已移除，返回类型由 `([]byte, string, string, error)` 改为结构化的 `CaptureResult`。

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

    // 统一截图（焦点窗口优先，失败降级全屏）
    result, err := module.Capture(module.CaptureRequest{
        Mode:   module.ModeAuto,
        Format: "png",
    })
    if err != nil {
        panic(err)
    }
    fmt.Println(result.Mode, result.Format, result.Width, result.Height, result.WindowTitle)

    // 窗口相对精准区域截图
    result, err = module.Capture(module.CaptureRequest{
        Mode:    module.ModeWindow,
        OffsetX: 0,
        OffsetY: 0,
        Width:   800,
        Height:  600,
    })
    if err != nil {
        panic(err)
    }
    fmt.Println(len(result.Image))
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

- **焦点窗口捕获**（`auto`/`window` 模式）仅 Windows 平台可用，通过 `//go:build windows` 隔离实现（`lxn/win`），非 Windows 平台返回明确错误。
- **全屏 / 显示器 / 区域截图**基于 `kbinani/screenshot` 库，支持 Windows、macOS、Linux（X11）等主流桌面平台；无显示器环境（如远程会话）可能无法捕获画面。

---

## 相关文档

- [项目主文档](../../README.md) —— 环境要求与整体架构
- [配置管理子系统](../general_config/README.md) —— `DiffusionModel`、`VisualEngine`、`FfmpegPath` 配置
- [星图·月华](../../lunar_astral/README.md) —— image_processor 子系统的主要集成使用方