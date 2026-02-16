# 数据结构

## HistoryMessage

```json
{
  "role": "string",
  "content": "string",
  "isPrompt": false,
  "noRender": false,
  "imageUrl": "string",
  "deletable": true,
  "uuid": "string",
  "embedVector": [0.1, 0.2, ..., 0.5]
}
```

## KnowledgeMessage

```json
{
  "role": "string",
  "content": "string",
  "imageUrl": "string",
  "uuid": "string"
}
```

## WeightedKnowledgeMessage

```json
{
  "message": {
    "role": "string",
    "content": "string",
    "imageUrl": "string",
    "uuid": "string"
  },
  "weighted": 0.95
}
```

## HistoryDocument

```json
{
  "meta": {
    "exportedAt": "2025.12.31-23:59:59",
    "version": "25.1230"
  },
  "history": [
    {
      "role": "system",
      "content": "示例内容",
      "isPrompt": false,
      "noRender": false,
      "imageUrl": "",
      "deletable": true,
      "uuid": "uuid-2025.12.31-23:59:59",
      "embedVector": []
    }
  ]
}
```
