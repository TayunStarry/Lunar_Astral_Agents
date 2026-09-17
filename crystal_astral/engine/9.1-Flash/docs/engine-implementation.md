# LTP9-Flash 引擎开发文档（Engine Implementation & Plugin Development Guide）

> 定位：在「星月智能」LTPX 协议家族中的引擎分支 **LTP9-Flash**。插件层 / 引擎层 / 客户端三端组织；每插件运行于**独立 goja 沙箱**，以**同步回调**与引擎互动；权限由**代码哈希加密的 `permissions.key`** 声明；内置跨包函数调用与 Mini-LTP / Node-LTP 前端智能体调度能力。
>
> **沙箱 API 契约**：`docs/LTP 9.1 Flash.d.ts` —— 能力以**顶层全局**（`signal` / `file` / `memory` / `http` / `hash` …）注入沙箱，不再使用 `engine.*` 命名空间。

---

## 1. 协议定位与设计目标

| 项 | LTP9-Flash |
|----|------|
| 载体 | CodeAgent（进程内 goja 兼容层/事件容器） |
| 语言规范 | **ES2023+ 语法**（可选链、空值合并、类、BigInt 等）；插件回调统一为**同步函数**，切勿使用异步操作（回调/导出支持 Promise 敲定等待，但 `database.transaction` 回调内禁异步） |
| 沙箱 | **每插件一个独立 goja Runtime**（`goja_nodejs/eventloop` 提供事件循环与定时器） |
| 组织架构 | 插件层 / 引擎层 / 客户端（三端） |
| 沙箱 API | **顶层全局**（见 §5），契约见 `docs/LTP 9.1 Flash.d.ts` |
| 权限系统 | `allow-*`（文件 / 数据库 / 记忆库 / 网络 / 调用 / agent / 广播 / 加解密） |
| 授权认证 | **`permissions.key`**：代码哈希作解密密钥，权限声明与代码强绑定 |
| 内置能力 | 同步 `http`/`fetch`、同步 `sleep`、WebSocket 全局、定时器、时间戳、事件总线、广播、跨包调用、前端智能体调用、lunar-decoder 加解密 |

**设计目标**：一个插件 = 一段（打包后的）js 运行在金属隔离的 goja 沙箱里，通过一组**顶层全局白名单 API** 与引擎互动；引擎负责权限校验、事件路由、跨端调度；客户端负责发起事件并**同步等待**脚本回调结果。

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
│  - 绑定器（把 LTP9-Flash 顶层全局按 allow-* 注入沙箱）           │
│  - 权限校验器（permissions.key + 能力开关）                      │
│  - 事件总线（topic → 订阅器列表，含拦截/改写/撤回调度）           │
│  - 跨端调度：callFunction / agent.synergy（Mini-LTP / Node-LTP） │
└──────────────────────────────┬─────────────────────────────────┘
                               │ LoadPlugin / route / broadcast
┌──────────────────────────── 插件层（Plugin）─────────────────────┐
│  LTP9 插件包（metadata.json + execute.js + config.yaml + ...）   │
│  在 execute.js 中直接调用顶层全局：event.* / signal.* /          │
│  http.* / file.* / database.* / memory.* / callFunction 等       │
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
├── data/              # 插件运行时数据目录（file.* 的作用域）
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

插件默认配置；引擎在沙箱启动时解析并注入 `config.read()`。插件可在运行时用 `config.write(obj)` 更新内存并同步写回磁盘 config.yaml。

### 3.3 permissions.key（授权认证）

`permissions.key` **既是密钥凭证、也是权限声明**，与代码文件 `execute.js` **强绑定**：

- **机制**：授权时，引擎取绑定代码文件的**哈希**作为密钥，把插件获许的权限清单**加密**写入 `permissions.key`。
- **加载**：引擎对当前代码文件重新计算哈希，用该哈希解密 `permissions.key`，解出权限清单后加载插件。
- **效果**：授权与「原版未改动的代码文件」绑定；未解出对应权限则拒绝该能力。**解码失败即拒绝**，不退化到明文。

### 3.4 配套密钥生成模块（LTP9 内置）

LTP9 使用**配套的密钥生成模块** `subsystem/ltp9_keygen` 生成 `permissions.key`：

1. **输入**：打包后的 `execute.js` + 开发者勾选的 `allow-*` 权限清单。
2. **哈希密钥**：`sha256(execute.js)[:16]` hex（与引擎 `codeHash` 规则一致）。
3. **权限清单编码**：整体加密写入，每项 `allow-*` 权限名填充到定长后以 `+` 连接。
4. **一致性**：生成器内置校验入口，供提交前确认 `permissions.key` 能被引擎规则解出预期权限。
5. **改代码必须重签**：execute.js 变更后哈希变化，需用 keygen 重新签发（或开发模式跳过校验）。

---

## 4. 沙箱模型（每插件独立 goja + 同步回调）

### 4.1 隔离原则

- **每插件一个独立 `goja.Runtime`**：插件 A 的全局变量、定时器、网络连接、注册表各自独立。
- **插件自包含为单一 `execute.js`**：多脚本经打包器拼接为单个自包含文件。
- **沙箱运行时**：每插件以 `goja_nodejs/eventloop.NewEventLoop()` 各建一个独立 loop（自带定时器），网络能力由绑定器注入（同步 `http` 与全局 `fetch`/`WebSocket`）。

### 4.2 引擎侧沙箱生命周期

```
load(pluginID):
  1. 解析 config.yaml 注入 config.read()（失败仅告警，不阻断加载）
  2. 以代码文件哈希解密 `permissions.key`，解出权限声明（开发模式授予全部 allow-*）
  3. 新建该插件独立 goja 事件循环
  4. bindSandbox：按权限清单注入 LTP9-Flash 顶层全局（allow-network → http / network / fetch / WebSocket）
  5. 执行 execute.js（注册订阅器、导出函数）
  6. 注册完成后该插件开始接收事件

unload(pluginID): 停止事件循环（带存活探针与宽限）→ 清空订阅器/导出/配置
```

> LTP9-Flash 契约不含 `onLoad` / `onUnload` / `onConfigUpdate` 生命周期钩子；卸载清理由引擎内部完成。

### 4.3 语言能力（同步回调模型）

- **沙箱全局**：`console`（log/info/warn/error/debug，输出走引擎日志）与定时器（`setTimeout`/`setInterval`/`setImmediate` 及对应 clear，由 eventloop 注入）。
- **同步为主、异步可用**：事件订阅回调、`exportFunction` 导出的函数优先写成**同步函数**返回普通对象；需要网络时用阻塞式 `http.get/post` 或同步 `fetch`，需要等待时用 `sleep(ms)`。
- `WebSocket` 全局以事件回调形态存在（`allow-network` 门控）。若回调/导出函数返回 pending Promise（`async` 函数），`callFn` 会让出插件事件循环并轮询直到 Promise 兑现或超时（默认 90s）；`database.transaction` 回调内返回 Promise 会立即回滚并报错。

### 4.4 定时器与时间戳（来自 goja_nodejs/eventloop）

**定时器随 `eventloop.NewEventLoop()` 自动注入**；时间戳由 `time.now()/nowMs()` 暴露：

```js
time.now()            // 秒级时间戳
time.nowMs()          // 毫秒时间戳
const id = setTimeout(fn, ms)
clearTimeout(id)
```

### 4.5 网络模块（全同步形态）

```js
// 同步形态（插件主链路推荐，allow-network）
const r = http.get(url, headers?)     // 阻塞直到返回 → { success, status, body, error? }
const r = http.post(url, body, headers?)
const d = http.download(url, savePath?) // { success, path, size } / { success:false, error }

// 同步 fetch（同一 allow-network 门控，body 为 JSON 时自动解析为对象）
const r = fetch(url, {method, headers, body})  // → { success, status, ok, url, headers, body, error? }

// WebSocket 事件回调形态（连接型场景）
const ws = new WebSocket("wss://...")   // onopen / onmessage({data: string | ArrayBuffer}) / onerror / onclose
```

- `http`、`fetch` 共用同一套请求实现（`api_net.go` 的 `doLTP9Fetch`），全部同步阻塞；`WebSocket` 事件回调排到该插件自己的 `eventloop` 线程。
- 裸 TCP/UDP/DNS 见 `network`（§5.13）。

---

## 5. 运行时 API（LTP9-Flash 顶层全局）

以下全局由绑定器按 `allow-*` 注入每个沙箱。**未获权限的全局不注入**，脚本调用即触发 ReferenceError；获得权限的调用被拒时返回带 `error` 的结果对象。

> **错误处理约定（统一 Result 风格）**：一切可失败操作返回 `{ success: boolean, error?: string, ...数据 }`（失败数据字段省略），由插件检查 `success`/`error`；仅参数用法错误（如 `fetch` 缺 url）抛 TypeError。失败不中断插件，由插件自行处置。

### 5.1 事件订阅器 `event`（常驻）

```js
let id = event.subscribe("weather.query", (event) => {
  // event = { type: topic, payload: 触发负载 }
  return { handled: true }            // 放行（返回任意业务结果对象）
  // { intercept:true }  → 短路该插件后续订阅器
  // { modifiedData:X }  → 改写负载，供下游订阅器与客户端读取
  // { return:X }        → 业务结果回执给客户端（不参与负载链）
  // { cancel:true }     → 撤回事件，停止派发（含其他插件）
}, 0)  // 可选优先级：仅支持非负整数，0 最高；未设置则按时间顺序
event.unsubscribe("weather.query", id)

event.publish("weather.query", { city: "北京" })  // 主动发布：派发给其它插件订阅器 + 转发宿主 outbound（fire-and-forget，跳过自身）
```

**派发顺序**：优先级升序（0 最高）→ 同优先级按订阅时间 → 未设置优先级者排最后（按各自时间顺序）。

### 5.2 广播 `signal`（allow-signal）

```js
signal.all(payload)            // 向所有插件广播
signal.target(pkgId, payload)  // 仅向目标插件广播
const id = signal.subscribe((data) => { /* 接收广播 */ })
signal.unsubscribe(id)
```

广播方向为插件间 / 引擎→插件单向通报；宿主注入 `SetOutbound` 后，广播同时转发给外部 Go 程序消费。

### 5.3 数据库 `database`（allow-database）

```js
database.query(sql, params?)   // 读 → { success, rows: [{列名:值}...] }
database.exec(sql, params?)    // 写 → { success, rows_affected }（失败时 rows_affected 为 0）

// 同步事务：回调接收 { query, exec }（绑定到事务）；正常返回提交，抛错/返回 Promise 回滚
const r = database.transaction((tx) => {
  tx.exec("INSERT INTO t(name) VALUES (?)", ["a"])
  return tx.query("SELECT * FROM t")
})  // → { success, result } / { success:false, error }

// 按名称迁移：未应用则在事务内执行 upSql（支持多语句，禁含 BEGIN/COMMIT）并记录到 _migrations
database.migrate("init-t", "CREATE TABLE IF NOT EXISTS t (id INTEGER PRIMARY KEY, name TEXT)")
// → { success, applied }（applied=false 表示此前已应用，本次跳过）

// 命名空间作用域：独立 <name>.db 文件（名称仅允许字母数字-_），真隔离
const ns = database.namespace("myplugin")   // { query, exec, transaction, migrate }
ns.query("SELECT * FROM t")
```

接入项目 SQLite（mattn/go-sqlite3）。共享库为 `local_data/database/knowledge.db`（WAL 模式）；
命名空间库为 `local_data/database/<name>.db`（每库自动建 `_migrations` 迁移记录表）。

### 5.4 向量记忆库 `memory`（allow-memory）

```js
memory.store({ content, tags? })        // 写入 → { success, id }
memory.search({ query, limit? })        // 检索 → { success, results: [{ content, similarity }] }（limit 默认 5，上限 50）
memory.searchImage(query, { limit? })   // 图片语义检索 → { success, results: [{ image, similarity }] }
memory.storeImage(base64)               // 添加图片（标签由记忆库 LLM 自动生成）→ { success, id }
memory.randomImage(query?)              // 按语义随机返回一张 → { success, image }
```

- 文本集合 `ltp9_memory` 惰性初始化；图片复用项目记忆库 `stickers`（image 型）集合，为集合级管理，不提供按 id 单条读删。

### 5.5 文件 `file`（allow-file）

```js
file.write(path, data)   // → { success } / { success:false, error }
file.read(path)          // → { success, text } / { success:false, error }
file.delete(path)        // → { success } / { success:false, error }
```

路径作用于插件数据目录（`<包目录>/data/`），引擎对 path 做沙箱化校验（绝对路径/越界拒绝）。

### 5.6 配置 `config`（常驻）

```js
config.read()          // → { success, config? }；config 为 config.yaml 注入对象，无配置时省略
config.write(obj)      // 更新内存并同步写回 config.yaml
```

### 5.7 跨包函数调用 `callFunction`（allow-call）

```js
const r = callFunction(pkgId, fnName, [args])   // 同步阻塞 → { success:true, result } / { success:false, error }
if (r.success) use(r.result)
```

- 目标插件必须在其 execute.js 里 `exportFunction(fnName, (...args) => ...)`。
- 目标插件不存在/未导出该函数 → `{ success:false, error: "xxx 包拒绝响应…" }`（不抛异常）。

### 5.8 调用前端智能体 `agent`（allow-agent）

```js
agent.chat(messages, opts?)   // OpenAI 兼容对话 → { success, text, tool_calls?, error? }
                              // messages: PostMessage / ToolMessage（role='tool' 含 tool_call_id）
                              // opts: { temperature?, max_tokens?, system?, tools?: ChatTool[] }
agent.embed(input, opts?)     // 文本嵌入（单条 string 或数组）→ { success, embedding } / { success, embeddings }
agent.synergy(pkgId, text)    // 调用前端智能体（Mini-LTP / Node-LTP）→ { success, text } / { success:false, error }
agent.search(instruction)     // Web-LTP 网络搜索（allow-agent 绑定 + allow-network 执行）→ { success, text: 报告 }
```

- `agent.synergy` 目标包不存在或智能体不存在 → `{ success:false, error: "xxx 包拒绝响应" }`（不抛异常）。
- `agent.search` 同 `agent.synergy` 一致的双权限模型：未授予 `allow-network` 时返回 `{ success:false, error }`（智能体本质多需联网，详见 §5.17）。
- LLM 配置读取 `lunar_config.json` 的 `agent.multimodal_*` / `agent.embedding_*`（不硬编码）。
- `agent.chat` 传 `tools`（`ChatTool[]`，OpenAI 兼容函数定义）时模型走函数调用：content 为空，`tool_calls`（`ToolCall[]`）原样透传；执行结果以 `ToolMessage`（role='tool' + tool_call_id）回传继续对话。

### 5.9 编解码 `encoding`（allow-certificate）

```js
encoding.base64Encode(data)          // → base64 字符串
encoding.base64Decode(s)             // → { success, text } / { success:false, error }（不抛异常）
encoding.urlEncode(s)                // → URL 编码字符串
encoding.urlDecode(s)                // → { success, text } / { success:false, error }（不抛异常）
encoding.lunarEncoder(key, content)  // 加密 → { success, text: 密文, error? }（桥 lunar_decoder，与 permissions.key 同源）
encoding.lunarDecoder(key, cipher)   // 解密 → { success, text: 原文, error? }
```

### 5.10 哈希签名 `hash`（allow-certificate）

```js
hash.signJWT(claims, secret, algorithm?, kid?) // HS256（默认）/ EdDSA(Ed25519) / none；kid 可选写入 JWT 头
                                               // → { success, text: JWT } / { success:false, error }（不抛异常）
hash.md5(s) / sha1(s) / sha256(s)              // 小写十六进制摘要
hash.hmacSha1(key, s) / hmacSha256(key, s)     // 小写十六进制 MAC
hash.ed25519Sign(privKeyPemOrSeed, data)       // → { success, text: base64url 签名, error? }（不抛异常）
hash.generateJWT(claims, privKeyPemOrSeed, kid?) // EdDSA JWT → { success, text, error? }
```

### 5.11 图像 `image`（allow-file；download 另需 allow-network）

```js
image.loadValid(path)            // 读插件数据目录图片并校验 → { success, text: base64 }
image.download(url, fileName)    // 下载到数据目录并校验 → { success, text: base64 }；无 allow-network 返回 failure
// 缩放 + 重编码：path 可为数据目录相对路径或 data:..;base64 内联数据
image.resize(path, { max_dim? | width?, height?, format?='jpeg/png/webp', quality?=90 })  // → { success, text: dataURI, width, height, format }
// 仅重编码（编码格式化，不改尺寸）
image.convert(path, format, { quality?=90 })                                             // → { success, text: dataURI, width, height, format }
```

### 5.12 视频抽帧 `video`（allow-file；URL/dataURI 源另需 allow-network）

```js
video.frames(source, { times? | count? | fps?=5, dedup?=true, max_dim?=640, format?='jpeg', quality?=85 })
// source：相对数据目录路径 / http(s) URL / data:video;base64 URI
// → { success, frames:[{ data:dataURI, timestamp, width, height, format, index }], count, skipped? }
// 采样互斥优先级：times(显式秒) > count(等距帧数) > fps(均匀频率，超60帧自动等距抽样)
```

### 5.13 裸套接字 `network`（allow-network）

```js
network.resolveDNS(hostname, timeoutSec?)                     // → { success, addresses }
network.resolveSRV(service, proto, hostname, timeoutSec?)     // → { success, targets: [{target, port}] }
network.tcpConnect(host, port, timeoutSec?)                   // → 套接字对象
network.udpConnect(host, port, timeoutSec?)                   // → 已连接 UDP 套接字
network.udpListen(host?, port?)                               // → 监听套接字
// 套接字对象：send(data) / receive(timeoutSec?) / sendTo(host, port, data)（未连接 UDP） / close()
```

### 5.14 指令系统 `command`（常驻）

```js
command.register(name, pattern, (match, context) => result, { aliases?: string[] })
// pattern 可带 /.../ 分隔符；留空时仅按指令名/别名精确匹配
```

引擎侧经 Go 接口 `Command(pluginID, text, context)` / `CommandAll(text, context)` 触发。

### 5.15 异步子任务 `async`（常驻）

```js
async.run(taskFn, { timeout?, data? })  // taskFn 收到 ({ id, data })；→ { success, text: taskId }
async.reportProgress(taskId, progress)
async.getStatus(taskId)                 // → { success, taskId, status, progress }
async.list()                            // → { success, tasks: [{ taskId, status, progress }] }
```

任务仍在插件事件循环线程内执行（复用 callFn）；超时看门狗仅标记状态。

### 5.16 导出插件能力 `exportFunction`（常驻）

```js
exportFunction("myFunction", (a, b) => { return a + b })   // 同步函数，供 callFunction 调用
```

### 5.17 Web-LTP 网络搜索 `agent.search`（双权限：allow-agent 绑定 + allow-network 执行）

```js
agent.search(instruction)   // 自然语言指令 → 检索 → 多页面摘要 → 报告 → { success, text: 报告, error? }
// instruction 可含「前N页/个」等数量要求，底层复用 Web-LTP 进程内搜索流水线
// 无 allow-network 时返回 { success:false, error }
```

### 5.18 时间与休眠（常驻）

```js
time.now() / time.nowMs()
sleep(ms)   // 同步阻塞等待
```

---

## 6. 权限系统（`allow-*`）

LTP9-Flash 用**权限密钥 + 沙箱注入控制**双层实现。引擎经哈希解密 `permissions.key` 解出该插件获许的 `allow-*` 清单，据此注入对应顶层全局，未列入清单的全局不注入。

| 权限 | 控制的全局 | 说明 |
|------|-----------|------|
| `allow-file` | `file.*` / `image.loadValid/resize/convert` / `video.frames` | 文件读写删 + 图片读/校验/缩放/转码 + 视频抽帧 |
| `allow-database` | `database.*` | SQL 数据库读写 |
| `allow-memory` | `memory.*` | 向量记忆库与图片记忆读写 |
| `allow-network` | `http.*` / `network.*` / `fetch` / `WebSocket` / `image.download` / `video.frames(URL源)` / `agent.search(执行)` | 同步/异步网络、裸套接字、图片下载、视频URL下载与 Web-LTP 搜索 |
| `allow-call` | `callFunction` | 调用其他插件导出函数 |
| `allow-agent` | `agent.*` | LLM 对话 / 文本嵌入 / 前端智能体 / Web-LTP 搜索 |
| `allow-signal` | `signal.*` | 广播收发 |
| `allow-certificate` | `encoding.*` / `hash.*` | 编解码 + lunar 加解密 + JWT/摘要/HMAC/Ed25519 |
| `allow-send` | （预留） | 保留权限名，Flash 当前未挂载能力 |
| `allow-socket` | （预留） | 保留权限名，Flash 当前未挂载能力 |

> 常驻注入（不参与 `allow-*` 开关）：`event` / `signal.subscribe 之外的 event 方法` / `config` / `time` / `sleep` / `exportFunction` / `command` / `async` / `console` / 定时器。
> 开发模式（`GeneralConfig.Developer`）跳过密钥校验并默认授予全部 `allow-*`。

---

## 7. 事件订阅器模型（引擎层核心）

### 7.1 数据结构

```
订阅器注册表（引擎级，按插件存储）：
  topic ──► []eventSub {
    pluginID,       // 归属插件（用于卸载清理）
    orderID,        // 订阅 id（插件内单调递增，同时作为时间顺序依据）
    priority,       // 订阅优先级（0 最高；priorityUnset 排在所有设置者之后）
    handler,        // goja FunctionValue（在对应沙箱同步执行）
  }
```

### 7.2 派发流程

```
客户端 Emit(topic, payload):
  1. 构造事件负载 event = { type: topic, payload }
  2. 对每个插件：取出该插件 topic 订阅器列表，按优先级升序（同优先级按订阅时间）排序
  3. 对每个订阅器：经其沙箱事件循环同步调用 handler(event)
       - { cancel:true }      → 本事件标记为「已撤回」，停止派发（含后续插件）
       - { intercept:true }   → 停止派发该插件后续订阅器
       - { modifiedData:X }   → 用 X 替换负载，供下游订阅器与后续插件读取
       - { return:X }         → 记录业务结果 X 供客户端消费（同主题取最后一个非 null 者）
       - 其他对象             → 放行，继续派发
  4. 汇总各订阅器结果（Outcome）与汇总标记（subscribed/errored/intercepted/canceled/returned）回执给客户端
```

**宿主 Go 接口**（客户端直接调用，不经 WebSocket 链路）：

```go
Emit(topic string, payload any, requestID string) EmitResult   // 发起事件，同步拿到各插件结果
Call(pluginID, fnName string, args []any) (any, error)         // 等价 callFunction
Agent(pluginID, naturalLang string) (any, error)               // 等价 agent.synergy
Broadcast(payload any) / BroadcastTo(pluginID string, payload any) // 等价 signal.all/target
Command(pluginID, text string, context) / CommandAll(text, context) // 触发指令
```

---

## 8. 引擎层职责清单与文件布局

| 模块 | 文件 | 职责 |
|------|------|------|
| host 入口 | `host.go` | 包级入口 `Init/Close/Rescan/Version` + 公开 Go 接口（Emit/Call/Agent/Broadcast/Command）+ 宿主通道注入（SetOutbound/SetAgentInvoker） |
| 管理器 | `engine.go` | 插件扫描/加载/卸载/对账（reconcile），`pluginID → sandbox` 映射 |
| 沙箱 | `plugin.go` | 每插件独立 goja 事件循环：加载/卸载、同步回调执行（callFn 同步阻塞）、事件派发（intercept/modifiedData/return/cancel） |
| 绑定器 | `binder.go` | 按 `allow-*` 把 LTP9-Flash 顶层全局 + 网络全局注入沙箱 |
| 权限 | `permission.go` | `permissions.key` 哈希解密校验（代码哈希即解密密钥）+ `allow-*` 能力开关 |
| 配置 | `yaml.go` | config.yaml 极简 YAML 解析/序列化（启动注入 + config.write 回写） |
| 能力实现 | `api.go` / `api_ext.go` | 广播、事件发布、跨包调用、文件、记忆库（文本+图片）、图像、LLM chat/embed 的 Go 侧实现 |
| 数据库 | `api_database.go` | `database` query/exec/transaction/migrate/namespace（共享 knowledge.db + 命名空间独立库，句柄缓存） |
| 网络 | `api_net.go` / `api_network.go` / `api_ws.go` | `api_net.go`：同步 `http` 与同步 `fetch`（共用 `doLTP9Fetch`）；`api_ws.go`：全局 WebSocket **客户端**（文本帧 string / 二进制帧 ArrayBuffer）；`api_network.go`：`network` 套接字（TCP/UDP/DNS） |
| 指令/异步/编解码 | `api_command.go` / `api_async.go` / `api_encoding.go` | `command` 指令系统、`async` 后台子任务、`encoding` 编解码与 lunar 加解密桥 |
| 探针 | `api_probe.go` | 前端可视化测试探针（ltp9/test 信封，复用各能力同源实现） |
| 类型/常量 | `type.go` / `variable.go` | 类型定义与常量/变量集中管理 |

---

## 9. 与既有组件复用对照

| LTP9 能力 | 复用来源 | 复用点 |
|------|------|------|
| goja 运行时 / 事件循环 | `goja_nodejs/eventloop` | 每插件 `eventloop.NewEventLoop()`（自带定时器） |
| 加解密 | `subsystem/lunar_decoder` | `encoding.lunarEncoder/lunarDecoder` 与 permissions.key 同源 |
| 记忆库 / 数据库 | 项目 FileManager/module、SQLite（mattn/go-sqlite3） | `memory.*` / `database.*`（`api_database.go`：共享库 + 命名空间独立库）；图片记忆复用 `stickers` 集合 |
| 同步 `http` / `fetch` | `net/http` | `api_net.go` 共用 `doLTP9Fetch`（参照 lunar_astral engine.syncFetch 的同步方案） |
| WebSocket 客户端 | `github.com/gorilla/websocket` | `api_ws.go` |
| LLM / 嵌入 | lunar_config.json 的 `agent.multimodal_*` / `agent.embedding_*` | `agent.chat` / `agent.embed` |
| 前端智能体调用 | Mini-LTP / Node-LTP | `agent.synergy` 经宿主 `SetAgentInvoker` 路由 |
| 密钥生成 | `subsystem/ltp9_keygen` | 与引擎 `variable.go`（`AllowPermissionNames`）规则一致地生成 `permissions.key` |

---

## 10. 安全边界与约束

- **沙箱隔离**：插件各自运行于独立沙箱；插件间通过 `signal` 广播或 `callFunction` 显式通信。
- **自包含单一脚本**：插件以打包后的单一 `execute.js` 运行。
- **路径沙箱化**：引擎对 `file` 路径做沙箱化校验，限定插件数据目录内。
- **能力最小化**：权限经 `allow-*` 声明 + 代码哈希绑定授予；网络（`http`/`fetch`/`WebSocket`/`network`）统一由 `allow-network` 门控。
- **同步回调线程安全**：一切 goja 操作协调到对应沙箱事件循环（`RunOnLoop`）执行；`callFn` 同步阻塞等待回调结果。
- **拒绝响应**：`callFunction` / `agent.synergy` 目标不可达或未导出 → `{ success:false, error: "xxx 包拒绝响应" }`（Result 风格，不抛异常）。
- **解码失败即拒绝**：`permissions.key` 解码失败或权限名非法时拒绝对应权限，不退化到明文。

---

## 11. 目录与文件设计

```
crystal_astral/engine/9.1-Flash/
├── host.go            # 包级入口 + 公开 Go 接口 + 宿主通道注入
├── engine.go          # Engine：插件映射/对账/调度
├── plugin.go          # 单插件 goja 沙箱（生命周期、同步回调执行、事件派发）
├── binder.go          # LTP9-Flash 顶层全局绑定（按 allow-* 注入）
├── bridge.go          # 宿主接线入口 Bridge（引擎初始化 + 通道注入）
├── debug.go           # LTP9-Flash 调试信封（HandleInbound + ltp9/test 动作分发）
├── permission.go      # permissions.key 校验 + allow-* 开关
├── yaml.go            # config.yaml 极简 YAML 解析/序列化
├── api.go             # 广播 / 事件发布 / 跨包调用 / 文件 / 记忆库
├── api_database.go    # database：query/exec/transaction/migrate/namespace
├── api_ext.go         # JWT / 摘要 / Ed25519 / 图像 / LLM（Go 侧）
├── api_net.go         # 同步 http + 同步 fetch（共用 doLTP9Fetch）
├── api_ws.go          # 全局 WebSocket 客户端
├── api_network.go     # network 套接字（TCP/UDP/DNS）
├── api_command.go     # command 指令系统
├── api_async.go       # async 后台子任务
├── api_encoding.go    # encoding 编解码 + lunar 加解密桥
├── api_probe.go       # 前端可视化测试探针
├── type.go            # 类型定义集中
├── variable.go        # 常量/变量集中
└── docs/              # LTP 9.1 Flash.d.ts（插件层契约）+ engine-implementation.md
```

---

## 12. 功能验证

验证入口（客户端侧）：
- **宿主 Go 接口**：`star.Init()` → `star.Emit(topic, payload, requestID)` / `star.Call(...)` / `star.PluginStates()`。
- **调试信封（WebSocket）**：`ltp9/event`、`ltp9/call`、`ltp9/stats`、`ltp9/broadcast`、`ltp9/probe`（见 `crystal_astral/ltp9_debug.go`）。

覆盖点：
1. 单插件加载 + 订阅事件被客户端触发，收到并回执。
2. 事件订阅器支持拦截（intercept）、改写（modifiedData）、业务回传（return）、撤回（cancel），汇总回执含对应标记。
3. `config.read()` 正确注入 config.yaml；`config.write` 回写磁盘。
4. 未授权全局不注入（脚本调用即 ReferenceError）；`permissions.key` 授权与代码哈希绑定生效。
5. `callFunction` 指向不存在包 → `{ success:false, error }`（Result 风格）。
6. `http`/`fetch` 同步请求与 `sleep` 阻塞行为正确；`WebSocket` 文本/二进制帧回调在 allow-network 下可用。
7. `encoding.lunarEncoder/lunarDecoder` 与 `hash.signJWT` 往返复核（失败路径返回 Result，不抛异常）。
8. `database.transaction` 提交/回滚（含回调抛错回滚）、`migrate` 幂等（二次调用 applied=false）、`namespace` 独立库隔离。
9. 插件脚本改动后：用 `subsystem/ltp9_keygen` 重新签发 `permissions.key`（或开发模式验证）。
