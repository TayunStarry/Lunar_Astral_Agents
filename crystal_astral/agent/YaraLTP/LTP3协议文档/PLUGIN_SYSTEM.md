# YaraFlow 插件系统架构文档

## 一、总体架构

YaraFlow 插件系统采用 **Goja JavaScript 沙箱** 作为运行时，插件使用 **JavaScript** 编写（不支持 TypeScript，因为内置 TS 编译器会显著增加运行时体积），运行在由 Go 语言宿主管理的隔离沙箱中。

### 架构层次

```
┌─────────────────────────────────────────────────────────┐
│                    YaraFlow 主程序                        │
│  ┌───────────┐  ┌──────────┐  ┌──────────────────────┐ │
│  │  LLM 引擎  │  │ 消息路由  │  │  PluginManager      │ │
│  │ (回复器)  │  │          │  │  (插件管理器)        │ │
│  └───────────┘  └──────────┘  └──────────┬───────────┘ │
│                                          │              │
│  ┌───────────────────────────────────────┴───────────┐ │
│  │  Goja Sandbox (JS 沙箱) × N                        │ │
│  │  ┌─────────────┐  ┌─────────────┐                  │ │
│  │  │  插件 A      │  │  插件 B      │  ...            │ │
│  │  │  (index.js)  │  │  (index.js)  │                 │ │
│  │  └─────────────┘  └─────────────┘                  │ │
│  └───────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────┘
```

### 核心组件

| 组件                  | 文件                   | 职责                                                |
| ------------------- | -------------------- | ------------------------------------------------- |
| **PluginManager**   | `manager.go`         | 插件生命周期管理：发现、加载、卸载、重载、热加载                          |
| **GojaRuntime**     | `runtime.go`         | 单插件运行时：编译、执行 JS 脚本、生命周期函数调度                       |
| **Sandbox**         | `sandbox.go`         | 安全沙箱：超时控制、全局 API 注入、`setTimeout`/`setInterval` 禁止 |
| **APIRegistry**     | `api_inject.go`      | API 注册中心：管理所有注入器，注入 `yara.*` 全局对象                 |
| **InjectorContext** | `api_inject_base.go` | 共享上下文：持有沙箱引用、回调注册表、Hook/命令/工具定义                   |
| **PluginManifest**  | `manifest.go`        | 插件清单：声明式定义插件的元数据、权限、组件                            |

***

## 二、插件分类体系

插件通过 **六种组件** 与主程序交互。**组件由插件 JS 代码通过** **`yara.*.register()`** **注册（函数驱动），`plugin.json`** **不声明组件**：

| 组件类型         | JS 注册 API                      | 说明           |
| ------------ | ------------------------------ | ------------ |
| Command      | `yara.command.register()`      | 正则匹配用户指令     |
| Tool         | `yara.tool.register()`         | LLM 可调用的函数工具 |
| Hook         | `yara.hook.register()`         | 消息处理管线拦截/观察  |
| EventHandler | `yara.eventHandler.register()` | 系统事件响应处理     |
| API          | `yara.api.register()`          | 跨插件 API 暴露   |
| LLM Provider | `yara.llmProvider.register()`  | 自定义 LLM 模型接入 |

### 2.1 指令 (Command)

指令是用户通过聊天界面触发的操作，通过正则表达式匹配用户输入。

- **定义方式**：在 JS 代码中通过 `yara.command.register()` 注册指令名、正则模式与处理器

- **触发条件**：用户消息匹配正则表达式

- **典型用途**：`/查天气`、`/翻译`、`/统计` 等斜杠命令

### 2.2 工具 (Tool)

工具是插件提供的可调用函数，分为三种类型：

| 类型           | 常量           | 可见性       | 说明                                     |
| ------------ | ------------ | --------- | -------------------------------------- |
| **Agent 工具** | `agent`      | `visible` | LLM Agent 可主动调用，通过 Function Calling 机制 |
| **自主运行工具**   | `autonomous` | `hidden`  | LLM 不可见，由 Hook 触发自动执行                  |
| **核心工具**     | `core`       | 始终可见      | 主程序核心功能，始终暴露给 LLM                      |

#### 工具可见性

| 可见性        | 常量                     | 说明     |
| ---------- | ---------------------- | ------ |
| `visible`  | 对 LLM 完全可见，Agent 可主动调用 | <br /> |
| `hidden`   | 对 LLM 隐藏，仅程序内部/自主运行    | <br /> |
| `deferred` | 延迟可见，默认不暴露给 LLM（按需加载）  | <br /> |

#### 自主运行工具的工作流程

自主运行工具用于实现消息拦截和处理——例如检测到 B站视频链接时自动解析视频内容并注入回聊天流：

```
1. 用户消息到达 → 2. Hook 触发 (chat.receive.before_process)
    → 3. 插件中的自主工具检查消息内容
    → 4. 如果匹配（如检测到视频链接），执行处理逻辑
    → 5. 通过 yara.send.text() 将结果注入聊天流
    → 6. Hook 返回拦截结果
```

***

## 三、生命周期

```
┌──────────┐    ┌──────────┐    ┌──────────┐    ┌──────────┐
│ DISCOVER │───▶│  LOAD    │───▶│  RUNNING │───▶│  UNLOAD  │
│ (发现)    │    │ (加载)    │    │ (运行中)  │    │ (卸载)    │
└──────────┘    └──────────┘    └──────────┘    └──────────┘
                     │                ▲               │
                     └────────────────┘               │
                          RELOAD (重载)                │
                                                      ▼
                                               ┌──────────┐
                                               │  ERROR   │
                                               │ (异常)    │
                                               └──────────┘
```

### 3.1 发现阶段 (Discover)

1. `PluginManager` 扫描 `plugins/` 目录下的所有子目录
2. 查找 `plugin.json` 文件
3. 解析并校验清单 (`LoadManifest`)
4. 按依赖关系拓扑排序

### 3.2 加载阶段 (Load)

1. 创建 `APIRegistry` 和 `GojaRuntime`
2. 创建 `Sandbox`（Goja 虚拟机实例）
3. **注入阶段**：`api.Inject()` 遍历所有注入器，根据 `plugin.json` 中的 `permissions` 字段决定注入哪些 API
4. **编译阶段**：读入 `main` 脚本文件，编译为 Goja 字节码
5. **执行阶段**：运行编译后的程序，调用 `onLoad()` 生命周期函数

### 3.3 运行阶段 (Running)

- 插件通过 `yara.*` API 与主程序交互

- 注册的 Hook 在对应事件触发时被回调

- 注册的命令在用户输入匹配时被调用

- 注册的工具在 LLM 决策后被调用

### 3.4 卸载阶段 (Unload)

1. 调用 `onUnload()` 生命周期函数
2. 清理所有 Hook 注册
3. 清理所有事件订阅
4. 清理所有命令注册
5. 中断沙箱执行
6. 释放所有回调引用

### 3.5 热加载 (Hot Reload)

插件管理器支持文件变化监听（基于 `fsnotify`），当插件目录中的文件发生变更时：

- 500ms 防抖后再触发重载

- 自动执行 Unload → Load 流程

- 忽略临时文件（`~` 结尾）和隐藏文件（`.` 开头）

***

## 四、安全沙箱

### 4.1 隔离机制

- **独立 VM**：每个插件运行在独立的 `goja.Runtime` 实例中，互不干扰

- **超时控制**：30 秒超时，超时后中断执行

- **禁止 API**：`setTimeout`、`setInterval` 被明确禁用（调用会抛出 TypeError，因沙箱无事件循环）；延时请用 `yara.time.sleep(ms)`

- **路径遍历防护**：所有文件操作均经过 `filepath.Clean` + `filepath.HasPrefix` 检查

### 4.2 权限系统

插件必须在 `plugin.json` 的 `permissions` 字段中声明所需权限。未声明的权限对应的 API 不会被注入。

权限列表详见 [插件开发文档](./PLUGIN_DEV_GUIDE.md)。

### 4.3 资源限制

- 脚本执行超时：30 秒

- 跨插件 API 调用超时：30 秒

- HTTP 请求超时：30 秒

- 异步任务默认超时：300 秒（5 分钟）

***

## 五、API 注入系统

> **`yara`** **命名空间**：`yara` 是 YaraFlow 注入到每个插件沙箱的全局 API 命名空间（`APIRegistry` 合并出的 `yara` 全局对象），不是某个具体的人或架构名。插件通过 `yara.*` 与主程序交互，各 API 的详细用法见 [插件开发文档的 API 参考](./PLUGIN_DEV_GUIDE.md#完整-api-参考)。

### 5.1 注入器架构

每个 API 模块对应一个注入器（Injector），实现 `APIInjector` 接口：

```go
type APIInjector interface {
    Inject() error   // 将 API 注入到沙箱中
    APIName() string // 返回 API 名称（对应 yara 全局对象下的 key）
}
```

### 5.2 已实现的注入器（共 23 个）

| 注入器                  | yara 键              | 功能                                           |
| -------------------- | ------------------- | -------------------------------------------- |
| EventInjector        | `yara.event`        | 事件发布/订阅                                      |
| HookInjector         | `yara.hook`         | Hook 注册/注销                                   |
| CommandInjector      | `yara.command`      | 指令注册                                         |
| MessageInjector      | `yara.send`         | 消息发送                                         |
| LoggerInjector       | `yara.logger`       | 日志输出                                         |
| ToolInjector         | `yara.tool`         | 工具注册/管理                                      |
| FileInjector         | `yara.file`         | 文件操作                                         |
| EncodingInjector     | `yara.encoding`     | 编解码（base64/hex/URL/UTF-8）                    |
| TimeInjector         | `yara.time`         | 时间工具（格式化/解析/时长计算）                            |
| CryptoInjector       | `yara.crypto`       | 加解密/哈希（MD5/SHA/HMAC/Ed25519/JWT）             |
| EventHandlerInjector | `yara.eventHandler` | 事件处理器注册                                      |
| LLMProviderInjector  | `yara.llmProvider`  | LLM 提供商注册                                    |
| APIRegisterInjector  | `yara.api`          | 自定义 API 暴露                                   |
| ConfigFileInjector   | `yara.configFile`   | 配置文件管理                                       |
| ModelAccessInjector  | `yara.model`        | LLM 模型调用                                     |
| HTTPInjector         | `yara.http`         | HTTP 请求                                      |
| AsyncTaskInjector    | `yara.async`        | 异步任务                                         |
| NetworkInjector      | `yara.network`      | TCP/UDP/DNS 网络通信                             |
| DatabaseInjector     | `yara.database`     | 数据库查询                                        |
| EmojiInjector        | `yara.emoji`        | 表情包管理                                        |
| KnowledgeInjector    | `yara.knowledge`    | 知识库搜索                                        |
| PlatformInjector     | `yara.platform`     | 平台命令/用户查找                                    |
| ImageInjector        | `yara.image`        | 图片缓存读取/校验（`getCached`/`loadValid`/`isImage`） |

### 5.3 注入流程

```
NewAPIRegistry
  └─ NewInjectorContext (创建共享上下文)
  └─ registerBuiltinInjectors (注册所有注入器)
  └─ Inject()
       └─ 遍历所有注入器
            └─ 每个注入器检查 permissions
                 ├─ 权限不足 → 跳过，返回 nil
                 └─ 权限满足 → 调用 mergeIntoYara() 注入到 yara 全局对象
```

***

## 六、Hook 系统

Hook 允许插件在消息处理的各个阶段插入自定义逻辑。

### 6.1 支持的 Hook 点

**聊天消息链：**

- `chat.receive.before_process` — 入站消息处理前

- `chat.receive.after_process` — 入站消息处理后

**命令执行链：**

- `chat.command.before_execute` — 命令执行前

- `chat.command.after_execute` — 命令执行后

**表情包链：**

- `emoji.chat.before_select` — 表情选择前

- `emoji.chat.after_select` — 表情选择后

- `emoji.register.after_build_description` — 表情描述生成后

- `emoji.register.after_build_emotion` — 表情情绪标签生成后

**发送服务链：**

- `send_service.after_build_message` — 消息构建后

- `send_service.before_send` — 消息发送前

- `send_service.after_send` — 消息发送后

**规划器链：**

- `chat.planner.before_request` — 规划器请求前

- `chat.planner.after_response` — 规划器响应后

**回复器链：**

- `chat.replyer.before_request` — 回复器请求前

- `chat.replyer.before_model_request` — 回复器模型请求前

- `chat.replyer.after_response` — 回复器响应后

**黑话链：**

- `jargon.query.before_search` — 黑话查询前

- `jargon.query.after_search` — 黑话查询后

- `jargon.extract.before_persist` — 黑话写库前

- `jargon.inference.before_finalize` — 黑话推断结果写回前

**表达方式链：**

- `expression.select.before_select` — 表达方式选择前

- `expression.select.after_selection` — 表达方式选择后

- `expression.learn.after_extract` — 表达方式学习解析后

- `expression.learn.before_upsert` — 表达方式写库前

### 6.2 Hook 模式

| 模式         | 说明                    |
| ---------- | --------------------- |
| `blocking` | 阻塞模式，Hook 执行完成后才继续    |
| `observe`  | 观察模式，Hook 异步执行，不阻塞主流程 |

### 6.3 Hook 能力约束

每个 Hook 点有特定的能力约束：

- `AllowBlocking` — 是否允许阻塞模式

- `AllowObserve` — 是否允许观察模式

- `AllowAbort` — 是否允许中止后续流程

- `AllowKwargsMutation` — 是否允许修改传递的参数

### 6.4 消息修改能力

插件可以通过 Hook 返回 `modifiedData` 来修改消息内容，这对于将系统原本不处理的链接、文件等消息替换为 LLM 可读内容非常有用。

**支持消息修改的 Hook 点：**

| Hook 点                        | 修改时机 | 说明                                 |
| ----------------------------- | ---- | ---------------------------------- |
| `chat.receive.before_process` | 预处理前 | 修改原始消息，后续所有处理（图片分析、命令识别等）都基于修改后的内容 |
| `chat.receive.after_process`  | 预处理后 | 修改处理后的消息，直接影响 LLM 看到的内容            |

**修改流程：**

```
消息到达
  │
  ▼
HookChatReceiveBeforeProcess
  │  ← 插件可返回 modifiedData 修改 content
  │  ← 修改后流入后续预处理（图片分析等）
  ▼
预处理（图片分析、@解析等）
  │  ← 基于修改后的 content 处理
  ▼
HookChatReceiveAfterProcess
  │  ← 插件可再次返回 modifiedData 修改 content
  ▼
SetContext(processedMsg)  ← 聊天历史存的是修改后的消息
  │
  ▼
gateMessage → replyMessage  ← LLM 看到的是修改后的内容
```

**Hook 返回值结构：**

```javascript
{
  allowContinue: true,
  modifiedData: {
    content: "替换后的消息内容",
    senderName: "新的发送者名称"
  }
}
```

**Session 上下文传递：**

在 `chat.receive.before_process` 和 `chat.receive.after_process` 两个 Hook 点，以及命令执行时，系统会自动传入 `session` 上下文。这意味着：

- 插件在 Hook 处理器中调用 `yara.send.text()` 等发送方法时，消息会自动记入聊天历史

- 插件在命令处理器中调用 `yara.send.text()` 时，消息也会自动记入聊天历史

- 智能体能看到插件发送的所有消息

***

## 七、事件系统

插件可以订阅事件总线上的事件，事件类型包括：

| 事件类型                     | 说明      |
| ------------------------ | ------- |
| `ON_START`               | 系统启动    |
| `ON_STOP`                | 系统停止    |
| `ON_MESSAGE_PRE_PROCESS` | 消息预处理   |
| `ON_MESSAGE`             | 收到消息    |
| `ON_PLAN`                | 规划阶段    |
| `POST_LLM`               | LLM 调用后 |
| `AFTER_LLM`              | LLM 响应后 |
| `POST_SEND_PRE_PROCESS`  | 发送前预处理  |
| `POST_SEND`              | 发送后     |
| `AFTER_SEND`             | 发送完成后   |

***

## 八、插件间通信

### 8.0 沙箱隔离与通信模型

**每个插件运行在独立的沙箱（独立 goja VM）中，全局作用域、变量、对象完全隔离**：插件 A 无法读取或修改插件 B 的任何状态，反之亦然。主程序在加载每个插件时创建专属 VM（`NewGojaRuntime → NewSandbox → goja.New()`），并在该 VM 内注入独立的 `yara.*` API。

因此插件之间**不存在共享内存/全局变量**，所有跨插件交互都由主程序作为"中间人"转发，只有两条通道：

| 通道 | API | 语义 |
|------|-----|------|
| 跨插件函数调用 | `yara.api.register()` / `yara.api.call()` | 同步调用另一插件暴露的 API，可传参并取回返回值 |
| 事件广播 | `yara.event.publish()` / `yara.event.subscribe()` | 异步广播/订阅事件，发布方与订阅方完全解耦 |

这正符合"每个模块是完全独立的运行空间、通过一套协议跨模块调用"的模型：**全局作用域是插件私有的，跨模块只通过协议（API 调用 / 事件）通信**，主程序负责在沙箱间推送数据。

### 8.1 跨插件 API 调用

插件可以通过 `yara.api` 暴露自定义 API，其他插件通过 `yara.api.call()` 调用（由主程序路由到目标插件的沙箱并返回结果）：

```
插件 A: yara.api.register("getWeather", handler)         // A 在自己沙箱内注册
插件 B: yara.api.call("com.example.plugin-a.getWeather", params)  // B 通过主程序调用 A
```

### 8.2 事件总线

插件通过 `yara.event.subscribe()` 和 `yara.event.publish()` 实现松耦合通信（事件经主程序总线转发，订阅方在各自沙箱内收到回调）。

***

## 九、目录结构

```
plugins/
├── com.example.my-plugin/
│   ├── plugin.json          # 插件清单（必需）
│   ├── metadata.json        # LTP 包元信息（含 LTP3/LTPX 标签，琉璃管理页展示用）
│   ├── index.js             # 主入口脚本（在 plugin.json 的 main 字段指定）
│   ├── index.html           # 插件介绍页（琉璃管理页展示，可选）
│   ├── script.js            # 介绍页脚本（可选）
│   ├── styles.css           # 介绍页样式（可选）
│   ├── data/                # 插件数据目录（运行时自动创建）
│   ├── config.yaml          # 插件配置文件（由 plugin.json 的 config.configFile 指定唯一文件，首次运行自动生成）
│   └── locales/             # 国际化资源（可选）
│       ├── zh-CN.json
│       └── en-US.json
└── com.example.another-plugin/
    └── ...
```

