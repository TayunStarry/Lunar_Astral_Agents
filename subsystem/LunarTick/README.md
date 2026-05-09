# LunarTick 编程语言解释器

一个功能完整的 LunarTick 编程语言解释器，支持 WebSocket 通信、独立运行和模块嵌入。

## 功能特性

- ✅ 完整的 LunarTick 语言实现
- ✅ Tick 驱动的调度引擎
- ✅ 变量系统（支持普通、只读、只写模式）
- ✅ 指针系统（Class 指针、Snapshot 指针）
- ✅ WebSocket 通信支持
- ✅ 独立运行模式
- ✅ 模块嵌入支持

## 编译和运行

### 独立运行模式

```bash
# 运行示例代码
go run . --mode standalone

# 运行指定的脚本文件
go run . --mode standalone --file example.md
```

### WebSocket 服务器模式

```bash
# 启动 WebSocket 服务器（默认端口 8080）
go run . --mode server --addr :8080
```

## WebSocket 协议

### 连接

```
ws://localhost:8080/ws
```

### 消息格式

#### 注入代码

```json
{
  "type": "inject",
  "data": {
    "lines": [
      "@log \"Hello from WebSocket!\"",
      "SET test \"value\""
    ]
  }
}
```

#### 调用指针

```json
{
  "type": "invoke",
  "data": {
    "pointerName": "myPointer"
  }
}
```

#### 启动和停止

```json
{ "type": "start" }
```
```json
{ "type": "stop" }
```

### 接收消息

解释器会通过 WebSocket 发送运行时消息：

```json
{
  "type": "log",
  "content": "Hello, World!",
  "time": 1234567890
}
```

## LunarTick 语言基础

### 变量操作

```
SET var "value"        # 赋值
ADD var "append"       # 追加
WRT var "write-only"   # 只写变量
RON var "read-only"    # 只读变量
UNL var "unlocked"     # 解锁变量
```

### 控制流

```
@if "condition" ? truePtr : falsePtr
@cycle "condition" ? loopPtr
@sleep milliseconds
@wait varname
```

### 指针

```
@lazy *myPointer       # 定义惰性指针
@build *ptr class      # 构建类指针
@build *ptr snapshot   # 构建快照
*myPointer             # 调用指针
```

### 其他

```
@log "message"
@web "https://example.com"
@read "file.txt" dest
@write "file.txt" source
@stop
```

## 项目结构

```
lunartick/
├── main.go           # 主程序入口
├── types.go          # 类型定义
├── variables.go      # 变量系统
├── pointers.go       # 指针系统
├── lexer.go          # 词法分析器
├── interpreter.go    # 核心解释器
├── websocket.go      # WebSocket 服务
├── example.md        # 示例代码
├── go.mod            # Go 模块
└── go.sum            # 依赖锁定
```

## 开发计划

- [ ] 集成 storage 子系统
- [ ] 集成 browser 子系统（高级功能）
- [ ] 添加更多测试
- [ ] 性能优化
- [ ] 完善文档

## 许可证

MIT License
