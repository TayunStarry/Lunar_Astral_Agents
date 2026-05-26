# 子系统——屏幕截图（screenshot）

跨平台屏幕截取与图像处理模块，支持多显示器拼接、区域截图、高质量缩放与 JPEG/PNG 编码。

---

## 目录

- [功能概述](#功能概述)
- [核心模块说明](#核心模块说明)
- [API 接口定义](#api-接口定义)
- [使用示例](#使用示例)
- [常见问题](#常见问题)

---

## 功能概述

`screenshot` 子系统提供完整的桌面截屏能力：

| 功能 | 说明 |
|------|------|
| 全屏截图 | 截取所有显示器并智能拼接 |
| 指定显示器 | 按索引截取单个显示器 |
| 区域截图 | 指定坐标 (x, y, w, h) 截取指定区域 |
| 显示器枚举 | 获取所有连接的显示器信息（尺寸、位置） |
| 图片缩放 | 高质量 Lanczos 缩放 + 1080p 自适应压缩 |
| 格式编码 | 支持 JPEG（可调质量）和 PNG 格式 |
| 频率限制 | 50ms 冷却期防止过度截取 |

---

## 核心模块说明

### 文件职责

| 文件 | 职责 |
|------|------|
| [type.go](type.go) | 请求/响应结构体定义 + 频率限制变量 |
| [module.go](module.go) | 核心截图逻辑 + 多显示器拼接 + 缩放 + 编码 |
| [server.go](server.go) | HTTP 处理器层（路由分发） |

### 截图决策树

```
Screenshot(req)
    │
    ├── checkScreenshotRateLimit()    ← 50ms 冷却检查
    │
    ├── req.Region != ""
    │   └── screenshot.CaptureRect()   ← 区域截图
    │
    ├── req.DisplayIndex >= 0
    │   └── CaptureDisplay(index)      ← 指定显示器
    │
    ├── req.DisplayIndex == -1
    │   └── screenshotAllDisplaysOptimized()  ← 多屏拼接
    │       ├── 获取所有显示器边界
    │       ├── 计算总边界 (minX, minY) → (maxX, maxY)
    │       ├── 创建 totalWidth × totalHeight 画布
    │       ├── 逐个截取每个显示器
    │       └── draw.Draw 绘制到总图正确位置
    │
    └── 默认
        └── CaptureDisplay(0)         ← 主显示器
```

### 多显示器拼接算法

```
1. NumActiveDisplays() → 获取所有显示器边界
2. 遍历计算:
   minX, minY = min(所有显示器左上角坐标)
   maxX, maxY = max(所有显示器右下角坐标)
3. totalWidth  = maxX - minX
   totalHeight = maxY - minY
4. 创建总画布 image.NewRGBA(totalWidth, totalHeight)
5. 逐个截取每个显示器:
   img = CaptureDisplay(i)
   dx = bounds.Min.X - minX
   dy = bounds.Min.Y - minY
   draw.Draw(总画布, img 区域, img, origin, draw.Src)
6. 单个显示器截图失败不中断，继续处理其余
```

### 缩放处理流水线

```
applyScale(img, scaleStr)
    │
    ├── scaleStr != "" → resizeImage()
    │   ├── 含逗号 → "width,height" 绝对尺寸
    │   └── 不含逗号 → "0.5" 比例缩放
    │   均使用 imaging.Resize (Lanczos 算法)
    │
    └── scaleStr == "" → ResizeToFit(img, MaxWidth, MaxHeight)
        └── 保持宽高比缩小到配置的最大尺寸以内
```

### 频率限制

```go
var (
    captureCooldown = 50 * time.Millisecond  // 50ms 最小截图间隔
    lastCapture     time.Time                // 上次截图时间
    captureMutex    sync.RWMutex             // 读写锁保护
)
```

超频时返回 `429` 状态码并提示剩余等待时间。

---

## API 接口定义

### 截图请求参数

| 参数 | 类型 | 说明 |
|------|------|------|
| `display_index` | `int` | 显示器索引：`-1`=所有显示器，`≥0`=指定显示器 |
| `region` | `string` | 区域坐标：`"x,y,width,height"` |
| `scale` | `string` | 缩放：`"width,height"` 或 `"0.5"`（比例） |
| `format` | `string` | 输出格式：`"png"` / `"jpg"` / `"jpeg"` |
| `quality` | `int` | JPEG 质量：`1`-`100`（默认 `85`） |

### RESTful 端点

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/capture` | 通用截图（通过 JSON body 或 query 参数） |
| GET | `/capture/display/{index}` | 指定显示器截图 |
| GET | `/capture/region?region=x,y,w,h` | 区域截图 |
| GET | `/capture/displays` | 获取显示器列表 |
| POST | `/resize` | 图片缩放到 1080p（multipart `image`） |

### 响应格式

截图接口返回 Base64 编码的 Data URI：

```json
{
  "image": "data:image/jpeg;base64,/9j/4AAQSkZJRg..."
}
```

显示器列表接口返回：

```json
{
  "displays": [
    {"index": 0, "width": 1920, "height": 1080, "x": 0, "y": 0},
    {"index": 1, "width": 1920, "height": 1080, "x": 1920, "y": 0}
  ]
}
```

### 图片缩放接口

```
POST /resize
Content-Type: multipart/form-data
Body:
  image: <图片文件>

Response:
{
  "raw_bytes": "<二进制数据>",
  "base64": "data:image/jpeg;base64,..."
}
```

缩放规则：保持宽高比缩放到最大 1080×1080 以内。

---

## 使用示例

### HTTP API 调用

```bash
# 截取主显示器
curl "http://localhost:36789/capture/display/0"

# 区域截图
curl "http://localhost:36789/capture/region?region=100,100,500,400"

# 全屏截图（所有显示器拼接）
curl -X POST http://localhost:36789/capture \
  -H "Content-Type: application/json" \
  -d '{"display_index": -1, "format": "png"}'

# 缩放截图至 50%
curl -X POST http://localhost:36789/capture \
  -d '{"display_index": 0, "scale": "0.5", "quality": 90}'

# 获取显示器列表
curl http://localhost:36789/capture/displays

# 图片缩放
curl -X POST http://localhost:36789/resize \
  -F "image=@photo.jpg"
```

### Go 代码中使用

```go
package main

import (
    "screenshot"
)

func main() {
    // 截取主显示器
    req := screenshot.ScreenshotRequest{
        DisplayIndex: 0,
        Format:       "jpg",
        Quality:      85,
    }
    imgBytes, mimeType, err := screenshot.Screenshot(req)

    // 缩放截图
    req2 := screenshot.ScreenshotRequest{
        DisplayIndex: 0,
        Scale:        "0.5",
        Format:       "png",
    }
    imgBytes2, _, _ := screenshot.Screenshot(req2)

    // 获取显示器列表
    displays := screenshot.GetDisplays()
}
```

---

## 常见问题

### Q: 截图速度慢怎么办？

1. 缩小截图区域（使用 `region` 参数只截取需要的部分）
2. 降低 JPEG 质量（`quality: 60-70` 通常不影响阅读）
3. 使用 `scale` 参数缩小输出尺寸

### Q: 频率限制如何调整？

频率限制硬编码为 50ms，如需修改请编辑 [module.go](module.go) 中的 `captureCooldown` 常量。

### Q: 多显示器截图显示不正确？

多显示器拼接基于各显示器的 `Bounds()` 坐标。如果显示器布局在系统设置中配置有误（如重叠），可能导致拼接异常。请先检查系统显示设置中的显示器排列。

### Q: 支持哪些图片格式输出？

支持 JPEG（可调质量 1-100）和 PNG 两种格式。默认输出 `config.Format` 指定的格式（默认 "jpg"）。

---

## 相关文档

- [项目主文档](../../README.md) —— 环境要求与整体架构
- [配置管理子系统](../config/README.md) —— `MaxWidth`、`MaxHeight`、`JPEGQuality`、`Format` 配置
- [星图·琉璃](../../crystal_astral/README.md) —— 截图 HTTP 端点使用方