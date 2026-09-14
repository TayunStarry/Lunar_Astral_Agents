# 09 LTPX 协议 —— 月华工具包（Lunar Tool Package）

> [🏠 文档地图](README.md) | [◀ 上一章](08-构建运行与配置.md)
> 相关源文档：[月华 LTPX 协调层](../../lunar_astral/adapters/ltpx_remote.go) · [琉璃 LTPX 远端](../../crystal_astral/ltpx_remote.go) · [琉璃包加载/中转](../../crystal_astral/assets/script.js) · [Mini-LTP 智能体](../../crystal_astral/assets/mini_ltp_agent.js) · [Self-LTP 智能体](../../crystal_astral/assets/self_ltp_agent.js)

**LTPX（Lunar Tool Package）= 月华工具包协议**，控制系统内「如何把工具注册给月华、并让 AI 智能体调用」。核心机制是 **AtoA（Agent-to-Agent）**：月华把自然语言指令交给目标包的专用 LLM 智能体执行，由智能体自行理解意图并完成多步操作。

---

## 1. 分支体系（LTP1–LTP9）

LTPX 按「后端宿主 + 载体形态 + 交互方式」分为多条并列分支，命名以「-LTP」结尾；「LTP+数字」（LTP1–LTP9）是同系列下的**路线编号**（不是版本号，分支间无升级取代关系），两种命名一一对应：

| 路线 | 分支名 | 载体 | 定位 | 状态 |
|------|--------|------|------|------|
| LTP1 | — | `*.ltp.md` | 「工具定义 + js 实现 + 文档」单 md 文件载入 | 已废弃 |
| LTP2 | **Zero-LTP** | `.ltpx` 包（zip + html/md 页面） | 协议基座：「包 = 页面 + 元数据 + 工具定义」，以 LunarSystem（月华）为后端，经 web UI 加载/运行 | 现行 |
| LTP3 | **Yara-LTP** | CodeAgent（goja 沙箱插件） | YaraFlow 本地插件引擎，事件容器（hook/event/command）。见 §6 | 现行 |
| LTP4 | **Node-LTP** | WebApp 页面 | 专为特定 web 前端应用特调的专用 webAgent | 现行 |
| LTP5 | **Mini-LTP** | WebAgent | 通用页面操作智能体：页面最小化嵌入 + 键鼠/滚动/组合键模拟。见 §4 | 现行 |
| LTP6 | **Self-LTP** | WebAgent | 自主页面操作智能体：页面内多轮自循环，不经月华调度。见 §5 | 现行 |
| LTP7 | Face-LTP | WindowAgent | 面向桌面的通用智能体 | 已废弃 |
| LTP8 | **Auto-LTP** | WindowAgent | 桌面闭环自治智能体（多角色编排）。见 §5.3 | 现行 |
| LTP9 | **Star-LTP** | CodeAgent（goja 沙箱插件） | 插件层/引擎层/客户端三端、全同步事件模型、权限强绑定。见 §7 | 现行（主线） |
| LTP10 | **Web-LTP** | BrowserSession（顶层 WebView 会话） | 后端网络搜索智能体：真浏览器检索流水线（月华自然语言 → 搜索报告）。见 §5.4 | 现行 |

各分支共享同一包管理机制（`local_data/package/*/metadata.json` 动态扫描），差异集中在载体形态（本地页面 / WebApp / 桌面窗口 / 沙箱插件）与是否接入月华 AtoA 调度。

---

## 2. 包注册与元数据（metadata.json）

LTPX 包位于 `local_data/package/*/`，每个包用 `metadata.json` 自声明身份与工具。工具链采用**动态扫描**：`GET /ltpx/tools` 每次请求扫描各包的 `metadata.json` 中 `tools` 数组，琉璃核心不随包增删改动。

```json
{
  "id": "lunar.click-monitor",
  "title": "点击监控",
  "icon": "icon.webp",
  "tags": ["Mini-LTP"],
  "background_retention": true,
  "tools": [
    { "name": "click_monitor", "description": "虚拟键盘按键可视化工具" }
  ]
}
```

字段约定：

| 字段 | 说明 |
|------|------|
| `id` | 包唯一标识，LTPX 广播与插件引擎装载均以它路由 |
| `tags` | 分支/类别标签数组，语义见 §2.1 |
| `icon` | **相对路径**（如 `icon.webp`） |
| `background_retention` | 后台保活开关（布尔，缺省 `false`）。`true` 时琉璃关闭覆盖层后不释放该包的 `<iframe>`，再次打开优先复用已加载页面（保留输入内容、选中状态、已注入的智能体上下文）；`false` 时关闭即卸载 iframe 并回到空闲欢迎页 `/ltpx_welcome.html`，下次打开重新加载 |
| `tools[]` | `name + description`，供月华归一化为 OpenAI function schema 注册给模型 |

嵌入式 iframe 的初始/空闲页为琉璃内置的 `assets/ltpx_welcome.html`（对外 `/ltpx_welcome.html`）。

### 2.1 标签（tags）语义与示例

`tags` 声明包的**分支归属**（LTP 分支标签）与**类别**（DeepDemos / DeepSeek / Git 等）。**能否被月华 AtoA 调用与 tags 无关**——由 `metadata.json` 中是否存在非空 `tools[]` 决定（`scanAtoaToolchain` 动态扫描）。完整包清单见 [06 §6.7](06-前端资源库.md)。

**LTP 分支标签**

| 标签 | 含义 | 接入 AtoA | 当前使用示例 |
|------|------|-----------|--------------|
| `Zero-LTP` | 协议基座：本地包页面，经 web UI 加载/运行，不内嵌专用智能体 | — | `lunar.image-studio`、`lunar.novel-studio`、`lunar.engine_manager` 等 |
| `Node-LTP` | 专用 WebApp 智能体：为特定 web 应用定制指令理解与操作序列 | ✅ | `lunar.file-explorer`、`lunar.search-weather` |
| `Mini-LTP` | 通用页面操作智能体：iframe 最小化嵌入 + DOM 感知 + 键鼠/滚动/组合键 | ✅ | `lunar.click-monitor`、`deepdemos.anime-rubik-solver` 等 |
| `Self-LTP` | 自主页面操作智能体：页面（开始/停止）按钮触发，多轮自循环 | — | `deepdemos.voxel-disaster` |
| `LTP3` | YaraLTP 插件（CodeAgent，`index.js` + `config.yaml`） | 引擎装载 | `com.yaraflow.*` |
| `LTP9` | StarLTP 插件（CodeAgent，`execute.js` + `config.yaml`） | 引擎装载 | `com.yaraflow.*-ltp9` |

**非 LTP 类别标签**

| 标签 | 含义 | 示例 |
|------|------|------|
| `DeepDemos` | 自包含 web 演示/游戏包 | `deepdemos.*` 全部演示包 |
| `DeepSeek` | 外部 DeepSeek 页面入口 | `external.deepseek_api` |
| `Windows-EXE` | 外部程序启动器 | `external.napcat` |

---

## 3. AtoA 调用链路

```
月华（思考链起点）
  ──GET /ltpx/ping──▶ 琉璃           心跳探测，失败则清空联络并缓存工具链
  ──GET /ltpx/tools──▶ 琉璃          拉取最新工具链（含包页面工具 + 内置工具）
  （工具链聚合为单一 use_the_program 工具注册给模型）
月华 ──POST /ltpx/call(tool, arguments)──▶ 琉璃（按工具名路由）
  ──/ws 广播 ltpx_call──▶ 琉璃前端 → 打开包 iframe（/file/read/package/<目录>/index.html）
  ──postMessage ltpx_run──▶ 包内 LLM 智能体多轮执行（function calling 循环）
  ──postMessage ltpx_result──▶ 琉璃前端 ──POST /ltpx/result──▶ 月华
月华 ──响应──▶ 返回给调用方
```

事件链路：月华在「事件发生前」触发点经 `POST /ltpx/event` 推送事件负载给琉璃（分发给订阅了该事件的包），并以 `{return: …}` 取回订阅方回执。

**端点与超时**（月华侧协调实现在 [adapters/ltpx_remote.go](../../lunar_astral/adapters/ltpx_remote.go)，琉璃侧在 [ltpx_remote.go](../../crystal_astral/ltpx_remote.go)）：

| 端点 | 方向 | 说明 |
|------|------|------|
| `GET /ltpx/ping` | 月华→琉璃 | 心跳探测，失败则清空联络并缓存工具链 |
| `GET /ltpx/tools` | 月华→琉璃 | 拉取最新工具链 |
| `POST /ltpx/call` | 月华→琉璃 | 转发工具调用（tool + arguments） |
| `POST /ltpx/result` | 琉璃→月华 | 回传智能体执行结果 |
| `POST /ltpx/event` | 月华→琉璃 | 推送事件负载 |
| `/ltpx/register` | 琉璃→月华 | 琉璃启动时注册联络 URL（多开以最新为准） |

- 工具链同步：月华在每次思考链起点向琉璃（固定引擎端口 `BasicPort+3` = 36792）心跳并拉取最新工具链。
- 内置工具（无需包承载）：`window_agent`（→ Auto-LTP，见 §5.3）、`web_search`（→ Web-LTP，见 §5.4）、`yara_ltp`（→ YaraLTP hook 路由）。
- 超时：月华端 HTTP 请求 8s、工具调用等待 150s；琉璃端挂起调用登记 120s 超时（内置工具 `window_agent`/`web_search` 为进程内直跑，不受 120s 限制）。
- 回执协议：`ltpx_result` 含 `request_id / success / text / error / keep_open`；`keep_open` 时执行页面保持打开供用户观察。

---

## 4. Mini-LTP —— 通用页面操作智能体

入口 [mini_ltp_agent.js](../../crystal_astral/assets/mini_ltp_agent.js)，运行在目标页面 iframe 内的通用操作智能体，由琉璃在 iframe `load` 后动态注入，不改包源码。

- **多轮 function calling**：系统提示词 + 独立上下文，LLM 经工具循环逐次执行 → `tool` 消息回填 → 直到无工具调用给出最终答复。
- **模型配置**：经 `fetch('/file/read/lunar_config.json')` 读取 `agent.multimodal_model`，走同源 `/v1` 代理。
- **操作队列**：一条 `execute_operations` 提交完整操作队列，程序从前往后逐个执行（步间 0.5s），失败即中断并上报已执行记录。
- **工具集**：`capture_page / click / type / key / mouse / wheel / scroll / hover / select / wait`，支持组合键与三态按住（键入 / 短按 / 长按）。
- **视觉定位**：每轮注入一张最新视口截图，覆盖 50px 坐标网格 + 与【页面元素】列表同序的编号框，结合文本元素列表双重定位。
- **页面操作原语**：统一来自共享模块 `window.SharedInput`（`/shared-input.js`，与 Self-LTP 共享）。

---

## 5. 自主与桌面智能体

### 5.1 Self-LTP —— 自主页面操作智能体

入口 [self_ltp_agent.js](../../crystal_astral/assets/self_ltp_agent.js)。用户通过页面上的（开始/停止）按钮 + 文本框指定初始任务，智能体在目标页面内**多轮自循环执行**，不经月华调度。

**核心循环**：收到初始任务先 `set_plan` 拆解为计划项 → 每轮只执行一个原子操作 → 执行后重新观测页面（截图 + 元素）验证是否生效 → 未生效用同一操作重试或 `wait` 后再试 → 关键步骤用 `confirm_step(no, passed)` 标记 → 全部完成后 `finish` 总结。

**工具集**：`set_plan / capture_page / get_state / capture_screenshot / click / type_text / press_key / mouse_press / hover / select_option / scroll_page / scroll_wheel / wait / confirm_step / finish`。

**关键行为约定**：x/y 坐标按截图网格（`SCREEN_GRID_STEP`）编号读取；`mouse_press` 派发完整事件序列（pointerdown/mousedown → pointerup/mouseup → click）；复合指令按连词（然后/接着/并且/同时等）与标点拆分逐步执行；`press_key` 支持三态按住（默认/`short:`/`long:`）与组合键；执行后回执 `keep_open: true` 保持页面打开。

**模型**：走琉璃同源 `/v1` 代理，参数从 `lunar_config.json` 的 `agent` 字段读取。

### 5.2 Self-LTP 与 Mini-LTP 的关系

两者共享 `window.SharedInput` 页面操作原语；差异在调度方式——Mini-LTP 由月华经 AtoA 调用并逐轮回传结果，Self-LTP 在页面内自主循环直至完成。

### 5.3 Auto-LTP —— 桌面闭环自治智能体（window_agent）

实现于 `crystal_astral/agent/AutoLTP`（CGO，Windows），工具名 **`window_agent`**，作为琉璃内置工具随 `/ltpx/tools` 暴露；月华经 `/ltpx/call`（`tool=window_agent` + `instruction`）调用，路由 `AutoLTP.Run(instruction)`（[ltpx_remote.go](../../crystal_astral/ltpx_remote.go)）。

**多角色编排**（`host.go` `Run`）：

1. **提示词编纂者**（无工具）：优化/完善用户指令；
2. **软件启动者**：`DTLaunchProgram` 启动程序后 `DTActivateWindow` 置前新窗口；
3. **执行循环**（上限 30 轮）：视觉理解者（只读自动注入截图）→ UIA 理解者（读 UI 树/定向查询）→ 任务规划者（决策 complete/action）→ 单操作执行者 → 进度书记者（截图 + 记录）。

每个角色**独立全新上下文、工具白名单物理隔离**；`HandoffRecord` 为跨轮次唯一信息媒介。

**能力**：视觉 + UIA 双路理解界面、按名称/控件类型定位元素、`Invoke/Value/SelectionItem` 直接操作、坐标/键鼠兜底；`type_and_send` 原子输入发送。

**可观测性**：逐角色 trace 落盘 `local_data/logs/auto_ltp_trace.log`，截图归档 `local_data/images/moment`。

**模型**：从 `lunar_config.json` 的 `agent` 字段读取。

### 5.4 Web-LTP —— 网络搜索智能体（web_search）

实现于 `crystal_astral/agent/WebLTP/`，工具名 **`web_search`**，作为琉璃内置工具随 `/ltpx/tools` 暴露；月华经 `/ltpx/call`（`tool=web_search` + `instruction`）调用，进程内直跑 `WebLTP.Run(instruction)`（不依赖任何前端包，不受 120s 挂起登记限制）。

**固定流水线**（`host.go`）：

```
提炼检索词（模型，失败回退原指令）
→ 打开真浏览器会话（BrowserClient.WebViewSession 顶层窗口，不受 X-Frame-Options 限制）
→ 必应搜索 → 结果页滚动截图（≤3 张）→ 回到页顶
→ DOM 元素识别结果页（li.b_algo：标题/链接/摘要，天然排除广告）
→ 依次进入前 N 个结果页（≤10，指令「前N个」可覆盖）
   → 逐页滚动截图（≤10 张/页，触底即止）
   → 逐页情报摘要：DOM 提取文本为主 + 首屏截图互印证（同一次多模态调用），
     硬切断 ≤4096 字符；模型失败以正文前段兜底
→ 回到搜索引擎页（补一张截图）
→ 汇编报告：以「月华打开了什么页面、看到了什么内容」的操作旅程口吻输出
  （检索概述 → 逐页情报 → 综合结论）
→ 关闭浏览器页面；窗口意外关闭时立即停止后续操作，以已采集信息收尾
```

**DOM 与视觉的分工**：DOM 提取（`eval` 在页面上下文执行，非 OCR）承担全部硬信息（数字/名称/结论，零识别误差）；截图承担版面观感、图表信息与「验证码/错误页/付费墙」识别，两者在同一次多模态调用中互相印证。

**配置**：`lunar_config.json` 的 `web_search` 字段（每次运行热读取）——`save_screenshots`（截图是否落盘，默认 `false`）、`screenshot_dir`（相对 LocalDir）、`max_pages`（默认 3，硬上限 10）、`max_results`、`results_scroll_captures`、`page_scroll_captures`、`summary_max_chars`（≤4096）、`max_run_seconds`（总时长软上限，超时以已采集信息收尾）。

**截图**：`PrintWindow(PW_CLIENTONLY|PW_RENDERFULLCONTENT)` 直接从窗口取内容，与窗口遮挡状态无关（最小化除外）；失败回退屏幕 DC 区域截图。落盘命名 `<序号>-<标签>-视口NN.jpg`。

**端点支撑**：会话原语经 `/webview/*` 端点族暴露（[03 §4.11](03-扩展系统-钛宇-琉璃.md)），亦可供前端扩展包（如 `lunar.bing_search` 的 `backend_search`/`backend_deep_read` 工具）复用。

---

## 6. LTP3 —— YaraLTP 插件引擎（YaraFlow）

**定位**：CodeAgent 事件容器，面向 YaraFlow 项目的本地插件扩展。实现位于 `crystal_astral/agent/YaraLTP/`（15 个 Go 文件 + `docs/`），配套密钥生成器 `subsystem/ltp3_keygen`，插件样例 `local_data/package/com.yaraflow.*`。

### 载体与装载

- 每插件独立 goja 沙箱；插件 = `index.js`（逻辑）+ `config.yaml`（唯一配置文件）+ `permissions.key` + `plugin.json`；`metadata.json` 的 `tags` 含 `LTP3` 且 `id` 非空即视为插件。
- 引擎按包 `id` 装载，对账循环周期性驱动新增/删除；默认 hook 主题 `chat.receive.after_process`。
- 插件开发指南见 `agent/YaraLTP/docs/plugin-dev-guide.md`、客户端开发指南 `docs/client-dev-guide.md`、类型定义 `docs/yara.d.ts`。

### 事件分发（WS 总线）

`bus.go` 经 StudioHub.Inbound 消费入站，`HandleIn` 只处理 `ltp3/*` 前缀信封，其余旁路：

| 信封 | 回执 | 说明 |
|------|------|------|
| `ltp3/hook` | `hook_result` | `summary` 聚合 `subscribed/errored/allow_continue/aborted`，支持 `action:"abort"` 与 `allowContinue` |
| `ltp3/event` | `event_ack` | 事件通知 |
| `ltp3/command` | `command_result` | 精确匹配 + 正则回退 |
| `ltp3/manage` / `ltp3/ping` | 各带回执 | 管理与探活 |

出站 `ltp3/send` 按 `request_id` 是否存在决定单播/广播。

### 权限

`permissions.key` 由脚本哈希（拼接根目录全部 `.js`、跳过 `data/`、排序后 `sha256` 截取 16 字节 hex）作为 `lunar_decoder` 密钥加密 `allow-*` 权限清单；解码失败即拒绝全部权限。密钥由 `subsystem/ltp3_keygen`（Web UI + `/api/gen`、`/api/verify`）生成。

### 已知限制

- 跨插件 `engine.call` 反向调用（A→B→A）会死锁（单向正常；同插件自调用已规避自锁）。
- `config.yaml` 使用自研 YAML 子集解析器（注释/嵌套/序列/内联/引号），复杂 YAML 语法未全覆盖。

---

## 7. LTP9 —— StarLTP 插件引擎（主线）

**定位**：新一代 CodeAgent 插件引擎——**插件层 / 引擎层 / 客户端**三端组织，每插件独立 goja 沙箱，**全同步阻塞事件模型**，`permissions.key` 权限强绑定，内置跨包调用与前端智能体调度。实现位于 `crystal_astral/agent/StarLTP/`（16 个 Go 文件 + `docs/engine-implementation.md`、`docs/code_completion.d.ts`），另有独立发布的模块副本 `subsystem/LTP9-StarLTP/`（独立 go.mod，供其他项目模块化引用）。配套：密钥生成器 `subsystem/ltp9_keygen`、插件样例 `local_data/package/com.yaraflow.*-ltp9/`、可视化测试工作台 `local_data/package/lunar.engine_manager-ltp9/`（见 [06 §6.3](06-前端资源库.md)）。

### 核心设计

- **同步优先**：插件回调统一为同步函数返回普通对象；`engine.http.get/post`、`engine.sleep` 为阻塞式；`fetch`/`WebSocket` 以全局 Promise/事件回调存在（`allow-network` 门控）。主链路事件系统完全同步阻塞；回调返回 pending Promise 时引擎让出插件事件循环并轮询至兑现或超时（默认 90s），支持 `await fetch` 等 async 用法。
- **三端组织**：客户端 `Emit(topic, payload)` → 引擎路由到订阅器 → 插件回调 → 汇总回执。订阅器支持优先级排序、`intercept/modifiedData/return/cancel`（`return` 为业务回传通道，月华各「事件发生前」触发点即以 `{return:…}` 取回插件结果）。
- **API 面**（按 `allow-*` 注入，未授权为 `undefined`）：`engine.event / event.publish / signal / frontEvent / database / memory / file / call / agent / encoder / decoder / config / export / http / sleep / crypto / llm / image / send / ws / emoji / tool / platform / time`。
- **权限强绑定**：以打包后 `execute.js` 的 `sha256`（截取 16 字节 hex）为解密密钥解码 `permissions.key`，解出 `allow-*` 清单；**解码失败或权限名非法 → 拒绝全部权限**（`permission.go`）；开发模式跳过校验。权限名共 10 个：`allow-file / allow-database / allow-memory / allow-network / allow-call / allow-agent / allow-signal / allow-certificate / allow-send / allow-socket`。密钥由 `subsystem/ltp9_keygen` 生成（哈希规则与引擎一致，自带 `/api/verify` 自检）。

### 宿主桥接（crystal_astral）

| 文件 | 职责 |
|------|------|
| `ltp9_bridge.go` | `BridgeLTP9()`：`star.Init()` 扫描 `local_data/package` 中 `LTP9` 标签包；注入宿主通道——`SetAgentInvoker`（前端页面智能体调用）、`SetOutbound`（插件单向通知）、`SetSendInvoker`（`ltp9_send` 消息推送）、`SetWsServer`（`engine.ws` 传输）、`SetPlatformResolver`（平台名解析） |
| `ltp9_debug.go` | 调试信封桥：消费 `/ws` 上 `ltp9/event|call|tool|stats|broadcast|probe|test`，分发到引擎 `Emit/Call/CallTool/PluginStates/Broadcast`，回执经 `/ws` 广播（`ltp9/result`、`ltp9/stats_ack`、`ltp9/broadcast_ack`、`ltp9/pong`） |
| `ltp9_ws.go` | 插件 `engine.ws` 专用传输：惰性启动的独立 WS 服务（`127.0.0.1` 随机端口），按路径挂载插件 `onMessage` 回调 |

### 插件结构

```text
com.yaraflow.<name>-ltp9/
├── metadata.json      # tags 含 "LTP9"，engine: ltp9，main: execute.js
├── execute.js         # 插件逻辑（同步回调 + engine.* API）
├── config.yaml        # 插件配置
├── permissions.key    # 加密权限清单
└── data/              # 插件本地数据
```

---

## 相关文档

- [🛰 文档地图](README.md)
- [后端 LTPX 协调与适配器](02-核心系统-钛宇-月华.md)
- [琉璃 LTPX 远端与内置引擎](03-扩展系统-钛宇-琉璃.md)
- [前端资源库与扩展包目录](06-前端资源库.md)
