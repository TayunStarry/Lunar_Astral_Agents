# 星月智能 · 核心人格（core_personality）合并优化规划书

> 目标目录：`d:\Lunar_Astral_Agents\subsystem\core_personality`
> 来源：`crystal_astral`（琉璃，36792） + `lunar_astral`（月华，36789）
> 版本：v1（规划阶段，未落地代码）

---

## 一、总体目标

| # | 目标 | 判定标准 |
|---|------|----------|
| 1 | 两个项目功能整合进 `subsystem/core_personality` | 单 Go module、单可执行文件 |
| 2 | 单服务端、统一监听 **36789**（`BasicPort`） | 36792 不再监听 |
| 3 | 配套 **2 个 WebView 前端页面** | 月华消息终端 `/`、琉璃桌面 `/desktop` |
| 4 | 琉璃主界面路由迁移 | `/` → **`/desktop`** |
| 5 | LTPX 跨服务协调进程内化 | 删除月华→琉璃的所有 HTTP 调用与心跳轮询 |
| 6 | `/write/engine` 采用「反射 + 透传」 | 默认原样反弹广播，服务端同时保留观测与主动发送能力 |

**非目标（本次不做）**：不改动 `local_data/package/` 中扩展包的协议与目录结构；不改动子系统 `general_config / file_manager / multimodal_analysis / media_tools / browser_client / logger_general / lunar_decoder / endpoint_docs` 的对外接口。

---

## 二、现状盘点

### 2.1 两侧入口与端口

| 项 | lunar_astral（月华） | crystal_astral（琉璃） |
|----|----------------------|------------------------|
| 入口 | [main.go](file:///d:/Lunar_Astral_Agents/lunar_astral/main.go) → `server.InitializeServer()` | [main.go](file:///d:/Lunar_Astral_Agents/crystal_astral/main.go) → `StartServer(*EnginePort, ...)` |
| 端口 | `:36789`（[create.go](file:///d:/Lunar_Astral_Agents/lunar_astral/server/create.go#L18)） | `127.0.0.1:36792`（[create.go](file:///d:/Lunar_Astral_Agents/crystal_astral/create.go#L99-L107)） |
| 端口定义 | [port.go](file:///d:/Lunar_Astral_Agents/subsystem/general_config/port.go#L7-L11)：`BasicPort=36789`、`EnginePort=BasicPort+3=36792` | 同左 |
| Go 包名 | `package server`（server/）+ `package engine` | `package main`（全部在仓库子目录根） |
| 前端静态根 | `hierarchy/assets/client` → `/`（[manage.go](file:///d:/Lunar_Astral_Agents/lunar_astral/server/manage.go#L55-L56)） | `assets/` → `/`（[create.go](file:///d:/Lunar_Astral_Agents/crystal_astral/create.go#L140-L150)） |
| WebView | `OpenBrowser(url)`（单主窗口） | `OpenBrowserWithReturnButton(url, url+"/")` |

### 2.2 关键实现位置

| 能力 | 月华 | 琉璃 |
|------|------|------|
| 路由注册表 | [server/variable.go#L55-L86](file:///d:/Lunar_Astral_Agents/lunar_astral/server/variable.go#L55-L86) | [variable.go#L71-L163](file:///d:/Lunar_Astral_Agents/crystal_astral/variable.go#L71-L163) |
| WebSocket | [websocket/websocket.go#L74-L153](file:///d:/Lunar_Astral_Agents/lunar_astral/websocket/websocket.go#L74-L153)（结构化 `WSMessage{type,data}`、阻塞式广播、`bridgeToQQ`） | [ws.go#L92-L165](file:///d:/Lunar_Astral_Agents/crystal_astral/ws.go#L92-L165)（`StudioHub`，**原始字节无差别广播** + `Inbound` 通道） |
| `/write/engine` | [handlers/message.go#L42-L88](file:///d:/Lunar_Astral_Agents/lunar_astral/server/handlers/message.go#L42-L88)：解析 `type` → `animation_list` 缓存 / `telemetry` 更新坐标，**不广播** | [ws.go#L170-L191](file:///d:/Lunar_Astral_Agents/crystal_astral/ws.go#L170-L191)：读原始 body → `Broadcast`，**不解析** |
| 反向代理 | — | [handler.go#L26](file:///d:/Lunar_Astral_Agents/crystal_astral/handler.go#L26) 反代 `http://localhost:36789`；前缀表 [variable.go#L166](file:///d:/Lunar_Astral_Agents/crystal_astral/variable.go#L166) |
| 对月华进程管理 | — | `/lunar/check`、`/lunar/start` 启停 `Lunar_Astral.exe`（[handler.go#L445-L516](file:///d:/Lunar_Astral_Agents/crystal_astral/handler.go#L445-L516)） |
| LTPX 协调 | [engine/ltpx_remote.go](file:///d:/Lunar_Astral_Agents/lunar_astral/engine/ltpx_remote.go)（HTTP，心跳 8s / 轮询 3s / 上限 30min） | [ltpx_remote.go](file:///d:/Lunar_Astral_Agents/crystal_astral/ltpx_remote.go)（`/ltpx/ping|tools|call|poll|result|event`，挂起 120s、保留 30min） |
| 智能体/引擎 | goja 运行时 + JS agentSystem.js（大模型角色）、桥接 `bridging/napcat`、`engine/QwenTTS` | `engine/9.1-Flash`（StarLTP 插件引擎）、`agent/AutoLTP`、`agent/WebLTP`、`engine/KokoroTTS`、`engine/OmniVoiceTTS` |

### 2.3 当前跨服务通道（将被消除）

```
琉璃进程(36792) ──reverse proxy /v1/ /write/message /tts …──▶ 月华进程(36789)
月华(36789) ──GET /ltpx/ping|tools → POST /ltpx/call → GET /ltpx/poll──▶ 琉璃(36792)
月华 ──POST /ltpx/event（同步阻塞 30s）──▶ 琉璃
琉璃 ──POST /lunar/check|start──▶ 启停月华进程
```

---

## 三、目标架构

### 3.1 进程与端口视图

```
┌─────────────────────── Core_Personality.exe（单进程，CGO, -tags webview） ───────────────────────┐
│                                                                                                  │
│  [WebView A] 月华消息终端  http://127.0.0.1:36789/                                              │
│  [WebView B] 琉璃桌面      http://127.0.0.1:36789/desktop                                       │
│         │                          │                                                            │
│         └────── WebSocket 同一端点 /ws（全部客户端连同一个 Hub）────────────────┘                │
│                                    │                                                            │
│   server（HTTP mux / 静态 / 路由注册表）                                                         │
│        ├── transport/ws       统一 WS Hub（原始广播 + 结构化广播 + Inbound 分发）                 │
│        ├── transport/enginebus /write/engine：反射观测 + 原样透传                                │
│        ├── protocol/ltpx      LTPX Broker 接口（进程内契约）                                     │
│        ├── personality/lunar  月华人格局（agentSystem.js / napcat / capabilities / hierarchy）    │
│        ├── personality/crystal 琉璃人格格（包管理 / Auto-LTP / Web-LTP / LTP9 宿主 / studio）       │
│        └── engine/*           QwenTTS / KokoroTTS / OmniVoiceTTS / StarLTP（LTP9）                │
└──────────────────────────────────────────────────────────────────────────────────────────────────┘
                     local_data/（配置、包、记忆库、多媒体、模型）单一数据根
```

### 3.2 依赖方向（防循环导入）

```
main.go ──▶ app（装配） ──▶ server ──▶ personality/lunar, personality/crystal
                              │                │              │
                              ▼                ▼              ▼
                        transport/*      protocol/ltpx   engine/*
                              ▲                ▲
                              └────────────────┘
personality/lunar  只依赖 protocol/ltpx（**不 import** personality/crystal）
personality/crystal 实现 protocol/ltpx.Broker，由 app 在启动时注入给 lunar
```

---

## 四、目录与模块设计

```
subsystem/core_personality/
├── go.mod                          module CorePersonality（replace 指向 ../general_config 等）
├── go.sum
├── build.ps1                       统一构建：rsrc 图标 → Qwen/OmniVoice 库 → TS 打包 → 端点文档 → go build
├── icon.ico / icon.syso
├── main.go                         入口：flag → app.Assemble() → 开双 WebView → WaitForShutdown
├── app/
│   ├── create.go                   装配：初始化各子系统、注入 Broker、注册硬件回调
│   └── variable.go                 装配期全局（Instance 句柄）
├── transport/
│   ├── ws/                         统一 WebSocket Hub
│   │   ├── type.go                 WSClient / WSMessage / WSResponse / Hub
│   │   ├── variable.go             全局实例、超时常量（Ping/Pong/写超时/消息上限）
│   │   └── hub.go                  BroadcastRaw / BroadcastMessage / Inbound 分发 / 注册注销
│   └── enginebus/
│       ├── type.go                 EngineEnvelope / EngineObserver / EngineBus
│       ├── variable.go             观察者注册表、method→type 反射索引缓存
│       └── bus.go                  Publish（反射 + 透传）/ Emit（服务端主动发送）/ Subscribe
├── protocol/
│   ├── endpoint/type.go            SystemEndpoint（路由元数据，供 server 与 endpoint_docs 共用）
│   └── ltpx/
│       ├── type.go                 Broker / Status / Result / EventResult / ToolDef
│       ├── variable.go             默认 Broker 占位、调用超时常量
│       └── broker.go               进程内实现：request_id 通道 + 前端包回执 + 内置智能体直跑
├── server/
│   ├── create.go                   监听、mux 装配、静态挂载、优雅关闭
│   ├── variable.go                 ★ 统一 SystemEndpoints 路由总表（唯一真相源）
│   ├── static.go                   `/`（月华）与 `/desktop/`（琉璃）前缀静态
│   ├── type.go                     LunarCheckResponse 等遗留类型清理
│   ├── docs_endpoint.go            `/api-docs`
│   ├── window.go                   双 WebView 生命周期（多窗口管理）
│   └── handlers/                   通用 handler（message.go 的 /write/message 等）
├── personality/
│   ├── lunar/
│   │   ├── agent/                  ← lunar_astral/engine/*（除 QwenTTS）
│   │   ├── runtime/                ← lunar_astral/engine/container（goja 容器）
│   │   ├── bridging/napcat/        ← lunar_astral/bridging/napcat
│   │   ├── TypeScript/             ← lunar_astral/TypeScript（TS 源码）
│   │   └── hierarchy/              ← lunar_astral/hierarchy（go:embed 月华前端 + agentSystem.js）
│   └── crystal/
│       ├── studio/                 ← 包扫描/模块创建/load application/logs
│       ├── agent/                  ← AutoLTP / WebLTP
│       ├── ltp9/                   ← ltp9_host.go（StarLTP 宿主接线、ltp9_debug、ltp9_ws）
│       ├── webview/                ← webview_endpoint.go（/webview/* 深读会话）
│       └── web/                    ← assets/（go:embed 琉璃前端：index/style/script/planning/ltpx_welcome）
├── engine/
│   ├── ltp9/                       ← crystal_astral/engine/9.1-Flash（package StarLTP）
│   ├── qwen_tts/                   ← lunar_astral/engine/QwenTTS
│   ├── kokoro_tts/                 ← crystal_astral/engine/KokoroTTS
│   └── omnivoice_tts/              ← crystal_astral/engine/OmniVoiceTTS
├── capabilities/                   ← 月华 engine/* 中的通用能力（memory/knowledge/multimodal/screenshot/file/network/narrator/message）
├── web/                            ← 构建期产物与共享资源（可选，用于放 tsc/rollup 输出）
├── package.json / tsconfig.json / rollup.config.js    ← 由 lunar_astral 迁入，路径改写
└── node_modules/                   ← 重新 npm install
```

**命名与职责规范**（沿用仓库既有约定）
- 每个包必有 `type.go`（类型集中）与 `variable.go`（全局/常量集中）。
- 跨包导出符号：仅暴露 `Init() / Endpoints() / Broker() / Close()` 等少数入口，其余保持包内小写。
- 标识：`Lunar` = 月华，`Crystal` = 琉璃；日志前缀 `LunarCore` / `CrystalAstral` 保留以便溯源。

---

## 五、关键合并点设计

### 5.1 端口与入口收敛

| 动作 | 说明 |
|------|------|
| 唯一监听 | `server.create.StartServer()` 绑 `127.0.0.1:36789`（`AllowLAN` 时放开），复用琉璃现有 `bindHost/urlHost` 逻辑 |
| 36792 退场 | 不再 `net.Listen`；`EnginePort` 仅保留 flag 占位（标注 deprecated），不参与任何寻址 |
| 保留的独立端口 | 仅 LTP9 插件 `engine.ws` 的惰性随机端口（`127.0.0.1:0`）与 WebViewSession 顶层窗口，均非业务端口 |
| 端口硬编码清理 | `model/llama/proxy.go`、`ltpx_remote` 旧常量、前端 `config.js` 全部归一到 `window.location.host` / `GeneralConfig.BasicPort` |
| 关闭流程 | 合并为单一 `WaitForShutdown`：信号 / 双窗口全关 → 关闭 Hub → 关闭 goja 容器 → 关闭 napcat → 关闭 TTS → `server.Shutdown` |

### 5.2 路由合并（ServeMux 冲突必须去重，重复注册会 panic）

**去重（两侧同路径同实现或需合一）**

| 路径 | 处置 |
|------|------|
| `/background` `/file/write` `/file/read/` `/memory/` `/capture` `/capture/displays` `/resize` | 两侧均指向同一 `FileManager` / `MultimodalAnalysis` 处理器，**保留一份** |
| `/ws` | 合并为唯一 Hub，见 §5.3 |
| `/write/engine` | 合并为「反射 + 透传」，见 §5.4 |
| `/api-docs` | 基于统一 `SystemEndpoints` 生成 |
| `/` | 让位给月华消息终端；琉璃迁 `/desktop/` |

**移除（合并后语义消失）**：`/lunar/check`、`/lunar/start`（进程已合一）→ 改造为 `POST /window/focus {id:"lunar"|"crystal"}`，供琉璃侧边栏「呼叫月华」按钮激活对应窗口。

**新增**：`/desktop/`（琉璃前端静态）、`/window/focus`。

**保留并集**：琉璃侧 50 项（文件归档/包管理/知识库/转换/LTPX/WebView 深读/TTS）与月华侧 17 项（`/generate*`、`/v1/`、`/vram/*`、`/proxy`、`/write/message`、`/tts`）全部并入同一 `server/variable.go`，不再有反向代理与 `/proxy` 转发层。

### 5.3 WebSocket 统一 Hub

两套 hub 语义不同，**不是二选一，而是双通道并存**：

| 语义 | 来源 | 统一后的 API |
|------|------|--------------|
| 引擎总线原始帧（**仅订阅者**） | 琉璃 `StudioHub.Broadcast` | `Hub.BroadcastBus([]byte)`：只投递给显式声明 `/ws?bus=1` 的连接（慢客户端缓冲满即断开） |
| 结构化广播（全部连接） | 月华 `BroadcastMessage(type, data)` | `Hub.BroadcastMessage(string, any)`（沿用月华：阻塞式不丢消息 + `bridgeToQQ`） |
| 入站消费 | 琉璃 `StudioHub.Inbound` | `Hub.Inbound` → 多消费者分发（LTP9 引擎 `HandleInbound` 等），空 `type`/无法解析者旁路 |

要点：
- **两条通道必须分离**：结构化消息（`context`/`image`/`engine`）投递给全部连接；引擎总线原始帧只投递给订阅者。
  原因是月华消息终端对无法识别的帧会以 JSON 文本兜底渲染（见 §11.8），无差别广播会把总线噪声灌进其聊天记录。
- 订阅声明由连接 URL 的 `bus` 参数给出，**默认不订阅**（安全取向）：引擎 iframe、工作室面板、琉璃桌面与蓝图页显式声明 `bus=1`。
- 入站消息仍不做来源过滤：原始帧同时进入总线与 `Inbound`（LTP9 调试信封依赖入站分发）。
- 心跳沿用琉璃：服务端 30s `ping` / 等待 `pong` 90s / 写超时 10s；消息上限 10MB（支持 base64 图片）。
- `CheckOrigin` 沿用琉璃的同源校验（更安全），但需放行无 Origin 的本地直连（`/desktop` 页面为同源，天然通过）。
- `bridgeToQQ` 只对 `context` 等结构化消息生效，原始广播不触发。

### 5.4 `/write/engine`：反射 + 透传

**设计原则**：Web 端发来的请求**默认在服务端原样反弹广播**（保真、零改写）；服务端同时具备「观测信息」与「主动发送」两种能力。

```go
// transport/enginebus/type.go
type Envelope struct {                 // 仅用于识别，不改变透传字节
    Type    string          `json:"type"`
    Payload json.RawMessage `json:"payload"`
}

// 观察者：实现 On<Type> 方法即自动被反射索引，无需登记表
// 例：OnTelemetry 处理 type="telemetry"；OnAnimationList 处理 "animation_list"
type Observer interface{ Name() string }
```

处理流程（`POST /write/engine`）：

1. `body := io.ReadAll(r.Body)` —— **原始字节，永不改写**。
2. **反射阶段**：`json.Unmarshal` 出 `Envelope.Type`；在启动时用 `reflect.TypeOf(observer).Method(i)` 扫描观察者方法名，建立 `snake_case(type) → 方法` 的索引（`OnAnimationList → animation_list`），逐个调用。观察者**只读观测**（更新自身缓存/遥测），异常被 recover 记录，**绝不阻断透传**。
3. **透传阶段**：`Hub.BroadcastRaw(body)` —— 无差别广播给全部 `/ws` 客户端。
4. **发送能力**：`EngineBus.Emit(type, payload)` 供服务端主动构造消息并广播（内部即 `BroadcastRaw(marshal)`）。

**首批观察者**（从月华原 handler 平移，语义不变）

| 观察者 | 关注 type | 行为 |
|--------|-----------|------|
| `AnimationObserver` | `animation_list` | `websocket.CacheAnimationList(body)` → 供智能体 `getAvailableActions` |
| `TelemetryObserver` | `telemetry` | 提取 `payload.character.{x,y,z}` → `engine.UpdateAgentPosition` |

**响应契约**（两侧行为统一）

```json
{ "success": true, "length": 1234, "observed": ["animation_list"], "broadcast": 3 }
```

- 缺失 `type`：不返回 400（琉璃原本接受任意 body），改为 `observed: []` + 透传照常 + 告警日志。
- 引擎事件依旧**不进入对话上下文**（保持既有硬约束：`/write/engine` 不进 AI 上下文）。

### 5.5 LTPX 进程内化

**接口（`protocol/ltpx`）**

```go
type Broker interface {
    Status() Status                                            // 原 GET /ltpx/ping + /ltpx/tools
    Call(ctx context.Context, tool string, args map[string]any) (Result, error)  // 原 /ltpx/call + /ltpx/poll
    Event(topic string, payload any) EventResult               // 原 POST /ltpx/event
    Submit(tool string, args map[string]any) (requestID string) // 异步受理（前端包长任务）
    Await(ctx context.Context, requestID string) (Result, error) // 进程内 channel 等待
}
```

**映射与删除**

| 原机制 | 合并后 |
|--------|--------|
| `GetLTPXRemoteURL()` 固定 36792 | 删除；`Broker` 为进程内单例 |
| `GET /ltpx/ping` 心跳 + 失败清空工具链 | 删除；`Status().Online` 恒为 true（引擎已初始化即在线） |
| `GET /ltpx/tools` 拉取 | 直调 `Broker.Status()`（仍动态扫描 `local_data/package/*/metadata.json`，包增删零改动） |
| `POST /ltpx/call` + 3s 轮询 + 30min 上限 + 连续 3 次失败判死 | 删除轮询；改进程内 `await` 一个 channel。前端包路径仍需 `request_id`（前端是独立 WebView 进程，回执仍经 `/ws` → `POST /ltpx/result`），但月华侧不再有 HTTP 心跳语义 |
| 内置智能体（`window_operate`/`web_search`）进程内直跑，不受 120s | 保持不变（原本就在琉璃进程内），去除 120s 挂起登记与 30min 保留清理的跨进程痕迹 |
| 事件 `POST /ltpx/event`（HTTP 30s） | 直调 `Broker.Event`；保留 30s `context` 超时作为引擎保护（LTP9 为同步事件模型） |
| 琉璃离线回退原始数据 | 保留「无插件订阅/未改写 → 回退原始 payload」的分支（真实存在），但去掉「离线」分支 |

**JS 绑定保持兼容**（月华 TS 侧零改动或最小改动）

| 原 goja 绑定 | 变更 |
|--------------|------|
| `syncLTPXRemoteStatus()` | 内部改调 `Broker.Status()`，返回结构不变 |
| `callLTPXRemoteTool(name, args)` | 内部改调 `Broker.Call(ctx,…)`，仍返回 Promise，文本/错误前缀不变 |
| `interactLTPXEvent(topic, payload)` | 内部改调 `Broker.Event`，返回 JSON 结构不变 |
| `clearLTPXRemoteTools()` | 保留为空操作（或删除并在 TS 侧同步移除调用） |

**端点保留策略**：`/ltpx/*` 作为 `Broker` 的 HTTP 薄封装**保留**（便于用 curl/测试脚本验证与将来外部接入），但内部链路一律走进程内直调；`/ltpx/result` 必须保留（前端包回执入口）。

**注入方式**：`app.Assemble()` 中 `ltpx.SetBroker(crystal.NewBroker(...))`，`personality/lunar` 只持有接口，彻底消除跨服务 HTTP 与循环依赖。

### 5.6 双 WebView 前端

**现状风险**：`subsystem/browser_client` 的 `OpenBrowser/OpenBrowserWithReturnButton` 只维护**单实例主窗口**（`webviewInstance`），而 `OpenWebViewSession` 可创建多个独立顶层窗口（已用于 `/webview/*` 深读与 Web-LTP）。

**方案（推荐 A）**

| 方案 | 做法 | 评价 |
|------|------|------|
| **A（推荐）** | 在 `browser_client` 内新增「桌面窗口组」API：`OpenDesktopWindow(id, url, title, w, h)`，基于既有 `WebViewSession` 实例机制，支持多窗口登记/关闭广播；`WebViewClosed()` 语义改为「全部桌面窗口关闭」 | 复用已验证的多实例能力，改动局部 |
| B | 仅用 `OpenBrowser` 开月华主窗口，琉璃窗口用 `OpenWebViewSession` 常驻会话 | 改动最小，但琉璃窗口不参与关闭联动，退出行为不一致 |

**窗口定义**

| ID | URL | 标题 | 尺寸 |
|----|-----|------|------|
| `lunar` | `http://127.0.0.1:36789/` | `[星月智能] : 钛宇-月华` | 1080×810 |
| `crystal` | `http://127.0.0.1:36789/desktop` | `[星月智能] : 钛宇-琉璃` | 1500×1050 |

**前端资源迁移映射**

| 现路径 | 新路径 | 需同步修改 |
|--------|--------|-----------|
| 琉璃 `assets/index.html` `/` | `/desktop/index.html` | WebView URL；页面内相对引用 `style.css` 天然正确 |
| 琉璃 `assets/planning/` | `/desktop/planning/` | `script.js` 中打开蓝图的路径 `/planning/index.html` → `/desktop/planning/index.html` |
| 琉璃 `assets/ltpx_welcome.html` | `/desktop/ltpx_welcome.html` | 若有绝对引用需同步 |
| 琉璃 `mini_ltp_agent.js`/`self_ltp_agent.js`/`shared_input.js` | 保持顶层 `/mini-ltp-agent.js` 等 | 无（本就是独立端点下发，被注入 iframe） |
| 琉璃 iframe 包页 `/file/read/package/*` | 不变 | 无 |
| 月华 `hierarchy/assets/client` | `/`（不变） | 无 |
| 月华 `mobile/` | `/mobile/`（不变） | 无 |

### 5.7 配置、数据与构建

| 项 | 处置 |
|----|------|
| 配置文件 | 统一 `local_data/lunar_config.json`（琉璃本就依赖 `GeneralConfig` 读同一文件），新增 `webview` 相关可选组仅当需要 |
| 数据根 | 统一 `local_data/`：`package/`、`database/messages.json`、`memory/`、`multimedia/`、`models/`、`logs/`、`audios/` |
| 嵌入式资源 | 单一二进制内嵌：月华前端 + `agentSystem.js` + `prompts/*.md`，琉璃前端（含 blueprint 页），TTS 桥与 LTP9 引擎随包编译 |
| TS 构建链 | `package.json` 的 `server.side`（`tsc constructor.ts → rollup → agentSystem.js`）迁入新根，输出路径改为 `personality/lunar/hierarchy/assets/agentSystem.js`；`rollup.config.js`/`tsconfig.json` 同步改路径；需在新目录重新 `npm install` |
| 统一构建脚本 | `build.ps1` 串联：rsrc 图标 → 关闭在运行的 `Core_Personality.exe` → 按需构建 QwenTTS / OmniVoiceTTS 库 → `npm run server.side` → `endpoint_docs` 生成 → `go build -tags webview` → 输出 `../Core_Personality.exe` |
| 总构建编排 | `subsystem/build.ps1` 增加 `core_personality` 目标（置于 lunar/crystal 之后或替换二者，见 §8 决策） |
| 文档 | 修订 `docs/code-wiki/01/02/03/08/09`：过时路径（`lunar_astral/adapters/ltpx_remote.go` → `engine/ltpx_remote.go`）与端口表述（36792 退场） |

### 5.8 引擎与 TTS 归并

| 组件 | 来源 | 归并后 |
|------|------|--------|
| `StarLTP`（LTP9） | 琉璃 `engine/9.1-Flash` | `engine/ltp9`，唯一插件引擎；`9.0-pro` **不迁入**（主包未引用，仅文档） |
| `QwenTTS` | 月华 `engine/QwenTTS` | `engine/qwen_tts` + `/tts` |
| `KokoroTTS` | 琉璃 `engine/KokoroTTS` | `engine/kokoro_tts` + `/kokoro/*` |
| `OmniVoiceTTS` | 琉璃 `engine/OmniVoiceTTS` | `engine/omnivoice_tts` + `/omnivoice/*` |
| goja 运行时 | 月华 `engine/container`（主事件循环）+ 琉璃 StarLTP（每插件独立沙箱） | 共存不变：**同一主事件循环**，插件沙箱各自独立，互不干扰 |
| 统一 C++ 库目录 | `local_data/models/{Qwen3-TTS,OmniVoice}` | 不变 |

### 5.9 桥接与其它

- `bridging/napcat`：进程内协程，配置读 `lunar_config.json.bridging`，不占端口；迁移后不变，出站复用统一 Hub 的 `bridgeToQQ`。
- `lunar.rendering_engine` iframe（月华客户端）与 `/file/read/package/*`：路径与 FileServer 读取逻辑不变。
- 琉璃侧边栏按钮改造：`crystalBtn`（配置/关于）不变；`callYuehuaBtn`（呼叫月华）→ 调 `/window/focus {id:"lunar"}`。

---

## 六、合并后路由总表（统一真相源）

| 路径 | 方法 | 归属 | 变更 |
|------|------|------|------|
| `/` `/mobile/` | GET | 月华 | 不变（月华前端根） |
| `/desktop/` | GET | 琉璃 | **新增**（原 `/`） |
| `/ws` | GET | 共享 | **合并 Hub** |
| `/write/engine` | POST | 共享 | **反射 + 透传** |
| `/write/message` | POST | 月华 | 不变（唯一时序写入） |
| `/background` `/file/write` `/file/read/` `/memory/` `/capture` `/capture/displays` `/resize` | * | 共享 | **去重保留一份** |
| `/file/delete/` `/file/list/` `/file/download/` `/file/preview` `/file/archive*` `/file/move` `/file/organize` `/file/package/*` `/file/hash-rename` | * | 琉璃 | 不变 |
| `/api/packages` `/api/module/create` `/api/module/inspect` `/load/application` | * | 琉璃 | 不变 |
| `/knowledge/` `/keyframe` `/convert/image` `/convert/batch` `/convert/list` `/gguf/metadata` | * | 琉璃 | 不变 |
| `/proxy/models` `/proxy/chat` | POST | 琉璃 | 不变（AI 网关代理） |
| `/generate` `/generate/wait` | * | 月华 | 不变 |
| `/v1/` | ANY | 月华 | **删除反代**，直连本地 llama 代理 |
| `/vram/guard` `/vram/unload` `/proxy` | POST | 月华 | 不变 |
| `/tts` | POST | 月华 | 不变 |
| `/kokoro/*` `/omnivoice/*` | * | 琉璃 | 不变 |
| `/webview/open|navigate|eval|ready|screenshot|close|deepread` | POST | 琉璃 | 不变 |
| `/mini-ltp-agent.js` `/self-ltp-agent.js` `/shared-input.js` | GET | 琉璃 | 不变 |
| `/ltpx/ping|tools|call|poll|event` | * | 共享 | 保留为 Broker 的 HTTP 薄封装（内部不再使用） |
| `/ltpx/result` | POST | 共享 | 保留（前端包回执入口） |
| `/lunar/check` `/lunar/start` | * | — | **删除**，改为 `/window/focus` |
| `/window/focus` | POST | 共享 | **新增** |
| `/api-docs` | GET | 共享 | 基于统一注册表 |
| `/logs` | GET | 琉璃 | 不变（LoggerGeneral 全量） |

---

## 七、分阶段实施计划

| 阶段 | 内容 | 关键产出 | 前置 |
|------|------|----------|------|
| **P0 骨架** | 建目录、`go.mod`（module `CorePersonality` + replace 指向 `../*`）、`build.ps1` 初版、`main.go` 空壳 | `go build` 通过的空服务 | — |
| **P1 传输层** | `transport/ws`（双语义 Hub）、`transport/enginebus`（反射+透传）、`protocol/endpoint` | 单测：广播/透传/反射索引 | P0 |
| **P2 静态与双窗口** | `server/static.go`（`/` + `/desktop/`）、`browser_client` 多窗口扩展、`server/window.go` | 两个页面可打开、`/ws` 双端可连 | P1 |
| **P3 月华迁移** | `personality/lunar/*`（agent、runtime、napcat、hierarchy、TS 链）、`capabilities/*`、`engine/qwen_tts` | 月华对话链路（含 QQ 桥、TTS）在 36789 复活 | P2 |
| **P4 琉璃迁移** | `personality/crystal/*`（studio、agent、ltp9、webview）、`engine/ltp9`、`engine/kokoro_tts`、`engine/omnivoice_tts` | 琉璃工作室/包管理/蓝图/LTP9 在 36789 + `/desktop` 复活 | P2 |
| **P5 LTPX 内化** | `protocol/ltpx` Broker 实现 + 注入；月华 goja 绑定改直调；删除反向代理与 `/lunar/*` | 工具调用/事件全进程内；无 36792 | P3,P4 |
| **P6 收口清理** | 去重路由表、端口硬编码清理、`/api-docs` 重生成、文档修订、构建编排接入 | `go build` + `go vet` + TS 编译全绿 | P5 |
| **P7 验收** | 按下表逐项回归 | 验收清单全过 | P6 |

---

## 八、风险与对策

| # | 风险 | 影响 | 对策 |
|---|------|------|------|
| 1 | `BrowserClient` 单主窗口限制 | 无法同时开两个桌面窗口 | 采纳 §5.6 方案 A：扩展「桌面窗口组」API；先做最小验证（两实例并存 + 关闭联动） |
| 2 | 路由重复注册 panic | 服务起不来 | 以统一 `SystemEndpoints` 为唯一真相源，构建期加「路径唯一性」自检 |
| 3 | 两侧 `package main` / `package server` 命名冲突 | 编译失败 | 拆为 `personality/lunar`、`personality/crystal` 包；跨包仅暴露少量入口 |
| 4 | 循环导入（Broker / Hub / Endpoint 类型） | 编译失败 | 叶子包 `transport/*`、`protocol/*` 承载体；`personality/*` 只依赖叶子包；由 `app` 注入 |
| 5 | 双 goja 运行时/事件循环冲突 | 卡死或竞态 | 保持「一个主事件循环 + 插件独立沙箱」；所有 `RunOnAgentLoop` 语义集中审计，禁止在沙箱线程触碰主循环 Promise |
| 6 | `/ltpx/result` 回执仍依赖前端进程 | 长任务接口不一致 | 保留 `request_id` + 通道等待，仅去掉 HTTP 心跳；明确「前端包执行仍是跨进程」 |
| 7 | 前端硬编码端口/绝对路径 | 页面 404、WS 连不上 | 全局替换为同源 `window.location.host`；`/planning/` → `/desktop/planning/`；琉璃 `assets` 内相对引用优先相对化 |
| 8 | `//go:embed` 路径随目录变动失效 | 编译失败/资源 404 | 迁移时逐个核对 embed 指令（琉璃 `assets/*` → `personality/crystal/web/*`；月华 `hierarchy/embedded.go`） |
| 9 | TS 打包输出路径与 `node_modules` | agentSystem.js 不更新 | 新目录 `npm install`；`package.json`/`rollup.config.js` 路径同步；构建脚本做输出存在性校验 |
| 10 | CGO/窗口消息循环与双窗口线程模型 | 窗口白屏/崩溃 | 复用已验证的 `WebViewSession` 机制（Web-LTP 已在用顶层多窗口） |
| 11 | 端口与配置语义变更波及文档/记忆 | 后续开发误导 | 同步修订 `docs/code-wiki/*` 与 `RELEASE.md` 升级说明（36792 退场、`/desktop` 迁移） |
| 12 | 大文件单次迁移导致难以定位回归 | 排障成本高 | 按 P3/P4 分侧迁移，每阶段保持可编译 + 可启动 |

---

## 九、验收清单（P7）

**进程与端口**
- [ ] 仅监听 36789；`netstat` 无 36792 监听
- [ ] 关闭任一桌面窗口不误退出；全部关闭 → 优雅关闭（goja/napcat/TTS/server 依次收敛）

**前端**
- [ ] `/` 月华消息终端正常（含渲染引擎 iframe、手机端跳转）
- [ ] `/desktop` 琉璃桌面正常（蓝图子页、侧边栏全部按钮）
- [ ] 两页面均可连 `/ws`，同源无硬编码端口

**通信**
- [ ] `/write/engine` 反射观测生效（动作列表缓存、坐标遥测更新）且**原样透传**至两端前端
- [ ] 智能体动作指令链路不受影响（`engine` 类型广播 → 渲染引擎执行）
- [ ] `/write/engine` 内容**不进入**对话上下文

**LTPX**
- [ ] `use_program` 工具链动态扫描正确（含 `window_operate` / `web_search` 与包工具）
- [ ] 前端包工具调用（`/ws` → iframe → `/ltpx/result`）端到端可用
- [ ] `xx事件发生前` 事件推送经 LTP9 插件处理并正确回传
- [ ] 全流程无任何 HTTP 跨服务调用

**能力面**
- [ ] 记忆库/知识库、文件管理、图像生成、截图、`/webview/*` 深读
- [ ] `/tts`+`/kokoro/*`+`/omnivoice/*` 三个 TTS 均可用
- [ ] napcat 桥接收发与三端用户名前缀一致
- [ ] `/api-docs` 与统一路由表一致

**工程**
- [ ] `go build ./...` / `go vet ./...` 通过；TS 编译 + rollup 通过
- [ ] `subsystem/build.ps1` 一键产出 `Core_Personality.exe`

---

## 十、决策记录

| # | 决策 | 结论 |
|---|------|------|
| D1 | 双窗口实现 | **扩展 `browser_client` 桌面窗口组**：新增 `OpenDesktopWindow(id, url, title, w, h)`，基于 `WebViewSession` 多实例机制，「全部桌面窗口关闭」才触发退出 |
| D2 | 旧项目去留 | **先保留** `crystal_astral` / `lunar_astral` 作对照与回退；P7 验收通过后再统一标记弃用或删除 |
| D3 | `/ltpx/*` 端点 | **保留为 Broker 调试薄封装**（内部链路一律进程内直调）；`/ltpx/result` 必须保留 |
| D4 | 工程命名 | module **`CorePersonality`**，可执行产物 **`Core_Personality.exe`** |
| D5 | `/write/engine` 缺 `type` | **仅透传 + 告警，返回 HTTP 200**（兼容琉璃「接受任意 body」语义，`observed: []`） |

### 由决策派生的执行要求

- **D1**：`browser_client` 扩展需同时改造 `WebViewClosed()` 语义；`server/window.go` 负责登记 `lunar` / `crystal` 两个窗口并在关闭流程中统一收敛。P2 阶段先做最小验证：两实例并存 + 关闭联动。
- **D2**：`subsystem/build.ps1` 在过渡期**同时**保留 `lunar_astral`、`crystal_astral`、`core_personality` 三个目标；验收后再收缩为单一目标。
- **D3**：`/ltpx/ping|tools|call|poll|event` 的实现体 = 一行转发到 `Broker`，不承载任何业务逻辑；禁止内部调用这些端点。
- **D4**：`go.mod` 首行 `module CorePersonality`；`replace` 指向 `../general_config` 等（注意：因为位于 `subsystem/` 内，路径为 `../<子系统名>`，与旧项目的 `../subsystem/<子系统名>` 不同）。

---

## 十一、执行进度与迁移映射

### 11.1 进度台账

| 阶段 | 状态 | 产出 |
|------|------|------|
| P0 骨架 | ✅ 已完成 | `go.mod`（module CorePersonality + 8 条 replace）、`main.go`、`app/{create,variable}.go`、目录骨架、图标 `icon_lunar.ico`/`icon_crystal.ico` |
| P1 传输层 | ✅ 已完成 | `protocol/endpoint/type.go`、`protocol/ltpx/{type,variable}.go`、`transport/ws/{type,variable,hub}.go`、`transport/enginebus/{type,variable,bus,bus_test}.go` |
| P2 静态与双窗口 | ✅ 已完成 | `server/{create,static,type,response,variable,engine_bus,window}.go`、`browser_client/{desktop,desktop_windows,desktop_other}.go`、双窗口装配 |
| P3 月华迁移 | ✅ 已完成 | `personality/lunar/{engine,model,bridging/napcat,hierarchy,TypeScript}` + `engine/qwen_tts`；月华端点与 `/write/message`、`/proxy` 归入 `personality/lunar` |
| P4 琉璃迁移 | ✅ 已完成 | `personality/crystal/{web,agent,ltp9_host,ltpx_remote,module_create,webview_endpoint,handler,variable,type}` + `engine/{ltp9,kokoro_tts,omnivoice_tts}`；琉璃侧 51 条端点最终集中到 `server/variable.go` |
| P5 LTPX 内化 | ✅ 已完成 | `personality/crystal/{broker,inprocess}.go` 实现 `protocol/ltpx.Broker`；月华 `engine/ltpx_remote.go` 全量改为进程内直调；删除反向代理/`/lunar/*`/`EnginePort` |
| P6 收口 | ✅ 已完成 | 路由集中化（方案 A）、依赖单向化、C++ 编译链、TS 打包链、统一构建脚本、端点文档重生成、编排接入。仅剩文档修订见 §11.5 |
| P7 验收 | ⏳ 待执行 | 需运行时验证（模型服务 + 双窗口） |

**验证结果（当前）**

| 项目 | build | vet | test |
|------|-------|-----|------|
| `subsystem/core_personality` | ✅ | ✅ | ✅（enginebus / kokoro_tts / crystal / llama） |
| `lunar_astral`（旧，回退用） | ✅ | — | — |
| `crystal_astral`（旧，回退用） | ✅ | — | — |

### 11.5 P6 已完成明细

**P6-1 路由集中化（方案 A）**

- `server/variable.go` 现为**全系统唯一路由真相源**：69 条端点（服务端自有 3 + 月华 15 + 琉璃 51）+ 3 条 `-extra`（`/`、`/desktop/`、`/ws`）= 生成文档 72 个路径。
- 消除反向依赖：`server/handlers/` 整体删除，`/write/message` 与 `/proxy` 及其类型迁入 `personality/lunar/{message,proxy,type}.go`；人格层不再 `import CorePersonality/server`（已 grep 校验 0 命中）。
  依赖方向现为单向下行：`app → server → personality/*`。
- 代价已付：琉璃侧 25 个 handler 导出改名（`loadApplicationHandler → LoadApplicationHandler`、`ltpRemoteCallHandler → LTPRemoteCallHandler`、`webviewOpenHandler → WebViewOpenHandler`、`kokoroHandler → KokoroHandler` 等），共 9 个文件 57 处。

**P6-2 C++ 编译链**

| 目标 | 迁入内容 |
|------|----------|
| `engine/qwen_tts/` | `build.ps1`、`build_cpp.ps1`、`build_ggml.ps1` + `cpp/`（CMakeLists、src、ggml；1940 文件 / 17.4MB） |
| `engine/omnivoice_tts/` | `build.ps1` + `cpp/`（CMakeLists、src、ggml、vendor、tests…；2438 文件 / 28.2MB） |

排除项：`cpp/build/`（构建产物，共约 774MB）与 `cpp/.git`——仓库根 `**/build/` 规则已覆盖新路径，无需新增 .gitignore。
两个脚本均以自身目录为基准并向上查找 `subsystem\build_common.ps1`，**无需改写任何路径**。

**P6-3 TS 打包链**

- 迁入 `package.json`（`compile.server` / `server.side` 路径改写为 `./personality/lunar/TypeScript/…` 与 `./personality/lunar/hierarchy/assets/agentSystem.js`）、`package-lock.json`、`tsconfig.json`（`rootDir`/`outDir`/`include` 改写）、`rollup.config.js`（input/output 改写，使 `release.min` 与新布局一致）、`vitest.config.ts`（测试路径改写）。
- `npm install` 成功（74 包）；`npm run server.side` 端到端产出 `agentSystem.js` **162.9 KB**，与迁移前的预编译产物同尺寸 → 源码与产物已重新同步。

**P6-4 统一构建与编排**

- 新增 `core_personality/build.ps1`：图标 → 关进程 → QwenTTS/OmniVoiceTTS 按需建库（`-WithTTS`/`-SkipTTS`）→ `npm run server.side` → 端点文档生成 → `go build -tags webview -H windowsgui` → **输出仓库根 `Core_Personality.exe`**（上溯两级：可执行文件按自身目录解析 `local_data`）。
- `subsystem/build.ps1` 接入 `Core Personality` 目标；按决策 D2 保留旧月华/旧琉璃目标作为过渡。
- 新增 `app/startup_sound_{windows,other}.go`：`-H windowsgui` 无控制台，启动失败以失败音效 + 日志提示（对齐原琉璃体验）。
- 端点文档重生成：`server/endpoint_docs.gen.json` 由占位 `[]` 变为 72 路径真实文档。

**P6 期间修复的运行时缺陷（由冒烟测试发现）**

`/write/engine` 首次冒烟返回 `observed: []` —— 反射观察者**从未注册**，动画缓存与位置遥测均不更新（会导致 `getAvailableActions` / `getAgentPosition` 长期为空）。已补 `personality/lunar/engine/observer.go`（`AnimationObserver` / `TelemetryObserver`）并在 `app.Assemble` 注册。

### 11.5.1 运行时冒烟结果（`Core_Personality.exe`，端口 36789）

| 项 | 结果 |
|----|------|
| `/api-docs` | 200（31KB，72 路径） |
| `/`（月华消息终端） | 200（14714 字节） |
| `/desktop/`（琉璃桌面） | 200（15182 字节） |
| `/ltpx/tools` | 200（1902 字节） |
| `/kokoro/health`、`/omnivoice/health` | 200 |
| `/logs` | 200 |
| `POST /write/engine` `animation_list` | `{"success":true,"observed":["animation_list"],...}` |
| `POST /write/engine` `telemetry` | `{"success":true,"observed":["telemetry"],...}` |
| `POST /write/engine` 未知类型 | `{"success":true,"observed":[],...}`（仍透传） |
| `/ws` 在线客户端 | 实测 3 个（双桌面窗口 + 内嵌渲染器） |

### 11.5.2 剩余待办

1. **文档修订**：`docs/code-wiki/03`、`04`、`08`、`09` 与 `RELEASE.md` 中 36792 与双服务端表述。
2. **P7 完整验收**：LTPX 前端包工具调用（`/ws → iframe → /ltpx/result`）、TTS 实际合成、QQ 桥接、长任务回退等需在装有模型服务的环境逐项回归。
3. **`transcribeViaYuehua` 目标地址复核**（LTP9 `qwen_asr` 动作）。
4. 前端产物与旧工程的 `.gitignore`/`RELEASE.md` 同步（TTS cpp、node_modules、deliverable 等）。

### 11.8 系统性缺陷修复：引擎总线与消息终端的通道分离

**现象**：合并后月华消息终端前端**持续刷新消息记录**。

**根因**：`/write/engine` 的透传采用 `BroadcastRaw`（无差别广播给全部 `/ws` 客户端），依据是琉璃旧约定「无差别广播 + 客户端自行过滤」。但两个前端的容错行为截然相反：

- `personality/lunar/hierarchy/assets/client/js/websocket.js` —— 对未知帧**兜底渲染为聊天内容**（`addMessage({content: '```json…'})`），导致每条引擎帧都成为一条聊天记录；
- `local_data/package/lunar.rendering_engine/engine.js` —— 只按 `type` 分派指令，未知类型静默忽略。

同时，合并前月华的 `/write/engine` **只做观测、不广播**（`animation_list`→缓存、`telemetry`→位置），只有琉璃的 `/write/engine` 才广播。合并时把「广播」这个只属于琉璃的语义套到了两者共用的端点上，这是设计错误。

高频来源：月岩引擎以 10Hz 上报 `telemetry`/`molang`（`TELEMETRY_HZ = 10`）。

**修复**：把原始通道从「无差别广播」改为**逐连接显式订阅的引擎总线**。

| 层 | 改动 |
|----|------|
| `transport/ws` | `BroadcastRaw` → `BroadcastBus`：只投递给 `bus` 订阅者；新增 `subscribeBus()` 解析连接 URL 的 `bus` 参数（**默认不订阅**）；新增 `BusCount()`；客户端结构体新增 `bus` 字段 |
| `/write/engine` | 透传递交 `BroadcastBus`，响应中 `broadcast` 改为总线订阅者数；**反射观测不变** |
| 其余总线投递点 | LTP9 调试回执、`ltpx_call` 广播、`enginebus.Emit` 一律改走 `BroadcastBus` |
| `local_data/package/ws_bridge.js` | 声明 `?bus=1`（本客户端即总线参与者）；宿主页面可用 `?bus=0` 显式退出 |
| 月华消息终端 / 手机端引擎 iframe | iframe URL 增加 `&bus=0`——它只消费结构化 `engine` 指令（与合并前行为一致：旧月华 `/write/engine` 从不广播给它） |
| 琉璃桌面 / 蓝图页 | WS 连接显式声明 `?bus=1`（它们需要 `ltpx_call` 与 LTP9 调试回执） |

**验证**

1. 单测 `transport/ws/hub_test.go`（新增）：总线帧只到订阅者、结构化消息到全部连接、`bus` 参数解析（含缺省不订阅）。
2. 全量 `go test ./...` / `go vet ./...` 全绿。
3. 运行时（真实 exe + 两条真实 WS 连接）：
   - `POST /write/engine` → `{"success":true,"observed":["telemetry"],"broadcast":2}`
   - `?bus=1` 连接**收到**该帧；无参连接（消息终端模式）**未收到**
   - `broadcast:2` = 琉璃桌面（自身声明 `bus=1`）+ 测试订阅者，与设计一致

**设计原则**：跨进程时代「谁看得见什么」由进程天然隔离；合并为单进程后，必须用**显式订阅**把这一隔离重建出来，而不是依赖各客户端自行容错——因为各客户端的容错策略并不一致。

### 11.9 修复：琉璃桌面误显示「返回琉璃」悬浮按钮

**现象**：合并后琉璃桌面 `/desktop/` 页面上出现了本应只在「被外部链接跳转走」时才出现的「返回琉璃」悬浮按钮。

**根因**：`subsystem/browser_client/execute.go::buildReturnButtonJS` 的「主页面自身不注入」判断**硬编码了根路径**（`p === '/' || p === ''`）——这是原琉璃单窗口时代的正确假设（其主页面在 `/`）。合并后琉璃主界面迁到 `/desktop/`，判断失效，于是按钮在自身页面也被注入。

**修复**：让注入脚本按 `mainURL` 的**实际路径**判断，不再假定主页面在根路径。

- 新增 `pathOf(rawURL)` 提取主页面路径（失败回退 `/`），随脚本一并注入；
- 脚本内以 `norm(location.pathname) === norm(mainPath)` 判定（忽略尾斜杠差异）；
- 旧琉璃不受影响：其 `mainURL` 为 `/`，判定结果与原先完全一致。

**验证**

1. 新增 `subsystem/browser_client/execute_test.go`：`pathOf` 用例 + 断言脚本已改为按主页面路径判断（且不再含硬编码根路径分支）；`go test`/`go vet` 通过。
2. 逻辑等价性用 Node 实跑判定表达式，5 个用例全过：`/desktop/` 与 `/desktop` 均跳过注入；`/`、`/desktop/planning/index.html`、`/file/read/package/…` 仍注入（子页与外链页保留返回能力）。
3. `Core_Personality.exe` 重建成功；`core_personality` 全量 test/vet 全绿；`crystal_astral` 构建回归 OK。

### 11.10 合并后收尾四项（去跨进程残留 + 关窗即退出）

**一、移除琉璃「呼叫月华」按钮与机制**

进程合一后「呼叫对方启动」已无意义。移除：侧边栏按钮 `callYuehuaBtn`、对话模态框 `callYuehuaModal`、CSS（`.call-yuehua-*` / `.sidebar-btn-yuehua` / `@keyframes yuehuaFloat`）、JS 全部逻辑（`handleCallYuehua` / `playYuehuaAudio` / 事件绑定 / `YUEHUA_*` 常量）。
注：`POST /window/focus` 保留——它是通用的「置前指定桌面窗口」能力，与「呼叫启动」无关。

**二、`ASR 卸载执行失败: undefined` 根因与修复**

先回答问题：**不是**因为没用上 ASR。实测：ASR 未加载时 `/vram/unload` 返回 `200 {"unloaded":false}`，`syncFetch` 不报错、不产生该日志。

真正原因有两层：

| 层 | 事实 |
|----|------|
| 触发条件 | `lunar_config.json` 中 `allow_lan:false` → 服务**只绑 127.0.0.1**；而 `url()`（`getEnvironmentUrl`）返回**局域网 IP** `192.168.1.8` → 智能体自调用 `http://192.168.1.8:36789/vram/unload` **连接被拒**。模型调用之所以正常，是因为配置里显式写了 `http://127.0.0.1:36789/v1` |
| 文案成因 | `syncFetch` 返回的 error 是 Go 侧 `error` 值，goja 按普通 Go 对象包装（无 `message` 属性），故 `error.message` 打印为 `undefined` |

修复：

1. `getEnvironmentUrl` 与服务绑定策略对齐：默认（`allow_lan:false`）返回 `GeneralConfig.LocalBaseURL()`（`127.0.0.1`），仅放开局域网时才返回网卡 IP。一并修好同样受影响的 `/vram/guard`。
2. `loop.ts` 日志改为 `error.message ?? String(error)`，避免再现 `undefined`；
3. **顺带修掉启动竞态**：`Assemble()` 原先在人格层之后才 `server.Start`，而月华智能体一经装配即开始思考链并回调本机端点——监听未就绪时这些回调必然失败。现改为**先启动服务、再装配人格层**。

实证：`http://127.0.0.1:36789/api-docs` → 200，`http://192.168.1.8:36789/api-docs` → 连接失败，机理得证。
（该段代码与旧 `lunar_astral` 逐字节相同，属遗留问题，非本次合并引入。）

**三、移除「琉璃是否在线」判定机制**

合并为单进程后不存在在线/离线概念，故整体移除该机制而非保留恒真值：

| 位置 | 改动 |
|------|------|
| `protocol/ltpx` | `Status` 去掉 `Online`/`URL`（仅留 `Tool`）；`EventResult` 去掉 `Online` |
| `personality/crystal/broker.go` | `Status()`/`Event()` 不再产出在线字段，失败即返回零值 |
| `personality/lunar/engine` | `LTPXRemoteStatusResult` 去掉 `Online`/`URL`；`LTPXRemoteEventResult` 去掉 `Online` |
| `capabilities/ltpx.ts` | 删除 `online` 判定：改为「工具链为空（无 AtoA 能力包）则卸载工具与提示词」，并删除「调用失败即移除工具」这一离线启发式（进程内单次失败不代表工具链消失） |
| `capabilities/ltp-event.ts` | 事件结果判定去掉 `res.online`，仅按 `returned` 判定 |
| `/ltpx/ping` 端点 | **删除**（纯在线探测，已无调用方） |

**四、关窗即退出服务端（经两轮实测修正）**

**第一版修复（错误方向）**：怀疑 `w.Destroy()` 阻塞，把「删除登记 + 发关闭信号」挪到 `Destroy()` 之前。用户复测仍不退出。

**第二轮定位（实测决定性证据）**：写脚本用 `PostMessageW(WM_CLOSE)` 精确模拟点击 X，并回查后端日志，得到两条硬事实：

1. **窗口确实被销毁**（枚举该 PID 的顶层窗口已消失），**但 `Run()` 不返回**——日志里没有对应的「桌面窗口已关闭」行；
2. **两个窗口行为不一致**：`crystal`（琉璃）的 `Run()` 会返回，`lunar`（月华）的永不返回。因此 lunar 的条目永远留在注册表里，`remaining` 永不归零 —— 这正是「窗口全关而服务端不退」的直接原因（也解释了用户日志里只出现 `id=crystal` 一行）。

**最终方案**：不再依赖 `Run()` 返回，改用**可靠可观测的判据**——前端 `/ws` 会话数。

| 机制 | 作用 |
|------|------|
| 前端会话监视（`app/watchFrontendSessions`） | **主判据**：页面与后端的 `/ws` 长连接在窗口销毁时必然断开，全部页面关闭后连接数归零。需「曾有过连接（armed）」且连续 3 次采样为空才判定，避开刷新/导航的瞬时断连 |
| `DesktopWindowsClosed()`（`remaining == 0`） | 快速路径：仅当所有 `Run()` 都返回时立即触发；常规情况下由主判据兜底 |
| `shutdownTimeout`（3s）兜底 | 优雅关闭置于独立 goroutine；卡在 `Run()` 的残留窗口可能使 `Terminate`/HTTP 关闭无法及时返回，超时即不再等待，由 `os.Exit(0)` 收尾 |

**实测矩阵**（真实 exe + WM_CLOSE 模拟点 X）：

| 场景 | 结果 |
|------|------|
| 只关「琉璃」 | 进程**继续运行**，剩余月华窗口（WS 计数 3→2） |
| 只关「月华」 | 进程**继续运行** |
| 先关月华再关琉璃 | **1 秒内退出**，ExitCode=0 |
| 先关琉璃再关月华 | **1 秒内退出**，ExitCode=0 |

即保持了「全部页面关闭才退出」的语义，且两种关闭顺序都可复现退出。

> **与策略建议的偏差（需说明）**：顾问建议退化为「任一窗口关闭即结束服务端」（用户已授权的备选方案）。实测表明该降级**并非必要**：`crystal` 的 `Run()` 会返回，若按「任一关闭即退出」，会出现「关琉璃就退出、关月华不退」的**语义不一致**。因此改为保留用户首选的「全部关闭才退出」语义，而用不依赖 `Run()` 的 WS 会话监视来保证其可靠性——该判据已由上表端到端验证。若后续希望改为「任一窗口关闭即退出」，只需把 `desktop.go` 的 `remaining == 0` 条件去掉即可。

**验证**：全量 build / vet / test 通过；旧 `lunar_astral`、`crystal_astral` 构建回归 OK（共享的 `browser_client`/`general_config` 改动无副作用）；`node --check` 前端脚本通过；`呼叫月华` 相关引用零残留；端点文档由 72 → **71** 路径（`/ltpx/ping` 已删）。

### 11.11 修复：两个引擎实例的遥测跨频道串扰（studio 信息栏高频刷动）

**现象**：琉璃 studio 底部信息栏（`panels/info.html`：FPS + 折线 + 模式 + 位置）持续高频刷新。

**是服务端机制改动造成的 —— 是**，机理如下：

| 环节 | 事实 |
|------|------|
| 两个引擎实例 | 月华窗口内嵌的渲染引擎（`channel=message-terminal-renderer`）与琉璃 studio 的编辑器引擎（`channel=lunar-engine-bus`）是**两个独立实例**，各自按 `TELEMETRY_HZ = 10` 广播 `telemetry` |
| 消息接收方 | `info.html` 只过滤自己的 `source`（`info-panel`），`telemetry` 一律接收并刷新 |
| 共享常量 | 两个引擎共用 `SOURCE_ENGINE = 'engine'`，`source` 无法区分实例 |
| 合并前 | 分属两个进程：月华侧 `/write/engine` **只观测、不广播**，starstudio 永远收不到月华引擎的帧 |
| 合并后 | 同一 Hub 无差别转发 → studio 信息栏同时收到**两路遥测**（~20Hz 且数值交替），表现为高频刷动 |
| 直接原因 | **`channel` 标识两个包都已声明，却从未被兑现**：`ws_bridge.js` 把它当「保留参数」丢弃，Hub 也无频道概念 |

**修复**：为总线补齐「频道」这一维，与既有「显式订阅」组合成「**订阅 + 同频道**」投递。

| 层 | 改动 |
|----|------|
| `transport/ws` | `WSClient` 增 `channel`；`BroadcastBus(channel, msg)` 只投递给**订阅且同频道**的连接；新增 `ChannelParam`/`DefaultBusChannel`/`RequestChannel(r)` |
| `/write/engine` | 频道由发送方经 `?channel=` 声明（缺省默认频道），**反射观测不受影响**（月华引擎的遥测仍被 `TelemetryObserver` 采集，`getAgentPosition` 正常） |
| `ws_bridge.js` | 真正携带频道：连接 `?bus=1&channel=<name>`，发送 `POST /write/engine?channel=<name>` |
| 宿主侧广播 | `ltpx_call`、LTP9 调试回执、`enginebus.Emit` 走 `DefaultBusChannel`（琉璃桌面与蓝图页订阅者接收） |

**验证**

1. 单测新增 `TestBroadcastBusIsolatesChannels`（双向断言：引擎频道帧不入 studio 频道、studio 频道帧不回流引擎频道）、`TestRequestChannelDefaulting`；加上既有订阅测试共 5 项，全绿。
2. 运行时（真实 exe，两个真实频道订阅者 + 探针令牌）：
   - 引擎频道订阅者 收到 T1 ✓、**未**收到 T2 ✓（同频道还观察到 `telemetry` 持续帧）
   - 默认频道订阅者 收到 T2 ✓、**未**收到 T1 ✓
   - 侧证：探针期间月华引擎频道持续收到真实遥测（`fps:144, mode:embedded`），确认月华引擎确在持续广播——这正是串扰源

**遗留**：studio 信息栏本身仍按 10Hz 刷新（这是设计上的实时读数，非缺陷）；现在它只反映**自己那个引擎**的数据了。

### 11.12 修复：`build.ps1` 在 OmniVoiceTTS 环节失败（双 BOM + stderr 误判）

`omnivoice.dll` 此前并不存在（只有 `qwen3tts.dll` 已预编译），所以 `build.ps1` 会真正调用 OmniVoice 的 C++ 构建——**这是迁入的 C++ 编译链第一次被实际执行**，两个潜伏问题随之暴露：

**问题 1：脚本开头有两个连续 UTF-8 BOM**

文件首 6 字节为 `EF BB BF EF BB BF`。第 2 个 BOM 被 PowerShell 视作零宽字符留在脚本首部，于是 `param(` 不再是脚本的第一个 token，整个参数块被当作**语句**解析：

```
[string]$BuildType = "Release"
The assignment expression is not valid. The input to an assignment operator must be
an object that is able to accept assignments...
```

该双 BOM **逐字节继承自 `crystal_astral/engine/OmniVoiceTTS/build.ps1`**（已核对文件哈希相同）——旧工程里 `omnivoice.dll` 已存在、该脚本从不被调用，故隐患从未暴露。

修复：把重复 BOM 归一为单个（并顺带扫描本工程全部 `.ps1`，仅此一处异常；node_modules 下的无 BOM 脚本为 npm 生成的纯 ASCII，无需处理）。

**问题 2：`2>&1` 在 `ErrorActionPreference = "Stop"` 下仍会把原生命令的 stderr 升级为终止错误**

脚本第 157 行 `$output = & cmake --build ... 2>&1`，而 cmake 只输出了一条 **`CMake Warning`**（vulkan-shaders-gen 的 TryCompile 提示，本工具链下必现），PowerShell 5.1 即抛出 `NativeCommandError` 并中断脚本 —— 仅含警告的构建被误判为失败。

修复：新增 `Invoke-NativeMerged`，在调用原生命令期间临时把 `$ErrorActionPreference` 放宽为 `Continue`（退出码仍由调用处的 `$LASTEXITCODE` 显式判定），两处 cmake 调用（configure / build）改用它。

**结果**（`.\build.ps1` 全链路）

| 项 | 结果 |
|----|------|
| QwenTTS | 跳过（`qwen3tts.dll` 已存在，60.7 MB） |
| OmniVoiceTTS | **从源码构建成功**：GGML → C++ → `omnivoice.dll` **48.17 MB** 落地 `local_data/models/OmniVoice/` |
| TS 打包 | `agentSystem.js` 162.7 KB |
| 端点文档 | 71 个路径 |
| 可执行文件 | `Core_Personality.exe` 30.9 MB（仓库根） |

**遗留提示**

1. `local_data/models/OmniVoice/omnivoice-.dll`（48.17 MB，**2026-10-07**，非本次构建产物）是一个命名异常的陈旧副本，运行时不会被加载（只加载 `omnivoice.dll`），建议清理。
2. QwenTTS 的 `build_cpp.ps1` / `build_ggml.ps1` 使用**相同的** `& cmake ... 2>&1` 写法，同样存在「cmake 警告 → 误判失败」的隐患（本次未触发是因为该构建未产生 stderr 输出）。如需一并加固，套用同一 `Invoke-NativeMerged` 即可。（已在 §11.13 落实）

### 11.13 QwenTTS 构建脚本加固 + 三重 BOM 归一 + LTPX 前端守护

#### 1. QwenTTS 构建脚本加固（落实 §11.12 遗留提示 2）

- `engine/qwen_tts/build_ggml.ps1`：脚本本就设置了 `$ErrorActionPreference = "Stop"`，两处 `& cmake ... 2>&1`（configure / build）在 cmake 输出 stderr 警告时会被升级为终止错误。新增与 OmniVoice 同源的 `Invoke-NativeMerged` 并改用。
- `engine/qwen_tts/build_cpp.ps1`：`run-cmake` 内的 `& cmake @ArgList 2>&1` 同样套用（防御性，该脚本未设 Stop）。

#### 2. 四个构建脚本「三重 BOM」归一

复查时发现 `qwen_tts/{build,build_cpp,build_ggml}.ps1` 与 `omnivoice_tts/build.ps1` 的文件头均被编辑器重新写入了**三重 UTF-8 BOM**（首 9 字节 `EF BB BF ×3`）——与 §11.12 问题 1 同源（双 BOM 即可破坏 `param()` 解析），不处理则 `-WithTTS` 全量构建会在 QwenTTS 环节再次失败。已全部归一为单 BOM 并以字节级校验确认。

#### 3. `-WithTTS` 全量构建验证

`.\build.ps1 -WithTTS` 全链路通过（exit 0）：QwenTTS 的 GGML → C++ 两阶段在修复后脚本下完整执行（增量重建，BUILD SUCCESSFUL，`qwen3tts.dll` 重新落地）；OmniVoice 增量重建；`agentSystem.js`、端点文档、`Core_Personality.exe` 30.9 MB 均正常产出。

#### 4. 新机制：LTPX 前端守护（琉璃前端离线自愈唤起）

背景：月华的机制运行不依赖前端（前端仅显示与记录），但琉璃与 LTPX 的功能实现依托前端——`ltpx_call` 派发与 LTP9 插件沙箱都在琉璃桌面页面执行。用户关掉琉璃窗口后，LTPX 工具调用会广播给无人接收直至轮询超时。

| 项 | 决策 |
|----|------|
| 就绪判据 | 集线器新增 `ChannelBusCount(channel)`：琉璃桌面以 `/ws?bus=1`（缺省频道）接入，月华侧引擎 iframe 已显式 `bus=0` 退订，故「缺省频道总线订阅数 > 0」≈ 琉璃前端在线（`transport/ws/hub.go`） |
| 唤起器 | 复用 `BrowserClient.OpenDesktopWindow`（按 ID 注册，已存在则聚焦）：装配层把 `crystalWindowSpec()` 注入 `crystal.SetFrontendRelauncher`，就绪探针注入 `crystal.SetFrontendReadyProbe`（app → crystal 单向依赖） |
| 派发时机 | 前端包工具的实际广播移入后台 goroutine：受理路径立即返回 `request_id`（唤起等待不得放大受理延迟），goroutine 内先 `EnsureFrontendForDispatch()` 再广播（`personality/crystal/ltpx_remote.go`） |
| 并发与兜底 | 守护以互斥锁单飞（并发派发共享同一次唤起，双检避免重复唤起）；唤起后轮询探针（500ms 间隔，最长 20s），超时则放行广播并告警——回退旧语义（广播无人接收由轮询超时收尾），绝不无限阻塞 |
| 与退出机制兼容 | 重新唤起会重建 `/ws` 会话，`watchFrontendSessions` 的静默采样计数被自然重置，不会误触「全部前端关闭」退出；琉璃窗口关闭后其 `Run()` 会返回（§11.10 实测），注册表与探针状态一致 |

新增/改动文件：`personality/crystal/frontend_guard.go`（新）、`personality/crystal/variable.go`、`personality/crystal/ltpx_remote.go`、`transport/ws/hub.go`（`ChannelBusCount`）、`transport/ws/hub_test.go`（`TestChannelBusCount`）、`app/create.go`（窗口规格抽取 `lunarWindowSpec`/`crystalWindowSpec` + 装配注入）。

验证：`go vet ./...` 全绿；`go test ./transport/ws/... ./personality/crystal/...` 全绿；`Core_Personality.exe` 重新编译通过。前端包工具调用的端到端回归（真实唤起 → 包执行 → 回执）仍归入 P7 验收。


### 11.14 月华 TTS 配置化：统一路由（engine/tts_router）+ 三引擎惰性加载与 5 分钟闲置卸载

用户需求：① `local_data/lunar_config.json` 增加 TTS 引擎选择字段（枚举 `Qwen` / `Kokoro` / `OmniVoice`），包含控制效果的统一参数（参考音频/参考文本/其他），参数层面统一样式、执行层按引擎自身格式适配（如 Kokoro 不支持参考音频则忽略）；② 全部 TTS 引擎支持惰性加载与 5 分钟无调用自动卸载。

#### 1. 架构：统一分发下沉中立包

月华存在三条 TTS 调用链（HTTP `/tts` 端点、月华 JS 运行时 `textToSpeech` goja 绑定、LTP9 `qwen_tts` 包工具），分属 lunar 与 crystal 两个人格包。为避免 lunar ↔ crystal 互相导入，分发/加载/守护统一下沉到中立包 `engine/tts_router`（五文件：`type.go` 统一 `TTSRequest`、`variable.go` 常量、`ensure.go` 引擎 Ensure 与错误落盘、`router.go` 分发与合成、`lifecycle.go` 闲置卸载守护）。

| 项 | 决策 |
|----|------|
| 统一参数样式 | 一个 `TTSRequest` 覆盖三引擎全部字段；请求字段缺省时回退配置节默认值；执行层 switch `GeneralConfig.TTSEngine` 分发，引擎不支持的参数直接忽略（Kokoro 丢 RefAudio/RefText，Qwen 丢 Instruct 等） |
| 配置节 | `lunar_config.json` 新增 `"tts"` 节（engine/ref_audio/ref_text/voice/mix/speed/lang/instruct/seed/disable_postproc）；`general_config` 的 `ModelConfig.TTS` 解析，init() 仅在非空/非 nil 时覆盖 flag 全局，引擎枚举 switch 校验（非法值 WARN 保留默认） |
| 三条调用链收敛 | ① `/tts` 端点 → `TTSDispatchHandler`；② narrator.go `textToSpeech` → `SynthesizeBase64`；③ LTP9 `qwen_tts` 工具 → 进程内直调 `/tts`。**工具名保留 `qwen_tts` 以兼容既有 LTP 配置**（引擎切换只改配置文件，不动 LTP 包定义） |
| 参考音频解析 | `ensure.go resolveAssetPath`：绝对路径原样；相对路径先试进程 cwd 再试 exe 目录拼接；OmniVoice 另注入 `SetRefAudioBaseDirs` |

#### 2. 生命周期：RWMutex 模式 + 闲置卸载

三引擎统一纪律：**写锁 = 加载/卸载（串行化），读锁 = 合成（WithEngine 钉住实例）**——合成期间引擎不可能被卸载守护销毁，杜绝悬垂句柄调用。

| 项 | 决策 |
|----|------|
| 惰性加载 | `sync.Once` 全部替换为「mutex + 已加载标志」：**语义变更**——原实现初始化失败后 `Once` 永久锁死，新实现每次请求重试。Qwen 的启动期 eager 初始化（`initQwenTTS`）删除 |
| 闲置卸载 | `lastUsedUnix`（atomic）仅真实合成路径 `touchEngine()` 更新（`/kokoro/health` 等辅助端点不阻止卸载）；`tts_router.StartIdleUnloader()` 守护 30s 扫描，超 300s 幂等卸载并记日志；装配层 `app/create.go` 启动守护 |
| 卸载能力 | Qwen：`qwen3_tts_destroy`（C API 已有导出）+ FreeLibrary；OmniVoice：`ov_free` + FreeLibrary + 导出表清空；Kokoro：`DynamicAdvancedSession.Destroy()` 释放模型权重——**onnxruntime_go 无环境反初始化，运行时库保留**（已核对 v1.36.0 源码） |
| 卸载时序 | 卸载先拿引擎合成锁，等在途合成完成才 destroy；失败可重试不泄漏：Qwen create 失败 FreeLibrary、OmniVoice initEngine defer freeLibrary、Kokoro 失败保持 nil 自然重试 |
| 初始化错误落盘 | Ensure 失败时 `writeEngineInitError("kokoro_init_error.log"/"omnivoice_init_error.log")` 落盘供诊断 |
| 竞态修复 | `ltp9_host.go` 两处 `GetEngine()→Synthesize` 改为 `WithEngine` 闭包，消除「取到引擎指针后、合成前恰被卸载」的悬垂窗口 |

新增/改动文件：`engine/tts_router/`（type/variable/ensure/router/lifecycle，新）、`general_config/tts.go`（新）+ `general_config/init.go`（TTS 节解析）、三引擎包（qwen_tts / kokoro_tts / omnivoice_tts 的 variable/engine/handler 生命周期改造）、`personality/crystal/kokoro_bridge.go` 与 `omnivoice_bridge.go`（重写为薄中间件）、`personality/lunar/engine/narrator.go`、`personality/crystal/ltp9_host.go`、`server/variable.go`、`app/lunar.go`（去 eager）、`app/create.go`（启动守护）、`local_data/lunar_config.json`（tts 节）。

验证：`go vet ./...` 无输出；`go test ./...` 全 ok（kokoro_tts 5.458s、crystal 7.212s、llama 0.577s 等）；`go build` 成功，`Core_Personality.exe` 30.9 MB。

#### 3. 端点体系完善：统一端点与专用端点并存

`/tts` 变为按配置分发后，「明确锁定某引擎」的调用方需要专用端点。Kokoro（`/kokoro/tts|voices|dict|dict/guess|health`）与 OmniVoice（`/omnivoice/tts|langs|health`）系列原本就存在，本次补齐 Qwen 系列并迁移调用方：

| 项 | 决策 |
|----|------|
| `/qwen/tts`（POST） | `tts_router/qwen_handler.go` 新增：与 `/tts` 共用 `serveTTSPOST` 前置（CORS/解析/text 校验）与 `synthQwen` 执行层，但不 switch 配置——强制 Qwen。请求格式与统一参数样式一致（Qwen 不支持的字段自动忽略），前端工作台采样参数原样透传 |
| `/qwen/health`（GET） | 与 `/kokoro/health`、`/omnivoice/health` 对齐：请求到达即 Ensure（惰性拉起），不触碰 `touchEngine`（health 轮询不阻止闲置卸载），返回 `last_used_unix` |
| 蓝图 `qwen_tts` 节点 | `ltp9_host.go` 测试动作改直调 `QwenTTSHandler`（原直调 `/tts` 统一端点）：蓝图节点明确锁定 Qwen 音色，不应随 `tts.engine` 配置漂移；工具名仍保留 `qwen_tts` 兼容 LTP 配置 |
| 琉璃播报策略 | 初版迁至 `/qwen/tts`，随后按用户决策改回 `/tts` 统一端点：月华与琉璃同为人格体，播报引擎随 `tts.engine` 配置统一切换（`speakAssistantText` 保留 `ref_audio` 参数——Qwen/OmniVoice 用于声音克隆，Kokoro 自动忽略；源副本 `crystal_astral/assets/script.js` + 服务端副本 `personality/crystal/web/script.js` 同步） |
| qwen-tts 工作台迁移 | `local_data/package/lunar.qwen_tts-studio/app.js` 两处 `${API_BASE}/tts` → `${API_BASE}/qwen/tts`（常规合成 + 批量分块合成）；`/tts` 统一端点保留给「随配置切换引擎」的场景 |
| 陈旧 DLL 清理 | `local_data/models/OmniVoice/omnivoice-.dll` 已不在目录中（§11.12 遗留提示 1 自然消解，运行时本就只加载 `omnivoice.dll`） |

验证：`go vet ./...` 无输出；`go test ./...` 全 ok；`go build` 成功，`Core_Personality.exe` 32.4 MB。

### 11.15 琉璃 AI 对话流式化：SSE 三路分流（思考 / 工具调用 / 正文）+ 仅正文 TTS

用户需求：琉璃 AI 功能改为流式输出，正确处理工具调用与文本输出，显示思考内容，但 TTS 只朗读正文部分。

| 项 | 决策 |
|----|------|
| 后端零改动 | 前端走 `/v1/chat/completions`（llama 透明反向代理），请求体 `stream: true` 直接透传到 llama.cpp/云端——流式能力天然具备，改动纯前端 |
| SSE 解析 | `streamAssistantReply`：fetch ReadableStream 逐块解码，按行拆分 `data:` 载荷，`[DONE]` 收尾；跨块残行由 buffer 兜底 |
| 三路分流 | ① `delta.reasoning_content`（llama.cpp/DeepSeek 风格）与 `delta.reasoning`（OpenRouter 风格）→ 思考；② `delta.tool_calls` 按 index 槽位累积 `function.name`/`arguments` 增量 → 工具调用；③ `delta.content` → 正文，且内联 `<think>...</think>` 标签由 `routeContentText` 状态机并入思考（兼容云端不拆 reasoning 字段的后端） |
| 实时渲染 | 消息气泡内三区：`<details>` 思考折叠块（流式期间展开、可收起）、工具调用列表（名称 + 参数截断 200 字）、正文 Markdown 重渲染区；80ms 节流重绘 + 正文快照比对防乱序覆盖 + `.streaming::after` 打字光标 |
| 工具调用处理 | 完成后若参数 JSON 含 `action` 字段则复用 `executeStructuredAction` 执行（open_page/get_config/modify_config），否则仅展示；结构化 JSON in content（原协议）解析逻辑不变 |
| 仅正文 TTS | 思考与工具调用不入对话历史、不参与 `extractSpeechText`；流式完成后对正文走既有 `/tts` 统一端点播报（原「播放开始后再显示文本」的时序反转随流式取消——正文已逐字显示，TTS 后台播放） |
| 历史一致性 | 仅 `st.content` 压入 `messages`（沿用 20 条上限与首行移除行为），思考/工具调用不污染上下文 |

改动文件：`personality/crystal/web/script.js`（服务端副本）、`personality/crystal/web/style.css`（服务端副本）、`crystal_astral/assets/script.js` 与 `style.css`（源副本，同步）。

验证：`node --check` 两副本语法通过；`go build` 成功，`Core_Personality.exe` 32.4 MB（含更新后的内嵌前端）。流式端到端回归（真实模型思考输出/工具调用/正文播报）归入 P7 验收。


### 11.6 与规划书的偏差（已实施，附理由）

| 项 | 规划 | 实际 | 理由 |
|----|------|------|------|
| 月华能力层 | 拆为 `capabilities/{memory,knowledge,multimodal,...}` 多个包 | 保留为单个 `personality/lunar/engine` 包 | 该包共享全局状态（goja 运行时、未读队列、位置缓存）与 `variable.go/type.go` 集中约定；强行拆分会产生大量跨包导出与循环导入风险，收益不抵风险 |
| 琉璃根文件 | `handler.go` → `studio.go`、`create.go` → `web.go` | `create.go` → `web.go`（仅留 `Gethierarchy`）；`handler.go` **保留原名**并删除代理/月华进程管理函数 | 纯文件名迁移无实质收益；保留原名可减少 diff、便于与原项目对照 |
| Broker 实现方式 | `/ltpx/*` 端点改为 Broker 的薄封装 | Broker 经 `inprocess.go` 请求适配器**复用既有端点处理器**（无 socket/端口/网络） | 反向改造需重写约 400 行调用链逻辑，回归风险高；当前方案保证「协议实现只有一份」且零行为漂移，端点同时保留给 curl/测试使用 |
| `EnginePort` 删除的连带影响 | — | 为 `lunar_astral` / `crystal_astral` 各补一个本地常量 `legacyEnginePort` | 旧项目按 D2 需保持可构建作为回退，共享 flag 删除后必须补本地常量 |

### 11.7 P3–P5 期间对共享/旧项目的改动

| 文件 | 改动 |
|------|------|
| `subsystem/general_config/port.go` | **删除** `EnginePort` 与 `-engine-port`；**新增** `LocalV1URL()` / `LocalBaseURL()` |
| `subsystem/browser_client/` | 新增 `desktop.go` / `desktop_windows.go` / `desktop_other.go`（桌面窗口组） |
| `lunar_astral/engine/{variable,ltpx_remote}.go` | 补 `legacyCrystalEnginePort` 本地常量（仅保可构建） |
| `crystal_astral/{variable,main,ltp9_host}.go` | 补 `legacyEnginePort` 本地常量（仅保可构建） |

**验证命令**（Go 工具链不在 PATH，须绝对路径）

```powershell
& "C:\Users\196530\sdk\go1.26.2\bin\go.exe" build -tags webview ./...
& "C:\Users\196530\sdk\go1.26.2\bin\go.exe" vet   -tags webview ./...
& "C:\Users\196530\sdk\go1.26.2\bin\go.exe" test  ./transport/...
```

当前三项全绿（`transport/enginebus` 单测通过）。

### 11.2 P3 月华导入路径映射

| 源路径 | 目标路径 | 包名 |
|--------|----------|------|
| `lunar_astral/engine/*` | `personality/lunar/engine/` | `engine` |
| `lunar_astral/engine/container/` | `personality/lunar/engine/container/` | `container` |
| `lunar_astral/engine/QwenTTS/` | `engine/qwen_tts/` | `QwenTTS`（保持原名，导入侧用别名） |
| `lunar_astral/model/` | `personality/lunar/model/` | `model` / `llama` |
| `lunar_astral/bridging/napcat/` | `personality/lunar/bridging/napcat/` | `napcat` |
| `lunar_astral/hierarchy/` | `personality/lunar/hierarchy/` | `hierarchy` |
| `lunar_astral/TypeScript/` | `personality/lunar/TypeScript/` | —（TS 源码） |
| `lunar_astral/websocket/` | **不迁移** | 由 `transport/ws` 取代（已重写） |
| `lunar_astral/server/create.go` | **不迁移** | 逻辑并入 `server/create.go` + `app/OpenDesktopWindows` |
| `lunar_astral/server/manage.go` | **不迁移** | 逻辑并入 `app/Assemble` / `app/WaitForShutdown` |
| `lunar_astral/server/{type,variable,docs_endpoint}.go` | **合并** | 类型与路由并入 `server/{type,variable}.go` |
| `lunar_astral/server/handlers/` | `server/handlers/` | `handlers` |

**导入前缀替换顺序**（长前缀优先，避免 `LunarAstral/model` 抢先匹配 `LunarAstral/model/llama`）

```
LunarAstral/engine/container   → CorePersonality/personality/lunar/engine/container
LunarAstral/engine/QwenTTS     → CorePersonality/engine/qwen_tts
LunarAstral/engine             → CorePersonality/personality/lunar/engine
LunarAstral/model/llama        → CorePersonality/personality/lunar/model/llama
LunarAstral/model              → CorePersonality/personality/lunar/model
LunarAstral/bridging/napcat    → CorePersonality/personality/lunar/bridging/napcat
LunarAstral/hierarchy          → CorePersonality/personality/lunar/hierarchy
LunarAstral/server/handlers    → CorePersonality/server/handlers
LunarAstral/websocket          → CorePersonality/transport/ws   （随后人工适配 API）
```

**websocket 包 API 适配表**（`transport/ws` 已重写，调用点需改造）

| 旧符号 | 新归属 | 说明 |
|--------|--------|------|
| `websocket.BroadcastMessage(type, data)` | `ws.Instance.BroadcastMessage(type, data)` | 结构化广播 |
| `websocket.CacheAnimationList(body)` | `engine.CacheAnimationList(body)` | 动画缓存归属月华能力层 |
| `websocket.GetAnimCacheFunc`（注入） | 由 `engine` 自持 | 动画缓存不再寄居传输层 |
| `websocket.SetupWebSocketHandler(mux)` | 删除 | 由 `server.Start` 直接挂 `/ws` |
| `websocket.CloseWebSocketServer()` | `ws.Instance.CloseAll()` | — |
| `websocket.WSMessage/WSResponse` | `ws.WSMessage/ws.WSResponse` | 同名同结构 |
| `animCache`（包内变量） | `personality/lunar/engine` 内的 `AnimationListCache` | 从 `websocket/type.go` 迁出 |

### 11.3 P4 琉璃导入路径映射

| 源路径 | 目标路径 | 包名 |
|--------|----------|------|
| `crystal_astral/*.go`（14 个根文件） | `personality/crystal/` | `crystal`（原 `package main` 改名） |
| `crystal_astral/agent/AutoLTP/` | `personality/crystal/agent/AutoLTP/` | `AutoLTP` |
| `crystal_astral/agent/WebLTP/` | `personality/crystal/agent/WebLTP/` | `WebLTP` |
| `crystal_astral/assets/` | `personality/crystal/web/` | `//go:embed` |
| `crystal_astral/engine/9.1-Flash/` | `engine/ltp9/` | `StarLTP`（保持原名） |
| `crystal_astral/engine/KokoroTTS/` | `engine/kokoro_tts/` | `KokoroTTS` |
| `crystal_astral/engine/OmniVoiceTTS/` | `engine/omnivoice_tts/` | `OmniVoiceTTS` |
| `crystal_astral/engine/9.0-pro/` | **不迁移** | 主包未引用 |

**导入前缀替换**

```
CrystalAstral/engine/9.1-Flash   → CorePersonality/engine/ltp9
CrystalAstral/engine/KokoroTTS   → CorePersonality/engine/kokoro_tts
CrystalAstral/engine/OmniVoiceTTS → CorePersonality/engine/omnivoice_tts
CrystalAstral/agent/AutoLTP      → CorePersonality/personality/crystal/agent/AutoLTP
CrystalAstral/agent/WebLTP       → CorePersonality/personality/crystal/agent/WebLTP
```

**琉璃根文件归属**（`package main` → `package crystal` 后的职责去向）

| 原文件 | 去向 |
|--------|------|
| `create.go` | `StartServer` → `server/create.go`；`Gethierarchy` → `personality/crystal/web.go`；`initMemoryDatabase` → `app` |
| `ws.go` | `StudioHub` → `transport/ws`（已重写）；`StudioEngineHandler` → `server/engine_bus.go` |
| `variable.go` | `SystemEndpoints` → `server/variable.go`；LTPX 工具链全局变量 → `personality/crystal` |
| `handler.go` | `loadApplicationHandler`/`scanPackagesHandler`/`proxy*` → `personality/crystal/studio.go`；`getProxyHandler`/`yuehuaCheck|Start` **删除**；`logsHandler` → `personality/crystal/studio.go` |
| `ltpx_remote.go` | 保留为 `Broker` 的 HTTP 薄封装（D3） |
| `ltp9_host.go` | `personality/crystal/ltp9_host.go`（`qwen_tts` 动作改用进程内 TTS） |
| `module_create.go` | `personality/crystal/module_create.go` |
| `webview_endpoint.go` | `personality/crystal/webview_endpoint.go` |
| `docs_endpoint.go` | 并入 `server/docs_endpoint.go` |
| `kokoro_bridge.go` / `omnivoice_bridge.go` | `personality/crystal/` 内保留（TTS 桥） |

### 11.4 关键改动清单（跨阶段）

- `subsystem/general_config/port.go`：**删除 `EnginePort` 与 `-engine-port` flag**（36792 彻底退场；已确认全仓库仅 3 处引用且全部消失）。`ProxyPort`/`ModelPort` 保持不动。
- `subsystem/browser_client/`：新增桌面窗口组（`desktop.go` + `desktop_windows.go` + `desktop_other.go`），原有单实例主窗口 API 保持兼容。
- 文档：`docs/code-wiki/03`、`04`、`08`、`09`、`RELEASE.md` 中 36792 相关表述待 P6 修订。