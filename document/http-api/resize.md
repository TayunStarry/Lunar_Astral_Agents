# 图片缩放 API

## 接口描述

该接口用于接收图片文件并进行等比例缩放处理，确保缩放后的图片任意一个方向上的尺寸不会大于1080。

## 请求方式

- **请求方法**: POST
- **请求路径**: `/resize`
- **Content-Type**: multipart/form-data

## 请求参数

| 参数名 | 类型 | 必填 | 描述 |
| :--- | :--- | :--- | :--- |
| `image` | file | 是 | 要缩放的图片文件 |

## 响应参数

| 参数名 | 类型 | 描述 |
| :--- | :--- | :--- |
| `image` | binary | 缩放后的图片数据 |
| `base64` | string | 包含数据类型头的base64编码 |
| `format` | string | 图片格式 |
| `width` | number | 缩放后的宽度 |
| `height` | number | 缩放后的高度 |

## 请求示例

```bash
# 使用curl命令发送请求
curl -k -X POST "https://localhost:36789/resize" \
  -F "image=@path/to/image.jpg"
```

## 响应示例

```json
{
  "image": "[binary data]",
  "base64": "data:image/jpeg;base64,/9j/4AAQSkZJRgABAQAAAQABAAD...",
  "format": "jpeg",
  "width": 1080,
  "height": 720
}
```

## 注意事项

1. 接口支持常见的图片格式，如JPEG、PNG等。
2. 接口会自动检测图片格式并进行相应的编码。
3. 缩放过程保持图片的原始比例。
4. 响应中的`image`字段包含二进制图片数据，`base64`字段包含可直接用于HTML img标签的base64编码。
