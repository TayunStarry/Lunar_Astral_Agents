# crystal_astral LTP9 插件开发指南（engine.*）

> 本文档面向 **crystal_astral（琉璃）** 的 LTP9 插件开发者。本引擎位于 `crystal_astral/engine/9.0-pro`（package `ltp9`），
> 是 YaraFlow `engine.*` LTP9 插件引擎的**琉璃基板桥接版**：插件 JS 层机制（命名空间、生命周期、事件语义、权限密钥）与 YaraFlow 完全一致，
> 但后台一切能力都落到琉璃自己的系统（日志→`LoggerGeneral`、模型→`lunar_config.json` 的 agent 字段、记忆/表情→`FileManager/module`、加解密→`LunarDecoder`）。
>
> 每个插件是一个**独立目录**（`metadata.json` + `execute.js` + `permissions.key`），在**独立的 goja 沙箱 + 事件循环**中运行，
> 通过全局对象 `engine.*` 访问宿主能力。所有权限经 **permissions.key 代码哈希加密**校验，未授予的能力在 JS 中呈 `undefined`。

## 目录

1. [快速开始](#快速开始)
2. [包结构（metadata.json）](#包结构metadatajson)
3. [权限与 permissions.key](#权限与-permissionskey)
4. [配置（engine.config）](#配置engineconfig)
5. [完整 API 参考](#完整-api-参考)
6. [工具 (Tool) 开发](#工具-tool-开发)
7. [指令 (Command) 开发](#指令-command-开发)
8. [事件与插件间通信](#事件与插件间通信)
9. [生命周期](#生命周期)
10. [调试与日志](#调试与日志)
11. [从 LTP3（yara.*）迁移指南](#从-ltp3yara-迁移指南)
12. [已知限制](#已知限制)
13. [常见问题](#常见问题)
14. [附录：API 速查表](#附录api-速查表)

***

## 快速开始

### 最小插件示例

**目录结构**（单层目录，无 `plugin.json` / `index.js`）：

```
local_data/package/com.example.hello/
├── metadata.json      # LTP9 标签清单（必需）
├── execute.js         # 插件主脚本（必需，固定文件名）
├── permissions.key    # 权限密钥（必需，随插件包由作者提供）
├── config.yaml        # 插件配置（可选，首次运行可自动生成）
└── data/              # 运行时数据目录（engine.file / engine.image 写入隔离区）
```

**metadata.json**：

```json
{
  "id": "com.example.hello",
  "title": "你好插件",
  "author": "Crystal",
  "version": "1.0.0",
  "type": "plugin",
  "description": "一个简单的 LTP9 示例插件",
  "tags": ["LTP9"]
}
```

**execute.js**：

```javascript
// 顶层执行：注册工具 / 订阅事件 / 导出函数 / 初始化（同步执行）
engine.tool.register("hello", {
  description: "打个招呼。当用户说你好、hello、hi 时调用。",
  parameters: [
    { name: "name", type: "string", description: "打招呼对象的名字", required: false }
  ]
}, function(params, context) {
  var name = (params && params.name) ? params.name : "世界";
  var groupId = context ? context.groupId : "";
  var r = engine.send.text(groupId, "你好呀，" + name + "！");
  return r && r.success ? "已打招呼" : "发送失败: " + (r && r.error ? r.error : "未知错误");
});

// 生命周期：插件加载完成时调用
function onLoad() {
  console.info("你好插件已加载");
}

// 生命周期：插件卸载时调用
function onUnload() {
  console.info("你好插件已卸载");
}

// 生命周期：config.yaml 回写后触发（engine.config.setFile 调用后）
function onConfigUpdate(scope, config, version) {
  console.info("配置已更新: scope=" + scope + " version=" + version);
}
```

### 获取权限密钥

> 🔑 **权限密钥由插件作者生成，并随插件包（含 `permissions.key`）一起分发。**
> 插件使用者直接使用作者提供的插件包即可，**不需要自行生成密钥**；
> 缺少密钥 / 密钥失效 / 改了 execute.js 需要重新签发时，**联系插件作者**获取新密钥。
>
> ⚠️ **改过 execute.js 就必须让作者重新签发 permissions.key**：密钥与代码绑定，代码一变旧密钥即失效，插件会以无权限加载。

### 加载与触发

- 插件放进 `local_data/package/`（＝ `GeneralConfig.LocalDir/package`，宿主可用 `ltp9.SetRootOverride` 显式指定）后，引擎启动时扫描加载，之后每 3 秒对账一次，目录变化自动热加载/卸载。
- 插件注册的工具经宿主汇总进 LLM 工具系统，LLM 在对话中自主决定调用；工具 handler 同步执行，返回值作为工具结果文本回给模型。
- 插件注册的命令经宿主命令处理器匹配后，以 `ltp9.Command(pluginID, text, context)` 触发对应插件。
- 插件事件默认不参与主程序消息管线（见 [事件](#事件与插件间通信) 的已知限制）。

***

## 包结构（metadata.json）

LTP9 插件是**单层目录**，不再有 `plugin.json` 清单（LTP3 时代的 `plugin.json`、`index.js`、`main` 字段均已废弃）。
引擎按目录下的 `metadata.json` 中 `tags` 含 `"LTP9"` 识别插件，目录名即插件目录名（`id` 为空时用目录名兜底）。

### metadata.json 字段

| 字段 | 类型 | 必需 | 说明 |
|------|------|------|------|
| `id` | string | 是 | 插件唯一 ID（建议 `com.xxx.yyy` 反域名格式）。为空时用目录名 |
| `title` | string | 是 | 插件显示名（前端列表 / 日志使用） |
| `author` | string | 否 | 作者名 |
| `version` | string | 否 | 版本号（如 `1.0.0`） |
| `type` | string | 否 | 插件类型，惯例填 `"plugin"` |
| `description` | string | 否 | 插件描述 |
| `tags` | string[] | 是 | **必须含 `"LTP9"`**（引擎以此识别）；`"Mini-LTP"`/`"Node-LTP"` 标签表示前端智能体包（不参与本引擎加载，作为 `engine.agent` 的可选目标） |
| `icon` | string | 否 | 图标路径（如 `icon.svg`） |

### 目录内其他文件

| 文件 | 必需 | 说明 |
|------|------|------|
| `execute.js` | 是 | 插件主脚本（固定文件名）。顶层执行注册组件，之后按生命周期回调 |
| `permissions.key` | 是 | 权限密钥（**由插件作者生成并随插件包分发**，见『获取权限密钥』章节） |
| `config.yaml` | 否 | 插件配置（引擎启动时读取解析；缺失时插件自行生成，见配置章节） |
| `data/` | 否 | 运行时数据目录。`engine.file` / `engine.image` 的**所有路径都相对此目录解析**，写入隔离 |
| `icon.svg` 等 | 否 | 前端展示资源 |

***

## 权限与 permissions.key

插件的 `engine.*` 能力按 `permissions.key` 中的 `allow-*` 权限门控：**未纳入的权限对应命名空间不会注入**（JS 中呈 `undefined`），脚本顶层调用即抛 `TypeError`。

### 权限全集（10 项）

| 权限名 | 注入的 engine.* 能力 |
|--------|----------------------|
| `allow-file` | `engine.file`（read/write/delete）、`engine.image`（loadValid/download，下载另需 allow-network） |
| `allow-network` | `engine.http`、`engine.network`、全局 `fetch`、全局 `WebSocket` |
| `allow-agent` | `engine.llm`（chat/embed/模型池查询）、`engine.tool`（register/getDefinitions）、`engine.agent` |
| `allow-send` | `engine.send`（text/image/hybrid）、`engine.platform`（getName/getGroupId/lookupUser/sendCommand） |
| `allow-signal` | `engine.signal`（all/target）、`engine.frontEvent`（signal.subscribe/unsubscribe） |
| `allow-certificate` | `engine.crypto`（JWT/摘要/HMAC/Ed25519）、`engine.encoder`/`engine.decoder`（复用琉璃 `LunarDecoder`） |
| `allow-call` | `engine.call`（跨包函数调用） |
| `allow-memory` | `engine.memory`（store/search，桥接琉璃记忆库）、`engine.emoji`（search/random；store 未接入） |
| `allow-database` | `engine.database`（query/exec，未接入） |
| `allow-socket` | `engine.ws`（expose/publish） |

**常驻基础能力（不参与 allow-* 开关，始终注入）**：`engine.config`、`engine.event`、`engine.time`、`engine.sleep`、`engine.encoding`、`engine.command`、`engine.async`、`engine.export`、全局 `console`。

### 密钥签发与失效

`permissions.key` 由**插件作者**用密钥工具按 `execute.js` 内容签发，并随插件包分发；**改过 `execute.js` 必须让作者重新签发**。
密钥与 `execute.js` 的内容强绑定（取 `execute.js` 的 SHA-256 前 128 位 hex 作为解密密钥，解出 `allow-*` 清单）。

若解不出合法权限（密钥失效 / 文件缺失 / 修改过代码），插件**仍会加载**（加载不失败），但权限清单为空 → 所有 `allow-*` 能力不注入：
调用 `engine.send.*` / `engine.http.*` 等会得到 `TypeError: engine.send is undefined`。引擎日志（Warn 级）会输出：

```
插件权限密钥解码失败，已拒绝全部权限
```

两种降级情况：

- `permissions.key` 文件缺失 → 按无权限加载，日志：`插件缺少权限密钥文件，按无权限加载（仅保留基础 engine API）`
- 解出的权限名不在允许集合内 → 该项被跳过，日志：`插件权限密钥解出非法权限名`

### 开发模式

宿主调用 `ltp9.SetDeveloperMode(true)` 可跳过权限密钥校验并默认授予全部 `allow-*` 权限。**默认 `false`（强制密钥校验）**，
便于调试但**发布前务必关闭**并重新生成 `permissions.key`。

***

## 配置（engine.config）

插件配置存放在插件根目录的 `config.yaml`，引擎加载时读取并解析为对象（解析失败仅告警，不阻断加载）。

```javascript
var config = engine.config.getFile();          // → config.yaml 解析对象；无配置时为空对象
var apiKey = config.api ? config.api.key : "";

engine.config.setFile({ plugin: { enabled: true }, api: { key: "..." } });
// 行为：1) 内存替换配置对象；2) 序列化回写 config.yaml；3) 触发 onConfigUpdate("plugin", config, "")
```

> ⚠️ `setFile` **只接收对象**，不接收 YAML 字符串。想以 YAML 文本方式写配置请用 `engine.file.write("config.yaml", yaml字符串)`。

### 让 WebUI 渲染你的配置（registerSchema）

插件**自己负责**配置项的中文名/说明/类型：在 execute.js 顶层调用 `registerSchema` 声明各配置节与字段，
宿主不做任何翻译——未声明的配置项由宿主按值自动推断类型并显示原 key。

```javascript
engine.config.registerSchema([
  {
    name: "plugin",
    label: "插件设置",
    order: 1,
    fields: [
      { name: "enabled", label: "启用插件", type: "boolean", description: "总开关（通常由宿主统一管理）" }
    ]
  },
  {
    name: "api",
    label: "接口配置",
    order: 2,
    fields: [
      { name: "key", label: "API Key", type: "string", description: "调用的密钥" }
    ]
  }
]);
```

- `type` 可选值：`boolean` / `integer` / `number` / `string` / `array` / `object`。
- 建议所有用户可见配置文本使用简体中文。

### 首次无配置时的默认生成（推荐写法）

顶层定义 `DEFAULT_CONFIG`，检测到配置为空时 `setFile(DEFAULT_CONFIG)` 自动生成：

```javascript
var DEFAULT_CONFIG = { plugin: { enabled: true }, demo: { flag: false } };

var config = engine.config.getFile();
if (!config || Object.keys(config).length === 0) {
  engine.config.setFile(DEFAULT_CONFIG);
  console.info("配置不存在，已生成默认配置");
}
```

### onConfigUpdate(scope, config, version)

```javascript
function onConfigUpdate(scope, config, version) {
  // scope: "plugin"；config: 回写后的配置对象；version: ""
  // 典型用途：配置变化后重新注册/注销工具
}
```

> 该回调由 `setFile` 以 fire-and-forget 方式派发，入场即返回，不阻塞调用方。

***

## 完整 API 参考

> **全局约定**：
> - 所有 API 为**同步阻塞**调用，返回普通对象（非 Promise）。需要异步等待时用 `engine.sleep` 轮询。
> - 通用返回结构 `rwResult`：`{ success: bool, text?: string, error?: string, tool_calls?: [...] }`（JSON 小写字段名）。
> - 图片一律以 **base64 字符串**传递；二进制数据经 Go→JS 字符串转换会损坏，**图片校验/转 base64 必须走 `engine.image`**（Go 侧按真实字节校验）。
> - 文件路径（`engine.file` / `engine.image` / `engine.http.download`）**均相对插件 `data/` 目录**解析，配置里的 `"data/..."` 前缀需去掉（否则会双重嵌套成 `data/data/...`）。

### 6.1 时间与休眠

| API | 签名 | 返回 |
|-----|------|------|
| `engine.time.now()` | `() → int64` | 当前 Unix 时间戳（秒） |
| `engine.time.nowMs()` | `() → int64` | 当前 Unix 时间戳（毫秒） |
| `engine.sleep(ms)` | `(ms: number)` | 无返回，同步阻塞指定毫秒 |

```javascript
var now = engine.time.now();        // 秒
engine.sleep(5000);                 // 同步等待 5 秒（距离开花轮询）
function today() {
  var d = new Date(engine.time.now() * 1000);
  return d.getFullYear() + "-" + (d.getMonth() + 1) + "-" + d.getDate();
}
```

插件事件循环（goja_nodejs）自带 `setTimeout` / `setInterval` 定时器，但宿主对命令/工具的调用是**同步阻塞**的，
同步调用期间定时器回调不会被调度；需要"等待一段时间"请用 `engine.sleep`。

### 6.2 日志与调试（console）

沙箱注入全局 `console`，输出**路由到琉璃 `LoggerGeneral`**（不落本盘），带 `[console.xxx]` 前缀：

| API | 日志级别 |
|-----|----------|
| `console.log(...)` / `console.info(...)` / `console.debug(...)` | Info |
| `console.warn(...)` | Warn |
| `console.error(...)` | Error |

```javascript
console.info("插件已加载");
console.warn("网络失败，回退到备用地址");
console.error("处理出错: " + errMsg);
```

### 6.3 配置（engine.config）

| API | 签名 | 返回 |
|-----|------|------|
| `engine.config.getFile()` | `() → object` | 解析后的配置对象 |
| `engine.config.setFile(v)` | `(v: object)` | 无返回；回写 config.yaml 并触发 `onConfigUpdate` |
| `engine.config.registerSchema(sections)` | `(sections: array)` | 声明配置节/字段的中文名、类型、说明 |

### 6.4 文件（engine.file，allow-file）

所有路径相对插件 `data/` 目录解析，越界路径（`..`、绝对路径、盘符/UNC 前缀）直接拒绝。

| API | 签名 | 返回 |
|-----|------|------|
| `engine.file.read(path)` | `(path: string)` | `{success, text}`（成功）或 `{success:false, error}` |
| `engine.file.write(path, data)` | `(path, data: string)` | `{success:true}` 或 `{success:false, error}` |
| `engine.file.delete(path)` | `(path: string)` | `{success:true}` 或 `{success:false, error}` |

> ⚠️ `read` 返回的是 **`{success, text}` 对象，不是裸字符串**！取内容用 `r.text`。

```javascript
engine.file.write("daily.json", JSON.stringify(record));
var r = engine.file.read("daily.json");
var data = (r && r.success) ? JSON.parse(r.text || "{}") : {};
engine.file.delete("tmp.bin");
```

### 6.5 图片（engine.image，allow-file；下载另需 allow-network）

图片的**校验与 base64 转换在 Go 侧完成**（按真实字节检查 magic：PNG/JPEG/GIF/WebP/BMP），
不要用 `engine.file.read` 读图片再自行 base64（二进制经 JS 字符串会损坏）。

| API | 签名 | 返回 |
|-----|------|------|
| `engine.image.loadValid(path)` | `(path: string)` | `{success:true, text: base64}` 或 `{success:false, error}` |
| `engine.image.download(url, fileName)` | `(url, fileName: string)` | 下载到 `data/`，校验 magic 后 `{success:true, text: base64}` 或 `{success:false, error}` |

```javascript
function stripDataPrefix(path) { return String(path || "").replace(/^data[\/\\]/, ""); }
var b64 = (function (path) {
  var r = engine.image.loadValid(path);
  return (r && r.success && r.text) ? r.text : null;
})(stripDataPrefix("data/selfie.png"));

var dl = engine.image.download(url, "ref_1.png");  // → base64
```

### 6.6 HTTP（engine.http，allow-network）

同步阻塞，返回普通对象。

| API | 签名 | 返回 |
|-----|------|------|
| `engine.http.get(url, headers?)` | `(url: string, headers?: object)` | `{status, body}` 或 `{status:0, body:"", error}` |
| `engine.http.post(url, body, headers?)` | `(url, body: string, headers?: object)` | 同上 |
| `engine.http.download(url, savePath?)` | `(url, savePath?: string)` | 保存到 `data/`，`{success, path, size}` 或 `{success:false, error}` |

```javascript
var resp = engine.http.get("https://api.example.com/items");
if (resp.error) { console.error("请求失败: " + resp.error); }
if (resp.status !== 200) { console.error("状态码: " + resp.status); }
var data = JSON.parse(resp.body);

var resp2 = engine.http.post(endpoint, JSON.stringify(payload), { "Content-Type": "application/json", "Authorization": "Bearer " + token });
```

> 图片下载优先 `engine.image.download`（Go 侧校验 + 直接得 base64）；`engine.http.download` 不校验内容类型。

### 6.7 全局 fetch / WebSocket（allow-network）

沙箱还提供标准形态的网络能力（同为 allow-network 门控）：

**fetch**：`fetch(url, init?) → Promise<Response>`，`init` 支持 `method` / `body` / `headers` / `timeout`(秒) / `redirect:"manual"`。
`Response` 对象：`status` / `statusText` / `ok` / `url` / `redirected` / `headers` / `text()` / `json()` / `arrayBuffer()`。

```javascript
var resp = await fetch("https://api.example.com/data", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ q: "天气" }) });
var data = await resp.json();
```

**WebSocket 客户端**：`new WebSocket(url)`，属性 `readyState`，事件回调 `onopen` / `onmessage`（含 `data`）/ `onerror` / `onclose`，方法 `send(text)` / `close()`。
回调经事件循环排到插件自己的线程执行，保证 goja 线程安全。

### 6.8 网络（engine.network，allow-network）

裸 TCP/UDP/DNS（同步阻塞模型）。**套接字支持二进制收发**：`send` 接受整数数组 / Uint8Array / ArrayBuffer / 字符串，`receive` 返回字节整数数组，另有 `receiveString` 按文本读取。

| API | 签名 | 返回 |
|-----|------|------|
| `engine.network.resolveDNS(host, timeoutSec?)` | `(host: string, t?: number)` | `{success, addresses: string[]}` 或 `{success:false, error}` |
| `engine.network.resolveSRV(service, proto, host, timeoutSec?)` | `(s, p, h: string, t?: number)` | `{success, targets: [{target, port}]}` 或 `{success:false, error}` |
| `engine.network.tcpConnect(host, port, timeoutSec?)` | `(host, port: number, t?: number)` | 套接字对象或 `{success:false, error}` |
| `engine.network.udpConnect(host, port, timeoutSec?)` | 同上 | 已连接 UDP 套接字 |
| `engine.network.udpListen(host, port)` | `(host, port: number)` | UDP 监听套接字 |

套接字对象方法：`sock.send(data)` → `{success}`；`sock.receive(t?)` → `{success, data: number[]}`（默认超时 5 秒）；`sock.receiveString(t?)` → `{success, data: string}`；`sock.sendTo(host, port, data)`（UDP 未连接场景）；`sock.close()` → `{success}`。

### 6.9 LLM（engine.llm，allow-agent）—— 模型垄断

> 🔒 **模型垄断语义**：`engine.llm` 保留 YaraFlow 的完整方法签名，但**后台一律忽略插件的模型选择**。
> 无论传 `baseUrl` / `apiKey` / `model` / `taskType` / `selectionStrategy` / `model_list`，实际请求、返回的配置、记账、熔断
> **都以琉璃 `lunar_config.json` 的 `agent` 字段为准**：
> - 对话/多模态 → `agent.multimodal_{model,url,key}`
> - 嵌入 → `agent.embedding_{model,url,key}`
>
> 插件层对自己最终使用哪个模型没有发言权，这由琉璃统一垄断。

| API | 签名 | 返回 |
|-----|------|------|
| `engine.llm.chat(messages, opts?)` | `(messages: array, opts?: object)` | `{success:true, text}`（含工具调用时带 `tool_calls`）或 `{success:false, error}` |
| `engine.llm.embed(input, opts?)` | `(input: string\|string[], opts?: object)` | 单条 `{success:true, embedding: number[]}`；多条 `{success:true, embeddings: number[][]}` |
| `engine.llm.chatWithTools(opts)` | `(opts: object)` | 等价 `chat`，`opts.messages` 必填 |
| `engine.llm.chatWithConfig(opts)` | `(opts: object)` | 等价 `chat`，`opts.messages` 必填 |
| `engine.llm.getConfig(taskType)` | `(taskType: string)` | 某任务的模型池配置概要（恒为 agent 单模型） |
| `engine.llm.getAllConfigs()` | `()` | 全部任务模型池配置概要 |
| `engine.llm.listTasks()` | `()` | `["replyer","planner","tool_use","vlm","voice","embedding"]` |
| `engine.llm.getAvailableModels()` | `()` | `[agent 对话模型名]` |
| `engine.llm.getCallRecords(n)` | `(n: number)` | 最近 n 条调用记录（agent 模型） |
| `engine.llm.getCircuitBreakers()` | `()` | 熔断器状态（agent 单模型） |

`chat` 的 `opts` 中**只有采样/提示参数生效**：`temperature` / `max_tokens` / `system`（前置为 system 消息）/ `tools`（透传函数定义）。
`baseUrl` / `apiKey` / `model` / `taskType` / `selectionStrategy` / `model_list` 均被忽略（模型垄断）。

```javascript
var resp = engine.llm.chat([
  { role: "system", content: systemPrompt },
  { role: "user", content: userPrompt }
], { temperature: 0.7 });
var content = (resp && resp.success && resp.text) ? resp.text : "";

var emb = engine.llm.embed("要嵌入的文本");       // → { success:true, embedding:[...] }
```

### 6.10 发送（engine.send，allow-send）

| API | 签名 | 返回 |
|-----|------|------|
| `engine.send.text(target, text)` | `(target: string, text: string)` | `{success:true}` 或 `{success:false, error}` |
| `engine.send.image(target, b64)` | `(target: string, b64: string)` | 同上（b64 为图片 base64） |
| `engine.send.hybrid(target, segs)` | `(target: string, segs: array)` | 同上（图文混合分段） |

- `target` 为**群 ID**，来自工具 handler 的 `context.groupId`。
- hybrid 分段约定：图片段 `{ type:"image", content:b64 }`（兼容 `{type:"image", image:b64}` 写法），文本段 `{ type:"text", content:"..." }`。
- 宿主把 `engine.send` 组装成平台消息透传发送通道（`ltp9.SetSendInvoker`）；通道未注入时返回 `{success:false, error:"未注入发送通道"}`。
- 经验：单条 hybrid 消息不要超过 8 张图。

### 6.11 平台（engine.platform，allow-send）

| API | 签名 | 返回 |
|-----|------|------|
| `engine.platform.getName()` | `()` | 平台名（如 `"qq"`） |
| `engine.platform.getGroupId()` | `()` | 当前群 ID（宿主未注入时为空串） |
| `engine.platform.lookupUser(groupId, name)` | `(groupId, name: string)` | 按昵称解析用户 ID |
| `engine.platform.sendCommand(command, args)` | `(command: string, args: object)` | `{success:true}` 或 `{success:false, error}` |

`sendCommand` 支持的平台命令（由宿主 `SetPlatformResolver` 委托平台驱动执行）：`GROUP_BAN`（禁言，`{ group_id, qq_id, duration }`）、`DELETE_MSG`（撤回，`{ group_id, message_id }`）。
平台解析器未注入时统一返回 `{success:false, error:"未接入平台（宿主未注入 platformResolver）"}`（`getName` 例外）。

### 6.12 工具（engine.tool，allow-agent）

```javascript
engine.tool.register("get_weather", {
  description: "查询指定城市实时天气。当用户问天气时调用。",
  parameters: [
    { name: "city", type: "string", description: "城市名称，示例: 北京", required: true }
  ]
}, function(params, context) {
  // params:  工具参数对象；context: 宿主注入的调用上下文（见下）
  var city = params.city;
  return getWeather(city);   // 返回对象/字符串，宿主字符串化后作为工具结果回给模型
});
```

**工具定义只读取 `description` + `parameters` 两个字段**；`parameters` 为数组，元素建议 `{ name, type, description, required }`（原样透传给 LLM）。

**context 字段清单**：

| 字段 | 类型 | 说明 |
|------|------|------|
| `sessionId` | string | 会话 ID |
| `platform` | string | 平台名 |
| `groupId` | string | 群 ID（发送目标） |
| `userId` / `senderName` | string | 发送者 ID / 昵称 |
| `messageId` / `replyMessageId` | string | 当前消息 ID / 被回复消息 ID |
| `messageIDMapping` | object | 短 ID → 真实消息 ID 映射 |
| `image_urls` | string[] | 会话内图片 URL 列表（`[图片1]` → `image_urls[0]`） |
| `image_descriptions` | string[] | 图片描述列表（`[图片N：描述]`） |
| `isGroup` | bool | 是否群聊 |

**其他要点**：工具执行是**同步**的；同名注册**覆盖**；`engine.tool.getDefinitions()` 返回已注册定义数组；宿主侧经 `ltp9.PluginToolDefs()` 汇总后交给 LLM 工具系统调用。

### 6.13 指令（engine.command，常驻）

```javascript
engine.command.register("qy", "^/qy\\s+(?P<target>\\S+)\\s+(?P<duration>\\d+)$", function(match, context) {
  // match: 捕获组数组（match[0] 完整匹配，match[1..n] 分组）；命名分组可经 match.groups.target 访问
  var target = match.groups.target;
  var duration = match.groups.duration;
  return executeMute(target, duration, context.groupId);
}, { aliases: ["qy1", "禁言"] });
```

| 参数 | 说明 |
|------|------|
| `name` | 指令名（唯一），同时作为精确匹配的触发词 |
| `pattern` | 正则模式；支持 `/regex/` 或裸正则；留空则仅按指令名/别名精确匹配 |
| `handler` | `(match, context)` 处理器，返回结果文本 |
| `options.aliases` | 别名数组，与指令名等同参与精确匹配 |

**触发路径**：宿主命令处理器匹配到指令后，经 `ExecuteCommand` → `ltp9.Command(pluginID, text, context)` 派发到对应插件；引擎先精确匹配指令名/别名，再按注册正则逐一匹配首条命中。

### 6.14 事件（engine.event，常驻）

```javascript
// 订阅：返回订阅 id（供 unsubscribe 退订）；priority 0 最高，越小越先执行，可省略
var id = engine.event.subscribe("weather.query", function(event) {
  // event: { type: topic, payload: 载荷 }
  var p = event.payload || {};
  return getWeather(p.city);   // 返回给发起方（宿主 Emit 场景）或供拦截/改写使用
}, 0);

engine.event.unsubscribe("weather.query", id);

// 发布：异步分发给其它插件的订阅器 + 转发宿主 outbound；跳过发起插件自身订阅器（避免自锁/自循环）
engine.event.publish("my.topic", { foo: 1 });
```

**回调返回值可控制事件流**（按优先级排序后逐订阅器派发）：

| 返回值 | 效果 |
|--------|------|
| `{ intercept: true }` | 拦截：事件已被消费，短路**该插件内**后续订阅器（不阻断其它插件） |
| `{ modifiedData: X }` | 改写：用 X 替换事件负载，供后续订阅器读取（含其余插件） |
| `{ cancel: true }` | 撤回：停止派发，事件不再向后续插件扩散（全局停止） |
| `{ return: X }` | 回传业务结果给事件发起方（宿主 `EmitResult.Return` 消费） |

**优先级**：`priority` 0 最高，越小越先；未设置优先级的订阅排在所有设置者之后（保持时间顺序）。

### 6.15 广播与前端事件（engine.signal / engine.frontEvent，allow-signal）

```javascript
engine.signal.all({ topic: "weather.query", result: result });          // 向所有插件 frontEvent + 宿主 outbound 广播
engine.signal.target("com.example.b", payload);                          // 只发给指定插件
var sid = engine.frontEvent.signal.subscribe(function(payload) {         // 接收端订阅
  console.info("收到广播: " + JSON.stringify(payload));
});
engine.frontEvent.signal.unsubscribe(sid);
```

### 6.16 跨包调用（engine.export / engine.call / engine.agent）

```javascript
// 被调用方导出（常驻能力）
engine.export("translate", function(text, toLang) { return "翻译结果"; });

// 调用方（allow-call）
var out = engine.call("com.example.translator").run("translate", ["你好", "en"]);
// 目标插件不存在或未导出 → "xxx 包拒绝响应"

// 前端智能体（allow-agent）：把 appID + 自然语言指令转交 Mini-LTP / Node-LTP 包
var text = engine.agent("com.example.agent").run("帮我查一下今天的天气");
// 未注入 → "xxx 包拒绝响应"
```

### 6.17 编解码（engine.encoding，常驻）

| API | 签名 | 返回 |
|-----|------|------|
| `engine.encoding.base64Encode(data)` | `(data: string) → string` | base64 编码 |
| `engine.encoding.base64Decode(s)` | `(s: string) → string` | base64 解码（失败抛错） |
| `engine.encoding.hexEncode(data)` / `hexDecode(s)` | 同上 | hex 编解码 |
| `engine.encoding.urlEncode(s)` / `urlDecode(s)` | 同上 | URL 编解码 |
| `engine.encoding.utf8Encode(s)` / `utf8Decode(v)` | `utf8Encode → number[]`；`utf8Decode(int[]\|Uint8Array\|ArrayBuffer\|string) → string` | UTF-8 编解码（供二进制协议打包字符串） |

### 6.18 加解密（engine.crypto / engine.encoder / engine.decoder，allow-certificate）

| API | 签名 | 返回 |
|-----|------|------|
| `engine.crypto.signJWT(claims, secret, algorithm, kid?)` | `(claims: object, secret: string, algorithm: string, kid?: string)` | JWT；algorithm 支持 `HS256`（默认）/ `EdDSA` / `none` |
| `engine.crypto.generateJWT(claims, privKey, kid?)` | 同上 | 用 Ed25519 私钥生成 EdDSA JWT |
| `engine.crypto.md5(data)` / `sha1(data)` / `sha256(data)` | `(data: string) → string` | 小写 hex 摘要 |
| `engine.crypto.hmacSha1(key, data)` / `hmacSha256(key, data)` | `(key, data: string) → string` | 小写 hex HMAC |
| `engine.crypto.ed25519Sign(privKey, data)` | `(priv, data: string) → string` | Ed25519 签名（base64url） |
| `engine.encoder(key, content)` | `(key, content: string) → string` | **复用琉璃 LunarDecoder** 加密 |
| `engine.decoder(key, cipher)` | `(key, cipher: string) → string` | **复用琉璃 LunarDecoder** 解密 |

```javascript
var jwtToken = engine.crypto.signJWT({ sub: pid, iat: now, exp: now + 900 }, privateKey, "EdDSA", keyID);
```

### 6.19 记忆库 / 表情包（allow-memory，桥接琉璃记忆库）

`engine.memory` / `engine.emoji` **桥接到琉璃 `FileManager/module` 的记忆库**（不再是"未接入"占位）：

| API | 说明 |
|-----|------|
| `engine.memory.store(obj)` | 写入记忆库（`ltp9_memory` 集合；记忆库未初始化时返回"未接入"错误） |
| `engine.memory.search(obj)` | 语义检索记忆库 |
| `engine.emoji.search(query, opts?)` | 从记忆库 `stickers` 集合检索表情（已接入） |
| `engine.emoji.random(query)` | 从 `stickers` 集合随机返回单张（已接入） |
| `engine.emoji.store(image)` | **未接入**（仅支持查询/随机），返回"表情包存储未接入"错误 |

```javascript
engine.memory.store({ content: "一段需要被记下的内容" });
var hits = engine.memory.search({ query: "关键词", limit: 5 });
var sticker = engine.emoji.random("开心");  // → { success, text: base64 }
```

### 6.20 数据库（engine.database，allow-database，未接入）

```javascript
// 保留签名与权限门控，但当前宿主未注入真实实现，统一返回未接入：
engine.database.query(sql, params)  // → {success:false, error:"未接入数据库(host 未注入)"}
engine.database.exec(sql, params)   // → 同上
```

> 开发新插件请勿依赖数据库能力；宿主接入后本文档会同步更新。

### 6.21 WebSocket 服务端（engine.ws，allow-socket）

| API | 签名 | 返回 |
|-----|------|------|
| `engine.ws.expose(path, handler)` | `(path: string, handler: (msg: string) => string)` | `{success:true, path: 访问地址}` 或 `{success:false, error}` |
| `engine.ws.publish(data)` | `(data: string)` | `{success:true}` 或 `{success:false, error}` |

```javascript
var r = engine.ws.expose("/stream", function(msg) {
  console.info("WS 收到: " + msg);
  return "pong";
});
engine.ws.publish(JSON.stringify({ status: "ok" }));
```

宿主未注入 WebSocket 传输（`ltp9.SetWsServer`）时返回 `{success:false, error:"未注入 WebSocket 传输"}`。

### 6.22 异步子任务（engine.async，常驻）

```javascript
var r = engine.async.run(function(task) {
  // taskFn 仍在插件事件循环线程内执行（复用 callFn），运行期间占用循环
  engine.async.reportProgress(task.id, "处理中 50%");
  return "完成";
}, { timeout: 60000, data: { key: "value" } });
// → { success:true, text:"任务id" }

engine.async.getStatus(1);         // → { success:true, taskId:1, status:"running"|"done"|"timeout"|"error", progress }
engine.async.reportProgress(1, 80); // → { success:true }
engine.async.list();                // → [{ taskId, status, progress }]
```

任务状态看门狗：`timeout` 毫秒后仍 running 的任务被标记 `timeout`（仅标记，不强制中断阻塞中的任务）；已终结记录保留 1 小时自动清理。

***

## 生命周期

LTP9 插件的执行模型：

1. **顶层执行**（加载时）：引擎读取 `config.yaml` → 校验 `permissions.key` → 在插件事件循环内绑定 `engine.*` 并整段执行 `execute.js`。
   组件注册（`engine.tool.register` / `engine.command.register` / `engine.event.subscribe` / `engine.export`）都在顶层完成。
   **顶层执行有 30 秒超时**：死循环脚本不会无限阻塞加载方，日志输出 `execute.js 顶层执行超过 30s 未完成（可能存在死循环），加载中止`。
2. **onLoad()**：顶层执行完成后调用（若定义了该函数）；异常仅告警不阻断加载。
3. **onConfigUpdate(scope, config, version)**：`engine.config.setFile` 回写后触发（fire-and-forget）。
4. **onUnload()**：卸载时调用，随后取消全部定时器并终止插件事件循环。

```javascript
function onLoad() { console.info("已加载"); }
function onConfigUpdate(scope, config, version) { console.info("配置已更新"); }
function onUnload() { console.info("已卸载"); }
```

**隔离性**：每个插件拥有**独立的 goja VM + 独立事件循环**，全局完全隔离（不共享任何 JS 全局状态）；跨插件通信只能走
`engine.call`（导出函数）、`engine.event.publish` / `engine.signal`（广播）、或宿主中转。

**热更新**：引擎每 3 秒对账一次包目录（比对 `metadata.json` / `execute.js` / `permissions.key` / `config.yaml` 的 mtime+size），目录变化自动重载/卸载；加载失败的插件在目录变化后自动重试。

***

## 工具 (Tool) 开发

工具是 LTP9 插件的主入口：工具定义经宿主汇总进主程序工具系统，LLM 在对话中自主调用。

```javascript
engine.tool.register("get_weather", {
  description: "查询指定城市实时天气和未来2天预报。支持中文城市名。",
  parameters: [
    { name: "city", type: "string", description: "城市名称，示例: 北京、上海", required: true }
  ]
}, function(params, context) {
  try {
    var result = getWeather(params.city);
    return result.error ? { error: result.error } : result;
  } catch (e) {
    return { error: "天气查询异常: " + e.message };
  }
});
```

**工具开发要点**：
1. `description` 写清楚"何时调用"（"当用户要求 xxx 时调用"）；`parameters` 里给示例值。
2. handler 第一行取 `context.groupId` 作为发送目标；没有 groupId 时不要发送，返回可读错误文本。
3. 参考图：`context.image_urls` → `engine.image.download(url, "ref_N.png")` 得到 base64；`context.image_descriptions[N-1]` 即 `[图片N：描述]` 的描述，可直接拼进提示词。
4. 失败一律返回可读文本（带上原因），不要静默；用 `console.info/error` 记录关键路径。
5. 涉及耗时轮询用 `engine.sleep` 同步循环，注意控制总时长。

***

## 指令 (Command) 开发

```javascript
engine.command.register("qy", "^/qy\\s+(?P<target>\\S+)\\s+(?P<duration>\\d+)(?:\\s+(?P<reason>.+))?$", function(match, context) {
  var target = match.groups.target;       // 命名分组
  var duration = match.groups.duration;
  var reason = match.groups.reason || "管理员操作";
  var userId = context ? context.userId : "";
  if (!checkPermission(userId)) {
    try { engine.send.text(context.groupId, "无权限"); } catch (e) {}
    return "";
  }
  return executeMute(target, duration, reason, context.groupId);
});
```

**指令开发要点**：`pattern` 支持 `/正则/` 或裸正则；命名分组 `(?P<name>...)` 经 `match.groups.<name>` 读取，未命名分组用 `match[1]`；`name` 同时是精确触发词（含别名）。

***

## 事件与插件间通信

### 事件总线（engine.event）

```javascript
// 发布方（A 插件）
engine.event.publish("weather.query", { city: "北京" });

// 订阅方（B 插件）
engine.event.subscribe("weather.query", function(event) {
  return getWeather(event.payload.city);
}, 0);
```

`publish` 异步派发（fire-and-forget）：派发给**其它**插件的订阅器 + 转发宿主 outbound；跳过发起插件自身订阅器，避免自锁/自循环。并发派发上限 256，满载时丢弃本次派发并告警（防插件互激 re-publish 死循环）。

### 广播（engine.signal，allow-signal）

`engine.signal.all(payload)` 向所有插件的 `frontEvent.signal` 订阅器 + 宿主 outbound 广播；`engine.signal.target(id, payload)` 只发指定插件。

### 跨包函数调用（engine.export / engine.call，allow-call）

```javascript
// A 插件导出
engine.export("fetchSummary", function(url) { return "摘要"; });
// B 插件调用（需 allow-call）
var out = engine.call("com.example.a").run("fetchSummary", ["https://x"]);
```

### 与消息管线的交互（宿主桥接）

宿主经 `ltp9.Emit(topic, payload, requestID)` 把消息管线关键节点转发给引擎，插件可订阅管线事件：
- 订阅 `chat.receive.before_process` / `chat.receive.after_process`，返回 `{cancel:true}` 撤回（不回复）、`{modifiedData:X}` 改写负载；
- `engine.event.publish` 只派发给其它插件与宿主 outbound，不进入消息管线（管线事件由宿主单独桥接）。
- 具体哪些管线事件已桥接，取决于宿主的接入（crystal_astral 主程序）——请以宿主实际调用 `ltp9.Emit` 的主题为准。

***

## 调试与日志

### 日志输出

- `console.log/info/debug` → Info 级；`console.warn` → Warn 级；`console.error` → Error 级。
- 日志经**琉璃 `LoggerGeneral`** 输出（不落本盘），建议带插件名前缀（如 `console.info("我的插件: ...")`）。

### 加载失败排查

- 插件加载失败不会让程序崩溃，日志输出 `LTP9 插件加载失败`（含 `execute.js 执行失败` / `读取 execute.js 失败` / 30 秒超时提示等）。
- `onLoad` 异常仅告警：`插件 onLoad 执行异常`。
- 权限问题：`插件权限密钥解码失败，已拒绝全部权限` / `插件缺少权限密钥文件`；此时 `engine.send` 等为 `undefined`，调用即 `TypeError`。
- 配置问题：`config.yaml 解析失败`（仅告警，配置对象为空）。

***

## 从 LTP3（yara.*）迁移指南

LTP3（`yara.*` API + `plugin.json`）与 LTP9（`engine.*` API + `metadata.json`）差异较大。

### 1. 包结构迁移

| LTP3 | LTP9 |
|------|------|
| `plugin.json` | `metadata.json`（`tags: ["LTP9"]`），**无 plugin.json** |
| `index.js` | `execute.js`（固定文件名） |
| `config.yaml` + 默认配置节 | `config.yaml` + `engine.config.setFile(DEFAULT_CONFIG)` 自动生成 |
| （无） | `permissions.key`（权限密钥，随插件包由作者提供） |
| （无） | `data/` 运行时数据目录（所有文件路径相对它解析） |

### 2. API 映射表

| LTP3（yara.*） | LTP9（engine.*） | 差异要点 |
|----------------|------------------|----------|
| `yara.config.getFile/setFile` | `engine.config.getFile/setFile` | 一致；`setFile` 只收对象 |
| `yara.logger.info/warn/error/debug` | `console.info/warn/error/debug` | 沙箱全局 console |
| `yara.file.read(path)` | `engine.file.read(path)` | **返回 {success, text} 对象**，取内容用 `.text` |
| `yara.model.chat*` | `engine.llm.chat*` | 返回 `{success, text}`；**模型垄断**（忽略模型参数） |
| `yara.http.get/post` | `engine.http.get/post` | 返回 `{status, body}` 或含 `error` |
| `yara.send.text/image/hybrid` | `engine.send.text/image/hybrid` | target 即群 ID（context.groupId） |
| `yara.tool.register` | `engine.tool.register(name, {description,parameters}, handler)` | handler `(params, context)` |
| `yara.command.register` | `engine.command.register(name, pattern, handler, {aliases})` | 支持 `/regex/` 或裸正则 |
| `yara.hook.register` | `engine.event.subscribe` | 消息管线事件由宿主桥接（订阅 `chat.receive.before_process` 等并返回 `{cancel:true}`/`{modifiedData:X}`） |
| `yara.event.subscribe` | `engine.event.subscribe(topic, cb, priority)` | 回调收 `{type, payload}`；支持 intercept/modifiedData/cancel/return |
| `yara.encoding.*` | `engine.encoding.*` | 还有 utf8Encode/Decode |
| `yara.crypto.*` | `engine.crypto.*` | 一致 |
| `yara.image.loadValid/download` | `engine.image.loadValid/download` | 去掉配置里的 `data/` 前缀 |
| `yara.database.*` | `engine.database.*` | 未接入占位 |
| `yara.memory.*` / `yara.emoji.*` | `engine.memory.*` / `engine.emoji.*` | 桥接琉璃记忆库（emoji.store 未接入） |

### 3. 典型迁移步骤

1. 建目录：`metadata.json`（加 `"LTP9"` 标签）+ 把 `index.js` 改名/复制为 `execute.js`；
2. 全局替换：`yara.logger` → `console`、`yara.config` → `engine.config`、`yara.send` → `engine.send`、`yara.tool` → `engine.tool`、`yara.command` → `engine.command`、`yara.event` → `engine.event`；
3. 逐处调整返回解包：`engine.file.read` 取 `.text`；`engine.http.get/post` 判 `resp.error` 再 `resp.body`；`engine.llm.chat` 判 `resp.success` 再 `resp.text`；
4. 图片路径全部改相对 `data/`：配置里的 `"data/xxx"` 先 `stripDataPrefix` 再去 `engine.image.loadValid` / `download`；
5. 移除 `yara.hook` 等不存在的能力：hook 类插件改用订阅管线事件（`engine.event.subscribe("chat.receive.before_process", cb, 0)` + 返回 `{cancel:true}`）替代；
6. 向插件作者获取随插件包分发的 `permissions.key`（缺密钥/失效时找作者重新签发）；
7. 本地测试（可开 `SetDeveloperMode(true)`），确认加载日志无告警。

***

## 已知限制

1. **改 execute.js 忘重新签发 key**：密钥与代码强绑定，代码一变旧密钥失效 → 无任何权限 → `engine.send` 等为 `undefined`。加载不报错，只看日志告警。需 **联系插件作者重新签发** `permissions.key`。
2. **config.setFile 只收对象**：传 YAML 字符串会被当作对象处理导致回写异常；要写 YAML 文本用 `engine.file.write("config.yaml", ...)`。
3. **engine.file.read 返回对象**：`{success, text}`，忘了取 `.text` 会得到 `[object Object]`。
4. **二进制经 JS 字符串损坏**：读图片/下载图片一律走 `engine.image`（Go 侧按真实字节校验并转 base64）；用 `engine.file.read` 读图片再 base64 会失败。
5. **模型垄断**：`engine.llm` 忽略插件的 `baseUrl`/`apiKey`/`model`/`taskType`/`model_list`，一律强制琉璃 agent 字段模型。插件作者不要试图自定义模型。
6. **database 未接入**：`engine.database` 返回"未接入"占位错误；`engine.emoji.store` 未接入（仅 search/random）。
7. **事件总线与消息管线隔离**：`engine.event.publish` 只派发给其它插件与宿主 outbound，不进入消息管线；管线事件由宿主单独桥接（以宿主接入为准）。
8. **顶层执行 30 秒超时**：execute.js 顶层死循环会导致加载中止。
9. **同步模型**：命令/工具/事件回调都是同步执行的；`setTimeout` 回调在同步调用期间不会被调度，等待请用 `engine.sleep`。

***

## 常见问题

**Q1：插件加载了，但调用 `engine.send.text` 报 `TypeError: engine.send is undefined`？**
A：权限未授予或密钥失效。检查：① 插件包是否含有效 `permissions.key`（由作者生成）；② 是否改过 execute.js 忘了让作者重新生成 key；③ 开发模式 `SetDeveloperMode(true)` 可临时跳过校验。

**Q2：`engine.file.read` 返回 `[object Object]`？**
A：`read` 返回 `{success, text}` 对象，需 `String(r.text || "")` 取内容。

**Q3：图片路径变成 `data/data/...`？**
A：配置里写了 `data/xxx` 前缀。LTP9 所有 `engine.file`/`engine.image` 路径已相对 `data/` 解析，需 `stripDataPrefix` 去掉前缀。

**Q4：图片下载返回 `{success:false, error:"下载内容非有效图片（链接可能过期）"}`？**
A：`engine.image.download` 在 Go 侧校验了图片 magic。链接失效、被平台移除或返回了 JSON/HTML 错误体会这样。纯二进制下载用 `engine.http.download`（不校验类型）。

**Q5：`engine.config.setFile(yaml字符串)` 没有生效？**
A：`setFile` 只接收对象。传 YAML 文本请用 `engine.file.write("config.yaml", 文本)`。

**Q6：`engine.llm.chat` 返回 `{success:false, error:"缺少 messages"}`？**
A：messages 必须是数组，元素为 `{role, content}`。`chatWithTools`/`chatWithConfig` 走 `opts.messages`。

**Q7：我传了 `buildUrl`/`model` 给 `engine.llm.chat` 却不生效？**
A：这是**模型垄断**预期行为 —— 后台强制使用琉璃 agent 字段模型，插件无法自选模型。

**Q8：改配置后工具没变化？**
A：`engine.tool.register` 按同名覆盖，`onConfigUpdate` 里重新注册即可；但**注销**已注册工具需要重载插件（引擎无 unregister API）。

**Q9：想给群发多条图文（>8 张图）？**
A：拆成多条 `engine.send.hybrid`，每条不超过 8 张图。

**Q10：插件之间的时间/日期格式化？**
A：引擎只提供 `engine.time.now()`（秒）/`nowMs()`（毫秒），用 `new Date(...)` 自行格式化。

**Q11：怎么在命令/工具里拿到发送者信息？**
A：工具 handler 第二参数 `context` 含 `userId`/`senderName`/`groupId`/`messageId`/`replyMessageId`/`messageIDMapping` 等；命令 handler 的 `context` 含 `platform`/`groupId`/`match`。

**Q12：插件之间怎么互通？**
A：`engine.export` 导出 + `engine.call(id).run(fnName, args)`（需 allow-call）；或 `engine.event.publish` 事件总线 / `engine.signal` 广播。

***

## 附录：API 速查表

### engine.* 全量签名

| 分组 | API | 权限 |
|------|-----|------|
| 配置 | `config.getFile()`、`config.setFile(obj)`、`config.registerSchema(sections)` | 常驻 |
| 文件 | `file.read(path) → {success,text}`、`file.write(path,data)`、`file.delete(path)` | allow-file |
| 图片 | `image.loadValid(path) → {success,text:base64}`、`image.download(url,fileName)` | allow-file（下载另需 allow-network） |
| HTTP | `http.get(url,headers?) → {status,body}`、`http.post(url,body,headers?)`、`http.download(url,savePath?)` | allow-network |
| 网络 | `network.resolveDNS/resolveSRV/tcpConnect/udpConnect/udpListen`；sock: `send/receive/receiveString/sendTo/close` | allow-network |
| fetch | `fetch(url, init?) → Promise<Response>`、`new WebSocket(url)` | allow-network |
| LLM | `llm.chat/embed/chatWithTools/chatWithConfig/getConfig/getAllConfigs/listTasks/getAvailableModels/getCallRecords/getCircuitBreakers` | allow-agent（**模型垄断**） |
| 工具 | `tool.register(name, {description,parameters}, handler)`、`tool.getDefinitions()` | allow-agent |
| 发送 | `send.text(target,text)`、`send.image(target,b64)`、`send.hybrid(target,segs)` | allow-send |
| 平台 | `platform.getName()`、`platform.getGroupId()`、`platform.lookupUser(groupId,name)`、`platform.sendCommand(command,args)` | allow-send |
| 命令 | `command.register(name, pattern, handler, {aliases?})` | 常驻 |
| 事件 | `event.subscribe(topic, cb, priority?) → id`、`event.unsubscribe(topic, id)`、`event.publish(topic, payload)` | 常驻 |
| 信号 | `signal.all(payload)`、`signal.target(id, payload)`、`frontEvent.signal.subscribe/unsubscribe` | allow-signal |
| 跨包 | `export(name, fn)`、`call(id) → {run(fnName, args)}`、`agent(id) → {run(text)}` | export 常驻；call allow-call；agent allow-agent |
| 时间 | `time.now()`、`time.nowMs()`、`sleep(ms)` | 常驻 |
| 编解码 | `encoding.base64Encode/Decode`、`hexEncode/Decode`、`urlEncode/Decode`、`utf8Encode/Decode` | 常驻 |
| 加解密 | `crypto.signJWT/generateJWT/md5/sha1/sha256/hmacSha1/hmacSha256/ed25519Sign`；`encoder(key,content)`、`decoder(key,cipher)` | allow-certificate |
| WS 服务端 | `ws.expose(path, handler) → {success,path}`、`ws.publish(data)` | allow-socket |
| 异步 | `async.run(taskFn, {timeout?,data?}) → {success,text:taskId}`、`async.reportProgress/getStatus/list` | 常驻 |
| 记忆 | `memory.store(obj)`、`memory.search(obj)` | allow-memory（桥接琉璃记忆库） |
| 表情 | `emoji.search(query,opts?)`、`emoji.random(query)`（已接入）、`emoji.store(image)`（未接入） | allow-memory |
| 数据库 | `database.query(sql, params?)`、`database.exec(sql, params?)` | allow-database（未接入） |
| 日志 | `console.log/info/debug/warn/error(...)` | 常驻 |
| 生命周期 | `onLoad()`、`onUnload()`、`onConfigUpdate(scope, config, version)` | 定义即生效 |

### 通用返回结构

```jsonc
// rwResult（多数 engine.* 的统一返回）
{ "success": true,  "text": "...", "tool_calls": [...] }   // 成功
{ "success": false, "error": "原因" }                      // 失败

// engine.http
{ "status": 200, "body": "..." }                            // 成功
{ "status": 0, "body": "", "error": "..." }                 // 失败

// 工具调用 context
{ "sessionId": "...", "platform": "qq", "groupId": "...", "userId": "...",
  "senderName": "...", "messageId": "...", "replyMessageId": "...",
  "messageIDMapping": {}, "image_urls": [], "image_descriptions": [], "isGroup": true }
```

***

> 本文档与 `crystal_astral/engine/9.0-pro`（package `ltp9`）源码同步维护。API 行为如有出入，以源码为准。
> 类型补全见同目录 `engine.d.ts`。