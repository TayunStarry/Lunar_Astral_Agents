# 嵌入服务接口

**功能**：生成文本嵌入向量

**请求方式**: POST
**路径**: `/v1/embeddings`

## 请求体

```json
{
  "model": "text-embedding.gguf",
  "input": ["宇宙的未来会是什么样子呢?"],
  "task_type": "search_document",
  "dimensionality": 256
}
```

## 参数说明

| 参数名           | 类型   | 必填 | 说明                                                                         |
| ---------------- | ------ | ---- | ---------------------------------------------------------------------------- |
| `model`          | string | 是   | 模型名称，例如`Qwen3-Embedding-0.6B-Q8_0.gguf`                               |
| `input`          | array  | 是   | 输入文本数组，每个元素为一个字符串                                           |
| `task_type`      | string | 否   | 任务类型，可选值：`search_document`(默认)、`search_query`、`text_similarity` |
| `dimensionality` | number | 否   | 嵌入向量维度，可选值：128、256、512、768，默认 256                           |

## 响应示例 (200 OK)

```json
{
  "object": "list",
  "data": [
    {
      "object": "embedding",
      "embedding": [0.0123, -0.0456, 0.0789],
      "index": 0
    }
  ],
  "model": "text-embedding.gguf",
  "usage": {
    "prompt_tokens": 11,
    "total_tokens": 11
  }
}
```
