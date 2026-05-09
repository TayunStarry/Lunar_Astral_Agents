# 代理访问 API

本文档介绍代理访问相关的 API 接口。

## 通用代理

通过服务器代理访问外部资源。

- **路径**: `/proxy`
- **方法**: `POST`

### 请求体

```json
{
    "url": "https://api.example.com/data",
    "requestInit": {
        "method": "POST",
        "headers": {
            "Content-Type": "application/json",
            "Authorization": "Bearer token"
        },
        "body": {
            "key": "value"
        },
        "credentials": "omit"
    }
}
```

### 请求参数说明

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| url | string | 是 | 目标 URL |
| requestInit.method | string | 否 | HTTP 方法 |
| requestInit.headers | object | 否 | 请求头 |
| requestInit.body | any | 否 | 请求体 |
| requestInit.credentials | string | 否 | 凭证模式 |

### 响应示例

```json
{
    "status": 200,
    "statusText": "OK",
    "headers": {
        "content-type": "application/json"
    },
    "body": "{\"result\": \"success\"}"
}
```

---

*文档版本：1.0 | 最后更新：2026-05-09*

[返回 API 索引](./index.md) | [下一篇：琉璃接口](./ruri.md)
