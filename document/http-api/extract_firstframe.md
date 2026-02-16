# 视频第一帧提取接口

**功能**：提取上传视频的第一帧图像

**请求方式**: POST
**路径**: `/extract/firstframe`

## 请求格式

- **Content-Type**: `multipart/form-data`
- **参数**:
  - `video` (必需): 要提取第一帧的视频文件

## 支持的视频格式

- `.mp4`, `.avi`, `.mov`, `.wmv`, `.flv`, `.mkv`, `.webm`, `.m4v`

## 响应示例 (200 OK)

```json
{
  "firstFrame": {
    "filePath": "first_frame.jpg",
    "timestamp": "00:00:00",
    "frameNum": 1,
    "data": "base64编码的图像数据"
  }
}
```

### 字段说明

| 字段名     | 类型   | 说明                  |
| ---------- | ------ | --------------------- |
| `filePath` | string | 第一帧文件名          |
| `timestamp`| string | 时间戳 (HH:MM:SS 格式)|
| `frameNum` | int    | 帧编号                |
| `data`     | byte[] | 图像数据 (JPEG 格式)  |

## 错误响应

| 状态码 | 描述               |
| ------ | ------------------ |
| `400`  | 请求参数错误       |
| `405`  | 不允许的请求方法   |
| `500`  | 服务器内部错误     |

```json
{
  "error": {
    "code": 400,
    "message": "获取文件失败",
    "details": "no file uploaded"
  }
}
```

## 请求示例

```javascript
// 提取视频第一帧
const formData = new FormData();
formData.append("video", videoFile);

fetch("/extract/firstframe", {
  method: "POST",
  body: formData,
})
  .then((response) => {
    if (!response.ok) throw new Error("提取第一帧失败");
    return response.json();
  })
  .then((data) => {
    console.log("提取第一帧成功:", data);

    // 处理返回的第一帧数据
    const firstFrame = data.firstFrame;
    if (firstFrame.data) {
      // 转换base64数据为Blob
      const blob = new Blob([new Uint8Array(firstFrame.data)], { type: "image/jpeg" });
      const imageUrl = URL.createObjectURL(blob);

      // 显示图像
      const img = document.createElement("img");
      img.src = imageUrl;
      document.body.appendChild(img);
    }
  })
  .catch((error) => console.error("提取第一帧失败:", error));
```

## 处理流程

1. 接收并解析上传的视频文件
2. 检查视频格式是否支持
3. 创建临时文件存储视频
4. 使用FFmpeg提取视频第一帧
5. 解码并优化提取的图像
6. 返回第一帧数据

## 注意事项

- 视频文件大小建议不超过100MB
- 提取过程可能需要一定时间，取决于视频大小
- 返回的图像数据为JPEG格式，质量设置为85
- 图像宽度和高度会保持原始视频的比例
- 接口默认返回第一帧图像的base64编码数据
