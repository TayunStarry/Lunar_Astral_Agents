# 09 LTPX 协议 —— 月华工具包（Lunar Tool Package）

> [🏠 文档地图](README.md) | [◀ 上一章](08-构建运行与配置.md)
> 相关源文档：[月华 LTPX 协调层](../../lunar_astral/adapters/ltpx_remote.go) · [琉璃包加载/中转](../../crystal_astral/assets/script.js) · [Mini-LTP 智能体](../../crystal_astral/assets/mini_ltp_agent.js) · [包 metadata 示例](../../local_data/package/lunar.web-view.image-confusion/metadata.json)

**LTPX（Lunar Tool Package）= 月华工具包协议**，是控制系统内「如何把第三方工具注册给月华、并让 AI 智能体调用」的统称。核心思想是 **AtoA（Agent-to-Agent）**：月华把自然语言指令交给某个包的专用 LLM 智能体执行，由智能体自行理解意图并做多步页面操作，而非用正则/规则引擎模拟。

---

## 1. 协议分支总览

LTPX 按「后端宿主 + 载体形态 + 交互方式」分为多个分支，命名上以「-LTP」结尾：

| 分支 | 适配 Lunar AtoA | 载体 | 定位 | 状态 |
|------|----------------|------|------|------|
| **Zero-LTP** | —（协议基座） | 本地包页面 | 基础的协议版本（*.ltpx 格式：zip 压缩 + html/md 可视化页面），以 LunarSystem 为后端 | 现行 |
| **Node-LTP** | 是 | webApp 页面 | 专为特定 web 前端应用特调的专用 webAgent | 现行 |
| **Mini-LTP** | 是 | 最小化嵌入页面 | 基于「页面最小化嵌入」原则的通用页面操作智能体，含键盘/鼠标/滚动/组合键模拟 | 现行 |
| **Self-LTP** | 否 | 最小化嵌入页面 | 由用户通过页面上的（开始/停止）按钮 + 文本框指定初始任务；智能体完成操作后自动唤起自己规划下一步 / 等待 / 结束 | 暂未实现 |
| **Face-LTP** | 是 | WindowAgent | 面向**桌面**内容与应用程序的通用智能体（点击、键入、滚动等） | 暂未实现 |
| **Auto-LTP** | 是 | WindowAgent | 根据初始指令持续规划每一步做什么，并等待操作返回的通用智能体 | 暂未实现 |

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

### 1.4 Self-LTP / Face-LTP / Auto-LTP —— 规划中
- **Self-LTP**：不再接入 Lunar AtoA，改由用户在页面上点「开始/停止」并输入初始任务；智能体发布一组页面操作后**自动唤起自己**，基于当前页面内容规划下一步 / 等待指定时长 / 决定结束任务。
- **Face-LTP**：把 AtoA 能力从「页面」延伸到「桌面」，作为 WindowAgent 操作桌面内容与应用程序。
- **Auto-LTP**：作为 WindowAgent，依据初始指令持续规划每一步行动，并等待操作返回再继续，形成闭环自治。

---

## 2. 版本演进（协议版本史）

| 版本 | 载体/文件名 | 说明 | 状态 |
|------|-----------|------|------|
| LTP 1.0 | `*.ltp` / `.ltp1` | 将「工具定义 + js 函数实现 + 工具文档」打包为一个 **md 文本文件**放入指定目录供运行时自动加载；可通过 `import * as from "system.js"` 调用月华函数、甚至改写月华运行机制。 | **整套链路已废弃** |
| LTP 2.0 | `*.ltp2` / `.ltpx` | 即 **Zero-LTP**。引入琉璃对工具进行显示与管理，全面投入 WebApp 生态。曾规划「在琉璃界面点击加载工具 → 向月华注册/卸载工具」的链路，**该链路已移除**，仅保留 web UI 的加载与运行支持。 | 现行基座 |
| LTP 3.0 | `*.ltp3` | 为 YuTong 项目预留，因该项目组织架构重构与调整而**废弃**。 | 已废弃 |
| LTP 4.0(标识) | `.ltpx`（`ltp4` 仅用于版本标识，不再作后缀） | 沿用 LTP2 的组织架构，引入并针对专门 webApp 特定开发的、适配 Lunar AtoA 的 webAgent（即 **Node-LTP**）；放弃 LTP2 的工具调用/加载链路。 | 现行 |
| LTP 5.0(标识) | `.ltpx` | 基于 LTP2 的组织架构与 LTP3 的 AtoA 协议，开发的**通用** webAgent。 | 现行 |

> 结论：现行 **AtoA 时代**（LTP4/LTP5）放弃了 LTP1 的「md 单文件 + 直接脚本注入」与 LTP2 的「琉璃手动注册/卸载」链路，统一收敛为「包自声明工具 + LLM 智能体自执行 + 结果回执」的新协议。

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