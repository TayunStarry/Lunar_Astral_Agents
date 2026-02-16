# 模型列表接口

**功能**：获取当前加载的GGUF模型列表

**请求方式**: GET
**路径**: `/v1/models`

## 响应示例 (200 OK)

```json
{
  "object": "list",
  "data": [
    {
      "id": "chat.gguf",
      "object": "model",
      "owned_by": "organization_owner"
    },
    {
      "id": "text-embedding.gguf",
      "object": "model",
      "owned_by": "organization_owner"
    }
  ]
}
```

## 示例代码

```javascript
fetch("/v1/models")
  .then(res => res.json())
  .then(data => console.log("加载的模型:", data.data.map((m) => m.id)))
  .catch(error => console.error("读取模型失败:", error));
```
