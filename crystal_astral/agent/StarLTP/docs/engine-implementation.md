# LTP9 引擎开发文档（Engine Implementation & Plugin Development Guide）

> 定位：在「星月智能」LTPX 协议家族中的引擎分支 LTP9。LTP9 采用插件层 / 引擎层 / 客户端三端组织；每插件运行于**独立 goja 沙箱**，以**同步回调**与引擎互动；权限由**代码哈希加密的 `permissions.key`** 声明；内置跨包函数调用与 Mini-LTP / Node-LTP 前端智能体调度能力。

---

## 1. 协议定位与设计目标

| 项 | LTP9 |
|----|------|
| 载体 | CodeAgent（进程内 goja 兼容层/事件容器） |
| 语言规范 | **ES2023+ 语法**（可选链、空值合并、类、BigInt 等）；插件回调统一为**同步函数**，切勿使用异步操作 |
| 沙箱 | **每插件一个独立 goja Runtime 实例**（`goja_nodejs/eventloop` 提供事件循环与定时器） |
| 组织架构 | 插件层 / 引擎层 / 客户端（三端） |
| 插件层事件 | **事件订阅器**（`engine.event.<topic>.subscribe/unsubscribe`），支持**拦截、改写事件参数** |
| 权限系统 | `allow-*`（文件 / 数据库 / 记忆库 / 网络 / 调用 / agent / 广播 / 加解密 / 发送 / WebSocket） |
| 授权认证 | **`permissions.key`**：代码哈希作解密密钥，权限声明与代码强绑定 |
| 内置能力 | 同步 HTTP（`engine.http`）、同步休眠（`engine.sleep`）、fetch/WebSocket 全局、定时器、时间戳、内存事件总线、跨包调用、前端智能体调用、lunar-decoder 加解密 |

**设计目标**：一个插件 = 一段（打包后的）js 运行在金属隔离的 goja 沙箱里，通过一组**白名单 API** 与引擎互动；引擎负责权限校验、事件路由、跨端调度；客户端负责发起事件并**同步等待**脚本回调结果。

---

## 2. 组织架构（三端布局）

```
┌──────────────────────── 客户端（Client）────────────────────────┐
│ 实质上“发起事件并等待脚本回调结果”的程序：                        │
│  - crystal_astral（琉璃）/ lunar_astral（月华）                 │
│  - 事件源：QQ/NapCat 消息、命令、前端页面操作、外部 WebSocket 等   │
│  - 调用方式：Emit(topic, payload) → 同步等待引擎回执             │
└──────────────────────────────┬─────────────────────────────────┘
                               │ Emit(事件) + 同步等待(回调结果)
┌──────────────────────────── 引擎层（Engine）─────────────────────┐
│  - 插件生命周期：扫描/加载/卸载/对账（reconcile）                 │
│  - 每插件独立沙箱（goja Runtime + eventloop）                    │
│  - API 绑定器（把 engine.* 注册进沙箱，按 allow-* 注入）          │
│  - 权限校验器（permissions.key + 沙箱内能力开关）                │
│  - 事件总线（topic → 订阅器列表，含拦截/改写/撤回调度）           │
│  - 跨端调度：engine.call / engine.agent（Mini-LTP / Node-LTP）   │
│  - 加解密桥：engine.encoder / engine.decoder → lunar_decoder     │
└──────────────────────────────┬─────────────────────────────────┘
                               │ LoadPlugin / route / broadcast
┌──────────────────────────── 插件层（Plugin）─────────────────────┐
│  LTP9 插件包（metadata.json + execute.js + config.yaml + ...）   │
│  在 execute.js 中调用 engine.event.* / engine.signal.* /         │
│  engine.http / engine.file / database / memory / call / agent 等  │
└──────────────────────────────────────────────────────────────────┘
```

客户端与引擎之间的唯一联动方式：**客户端发起事件 → 引擎路由到已订阅该事件的插件回调 → 引擎把各插件回调结果汇总回执给客户端**。

---

## 3. 插件包结构（LTP9 最简架构）

一个标准 LTP9 插件包（目录即插件 ID，如 `local_data/package/xxx/`）：

```
<包目录>/
├── metadata.json      # 插件元数据（id/title/version/icon/tags）
├── icon.webp          # 插件图标
├── execute.js         # 插件的代码实现
├── README.md          # 插件说明
├── config.yaml        # 插件配置
└── permissions.key    # 授权凭证（被代码文件哈希加密的 allow-* 权限清单）
```

### 3.1 metadata.json 示例

```json
{
  "id": "com.example.hello",
  "title": "Hello LTP9",
  "description": "示例 LTP9 插件",
  "icon": "icon.webp",
  "version": "1.0.0",
  "tags": ["LTP9"]
}
```

### 3.2 config.yaml

插件默认配置；引擎在沙箱启动时解析并注入到 `engine.config.getFile()`。插件可在运行时用 `engine.config.setFile()` 回写（同步写回磁盘 config.yaml）。

### 3.3 permissions.key（授权认证）

`permissions.key` **既是密钥凭证、也是权限声明**，与代码文件 `execute.js` **强绑定**：

- **机制**：授权时，引擎取绑定代码文件的**哈希**作为密钥，把插件获许的权限清单**加密**写入 `permissions.key`。
- **加载**：引擎对当前代码文件重新计算哈希，用该哈希解密 `permissions.key`，解出权限清单后加载插件。
- **效果**：授权与「原版未改动的代码文件」绑定；引擎以当前代码哈希解出权限清单，未解出对应权限则拒绝该插件加载。

### 3.4 配套密钥生成模块（LTP9 内置）

LTP9 使用**配套的密钥生成模块** `subsystem/ltp9_keygen` 生成 `permissions.key`，职责与约束：

1. **输入**：打包后的 `execute.js`（或插件包目录，后端自行读取 `execute.js` 保证与引擎哈希规则一致）+ 开发者勾选的 `allow-*` 权限清单。
2. **哈希密钥**：对绑定代码文件计算哈希（与引擎端一致的规则：`sha256(execute.js)[:16]` hex），作为解密密钥。
3. **权限清单编码**：把授权列表编码为密文写入 `permissions.key`（整体加密，每项 `allow-*` 权限名填充到定长后以 `+` 连接）。
4. **一致性**：生成器的哈希、加密、权限解析规则与 LTP9 引擎端的 `verifyPermissions` 保持一致；生成器内置校验入口，供提交前确认 `permissions.key` 能被引擎规则解出预期权限。
5. **解码失败即拒绝**：引擎与生成器校验在解码失败时拒绝授权。

---

## 4. 沙箱模型（每插件独立 goja + 同步回调）

### 4.1 隔离原则

- **每插件一个独立 `goja.Runtime`**：插件 A 的全局变量、定时器、网络连接、注册表各自独立。
- **插件自包含为单一 `execute.js`**：多脚本经打包器拼接为单个自包含文件，作为沙箱加载的完整脚本。
- **沙箱运行时**：每插件以 `goja_nodejs/eventloop.NewEventLoop()` 各建一个独立 loop（其内部 `goja.New()` 新建独立 Runtime，自带定时器），网络能力由引擎绑定（同步 `engine.http` 与全局 `fetch`/`WebSocket`）。

### 4.2 引擎侧沙箱生命周期

```
load(pluginID):
  1. 解析 config.yaml 注入 p.config（失败仅告警，不阻断加载）
  2. 以代码文件哈希解密 `permissions.key`，解出权限声明（开发模式授予全部 allow-*）
  3. 新建该插件独立 goja 事件循环
  4. 按权限清单注入对应 engine.* 能力与网络全局（allow-network → engine.http / fetch / WebSocket）
  5. 执行 execute.js（注册订阅器、导出函数、生命周期回调）
  6. 调用 onLoad；注册完成后该插件开始接收事件

unload(pluginID): 调用 onUnload → 停止事件循环 → 清空订阅器/导出/配置
```

### 4.3 语言能力（同步回调模型）

沙箱基于 goja（支持 ES2022+/部分 ES2023）。常用能力：
- 类、可选链（`a?.b`）、空值合并 `??`、逻辑赋值、`Array.prototype.at`、BigInt 等；插件以**回调注册**为统一入口。
- **沙箱全局**：注入 `console`（log/info/warn/error/debug，输出走引擎日志）与定时器（`setTimeout`/`setInterval`/`setImmediate` 及对应 clear）。
- **同步为主、异步可用**：事件订阅回调、`engine.export` 导出的函数优先写成**同步函数**返回普通对象；需要网络时用阻塞式 `engine.http.get/post`，需要等待时用 `engine.sleep(ms)`。
- `fetch` / `WebSocket` 全局以 Promise / 事件回调形态存在（`allow-network` 门控），供需要异步连接的场景使用。**异步亦被支持**：若回调/导出函数返回 pending Promise（如 `async` 函数、`await fetch` 的结果），`callFn` 会让出插件事件循环并定时轮询直到 Promise 兑现或超时（`callAwaitTimeout`，默认 90s），因此 `engine.export` 的跨插件调用（`engine.call`）、事件订阅器等也能正确承接 `async/await`。

### 4.4 定时器与时间戳（来自 goja_nodejs/eventloop）

**定时器随 `goja_nodejs/eventloop.NewEventLoop()` 自动注入**：每次构造时向 vm 注册 `setTimeout / setInterval / setImmediate / clearTimeout / clearInterval / clearImmediate`。时间戳由引擎暴露：

```js
engine.time.now()            // 秒级时间戳
engine.time.nowMs()          // 毫秒时间戳
const id = setTimeout(fn, ms)
const ivi = setInterval(fn, ms)
clearTimeout(id)
clearInterval(ivi)
setImmediate(fn); clearImmediate(id)
```

> 说明：定时/时间戳能力来自 `goja_nodejs/eventloop`。回调都会排入该插件 loop 的事件循环，天然线程安全（`goja.Runtime` 只在 loop 线程内使用）。

### 4.5 网络模块（同步 + 异步双形态）

沙箱内提供两套网络能力，全部由 `allow-network` 门控：

**同步形态（插件主链路推荐）**：

```js
const r = engine.http.get(url, headers?)   // 阻塞直到返回 → { status, body, error? }
const r = engine.http.post(url, body, headers?) // 同上，body 为字符串
```

**异步全局形态（连接型场景）**：

```js
const r = await fetch(url, {method, headers, body})  // 标准 Fetch，返回 Promise<Response>
r.status / r.headers / await r.text() / r.json() / r.arrayBuffer()

const ws = new WebSocket("wss://...")
ws.onopen / ws.onmessage({data}) / ws.onerror / ws.onclose
ws.send(text); ws.close()
```

- `engine.http` 与 `fetch` 共用同一套请求实现（`api_net.go`），`fetch`/`WebSocket` 的 resolve/reject 与事件回调排到该插件自己的 `eventloop` 线程。

---

## 5. 运行时 API（`engine.*`）

以下命名空间由引擎绑定器按 `allow-*` 注入每个沙箱。**未获权限的能力对应命名空间为 `undefined`**，脚本调用即触发 TypeError；获得权限的调用被拒时返回带 `error` 的对象。

### 5.1 事件订阅器 `engine.event`

LTP9 以**事件订阅器** `engine.event.subscribe(topic, handler, priority)` 承接插件的事件订阅与管理。同一个事件的订阅器**可重复 subscribe**（允许多个回调），按**优先级 + 时间顺序**串行派发；订阅回调**可拦截、改写客户端触发该事件时的参数，以及撤回该事件**。发布方向见 §5.14 `engine.event.publish`。

```js
// 订阅某事件（topic 为主题字符串）；返回订阅 id（引擎按单调递增分配）
let id = engine.event.subscribe("weather.query", (event) => {
  // event = { type: topic, payload: 客户端触发负载 }
  // 放行：返回任意业务结果对象（非 Promise）
  return { handled: true, note: "ok" }
  // 拦截：返回 { intercept:true } 停止该插件后续订阅器处理；
  // 改写：返回 { modifiedData:{...} } 供下游订阅器（含其他插件）读取改写后的负载；
  // 回传：返回 { return:<业务结果> } 把本次事件的业务结果回执给客户端（不参与负载链）；
  // 撤回：返回 { cancel:true } 使本次事件不再派发，回执标记为已撤回。
}, 0)  // 可选优先级：仅支持正整数，0 最高；未设置则按时间顺序
// 取消订阅
engine.event.unsubscribe("weather.query", id)
```

- 事件名（topic）为主题字符串（任意合法主题，如 `weather.query` / `signal` / `execute_search_before`）。
- **派发顺序**：订阅器按优先级升序（数值小者优先，`0` 最高）排列；同优先级按订阅时间顺序；未设置优先级的订阅排在所有设置者之后并按各自时间顺序。若全部未设置优先级，则纯粹按订阅时间顺序派发。
- 引擎把客户端触发的原始负载构造为事件对象 `{type, payload}`，按上述顺序串行传入所有订阅器，最后汇总结果回执客户端。

### 5.2 前端事件（广播接收） `engine.frontEvent`

```js
engine.frontEvent.signal.subscribe((data) => { /* 接收 engine.signal 广播 */ })
engine.frontEvent.signal.unsubscribe(id)
```

引擎层存在两条独立总线：
- **事件订阅器总线**（`engine.event.<topic>`，客户端发起，带结果回执）。
- **广播总线**（`engine.signal` / `engine.frontEvent.signal`，插件单向通报）。

### 5.3 广播 `engine.signal`

```js
engine.signal.all(payload)          // 向所有插件广播
engine.signal.target(pkgId, payload) // 仅向目标插件广播
```

配合 `engine.frontEvent.signal.subscribe` 使用，用于插件间 / 引擎→插件单向信息通报。`allow-signal` 控制。

### 5.4 数据库 `engine.database`

```js
engine.database.query(sql, params?)   // 读 → { success, rows: [{列名:值}...] }
engine.database.exec(sql, params?)    // 写（INSERT/UPDATE/DELETE）→ { success, rows_affected }
```

接入项目 SQLite（mattn/go-sqlite3），所有插件共享 `local_data/database/knowledge.db`（WAL 模式）。`allow-database` 控制。

### 5.5 向量记忆库 `engine.memory`

```js
engine.memory.store({ content, tags })     // 写入 → { success, id }
engine.memory.search({ query, limit })     // 检索 → { success, results: [{content, similarity}...] }
```

接入项目记忆库（FileManager/module 的向量检索），`ltp9_memory` 集合按需惰性初始化。记忆库为**集合级管理**，不提供按 id 单条读删。`allow-memory` 控制。

### 5.6 文件 `engine.file`

```js
engine.file.write(path, data)      // 写 → { success } / { success:false, error }
engine.file.read(path)             // 读 → { success, text } / { success:false, error }
engine.file.delete(path)           // 删 → { success } / { success:false, error }
```

`engine.file` 路径作用于插件数据目录（`<包目录>/data/`），引擎对 path 做沙箱化校验（越界拒绝）。`allow-file` 控制。

### 5.7 跨包函数调用 `engine.call`

```js
let x = engine.call(pkgId).run(fnName, [args])   // 同步阻塞，直接返回目标函数值
```

- 目标插件必须暴露该函数（在其 execute.js 里注册 `engine.export(fnName, (...args) => ...)`，同步函数）。
- 引擎负责串行转发并在目标沙箱事件循环执行；目标插件不存在/未导出该函数 → 抛出 `xxx 包拒绝响应`。

### 5.8 调用前端智能体 `engine.agent`（内置 Mini-LTP / Node-LTP）

```js
let x = engine.agent(pkgId).run("自然语言需求")   // 同步阻塞，返回智能体执行文本
```

- LTP9 引擎内置把请求转交到 **Mini-LTP / Node-LTP**（WebAgent）的能力：引擎把这些包作为可路由的 agent 目标。
- 若对应包不存在或智能体不存在 → 抛出 `xxx 包拒绝响应`。
- 目标包须在 metadata.agents 里声明、且宿主已注入前端智能体通道（`SetAgentInvoker`）。

### 5.9 加解密 `engine.encoder` / `engine.decoder`（复用 lunar_decoder）

```js
let enc = engine.encoder(密钥, 内容)   // 加密：字符串/字节 → encrypted
let dec = engine.decoder(密钥, enc)   // 解密：enc → 原文
```

实现桥到 `subsystem/lunar_decoder`（`EncodeFilesWithKeyString` / `DecodeFilesWithKeyString`）。`allow-certificate` 控制。

### 5.10 配置 `engine.config`

```js
engine.config.getFile()      // 读 config.yaml（引擎启动时解析注入）
engine.config.setFile(obj)   // 回写（更新内存并同步写回 config.yaml）
```

### 5.11 导出插件能力（供 engine.call 调用）

```js
engine.export("myFunction", (a, b) => { return a + b })   // 同步函数
```

### 5.12 同步网络与休眠

```js
const r = engine.http.get(url, headers?)      // 同步 GET → { status, body, error? }
const r = engine.http.post(url, body, headers?) // 同步 POST
engine.sleep(ms)                              // 同步阻塞等待
```

- `engine.http` 由 `allow-network` 控制；`engine.sleep` 为常驻基础能力。

### 5.13 扩展能力（JWT / 摘要 / 图像 / LLM / 发送 / WebSocket）

以下能力由宿主注入真实通道或读取本地配置，按 `allow-*` 授权后注入：

```js
// JWT 签名（allow-certificate，HS256/EdDSA/none；kid 可选写入 JWT 头）
engine.crypto.signJWT({ sub: "project", iat: now, exp: now + 900 }, secret, "HS256")

// 摘要 / HMAC / Ed25519（allow-certificate，均为小写十六进制）
engine.crypto.md5("text")            // Hex MD5
engine.crypto.sha1("text")           // Hex SHA-1
engine.crypto.sha256("text")         // Hex SHA-256
engine.crypto.hmacSha1(key, data)    // Hex HMAC-SHA1
engine.crypto.hmacSha256(key, data)  // Hex HMAC-SHA256
const sig = engine.crypto.ed25519Sign(pemOrSeed, data) // base64url 签名
const j = engine.crypto.generateJWT(claims, pemOrSeed, kid) // EdDSA JWT

// LLM 对话（allow-agent，读取 lunar_config.json 的 agent.multimodal_model/multimodal_url/multimodal_key）
engine.llm.chat([{ role: "user", content: "..." }], { temperature: 0.7 })
// → { success:true, text: "回复内容" } 或 { success:false, error }

// 图像（allow-file；下载另需 allow-network）：读插件数据目录图片校验后返回 base64
engine.image.loadValid("data/selfie.png")     // { success:true, text: base64 }
engine.image.download(url, "ref_1.png")        // 下载→校验→写数据目录→返回 base64

// 发送（allow-send，宿主注入发送通道，经 /ws 桥接广播）：text/image/hybrid
engine.send.image("g1", b64)                   // { success:true }
engine.send.hybrid("g1", [{ type: "image", content: b64 }, { type: "text", content: "描述" }])

// WebSocket 服务端（allow-socket，宿主注入 WsBridge 传输）
let addr = engine.ws.expose("/ws/flow", (msg) => { /* 返回回给客户端的文本 */ }) // { success:true, path }
engine.ws.publish(JSON.stringify({ ... }))     // { success:true }
```

- `engine.image` / `engine.llm` / `engine.send` / `engine.ws` 未获对应权限时对应命名空间为 `undefined`，脚本调用即 `TypeError`。
- 宿主真实通道由 `SetSendInvoker`（发送）与 `SetWsServer`（WebSocket 传输）注入；未注入时返回 `未注入发送通道` / `未注入 WebSocket 传输`。

### 5.14 事件发布 `engine.event.publish`（常驻）

对称补齐 LTP9 「事件订阅器总线」缺失的**发布**方向：插件可主动在总线上发起一个事件，派发给其它订阅该主题的插件订阅器，并转发给宿主 `outbound`（供外部客户端 / 前端消费）。

```js
engine.event.publish("weather.query", { city: "北京" })   // 发布到主题 weather.query
```

- **异步 fire-and-forget**：`publish` 自身不阻塞、无回执；按订阅器优先级/顺序在各目标插件自己的 loop 内同步派发。
- **跳过发起插件自身**（避免回调内等待自身回调造成自锁 / 自循环）；插件想让自己也消费可自行 `engine.call` 或在 `engine.export` 内处理。
- 复用宿主 `outbound`（= `engine.signal` 同一通路）转发 payload，外部 Go 程序可用既有收包逻辑消费。

### 5.15 工具注册 `engine.tool`（allow-agent）

插件可注册 **LLM / AtoA 可调用的函数工具**，宿主或外部队列经 `star.CallTool(pluginID, name, args)` 触发：

```js
engine.tool.register("get_weather", {
  description: "查询指定城市实时天气",
  parameters: [{ name: "city", type: "string", description: "城市名称", required: true }]
}, function (params) {
  // params.city ...
  return { city: params.city, temperature: 25, condition: "晴" };
});

const defs = engine.tool.getDefinitions(); // 已注册工具定义（name/description/parameters）
```

- 注册覆盖同名工具；`handler` 为同步函数，参数 `params` 为一对象，返回任意业务结果。
- 与 `engine.llm.chat({ tools })` 形成完整闭环：`engine.llm.chat` 可把工具定义交给模型，模型返回 `tool_calls` 后由宿主按 `tool_calls.function.name` + 解析参数调用 `CallTool` 并回填。
- `PluginState.Tools` 暴露各插件已注册工具名，供前端引擎管理器动态下拉。

### 5.16 平台上下文 `engine.platform`（allow-send）

由宿主注入 `SetPlatformResolver(func(method string, args map[string]any) (any, error))` 实现；未注入时返回错误提示：

```js
engine.platform.getName()                        // → { success, value: 平台名 }
engine.platform.getGroupId()                     // → { success, value: 当前群 ID }
engine.platform.lookupUser(groupId, name)        // → { success, value: 用户标识或 null }
```

### 5.17 表情包 `engine.emoji`（allow-memory）

LTP9 表情包**不新增独立存储**，复用项目记忆库的 `stickers`（image 型）集合，与月华侧「表情包记忆库」数据互通：

```js
engine.emoji.search("开心", { limit: 3 })   // → { success, results: [{ image, similarity }] }
engine.emoji.store(base64Image)            // → { success, id }，标签由记忆库 LLM 自动生成
engine.emoji.random("happy")               // → { success, image }，按语义随机返回一张
```

- 首次调用惰性初始化 `stickers` 集合（`CollectionInit(..., CollectionTypeImage)`）；`store` 用 `module.MemoryAddImage`（同步等待 LLM 标签，耗时操作）。
- 对应后端 `ltp9/test` 动作 `emoji`（op: `search` / `store` / `random`）。

### 5.18 文本嵌入 `engine.llm.embed`（allow-agent）

读取 `lunar_config.json` 的 `agent.embedding_*` 文本嵌入模型，调用 OpenAI 兼容 `/v1/embeddings`：

```js
engine.llm.embed("琉璃")                     // → { success: true, embedding: [0.1, ...] }
engine.llm.embed(["a", "b"])                // → { success: true, embeddings: [[...], [...]] }
engine.llm.embed("x", { text: "y" })        // 兼容 opts.text 追加
```

返回嵌入向量与前序文本一一对应；`embed` 与 `chat` 共用 `allow-agent` 门控。

---

## 6. 权限系统（`allow-*`）

LTP9 用**权限密钥 + 沙箱注入控制**双层实现。权限声明由 `permissions.key` 承载；引擎经哈希解密 `permissions.key` 解出该插件获许的 `allow-*` 清单，据此把对应 `engine.*` 命名空间注入沙箱，未列入清单的能力不注入。

| 权限 | 控制的 API | 说明 |
|------|-----------|------|
| `allow-file` | `engine.file.*` / `engine.image.loadValid` | 文件读写删 + 读/校验图片 |
| `allow-database` | `engine.database.*` | SQL 数据库读写 |
| `allow-memory` | `engine.memory.*` | 向量记忆库读写 |
| `allow-network` | `engine.http.*` / `fetch` / `WebSocket` / `engine.image.download` | 同步/异步网络与图片下载 |
| `allow-call` | `engine.call(包ID).run` | 调用其他插件函数 |
| `allow-agent` | `engine.agent(包ID).run` / `engine.llm.chat` / `engine.llm.embed` / `engine.tool.*` | 前端智能体 / LLM 对话 / 文本嵌入 / Agent 工具注册 |
| `allow-signal` | `engine.signal.*` / `engine.frontEvent.signal` | 广播收发 |
| `allow-certificate` | `engine.encoder` / `engine.decoder` / `engine.crypto.*` | 加解密 + JWT/摘要/HMAC/Ed25519 签名 |
| `allow-send` | `engine.send.text/image/hybrid` / `engine.platform.*` | 发送到会话 + 平台上下文（宿主注入，经 /ws 桥接） |
| `allow-socket` | `engine.ws.expose/publish` | WebSocket 服务端（宿主注入传输） |

> `engine.memory.*` 负责向量记忆库读写（`allow-memory`），`engine.emoji.*`（表情包）复用 `allow-memory`（内部访问记忆库 stickers 集合）。

> 基础能力（`engine.event` / `engine.config` / `engine.time` / `engine.sleep` / `engine.export`）常驻注入，不参与 `allow-*` 开关。

> 授权认证：`permissions.key` 是被代码文件哈希加密的权限声明。引擎以当前代码哈希解密 `permissions.key`，解出权限清单后加载插件；开发模式（`GeneralConfig.Developer`）跳过密钥校验并默认授予全部 `allow-*`。

---

## 7. 事件订阅器模型（引擎层核心）

### 7.1 数据结构

```
订阅器注册表（引擎级，按插件存储）：
  topic ──► []eventSub {
    pluginID,       // 归属插件（用于卸载清理）
    orderID,        // 订阅 id（插件内单调递增，同时作为时间顺序依据）
    priority,       // 订阅优先级（0 最高；未设置用 priorityUnset，排在所有设置者之后）
    handler,        // goja FunctionValue（在对应沙箱同步执行）
  }
```

### 7.2 派发顺序

事件订阅器按以下规则排序后串行派发：
1. **优先级升序**：优先级数值小（`0` 最高）者先执行，用于匹配需要提前拦截/改写参数的订阅。
2. **同优先级按时间顺序**：取订阅 id（注册先后）升序。
3. **未设置优先级**：视作最大优先级，排在所有设置者之后并按各自时间顺序；若全部未设置，则纯粹按订阅时间顺序。

### 7.3 派发流程

```
客户端 Emit(topic, payload):
  1. 构造事件负载 event = { type: topic, payload }
  2. 对每个插件：取出该插件 topic 订阅器列表，按优先级升序（同优先级按订阅时间）排序（见 §7.2）
  3. 对每个订阅器：经其沙箱事件循环同步调用 handler(event)
       - 返回 { cancel:true }      → 本事件标记为「已撤回」，停止派发
       - 返回 { intercept:true }   → 停止派发该插件后续订阅器（认为已被消费）
       - 返回 { modifiedData:X }   → 用 X 替换负载，供下游订阅器与后续插件读取
       - 返回 { return:X }         → 记录业务结果 X 供客户端消费（同主题取最后一个非 null 者），不影响负载链
       - 返回其他对象              → 放行，继续派发
  4. 汇总各订阅器结果（Outcome）与汇总标记（subscribed/errored/intercepted/canceled/returned）回执给客户端
```

**宿主 Go 接口**（客户端直接调用，不经 WebSocket 链路）：

```go
Emit(topic string, payload any, requestID string) EmitResult   // 发起事件，同步拿到各插件结果
Call(pluginID, fnName string, args []any) (any, error)         // 等价 engine.call
Agent(pluginID, naturalLang string) (any, error)               // 等价 engine.agent
Broadcast(payload any) / BroadcastTo(pluginID string, payload any) // 等价 engine.signal.all/target
```

---

## 8. 引擎层职责清单与文件布局

| 模块 | 文件 | 职责 |
|------|------|------|
| host 入口 | `host.go` | 包级入口 `Init/Close/Rescan` + 公开 Go 接口（Emit/Call/Agent/Broadcast）+ 宿主通道注入（SetOutbound/SetAgentInvoker/SetSendInvoker/SetWsServer） |
| 管理器 | `engine.go` | 插件扫描/加载/卸载/对账（reconcile），`pluginID → sandbox` 映射 |
| 沙箱 | `plugin.go` | 每插件独立 goja 事件循环：加载/卸载、同步回调执行（callFn 同步阻塞）、事件派发（intercept/modifiedData/return/cancel） |
| 绑定器 | `binder.go` | 按 `allow-*` 把 §5 全部 `engine.*` + 网络全局注入沙箱 |
| 权限 | `permission.go` | `permissions.key` 哈希解密校验（代码哈希即解密密钥）+ `allow-*` 能力开关 |
| 配置 | `yaml.go` | config.yaml 极简 YAML 解析/序列化（启动注入 + setFile 回写） |
| 能力实现 | `api.go` / `api_ext.go` | 广播、跨包调用、加解密、文件、记忆库、数据库、摘要/HMAC/Ed25519/JWT、图像、LLM、发送、WebSocket 服务端的 Go 侧实现 |
| 网络 | `api_net.go` / `api_network.go` / `api_ws.go` | `api_net.go`：同步 `engine.http` 与全局 `fetch`（共用 `doLTP9Fetch`）+ 全局 WebSocket **客户端**；`api_network.go`：`engine.network` 套接字（TCP/UDP/DNS）；`api_ws.go`：`engine.ws` WebSocket **服务端**（宿主注入 `WsBridge`） |
| 指令/异步/编解码/探针 | `api_command.go` / `api_async.go` / `api_encoding.go` / `api_probe.go` | `engine.command` 指令系统、`engine.async` 后台子任务、`engine.encoding` 编解码、`engine.network`/`probe` 前端可视化探针 |
| 类型/常量 | `type.go` / `variable.go` | 类型定义与常量/变量集中管理 |

---

## 9. 与既有组件复用对照

| LTP9 能力 | 复用来源 | 复用点 |
|------|------|------|
| goja 运行时 / 事件循环 | `goja_nodejs/eventloop` | 每插件 `eventloop.NewEventLoop()`（内部 `goja.New()`，自带定时器） |
| setTimeout / setInterval / clearTimeout 等 | `goja_nodejs/eventloop` | `NewEventLoop()` 构造时已注入，无需自实现；时间戳由引擎暴露 `engine.time` |
| 加解密 | `subsystem/lunar_decoder` | `EncodeFilesWithKeyString` / `DecodeFilesWithKeyString` |
| 记忆库 / 数据库 | 项目 FileManager/module、SQLite（mattn/go-sqlite3） | `engine.memory` / `engine.database`；`engine.emoji` 复用其 `stickers`（image 型）集合 |
| 同步 `engine.http` / 全局 `fetch` | `net/http` | `api_net.go` 共用 `doLTP9Fetch` |
| WebSocket 客户端 / 服务端 | `github.com/gorilla/websocket` | 客户端 `api_net.go`；服务端 `api_ws.go`（`engine.ws`，宿主注入 `WsBridge`） |
| 文本嵌入 / LLM | lunar_config.json 的 `agent.embedding_*` / `agent.multimodal_*` | `engine.llm.embed` / `engine.llm.chat` |
| 前端智能体调用 | Mini-LTP / Node-LTP | `engine.agent(包ID).run` 经宿主 `SetAgentInvoker` 路由到对应 WebAgent |
| 密钥生成 | `subsystem/ltp9_keygen` | 与引擎 `variable.go`（`AllowPermissionNames`）规则一致地生成 `permissions.key` |

---

## 10. 安全边界与约束

- **沙箱隔离**：插件各自运行于独立沙箱；插件间通过 `engine.signal` 广播或 `engine.call` 显式通信。
- **自包含单一脚本**：插件以打包后的单一 `execute.js` 运行。
- **路径沙箱化**：引擎对 `engine.file` 路径做沙箱化校验，限定插件数据目录内。
- **能力最小化**：权限经 `allow-*` 声明 + 代码哈希绑定授予；网络（`engine.http`/`fetch`/`WebSocket`）统一由 `allow-network` 门控。
- **同步回调线程安全**：一切 goja 操作（含事件派发、call 执行、网络回调）协调到对应沙箱事件循环（`RunOnLoop`）执行；`callFn` 同步阻塞等待回调结果。
- **拒绝响应**：`engine.call` / `engine.agent` 目标不可达或未导出 → 抛出 `xxx 包拒绝响应`。
- **解码失败即拒绝**：`permissions.key` 解码失败或权限名非法时拒绝对应权限，不退化到明文。

---

## 11. 目录与文件设计

```
crystal_astral/agent/StarLTP/
├── host.go            # 包级入口 Init/Close/Rescan + 公开 Go 接口 + 宿主通道注入
├── engine.go          # Engine：插件映射/对账/调度
├── plugin.go          # 单插件 goja 沙箱（生命周期、同步回调执行、事件派发）
├── binder.go          # engine.* / 网络全局绑定（按 allow-* 注入）
├── permission.go      # permissions.key 校验 + allow-* 开关
├── yaml.go            # config.yaml 极简 YAML 解析/序列化
├── api.go             # 广播 / 跨包调用 / 加解密 / 文件 / 记忆库 / 数据库 / 表情包
├── api_ext.go         # 扩展能力：JWT / LLM / 图像 / 发送 / WebSocket 服务端（Go 侧）
├── api_net.go         # 同步 engine.http + 全局 fetch（共用 doLTP9Fetch）+ WebSocket 客户端
├── api_network.go     # engine.network 套接字（TCP/UDP/DNS）
├── api_ws.go          # engine.ws WebSocket 服务端（宿主注入 WsBridge）
├── api_command.go     # engine.command 指令系统
├── api_async.go       # engine.async 后台子任务
├── api_encoding.go    # engine.encoding 编解码
├── api_probe.go       # 前端可视化测试探针
├── type.go            # 类型定义集中
├── variable.go        # 常量/变量集中
└── docs/              # engine-implementation.md + code_completion.d.ts（插件层类型声明）
```

---

## 12. 功能验证

验证入口（客户端侧）：
- **宿主 Go 接口**：`star.Init()` → `star.Emit(topic, payload, requestID)` / `star.Call(...)` / `star.PluginStates()`。
- **调试信封（WebSocket）**：`ltp9/event`、`ltp9/call`、`ltp9/stats`、`ltp9/broadcast`、`ltp9/probe`（见 `crystal_astral/ltp9_debug.go`）。
- **自动化脚本**：`crystal_astral/tools/test_ltp9_call.js`（经 WebSocket 调试信封验证 `ping` / `queryWeather` / `stats` / `probe`）。

覆盖点：
1. 单插件加载 + 订阅事件被客户端触发，收到并回执。
2. 事件订阅器支持拦截（intercept）、改写（modifiedData）、业务回传（return）、撤回（cancel），汇总回执含对应标记。
3. `engine.config` 正确注入 config.yaml；`setFile` 回写磁盘。
4. 未授权能力不注入（命名空间为 undefined）；`permissions.key` 授权与代码哈希绑定生效。
5. `engine.call` 指向不存在包返回 `xxx 包拒绝响应`。
6. `engine.http` 同步请求与 `engine.sleep` 阻塞行为正确；`fetch`/`WebSocket` 在 allow-network 下可用。
7. encoder/decoder 用 lunar_decoder 现成用例复核往返。
