# LTP3 引擎层实现文档

> 本文档描述 LTP3（YaraFlow）引擎层在 **crystal_astral** 内的完整实现：包结构、逐插件 goja 沙箱、加载/卸载与对账机制、事件/钩子分发、以及与真实客户端之间的 WS 总线。实现位于 `d:\Lunar_Astral_Agents\crystal_astral\agent\YaraLTP\`，与 LTPX/AtoA 完全解耦。

---

## 一、定位与边界

- **引擎层** = crystal_astral（琉璃）内的 `agent/YaraLTP` 包。负责扫描 `local_data/package/` 下的 LTP3 包、为每个包启动**独立 goja 沙箱**、接收来自客户端的 `ltp3/*` 信封并分发给插件订阅器。
- **插件层** = `local_data/package/<包>/`，`metadata.json` 含 `LTP3` 标签即被识别；插件逻辑是 `index.js`，唯一配置文件是 `config.yaml`。
- **客户端层** = 真实消费方（消息桥/月华等），通过 `/ws` 用 `ltp3/*` 信封调用引擎、接收回执与插件主动消息。
- **与 LTPX 的不同**：LTP3 协议的 `metadata.json` **不含 `tools` 字段**（那是 AtoA/LTPX 的能力）、不走 AtoA、不注入页面智能体；`plugin.json` 与除 `config.yaml` 外的配置格式一律忽略。

---

## 二、目录与职责

```
crystal_astral/agent/YaraLTP/
├── variable.go      # 常量（ServiceName=“LTP3”、hook/event 常量表、引擎全局、模型配置缓存、包根目录）
├── type.go          # 类型集中区：Plugin、engine、订阅/指令/工具/API 定义、WS 信封、分发结果
├── yaml.go          # config.yaml 纯 Go YAML 子集解析/序列化（无第三方依赖）
├── plugin.go        # 单插件 goja 沙箱：绑定 yara、执行 index.js、调用生命周期、卸载释放
├── binder.go        # 组装 yara 全局对象并调用各 bind* 注入
├── api_core.go      # logger / event / hook / eventHandler / command / tool / api / llmProvider / send
├── api_net.go       # http / network（TCP/UDP/DNS）/ platform（含 SSRF 防护）
├── api_data.go      # encoding / time / crypto
├── api_model.go     # model（复用 OpenAI v1，配置读 lunar_config.json）
├── api_res.go       # config / file / database / knowledge / image（复用 SQLite 知识库与向量记忆库）
├── api_misc.go      # async / emoji
├── model.go         # OpenAI v1 对话/嵌入客户端
├── manager.go       # 引擎管理器：扫描/装载/卸载/对账环、hook 与事件分发、跨插件 API 调用
├── bus.go           # WS 总线：ltp3/* 信封收发与路由、manage 动作
└── host.go          # 包级入口 Init/Close/Run（对 crystal_astral 暴露的最小面）
```

对应外部接线（crystal_astral）：
- `create.go` —— 注入出站发送函数、调用 `YaraLTP.Init()`、消费入站信封；
- `ws.go` / `type.go` —— `StudioHub` 新增 `Inbound` 通道，把收到的 ws 消息非阻塞送入引擎；
- `ltpx_remote.go` —— 保留内置 `yara_ltp` 工具，调用 `YaraLTP.Run` 把文本路由到默认钩子点。

---

## 三、逐插件独立沙箱

每个插件持有一个独立的 `*goja.Runtime`（`plugin.go`）：

```go
type plugin struct {
    ID, DirName, Title string
    Root, MainPath, ConfigPath, DataDir string
    config map[string]any
    vm *goja.Runtime
    mu  sync.Mutex        // 串行化同一插件所有 JS 执行（goja 非线程安全）
    hooks, events, commands, tools, apis, llmProviders ...
    currentRequestID string // 当前分发上下文的 request_id（供 send 单播回执）
}
```

- **加载流程**：读取 `config.yaml`（失败仅告警）→ `goja.New()` → `bindYara(p)` 注入 `yara.*` 与 `YaraEvents`/`YaraHooks` → 执行 `index.js` → 捕获 `onLoad/onUnload/onConfigUpdate`（用 `vm.Get` + `isJSFunc`，避免 `goja.Value(Callable)` 非法转换）→ 调用 `onLoad()` → 广播 `ltp3/lifecycle ON_START`。
- **串行锁**：所有进入插件 JS 的入口（hook/event/command/工具/api）都先 `p.mu.Lock()`。同一插件内 JS 互相调用（如 `event.publish`、`api.call` 本插件）**不再加锁**（由调用方已持锁保证），避免自锁死锁。
- **跨插件 API**：`yara.api.call("插件ID.方法名", params)` 由 `manager.callCrossPlugin` 解析分段名，锁定并调用目标插件公开 API。
- **async**：`yara.async.run` 在后台上 goroutine，仍经该插件互斥锁串行执行，因此不会与其它 JS 并发触碰 VM。

> 说明：当前未启用 30s 看门狗硬打断（协议文档定义）；`setTimeout` 等天然被沙箱禁用（无事件循环）。

---

## 四、加载 / 卸载 / 对账（manager.go）

```go
func (e *engine) LoadAll()   // 启动时：扫描包根目录并加载全部 LTP3 包
func (e *engine) reconcile() // 对账：新增→loadPackage；磁盘已无（含摘 LTP3 标签）→unloadPackage
func (e *engine) fingerprint() string // 目录名集合 + 已加载集合的签名，用于探测变化
func (e *engine) startReconcile()     // 每 3s tick，指纹变化才执行 reconcile
```

- 识别：`readMeta(root)` 读取 `metadata.json`，`tags` 含 `LTP3` 且 `id` 非空即视为插件。
- 装载维度：以包 `id` 为 key，`byDir` 维护目录名→id。
- 卸载：`p.unload()` 调用 `onUnload`、广播 `ltp3/lifecycle ON_STOP`、清空注册表并置 `vm=nil`。
- 前端在后端新增/删除包 → 对账环自动驱动对应 VM 的加载/卸载（无需重启引擎）。
- 同时支持通过 `ltp3/manage`（`scan/reload/reload_one/unload_one`）手动触发。

---

## 五、事件 / 钩子分发语义

### 5.1 钩子（Hook）

```
客户端 ltp3/hook( hook, payload, context, request_id )
   ↓ manager.DispatchHook(hook, payload, ctx, reqID)
   对每个订阅该 hook 的插件：
      p.mu.Lock()
      p.currentRequestID = reqID
      p.runHook(hookType, message, ctx)   // 逐个调用回调，事件对象 {type,message,context}
      p.currentRequestID = 旧值
      p.mu.Unlock()
   聚合结果 → ltp3/hook_result{ results[], summary }
```

- `message` = 客户端 `payload`（原样透传）；`ctx` = `context`。
- `summary`：`subscribed` / `errored` / `allow_continue` / `aborted`（由各回调返回的 `allowContinue` / `action:"abort"` 聚合，默认 allowContinue=true）。
- 分发期间设置 `plugin.currentRequestID`，使插件内的 `yara.send.*` 能携带本次 `request_id` → **单播回执**。

### 5.2 事件（Event）

```
客户端 ltp3/event( event, payload, request_id )
   ↓ manager.PublishEvent(topic, payload)
   对每个订阅该 topic 的插件：fireEventLocal 逐个调用订阅回调
   → ltp3/event_ack{ subscribed }
```

- `event.subscribe` 与 `eventHandler.register` 都落入 `plugin.events[topic]`。

### 5.3 指令（Command）

`DispatchCommand`：先按 `command` 精确匹配各插件注册表；未命中回退为用每条指令正则匹配整段文本（`dispatchCommandByRegex`）。→ `ltp3/command_result`。

---

## 六、WS 总线（bus.go + 集线器改造）

### 6.1 入站

`StudioHub` 新增 `Inbound chan []byte`（`type.go`），读协程在 `Broadcast` 后**非阻塞**推入（`ws.go`）：

```go
h.Broadcast <- message
select { case h.Inbound <- message: default: }
```

create.go 启动消费者把入站逐条交给引擎：

```go
go func() { for data := range StudioHubInstance.Inbound { YaraLTP.HandleIn(data) } }()
```

`HandleIn` 解析 `InMessage`，仅处理 `type` 前缀为 `ltp3/` 的消息（自由旁路转发给其它订阅者）。

### 6.2 出站

create.go 注入发送函数：

```go
YaraLTP.SetSend(func(data []byte){ StudioHubInstance.Broadcast <- data })
```

`emitBus(v)` 把结构体 `json.Marshal` 后广播；`send_message` 等出站信封均走该通路。

### 6.3 信封类型清单

| In | Out |
|---|---|
| `ltp3/hook` | `ltp3/hook_result` |
| `ltp3/event` | `ltp3/event_ack` |
| `ltp3/command` | `ltp3/command_result` |
| `ltp3/manage` | `ltp3/manage_ack` |
| `ltp3/ping` | `ltp3/pong` |
| — | `ltp3/send`（插件主动发消息） |
| — | `ltp3/lifecycle`（ON_START/ON_STOP） |
| — | `ltp3/error` |

`send` 送达策略：`request_id` 存在 → 单播回触发客户端；缺失 → 默认广播。

---

## 七、复用现有基建

| yara API | 落地后端 |
|---|---|
| `model.*` | OpenAI v1 chat/embeddings，配置读 `lunar_config.json` 的 agent 组（`model.go`） |
| `database.*` | 复用 `FileManager/module` 的记忆库（`MemoryGetDocuments` / `MemoryQueryMessagesWithContent`） |
| `knowledge.*` | 同上，走向量语义检索 |
| `file.*` / `config.*` | 插件目录 / `data/` 目录，`config.yaml` 用自研 YAML 解析器 |
| `http.*` / `network.*` | Go 标准库 net/http、net（含 SSRF 防护） |
| `crypto.*` | crypto/ed25519、crypto/*、encoding/base64/hex |
| `image.*` | 图片魔数校验 + 插件目录读取 |

未接入后端（返回占位/空）：`platform.sendCommand`（真实平台在客户端侧）、`emoji.*`、`image.getCached`。

---

## 八、模型客户端与配置

- `chatCfg()` 惰性读取 `GeneralConfig.AgentMultimodal{Model,URL,Key}` 与 `AgentEmbedding{Model,URL,Key}`，**不硬编码模型名**。
- `chatComplete` / `embedText(s)`：OpenAI v1 协议，`stream:false`，复用 `httpClient`。
- `yara_ltp` 内置工具（`ltpx_remote.go`）调用 `YaraLTP.Run(instruction)`：将指令构造为 `YaraMessage` 路由到默认钩子点 `chat.receive.after_process`，返回 `DispatchHook` 聚合结果。

---

## 九、入口面（crystal_astral 视角）

```go
YaraLTP.SetSend(func([]byte)) // 注入出站广播（create.go）
YaraLTP.Init() error          // 构建引擎、加载插件、启动对账环
YaraLTP.HandleIn([]byte)      // 处理入站 ltp3/* 信封（ws 消费者调用）
YaraLTP.Close()               // 停止对账、卸载全部插件（服务关闭时）
YaraLTP.Run(string) (string, error) // yara_ltp 工具：文本路由到默认钩子点
```

调用顺序：集线器就绪 → `SetSend` → `Init`（此时才扫描加载插件）→ 起入站消费者；进程退出前 `Close`。

---

## 十、已知边界与取舍

- **沙箱超时**：协议定义 30s 看门狗，当前实现未做硬打断；靠插件串行锁保证同一插件内无并发。
- **跨插件反向死锁**：`yara.api.call` 若出现 A→B→A 反向调用会死锁（单向调用正常）；本插件自调用不加锁已规避自锁。
- **平台能力占位**：`platform.sendCommand`、`emoji.*` 等需真实客户端侧承接，引擎返回明确占位/空。
- **日志合规**：引擎不向本地落盘任何日志（沿用项目"不做本盘日志"约束），仅走 `LoggerGeneral` 供运行期查看。

---

## 附：相关文档

- 插件编写 → `plugin-dev-guide.md`（本文档所在目录）
- 事件客户端接入 → `client-dev-guide.md`
- 插件代码补全 → `yara.d.ts`
- 规范来源 → `../LTP3协议文档/`（PLUGIN_SYSTEM / PLUGIN_DEV_GUIDE / PLUGIN_AI_BOUNDARIES）