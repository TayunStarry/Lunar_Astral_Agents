# 等待图片生成接口

**功能**：通过WebSocket连接等待图片生成完成，并在完成后返回结果

**请求方法**：GET
**路径**：`/generate/wait`

## 请求参数

| 参数名称  | 类型   | 必填 | 默认值 | 描述            |
| --------- | ------ | ---- | ------ | --------------- |
| `task_id` | string | 是   | -      | 要等待的任务 ID |

## 响应

### 成功响应 (200 OK)

当任务已完成时，返回JSON格式：

```json
{
  "task_id": "task_1234567890",
  "status": "completed",
  "result": "local_data/generated/generated_20231001_120000.png",
  "error": "",
  "read_path": "/read/generated/generated_20231001_120000.png"
}
```

当任务执行失败时，返回JSON格式：

```json
{
  "task_id": "task_1234567890",
  "status": "failed",
  "result": "",
  "error": "执行失败的原因",
  "read_path": ""
}
```

当任务正在执行时，使用WebSocket（SSE）返回事件流：

```
data: {"task_id": "task_1234567890", "status": "completed", "result": "local_data/generated/generated_20231001_120000.png", "error": "", "read_path": "/read/generated/generated_20231001_120000.png"}

```

## 错误响应

| 状态码 | 描述               |
| ------ | ------------------ |
| `400`  | 缺少 task_id 参数  |
| `405`  | 不允许的请求方法   |
| `404`  | 任务不存在         |
| `503`  | 灵绘坊功能未启用 |

## 任务状态说明

| 状态        | 描述                       |
| ----------- | -------------------------- |
| `queued`    | 任务已加入队列，等待处理   |
| `running`   | 任务正在处理中             |
| `completed` | 任务已完成，生成结果可用   |
| `failed`    | 任务执行失败，错误信息可用 |

## 请求示例

### 基本请求示例

```javascript
// 等待任务完成
fetch('/generate/wait?task_id=task_1234567890')
  .then(response => {
    if (!response.ok) {
      throw new Error(`HTTP error! status: ${response.status}`);
    }
    return response.json();
  })
  .then(data => {
    console.log('任务状态:', data.status);

    if (data.status === 'completed') {
      console.log('生成结果:', data.result);
      console.log('读取路径:', data.read_path);
      // 使用读取路径获取图片
      fetch(data.read_path)
        .then(res => res.blob())
        .then(blob => {
          const img = document.createElement('img');
          img.src = URL.createObjectURL(blob);
          document.body.appendChild(img);
        });
    } else if (data.status === 'failed') {
      console.error('生成失败:', data.error);
    }
  })
  .catch(error => {
    console.error('请求失败:', error);
  });
```

### WebSocket (SSE) 示例

```javascript
// 使用服务器发送事件(SSE)等待任务完成
function waitForTaskCompletion(taskId) {
  const eventSource = new EventSource(`/generate/wait?task_id=${taskId}`);

  eventSource.onmessage = function(event) {
    const data = JSON.parse(event.data);
    console.log('任务状态:', data.status);

    if (data.status === 'completed') {
      console.log('生成结果:', data.result);
      console.log('读取路径:', data.read_path);
      // 使用读取路径获取图片
      fetch(data.read_path)
        .then(res => res.blob())
        .then(blob => {
          const img = document.createElement('img');
          img.src = URL.createObjectURL(blob);
          document.body.appendChild(img);
        });
      eventSource.close();
    } else if (data.status === 'failed') {
      console.error('生成失败:', data.error);
      eventSource.close();
    }
  };

  eventSource.onerror = function(error) {
    console.error('SSE 错误:', error);
    eventSource.close();
  };
}

// 使用示例
waitForTaskCompletion('task_1234567890');
```

## 处理流程

1. 客户端发送 GET 请求到 `/generate/wait?task_id=任务ID`
2. 服务器验证 `task_id` 参数
3. 检查灵绘坊功能是否启用
4. 查询任务状态
5. 如果任务已完成或失败，直接返回 JSON 格式的结果
6. 如果任务正在执行，建立 WebSocket (SSE) 连接
7. 当任务完成或失败时，通过 WebSocket (SSE) 发送结果
8. 客户端接收结果并处理

## 注意事项

- 此接口使用服务器发送事件(SSE)技术，模拟 WebSocket 连接
- 连接会在任务完成或失败后自动关闭
- 生成结果中的 `read_path` 可以直接用于获取生成的图片
- 建议在任务创建后立即调用此接口等待结果