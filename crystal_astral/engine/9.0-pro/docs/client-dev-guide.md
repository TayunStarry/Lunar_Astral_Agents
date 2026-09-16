# 事件客户端开发指南

> LTP3 引擎层运行在 **crystal_astral** 内，负责加载并执行 LTP3 插件的 goja 沙箱。真正的"消费方"——例如消息桥（NapCat）、月华、其它需要被插件调控的客户端——通过 **WebSocket 与引擎层链接**：客户端在事件/钩子点**发布**消息，引擎把消息**分发给所有订阅该钩子的插件订阅器**执行，再把各插件的处理结果**返回**给客户端。

本指南覆盖：连接方式、消息信封协议（`ltp3/*`）、请求/响应配对、以及 `ltp3/send`（插件主动发消息）的送达策略。

---

## 一、连接

- 引擎复用现有 WebSocket 集线器，端点为 `ws://<host>:<port>/ws`。
- 客户端连上后即可发送 JSON 信封；**非 `ltp3/*` 前缀的消息会被引擎忽略**（只转发给其它订阅者，互不干扰）。
- 消息大小上限 10MB；默认 60 秒无读超时（可自行维持心跳）。

> 引擎层自身会把 `ltp3/*` 出站消息广播回 `/ws`，因此**另一个 ltp3 客户端也能接到其它客户端触发的广播**。所有客户端都应按 `request_id` 过滤出属于自己的回执。

---

## 二、信封总览

所有报文为 JSON 对象，公共字段：

| 字段 | 说明 |
|---|---|
| `type` | 报文类型（`ltp3/*` 前缀） |
| `request_id` | 请求唯一 ID（客户端发起时生成随机串，用于回执配对） |

### 客户端 → 引擎（请求）

| type | 说明 | 关键字段 |
|---|---|---|
| `ltp3/hook` | 在某个钩子点发布消息 | `hook`、`payload`、`context` |
| `ltp3/event` | 发布系统/自定义事件 | `event`、`payload` |
| `ltp3/command` | 调用插件注册的指令 | `command`、`match`、`context` |
| `ltp3/tool` | 调用插件注册的 Agent 工具 | `tool`、`payload`（工具参数）、`context` |
| `ltp3/manage` | 插件/引擎管理 | `action`（list/scan/reload/reload_one/unload_one）、`id` |
| `ltp3/ping` | 存活探测 | — |

### 引擎 → 客户端（响应 / 推送）

| type | 说明 |
|---|---|
| `ltp3/hook_result` | 钩子分发结果（含各插件回调结果与聚合汇总） |
| `ltp3/event_ack` | 事件发布确认（含订阅数） |
| `ltp3/command_result` | 指令执行结果 |
| `ltp3/tool_result` | 工具调用结果 |
| `ltp3/manage_ack` | 管理动作确认 + 插件状态列表 |
| `ltp3/pong` | ping 响应 |
| `ltp3/send` | **插件主动发来的消息**（单播/广播） |
| `ltp3/lifecycle` | 插件加载/卸载广播（`ON_START`/`ON_STOP`） |
| `ltp3/error` | 未知类型等错误 |

---

## 三、请求示例与响应

### 3.1 发布钩子

```json
→ {"type":"ltp3/hook","request_id":"req-001","hook":"chat.receive.before_process","payload":{"id":"m1","groupId":"123","senderName":"小明","content":"你好","platform":"qq"},"context":{"session":"s1"}}

← {"type":"ltp3/hook_result","request_id":"req-001","hook":"chat.receive.before_process",
   "results":[
     {"plugin_id":"com.example.hello","handled":true,"result":{"allowContinue":true}},
     {"plugin_id":"com.yaraflow.weather","handled":true,"result":{"allowContinue":true,"modifiedData":{"content":"..."}}}
   ],
   "summary":{"subscribed":2,"errored":0,"allow_continue":true,"aborted":false}}
```

- `results[]`：每个订阅该钩子的插件返回一个结果；`error` 非空表示该插件回调抛错。
- `summary`：`subscribed` 订阅数、`errored` 出错数、`allow_continue`/`aborted`（由各插件返回的 `allowContinue`/`action:"abort"` 聚合）。
- `payload` 会被引擎原样透传给插件回调的 `event.message`（钩子）或 `eventData`（事件）。

### 3.2 发布事件

```json
→ {"type":"ltp3/event","request_id":"req-002","event":"ON_START","payload":{}}
← {"type":"ltp3/event_ack","request_id":"req-002","event":"ON_START","subscribed":2}
```

`subscribed` = 订阅该事件的插件订阅器执行次数。

### 3.3 调用插件指令

```json
→ {"type":"ltp3/command","request_id":"req-003","command":"/天气","context":{"groupId":"123"}}
```
引擎先按 `command` 精确匹配各插件注册的指令；未命中时回退为用每条指令的**正则**去匹配整段文本（`/天气 上海`）。响应为 `ltp3/command_result`（结构同 `hook_result`）。

### 3.4 调用插件工具（Tool）

```json
→ {"type":"ltp3/tool","request_id":"req-tool","tool":"get_weather","payload":{"city":"北京"},"context":{"groupId":"123"}}
← {"type":"ltp3/tool_result","request_id":"req-tool","tool":"get_weather",
   "results":[{"plugin_id":"com.yaraflow.weather","handled":true,"result":{"city":"Beijing","current":{"temperature":25,"condition":"晴"},..."}}],
   "summary":{"subscribed":1,"errored":0,"allow_continue":true,"aborted":false}}
```

`payload` 即工具参数对象，原样传给插件 handler 的 `params`；不存在的工具返回 `subscribed:0` 的空结果。

### 3.5 管理动作

```json
→ {"type":"ltp3/manage","request_id":"req-004","action":"scan"}
← {"type":"ltp3/manage_ack","request_id":"req-004","action":"scan","ok":true,"message":"对账完成",
   "plugins":[{"id":"com.example.hello","dir_name":"com.example.hello","loaded":true}, ...]}

→ {"type":"ltp3/manage","request_id":"req-005","action":"reload_one","id":"com.example.hello"}
← {"type":"ltp3/manage_ack","request_id":"req-005","action":"reload_one","ok":true,"plugins":[ ... ]}
```

- `action`：`list`（列出状态）、`scan`（强制对账一次）、`reload`（重载全部）、`reload_one`（`id` 指定单个重载）、`unload_one`（`id` 指定单个卸载）。

### 3.6 探测

```json
→ {"type":"ltp3/ping","request_id":"req-006"}
← {"type":"ltp3/pong","request_id":"req-006","engine":"LTP3","plugins":3}
```

---

## 四、request_id 配对（单播回执）

引擎把响应**广播**给所有 ltp3 客户端。客户端收到后，用**自己发起的 `request_id`** 过滤出属于自己的回执：

```javascript
ws.onmessage = (e) => {
  const m = JSON.parse(e.data);
  if (m.request_id && pending.has(m.request_id)) {   // 只处理自己发起的
    resolvePending(m);
  }
};
```

> 因此多客户端并存时互不串扰；未配对的广播（`ltp3/send` 无 request_id、`ltp3/lifecycle`）由各客户端自行决定是否处理。

---

## 五、插件主动发消息：`ltp3/send`

当插件内调用 `yara.send.*`：

```json
{ "type":"ltp3/send", "request_id":"req-001", "plugin_id":"com.yaraflow.weather",
  "kind":"text", "group_id":"123", "content":"现在 25°C", "success":true }

{ "type":"ltp3/send", "plugin_id":"com.yaraflow.weather", "kind":"image",
  "group_id":"123", "image":"<base64 或 url>", "success":true }
```

- **`request_id` 存在** → 这是对某次触发（钩子/事件/指令）的**回执**，单播回给该次 `request_id` 对应的客户端。
- **`request_id` 缺失** → **默认广播**给所有客户端。

字段：`kind`（text/image/emoji/hybrid）、`group_id`、`content`/`image`/`emoji`/`segments`、`success`。

---

## 六、生命周期广播：`ltp3/lifecycle`

插件加载/卸载时，引擎广播给所有客户端：

```json
{"type":"ltp3/lifecycle","event":"ON_START","plugin":"com.example.hello","title":"Hello"}
{"type":"ltp3/lifecycle","event":"ON_STOP","plugin":"com.example.hello","title":"Hello"}
```

客户端可据此刷新本地的插件能力/状态视图。

---

## 七、接入示例（最小 ltp3 客户端）

```javascript
// Node 侧（依赖 ws 包）
const WS = require('ws');
const ws = new WS('ws://localhost:36789/ws');

ws.on('open', () => {
  ws.send(JSON.stringify({ type: 'ltp3/hook',
    request_id: 'r' + Date.now(),
    hook: 'chat.receive.after_process',
    payload: { id: '1', groupId: 'g1', senderName: '小明', content: '今天天气如何', platform: 'qq' }
  }));
});

ws.on('message', (raw) => {
  const m = JSON.parse(raw);
  if (m.type === 'ltp3/hook_result') console.log('订阅器结果:', m.results, m.summary);
  if (m.type === 'ltp3/send') console.log('插件想发言:', m.kind, m.content || m.image || '', 'group=', m.group_id);
});
```

> 注意：示例仅作演示。真实客户端起消息桥作用时，应在对应业务节点（如收到聊天消息）发布 `ltp3/hook` 到相关钩子点，并把插件回执转成真正发出的消息。`yara.platform.sendCommand` 等平台级能力同样应由真实客户端承接（引擎层对这类能力返回"未接入"占位）。