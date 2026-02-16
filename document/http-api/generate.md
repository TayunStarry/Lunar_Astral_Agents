# 生成服务 API

**功能**：提供图像生成服务，支持文生图和图生图功能

**请求方法**：POST
**路径**：`/generate`

## 请求参数

| 参数名称          | 类型    | 必填 | 默认值 | 描述                                             |
| ----------------- | ------- | ---- | ------ | ------------------------------------------------ |
| `prompt`          | string  | 是   | -      | 生成图像的正面提示词                             |
| `negative_prompt` | string  | 否   | -      | 生成图像的负面提示词                             |
| `batch_size`      | int     | 否   | 1      | 批量生成数量                                     |
| `width`           | int     | 否   | 512    | 生成图像宽度                                     |
| `height`          | int     | 否   | 512    | 生成图像高度                                     |
| `strength`        | float64 | 否   | 0.75   | 图生图时的图像强度                               |
| `steps`           | int     | 否   | 20     | 生成图像的步数                                   |
| `seed`            | int64   | 否   | -1     | 生成图像的种子值                                 |
| `cfg_scale`       | float64 | 否   | 7.0    | 条件引导尺度                                     |
| `init_img`        | string  | 否   | -      | 图生图时的初始图像路径（相对于 local_data 目录） |

## 响应

### 成功响应 (200 OK)

```json
{
  "status": "queued",
  "message": "任务已加入队列",
  "task_id": "task_1234567890",
  "queue_pos": 0
}
```

### 错误响应

| 状态码 | 描述             |
| ------ | ---------------- |
| `400`  | 请求参数错误     |
| `405`  | 不允许的请求方法 |
| `503`  | 任务队列已满     |

## 请求示例

```javascript
// 文生图请求
fetch("/generate", {
  method: "POST",
  headers: {
    "Content-Type": "application/json"
  },
  body: JSON.stringify({
    "prompt": "一只可爱的猫，蓝色眼睛，白色毛发，背景是花园",
    "negative_prompt": "模糊，变形，低质量",
    "width": 512,
    "height": 512,
    "steps": 20,
    "cfg_scale": 7.0
  })
})
.then(response => response.json())
.then(data => {
  console.log("任务已提交:", data.task_id);
  // 使用返回的 task_id 查询任务状态
})
.catch(error => console.error("生成请求失败:", error));
```

## 处理流程

1. 客户端发送生成请求，包含图像生成参数
2. 服务器验证请求参数，创建生成任务
3. 将任务加入队列，返回任务 ID 和队列位置
4. 任务处理器从队列中取出任务，执行图像生成
5. 客户端通过 `task_id` 查询任务状态
6. 任务完成后，客户端可以获取生成结果

## 注意事项

- 生成服务依赖于 Stable Diffusion 模型，需要足够的显存（建议 8GB+）
- 生成过程可能需要较长时间，建议使用异步方式处理
- 生成结果默认保存在 `local_data/generated/` 目录下
- 可以通过 `/generate/status` 端点查询任务状态
- 支持文生图和图生图两种模式
