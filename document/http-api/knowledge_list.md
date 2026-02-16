# 知识库列表接口

**功能**：根据知识库文件路径，返回知识库下所有条目的HistoryMessage数组

**请求方式**: POST
**路径**: `/knowledge/list`

## 请求参数

| 参数名     | 类型   | 必填 | 说明                                       |
| ---------- | ------ | ---- | ------------------------------------------ |
| `filePath` | string | 是   | 知识库文件路径，相对于 local_data 目录     |

## 请求体示例

```json
{
  "filePath": "knowledge/example.json"
}
```

## 响应示例 (200 OK)

```json
[
  {
    "role": "system",
    "content": "第一条消息内容",
    "isPrompt": false,
    "noRender": false,
    "imageUrl": "",
    "deletable": true,
    "uuid": "uuid-1",
    "embedVector": []
  },
  {
    "role": "user",
    "content": "第二条消息内容",
    "isPrompt": false,
    "noRender": false,
    "imageUrl": "",
    "deletable": true,
    "uuid": "uuid-2",
    "embedVector": []
  }
]
```

## 错误响应

| 状态码 | 说明           |
| ------ | -------------- |
| `400`  | 请求参数错误   |
| `403`  | 访问被拒绝     |
| `500`  | 服务器内部错误 |

```json
{
  "error": {
    "code": 400,
    "message": "KnowledgeList请求[ERROR] -> 缺少文件路径"
  }
}
```

## 处理流程

1. 验证请求参数，确保filePath有效
2. 检查文件路径安全性，防止路径遍历攻击
3. 尝试打开知识库文件
4. 如果文件不存在，初始化文件
5. 读取文件中的HistoryDocument结构
6. 将HistoryMessage转换为SmallHistoryMessage数组
7. 返回结果，其中EmbedVector字段始终为空数组

## 注意事项

- 如果知识库文件不存在，会自动初始化文件
- 返回的是SmallHistoryMessage数组，包含HistoryMessage的所有字段，但EmbedVector始终为空数组
- 这样设计的目的是减少返回数据的体积，因为EmbedVector字段通常较大
- 文件读取过程中会加锁，确保并发安全
