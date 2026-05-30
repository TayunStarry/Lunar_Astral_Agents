# LunarTick 通用程序执行引擎

LunarTick 是一个轻量级、tick 驱动的反应式通用程序执行引擎，能够调用外部程序、传递自定义运行时参数，并捕获标准输出、标准错误和执行状态。支持作为独立模块被其他程序调用，或作为 HTTP API 服务独立运行。

## 目录

- [快速开始](#快速开始)
- [部署模式](#部署模式)
  - [模式一：独立模块（库调用）](#模式一独立模块库调用)
  - [模式二：HTTP API 服务](#模式二http-api-服务)
- [命令行参数](#命令行参数)
- [配置方法](#配置方法)
- [架构设计](#架构设计)
- [API 接口文档](#api-接口文档)
  - [GET /api/health](#get-apihealth)
  - [GET /api/status](#get-apistatus)
  - [POST /api/start](#post-apistart)
  - [POST /api/stop](#post-apistop)
  - [POST /api/shutdown](#post-apishutdown)
  - [POST /api/run](#post-apirun)
  - [POST /api/load](#post-apiload)
  - [POST /api/inject](#post-apiinject)
  - [POST /api/invoke](#post-apiinvoke)
  - [GET/POST /api/variables](#getpost-apivariables)
  - [GET /api/pointers](#get-apipointers)
- [LunarTick 指令参考](#lunartick-指令参考)
- [示例代码](#示例代码)
- [项目结构](#项目结构)
- [构建与测试](#构建与测试)

---

## 快速开始

### 构建

```bash
cd d:\Lunar_Astral_Agents\subsystem\LunarTick
go build -o bin/lunartick.exe ./cmd/lunartick/
```

### 启动 HTTP API 服务

```bash
.\bin\lunartick.exe --api-port 36800 --tick-ms 100
```

### 启动开发者模式

```bash
.\bin\lunartick.exe --developer --api-port 36800
```

### 启动时加载脚本文件

```bash
.\bin\lunartick.exe --load ./scripts/example.md
```

---

## 部署模式

### 模式一：独立模块（库调用）

将 LunarTick 引擎作为 Go 库集成到你的项目中。

```go
package main

import (
    "time"
    "lunartick/engine"
)

func main() {
    // 创建引擎，设置 tick 间隔 100ms
    eng := engine.NewEngine(100 * time.Millisecond)

    // 注入代码块
    eng.Inject([]string{
        "@SET #name 'LunarTick'",
        "@log '你好，#name!'",
        "@stop",
    })

    // 启动引擎
    eng.Start()
    time.Sleep(1 * time.Second)
    eng.Stop()
}
```

- **模块路径**: `lunartick/engine` — 核心引擎，包含 tick 调度器、代码块管理、变量系统、指针系统、指令执行器
- **模块路径**: `lunartick/api` — HTTP API 服务层，基于 engine 封装 RESTful 接口

### 模式二：HTTP API 服务

以独立进程方式运行，对外暴露 RESTful API。

默认监听端口 **36800**，可通过 `--api-port` 参数自定义。

所有 API 返回 JSON 格式数据，支持 CORS 跨域访问。

---

## 命令行参数

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `--api-port` | int | 36800 | HTTP API 服务端口 |
| `--tick-ms` | int | 100 | Tick 间隔（毫秒） |
| `--developer` | bool | false | 启用开发者模式（详细日志输出） |
| `--load` | string | "" | 启动时加载的 Markdown 脚本文件路径 |

---

## 配置方法

### 环境变量

无需特殊环境变量，所有配置通过命令行参数指定。

### 日志配置

- **默认模式**: 简洁日志输出，适合生产环境
- **开发者模式** (`--developer`): 详细日志输出，包含每个 Tick 的执行状态、变量变化、代码块状态转换等，适合调试

### 模块集成配置

在 Go 项目中集成时，通过 `go.mod` 添加本地依赖：

```go
// go.mod
module yourproject

require lunartick v0.0.0

replace lunartick => d:\Lunar_Astral_Agents\subsystem\LunarTick
```

---

## 架构设计

### 模块划分

```
┌─────────────────────────────────────────────────────────┐
│                     入口层 (cmd/lunartick)               │
│                CLI 启动 / 参数解析 / 信号处理             │
├─────────────────────────────────────────────────────────┤
│                     服务层 (api)                         │
│         HTTP Server / Router / Request Handler          │
├─────────────────────────────────────────────────────────┤
│                   引擎编排层 (engine)                     │
│    ┌──────────┐ ┌──────────┐ ┌──────────────┐          │
│    │  Engine  │ │  Ticker  │ │ BlockManager │          │
│    │ (编排器) │ │ (调度器) │ │ (代码块管理) │          │
│    └──────────┘ └──────────┘ └──────────────┘          │
├─────────────────────────────────────────────────────────┤
│                   核心执行层 (engine)                     │
│  ┌──────────┐ ┌─────────┐ ┌──────────┐ ┌────────────┐ │
│  │Parser    │ │Variable │ │Pointer   │ │ProcessExec │ │
│  │(指令解析)│ │(变量系统)│ │(指针系统)│ │(进程执行器)│ │
│  └──────────┘ └─────────┘ └──────────┘ └────────────┘ │
│  ┌──────────────┐ ┌────────────────┐                    │
│  │ Instructions │ │ Expression     │                    │
│  │ (指令执行器) │ │ (表达式求值器) │                    │
│  └──────────────┘ └────────────────┘                    │
├─────────────────────────────────────────────────────────┤
│                   基础设施层                              │
│           logger (日志) / sync (并发控制)                 │
└─────────────────────────────────────────────────────────┘
```

### 核心组件交互

```
                    ┌──────────────┐
                    │   Ticker     │ ◄── Tick 时钟驱动
                    │  (调度器)    │
                    └──────┬───────┘
                           │ 每个 Tick 执行:
            ┌──────────────┼──────────────┐
            ▼              ▼              ▼
     phaseInject    phaseExecute    phaseCheckReady
     (注入待处理块)  (执行就绪块)    (检查等待块)
            │              │              │
            ▼              ▼              ▼
     BlockManager    Instruction     WaitCondition
     (代码块状态)    (指令分发)      (条件检查)
                           │
            ┌──────────────┼──────────────┐
            ▼              ▼              ▼
        execRun        execSET         execLog
     (进程调用)      (变量操作)      (日志输出)
            │
            ▼
     ProcessExecutor
  (stdout/stderr/exit code)
```

### 数据流图

```
用户请求 (HTTP / 库调用)
    │
    ▼
Engine.Inject() / Engine.Invoke() / Engine.LoadMarkdown()
    │
    ▼
BlockManager.CreateBlock() → CodeBlock (指令列表)
    │
    ▼ (就绪状态)
Ticker.phaseExecute()
    │
    ▼
ExecuteInstruction() → execRun/execSET/execLog/...
    │
    ├── execRun → ProcessExecutor.Start()
    │              ├── stdout → 变量存储
    │              ├── stderr → 变量存储
    │              └── exit code → 变量存储
    │
    ├── execSET → VarStore.Set()
    │
    └── execLog → logger.Info()
    │
    ▼
WaitCondition / @stop / 指令结束
    │
    ▼
BlockManager 状态更新 (就绪→等待→终止)
```

---

## API 接口文档

基础 URL: `http://localhost:36800`

所有请求/响应均使用 `Content-Type: application/json`。

### `GET /api/health`

**描述**: 健康检查接口。

**响应**:

```json
{
    "status": "healthy",
    "running": true
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `status` | string | 服务状态，固定值 `"healthy"` |
| `running` | boolean | 引擎是否在运行中 |

---

### `GET /api/status`

**描述**: 获取引擎完整状态信息，包括 tick 进度、代码块数量、变量、指针和错误列表。

**响应**:

```json
{
    "running": true,
    "suspended": false,
    "tick_number": 42,
    "ready_blocks": 2,
    "waiting_blocks": 1,
    "variables": {
        "TICK": "42",
        "TICK_MS": "100",
        "result": "hello world"
    },
    "pointers": ["myTask", "cleanupHandler"],
    "errors": []
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `running` | boolean | 引擎是否正在运行 |
| `suspended` | boolean | 引擎是否处于停摆状态（无待处理任务） |
| `tick_number` | int | 当前 Tick 序号 |
| `ready_blocks` | int | 就绪代码块数量 |
| `waiting_blocks` | int | 等待中代码块数量 |
| `variables` | object | 所有变量名→值的映射 |
| `pointers` | string[] | 已注册的指针名称列表 |
| `errors` | ErrorEntry[] | 最近的错误记录 |

**ErrorEntry**:

| 字段 | 类型 | 说明 |
|------|------|------|
| `block_id` | string | 发生错误的代码块 ID |
| `message` | string | 错误描述信息 |
| `tick_number` | int | 错误发生的 Tick 序号 |

---

### `POST /api/start`

**描述**: 启动引擎。

**请求体**: 无

**响应**:

```json
{
    "status": "started"
}
```

---

### `POST /api/stop`

**描述**: 停止引擎（停止 tick 循环，但保留所有状态）。

**请求体**: 无

**响应**:

```json
{
    "status": "stopped"
}
```

---

### `POST /api/shutdown`

**描述**: 关闭引擎并清理所有资源。

**请求体**: 无

**响应**:

```json
{
    "status": "shutdown"
}
```

---

### `POST /api/run`

**描述**: 调用外部程序并注入运行块。引擎会创建一个包含 `@run` 指令的代码块。

**请求体**:

```json
{
    "path": "notepad.exe",
    "args": ["C:\\temp\\file.txt"]
}
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `path` | string | 是 | 外部可执行程序路径 |
| `args` | string[] | 否 | 传递给程序的运行时参数列表 |

**响应**:

```json
{
    "block_id": "injected",
    "status": "running"
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `block_id` | string | 注入的代码块 ID |
| `status` | string | 执行状态，固定值 `"running"` |

---

### `POST /api/load`

**描述**: 加载脚本内容（支持 Markdown 或 JSON 格式）。

**请求体**:

```json
{
    "source": "```LunarTick\n@log 'hello'\n@stop\n```",
    "format": "markdown"
}
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `source` | string | 是 | 脚本内容源码 |
| `format` | string | 否 | 格式类型：`"markdown"` 或 `"json"`，默认 `"markdown"` |

**Markdown 格式说明**: 使用代码块围栏 ` ```LunarTick ... ``` ` 包裹指令，支持多个代码块。

**JSON 格式说明**: 二维字符串数组 `[["@指令1", "@指令2"], ["@指令3"]]`，每个子数组为一个代码块。

**响应**:

```json
{
    "status": "loaded",
    "block_count": 0
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `status` | string | 加载状态，固定值 `"loaded"` |
| `block_count` | int | 加载的代码块数量 |

---

### `POST /api/inject`

**描述**: 直接注入一行或多行指令作为代码块立即执行。

**请求体**:

```json
{
    "lines": [
        "@SET #target 'World'",
        "@log 'Hello, #target!'",
        "@stop"
    ]
}
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `lines` | string[] | 是 | 指令行列表，每行为一条 LunarTick 指令 |

**响应**:

```json
{
    "block_count": 1,
    "status": "injected"
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `block_count` | int | 注入的代码块数量 |
| `status` | string | 注入状态，固定值 `"injected"` |

---

### `POST /api/invoke`

**描述**: 调用已定义的指针（指针需要先通过 `@DEF` 或 `@LAZY` 注册）。

**请求体**:

```json
{
    "pointer": "myTask"
}
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `pointer` | string | 是 | 要调用的指针名称（不含 `*` 前缀） |

**响应**:

```json
{
    "status": "invoked"
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `status` | string | 调用状态，固定值 `"invoked"` |

---

### `GET/POST /api/variables`

**描述**: 查看和设置引擎变量。

**GET 请求** — 获取所有变量:

响应返回变量名→值的映射对象:

```json
{
    "TICK": "15",
    "TICK_MS": "100",
    "myVar": "someValue"
}
```

**POST 请求** — 设置变量:

```json
{
    "name": "config_path",
    "value": "/etc/app/config.yml"
}
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `name` | string | 是 | 变量名称 |
| `value` | string | 是 | 变量值（字符串类型） |

**响应**:

```json
{
    "name": "config_path",
    "value": "/etc/app/config.yml"
}
```

---

### `GET /api/pointers`

**描述**: 获取所有已注册的指针名称列表。

**响应**:

```json
{
    "pointers": ["myTask", "cleanupHandler"]
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `pointers` | string[] | 已注册的指针名称列表 |

---

### 错误响应格式

所有接口在发生错误时返回以下格式:

```json
{
    "error": "Bad Request",
    "message": "path is required"
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `error` | string | HTTP 状态码对应的标准文本（如 `"Bad Request"`） |
| `message` | string | 具体错误描述 |

HTTP 状态码:
- `200 OK` — 请求成功
- `400 Bad Request` — 请求参数错误或 JSON 不合法
- `405 Method Not Allowed` — 请求方法不匹配

---

## LunarTick 指令参考

| 指令 | 格式 | 说明 |
|------|------|------|
| `@SET` | `@SET #var 'value'` | 设置变量值（支持覆盖赋值） |
| `@ADD` | `@ADD #var 'value'` | 追加变量值 |
| `@LOG` | `@LOG 'message'` | 输出日志信息 |
| `@RUN` | `@RUN 'program' 'arg1' 'arg2'` | 调用外部程序 |
| `@WAIT` | `@WAIT #running_var` | 等待变量就绪 |
| `@SLEEP` | `@SLEEP 1000` | 等待指定毫秒数 |
| `@IF` | `@IF #x = '1'` | 条件判断 |
| `@CYCLE` | `@CYCLE #items '\n'` | 循环遍历 |
| `@RETRY` | `@RETRY 3 1000` | 重试机制 |
| `@WRT` | `@WRT #var` | 将变量设为只写模式 |
| `@RON` | `@RON #var` | 将变量设为只读模式 |
| `@UNL` | `@UNL #var ''` | 将变量设为自由模式 |
| `@DEF` | `@DEF *pointer` | 定义指针 |
| `@LAZY` | `@LAZY` | 标记当前块为惰性指针 |
| `@CALL` | `@CALL #result *handler` | 调用指针 |
| `@CATCH` | `@CATCH #var 'pattern'` | 捕获输出 |
| `@BUILD` | `@BUILD *ptr 'class'` | 构建指针 |
| `@FILTER` | `@FILTER #var 'pattern' 'replace'` | 过滤替换 |
| `@MATH` | `@MATH #var '1 + 1'` | 数学运算 |
| `@WRITE` | `@WRITE 'path' #content` | 写入文件 |
| `@READ` | `@READ #var 'path'` | 读取文件 |
| `@STOP` | `@STOP` | 停止代码块执行 |
| `@END` | `@END` | 结束代码块定义 |
| `@LIMIT` | `@LIMIT 5000` | 设置执行超时（毫秒） |

---

## 示例代码

### 示例 1: 调用外部程序并捕获输出

```go
package main

import (
    "fmt"
    "time"
    "lunartick/engine"
)

func main() {
    eng := engine.NewEngine(100 * time.Millisecond)
    eng.Start()

    // 调用 echo 命令（Windows 用 cmd /c echo）
    eng.Inject([]string{
        "@RUN 'cmd' '/c' 'echo' 'Hello from LunarTick!'",
        "@CATCH #result 'Hello'",
        "@LOG '#result'",
        "@stop",
    })

    time.Sleep(1 * time.Second)

    fmt.Println("执行结果:", eng.GetVariable("result"))
    eng.Stop()
}
```

### 示例 2: HTTP API 调用外部程序

```bash
# 启动服务
.\bin\lunartick.exe --api-port 36800

# 另一个终端，调用外部程序
curl -X POST http://localhost:36800/api/run \
  -H "Content-Type: application/json" \
  -d '{"path":"cmd","args":["/c","dir","C:\\"]}'

# 查看状态
curl http://localhost:36800/api/status

# 设置变量
curl -X POST http://localhost:36800/api/variables \
  -H "Content-Type: application/json" \
  -d '{"name":"target","value":"LunarTick"}'

# 注入代码块
curl -X POST http://localhost:36800/api/inject \
  -H "Content-Type: application/json" \
  -d '{"lines":["@LOG \u0027Hello, #target!\u0027","@stop"]}'
```

### 示例 3: 加载 Markdown 脚本

```bash
curl -X POST http://localhost:36800/api/load \
  -H "Content-Type: application/json" \
  -d '{
    "source": "```LunarTick\n@SET #count \u00270\u0027\n@CYCLE #count \u0027 \u0027\n@LOG \u0027Count: #count\u0027\n@END\n@log \u0027done!\u0027\n@stop\n```",
    "format": "markdown"
  }'
```

### 示例 4: 作为库模块集成

```go
package main

import (
    "sync"
    "time"
    "lunartick/engine"
)

func main() {
    eng := engine.NewEngine(50 * time.Millisecond)

    var wg sync.WaitGroup
    wg.Add(1)

    eng.SetLogFn(func(msg string) {
        println("[LunarTick]", msg)
    })

    eng.Inject([]string{
        "@SET #name 'World'",
        "@log 'Hello, #name!'",
        "@stop",
    })

    go func() {
        eng.Start()
        wg.Done()
    }()

    time.Sleep(500 * time.Millisecond)
    eng.Shutdown()
    wg.Wait()

    println("引擎已安全关闭")
}
```

---

## 项目结构

```
subsystem/LunarTick/
├── api/
│   ├── handler.go          # API 请求处理器（路由注册、请求处理、响应写入）
│   ├── server.go           # HTTP 服务器封装（启动、停止、优雅关闭）
│   └── types.go            # API 请求/响应数据结构定义
├── cmd/
│   └── lunartick/
│       └── main.go         # CLI 入口（命令行参数解析、引擎初始化、信号处理）
├── engine/
│   ├── block.go            # 代码块管理器（创建、状态管理、生命周期）
│   ├── engine.go           # 引擎编排层（对外统一接口）
│   ├── executor.go         # 进程执行器（外部程序调用、stdout/stderr 捕获）
│   ├── expression.go       # 表达式求值器（变量引用、字符串拼接、条件判断）
│   ├── extended_test.go    # 扩展单元测试（覆盖率 ≥ 81%）
│   ├── engine_test.go      # 引擎测试
│   ├── expression_test.go  # 表达式测试
│   ├── ticker_test.go      # 调度器测试
│   ├── instructions.go     # 指令集实现（25+ 种指令）
│   ├── parser.go           # 指令解析器（文本行 → 结构化指令）
│   ├── pointer.go          # 指针系统（注册、构建、调用）
│   ├── ticker.go           # Tick 调度器（生命周期循环、阶段管理）
│   ├── types.go            # 核心类型定义（指令、代码块、等待条件）
│   └── variable.go         # 变量存储系统（模式控制、原子操作）
├── bin/
│   └── lunartick.exe       # 编译产物（可执行程序）
├── go.mod                  # Go 模块定义
├── go.sum                  # 依赖校验
├── build.ps1               # 构建脚本
└── README.md               # 本文档
```

---

## 构建与测试

### 构建可执行程序

```bash
cd d:\Lunar_Astral_Agents\subsystem\LunarTick
go build -o bin/lunartick.exe ./cmd/lunartick/
```

### 运行单元测试

```bash
# 运行所有测试
go test ./... -count=1

# 查看覆盖率
go test ./engine/... -cover -count=1

# 生成覆盖率报告
go test ./engine/... -coverprofile=coverage.out -count=1
go tool cover -func=coverage.out
go tool cover -html=coverage.out  # 浏览器查看
```

当前测试覆盖率: **≥ 81%**

### 运行 API 集成测试

```bash
# 终端 1: 启动服务
.\bin\lunartick.exe --api-port 36800

# 终端 2: 运行 API 测试
curl -X GET http://localhost:36800/api/health
curl -X POST http://localhost:36800/api/inject -H "Content-Type: application/json" -d '{"lines":["@LOG \"test\"","@stop"]}'
```