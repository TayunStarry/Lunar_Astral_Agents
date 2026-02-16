# 任务状态查询 API

**功能**：查询图像生成任务的状态和结果

**请求方法**：GET
**路径**：`/generate/status`

## 请求参数

| 参数名称  | 类型   | 必填 | 默认值 | 描述            |
| --------- | ------ | ---- | ------ | --------------- |
| `task_id` | string | 是   | -      | 要查询的任务 ID |

## 响应

### 成功响应 (200 OK)

```json
{
  "task_id": "task_1234567890",
  "status": "completed",
  "created": "2023-10-01T12:00:00Z",
  "result": "local_data/generated/generated_20231001_120000.png",
  "error": ""
}
```

### 错误响应

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

```javascript
// 查询任务状态
function checkTaskStatus(taskId) {
  fetch(`/generate/status?task_id=${taskId}`)
    .then(response => {
      if (!response.ok) {
        throw new Error(`HTTP error! status: ${response.status}`);
      }
      return response.json();
    })
    .then(data => {
      console.log("任务状态:", data.status);

      if (data.status === "completed") {
        console.log("生成结果:", data.result);
        // 处理生成结果
      } else if (data.status === "failed") {
        console.error("生成失败:", data.error);
        // 处理错误情况
      } else {
        // 任务仍在处理中，继续查询
        setTimeout(() => checkTaskStatus(taskId), 2000);
      }
    })
    .catch(error => {
      console.error("查询任务状态失败:", error);
    });
}

// 使用示例
checkTaskStatus("task_1234567890");
```

## 处理流程

1. 客户端发送 GET 请求，包含要查询的 task_id
2. 服务器验证 task_id 参数
3. 检查灵绘坊功能是否启用
4. 查询任务状态
5. 返回任务状态、创建时间、结果路径（如果已完成）和错误信息（如果失败）

## 注意事项

- 任务状态保留在内存中，重启服务器后会丢失
- 建议在任务完成后及时保存结果路径，以便后续访问
- 可以通过轮询方式查询任务状态，建议间隔 2-5 秒
- 生成结果可以通过 `/download/` API 下载
