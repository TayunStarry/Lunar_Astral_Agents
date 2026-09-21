# crystal_astral LTP9 引擎层实现文档

> 本文档描述 **crystal_astral（琉璃）** 的 LTP9 插件引擎实现，位于 `d:\Lunar_Astral_Agents\crystal_astral\engine\9.0-pro`（package `ltp9`，ServiceName `CrystalLTP9`，EngineBuild `2026-09-19`）。
> 它是 YaraFlow `engine.*` LTP9 插件引擎的**琉璃基板桥接版**：JS 层机制与 YaraFlow 完全一致，但后台所有能力落到琉璃自己的系统（日志→`LoggerGeneral`、模型→`lunar_config.json` agent 字段、记忆/表情→`FileManager/module`、加解密→`LunarDecoder`）。
> 与 LTPX/AtoA 完全解耦。

---

## 一、定位与边界

- **引擎层** = `crystal_astral/engine/9.0-pro`（package `ltp9`）。负责扫描 `local_data/package/` 下的 LTP9 包、为每个包启动**独立 goja 沙箱 + 事件循环**、经宿主注入的 `Set*` 门面把插件能力桥接到琉璃系统。
- **插件层** = `local_data/package/<包>/`，`metadata.json` 含 `LTP9` 标签即被识别；插件逻辑是 `execute.js`（打包后的单一 JS），唯一配置文件是 `config.yaml`。
- **宿主接入** = crystal_astral 主程序通过 **Go 层门面**（`Init`/`Emit`/`Call`/`CallTool`/`PluginStates` 等 + 注入 `SetModelInvoker`/`SetSendInvoker`/...）使用引擎，**不经 WebSocket**（废弃了 LTP3 时代的 `ltp3/*` WS 信封总线）。
- **与 LTPX 的不同**：LTP9 协议的 `metadata.json` **不含 `tools` 字段**（那是 AtoA/LTPX 的能力）、不走 AtoA；插件能力以 `engine.*` 全局对象提供。

---

## 二、目录与职责

```
crystal_astral/engine/9.0-pro/
├── variable.go          # 常量（ServiceName=CrystalLTP9、EngineBuild、LTP9Tag、allow-* 全集、全局注入点、模型垄断变量、包根目录）
├── type.go              # 类型集中区：plugin、engine、订阅/指令/工具/异步、模型池降级类型、熔断器与调用记录
├── yaml.go              # config.yaml 纯 Go YAML 子集解析/序列化（无第三方依赖）
├── model.go             # OpenAI v1 对话/嵌入客户端；chatCfg 惰性读琉璃 GeneralConfig.Agent*
├── mono_pool.go         # 模型垄断：单一虚拟模型池（engine.llm 强制 agent 模型）
├── circuit_breaker.go   # 模型调用熔断器（模型垄断下聚焦 agent 单模型）
├── model_stats.go       # LLM 调用记录收集器（engine.llm.getCallRecords）
├── engine.go            # 引擎管理器：包扫描、加载/卸载、对账环（热重载）、插件状态
├── plugin.go            # 单插件 goja 沙箱：事件循环、加载/卸载、回调执行、事件派发
├── permission.go        # 权限密钥：execute.js SHA-256 哈希 + LunarDecoder 解码出 allow-* 清单
├── binder.go            # 组装 engine.* 全局对象（按 allow-* 门控注入各命名空间）
├── api.go               # engine.* 实现：广播/加解密/文件/记忆/表情/数据库/HTTP 等
├── api_async.go         # engine.async 异步子任务
├── api_command.go       # engine.command 指令系统
├── api_encoding.go      # engine.encoding 编解码
├── api_network.go       # engine.network 裸 TCP/UDP/DNS
├── api_net.go           # 沙箱全局 fetch / WebSocket 客户端 / engine.http
├── api_ws.go            # 沙箱 WebSocket 客户端
├── api_ext.go           # engine.image / engine.platform / engine.send / engine.ws 服务端
├── export.go            # 宿主 facade 导出（PluginStates / CallTool / Emit / Call / Agent 等）
└── host.go              # 包级入口 Init/Version/Close/Set* 注入 + 公开查询
```

> 注意：本引擎不再有 LTP3 时代的 `agent/YaraLTP`、`bus.go`（WS 信封）、`ltp3/*` 协议。宿主改用 Go 门面直连。

---

## 三、逐插件独立沙箱

每个插件持有一个独立的 **goja Runtime + goja_nodejs 事件循环**（自带 `setTimeout`/`setInterval` 定时器）：

```go
type plugin struct {
    ID, DirName, Title, Root string
    MainPath, ConfigPath, DataDir, KeyPath string   // execute.js / config.yaml / data/ / permissions.key
    config  map[string]any
    granted map[string]bool   // 经 permissions.key 代码哈希解密校验通过的 allow-* 清单
    loop *eventloop.EventLoop  // 每插件独立事件循环
    mu   sync.Mutex            // 串行化同一插件全部 JS 执行（goja 非线程安全）
    onLoad / onUnload / onConfigUpdate jsFunc
    events / exports / commands / tools / frontSignal / asyncTasks ...
}
```

- **加载流程**：读取 `config.yaml`（失败仅告警）→ `verifyPermissions()` 解密 `permissions.key` 得 `allow-*` → 在插件事件循环内绑定 `engine.*` 并执行 `execute.js`（**30 秒超时**，防死循环脚本无限阻塞，超时 dump goroutine 栈并在卸载时 `StopNoWait` 兜底）→ 捕获 `onLoad/onUnload/onConfigUpdate` → 调用 `onLoad`。
- **事件循环**：`plugin.callFn` 在事件循环内调用 JS；若返回 Promise（async 函数 / await fetch/WebSocket），由后台 goroutine 反复短回插件事件循环轮询，直到敲定或超时（`callAwaitTimeout`=90s），期间循环保持空闲以继续处理网络/微任务回调。
- **回调执行同步**：命令/工具/事件回调都是同步阻塞的；同步调用期间定时器回调不会被调度，插件需用 `engine.sleep` 轮询等待。

---

## 四、加载 / 卸载 / 对账（engine.go）

```go
func (e *engine) LoadAll()          // 启动：扫描包根并加载全部 LTP9 插件，启动对账环
func (e *engine) reconcile()        // 对账：新增→load；磁盘已无→unload；加载失败→随目录变化重试
func (e *engine) fingerprint() string // 目录名 + metadata/execute/permissions.key/config 的 mtime+size 指纹
```

- 识别：`readMeta(root)` 读取 `metadata.json`，`tags` 含 `LTP9`（`LTP9Tag`）即视为插件。
- **热重载**：每 3 秒对账一次（目录指纹变化才执行），新增/删除/改包自动加载/卸载/重载，无需重启引擎。
- `data/` 目录不参与指纹（运行时数据）。

---

## 五、权限密钥（permission.go）

`permissions.key` 携带 `allow-*` 权限清单，被 **execute.js 的内容哈希加密**。

1. `codeHash(root)`：对 `execute.js` 取 `sha256`，取前 128 位 hex 作为解密密钥（`keyStr`）。
2. `verifyPermissions()`：读取 `permissions.key`，用 `LunarSubsystem/LunarDecoder` 的 `DecodeFilesWithKeyString(keyStr)` 整体解码。
   - 解码失败 / 解出的权限名不在 `AllowPermissionNames` 内 → 该项拒绝；全部失败则无权限加载（`engine.*` 中该能力呈 `undefined`）。
   - **无明文回退**：解码失败一律拒绝权限。
3. **开发模式**：`DeveloperMode` 默认 `false`（强制校验）；宿主 `SetDeveloperMode(true)` 跳过校验并授予全部 `allow-*`。

权限全集（`AllowPermissionNames`，10 项）：`allow-file` / `allow-database` / `allow-network` / `allow-call` / `allow-agent` / `allow-signal` / `allow-certificate` / `allow-send` / `allow-socket` / `allow-memory`。

> 加解密底层复用琉璃 **`LunarSubsystem/LunarDecoder`**（权威实现，带 `decoder_test.go`），本引擎内不再携带 `ltp9codec` 副本，避免双份实现漂移。

---

## 六、模型垄断（mono_pool.go）—— 本引擎核心差异化设计

`engine.llm` 保留 YaraFlow 的完整方法签名（`chat` / `chatWithTools` / `chatWithConfig` / `embed` / `getConfig` / `getAllConfigs` / `listTasks` / `getAvailableModels` / `getCallRecords` / `getCircuitBreakers`），但 **Go 层一律丢弃插件传入的模型参数**（`baseUrl` / `apiKey` / `model` / `taskType` / `selectionStrategy` / `model_list` / `ltp9_models.json`），强制使用琉璃 `lunar_config.json` 的 `agent` 字段模型：

- 对话/多模态 → `GeneralConfig.AgentMultimodal{Model,URL,Key}`（`chatCfg()`）
- 嵌入 → `GeneralConfig.AgentEmbedding{Model,URL,Key}`

实现要点：

- `agentChatModel()` / `agentEmbedModel()` 读取琉璃 agent 字段（`model.go`）。
- `engineLLMChat` 走单一熔断器（`getCircuitBreaker(model)`）+ 调用记账（`downstreamRecord`），模型恒为 agent 对话模型。
- 宿主可注入统一下层传输 `SetModelInvoker` / `SetEmbedInvoker`，但注入的 opts 会被 `monopolyOpts` **强制改写为 agent 模型**，保证即使走宿主传输也守住垄断。
- 查询类 API 全部归并：`getConfig(taskType)` / `getAllConfigs()` 返回 agent 单模型概要（task_type 保留入参名义）；`listTasks()` 返回 `["replyer","planner","tool_use","vlm","voice","embedding"]`；`getAvailableModels()` 返回 `[agent 对话模型名]`；`getCallRecords` / `getCircuitBreakers` 聚焦 agent 单模型。

`monoTasks` / `creatorProviderName="crystal-agent"` 在 `variable.go` 定义。

---

## 七、皮套桥接（复用琉璃基板）

| engine.* 能力 | 后台落地（琉璃） |
|---|---|
| 日志 `console` | `LunarSubsystem/LoggerGeneral`（Info/Warn/Error，不落本盘） |
| 模型 `llm.*` | 琉璃 `GeneralConfig.Agent*` 字段 + `model.go` 的 OpenAI v1 客户端（模型垄断） |
| 加解密 `encoder/decoder` | `LunarSubsystem/LunarDecoder` |
| 记忆 `memory.*` | `LunarSubsystem/FileManager/module` 记忆库（`ltp9_memory` 集合） |
| 表情 `emoji.*` | `FileManager/module` 记忆库 `stickers`（image 型）集合 |
| 文件 `file.*` / 图片 `image.*` | 插件 `data/` 目录 + 魔数校验 |
| 数据库 `database.*` | **未接入**（占位错误） |
| 网络 `http`/`network`/`fetch`/`WebSocket` | Go 标准库 net/http、net |
| 发送 `send.*` | 宿主 `SetSendInvoker` 注入（未注入→"未接入发送通道"） |
| 平台 `platform.*` | 宿主 `SetPlatformResolver` 注入 |
| WS 服务端 `ws.*` | 宿主 `SetWsServer` 注入 |
| 前端智能体 `agent.*` | 宿主 `SetAgentInvoker` 注入（转交 Mini-LTP / Node-LTP 包） |
| 包根目录 | `GeneralConfig.LocalDir/package`（宿主 `SetRootOverride` 可覆盖） |

**未接入占位**（保留签名与门控）：`database.query/exec`、`emoji.store`。其余 host 注入能力在未注入时返回明确的"未接入"错误。

---

## 八、宿主接入（Go 门面，host.go / export.go）

宿主通过 Go 直接调用，**不经 WS**：

```go
ltp9.Init()                                       // 构建引擎、加载插件、启动对账环（幂等）
ltp9.SetRootOverride(dir)                         // 可选，Init 前指定插件根目录
ltp9.SetDeveloperMode(true)                       // 可选，跳过权限密钥校验
ltp9.SetSendInvoker(fn) / SetPlatformResolver(fn) // 注入发送/平台能力
ltp9.SetModelInvoker(fn) / SetEmbedInvoker(fn)    // 可选，统一下层模型传输（仍强制 agent 模型）
ltp9.SetWsServer(bridge) / SetAgentInvoker(fn)    // 注入 WS 服务端 / 前端智能体
ltp9.SetOutbound(fn)                              // 接收 engine.signal 单向通报

ltp9.Emit(topic, payload, requestID) EmitResult   // 向订阅该 topic 的插件派发事件（含拦截/改写/撤回/回传）
ltp9.Call(pluginID, fnName, args)                 // 跨插件调用导出函数
ltp9.CallTool(pluginID, name, args)               // 调用插件工具（AtoA/LLM 接头）
ltp9.Agent(pluginID, text)                        // 调用前端智能体
ltp9.Broadcast / BroadcastTo                      // 向 frontEvent.signal 广播
ltp9.PluginStates() []PluginState                 // 枚举插件状态（事件/导出/工具）
ltp9.Rescan()                                     // 手动对账（热更新）
ltp9.Version() / ltp9.Close()                      // 版本 / 关闭并冲刷调用记录
```

调用顺序：宿主就绪 → （可选 `SetRootOverride`/`SetDeveloperMode`）→ `Init`（此时扫描加载插件）→ 使用各 Emit/Call/注入 → 进程退出前 `Close`。

> 引擎当前是否已接入 crystal_astral 主程序二进制、以及哪些管线事件被桥接到 `Emit`，取决于主程序接入——接入后以宿主实际调用为准。

---

## 九、已知边界与取舍

- **模型垄断是硬约束**：插件无法自定义模型，查询类 API 全归并 agent 单模型。
- **沙箱事件循环**：采用 goja_nodejs eventloop，定时器与 Promise 在插件自己的循环上执行；同步调用期间定时器回调不会被调度。
- **权限无明文回退**：密钥解码失败一律拒绝权限。
- **未接入占位**：`database`、`emoji.store` 未接入；`send/platform/ws/agent` 依赖宿主注入。
- **日志合规**：引擎不向本地落盘任何日志（沿用项目约束），仅走琉璃 `LoggerGeneral` 供运行期查看。
- **数据库**：引擎不再自带 LTP3 时代的 SQLite/知识库桥接，`database.*` 保留签名待宿主接入。

---

## 附：相关文档

- 插件编写 → `plugin-dev-guide.md`（本文档所在目录）
- 插件代码补全 → `engine.d.ts`
- 客户端接入 → `client-dev-guide.md`（旧 LTP3 WS 协议，已不适用于本引擎的 Go 门面接入，仅供参考）