# 模型交互 API

本文档介绍与 AI 模型交互相关的 API 接口。

## 获取可用模型列表

获取系统中所有可用的 AI 模型列表。

- **路径**: `/v1/models`
- **方法**: `GET`

### 响应示例

```json
{
    "object": "list",
    "data": [
        {
            "id": "multimodal",
            "object": "model",
            "owned_by": "organization_owner"
        },
        {
            "id": "embedding",
            "object": "model",
            "owned_by": "organization_owner"
        }
    ]
}
```

## 聊天补全

与 AI 模型进行对话，支持多轮对话和工具调用。

- **路径**: `/v1/`
- **方法**: `POST`

### 请求头

| 头部 | 类型 | 必填 | 说明 |
|------|------|------|------|
| Content-Type | string | 是 | application/json |

### 请求体

```json
{
    "model": "multimodal",
    "messages": [
        {
            "role": "system",
            "content": "你是月华，一个可爱的AI少女助理。"
        },
        {
            "role": "user",
            "content": "你好，月华！"
        }
    ],
    "temperature": 0.8,
    "max_tokens": 2048,
    "stream": false
}
```

### 请求参数说明

| 字段 | 类型 | 必填 | 默认值 | 说明 |
|------|------|------|--------|------|
| model | string | 是 | - | 模型标识符 |
| messages | array | 是 | - | 消息数组 |
| messages[].role | string | 是 | - | 角色：system/user/assistant/tool |
| messages[].content | string/array | 是 | - | 消息内容 |
| temperature | float | 否 | 0.8 | 生成温度参数 |
| max_tokens | int | 否 | 2048 | 最大生成 token 数 |
| stream | bool | 否 | false | 是否启用流式输出 |

### 响应示例

```json
{
    "id": "chatcmpl-123",
    "object": "chat.completion",
    "created": 1677652288,
    "model": "multimodal",
    "choices": [
        {
            "index": 0,
            "message": {
                "role": "assistant",
                "content": "你好呀！有什么我可以帮助你的吗？🌟"
            },
            "finish_reason": "stop"
        }
    ],
    "usage": {
        "prompt_tokens": 50,
        "completion_tokens": 30,
        "total_tokens": 80
    }
}
```

### 状态码说明

| 状态码 | 说明 |
|--------|------|
| 200 | 请求成功 |
| 400 | 请求参数错误 |
| 404 | 模型不存在 |
| 500 | 服务器内部错误 |
| 503 | 系统繁忙，模型未就绪 |

---

*文档版本：1.0 | 最后更新：2026-05-09*

[返回 API 索引](./index.md) | [下一篇：文件管理](./file.md)
