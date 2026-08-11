# 子系统——图像处理（image）

图像生成与视频关键帧提取的共享库模块，封装 stable-diffusion.cpp 调用流程与视频帧截取逻辑，采用 Module（逻辑层）+ Server（HTTP 层）二层架构。

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

`image` 子系统是图像生成与视频处理能力的共享库，为星图·月华提供文生图和视频关键帧提取能力。

| 功能 | 说明 |
|------|------|
| 扩散图像生成 | 调用 stable-diffusion.cpp 命令行引擎，支持文生图 |
| 视频关键帧提取 | 从视频文件中智能提取关键帧图片 |
| 图像格式处理 | Base64 编解码、像素格式转换、着色处理 |

---

## 项目结构

<div style="font-family: 'Cascadia Code', 'SF Mono', Consolas, monospace; font-size: 0.9em; line-height: 1.6;">
  <ul style="list-style-type: none; padding-left: 0;">
    <li><strong>image/</strong></li>
    <li style="padding-left: 1.5em;"><code>go.mod</code> / <code>go.sum</code> <span style="color: #6a737d;">— Go 模块定义</span></li>
    <li style="padding-left: 1.5em;"><strong>module/</strong> <span style="color: #6a737d;">— 核心业务逻辑层</span>
      <ul style="list-style-type: none; padding-left: 1.5em;">
        <li><code>generate.go</code> <span style="color: #6a737d;">— 扩散图像生成逻辑（调用 sd-cli 引擎）</span></li>
        <li><code>keyframe.go</code> <span style="color: #6a737d;">— 视频关键帧提取（FFmpeg 辅助）</span></li>
        <li><code>type.go</code> <span style="color: #6a737d;">— 数据类型定义</span></li>
        <li><code>variable.go</code> <span style="color: #6a737d;">— 全局变量与初始化</span></li>
      </ul>
    </li>
    <li style="padding-left: 1.5em;"><strong>server/</strong> <span style="color: #6a737d;">— HTTP 服务层</span>
      <ul style="list-style-type: none; padding-left: 1.5em;">
        <li><code>generate.go</code> <span style="color: #6a737d;">— 图像生成 HTTP 处理器</span></li>
        <li><code>keyframe.go</code> <span style="color: #6a737d;">— 关键帧提取 HTTP 处理器</span></li>
        <li><code>type.go</code> <span style="color: #6a737d;">— HTTP 请求/响应类型定义</span></li>
      </ul>
    </li>
  </ul>
</div>

### 依赖关系

<div style="font-family: 'Cascadia Code', 'SF Mono', Consolas, monospace; font-size: 0.9em; line-height: 1.6;">
  <ul style="list-style-type: none; padding-left: 0;">
    <li><code>image</code></li>
    <li style="padding-left: 1.5em;"><code>config</code> <span style="color: #6a737d;">(../config) — 配置管理（扩散模型路径、图像参数）</span></li>
    <li style="padding-left: 1.5em;"><code>logger</code> <span style="color: #6a737d;">(../logger) — 彩色终端日志</span></li>
    <li style="padding-left: 1.5em;"><strong>外部依赖</strong>
      <ul style="list-style-type: none; padding-left: 1.5em;">
        <li><code>sd-cli.exe</code> <span style="color: #6a737d;">— stable-diffusion.cpp 命令行引擎</span></li>
        <li><code>ffmpeg</code> <span style="color: #6a737d;">— 视频解码与帧提取</span></li>
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
│  GenerateHandler  KeyframeHandler  ...           │
│  解析 HTTP 请求 → 调用 module 函数 → 写 HTTP 响应  │
└──────────────────────┬──────────────────────────┘
                       │
┌──────────────────────▼──────────────────────────┐
│          业务逻辑层（module/）                     │
│  图像生成: prompt → sd-cli 引擎 → JPEG 输出        │
│  关键帧提取: 视频 → FFmpeg 解码 → 帧图片           │
│  安全机制: 路径校验 / 格式验证 / 超时控制           │
└──────────────────────┬──────────────────────────┘
                       │
          ┌────────────┼────────────┐
          ▼            ▼            ▼
     ┌─────────┐  ┌──────────┐  ┌──────────┐
     │ sd-cli  │  │  ffmpeg  │  │  config  │
     │外部引擎 │  │外部工具   │  │配置模块  │
     └─────────┘  └──────────┘  └──────────┘
```

### 图像生成流水线

```
用户请求
    │  (prompt_text, negative_prompt, steps, seed, ...)
    ▼
module.GenerateImage()
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
    └── ⑥ 返回编码后的图片数据（JSON → server handler → HTTP response）
```

### 视频关键帧提取流程

```
视频文件
    │
    ▼
module.ExtractKeyframes()
    │
    ├── ffprobe 获取视频时长与关键帧信息
    ├── ffmpeg 按时间间隔提取帧 → 临时图片文件
    ├── 图片读取与 Base64 编码
    ├── 清理临时文件
    └── 返回帧图片数组
```

---

## 核心模块说明

### module/generate.go — 图像生成

| 函数 | 说明 |
|------|------|
| `GenerateImage(prompt, negativePrompt, width, height, steps, seed)` | 主生成函数：构建参数 → 调用引擎 → 返回图片 |
| `EncodeToBase64(imageBytes)` | 图片 Base64 编码为 Data URI |
| `DecodeBase64(dataURI)` | Data URI 解码为原始字节 |

### module/keyframe.go — 关键帧提取

| 函数 | 说明 |
|------|------|
| `ExtractKeyframes(videoPath, interval)` | 从视频按时间间隔提取关键帧 |
| `GetVideoInfo(videoPath)` | 获取视频基本信息（时长、分辨率、编码） |

---

## API 接口定义

### HTTP 端点

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/generate` | 扩散图像生成 |
| POST | `/video` | 视频上传与关键帧提取 |

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
POST /video
Content-Type: multipart/form-data

# 表单字段：video = <视频文件>
```

### 视频关键帧响应

```json
{
  "success": true,
  "frames": [
    "data:image/jpeg;base64,...",
    "data:image/jpeg;base64,..."
  ],
  "duration": 30.5,
  "frame_count": 10
}
```

---

## 使用示例

### Go 代码中使用

```go
package main

import (
    "image/module"
)

func main() {
    // 图像生成
    imageBytes, err := module.GenerateImage(
        "月光下的樱花树",
        "模糊，畸形",
        512, 512,
        20, 42, 7.5,
    )
    if err != nil {
        panic(err)
    }

    // 编码为 Data URI
    dataURI := module.EncodeToBase64(imageBytes)
    
    // 关键帧提取
    frames, err := module.ExtractKeyframes("video.mp4", 3.0)
    if err != nil {
        panic(err)
    }
}
```

---

## 常见问题

### Q: sd-cli.exe 在哪里？

sd-cli.exe 由 [sd_lunar 子系统](../sd_lunar/README.md) 编译生成，位于 `cpp/build/bin/sd-cli.exe`。图像生成模块通过 [config 子系统](../config/README.md) 中的 `VisualEngine` 路径配置找到它。

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

---

## 相关文档

- [项目主文档](../../README.md) —— 环境要求与整体架构
- [配置管理子系统](../config/README.md) —— `DiffusionModel`、`VisualEngine`、`FfmpegPath` 配置
- [SD 图像生成引擎](../sd_lunar/README.md) —— stable-diffusion.cpp C++ 推理引擎
- [星图·月华](../../lunar_astral/README.md) —— image 子系统的主要集成使用方