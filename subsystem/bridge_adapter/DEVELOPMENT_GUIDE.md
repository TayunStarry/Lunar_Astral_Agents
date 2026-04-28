# Bridge Adapter 项目开发提示词

## 一、项目概述

本项目是一个 **QQ 机器人适配器**（Bridge Adapter），负责在 **Napcat**（QQ 客户端接口）与 **lunar_core**（核心服务）之间进行双向消息转发。适配器处理多种消息类型（文本、图片、回复、@提及、转发消息、文件等），并将消息格式转换为 OpenAI 兼容的 JSON 格式。

### 整体架构

```
QQ客户端 <---> Napcat <---> 适配器 <---> lunar_core
                          (本服务)
```

**适配器双角色定位：**
1. **Napcat 消费者**：接收 Napcat WebSocket 推送的 QQ 消息，转换后转发给 lunar_core
2. **lunar_core 消费者**：接收 lunar_core WebSocket 推送的回复，通过 Napcat HTTP API 发送回 QQ 群

---

## 二、核心组件说明

### 2.1 项目结构

```
bridge_adapter/
├── main.go                    # 入口文件
├── go.mod                     # Go 依赖管理
├── local_data/
│   └── lunar_config.json      # 配置文件
└── pkg/
    ├── config/
    │   └── config.go          # 配置管理
    ├── logger/
    │   └── logger.go          # 日志工具
    ├── lunar/
    │   └── lunar.go           # lunar_core WebSocket 客户端
    ├── message/
    │   └── message.go         # 消息处理与转换
    ├── napcat/
    │   └── napcat.go          # Napcat WebSocket/HTTP 客户端
    └── types/
        └── types.go           # 数据类型定义
```

### 2.2 组件职责表

| 组件 | 职责 | 关键功能 |
|------|------|----------|
| `main.go` | 程序入口 | 启动配置加载、WebSocket 连接 |
| `config.go` | 配置管理 | 加载配置、群组校验、消息缓存、关键词检测 |
| `logger.go` | 日志输出 | Info/Error/Warn/Debug 级日志 |
| `lunar.go` | 上游通信 | 连接 lunar_core WebSocket、处理响应消息 |
| `napcat.go` | 下游通信 | 连接 Napcat WebSocket、调用 HTTP API |
| `message.go` | 消息转换 | 解析消息段、构建 OpenAI 格式消息 |
| `types.go` | 类型定义 | 所有数据结构定义 |

---

## 三、核心运行逻辑

### 3.1 启动流程

```
1. 加载配置 (config.LoadConfig)
2. 获取群成员列表 (napcat.FetchGroupMembers)
3. 启动 Napcat WebSocket 连接 (goroutine)
4. 启动 lunar_core WebSocket 连接 (goroutine)
5. 等待系统信号 (SIGINT/SIGTERM)
```

### 3.2 消息处理流程（Napcat → lunar_core）

```
1. Napcat WebSocket 推送消息
2. HandleNapcatMessage 接收
   ├─ 过滤：用户ID == 自己ID → 忽略
   ├─ 过滤：群组不在 listen_group_ids → 忽略
   ├─ 过滤：消息类型 != group → 忽略
3. ParseMessageSegments 解析消息段
   ├─ text → 提取文本
   ├─ reply → 调用 get_msg 获取原文
   ├─ image → 提取 URL
   ├─ at → 转换为 @用户名
   ├─ forward → 调用 get_forward_msg 递归解析
   └─ file → 提取文件名和链接
4. 添加到消息缓存 (config.AddToMessageCache)
5. 检测触发关键词 (config.ContainsTriggerKeyword)
   ├─ 不包含 → 仅缓存
   └─ 包含 → 构建消息并发送到 lunar_core
6. 发送成功后清除缓存
```

### 3.3 响应处理流程（lunar_core → Napcat）

```
1. lunar_core WebSocket 推送消息
2. HandleLunarMessage 接收
   ├─ type: context → 随机群组发送文本
   ├─ type: image → 随机群组发送图片
   ├─ type: response → 发送到最后接收消息的群组
   └─ type: active → 广播到所有监听群组
3. 调用 napcat.SendGroupTextMessage / SendGroupImageMessage
4. 通过 Napcat HTTP API 发送到 QQ
```

---

## 四、关键技术要点

### 4.1 消息段类型处理

| 类型 | 处理逻辑 | 转换结果 |
|------|----------|----------|
| `text` | 直接提取 | 纯文本字符串 |
| `reply` | 调用 `/get_msg` 获取原文 | `[回复: 内容]` |
| `image` | 提取 URL | OpenAI image_url 格式 |
| `at` | 查询成员列表获取昵称 | `@用户名` |
| `forward` | 调用 `/get_forward_msg` 递归解析 | 完整对话内容 |
| `file` | 提取文件名和 URL | `[文件] 文件名 URL` |

### 4.2 消息缓存机制

```go
// 缓存容量限制
const MaxMessageCache = 5

// 缓存结构
type CachedMessage struct {
    GroupID   int64
    UserID    int64
    Content   interface{}  // string 或 []map[string]interface{}
    HasImages bool
}
```

**缓存策略：**
- FIFO 队列，超出容量时移除最早的消息
- 包含图片的消息转为数组格式，纯文本保持字符串
- 触发关键词后，批量发送所有缓存消息

### 4.3 触发关键词机制

```go
// 配置示例
"trigger_keywords": ["月之华", "月华", "3826713076"]

// 匹配逻辑
func ContainsTriggerKeyword(message string) bool {
    // 如果关键词列表为空，所有消息都触发
    // 否则检查消息是否包含任意关键词
}
```

### 4.4 群组选择策略

| 消息类型 | 目标群组 | 说明 |
|----------|----------|------|
| `context` | 随机（首个）群组 | 用于普通响应 |
| `image` | 随机（首个）群组 | 用于图片响应 |
| `response` | `LastGroupID` | 回复到触发消息的来源群 |
| `active` | 所有监听群组 | 广播消息 |

---

## 五、API 接口规范

### 5.1 Napcat HTTP API

| 接口 | 方法 | 作用 | 调用场景 |
|------|------|------|----------|
| `/get_group_member_list` | POST | 获取群成员列表 | 启动时预加载 |
| `/get_msg` | POST | 获取单条消息详情 | 处理 reply 类型 |
| `/get_forward_msg` | POST | 获取转发消息详情 | 处理 forward 类型 |
| `/send_group_msg` | POST | 发送群消息 | 发送文本/图片 |

**认证方式：** `Authorization: Bearer <token>`

### 5.2 lunar_core 接口

| 接口 | 方法 | 作用 |
|------|------|------|
| `ws://<host>/ws` | WebSocket | 接收响应消息 |
| `/write/message` | POST | 发送消息到队列 |

**消息格式（发送到 lunar_core）：**
```json
{
    "messages": [
        {
            "role": "user",
            "content": "文本内容"
        }
    ]
}
```

**消息格式（lunar_core 推送）：**
```json
// 文本响应
{ "type": "context", "data": { "type": "response", "content": "..." } }
// 图片响应
{ "type": "image", "data": { "type": "image", "images": ["base64..."] } }
```

---

## 六、配置文件规范

```json
{
    "qq_adapter": {
        "napcat_ws_server": "ws://localhost:4567",      // Napcat WebSocket 地址
        "napcat_ws_token": "your_token_here",           // Napcat 认证令牌
        "lunar_core_url": "http://localhost:36789",     // lunar_core HTTP 地址
        "lunar_ws_server": "ws://localhost:36797/ws",   // lunar_core WebSocket 地址
        "poll_interval": 10,                            // 轮询间隔（秒）
        "listen_group_ids": [123456789, 987654321],     // 监听的群组 ID 列表
        "trigger_keywords": ["月华", "月之华"],          // 触发关键词
        "display_logs": true,                           // 是否显示详细日志
        "default_reply": "不知道哦~"                     // 默认回复
    }
}
```

---

## 七、注意事项与最佳实践

### 7.1 消息过滤规则

1. **忽略自己发送的消息**：`user_id == self_id` 时直接返回
2. **仅限监听群组**：`group_id` 必须在 `listen_group_ids` 列表中
3. **仅限群消息**：`message_type == "group"`

### 7.2 错误处理

- **HTTP 调用**：设置 10 秒超时，记录错误日志
- **WebSocket 连接失败**：记录错误但不阻塞主流程
- **消息解析失败**：记录警告，继续处理其他消息
- **网络异常**：自动重连（当前实现需手动重启）

### 7.3 性能优化

- **消息缓存批量发送**：减少 HTTP 请求次数
- **成员列表预加载**：避免实时查询，减少 API 调用
- **日志分级**：生产环境关闭 Debug 日志

### 7.4 安全注意事项

- **Token 管理**：配置文件中的 token 不应提交到版本控制
- **URL 验证**：确保只向可信地址发送请求
- **消息内容过滤**：可考虑添加敏感词过滤

### 7.5 扩展建议

- **重试机制**：添加失败重试和指数退避
- **连接池**：复用 HTTP 连接
- **配置热更新**：支持运行时修改配置
- **消息去重**：基于 message_id 去重

---

## 八、开发调试指南

### 8.1 本地开发环境

1. **启动 Napcat**：确保 Napcat 服务运行在配置的端口
2. **启动 lunar_core**：确保核心服务正常运行
3. **配置文件**：修改 `local_data/lunar_config.json`
4. **运行**：`go run main.go`

### 8.2 日志调试

```go
// 开启详细日志
"display_logs": true

// 日志级别
logger.Info()    // 关键信息
logger.Error()   // 错误信息
logger.Warn()    // 警告信息
logger.Debug()   // 调试信息（仅 display_logs=true 时显示）
```

### 8.3 测试消息格式

**纯文本消息：**
```json
{
    "self_id": 123456789,
    "user_id": 987654321,
    "group_id": 123456789,
    "message_type": "group",
    "message": [{"type": "text", "data": {"text": "月华，你好"}}]
}
```

**@提及消息：**
```json
{
    "self_id": 123456789,
    "user_id": 987654321,
    "group_id": 123456789,
    "message_type": "group",
    "message": [
        {"type": "at", "data": {"qq": "123456789"}},
        {"type": "text", "data": {"text": " 现在几点了"}}
    ]
}
```

---

## 九、代码风格规范

### 9.1 命名规则

- **包名**：小写，无下划线（如 `pkg/config`）
- **文件名**：小写，下划线分隔（如 `message.go`）
- **类型名**：大驼峰（如 `NapcatMessage`）
- **函数名**：大驼峰（如 `HandleNapcatMessage`）
- **变量名**：小驼峰（如 `messageCache`）

### 9.2 错误处理

```go
// 正确：检查错误并记录日志
resp, err := httpClient.Do(req)
if err != nil {
    logger.Error("发送失败: %v", err)
    return err
}
defer resp.Body.Close()
```

### 9.3 注释规范

- 包级注释：说明包的功能
- 函数注释：说明参数和返回值
- 复杂逻辑：添加必要的注释说明

---

## 十、部署与运维

### 10.1 构建

```bash
# Windows
go build -o bridge_adapter.exe

# Linux
go build -o bridge_adapter
```

### 10.2 运行

```bash
# 直接运行
./bridge_adapter

# 后台运行（Linux）
nohup ./bridge_adapter > output.log 2>&1 &
```

### 10.3 配置管理

- 配置文件必须放在 `local_data/lunar_config.json`
- 修改配置后需要重启服务
- 建议定期备份配置文件

---

## 十一、常见问题

### 11.1 连接失败

**现象**：无法连接到 Napcat 或 lunar_core

**排查**：
1. 检查服务是否启动
2. 检查端口是否正确
3. 检查网络连通性
4. 检查 Token 是否正确

### 11.2 消息不触发

**现象**：消息已接收但未发送到 lunar_core

**排查**：
1. 检查 `listen_group_ids` 配置
2. 检查 `trigger_keywords` 配置
3. 检查消息是否包含触发关键词
4. 查看日志确认过滤原因

### 11.3 图片消息处理失败

**现象**：图片消息无法正确发送

**排查**：
1. 检查图片 URL 是否有效
2. 检查 Base64 编码是否正确
3. 检查 Napcat API 权限

---

## 十二、版本历史

| 版本 | 日期 | 更新内容 |
|------|------|----------|
| v1.0 | 初始版本 | 基础消息转发功能 |
| v1.1 | - | 添加关键词触发机制 |
| v1.2 | - | 添加消息缓存功能 |
| v1.3 | - | 支持多消息类型解析 |