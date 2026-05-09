# 消息队列 API

本文档介绍消息队列相关的 API 接口。

## 批量写入消息

批量写入消息到队列。

- **路径**: `/write/message`
- **方法**: `POST`

### 请求体

```json
{
    "messages": [
        {
            "role": "user",
            "content": "消息内容1"
        },
        {
            "role": "assistant",
            "content": "消息内容2"
        }
    ]
}
```

### 响应示例

```json
{
    "success": true,
    "length": 2
}
```

## 批量写入视频 URL

批量写入视频 URL。

- **路径**: `/write/videourl`
- **方法**: `POST`

### 请求体

```json
{
    "urls": [
        "https://example.com/video1.mp4",
        "https://example.com/video2.mp4"
    ]
}
```

---

*文档版本：1.0 | 最后更新：2026-05-09*

[返回 API 索引](./index.md) | [下一篇：TTS 语音](./tts.md)
