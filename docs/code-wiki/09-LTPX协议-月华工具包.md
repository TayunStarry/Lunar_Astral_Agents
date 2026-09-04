# 09 LTPX 协议 —— 月华工具包（Lunar Tool Package）

> [🏠 文档地图](README.md) | [◀ 上一章](08-构建运行与配置.md)
> 相关源文档：[月华 LTPX 协调层](../../lunar_astral/adapters/ltpx_remote.go) · [琉璃包加载/中转](../../crystal_astral/assets/script.js) · [Mini-LTP 智能体](../../crystal_astral/assets/mini_ltp_agent.js) · [包 metadata 示例](../../local_data/package/lunar.web-view.image-confusion/metadata.json)

**LTPX（Lunar Tool Package）= 月华工具包协议**，是控制系统内「如何把第三方工具注册给月华、并让 AI 智能体调用」的统称。核心思想是 **AtoA（Agent-to-Agent）**：月华把自然语言指令交给某个包的专用 LLM 智能体执行，由智能体自行理解意图并做多步页面操作，而非用正则/规则引擎模拟。

---

## 1. 协议分支总览

LTPX 按「后端宿主 + 载体形态 + 交互方式」分为多个分支，命名上以「-LTP」结尾：

| 分支 | 适配 Lunar AtoA | 载体 | 定位 | 状态 |
|------|----------------|------|------|------|
| **Zero-LTP** | —（协议基座） | 本地包页面 | 基础的协议版本（*.ltpx 格式：zip 压缩 + html/md 可视化页面），以 LunarSystem 为后端 | ✅ 已实现 |
| **Node-LTP** | 是 | webApp 页面 | 专为特定 web 前端应用特调的专用 webAgent | ✅ 已实现 |
| **Mini-LTP** | 是 | 最小化嵌入页面 | 基于「页面最小化嵌入」原则的通用页面操作智能体，含键盘/鼠标/滚动/组合键模拟 | ✅ 已实现 |
| **Self-LTP** | 否 | 最小化嵌入页面 | 由用户通过页面上的（开始/停止）按钮 + 文本框指定初始任务；智能体完成操作后自动唤起自己规划下一步 / 等待 / 结束 | ✅ 已实现 |
| **Face-LTP** | 是 | WindowAgent | 面向**桌面**内容与应用程序的通用智能体（点击、键入、滚动等） | ❌ 已弃用 |
| **Auto-LTP** | 是 | WindowAgent | 根据初始指令持续规划每一步做什么，并等待操作返回的通用智能体 | ✅ 已实现 |

> 各分支共享同一 AtoA 协议骨架与包管理机制，差异集中在载体形态（本地页面 / webApp / 桌面窗口）与是否自带自我规划能力。

### 1.1 Zero-LTP —— 协议基座
LTPX 的基础版本，对应 `.ltpx` 包文件格式（zip 压缩 + html/md 可视化页面），调用 LunarSystem（月华）作为后端。它奠定了「包 = 页面 + 元数据 + 工具定义」的生态基础，其余分支均架构在其组织架构之上。

### 1.2 Node-LTP —— 专用 webApp 智能体
适配 Lunar AtoA 协议的 webAgent，为**专一**的 web 前端应用特调而设计。针对特定应用深度定制指令理解、选择器策略与操作序列，追求对目标应用的精准控制。

- 包声明时 `tags` 追加 `"Node-LTP"`。

### 1.3 Mini-LTP —— 通用页面操作智能体
适配 Lunar AtoA 协议的 webAgent，基于**页面最小化嵌入**原则，是**通用**的页面操作智能体。它对任意网页主持通用的 DOM 感知与操作能力：**键盘模拟输入、鼠标模拟输入、滚动、组合键**等。典型实现在 [MiniLTP Agent](09-LTPX协议-月华工具包.md)（详见 §4）。

- 包声明时 `tags` 追加 `"Mini-LTP"`。
- 智能体以 iframe 最小化嵌入目标页面，不动包源码；琉璃在 iframe `load` 后动态注入智能体脚本再投递 `ltpx_run`。
- 工具集：`capture_page / click / type / key / mouse / wheel / scroll / hover / select / wait`，支持组合键（`Ctrl+A`/`Cmd+Shift+P`）与三态按住（键入 / 短按 / 长按）。
- 每次运行只注入**一张**最新视口截图（覆盖 50px 坐标网格 + 与【页面元素】列表同序的编号框），供多模态视觉定位元素坐标。

### 1.4 Self-LTP —— 自主页面操作智能体

**定位**：不接入 Lunar AtoA 的「自主页面操作」分支。用户通过页面上的（开始/停止）按钮 + 文本框指定【初始任务】，智能体在目标页面内**多轮自循环执行**，完成操作后自动唤起自己规划下一步 / 等待指定时长 / 决定结束任务。

**载体与实现**：[self_ltp_agent.js](../../crystal_astral/assets/self_ltp_agent.js)（v2.0.0）。琉璃经 `/self-ltp-agent.js` 在 iframe 加载 Self-LTP 包后动态注入（自带开始/停止控制面板），不动包源码；包声明时 `tags` 追加 `"Self-LTP"`（如 `deepseek.web-view.voxel-disaster`）。

**核心机制**（区别于一次性批处理）：
1. **计划**：收到初始任务先调用 `set_plan` 拆解为一连串计划项，记录为「任务历史」；
2. **执行**：多轮循环，**每轮只调用一个原子操作工具**（一次一个操作）；
3. **验证**：每个操作执行后重新观测页面（最新截图 + 元素），判断是否真正生效；
4. **重试**：某步未命中/效果不正确时用同一操作重试（可多次），或先 `wait` 再试，未确认完成绝不跳下一步；
5. **确认**：关键步骤用 `confirm_step(no, passed)` 明确标记完成或需重试；
6. **结束**：全部完成时用 `finish` 总结。

**工具集**：`set_plan / capture_page / get_state / capture_screenshot / click / type_text / press_key / mouse_press / hover / select_option / scroll_page / scroll_wheel / wait / confirm_step / finish`。

**页面操作原语**：统一来自共享模块 `window.SharedInput`（`/shared-input.js`，与 Mini-LTP 共享）。

**关键行为约定**：x/y 坐标按截图网格（`SCREEN_GRID_STEP`）编号读取；`mouse_press` 派发完整事件序列（pointerdown/mousedown → pointerup/mouseup → click）并注入 x/y；复合指令按连词（然后/接着/接下来/之后/随后/并且/同时）与标点拆分逐步执行；运行于目标页面内时「打开/进入页面」步骤识别为「页面已打开」；单字符「输入 X」无输入框时回退按键；`press_key` 支持三态按住（默认/短按 `short:`/长按 `long:`）与组合键（`Ctrl+A`、`Cmd+Shift+P`）及修饰键别名；执行后回执 `keep_open: true` 保持页面打开供用户观察。

**模型与约束**：模型调用走琉璃后端 `/v1` 代理，参数从 `lunar_config.json` 的 `agent` 字段读取（**不硬编码**）；自然语言理解完全由 LLM 完成，**禁止正则/规则引擎**模拟。

**版本演进**：v1（LTPX 生态重构引入）→ **v2.0.0**（现行）。

---

### 1.5 Face-LTP —— 桌面 WindowAgent（已弃用）

**定位**：曾把 AtoA 能力从「页面」延伸到「桌面」的 WindowAgent 分支，面向桌面内容与应用程序执行**点击、键入、滚动**等操作。

**载体与实现（历史）**：`subsystem/face_ltp`（进程内库模块，CGO，Windows），工具名 **`face_ltp_desktop_agent`**，作为琉璃固有工具经 `/ltpx/call` 暴露给月华。

**核心机制（历史设计）**：
- **UIA 优先**：`uia_dump / uia_find / uia_click / uia_input` 优先于坐标工具；坐标/键鼠仅作 UIA 失败时的兜底；
- **截图验证循环**：每个工具执行后必做截图确认实际生效，禁止未验证即报成功；
- 原子输入+发送 `type_and_send`（防焦点丢失）、`open_folder`（explorer.exe）、`close_window`（WM_CLOSE）、`press_drag`（鼠标拖拽）、`press_key('ctrl+tab')` 循环任务管理器标签等；
- 严格 48 消息上下文，`agents/` 子包承载提示词优化、规划、验证、上下文接力等子智能体。

**状态**：❌ **已弃用并整体移除**。经对比测试（与 LTP8 Auto-LTP 属「进阶对决」，二者互不调用），`window_agent`(Auto-LTP) 采用率与多场景稳定度更优，`subsystem/face_ltp` 已从代码库删除，`crystal_astral` 不再 import/require 之。详见 [03 §3.6](03-扩展系统-钛宇-琉璃.md)。

**版本演进**：v1（LTPX 生态重构引入）→ **废弃**（重构为 Auto-LTP 时删除）。

---

### 1.6 Auto-LTP —— 桌面闭环自治智能体

**定位**：桌面 WindowAgent 的「闭环自治」分支。依据初始指令**持续规划每一步行动**，等待操作返回再继续，直至任务完成。

**载体与实现**：`subsystem/auto_ltp`（进程内库模块，CGO，Windows），工具名 **`window_agent`**，作为琉璃内置桌面智能体随 `/ltpx/tools` 暴露；月华经 `/ltpx/call`（`tool=window_agent` + `instruction`）调用，路由见 [ltpx_remote.go](../../crystal_astral/ltpx_remote.go)（`AutoLTP.Run(instruction)`）。

**核心机制**（多角色编排，`host.go` `Run`）：
1. **提示词编纂者**（无工具）：`editorPhase` 优化/完善用户指令；
2. **软件启动者**：`launcherPhase`——`DTLaunchProgram` 启动程序后立即 `DTActivateWindow` 将新窗口置前；
3. **执行循环**（上限 30 轮）：**视觉理解者**（无工具，只读自动注入截图）→ **UIA 理解者**（读 UI 树/定向查询）→ **任务规划者**（无工具，决策 complete/action）→ **仅启用一个操作者**（`operationPhase`）→ **进度书记者**（截图 + 记录）。

每个角色**独立全新上下文、工具白名单物理隔离**；`HandoffRecord` 为跨轮次唯一信息媒介。

**能力**：视觉 + UIA 双路理解界面、按名称/控件类型定位元素、`Invoke/Value/SelectionItem` 直接操作、坐标/键鼠兜底；`type_and_send` 无应用限制（可用于 QQ）；排除终端输入保护。

**可观测性**：逐角色 trace 落盘 `local_data/logs/auto_ltp_trace.log`（读到/想了/决策/做了什么），截图归档 `local_data/images/moment`。

**模型**：从 `lunar_config.json` 的 `agent` 字段读取（不硬编码）。

**版本演进**：v1（重构 Face-LTP 为 Auto-LTP 时引入，替换 LTP7）。

---

## 2. 版本演进（协议版本史）

| 版本 | 载体/文件名 | 说明 | 状态 |
|------|-----------|------|------|
| LTP 1.0 | `*.ltp` / `.ltp1` | 将「工具定义 + js 函数实现 + 工具文档」打包为一个 **md 文本文件**放入指定目录供运行时自动加载；可通过 `import * as from "system.js"` 调用月华函数、甚至改写月华运行机制。 | **整套链路已废弃** |
| LTP 2.0 | `*.ltp2` / `.ltpx` | 即 **Zero-LTP**。引入琉璃对工具进行显示与管理，全面投入 WebApp 生态。曾规划「在琉璃界面点击加载工具 → 向月华注册/卸载工具」的链路，**该链路已移除**，仅保留 web UI 的加载与运行支持。 | 现行基座 |
| LTP 3.0 | `*.ltp3` | 为 YuTong 项目预留，因该项目组织架构重构与调整而**废弃**。 | 已废弃 |
| LTP 4.0(标识) | `.ltpx`（`ltp4` 仅用于版本标识，不再作后缀） | 沿用 LTP2 的组织架构，引入并针对专门 webApp 特定开发的、适配 Lunar AtoA 的 webAgent（即 **Node-LTP**）；放弃 LTP2 的工具调用/加载链路。 | 现行 |
| LTP 5.0(标识) | `.ltpx` | 基于 LTP2 的组织架构与 LTP3 的 AtoA 协议，开发的**通用** webAgent（即 **Mini-LTP**）。 | 现行 |
| LTP 6.0(标识) | `.ltpx` + `self_ltp_agent.js` | **Self-LTP**：基于页面最小化嵌入的**自主**页面操作智能体，不接入 AtoA，由页面（开始/停止）按钮 + 文本框触发；多轮自循环（计划 → 执行 → 验证 → 重试 → 确认 → 结束）。 | 现行（v2.0.0） |
| LTP 7.0(标识) | WindowAgent（`subsystem/face_ltp`） | **Face-LTP**：面向桌面的通用智能体（UIA 优先 + 坐标/键鼠兜底 + 截图验证循环），工具 `face_ltp_desktop_agent`。 | **已废弃**（被 LTP8 替代，已删除） |
| LTP 8.0(标识) | WindowAgent（`subsystem/auto_ltp`） | **Auto-LTP**：桌面闭环自治智能体，多角色编排（编纂 → 启动 → 执行循环），工具 `window_agent`。 | 现行 |

> 结论：现行 **AtoA 时代** 放弃了 LTP1 的「md 单文件 + 直接脚本注入」与 LTP2 的「琉璃手动注册/卸载」链路，统一收敛为「包自声明工具 + LLM 智能体自执行 + 结果回执」的新协议。

---

## 3. 包注册与元数据（metadata.json）

LTPX 包位于 `local_data/package/*/`，每个包用 `metadata.json` 自声明身份与工具。工具链采用**动态扫描**：`GET /ltpx/tools` 每次请求扫描各包的 `metadata.json` 中 `tools` 数组；**琉璃核心不随包增删改动**。

```json
{
  "id": "lunar.image_confusion",
  "title": "图像混淆",
  "icon": "icon.webp",
  "tags": ["Mini-LTP"],
  "tools": [
    { "name": "image_grayscale_processor", "description": "对图片执行混淆与反混淆处理的<图像混淆>小程序" }
  ]
}
```

字段约定：
- `id`：应用唯一标识，LTPX 广播携带它来路由包；
- `tags`：`AtoA`（标记适配 Lunar AtoA 链路、可被月华调用；**LTPX 是数据格式（`.ltpx` 包），不作为标签**）+ 分支标签（`Mini-LTP` 通用页面操作 / `Node-LTP` 专用 webApp / `Self-LTP` 自主）等；
- `icon`：**相对路径** `icon.webp`（禁止绝对/跨包路径）；
- `tools[]`：`name + description`，供月华脚本将其归一化为 OpenAI function schema 注册给模型。

---

## 4. 关键实现：MiniLTP Agent（通用页面操作智能体）

入口 [mini_ltp_agent.js](../../crystal_astral/assets/mini_ltp_agent.js)，是一个内嵌 AtoA 集成层、运行在目标页面 iframe 内的通用操作智能体。

- **多轮 function calling**：系统提示词 + 模块级独立上下文（保留最近 40 轮），LLM 通过工具循环逐次执行 → 以 `tool` 消息回填 → 直到无工具调用给出最终答复（上限截断防死循环）。
- **模型非硬编码**：经 `fetch('/file/read/lunar_config.json')` 读取 `agent.multimodal_model` 作为请求 `model`，走同源 `/v1` 代理，失败仅回退占位值、不改写配置。
- **操作队列**：建议一条 `execute_operations` 提交完整操作队列，程序从前往后逐个执行（步间 0.5s），失败即中断并上报已执行记录。
- **视觉识图**：每轮注入**一张**最新视口截图，覆盖 50px 坐标网格 + 编号框（编号与【页面元素】列表同序），结合文本元素列表做双重定位。

---

## 5. 现行 AtoA 调用链路（已实现）

```
月华 ──POST /ltpx/call(name, arguments)──▶ 琉璃（按工具名路由到提供该工具的包）
   ──/ws 广播 ltpx_call ──▶ 琉璃前端 → 打开包 iframe（/file/read/package/<目录>/index.html 或 ltpxFrame）
   ──postMessage ltpx_run ──▶ 包内 LLM 智能体多轮执行（function calling 循环）
   ──postMessage ltpx_result ──▶ 琉璃前端 ──POST /ltpx/result ──▶ 月华
   ──响应──▶ 返回给调用方
```

月华侧协调实现在 [adapters/ltpx_remote.go](../../lunar_astral/adapters/ltpx_remote.go)，琉璃侧链路在 [assets/script.js](../../crystal_astral/assets/script.js)。

**端点与超时：**

| 端点 | 方向 | 说明 |
|------|------|------|
| `GET /ltpx/ping` | 月华→琉璃 | 心跳探测，失败则清空联络并缓存工具链 |
| `GET /ltpx/tools` | 月华→琉璃 | 拉取最新工具链（思考链起点动态扫描） |
| `POST /ltpx/call` | 月华→琉璃 | 转发工具调用（tool + arguments） |
| `POST /ltpx/result` | 琉璃→月华 | 回传智能体执行结果 |
| `/ltpx/register` | 琉璃→月华 | 琉璃启动时注册联络 URL（多开以最新为准） |

- 工具链同步：月华在思考链起点向琉璃心跳并拉取最新工具链（琉璃可能动态增删 LTPX 插件）。
- 回执协议：`ltpx_result` 含 `request_id / success / text / error / keep_open`；执行后 `keep_open` 时页面保持打开供用户观察。

---

## 相关文档
- [🛰 文档地图](README.md)
- [后端 LTPX 协调与 /ltpx 端点](02-核心系统-钛宇-月华.md)
- [琉璃与 /ltpx 代理路由、包加载](03-扩展系统-钛宇-琉璃.md)
- [前端资源库与扩展包目录](06-前端资源库.md)