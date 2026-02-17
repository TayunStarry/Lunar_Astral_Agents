# 知识库写入接口

**功能**：根据文本和知识库文件路径，将消息缓存到内存中，若文本已存在，则不重复添加。只有收到/knowledge/flush请求时，才会将缓存的消息写入文件

**请求方式**: POST
**路径**: `/knowledge/write`

## 请求参数

| 参数名     | 类型           | 必填 | 说明                                       |
| ---------- | -------------- | ---- | ------------------------------------------ |
| `filePath` | string         | 是   | 知识库文件路径，相对于 local_data 目录     |
| `message`  | HistoryMessage | 是   | 要写入的历史消息，包含文本内容和其他元数据 |

### HistoryMessage 结构

| 参数名        | 类型      | 必填 | 说明                                 |
| ------------- | --------- | ---- | ------------------------------------ |
| `role`        | string    | 是   | 消息角色，如 "system"、"user" 等     |
| `content`     | string    | 是   | 消息文本内容                         |
| `isPrompt`    | boolean   | 否   | 是否为提示词                         |
| `noRender`    | boolean   | 否   | 是否不渲染                           |
| `imageUrl`    | string    | 否   | 图片地址                             |
| `deletable`   | boolean   | 否   | 是否可删除                           |
| `uuid`        | string    | 是   | UUID，由客户端提供                   |
| `embedVector` | []float64 | 否   | 嵌入向量，由客户端提供                 |

## 请求体示例

```json
{
  "filePath": "knowledge/example.json",
  "message": {
    "role": "system",
    "content": "要写入的文本",
    "isPrompt": false,
    "noRender": false,
    "imageUrl": "图片地址",
    "deletable": true,
    "embedVector": []
  }
}
```

## 响应示例 (200 OK)

```json
{
  "message": "消息已缓存，等待写入文件"
}
```

或者（当内容已存在于缓存中时）：

```json
{
  "message": "内容已存在于缓存中，不重复添加"
}
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
    "message": "KnowledgeWrite请求[ERROR] -> 缺少文本内容"
  }
}
```

## 处理流程

1. 验证请求参数，确保filePath和message.content有效
2. 检查文件路径安全性，防止路径遍历攻击
3. 将消息缓存到内存中，不立即写入文件
4. 检查缓存中是否已存在相同文本内容的消息
5. 若内容已存在，则不重复添加，返回相应提示
6. 若内容不存在，则添加到缓存中，返回成功响应

## 注意事项

- UUID由客户端提供，系统不会自动生成
- 如果嵌入向量为空，系统会设置为空切片，后续由客户端生成
- 同一内容的消息不会重复添加到缓存中
- 缓存中的消息只有在收到/knowledge/flush请求时才会写入文件
- 消息的所有属性（包括UUID）将直接使用请求中的值
- 建议在批量写入后调用/knowledge/flush接口，将缓存的消息写入文件
