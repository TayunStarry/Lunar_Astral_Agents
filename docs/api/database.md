# 数据库 API

本文档介绍数据库操作相关的 API 接口。

## 数据管理

执行数据库操作，包括增删改查。

- **路径**: `/database/`
- **方法**: `POST`

### 请求体

```json
{
    "operation": "insert",
    "table": "chat_history",
    "data": {
        "id": 1,
        "message": "你好",
        "timestamp": "2026-01-02T15:04:05Z"
    }
}
```

### 操作类型说明

| 操作类型 | 说明 |
|----------|------|
| insert | 插入数据 |
| update | 更新数据 |
| delete | 删除数据 |
| query | 查询数据 |

---

*文档版本：1.0 | 最后更新：2026-05-09*

[返回 API 索引](./index.md) | [下一篇：图像生成](./image.md)
