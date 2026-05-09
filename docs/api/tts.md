# TTS 语音 API

本文档介绍 TTS 语音合成相关的 API 接口。

## TTS 代理

代理 TTS 请求到语音服务。

- **路径**: `/audio/generate`
- **方法**: `POST`

### 请求体

透传到 TTS 服务。

### 响应

音频数据（audio/wav 或 audio/mp3）。

## Qwen3 TTS 模型检测

获取可用的 Qwen3 TTS 模型列表。

- **路径**: `/qwen_tts/models`
- **方法**: `GET`

### 响应示例

```json
{
    "models": ["qwen3-tts"]
}
```

## Qwen3 TTS 请求代理

代理 Qwen3 TTS 请求。

- **路径**: `/qwen_tts/`
- **方法**: `POST`

---

*文档版本：1.0 | 最后更新：2026-05-09*

[返回 API 索引](./index.md) | [下一篇：代理访问](./proxy.md)
