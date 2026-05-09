# 琉璃接口

本文档介绍琉璃专属的功能接口。

## 加载应用

启动本地应用程序。

- **路径**: `/load/application`
- **方法**: `POST`

### 请求体

```json
{
    "path": "D:/NapCat.Shell.Windows.OneKey/NapCat.41785.Shell/napcat.bat"
}
```

### 请求参数说明

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| path | string | 是 | 应用程序路径（.exe, .ps1, .bat） |

### 响应示例

```json
{
    "success": true,
    "message": "Application started: D:/NapCat.Shell.Windows.OneKey/NapCat.41785.Shell/napcat.bat"
}
```

### 状态码说明

| 状态码 | 说明 |
|--------|------|
| 200 | 应用启动成功 |
| 400 | 不支持的文件类型 |
| 404 | 文件不存在 |
| 500 | 启动失败 |

## 随机背景

获取随机背景图片。

- **路径**: `/background`
- **方法**: `GET`

### 响应

图片二进制数据（JPEG/PNG/GIF/WebP）

### 图片目录

`local_data/images/background/`

## 屏幕截图

对整个屏幕截图。

- **路径**: `/capture/display/`
- **方法**: `GET`

### 响应

PNG 格式图片数据

## 区域截图

对指定区域截图。

- **路径**: `/capture/region`
- **方法**: `POST`

### 请求体

```json
{
    "x": 100,
    "y": 100,
    "width": 800,
    "height": 600
}
```

### 请求参数说明

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| x | int | 是 | 起始 X 坐标 |
| y | int | 是 | 起始 Y 坐标 |
| width | int | 是 | 区域宽度 |
| height | int | 是 | 区域高度 |

## 获取屏幕列表

获取所有可用显示器信息。

- **路径**: `/capture/displays`
- **方法**: `GET`

### 响应示例

```json
{
    "displays": [
        {"index": 0, "width": 1920, "height": 1080}
    ]
}
```

## 图片缩放

缩放图片尺寸。

- **路径**: `/resize`
- **方法**: `POST`

### 请求体

```json
{
    "path": "local_data/images/sample.png",
    "width": 512,
    "height": 512,
    "keep_aspect_ratio": true
}
```

### 请求参数说明

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| path | string | 是 | 图片路径 |
| width | int | 是 | 目标宽度 |
| height | int | 是 | 目标高度 |
| keep_aspect_ratio | bool | 否 | 是否保持宽高比 |

---

*文档版本：1.0 | 最后更新：2026-05-09*

[返回 API 索引](./index.md) | [下一篇：WebSocket](./websocket.md)
