# Bridge Adapter子系统 - 协议适配桥接文档

> 🔗 **Bridge Adapter子系统**是星月智能的协议适配模块，负责实现多种通信协议的适配与转换，支持Lunar协议和NapCat协议等第三方服务接入。

---

## 🏗️ 架构设计

### 模块结构

```
subsystem/bridge_adapter/
├── main.go                   # 主程序入口
├── DEVELOPMENT_GUIDE.md      # 开发指南
├── build.ps1                 # 构建脚本
├── local_data/               # 本地数据
│   └── lunar_config.json     # 配置文件
├── pkg/
│   ├── config/               # 配置管理
│   │   └── config.go
│   ├── logger/               # 日志系统
│   │   └── logger.go
│   ├── lunar/                # Lunar协议适配
│   │   └── lunar.go
│   ├── message/              # 消息处理
│   │   └── message.go
│   ├── napcat/               # NapCat协议适配
│   │   └── napcat.go
│   └── types/                # 类型定义
│       └── types.go
├── template/                 # 消息模板
├── go.mod
└── go.sum
```

---

## 🎯 功能特性

### 协议适配

| 协议 | 说明 | 状态 |
|------|------|------|
| **Lunar协议** | 星月智能内部通信协议 | ✅ 已实现 |
| **NapCat协议** | QQ机器人协议适配 | ✅ 已实现 |

### 消息处理

- 消息格式转换与适配
- 消息模板管理
- 消息路由与分发

### 配置管理

- 多协议配置支持
- 动态配置更新
- 配置校验与验证

---

## 📡 API接口

### 消息发送接口

#### POST /message/send

**功能**：发送消息

**请求体**：
```json
{
  "protocol": "lunar",
  "target": "user_001",
  "content": "Hello from Bridge Adapter",
  "type": "text"
}
```

**响应格式**：
```json
{
  "success": true,
  "message_id": "msg_12345"
}
```

### 消息接收接口

#### GET /message/receive

**功能**：接收消息

**请求参数**：
| 参数 | 类型 | 说明 |
|------|------|------|
| `protocol` | string | 协议类型 |
| `limit` | int | 最大数量 |

**响应格式**：
```json
{
  "success": true,
  "messages": [
    {
      "id": "msg_001",
      "protocol": "lunar",
      "source": "user_001",
      "content": "Hello",
      "type": "text",
      "created_at": "2024-01-15T10:30:00Z"
    }
  ]
}
```

---

## 🔧 配置说明

### 配置文件

**路径**：`local_data/lunar_config.json`

```json
{
  "bridge_adapter": {
    "protocols": {
      "lunar": {
        "enabled": true,
        "endpoint": "ws://localhost:8080"
      },
      "napcat": {
        "enabled": true,
        "endpoint": "http://localhost:5700"
      }
    },
    "message_templates": "./template/"
  }
}
```

---

## 🔗 关联文档

- [主项目README](../../README.md)
- [预留智能体文档](../reserved_agents.md)
- [存储子系统文档](storage.md)