# 文本补全接口

**功能**：提供文本续写服务

**请求方式**: POST
**路径**: `/v1/completions`

## 请求体

```json
{
  "model": "llama-2-7b-chat.gguf",
  "prompt": "Once upon a time"
}
```

## 响应示例 (200 OK)

```json
{
  "id": "cmpl-123",
  "object": "text_completion",
  "created": 1677652288,
  "model": "llama-2-7b-chat.gguf",
  "choices": [
    {
      "text": ", there was a small village nestled in the mountains...",
      "index": 0,
      "finish_reason": "length"
    }
  ],
  "usage": {
    "prompt_tokens": 5,
    "completion_tokens": 100,
    "total_tokens": 105
  }
}
```
