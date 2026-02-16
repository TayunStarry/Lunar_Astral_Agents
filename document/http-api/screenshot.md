# 截图 API 文档

## 1. 概述

截图 API 提供了强大的屏幕截图功能，支持多种截图模式，包括全屏截图、特定显示器截图和区域截图。

## 2. API 端点列表

| 端点                | 方法 | 描述               |
| ------------------- | ---- | ------------------ |
| `/capture`          | POST | 执行截图操作       |
| `/capture/display/` | GET  | 截图特定显示器     |
| `/capture/region`   | GET  | 截图指定区域       |
| `/capture/displays` | GET  | 获取所有显示器信息 |

## 3. API 详细说明

### 3.1 `/capture` - 执行截图操作

**方法**：POST

**请求参数**：

```json
{
  "display_index": -1, // 显示器索引，-1表示所有显示器，0表示第一个显示器
  "region": "", // 区域字符串，格式为 "x,y,width,height"
  "scale": "", // 缩放比例，格式为 "width,height" 或 "0.5"
  "format": "png", // 图像格式，支持 "png", "jpg", "jpeg"
  "quality": 85 // JPEG质量，范围 1-100
}
```

**响应**：

- 成功：返回图像数据
- 失败：返回错误信息

### 3.2 `/capture/display/` - 截图特定显示器

**方法**：GET

**请求参数**：

- `display`：显示器索引
- `format`：图像格式（可选）
- `scale`：缩放比例（可选）

**示例**：

```text
GET /capture/display/0?format=jpg&scale=0.5
```

**响应**：

- 成功：返回指定显示器的图像数据
- 失败：返回错误信息

### 3.3 `/capture/region` - 截图指定区域

**方法**：GET

**请求参数**：

- `region`：区域字符串，格式为 "x,y,width,height"
- `format`：图像格式（可选）
- `scale`：缩放比例（可选）
- `quality`：JPEG质量（可选）

**示例**：

```text
GET /capture/region?region=100,100,500,500&format=png
```

**响应**：

- 成功：返回指定区域的图像数据
- 失败：返回错误信息

### 3.4 `/capture/displays` - 获取所有显示器信息

**方法**：GET

**响应**：

- 成功：返回所有显示器的信息，包括索引、位置和尺寸
- 失败：返回错误信息

**示例响应**：

```json
[
  {
    "index": 0,
    "x": 0,
    "y": 0,
    "width": 1920,
    "height": 1080
  },
  {
    "index": 1,
    "x": 1920,
    "y": 0,
    "width": 1920,
    "height": 1080
  }
]
```

## 4. 使用示例

### 4.1 使用 POST 请求执行全屏截图

```http
POST /capture
Content-Type: application/json

{
  "display_index": 0,
  "format": "jpg",
  "quality": 90
}
```

### 4.2 使用 GET 请求截图特定区域

```http
GET /capture/region?region=0,0,1920,1080&format=png
```

### 4.3 获取所有显示器信息

```http
GET /capture/displays
```

## 5. 错误处理

| 错误代码 | 描述           |
| -------- | -------------- |
| 400      | 无效的请求参数 |
| 404      | 资源未找到     |
| 429      | 截图过于频繁   |
| 500      | 服务器内部错误 |

## 6. 注意事项

1. 截图 API 有频率限制，默认最小截图间隔为 1 秒
2. 截图会自动保存到配置的目录中
3. 支持多种图像格式，建议根据需要选择合适的格式
4. 区域截图时，坐标和尺寸必须为正数
5. 当显示器索引无效时，会自动使用默认显示器
