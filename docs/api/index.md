# API 文档

本文档介绍 Lunar Astral Agents 系统提供的 HTTP API 和 WebSocket API。

## 基础信息

- **Base URL**: `http://localhost:{port}`（默认端口 36789）
- **认证方式**: 默认无需认证（可通过代理层添加认证）
- **数据格式**: JSON
- **字符编码**: UTF-8

## 通用响应格式

### 成功响应

```json
{
    "status": "success",
    "data": { ... }
}
```

### 错误响应

```json
{
    "status": "error",
    "error": {
        "code": "ERROR_CODE",
        "message": "错误描述"
    }
}
```

## API 分类

| 分类 | 文档 | 说明 |
|------|------|------|
| 模型交互 | [model.md](./model.md) | 与 AI 模型进行对话、获取模型列表等 |
| 文件管理 | [file.md](./file.md) | 文件读取、保存、删除、归档等 |
| 数据库 | [database.md](./database.md) | 数据的增删改查操作 |
| 图像生成 | [image.md](./image.md) | 图像生成、关键帧提取等 |
| 消息队列 | [message.md](./message.md) | 批量写入消息和视频 URL |
| TTS 语音 | [tts.md](./tts.md) | 语音合成相关接口 |
| 代理访问 | [proxy.md](./proxy.md) | 通过服务器代理访问外部资源 |
| 琉璃接口 | [ruri.md](./ruri.md) | 琉璃专属的功能接口 |
| WebSocket | [websocket.md](./websocket.md) | 实时通信接口 |

---

*文档版本：1.0 | 最后更新：2026-05-09*

[返回主页](../README.md) | [返回安装配置](../setup.md)
