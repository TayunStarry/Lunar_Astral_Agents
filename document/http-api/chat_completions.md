# 聊天补全接口

**功能**：提供对话式文本生成服务

**请求方式**: POST
**路径**: `/v1/chat/completions`

## 请求参数

| 参数名     | 类型    | 必填 | 说明                        |
| ---------- | ------- | ---- | --------------------------- |
| `model`    | string  | 是   | 模型名称                    |
| `messages` | array   | 是   | 对话消息数组                |
| `stream`   | boolean | 否   | 是否启用流式输出，默认false |

### 消息对象结构

```json
{
  "role": "system|user|assistant",
  "content": "消息内容"
}
```

## 请求体示例

```json
{
  "model": "llama-2-7b-chat.gguf",
  "messages": [
    { "role": "system", "content": "你是一个 helpful 的助手" },
    { "role": "user", "content": "Hello!" }
  ],
  "stream": false
}
```

## 响应字段说明

| 字段名    | 类型   | 说明          |
| --------- | ------ | ------------- |
| `id`      | string | 请求ID        |
| `object`  | string | 对象类型      |
| `created` | number | 创建时间戳    |
| `model`   | string | 模型名称      |
| `choices` | array  | 生成结果数组  |
| `usage`   | object | token使用情况 |

### Choice对象结构

```json
{
  "index": 0,
  "message": {
    "role": "assistant",
    "content": "回复内容"
  },
  "finish_reason": "stop|length|content_filter"
}
```

## 响应示例 (200 OK)

```json
{
  "id": "Champlain-123",
  "object": "chat.completion",
  "created": 1677652288,
  "model": "llama-2-7b-chat.gguf",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": "Hello! How can I help you today?"
      },
      "finish_reason": "stop"
    }
  ],
  "usage": {
    "prompt_tokens": 24,
    "completion_tokens": 16,
    "total_tokens": 40
  }
}
```

## 错误响应

| 状态码 | 说明           |
| ------ | -------------- |
| `400`  | 请求参数错误   |
| `404`  | 模型不存在     |
| `500`  | 服务器内部错误 |

```json
{
  "error": {
    "code": 404,
    "message": "模型不存在",
    "type": "model_not_found"
  }
}
```

## 流式响应示例

当 `stream=true` 时，返回 Server-Sent Events 格式数据：

```text
data: {"id":"chatcmpl-123","object":"chat.completion.chunk","created":1694268190,"model":"gpt-3.5-turbo-0613","choices":[{"index":0,"delta":{"content":"Hello"},"finish_reason":null}]}

data: [DONE]
```
