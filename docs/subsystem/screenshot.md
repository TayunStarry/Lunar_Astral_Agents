# Screenshot子系统 - 屏幕截图功能文档

> 📸 **Screenshot子系统**是星月智能的屏幕截图模块，负责处理各种屏幕截图请求，支持全屏、区域和窗口截图。

---

## 🏗️ 架构设计

### 模块结构

```
subsystem/screenshot/
├── module.go    # 核心截图逻辑
├── server.go    # HTTP服务接口
├── type.go      # 类型定义
├── go.mod
└── go.sum
```

---

## 📋 API接口

### 基础路径

所有API接口的基础路径为：`http://localhost:{port}/`

---

### 1. 通用截图接口

#### POST /capture

**功能**：执行通用截图

**请求头**：
```
Content-Type: application/json
```

**请求体**：
```json
{
  "format": "png",
  "quality": 95
}
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `format` | string | 否 | 图片格式：png/jpg/webp，默认png |
| `quality` | int | 否 | 图片质量1-100，默认95 |

**响应**：
- Content-Type: image/{format}
- Body: 图片二进制数据

---

### 2. 屏幕列表接口

#### GET /capture/displays

**功能**：获取可用屏幕列表

**响应格式**：
```json
{
  "success": true,
  "displays": [
    {
      "id": 0,
      "width": 1920,
      "height": 1080,
      "primary": true,
      "name": "\\\\.\\DISPLAY1"
    },
    {
      "id": 1,
      "width": 1280,
      "height": 720,
      "primary": false,
      "name": "\\\\.\\DISPLAY2"
    }
  ]
}
```

---

### 3. 指定屏幕截图接口

#### GET /capture/display/{display_id}

**功能**：截取指定屏幕

**路径参数**：
| 参数 | 类型 | 说明 |
|------|------|------|
| `display_id` | int | 显示器ID（从0开始） |

**响应**：
- Content-Type: image/png
- Body: 图片二进制数据

---

### 4. 区域截图接口

#### POST /capture/region

**功能**：截取屏幕指定区域

**请求体**：
```json
{
  "x": 100,
  "y": 100,
  "width": 500,
  "height": 300,
  "display_id": 0,
  "format": "png"
}
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `x` | int | 是 | 区域左上角X坐标 |
| `y` | int | 是 | 区域左上角Y坐标 |
| `width` | int | 是 | 区域宽度 |
| `height` | int | 是 | 区域高度 |
| `display_id` | int | 否 | 屏幕ID，默认主屏幕 |
| `format` | string | 否 | 图片格式，默认png |

**响应**：
- Content-Type: image/{format}
- Body: 图片二进制数据

---

## 🔧 使用示例

### Go语言调用

```go
// 创建截图请求
req := screenshot.Request{
    Format: "png",
    Quality: 95,
}

// 执行截图
imageData, err := screenshot.Capture(req)
if err != nil {
    log.Fatal(err)
}

// 保存图片
os.WriteFile("screenshot.png", imageData, 0644)
```

### HTTP请求示例

```powershell
# 全屏截图
Invoke-RestMethod -Uri "http://localhost:8080/capture" -Method POST

# 获取屏幕列表
Invoke-RestMethod -Uri "http://localhost:8080/capture/displays" -Method GET

# 区域截图
$body = @{
    x = 100
    y = 100
    width = 500
    height = 300
} | ConvertTo-Json
Invoke-RestMethod -Uri "http://localhost:8080/capture/region" -Method POST -Body $body
```

---

## 📁 类型定义

### Request

```go
type Request struct {
    Format  string // 图片格式：png, jpg, webp
    Quality int    // 图片质量 1-100
}
```

### Display

```go
type Display struct {
    ID      int    // 显示器ID
    Width   int    // 宽度
    Height  int    // 高度
    Primary bool   // 是否为主屏幕
    Name    string // 显示器名称
}
```

---

## 🔗 关联文档

- [主项目README](../../README.md)
- [星图·琉璃文档](../crystal_astral.md)
- [星图·月华 文档](../luna_astral.md)
- [存储子系统文档](storage.md)