# Proxy子系统 - 代理服务文档

> 🌐 **Proxy子系统**是星月智能的代理服务模块，提供HTTP/HTTPS代理、SSL证书管理和请求转发功能。

---

## 🏗️ 架构设计

### 模块结构

```
subsystem/proxy/
├── proxy.go                 # 代理核心逻辑
├── certs/                   # SSL证书目录
│   ├── localhost.pem        # 自签名证书
│   └── localhost-key.pem    # 私钥文件
├── go.mod
└── go.sum
```

---

## 🎯 功能特性

### 代理服务

| 功能 | 说明 |
|------|------|
| **HTTP代理** | 支持HTTP请求代理 |
| **HTTPS代理** | 支持HTTPS请求代理与SSL终止 |
| **请求转发** | 将请求转发到目标服务 |
| **负载均衡** | 支持多后端服务负载均衡 |

### SSL管理

- 自签名证书生成
- 证书自动更新
- SSL/TLS协议支持

### 请求处理

- 请求日志记录
- 请求过滤与限流
- 请求头修改与添加

---

## 📡 API接口

### 代理请求接口

#### POST /proxy/forward

**功能**：转发HTTP请求

**请求体**：
```json
{
  "url": "https://api.example.com/data",
  "method": "GET",
  "headers": {
    "Authorization": "Bearer token"
  },
  "body": null
}
```

**响应格式**：
```json
{
  "success": true,
  "status": 200,
  "headers": {
    "Content-Type": "application/json"
  },
  "body": {
    "data": "response content"
  }
}
```

### 证书管理接口

#### GET /proxy/certs

**功能**：获取证书列表

**响应格式**：
```json
{
  "success": true,
  "certs": [
    {
      "name": "localhost",
      "path": "certs/localhost.pem",
      "expires_at": "2025-01-15T00:00:00Z"
    }
  ]
}
```

---

## 🔧 配置说明

### 命令行参数

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `-port` | int | 8080 | 代理服务端口 |
| `-ssl` | bool | false | 是否启用SSL |
| `-cert` | string | certs/localhost.pem | SSL证书路径 |
| `-key` | string | certs/localhost-key.pem | SSL私钥路径 |

---

## 🔗 关联文档

- [主项目README](../../README.md)
- [预留智能体文档](../reserved_agents.md)
- [浏览器子系统文档](browser.md)