# 视频关键帧提取接口

**功能**：接收客户端上传的视频文件，提取关键帧，并将结果作为文件数组返回给客户端，文件不写入硬盘。

**请求方式**: POST
**路径**: `/extract/keyframes`

## 请求头

| 字段名           | 必填 | 默认值 | 说明                           |
| ---------------- | ---- | ------ | ------------------------------ |
| `Content-Type`   | 是   | -      | 表单数据类型，必须为 `multipart/form-data` |
| `Content-Length` | 是   | -      | 请求体大小（字节）             |

## 请求体

| 字段名 | 类型     | 必填 | 说明                           |
| ------ | -------- | ---- | ------------------------------ |
| `video` | 文件     | 是   | 要处理的视频文件，必须为 MP4 格式 |

## 响应示例 (200 OK)

```json
{
  "keyFrames": [
    {
      "filePath": "key_frame_1.jpg",
      "timestamp": "00:00:00",
      "frameNum": 0,
      "data": "<base64-encoded image data>"
    },
    {
      "filePath": "key_frame_2.jpg",
      "timestamp": "00:00:05",
      "frameNum": 5,
      "data": "<base64-encoded image data>"
    },
    {
      "filePath": "key_frame_3.jpg",
      "timestamp": "00:00:12",
      "frameNum": 12,
      "data": "<base64-encoded image data>"
    }
  ],
  "count": 3
}
```

## 处理流程

1. 接收客户端上传的视频文件
2. 验证文件格式是否为 MP4
3. 创建临时文件存储上传的视频
4. 获取视频时长
5. 按秒提取视频帧
6. 计算相邻帧之间的差异
7. 当差异超过阈值时，将帧标记为关键帧
8. 将关键帧编码为 JPEG 并存储在内存中
9. 清理临时文件
10. 将关键帧数组返回给客户端

## 错误响应

| 状态码 | 说明                 |
| ------ | -------------------- |
| `400`  | 不允许的请求方法或文件格式错误 |
| `413`  | 文件过大             |
| `500`  | 服务器错误，如提取关键帧失败 |

```json
{
  "error": {
    "code": 400,
    "message": "输入文件必须是MP4视频",
    "details": "invalid file format"
  }
}
```

## 完整示例代码

### 前端上传示例

```javascript
class VideoKeyFrameExtractor {
  // 提取视频关键帧
  static async extractKeyFrames(videoFile) {
    // 检查文件格式
    if (!videoFile.name.endsWith('.mp4')) {
      throw new Error('视频文件必须是MP4格式');
    }

    // 创建FormData对象
    const formData = new FormData();
    formData.append('video', videoFile);

    // 发送请求
    const response = await fetch('/extract/keyframes', {
      method: 'POST',
      body: formData,
    });

    // 检查响应状态
    if (!response.ok) {
      const error = await response.json();
      throw new Error(error.message || '提取关键帧失败');
    }

    // 解析响应
    return await response.json();
  }

  // 显示提取的关键帧
  static displayKeyFrames(keyFrames) {
    const container = document.getElementById('keyframes-container');
    container.innerHTML = '';

    keyFrames.forEach((frame, index) => {
      // 创建图像元素
      const img = document.createElement('img');
      img.src = `data:image/jpeg;base64,${btoa(String.fromCharCode(...new Uint8Array(frame.data)))}`;
      img.alt = `Key frame ${index + 1}`;
      img.style.maxWidth = '200px';
      img.style.margin = '10px';

      // 创建信息元素
      const info = document.createElement('div');
      info.textContent = `${frame.filePath} (${frame.timestamp})`;

      // 添加到容器
      const frameElement = document.createElement('div');
      frameElement.appendChild(img);
      frameElement.appendChild(info);
      container.appendChild(frameElement);
    });
  }
}

// 使用示例
const videoInput = document.getElementById('videoInput');
videoInput.addEventListener('change', async (event) => {
  const videoFile = event.target.files[0];
  if (!videoFile) return;

  try {
    // 显示加载状态
    document.getElementById('status').textContent = '正在提取关键帧...';

    // 提取关键帧
    const result = await VideoKeyFrameExtractor.extractKeyFrames(videoFile);

    // 显示结果
    document.getElementById('status').textContent = `成功提取 ${result.count} 个关键帧`;
    VideoKeyFrameExtractor.displayKeyFrames(result.keyFrames);
  } catch (error) {
    // 显示错误信息
    document.getElementById('status').textContent = `错误: ${error.message}`;
    console.error('提取关键帧失败:', error);
  }
});
```

### 命令行上传示例

```bash
# 使用 curl 上传视频文件并提取关键帧
curl -X POST http://localhost:36789/extract/keyframes \
  -F "video=@./sample.mp4" \
  -o keyframes.json

# 查看结果
cat keyframes.json
```

## 注意事项

- 只支持 MP4 格式的视频文件
- 视频文件大小建议不超过 100MB（可在服务器配置中调整）
- 提取的关键帧数量取决于视频内容的变化程度
- 响应中的图像数据为 Base64 编码的 JPEG 格式
- 整个处理过程在内存中完成，不会在服务器上留下临时文件
- 处理大视频可能会占用较多系统资源，请确保服务器有足够的内存
- 提取关键帧的速度取决于视频长度和系统性能

## 技术实现细节

- 使用 FFmpeg 提取视频帧
- 使用内存缓冲区存储提取的帧
- 通过计算相邻帧的像素差异来识别关键帧
- 差异阈值默认为 0.15，超过此值的帧被认为是关键帧
- 第一帧总是被标记为关键帧
- 提取的帧会被编码为 JPEG 格式并存储在内存中
- 最终结果以 JSON 格式返回给客户端

## 性能优化建议

- 对于长视频，考虑分批次处理
- 前端可以添加进度条，提高用户体验
- 服务器端可以考虑使用并发处理来提高效率
- 对于大型应用，可以考虑使用队列系统来处理视频请求