# 代理请求 API

## 功能描述

代理请求 API 用于转发 HTTP 请求到指定的 URL，并返回响应结果。该 API 支持各种 HTTP 方法和请求参数，可用于跨域请求、访问受限资源等场景。

## 端点路径

- **路径**: `/proxy`
- **方法**: `POST`

## 请求参数

请求体为 JSON 格式，包含以下字段：

| 字段名 | 类型 | 必填 | 描述 |
|-------|------|------|------|
| `url` | string | 是 | 目标 URL，请求将被转发到该地址 |
| `requestInit` | object | 否 | 请求初始化参数 |
| `requestInit.method` | string | 否 | HTTP 请求方法，默认为 GET |
| `requestInit.headers` | object | 否 | 请求头信息，键值对格式 |
| `requestInit.body` | any | 否 | 请求体数据 |
| `requestInit.redirect` | string | 否 | 重定向策略 |
| `requestInit.credentials` | string | 否 | 凭证处理方式 |

## 响应格式

### 成功响应

- **状态码**: 200 OK

对于非图片响应，返回 JSON 格式：

```json
{
  "status": 200,
  "statusText": "OK",
  "headers": {
    "Content-Type": "application/json"
  },
  "body": {
    "example": "response"
  }
}
```

对于图片响应，直接返回图片数据，Content-Type 为对应的图片类型。

### 失败响应

- **状态码**: 400 Bad Request - 请求参数错误
- **状态码**: 500 Internal Server Error - 服务器内部错误

```json
{
  "error": "错误信息"
}
```

## 请求示例

### 转发 GET 请求

```json
{
  "url": "https://api.example.com/data",
  "requestInit": {
    "method": "GET",
    "headers": {
      "Authorization": "Bearer token123"
    }
  }
}
```

### 转发 POST 请求

```json
{
  "url": "https://api.example.com/submit",
  "requestInit": {
    "method": "POST",
    "headers": {
      "Content-Type": "application/json"
    },
    "body": {
      "name": "Test",
      "value": "123"
    }
  }
}
```

## 注意事项

1. 代理请求的超时时间为 30 秒
2. 支持跨域请求，响应头中包含 `Access-Control-Allow-Origin: *`
3. 对于图片响应，会直接返回图片数据，而不是 JSON 格式
4. 请确保目标 URL 是安全的，避免转发到恶意网站