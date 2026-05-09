# WebSocket API

本文档介绍 WebSocket 实时通信接口。

## 连接信息

- **URL**: `ws://localhost:{port}/ws`
- **协议**: WebSocket

## 客户端 → 服务器消息

### 消息格式

```json
{
    "type": "chat",
    "data": {
        "message": "你好，月华！"
    }
}
```

### 消息类型说明

| type | 说明 |
|------|------|
| chat | 聊天消息 |
| image | 图像相关操作 |
| context | 上下文更新 |

## 服务器 → 客户端消息

### 消息格式

```json
{
    "type": "context",
    "data": {
        "content": "你好呀！有什么我可以帮助你的吗？"
    }
}
```

### 消息类型说明

| type | 说明 |
|------|------|
| context | 文本回复 |
| image | 图像推送 |
| error | 错误信息 |

---

*文档版本：1.0 | 最后更新：2026-05-09*

[返回 API 索引](./index.md)
