# LTP3 插件层开发文档

> LTP3（YaraFlow）是一套与 LTPX/AtoA 完全不同的协议与架构。LTP3 插件以「扩展包」的形式放进 `local_data/package/<包名>/`，由引擎层（crystal_astral 内的 `agent/YaraLTP`）为每个插件启动**独立的 goja 沙箱**执行。插件**不走 AtoA / LTPX**：本协议的 `metadata.json` 不含 `tools` 字段（那是其它协议的能力），只负责识别与前端展示。

---

## 一、包结构（LTP3 最小约定）

```
local_data/package/com.yaraflow.weather/
├── metadata.json    # 识别标签（含 LTP3 即被引擎加载）+ 前端展示页（不定义工具）
├── index.js         # 插件主逻辑（引擎唯一执行入口）
└── config.yaml      # 插件唯一配置文件（仅支持 YAML，忽略其它格式）
```

其它文档（README.md、index.html 等）随插件需要，用于前端可视化展示，引擎不读取。

### 1.1 metadata.json

`metadata.json` 仅用于两件事：
1. **识别** —— `tags` 数组包含 `"LTP3"` 时，引擎才会把该包当插件加载；
2. **前端展示** —— 供琉璃管理页展示包详情。

> 注意：本协议版本的 `metadata.json` **没有 `tools` 字段**（`tools` 属于 AtoA/LTPX 协议），LTP3 不借它定义任何工具。

```json
{
  "id": "com.yaraflow.weather",
  "title": "天气助手",
  "description": "查询指定城市实时天气",
  "tags": ["LTP3"],
  "icon": "icon.webp",
  "url": "/file/read/package/com.yaraflow.weather/index.html"
}
```

| 字段 | 必需 | 说明 |
|---|---|---|
| `id` | 是 | 插件唯一 ID（反向域名风格），引擎按此 ID 作为沙箱标识 |
| `title` | 否 | 展示名 |
| `tags` | 是 | 必须含 `LTP3` 才被识别为插件 |

### 1.2 index.js

插件主逻辑，在加载时被引擎一次性执行（顶层注册订阅/指令/工具等）。可选的 `onLoad` / `onUnload` / `onConfigUpdate` 生命周期函数在对应时机被调用。

### 1.3 config.yaml

插件唯一的配置文件。引擎用**内置 YAML 子集解析器**解析（支持注释、键值、缩进嵌套、序列、内联数组/对象、引号）。前端不需要 schema —— 配置由插件自己通过 `yara.config.getFile()/setFile()` 读写，或由引擎在变更时调用 `onConfigUpdate`。

---

## 二、生命周期与沙箱约束

### 2.1 生命周期

| 时机 | 触发 |
|---|---|
| 插件加载 | 执行 `index.js` 顶层代码 → 调用 `onLoad()` |
| 引擎广播 | 加载完成广播 `ltp3/lifecycle: ON_START`；卸载广播 `ON_STOP` 给所有 ws 客户端 |
| 配置更新 | `config.setFile()` 成功后调用 `onConfigUpdate("plugin", config, "1.0.0")` |
| 卸载/移除 | 调用 `onUnload()` 后释放该插件 goja 虚拟机、清空注册表 |

### 2.2 沙箱约束（重要）

- **独立沙箱**：每个插件一个独立 goja VM，全局作用域互不可见；插件之间仅能通过 `yara.api.call` 跨插件调用（`插件ID.方法名`）。
- **禁用浏览器/Node API**：`require`、`import`、`fetch`、`setTimeout`、`setInterval`、`document`、`window`、`process`、`globalThis` 均不存在。请用 `yara.http`（网络）、`yara.time.sleep`（同步延时）。
- **串行执行**：同一插件的所有 JS 执行严格串行（引擎持插件互斥锁），请勿在同步循环里长阻塞；耗时工作用 `yara.async.run`（引擎负责入队、不会打断同插件其它调用，但会等待其结束）。
- **超时**：协议文档定义 30s 看门狗；当前实现未启用硬性打断，仍建议不必要的长循环。
- **路径隔离**：`yara.file` 只能访问插件目录（`/` 根）与 `data/` 运行时目录，越界（路径穿越）会被拒绝。
- **网络安全**：`yara.http` / `yara.network` 禁止访问本地/内网地址（SSRF 防护）。`yara.platform.sendCommand` 因真实平台在客户端侧，当前返回 `{ success:false, error:"未接入" }`；`yara.emoji` 因无表情库后端，返回空结构。

---

## 三、注册组件

### 3.1 事件订阅 / 事件处理器

```javascript
// 事件订阅（任意事件名）
yara.event.subscribe(YaraEvents.ON_MESSAGE, function (ev) {
  yara.logger.info("收到消息：" + ev.content);
});

// 或事件处理器（带权重/拦截标志）
yara.eventHandler.register("onMessage", "ON_MESSAGE", function (ev) {
  return { handled: true };
}, { weight: 1 });
```

### 3.2 Hook（消息处理链拦截/观察）

```javascript
// 用法1: (type, handler, options)
yara.hook.register(YaraHooks.CHAT_RECEIVE_BEFORE_PROCESS, function (event) {
  var msg = event.message;
  if (msg.content.indexOf("https://") === 0) {
    return { allowContinue: true, modifiedData: { content: "[链接]" } };
  }
  return { allowContinue: true };
}, { mode: "blocking", order: "early" });

// 用法2: (type, options, handler)
yara.hook.register("chat.receive.before_process", { mode: "observe" }, function (event) {
  yara.logger.info("观察消息: " + (event.message ? event.message.content : ""));
});
```

### 3.3 指令（Command）

```javascript
yara.command.register("weather", "/天气\\s+(.+)", function (match, context) {
  var city = match[1];
  var groupId = context ? context.groupId : "default";
  var resp = yara.http.get("https://api.example.com/weather?city=" + encodeURIComponent(city));
  yara.send.text(groupId, "天气结果：" + JSON.parse(resp.body).result);
  return "done";
}, { aliases: ["/查天气"] });
```

### 3.4 Agent 工具（Tool）

```javascript
yara.tool.register("get_weather", {
  description: "查询指定城市的实时天气",
  parameters: [
    { name: "city", type: "string", description: "城市名称，示例: 北京", required: true }
  ]
}, function (params, context) {
  return { city: params.city, temperature: 25, condition: "晴" };
});

// 自主运行工具（系统按 hookType/pattern 自动触发；handler 在 definition 内）
yara.tool.registerAutonomous({
  name: "auto_bilibili",
  hookType: "chat.receive.before_process",
  pattern: "bilibili\\.com/video/(BV\\w+)",
  handler: function (message) { return { intercepted: false }; }
});
```

### 3.5 跨插件 API

```javascript
yara.api.register("getVersion", function (params) { return "1.0.0"; }, { public: true });

// 调用本插件或其它插件: 插件ID.方法名
var v = yara.api.call("com.yaraflow.weather.getVersion", {});
```

### 3.6 消息发送

```javascript
yara.send.text("group-123", "你好");                       // 广播
yara.send.image("group-123", "https://.../a.png");
yara.send.emoji("group-123", "emoji_hash");
yara.send.hybrid("group-123", [ { type: "text", content: "x" }, { type: "image", content: "..." } ]);
```

`yara.send.*` 在**钩子/事件分发触发的回调**里调用时，会携带本次 `request_id`，从而**单播回**给发起该事件的 ws 客户端；在插件其余位置调用则默认**广播**给所有客户端。

---

## 四、config.yaml 读写

```javascript
function onLoad() {
  var cfg = yara.config.getFile() || {};
  if (!cfg.apiKey) {
    yara.config.setFile({ apiKey: "your-api-key", maxResults: 10 });
  }
}
```

> 约定：配置文件由插件**首次运行时自动生成并写示例值**；`description` 尽量携带示例格式（如 `示例: sk-xxx`）。

---

## 五、后台耗时任务

```javascript
yara.command.register("longtask", "/longtask", function () {
  var t = yara.async.run(function (task) {
    return doHeavyWork();          // 后台执行，不阻塞同插件的其它调用
  }, { timeout: 300 });
  return "任务已提交: " + t.taskId;
});
```

`yara.async.run` 返回 `{ taskId, status: "running", timeout }`，引擎在后台串行执行 `taskFn`。

---

## 六、一个完整示例插件

```
local_data/package/com.example.hello/
├── metadata.json            # tags: ["LTP3"]
├── index.js
└── config.yaml              # （首次运行自动生成也可）
```

```javascript
// index.js
function onLoad() {
  yara.logger.info("Hello 插件已加载");
  yara.config.setFile({ greeting: "你好呀", maxReplies: 3 });
}

function onUnload() { yara.logger.info("Hello 插件已卸载"); }

function onConfigUpdate(scope, config, version) {
  windowedGreeting = config.greeting || "你好呀";
}

var windowedGreeting = "你好呀";

yara.hook.register(YaraHooks.CHAT_RECEIVE_AFTER_PROCESS, function (event) {
  var msg = event.message;
  if (!msg || !msg.content) return { allowContinue: true };
  var cfg = yara.config.getFile() || {};
  yara.send.text(msg.groupId || "default", (cfg.greeting || windowedGreeting) + " " + msg.senderName);
  return { allowContinue: true };
}, { mode: "observe" });

yara.command.register("hi", "hi|hello|你好", function (match, context) {
  yara.send.text(context.groupId, "Hello World!");
  return "done";
});
```

> 编写时在 index.js 顶部加 `/// <reference path="yara.d.ts" />` 以启用 IDE 自动补全（见本目录 `yara.d.ts`）。

---

## 七、插件如何被加载/卸载

- **加载**：引擎启动时扫描 `local_data/package` 下所有含 `LTP3` 标签的包；之后每 3 秒自动对账，**新增包→加载沙箱**。
- **卸载**：包被删除、或被改为不含 `LTP3` 标签时，引擎自动卸载对应沙箱。
- **手动管理**：可通过事件客户端发 `ltp3/manage`（`list/scan/reload/reload_one/unload_one`）触发热加载/卸载。

详见《事件客户端开发指南》与《LTP3 引擎层实现文档》。