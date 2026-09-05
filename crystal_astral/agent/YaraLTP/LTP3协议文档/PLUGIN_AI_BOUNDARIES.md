# YaraFlow 插件开发 AI 安全边界文档

> **重要：本文档专门为 AI 助手编写 YaraFlow 插件时提供安全指导。**
> 所有由 AI 生成的插件代码必须严格遵守本文档中定义的所有边界和限制。

***

## 一、核心原则

### 1.1 沙箱隔离原则

YaraFlow 插件运行在 **Goja JavaScript 沙箱** 中。这意味着：

- **插件只能访问主程序通过** **`yara.*`** **对象显式暴露的 API**

- **插件无法直接访问 Go 运行时、文件系统（除插件目录）、网络（除 yara.http API）、进程、系统调用**

- **插件之间相互隔离，每个插件运行在独立的 VM 实例中**

### 1.2 最小权限原则

**永远不要在** **`permissions`** **中声明插件不需要的权限。** 每个权限都是一个潜在的攻击面。

### 1.3 声明即授权原则

插件在 `plugin.json` 的 `permissions` 中声明什么权限，就只能使用什么 API。未声明的权限对应的 API 不会被注入到沙箱中。

***

## 二、绝对禁止的行为

以下行为 AI **绝对不允许** 在插件代码中执行：

### 2.1 禁止使用全局 Node.js / 浏览器 API

以下 API **不存在于沙箱中**，调用会导致运行时错误：

```javascript
// ❌ 绝对禁止
require("fs")           // 没有 Node.js require
import ... from ...     // 没有 ES Module 导入
fetch()                 // 使用 yara.http.get() 代替
setTimeout()            // 已被禁用，会抛出 TypeError
setInterval()           // 已被禁用，会抛出 TypeError
XMLHttpRequest          // 不存在
WebSocket               // 不存在
localStorage            // 使用 yara.config 代替
sessionStorage          // 不存在
document                // 不存在（不是浏览器环境）
window                  // 不存在
process.env             // 不存在
globalThis             // 不存在
```

### 2.2 禁止绕过沙箱

```javascript
// ❌ 绝对禁止任何绕过沙箱的尝试
eval("...")                    // 虽然可能可用，但禁止使用
new Function("...")            // 禁止
this.constructor.constructor() // 禁止任何原型链逃逸
arguments.callee.caller        // 禁止
```

### 2.3 禁止操作主程序数据

```javascript
// ❌ 禁止直接操作主程序数据库、配置、文件
// 插件只能通过 yara.* API 访问主程序资源
```

### 2.4 禁止声明未使用的权限

```json
// ❌ 禁止：声明所有权限但只用了其中几个
{
  "permissions": [
    "send.text", "send.image", "send.emoji", "send.hybrid",
    "send.forward", "command.register", "tool.register", "hook.register", "event.subscribe",
    "event.publish", "event_handler.register", "llm_provider.register",
    "api.register", "api.call", "http.request",
    "model.access", "database.read", "database.write",
    "plugin.config.read", "plugin.config.write",
    "plugin.file.read", "plugin.file.write",
    "emoji.access", "knowledge.search",
    "async_task.execute", "llm.generate",
    "llm.generate_with_tools", "llm.embed",
    "data.directory.read", "data.directory.write"
  ]
}
```

### 2.5 禁止声明不可用的权限

```json
// ❌ 禁止：声明不存在的权限（会导致加载失败）
{ "permissions": ["admin.access", "system.root", "shell.execute"] }
```

**只有** **[插件开发指南](./PLUGIN_DEV_GUIDE.md)** **中列出的权限才是有效的。**

***

## 三、必须遵守的约束

### 3.1 plugin.json 必须完整

AI 生成的 `plugin.json` 必须包含以下必填字段：

```json
{
  "manifestVersion": 1,
  "id": "com.example.plugin-name",     // 必须：反向域名格式
  "name": "插件名称",                    // 必须：有意义的名称
  "version": "1.0.0",                   // 必须：语义化版本
  "description": "插件功能描述",          // 必须：清晰的描述
  "author": { "name": "作者" },          // 必须：作者信息
  "main": "index.js",                   // 必须：入口文件名
  "permissions": []                     // 必须：至少声明实际需要的权限
}
```

### 3.2 插件 ID 格式约束

```javascript
// ✓ 正确格式
"com.example.my-plugin"
"org.company.tool"
"net.author.hello-world"

// ✗ 错误格式
"my-plugin"              // 缺少命名空间
"com.example.my_plugin"  // 建议使用连字符，不要用下划线
"COM.EXAMPLE.PLUGIN"     // 必须小写
```

### 3.3 版本号格式约束

```javascript
// ✓ 正确格式
"1.0.0"
"2.1.3"

// ✗ 错误格式
"v1.0.0"    // 不要加 v 前缀
"1.0"       // 必须三段式
"1.0.0.0"   // 只能是三段式
```

### 3.4 参数类型必须有效

在 `parameters` 中定义参数时，`type` 必须是以下之一：

```javascript
// ✓ 有效的参数类型
"string" | "integer" | "number" | "float" | "boolean" | "array" | "object"

// ✗ 无效的参数类型
"int" | "bool" | "str" | "dict" | "any" | "mixed"
```

### 3.5 工具类型与可见性约束

自主运行工具（`autonomous`）必须设置 `visibility` 为 `hidden`：

```json
// ✓ 正确
{
  "name": "auto_intercept",
  "toolType": "autonomous",
  "visibility": "hidden"
}

// ❌ 错误：自主运行工具不能 visible
{
  "name": "auto_intercept",
  "toolType": "autonomous",
  "visibility": "visible"
}
```

### 3.6 Hook 类型必须有效

通过 `yara.hook.register()` 只能注册以下 Hook 点（`plugin.json` 不声明 Hook）：

```javascript
// 有效的 Hook 类型（完整列表）
"chat.receive.before_process"
"chat.receive.after_process"
"chat.command.before_execute"
"chat.command.after_execute"
"emoji.chat.before_select"
"emoji.chat.after_select"
"emoji.register.after_build_description"
"emoji.register.after_build_emotion"
"send_service.after_build_message"
"send_service.before_send"
"send_service.after_send"
"chat.planner.before_request"
"chat.planner.after_response"
"chat.replyer.before_request"
"chat.replyer.before_model_request"
"chat.replyer.after_response"
"jargon.query.before_search"
"jargon.query.after_search"
"jargon.extract.before_persist"
"jargon.inference.before_finalize"
"expression.select.before_select"
"expression.select.after_selection"
"expression.learn.after_extract"
"expression.learn.before_upsert"
```

### 3.7 事件类型必须有效

```javascript
// 有效的事件类型
"ON_START" | "ON_STOP" | "ON_MESSAGE_PRE_PROCESS" | "ON_MESSAGE" |
"ON_PLAN" | "POST_LLM" | "AFTER_LLM" | "POST_SEND_PRE_PROCESS" |
"POST_SEND" | "AFTER_SEND"
```

***

## 四、运行时边界

### 4.1 超时限制

- **脚本执行超时：30 秒。** 超过 30 秒的同步操作会被强制中断。

- **HTTP 请求超时：30 秒。** 由 `yara.http.*` API 保证。

- **异步任务默认超时：300 秒（5 分钟）。** 可通过 `yara.async.run(fn, { timeout: N })` 自定义。

**AI 必须确保插件代码不会导致超时：**

```javascript
// ✓ 好：使用异步任务处理长时间操作
yara.command.register("longtask", "/longtask", function() {
  yara.async.run(function(task) {
    // 这个函数可以运行 300 秒
    var result = doLongWork();
    return result;
  });
  return "任务已提交";
});

// ✗ 坏：同步执行长时间操作会导致超时
yara.command.register("longtask", "/longtask", function() {
  var result = doLongWork(); // 可能超过 30 秒导致超时
  return result;
});
```

### 4.2 内存限制

- 插件不应尝试加载大量数据到内存中

- 避免在插件中创建大量对象或无限循环

- 如果插件需要处理大量数据，应分批处理

### 4.3 文件操作限制

- **只能操作插件自身目录下的文件**

- **路径遍历攻击会被检测并阻止**

- 只能通过 `yara.file.*` API 操作文件

```javascript
// ✓ 正确：在插件目录内操作
yara.file.writeData("cache/data.json", content);
yara.file.readData("downloads/file.txt");

// ✗ 错误：会被阻止
yara.file.readData("../../../etc/passwd");  // 路径遍历
yara.file.read("../other-plugin/data.json"); // 跨插件目录
```

***

## 五、API 使用边界

### 5.1 消息发送边界

```javascript
// ✓ 正确：发送合理的消息
yara.send.text("group-123", "你好！");
yara.send.text("group-123", "查询结果如下：...");

// ✗ 禁止：发送垃圾消息、刷屏
// 不要在循环中无限制地发送消息
// 不要发送过长的文本（建议单条消息不超过 2000 字符）
```

### 5.2 HTTP 请求边界

```javascript
// ✓ 正确：使用 yara.http API
var resp = yara.http.get("https://api.example.com/data");
var resp = yara.http.post("https://api.example.com/submit", data);

// ✗ 禁止：访问 localhost 内部服务
// 不要请求 127.0.0.1、localhost 上的服务
// 不要请求 YaraFlow 自身的 API 端点
```

### 5.3 模型调用边界

```javascript
// ✓ 正确：使用 yara.model API
var resp = yara.model.chat({
  messages: [{ role: "user", content: "问题" }],
  taskType: "replyer"
});

// ✗ 禁止：绕过 yara.model API 直接调用 LLM 服务
// 不要通过 yara.http 直接调用 OpenAI/Anthropic API
// 所有 LLM 调用必须通过 yara.model 进行
```

### 5.4 数据库查询边界

```javascript
// ✓ 正确：查询合理数量的消息
yara.database.queryMessages({ platform: "qq", groupId: "123", limit: 20 });
yara.database.searchMessages({ query: "关键词", limit: 50 });

// ✗ 禁止：查询大量数据
// 不要设置 limit 超过 200
// 不要在循环中反复查询数据库
```

***

## 六、组件注册边界

### 6.1 指令 (Command) 注册边界

```javascript
// ✓ 正确：注册有意义的指令
yara.command.register("translate", "/翻译\\s+(.+)", handler);

// ✗ 禁止：注册与已有指令冲突的名称
// ✗ 禁止：注册过于宽泛的正则（如 .*）
// ✗ 禁止：注册空名称或空模式
```

### 6.2 工具 (Tool) 注册边界

```javascript
// ✓ 正确：注册有明确功能的工具
// 签名: yara.tool.register(name, definition, handler)
// - name: 工具名称
// - definition: 包含 description 和 parameters 的对象
// - handler: 处理器函数，接收 params 和 context
yara.tool.register("get_weather", {
  description: "查询指定城市的天气",
  parameters: [
    { name: "city", type: "string", description: "城市名称", required: true }
  ]
}, function(params, context) {
  return { city: params.city, temperature: 25 };
});

// ✗ 禁止：注册功能过于宽泛的工具
// ✗ 禁止：注册与主程序核心功能重复的工具
// ✗ 禁止：注册不返回任何值的工具（LLM 调用会困惑）
```

### 6.3 Hook 注册边界

```javascript
// ✓ 正确：直接通过 yara.hook.register 注册（无需在 plugin.json 声明 Hook）
// index.js（用法1: type, handler, options）:
yara.hook.register("chat.receive.before_process", function(event) {
  return { allowContinue: true };
}, { mode: "blocking", order: "normal" });

// index.js（用法2: type, options, handler）:
yara.hook.register("chat.receive.before_process", {
  mode: "blocking",
  order: "early"
}, function(event) {
  return { allowContinue: true };
});

// ✗ 禁止：注册无效的 Hook 类型（会直接抛错）
// ✗ 禁止：注册 Hook 但不声明对应的权限（hook.register）
```

***

## 七、插件间交互边界

### 7.1 跨插件 API 调用

```javascript
// ✓ 正确：调用其他插件公开的 API
// 权限：api.call
yara.api.call("com.example.other-plugin.getData", { key: "value" });

// ✗ 禁止：尝试直接访问其他插件的数据
// ✗ 禁止：尝试通过文件系统读取其他插件目录
// ✗ 禁止：硬编码其他插件的 API 名称（应通过文档可知）
```

### 7.2 事件发布边界

```javascript
// ✓ 正确：发布自定义事件
yara.event.publish("plugin.customEvent", { data: "value" });

// ✗ 禁止：发布系统事件类型（ON_START, ON_STOP 等）
// 系统事件类型只能由主程序发布
```

***

## 八、错误处理边界

### 8.1 必须处理错误

```javascript
// ✓ 正确：所有外部调用都应有错误处理
function onLoad() {
  try {
    var resp = yara.http.get("https://api.example.com/data");
    if (resp.error) {
      yara.logger.warn("API 请求失败: " + resp.error);
      return;
    }
    var data = JSON.parse(resp.body);
    // 处理数据
  } catch (e) {
    yara.logger.error("初始化失败: " + e.message);
  }
}

// ✗ 禁止：不处理错误直接使用结果
// ✗ 禁止：使用 try-catch 捕获错误后静默忽略
```

### 8.2 生命周期函数必须安全

```javascript
// ✓ 正确：onLoad 和 onUnload 不会抛出异常
function onLoad() {
  try {
    // 初始化逻辑
  } catch (e) {
    yara.logger.error("初始化失败: " + e.message);
    // 不要让异常传播到主程序
  }
}

function onUnload() {
  try {
    // 清理逻辑
  } catch (e) {
    yara.logger.error("清理失败: " + e.message);
  }
}

// ✗ 禁止：生命周期函数抛出未捕获的异常
```

***

## 九、代码质量边界

### 9.1 必须定义的生命周期函数

即使为空，也应定义 `onLoad` 和 `onUnload`：

```javascript
// ✓ 正确
function onLoad() {
  yara.logger.info("插件已加载");
}

function onUnload() {
  // 清理资源
}

// ✗ 禁止：完全不定义生命周期函数（虽然不会报错，但不规范）
```

### 9.2 沙箱隔离与全局作用域

**每个插件运行在独立的沙箱（goja VM）中，全局作用域是插件私有的**：在全局作用域定义变量不会影响其他插件，其他插件也读不到，因此**不需要用 IIFE/命名空间来"防污染"**。

需要遵守的：

- **不要修改** **`yara`** **全局对象**（`yara.*` 是主程序注入的 API 命名空间）。覆盖它（如 `yara.logger = ...`）会破坏本插件后续对 API 的调用，导致插件功能异常。

- **不要依赖跨插件共享的全局状态**（不存在这样的状态）。跨插件通信只能通过 `yara.api.call()`（函数调用）与 `yara.event.publish` / `yara.event.subscribe`（事件广播）进行。

- 为了代码可读性，仍建议把内部逻辑组织在命名空间/对象内，但**这不是安全要求**，只是编码规范。

```javascript
// ✓ 正确：直接在插件自己的全局作用域定义变量（沙箱隔离，不影响其他插件）
var myCounter = 0;
function doSomething() { return ++myCounter; }

// ✗ 禁止：修改主程序注入的 yara 全局对象（会破坏本插件 API 调用）
// yara.logger = null;  // 错误！会导致本插件后续日志调用失败
```

### 9.3 日志使用规范

```javascript
// ✓ 正确：使用合适的日志级别
yara.logger.info("用户查询了天气: " + city);
yara.logger.warn("API 请求失败，使用缓存数据");
yara.logger.error("严重错误: " + error.message);
yara.logger.debug("调试信息: " + JSON.stringify(data));

// ✗ 禁止：在循环中大量输出日志
// ✗ 禁止：在日志中输出敏感信息（API Key、密码等）
```

***

## 十、安全检查清单

AI 在生成插件代码后，必须逐项检查以下清单：

### plugin.json 检查

- [ ] `id` 是否为反向域名格式、小写、连字符分隔？

- [ ] `version` 是否为三段式语义化版本？

- [ ] `main` 指向的文件是否存在？

- [ ] `permissions` 是否只声明了实际需要的权限？

- [ ] 所有权限是否都在允许列表中？

- [ ] `dependencies` 中的 `type` 是否为 `plugin` 或 `python_package`？

- [ ] 插件是否依赖了自身？

### metadata.json 检查

- [ ] `id` 是否与目录名和 `plugin.json` 的 `id` 一致？

- [ ] `version` 是否为三段式语义化版本（不含 v 前缀）？

- [ ] `tags` 是否包含 `LTP3`（语瞳插件）和/或 `LTPX`（通用）标签？

- [ ] `url` 路径格式是否为 `/file/read/package/{id}/index.html`？

- [ ] `tools` 字段（工具型包）是否按 OpenAI function tool 格式定义？

### 代码检查

- [ ] 是否定义了 `onLoad` 和 `onUnload` 函数？

- [ ] 是否使用了不存在于沙箱中的 API（`require`、`fetch`、`setTimeout` 等）？

- [ ] 是否有任何绕过沙箱的尝试？

- [ ] 所有外部调用是否都有错误处理？

- [ ] 是否在同步代码中执行了可能超时的操作？

- [ ] 是否有路径遍历风险？

- [ ] 是否在日志中输出了敏感信息？

- [ ] 是否发送了过多消息（可能导致刷屏）？

- [ ] 是否注册了无效的 Hook 类型 / 事件类型？

- [ ] 是否发布了系统事件类型？

- [ ] 是否尝试直接调用 LLM API（而非通过 `yara.model`）？

- [ ] 是否操作了插件目录之外的文件？

- [ ] 代码中是否有无限循环或递归？

### 安全边界检查

- [ ] 插件是否只通过 `yara.*` API 与主程序交互？

- [ ] 插件是否尝试访问其他插件的数据？

- [ ] 插件是否尝试访问主程序的内部状态？

- [ ] 插件是否包含了硬编码的敏感信息（密码、密钥等）？

***

## 十一、模板示例

以下是一个 AI 生成插件时应遵循的模板：

### plugin.json 模板

```json
{
  "manifestVersion": 1,
  "id": "com.example.plugin-name",
  "name": "插件名称",
  "version": "1.0.0",
  "type": "tool",
  "description": "插件的简要描述",
  "author": {
    "name": "作者名",
    "url": "https://example.com"
  },
  "license": "MIT",
  "main": "index.js",
  "permissions": [
    "send.text",
    "http.request"
  ],
  "config": {
    "type": "yaml",
    "configFile": "config.yaml",
    "default": {}
  }
}
```

### index.js 模板

```javascript
/**
 * 插件名称
 * 描述：插件的简要描述
 * 版本：1.0.0
 */

// ===== 生命周期 =====

function onLoad() {
  try {
    yara.logger.info("插件已加载");
    // 初始化逻辑
    initPlugin();
  } catch (e) {
    yara.logger.error("插件初始化失败: " + e.message);
  }
}

function onUnload() {
  try {
    yara.logger.info("插件正在卸载");
    // 清理逻辑
    cleanup();
  } catch (e) {
    yara.logger.error("插件清理失败: " + e.message);
  }
}

function onConfigUpdate(scope, config, version) {
  yara.logger.info("配置已更新: " + scope);
  // 重新加载配置
}

// ===== 插件逻辑 =====

function initPlugin() {
  // 注册命令
  yara.command.register("mycmd", "/mycmd\\s+(.*)", handleCommand);
}

function handleCommand(match) {
  try {
    var arg = match[1];
    var result = processRequest(arg);
    yara.send.text("group-123", result);
    return "done";
  } catch (e) {
    yara.logger.error("命令执行失败: " + e.message);
    return "error: " + e.message;
  }
}

function processRequest(input) {
  // 核心业务逻辑
  var resp = yara.http.get("https://api.example.com/process?q=" + encodeURIComponent(input));
  if (resp.error) {
    return "请求失败: " + resp.error;
  }
  var data = JSON.parse(resp.body);
  return formatResponse(data);
}

function formatResponse(data) {
  // 格式化输出
  return "结果: " + JSON.stringify(data);
}

function cleanup() {
  // 清理资源
}
```

***

## 十二、常见违规示例

以下是一些 AI 常犯的错误及正确写法：

### 错误 1：使用 fetch 代替 yara.http

```javascript
// ❌ 错误
var data = fetch("https://api.example.com/data").then(r => r.json());

// ✓ 正确
var resp = yara.http.get("https://api.example.com/data");
var data = JSON.parse(resp.body);
```

### 错误 2：用 setTimeout 做延迟

沙箱没有事件循环，`setTimeout`/`setInterval` 不可用。延时请用 `yara.time.sleep()`（同步睡眠，可被插件超时安全打断）：

```javascript
// ❌ 错误：setTimeout 在沙箱中不存在
setTimeout(function() {
  yara.send.text("group-123", "延迟消息");
}, 1000);

// ✓ 正确：同步睡眠后继续执行（可被超时打断，不会永久卡死）
yara.time.sleep(1000);
yara.send.text("group-123", "延迟消息");

// ✓ 正确：超长任务用异步任务（避免超过同步超时上限）
yara.async.run(function(task) {
  // 耗时工作（如下载、计算）放这里，后台执行
  yara.send.text("group-123", "处理完成");
});
```

### 错误 3：声明过多权限

```json
// ❌ 错误
{
  "permissions": [
    "send.text", "send.image", "send.emoji", "send.hybrid", "send.forward",
    "command.register", "tool.register", "hook.register",
    "event.subscribe", "event.publish", "http.request", "model.access",
    "database.read", "database.write", "plugin.config.read", "plugin.config.write",
    "plugin.file.read", "plugin.file.write", "emoji.access", "knowledge.search",
    "async_task.execute", "llm.generate", "llm.generate_with_tools", "llm.embed"
  ]
}

// ✓ 正确：只声明实际需要的
{
  "permissions": [
    "send.text",
    "http.request",
    "command.register"
  ]
}
```

### 错误 4：不处理 HTTP 错误

```javascript
// ❌ 错误
var resp = yara.http.get("https://api.example.com/data");
var data = JSON.parse(resp.body);  // 如果 resp.error 存在，body 可能为空

// ✓ 正确
var resp = yara.http.get("https://api.example.com/data");
if (resp.error) {
  yara.logger.warn("请求失败: " + resp.error);
  return;
}
if (resp.status !== 200) {
  yara.logger.warn("HTTP 状态异常: " + resp.status);
  return;
}
var data = JSON.parse(resp.body);
```

### 错误 5：在循环中发送消息

```javascript
// ❌ 错误：可能导致刷屏
for (var i = 0; i < 100; i++) {
  yara.send.text("group-123", "第 " + i + " 条消息");
}

// ✓ 正确：合并为一条消息
var parts = [];
for (var i = 0; i < 100; i++) {
  parts.push("第 " + i + " 条数据");
}
yara.send.text("group-123", parts.join("\n"));
```

