# YaraFlow 插件开发指南

## 目录

1. [快速开始](#快速开始)
2. [plugin.json 清单文件](#pluginjson-清单文件)
3. [生命周期函数](#生命周期函数)
4. [完整 API 参考](#完整-api-参考)
5. [指令 (Command) 开发](#指令-command-开发)
6. [工具 (Tool) 开发](#工具-tool-开发)
7. [Hook 开发](#hook-开发)
8. [事件处理器开发](#事件处理器开发)
9. [插件白名单认证](#插件白名单认证)
10. [最佳实践](#最佳实践)
11. [LTP3 包标准](#ltp3-包标准)

***

## 快速开始

### 最小插件示例

**目录结构：**

```
plugins/com.example.hello/
├── plugin.json
└── index.js
```

**plugin.json：**

```json
{
  "manifestVersion": 1,
  "id": "com.example.hello",
  "name": "Hello World",
  "version": "1.0.0",
  "description": "一个简单的示例插件",
  "author": {
    "name": "你的名字",
    "url": "https://example.com"
  },
  "main": "index.js",
  "permissions": [
    "send.text"
  ]
}
```

**index.js：**

```javascript
// 生命周期：插件加载时调用
function onLoad() {
  yara.logger.info("Hello World 插件已加载！");
}

// 生命周期：插件卸载时调用
function onUnload() {
  yara.logger.info("Hello World 插件已卸载！");
}

// 注册一个命令
// 第二个参数 context 包含 { platform, groupId }
yara.command.register("hello", "你好|hello|hi", function(match, context) {
  var groupId = context ? context.groupId : "default";
  yara.send.text(groupId, "你好呀！我是 Hello World 插件~");
  return "done";
});
```

***

### 类型定义与 IDE 补全（yara.d.ts）

项目根目录下提供了 **`yara.d.ts`**（TypeScript 类型声明文件，路径 `YaraFlow/yara.d.ts`）。在插件 JS 文件顶部添加引用指令后，VS Code 等 IDE 会在你写 `yara.xxx` 时给出**自动补全、参数提示和类型检查**（即"一键填充"），无需记忆 API 名和参数：

```javascript
/// <reference path="../../yara.d.ts" />   // plugins/<插件ID>/index.js 里这样引用
```

**事件名 / Hook 点不再需要手敲裸字符串**，运行时会注入全局常量对象 `YaraEvents` / `YaraHooks`，配合 d.ts 的类型也能直接写字符串字面量（有补全提示）：

```javascript
// 用常量（运行时可用的全局对象，推荐）
yara.event.subscribe(YaraEvents.ON_MESSAGE, function (eventData) {
  yara.logger.info("收到消息：" + eventData.content);
});

// 或直接写字符串字面量（d.ts 联合类型同样会给出补全候选）
yara.hook.register(YaraHooks.CHAT_RECEIVE_BEFORE_PROCESS, function (event) {});
```

> 提示：`YaraEvents` / `YaraHooks` 是沙箱内注入的**全局常量**（不属于 `yara.*`），任何插件可直接引用，无需声明权限。常量清单以 [yara.d.ts](../yara.d.ts) 为准。

***

## plugin.json 清单文件

### 完整字段说明

```json
{
  "manifestVersion": 1,
  "id": "com.example.my-plugin",
  "name": "我的插件",
  "version": "1.0.0",
  "description": "插件描述",
  "author": {
    "name": "作者名",
    "url": "https://example.com"
  },
  "license": "MIT",
  "main": "index.js",
  "type": "plugin",
  "hostApplication": {
    "minVersion": "1.0.0",
    "maxVersion": "2.0.0"
  },
  "sdk": {
    "minVersion": "1.0.0"
  },
  "dependencies": [
    { "type": "plugin", "id": "com.example.other-plugin", "versionSpec": ">=1.0.0" }
  ],
  "capabilities": [],
  "permissions": [
    "send.text",
    "send.image",
    "command.register",
    "hook.register",
    "tool.register",
    "event.subscribe",
    "event_handler.register",
    "http.request",
    "network.tcp",
    "plugin.file.read",
    "plugin.file.write",
    "model.access",
    "database.read",
    "emoji.access",
    "knowledge.search"
  ],
  "config": {
    "type": "yaml",
    "configFile": "config.yaml",
    "default": {
      "apiKey": "",
      "maxResults": 10
    },
    "sections": [
      {
        "name": "general",
        "label": "通用设置",
        "description": "插件的基本配置",
        "fields": [
          {
            "name": "apiKey",
            "type": "string",
            "description": "API密钥",
            "required": true,
            "placeholder": "请输入你的API密钥"
          },
          {
            "name": "maxResults",
            "type": "integer",
            "description": "最大结果数",
            "default": 10
          }
        ]
      }
    ]
  },
  "i18n": {
    "defaultLocale": "zh-CN",
    "localesPath": "locales",
    "supportedLocales": ["zh-CN", "en-US"]
  }
}
```

### Config 配置节详解

插件通过 `config.sections` 定义前端可视化配置页的结构。每个 section 渲染为一个可折叠卡片，section 内的 fields 渲染为表单控件。

**Section 字段：**

| 字段            | 类型     | 必需 | 说明                                   |
| ------------- | ------ | -- | ------------------------------------ |
| `name`        | string | 是  | 配置节名称，对应配置文件的顶级键名                    |
| `label`       | string | 否  | 配置节显示标题                              |
| `description` | string | 否  | 配置节描述文字                              |
| `icon`        | string | 否  | 配置节图标（预留）                            |
| `order`       | int    | 否  | 排序权重，越小越靠前。`plugin` 节（order=0）始终在最上层 |
| `fields`      | array  | 是  | 配置项数组                                |

**Field 支持的 type 类型：**

| type       | 前端渲染    | 说明                          | 示例                                                              |
| ---------- | ------- | --------------------------- | --------------------------------------------------------------- |
| `boolean`  | 开关按钮    | 布尔值开关                       | `{ "name": "enabled", "type": "boolean", "default": true }`     |
| `string`   | 文本输入框   | 单行文本                        | `{ "name": "apiKey", "type": "string", "placeholder": "输入密钥" }` |
| `integer`  | 数字输入框   | 整数                          | `{ "name": "maxResults", "type": "integer", "default": 10 }`    |
| `number`   | 数字输入框   | 浮点数                         | `{ "name": "temperature", "type": "number", "default": 0.7 }`   |
| `textarea` | 多行文本框   | 长文本，支持 resize               | `{ "name": "prompt", "type": "textarea", "default": "..." }`    |
| `select`   | 下拉选择框   | 选项列表，支持三种格式                 | 见下方说明                                                           |
| `array`    | 列表编辑器   | 可添加/删除的字符串列表                | `{ "name": "whitelist", "type": "array", "default": [] }`       |
| `object`   | 嵌套键值编辑器 | 键值对映射，值可为 ip/port/name 等子字段 | `{ "name": "servers", "type": "object", "default": {} }`        |

**Select 字段选项格式：**

select 类型支持三种选项格式，优先使用 `options`（label/value 格式）：

```json
// 格式一：options（推荐，支持友好标签）
{
  "name": "provider",
  "type": "select",
  "options": [
    { "label": "自动回退（推荐）", "value": "auto" },
    { "label": "和风天气", "value": "qweather" },
    { "label": "Open-Meteo", "value": "openmeteo" }
  ]
}

// 格式二：enumValues（兼容旧插件）
{
  "name": "provider",
  "type": "select",
  "enumValues": ["auto", "qweather", "openmeteo"]
}

// 格式三：optionsFrom（动态选项，选项实时来自同 section 下的对象字段）
{
  "name": "default_model_id",
  "type": "select",
  "optionsFrom": "backends",
  "optionsFromLabel": ["name", "base_url"]
}
```

| 字段                 | 类型     | 说明                                                                                |
| ------------------ | ------ | --------------------------------------------------------------------------------- |
| `options`          | array  | 选项数组，每个元素包含 `label`（显示文本）和 `value`（实际值）                                           |
| `enumValues`       | array  | 选项数组，每个元素为字符串，label 和 value 相同                                                    |
| `optionsFrom`      | string | 同 section 下某个 `object` 字段的名称（如 `backends`），选项动态取自该字段配置的键                          |
| `optionsFromLabel` | array  | 可选。`optionsFrom` 选项标签要追加的子字段名列表（如 `["name","base_url"]`）；留空时默认取 `name`、`base_url` |

**optionsFrom 动态下拉说明：**

- 选项值 = 该 `object` 字段配置里的键（如 `model1`、`model2`）；标签会按 `optionsFromLabel` 指定的子字段自动追加显示，格式为 `键 · 名称 · 接口地址`。

- `optionsFromLabel` 可自定义：不是所有插件的子对象都叫 `name`/`base_url`，例如子对象只有 `display_name` 时可写 `"optionsFromLabel": ["display_name"]`；未指定时向后兼容，默认取 `name`、`base_url`。

- 前端会**实时读取当前编辑中的配置**动态生成选项：增删改该对象字段（如删除某个后端、修改名称/地址）会即时反映到下拉框；若当前选中的值被删除，会显示为「（已失效）」。

- 适合「默认选一个后端 / 模型 / 服务器」这类与下方对象配置联动的下拉场景。

**Field 完整字段：**

| 字段            | 类型     | 必需 | 说明             |
| ------------- | ------ | -- | -------------- |
| `name`        | string | 是  | 配置项名称，对应配置中的键名 |
| `type`        | string | 是  | 控件类型，见上表       |
| `description` | string | 否  | 配置项说明文字        |
| `default`     | any    | 否  | 默认值            |
| `required`    | bool   | 否  | 是否必填（前端标记红色边框） |
| `placeholder` | string | 否  | 输入框占位提示文字      |
| `order`       | int    | 否  | 字段排序权重，越小越靠前   |

**排序规则：**

- `section_order` 由后端根据 `order` 字段排序生成，`plugin` 节（含 `enabled` 开关）始终固定在最上层

- 同 `order` 的 section/field 按 plugin.json 中的定义顺序排列

- 建议为每个 section 和 field 显式设置 `order`，确保跨插件的体验一致

### 配置参数描述规范 ⚠️

**每个参数的** **`description`** **字段必须包含示例值或格式说明**，让用户一目了然地知道应该填什么。这是强制性要求，不写示例的参数描述是不合格的。

**✅ 正确示例：**

```json
{
  "name": "permissions",
  "label": "权限设置",
  "description": "配置管理员和授权用户，格式为 平台:ID（如 qq:123456）。留空数组表示不限制。",
  "fields": [
    { "name": "admin_users", "type": "array", "description": "超级管理员列表，不可被禁言。格式: 平台:用户ID，示例: qq:用户ID", "default": [] },
    { "name": "allowed_groups", "type": "array", "description": "授权群组列表。格式: 平台:群号，示例: qq:群号", "default": [] },
    { "name": "apiKey", "type": "string", "description": "API密钥，示例: sk-abc123xyz", "default": "" },
    { "name": "maxResults", "type": "integer", "description": "最大结果数，示例: 10", "default": 10 },
    { "name": "endpoint", "type": "string", "description": "API地址，示例: https://api.example.com/v1", "default": "" }
  ]
}
```

**❌ 错误示例（缺少示例格式）：**

```json
{
  "name": "permissions",
  "description": "配置管理员和授权用户",
  "fields": [
    { "name": "admin_users", "type": "array", "description": "超级管理员列表" },
    { "name": "allowed_groups", "type": "array", "description": "授权群组列表" }
  ]
}
```

**规范要点：**

| 字段类型               | 描述要求                | 示例                                                       |
| ------------------ | ------------------- | -------------------------------------------------------- |
| `array`            | 说明数组元素格式，给 1-2 个示例值 | `"格式: 平台:用户ID，示例: qq:用户ID"`                              |
| `string`           | 说明值的格式+示例           | `"API密钥，示例: sk-abc123xyz"`                               |
| `integer`/`number` | 说明含义+范围+示例          | `"禁言时长（秒），范围60-2592000，示例: 3600"`                        |
| `boolean`          | 说明开启/关闭的效果          | `"启用后插件将自动处理消息"`                                         |
| `select`           | 说明每个选项的含义           | （可选，选项名通常自解释）                                            |
| `object`           | 说明内部字段结构和示例         | `"服务器配置，示例: {\"host\": \"127.0.0.1\", \"port\": 25565}"` |

**工具参数同样适用：** 工具定义（`tools` 数组）中的 `parameters[].description` 也必须包含示例，帮助 LLM 正确理解参数格式。

```json
{
  "tools": [{
    "name": "mute",
    "parameters": [
      { "name": "target", "type": "string", "description": "要禁言的目标用户名，示例: 迁就", "required": true },
      { "name": "duration", "type": "integer", "description": "禁言时长（秒），范围60-2592000，示例: 3600（1小时）", "required": true },
      { "name": "reason", "type": "string", "description": "禁言原因，示例: 刷屏、发布违规内容", "required": true }
    ]
  }]
}
```

### 全部权限列表

| 权限                         | 说明                           |
| -------------------------- | ---------------------------- |
| `event.subscribe`          | 订阅事件                         |
| `event.publish`            | 发布事件                         |
| `hook.register`            | 注册 Hook                      |
| `command.register`         | 注册指令                         |
| `tool.register`            | 注册工具                         |
| `event_handler.register`   | 注册事件处理器                      |
| `llm_provider.register`    | 注册 LLM 提供商                   |
| `api.register`             | 注册自定义 API                    |
| `api.call`                 | 调用其他插件 API                   |
| `send.text`                | 发送文本消息                       |
| `send.image`               | 发送图片消息                       |
| `send.emoji`               | 发送表情包消息                      |
| `send.hybrid`              | 发送图文混合消息                     |
| `send.forward`             | 发送合并转发消息                     |
| `emoji.access`             | 访问表情包系统                      |
| `emoji.get_random`         | 获取随机表情                       |
| `emoji.get_by_description` | 按描述获取表情                      |
| `database.read`            | 读取数据库                        |
| `database.write`           | 写入数据库                        |
| `plugin.config.read`       | 读取插件配置                       |
| `plugin.config.write`      | 写入插件配置                       |
| `plugin.file.read`         | 读取插件文件                       |
| `plugin.file.write`        | 写入插件文件                       |
| `network.http`             | 网络请求                         |
| `network.tcp`              | TCP Socket 连接                |
| `network.udp`              | UDP Socket 通信                |
| `encoding.use`             | 编解码（base64/hex/URL/UTF-8）    |
| `time.use`                 | 时间工具（格式化/解析/时长计算）            |
| `crypto.use`               | 加解密/哈希（MD5/SHA1/SHA256/HMAC） |
| `http.request`             | HTTP 请求                      |
| `model.access`             | 调用 LLM 模型                    |
| `async_task.execute`       | 执行异步任务                       |
| `chat.get_streams`         | 获取聊天流                        |
| `chat.open_session`        | 打开聊天会话                       |
| `person.get_info`          | 获取用户信息                       |
| `frequency.read`           | 读取频率数据                       |
| `frequency.write`          | 写入频率数据                       |
| `component.manage`         | 管理组件                         |
| `message.history`          | 访问历史消息                       |
| `llm.generate`             | 调用 LLM 生成文本                  |
| `llm.generate_with_tools`  | 调用 LLM 生成（带工具调用）             |
| `llm.embed`                | 生成文本嵌入向量                     |
| `llm.get_available_models` | 获取可用模型列表                     |
| `knowledge.search`         | 搜索知识库                        |
| `render.html2png`          | 渲染 HTML 为图片                  |
| `data.directory.read`      | 读取插件数据目录                     |
| `data.directory.write`     | 写入插件数据目录                     |
| `platform.command`         | 发送平台级命令                      |

***

## 生命周期函数

插件必须在全局作用域定义以下函数（可选）：

### onLoad()

插件加载完成后调用，用于初始化。

```javascript
function onLoad() {
  yara.logger.info("插件初始化完成");
  // 注册 Hook、命令、工具等
}
```

### onUnload()

插件卸载前调用，用于清理资源。

```javascript
function onUnload() {
  yara.logger.info("插件正在清理资源");
  // 保存状态、关闭连接等
}
```

### onConfigUpdate(scope, config, version)

插件配置文件（`config.yaml` / `config.yml` / `config.toml` / `config.json`）发生变更时调用，用于热更新配置。

**注意：配置变更不会卸载/重载插件，而是直接调用本函数。** 这样正在执行的后台异步任务（如生图）不会被中途打断。插件应在该函数中重新读取配置并更新内存状态。

```javascript
function onConfigUpdate(scope, config, version) {
  yara.logger.info("配置已更新，作用域: " + scope);
  // config 参数为系统解析后的配置对象；也可自行重新读取配置文件
  myConfig.apiKey = config.apiKey;
}
```

> 若插件未实现 `onConfigUpdate`，则配置变更时系统会退化为整体重载插件以保证配置生效。**耗时工具型插件（声明** **`async: true`）建议实现** **`onConfigUpdate`**，否则在配置页保存配置会导致后台任务被重载打断。

***

## 完整 API 参考

> **`yara.*`** **是什么？** `yara` 是 YaraFlow 主程序注入到每个插件沙箱的**全局 API 命名空间**，不是某个具体的人或架构名。插件里所有与主程序交互的能力都挂在它下面（`yara.logger`、`yara.send`、`yara.network` 等），类似 Node.js 里的 `process`/`global`，只是更精简、只暴露插件权限内允许的能力。

**API 索引（快速跳转）：**

| API                                          | 说明                                   |
| -------------------------------------------- | ------------------------------------ |
| [yara.logger](#yara-logger--日志)              | 日志输出                                 |
| [yara.send](#yara-send--消息发送)                | 消息发送                                 |
| [yara.event](#yara-event--事件)                | 事件发布/订阅                              |
| [yara.hook](#yara-hook--hook)                | 消息处理链拦截/观察                           |
| [yara.command](#yara-command--指令)            | 指令注册                                 |
| [yara.http](#yara-http--http-请求)             | HTTP 请求                              |
| [yara.network](#yara-network--网络通信tcpudpdns) | TCP/UDP/DNS 网络通信                     |
| [yara.platform](#yara-platform--平台操作)        | 平台命令/用户查找                            |
| [yara.encoding](#yara-encoding--编解码)         | base64/hex/URL/UTF-8 编解码             |
| [yara.time](#yara-time--时间工具)                | 时间工具                                 |
| [yara.crypto](#yara-crypto--加解密哈希)           | 加解密/哈希                               |
| [yara.model](#yara-model--llm-模型调用)          | LLM 模型调用                             |
| [yara.config](#yara-config--配置)              | 配置读写                                 |
| [yara.database](#yara-database--数据库查询)       | 数据库查询                                |
| [yara.file](#yara-file--文件操作)                | 文件操作                                 |
| [yara.tool](#yara-tool--工具)                  | 工具注册/管理                              |
| [yara.image](#yara-image--图片读取校验)            | 图片读取/校验                              |
| [其他 API](#其他-api)                            | async/api/eventHandler/llmProvider 等 |

### yara.logger — 日志

```javascript
// 信息日志
yara.logger.info("这是一条信息");
yara.logger.info("用户 {id} 发送了消息", { id: "123" });

// 警告日志
yara.logger.warn("这是一条警告");

// 错误日志
yara.logger.error("发生了错误");

// 调试日志
yara.logger.debug("调试信息");
```

**所需权限：** 无需声明（始终可用）

***

### yara.send — 消息发送

```javascript
// 发送文本消息
// 权限：send.text
yara.send.text("123456", "你好！");

// 发送图片消息
// 权限：send.image
yara.send.image("123456", "https://example.com/image.png");

// 发送表情包消息
// 权限：send.emoji
yara.send.emoji("123456", "emoji_hash_value");

// 发送图文混合消息
// 权限：send.hybrid
yara.send.hybrid("123456", [
  { type: "text", content: "这是一段文字" },
  { type: "image", content: "https://example.com/pic.png" },
  { type: "emoji", content: "emoji_hash" }
]);
```

**注意：** 所有发送方法第一个参数 `groupID` 为必填，表示目标群组ID。可通过命令处理器的第二个参数 `context.groupId` 获取当前群组ID。

**返回值：** 所有发送方法返回 `true`（成功）或 `false`（失败）。

***

### yara.event — 事件

```javascript
// 订阅事件（事件名建议用 YaraEvents 常量，无需记忆裸字符串）
// 权限：event.subscribe
yara.event.subscribe(YaraEvents.ON_MESSAGE, function(eventData) {
  yara.logger.info("收到消息: " + eventData.content);
});

// 发布事件（自定义事件名也可用字符串）
// 权限：event.publish
yara.event.publish("custom.event", { key: "value" });
```

**支持的事件类型：** `ON_START`, `ON_STOP`, `ON_MESSAGE_PRE_PROCESS`, `ON_MESSAGE`, `ON_PLAN`, `POST_LLM`, `AFTER_LLM`, `POST_SEND_PRE_PROCESS`, `POST_SEND`, `AFTER_SEND`（对应全局常量 `YaraEvents.ON_START` 等，见 [类型定义与 IDE 补全](#类型定义与-ide-补全yaradts)）

***

### yara.hook — Hook

```javascript
// 注册 Hook
// 权限：hook.register
// 用法1: hook.register(type, handler, options?)
yara.hook.register("chat.receive.before_process", function(event) {
  return { allowContinue: true };
}, {
  mode: "blocking",
  order: "normal",
  errorPolicy: "log",
  timeoutMs: 8000
});

// 用法2: hook.register(type, options, handler)
yara.hook.register("chat.replyer.after_response", {
  mode: "blocking",
  order: "normal"
}, function(event) {
  return { allowContinue: true };
});
```

**Hook 参数说明：**

| 参数            | 类型     | 说明                                |
| ------------- | ------ | --------------------------------- |
| `hookType`    | string | Hook 点名称                          |
| `mode`        | string | `blocking`（阻塞）或 `observe`（观察）     |
| `order`       | string | `early`（早）、`normal`（默认）、`late`（晚） |
| `errorPolicy` | string | `abort`（中止）、`skip`（跳过）、`log`（记录）  |
| `timeoutMs`   | number | 超时毫秒数，默认 8000                     |

**Hook 事件对象 (event)：**

| 字段        | 类型     | 说明                                                |
| --------- | ------ | ------------------------------------------------- |
| `type`    | string | Hook 类型名称                                         |
| `message` | object | 当前处理的消息对象，包含 `content`、`senderName`、`groupId` 等字段 |
| `context` | object | 附加上下文，包含 `session`（聊天会话）等信息                       |

**Hook 返回值：**

```javascript
{
  allowContinue: true,      // 是否允许继续后续处理
  action: "abort",          // 可选，"abort" 表示中止处理，等价于 allowContinue: false
  modifiedData: {           // 可选，修改消息内容
    content: "替换后的内容",
    senderName: "新的发送者名称"
  }
}
```

**消息替换示例（将链接替换为 LLM 可读内容）：**

```javascript
// 在消息处理前替换链接内容
yara.hook.register("chat.receive.before_process", function(event) {
  var msg = event.message;
  // 检测链接并替换为网页内容
  if (msg.content && msg.content.includes("https://")) {
    var urlMatch = msg.content.match(/https?:\/\/\S+/);
    if (urlMatch) {
      // 调用 HTTP API 获取链接内容
      var response = yara.http.get(urlMatch[0]);
      var pageContent = response.status === 200 ? response.body : "[无法获取链接内容]";
      return {
        allowContinue: true,
        modifiedData: {
          content: "[用户分享了链接，内容如下]\n" + pageContent.substring(0, 1000)
        }
      };
    }
  }
  return { allowContinue: true };
}, { mode: "blocking" });
```

**注意：**

- `modifiedData` 仅在 `chat.receive.before_process` 和 `chat.receive.after_process` 两个 Hook 点生效

- 修改后的消息会存入聊天历史，LLM 在回复时会看到修改后的内容

- 在 Hook 处理器中调用 `yara.send.text()` 等发送方法时，消息会自动记入聊天历史（因为 session 上下文已传入）

***

### yara.command — 指令

```javascript
// 注册指令
// 权限：command.register
// 命令处理器签名: function(match, context)
// - match: 正则匹配结果对象
// - context: { platform: "qq", groupId: "123456" } — 当前聊天上下文
yara.command.register("mycmd", "/mycmd\\s+(.*)", function(match, context) {
  var arg = match[1];
  var groupId = context ? context.groupId : "default";
  yara.send.text(groupId, "你输入了: " + arg);
  return "done";
});
```

**注意：** 在命令处理器中调用 `yara.send.text()` 等发送方法时，消息会自动记入聊天历史，智能体能通过聊天历史看到自己发送的命令结果。

**参数说明：**

- `name`：指令名称（唯一标识）

- `pattern`：正则表达式模式

- `handler`：处理函数，接收 `match`（正则匹配结果）和 `context`（聊天上下文），返回字符串

- `options`：可选，`{ aliases: ["/别名1", "/别名2"] }`

***

### yara.http — HTTP 请求

```javascript
// 权限：http.request

// GET 请求
var response = yara.http.get("https://api.example.com/data");
// 返回: { status: 200, statusText: "OK", body: "...", headers: {...} }

// GET 请求（带自定义请求头）
var response = yara.http.get("https://api.example.com/data", {
  "Authorization": "Bearer token",
  "Accept": "application/json"
});

// POST 请求
var response = yara.http.post("https://api.example.com/submit", { key: "value" });

// POST 请求（带自定义请求头）
var response = yara.http.post("https://api.example.com/submit", { key: "value" }, {
  "Authorization": "Bearer token"
});

// 下载文件到插件 data 目录（默认行为，未指定路径时自动使用）
// 如果插件目录下没有 data 文件夹，下载模块会自动创建
var result = yara.http.download("https://example.com/file.pdf");
// 返回: { success: true, path: "...", size: 1024, fileSize: 1024 }

// 下载到插件内指定路径（相对于插件 data 目录）
var result = yara.http.download("https://example.com/file.pdf", "downloads/file.pdf");
```

**超时说明：** HTTP 请求默认超时时间为 **120 秒**（生图等耗时接口需要较长等待时间）。可通过 `get` / `post` / `download` 的**最后一个数字参数**自定义单次请求超时（单位：秒）：

```javascript
// 默认 120 秒超时
var r1 = yara.http.get("https://api.example.com/data");

// 自定义超时：把秒数作为最后一个数字参数传入（get 可省略中间参数）
var r2 = yara.http.get("https://api.example.com/slow", 300);
var r3 = yara.http.get("https://api.example.com/slow", { "Authorization": "Bearer token" }, 300);
var r4 = yara.http.post("https://api.example.com/submit", { key: "value" }, { "Authorization": "Bearer token" }, 300);
var r5 = yara.http.download("https://example.com/big.png", "downloads/big.png", 300);
```

***

### yara.network — 网络通信（TCP/UDP/DNS）

goja 沙箱中没有 Node.js 的 `net` 和 `dns` 模块，因此 YaraFlow 提供了通用的网络通信 API，支持 TCP Socket、UDP Socket 和 DNS 解析。

`send()`/`receive()` 的二进制数据支持三种写法：**字符串（UTF-8 文本）**、**整数数组**（兼容旧插件，如 `[0xFE, 0x01]`）、**`Uint8Array`/`ArrayBuffer`**（goja 原生支持）。涉及二进制协议时推荐配合 `yara.encoding.base64Decode` 解码后再发，接收到的字节用 `yara.encoding.base64Encode` 编码，不必手写/手读整数数组。

```javascript
// ─── DNS 解析 ───
// 无需权限声明（始终可用）

// 解析域名 A 记录
var addrs = yara.network.resolveDNS("example.com", 5);
// 返回: ["1.2.3.4", "5.6.7.8"] 或 { error: "..." }

// 解析 SRV 记录（Minecraft 等游戏常用）
var srv = yara.network.resolveSRV("minecraft", "tcp", "example.com", 5);
// 返回: { target: "mc.example.com", port: 25565 } 或 { error: "..." }

// ─── TCP Socket ───
// 权限：network.tcp

// 建立 TCP 连接
var sock = yara.network.tcpConnect("example.com", 25565, 10);
// 参数: host, port, timeoutSec(可选，默认10秒)
// 返回: socket 对象 或 { error: "..." }

// 发送数据（字符串 / 整数数组 / Uint8Array / base64 解码均可）
sock.send("hello");                             // 发送 UTF-8 字符串
sock.send([0xFE, 0x01, 0xFA]);                  // 发送整数数组（兼容旧插件）
sock.send(new Uint8Array([0xFE, 0x01, 0xFA]));  // 发送 typed array（goja 原生支持）
sock.send(yara.encoding.base64Decode("AP4B+g==")); // 发送 base64 解码的二进制（推荐）

// 接收数据
var data = sock.receive(5);                     // 返回整数数组（兼容旧插件），timeoutSec 可选
var b64 = yara.encoding.base64Encode(data);     // 转成 base64，方便打印/转发/落库
var text = sock.receiveString(5);               // 直接接收 UTF-8 字符串

// 关闭连接
sock.close();

// ─── UDP Socket ───
// 权限：network.udp

// 创建已连接的 UDP Socket（目标地址固定）
var sock = yara.network.udpConnect("example.com", 25565, 10);
// 参数: host, port, timeoutSec(可选，默认10秒)

// 创建监听 UDP Socket（可收发任意地址）
var sock = yara.network.udpListen("0.0.0.0", 0);
// 参数: host(可选，默认"0.0.0.0"), port(可选，默认0自动分配)
// 返回的对象包含 localAddr 属性

// 发送数据到已连接地址
sock.send([0xFE, 0xFD, 0x09]);       // 发送字节数组
sock.sendString("hello");             // 发送字符串

// 发送数据到指定地址
sock.sendTo([0xFE, 0xFD], "example.com", 25565);
sock.sendToString("hello", "example.com", 25565);

// 接收数据
var data = sock.receive(5);           // 返回整数数组
var text = sock.receiveString(5);     // 返回字符串

// 接收数据并获取发送方地址
var result = sock.receiveFrom(5);     // 返回 { data: [...], host: "...", port: 12345 }
var result = sock.receiveFromString(5); // 返回 { data: "...", host: "...", port: 12345 }

// 关闭连接
sock.close();
```

**返回值约定：** 所有 socket 方法在出错时返回 `{ error: "错误描述" }`，成功时返回对应的数据或 `{ success: true }`。

**注意：** TCP/UDP Socket 有 SSRF 防护，禁止连接内网地址。插件的网络权限应谨慎授予。

***

### yara.platform — 平台操作

提供平台级命令发送、用户查找等能力。

```javascript
// 权限：platform.command

// 发送平台级命令
var result = yara.platform.sendCommand("mute", {
  groupId: "123456",
  userId: "789012",
  duration: 600
});
// 返回: { success: true } 或 { success: false, error: "..." }

// 获取当前平台名称
var name = yara.platform.getName();
// 返回: "qq"、"telegram" 等

// 获取当前会话的群组 ID
var groupId = yara.platform.getGroupId();

// 根据显示名称查找用户 ID
var userId = yara.platform.lookupUser("123456", "用户昵称");
// 返回: 用户 ID 字符串，或 null（未找到）
```

**注意：** `sendCommand` 的具体命令由平台驱动实现，不同平台支持的命令可能不同。

***

### yara.encoding — 编解码

提供 base64、hex、URL、UTF-8 编解码能力。纯计算，无副作用，无需网络或文件权限。

```javascript
// 权限：encoding.use

// Base64 编解码
var encoded = yara.encoding.base64Encode("Hello World");  // 输入字符串或整数数组，返回字符串
var decoded = yara.encoding.base64Decode(encoded);         // 输入 base64 字符串，返回整数数组

// Hex 编解码
var hex = yara.encoding.hexEncode([0x48, 0x65, 0x6C]);   // 输入整数数组，返回 hex 字符串
var bytes = yara.encoding.hexDecode("48656C");             // 输入 hex 字符串，返回整数数组

// URL 编解码
var encoded = yara.encoding.urlEncode("你好 世界");       // 输入字符串，返回 URL 编码字符串
var decoded = yara.encoding.urlDecode("%E4%BD%A0%E5%A5%BD"); // 输入 URL 编码字符串，返回解码字符串

// UTF-8 编解码
var utf8Bytes = yara.encoding.utf8Encode("你好");          // 输入字符串，返回整数数组 [228, 189, 160, 229, 165, 189]
var str = yara.encoding.utf8Decode([228, 189, 160, 229, 165, 189]); // 输入整数数组，返回字符串 "你好"
```

**关于二进制数据的表示：** `base64Decode`、`hexDecode` 和 `utf8Encode` 返回**整数数组**（如 `[0x48, 0x65, 0x6C]`）。这是兼容旧插件的表示，goja 原生支持 `Uint8Array`，也可用 `new Uint8Array(bytes)` 转成 typed array 再使用。**跨接口传递二进制数据时，base64 是推荐格式**：`base64Encode` 编码输出、`base64Decode` 解码输入，正好与 `yara.network` 的 `send`/`receive`、`yara.image` 等接口衔接，避免手写/手读整数数组。如需将字节数组转为字符串，使用 `yara.encoding.utf8Decode(bytes)` 而非 `String.fromCharCode(...)` 逐个转换，后者无法正确处理中文等多字节字符。

***

### yara.time — 时间工具

提供时间戳获取、格式化、时长计算和解析能力。纯计算，无副作用，无需网络或文件权限。

```javascript
// 权限：time.use

// 获取当前时间戳
var now = yara.time.now();       // Unix 秒级时间戳，如 1720950000
var nowMs = yara.time.nowMs();   // Unix 毫秒级时间戳，如 1720950000123

// 格式化时间戳为字符串
// layout 使用 Go 风格模板：2006-01-02 15:04:05
var str = yara.time.format(1720950000, "2006-01-02 15:04:05");
// → "2024-07-14 18:20:00"

// 格式化时长（秒 → 人类可读）
var dur = yara.time.formatDuration(3661);
// → "1小时1分钟1秒"

// 解析时间字符串为时间戳
var ts = yara.time.parse("2024-07-14 18:20:00", "2006-01-02 15:04:05");
// → 1720950000

// 同步睡眠指定毫秒数（延时操作，如延迟发送消息）
// 可被插件超时安全打断（不会永久卡死），睡眠期间该插件的其他调用会等待
yara.time.sleep(1000);   // 阻塞 1 秒
yara.send.text("group-123", "延迟消息");
```

> **延时说明：** 沙箱没有事件循环，`setTimeout`/`setInterval` 不可用。需要"等一会儿再继续"用 `yara.time.sleep(ms)`；超过同步超时上限的耗时工作用 `yara.async.run()`（后台执行，不阻塞）。

**Go 风格 layout 常用模板：**

| 模板                      | 示例输出                  |
| ----------------------- | --------------------- |
| `"2006-01-02 15:04:05"` | `2024-07-14 18:20:00` |
| `"2006-01-02"`          | `2024-07-14`          |
| `"15:04:05"`            | `18:20:00`            |
| `"2006/01/02 15:04"`    | `2024/07/14 18:20`    |
| `"2006年01月02日"`         | `2024年07月14日`         |

***

### yara.crypto — 加解密/哈希

提供 MD5、SHA1、SHA256 哈希、HMAC 签名、Ed25519 签名和 JWT 生成能力。纯计算，无副作用，无需网络或文件权限。

```javascript
// 权限：crypto.use

// 哈希（输入字符串 / 整数数组 / Uint8Array，返回 hex 字符串）
var md5Hash = yara.crypto.md5("hello");         // "5d41402abc4b2a76b9719d911017c592"
var sha1Hash = yara.crypto.sha1("hello");       // "aaf4c61ddcc5e8a2dabede0f3b482cd9aea9434d"
var sha256Hash = yara.crypto.sha256("hello");   // "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c..."

// HMAC 签名（key 和 data 均支持字符串 / 整数数组 / Uint8Array，返回 hex 字符串）
var hmacSha1 = yara.crypto.hmacSha1("secret-key", "data-to-sign");
var hmacSha256 = yara.crypto.hmacSha256("secret-key", "data-to-sign");

// Ed25519 签名（输入私钥和数据，返回 base64url 编码签名）
// 私钥支持：PEM 格式（PKCS8）或原始 64 字节
var pemKey = "-----BEGIN PRIVATE KEY-----\nMIG2AgEAMB...\n-----END PRIVATE KEY-----";
var sig = yara.crypto.ed25519Sign(pemKey, "data-to-sign");

// JWT 生成（使用 Ed25519 签名）
// 签名算法：EdDSA
// 参数: claims(对象), privateKey(PEM或原始64字节), keyID(密钥ID)
var jwt = yara.crypto.generateJWT(
  { sub: "project-id", iat: Math.floor(Date.now() / 1000), exp: Math.floor(Date.now() / 1000) + 900 },
  pemKey,
  "your-key-id"
);
// 返回: "eyJhbGciOiJFZERTQSIsImtpZCI6InlvdXI..."
```

**JWT Header 结构：**

```json
{
  "alg": "EdDSA",
  "kid": "your-key-id"
}
```

**JWT Payload 常用字段：**

| 字段    | 说明             | 示例                                    |
| ----- | -------------- | ------------------------------------- |
| `sub` | 主题，通常为项目 ID    | `"project-id-123"`                    |
| `iat` | 签发时间（Unix 时间戳） | `1704067200`                          |
| `exp` | 过期时间（Unix 时间戳） | `1704068100`（建议设置为 iat + 900，即 15 分钟） |

**典型用途：** 对接第三方 API 需要签名验证、计算文件哈希校验、生成 JWT 令牌进行 API 认证等场景。

***

### yara.model — LLM 模型调用

```javascript
// 权限：model.access（所有 yara.model 下的方法共用此权限）

// 基本聊天调用
var response = yara.model.chat({
  messages: [
    { role: "system", content: "你是一个助手" },
    { role: "user", content: "你好" }
  ],
  taskType: "replyer"
});
// 返回: { content: "你好！有什么可以帮助你的？" }

// 指定任务类型聊天
var response = yara.model.chatWithTask("replyer", [
  { role: "user", content: "你好" }
]);

// 自定义配置聊天
var response = yara.model.chatWithConfig({
  messages: [{ role: "user", content: "你好" }],
  baseUrl: "https://api.openai.com",
  apiKey: "sk-xxxxx",
  model: "gpt-4",
  temperature: 0.7,
  timeout: 120
});

// 带工具调用的聊天
var response = yara.model.chatWithTools({
  messages: [{ role: "user", content: "今天天气怎么样？" }],
  tools: [{
    type: "function",
    function: {
      name: "get_weather",
      description: "获取天气信息",
      parameters: {
        type: "object",
        properties: { city: { type: "string", description: "城市名称" } },
        required: ["city"]
      }
    }
  }],
  taskType: "replyer"
});

// 获取文本嵌入向量
var result = yara.model.embed({ text: "你好世界", taskType: "replyer" });

// 批量嵌入
var result = yara.model.embed({ texts: ["文本1", "文本2", "文本3"], taskType: "replyer" });

// 获取可用模型列表
var models = yara.model.getAvailableModels();

// 获取模型配置
var config = yara.model.getConfig("replyer");
var allConfigs = yara.model.getAllConfigs();
var configs = yara.model.getAvailableConfigs();
var tasks = yara.model.listTasks();
```

***

### yara.config — 配置

```javascript
// 读取插件配置文件（YAML/JSON/TOML）
// 由 plugin.json 中 config.configFile 指定
// 权限：plugin.config.read
var config = yara.config.getFile();
// 返回整个配置文件解析后的对象

// 将配置对象序列化后写回配置文件（格式与 configFile 保持一致）
// 权限：plugin.config.write
yara.config.setFile(config);
```

***

### yara.database — 数据库查询

```javascript
// 权限：database.read

// 查询最近消息
var messages = yara.database.queryMessages({
  platform: "qq",
  groupID: "123456",
  limit: 20
});

// 搜索消息
var result = yara.database.searchMessages({
  platform: "qq",
  groupID: "123456",
  query: "关键词",
  limit: 50,
  offset: 0
});

// 获取用户消息
var messages = yara.database.getUserMessages({
  platform: "qq",
  userID: "user123",
  limit: 50
});

// 获取用户信息
var userInfo = yara.database.getUserInfo({
  platform: "qq",
  userID: "user123"
});
```

***

### yara.file — 文件操作

```javascript
// 权限：plugin.file.read / plugin.file.write

// 读取文件
var content = yara.file.read("data/cache.json");

// 写入文件
yara.file.write("data/cache.json", JSON.stringify(data));

// 数据目录操作（data/ 为插件运行时数据目录，写入不会触发热重载，也不会打断后台异步任务）
var cache = yara.file.readData("cache.json");
yara.file.writeData("cache.json", JSON.stringify(data));
var files = yara.file.listData("downloads/");
var dataPath = yara.file.getDataPath();
```

> **数据目录说明：** `data/` 用于存放插件自身的运行时数据（计数器、缓存、下载文件等）。系统热重载监听**完全忽略** `data/` 目录的写入，因此后台异步任务执行期间写入数据不会触发插件重载或打断任务。该目录可能被清理，请勿存放需要持久化的配置。

***

### yara.tool — 工具

> **注意：** 旧的 `yara.action` 动作系统已合并到工具系统中，请使用 `yara.tool.register()` 代替。

```javascript
// 注册 Agent 工具
// 权限：tool.register
// 签名: yara.tool.register(name, definition, handler)
// - name: 工具名称，LLM 通过此名称调用
// - definition: 工具定义对象，包含 description 和 parameters
// - handler: 工具处理器函数，签名: function(params, context)
//   - params: LLM 传入的参数对象，如 { city: "北京" }
//   - context: 聊天上下文，包含 { platform: "qq", groupId: "123456", messageId: "...", userId: "...", senderName: "..." }
yara.tool.register("get_weather", {
  description: "查询指定城市的实时天气和未来2天预报",
  parameters: [
    { name: "city", type: "string", description: "城市名称，如北京、上海", required: true }
  ]
}, function(params, context) {
  return { city: params.city, temperature: 25, condition: "晴" };
});

// 注册自主运行工具
// 签名: yara.tool.registerAutonomous(definition)
// definition 必须包含 name 和 handler 字段
yara.tool.registerAutonomous({
  name: "auto_intercept",
  description: "自动检测并拦截B站视频链接",
  hookType: "chat.receive.before_process",
  pattern: "bilibili\\.com/video/(BV\\w+)",
  handler: function(message) {
    return {
      intercepted: false,
      injectedContent: "视频摘要内容..."
    };
  }
});

// 获取当前插件已注册的所有工具定义
// 权限：tool.register
var definitions = yara.tool.getDefinitions();
// 返回: [{ name: "get_weather", description: "...", parameters: [...] }, ...]
```

***

### yara.image — 图片读取/校验

```javascript
// 权限：plugin.file.read
// 用于读取/校验图片，尤其用于参考图、自我形象等场景。
// 注意：以下接口都在主程序 Go 侧按真实字节校验，务必优先使用，
// 不要自行用 JS 的 indexOf("\x89PNG") 判断图片格式 —— 二进制经 Go→JS 字符串传递时，
// 非 UTF-8 头字节（如 PNG 的 0x89、JPEG 的 0xFF）会被替换成 U+FFFD，导致判断永远失败。

// 1) 获取主程序已缓存/可下载的图片（按图片 URL）
//    优先复用主程序 data/images/ 已下载保存的缓存（QQ 临时链接过期也能用）；
//    缓存缺失/被清理时自动尝试重新下载，均失败才返回 null。
var b64 = yara.image.getCached(context.image_urls[0]);
if (b64) { /* 得到图片原始字节的 base64 字符串 */ }

// 2) 读取并校验插件自身目录内的图片文件（相对插件根目录，语义同 yara.file.read）
//    按真实字节校验是否为有效图片（PNG/JPEG/GIF/WebP/BMP），有效则返回 base64，否则返回 null。
var selfieB64 = yara.image.loadValid("data/selfie.png");
if (selfieB64) {
  // 自我形象图片读取成功，可直接作为参考图提交
}

// 3) 校验一段 base64 原始字节是否为有效图片头（布尔）
var valid = yara.image.isImage(b64);
```

> **参考图推荐用法：** 用 `context.image_urls` 里的 URL 调 `yara.image.getCached`。主程序在图片到达时已落盘并记录 URL→文件映射（落库，程序重启不丢）；即使文件被 7 天清理定时任务删除，接口也会尝试重新下载，尽可能避开 QQ 临时链接过期导致的"无效图片"报错。

***

### 其他 API

```javascript
// 表情包
var emoji = yara.emoji.getRandom();
var emojis = yara.emoji.getByEmotion("开心");

// 知识库
var result = yara.knowledge.search({ query: "技术问题", limit: 5 });

// 异步任务
var taskId = yara.async.run(function(task) { return "完成"; }, { timeout: 300000 });

// 自定义 API 暴露
yara.api.register("getUserProfile", function(params) { return { name: "用户" }; });

// 调用其他插件 API（跨插件调用）
// 权限：api.call
var result = yara.api.call("com.example.plugin.method", { key: "val" });

// LLM 提供商注册
yara.llmProvider.register("custom_model", function(params) { return { content: "..." }; });

// 事件处理器
yara.eventHandler.register("onMessage", "ON_MESSAGE", function(eventData) { ... });

// 调试输出
console.log("调试信息");
```

***

## 指令 (Command) 开发

### 完整示例

```javascript
// 命令处理器接收两个参数：
// match: 正则匹配结果
// context: { platform: "qq", groupId: "123456" }

yara.command.register("translate", "/翻译\\s+(.+)", function(match, context) {
  var text = match[1];
  var groupId = context ? context.groupId : "default";
  var result = yara.http.get("https://api.example.com/translate?q=" + encodeURIComponent(text));
  var data = JSON.parse(result.body);
  yara.send.text(groupId, "翻译结果: " + data.translated);
  return "done";
});
```

***

## 工具 (Tool) 开发

工具是 LLM 可调用的函数，通过 `yara.tool.register()` 注册。工具分为三种类型：

| 类型       | toolType     | 说明                                                  |
| -------- | ------------ | --------------------------------------------------- |
| Agent 工具 | `agent`      | LLM 在推理过程中主动调用，通过 `yara.tool.register()` 的 `definition.parameters` 声明参数 schema |
| 自主运行工具   | `autonomous` | 系统自动触发（如消息拦截），无需 LLM 调用，必须设置 `visibility: "hidden"` |
| 核心工具     | `core`       | 主程序核心功能，始终暴露给 LLM，插件不可注册此类型（仅供系统内置工具使用）             |

### Agent 工具完整示例

```javascript
// 天气查询工具
// 权限：tool.register
yara.tool.register("get_weather", {
  description: "查询指定城市的实时天气和未来2天预报",
  parameters: [
    { name: "city", type: "string", description: "城市名称，示例: 北京、上海", required: true },
    { name: "lang", type: "string", description: "返回语言，示例: zh-CN、en-US", required: false }
  ]
}, function(params, context) {
  // params: LLM 传入的参数，如 { city: "北京" }
  // context: { platform: "qq", groupId: "123456", messageId: "...", userId: "...", senderName: "..." }
  var config = yara.config.getFile();
  var apiKey = config.weatherApiKey;
  var resp = yara.http.get("https://api.weather.com/v1/current?city=" + encodeURIComponent(params.city) + "&key=" + apiKey);
  if (resp.status !== 200) {
    return { error: "天气查询失败，状态码: " + resp.status };
  }
  var data = JSON.parse(resp.body);
  return {
    city: params.city,
    temperature: data.current.temp_c,
    condition: data.current.condition.text,
    humidity: data.current.humidity
  };
});
```

### 自主运行工具完整示例

```javascript
// 自动检测并拦截B站视频链接
// 权限：tool.register
yara.tool.registerAutonomous({
  name: "auto_bilibili_intercept",
  description: "自动检测并解析B站视频链接",
  hookType: "chat.receive.before_process",
  pattern: "bilibili\\.com/video/(BV\\w+)",
  handler: function(message) {
    var bvMatch = message.content.match(/bilibili\.com\/video\/(BV\w+)/);
    if (!bvMatch) return { intercepted: false };

    var bv = bvMatch[1];
    var resp = yara.http.get("https://api.bilibili.com/x/web-interface/view?bvid=" + bv);
    if (resp.status !== 200) return { intercepted: false };

    var data = JSON.parse(resp.body).data;
    var summary = "【B站视频】" + data.title + "\n" +
                  "UP主: " + data.owner.name + "\n" +
                  "播放: " + data.stat.view + " | 弹幕: " + data.stat.danmaku;

    return {
      intercepted: true,
      injectedContent: summary
    };
  }
});
```

### 工具定义规范

```javascript
// yara.tool.register() 的 definition 参数（工具定义对象），注册示例：
// yara.tool.register("get_weather", {
//   description: "...",
//   visibility: "visible",
//   toolType: "agent",
//   timeoutSeconds: 60,
//   parameters: [...]
// }, function(params, context) { ... });
{
  "name": "get_weather",
  "description": "查询指定城市的实时天气和未来2天预报",
  "briefDescription": "天气查询",
  "detailedDescription": "根据城市名称查询当前天气状况，包括温度、湿度、天气状况等",
  "visibility": "visible",
  "toolType": "agent",
  "timeoutSeconds": 60,
  "parameters": [
    { "name": "city", "type": "string", "description": "城市名称，示例: 北京", "required": true }
  ]
}
```

| 字段                    | 类型     | 必需 | 说明                                                                                                  |
| --------------------- | ------ | -- | --------------------------------------------------------------------------------------------------- |
| `name`                | string | 是  | 工具名称，LLM 通过此名称调用                                                                                    |
| `description`         | string | 是  | 工具描述，LLM 据此判断何时调用                                                                                   |
| `briefDescription`    | string | 否  | 简短描述，前端展示用                                                                                          |
| `detailedDescription` | string | 否  | 详细描述，帮助用户理解工具用途                                                                                     |
| `visibility`          | string | 是  | `visible`（LLM 可主动调用）、`hidden`（LLM 隐藏，仅程序内部/自主运行）或 `deferred`（延迟可见，默认不暴露给 LLM，由 Planner 按需搜索发现后动态注入） |
| `toolType`            | string | 是  | `agent`（LLM调用）或 `autonomous`（自主运行）                                                                  |
| `timeoutSeconds`      | int    | 否  | 工具执行超时（秒）。主程序对每次工具调用设兜底看门狗超时：超过该值会中断执行并返回错误。未声明时默认 300 秒。建议耗时工具显式声明一个合理值（如生图 300）                   |
| `async`               | bool   | 否  | 异步工具：调用后主程序立即返回"任务已启动"，耗时工作后台执行，完成后自动发送结果并通知 Planner。适合生图等耗时操作，避免阻塞对话                               |
| `parameters`          | array  | 是  | 参数定义数组，格式同 OpenAI function calling                                                                  |

**异步工具说明：**

- 声明 `async: true` 后，主程序调用工具会立即返回，不会阻塞后续对话。

- 工具函数在后台 goroutine 中执行，完成后的返回字符串会通过"上下文通知"告知 Planner，Planner 在下一轮规划时知晓结果。

- 异步工具在执行期间应避免依赖会被重载重置的状态；插件应实现 `onConfigUpdate` 以支持配置热更新而不被重载打断。

- **同一插件的所有 JS 执行是串行的**：异步工具在后台运行时，该插件的其他工具/命令调用会等待其结束（Goja 沙箱非线程安全）。因此耗时工具内不宜长期占用（如忙等待轮询），否则会阻塞同一插件的其他功能。

**工具执行超时机制（所有插件通用）：**

- 主程序对**每一次工具调用**都会启动一个兜底看门狗超时（时长取工具声明的 `timeoutSeconds`，未声明用默认 300 秒）。

- 一旦插件 JS 内某个同步调用（如外部 HTTP、LLM 请求）挂起、超过看门狗时限，主程序会中断该次执行并返回"工具执行超时"错误，避免永久阻塞该群后续消息的处理。

- 中断后 Goja 虚拟机状态不再安全，该插件**后续所有工具/命令调用将被拒绝**，直到插件被重载（修改配置触发 `onConfigUpdate`，或重启进程）恢复正常。因此：应始终为工具声明一个合理但不过小的 `timeoutSeconds`，避免正常耗时任务被误杀。

- 插件内发起的外部 HTTP 建议使用 `yara.http.get/post/download` 并可通过最后一个数字参数指定秒级超时；不指定时默认 120 秒。

***

## Hook 开发

Hook 允许插件在消息处理管线的特定节点插入自定义逻辑，可以修改消息内容、中断处理或观察流程。

### 支持的 Hook 点

| Hook 点                         | 触发时机          | 支持 modifiedData |
| ------------------------------ | ------------- | :-------------: |
| `chat.receive.before_process`  | 消息进入处理管线前     |        是        |
| `chat.receive.after_process`   | 消息预处理完成后      |        是        |
| `chat.replyer.before_response` | Replyer 生成回复前 |        否        |
| `chat.replyer.after_response`  | Replyer 生成回复后 |        否        |
| `chat.send.before_send`        | 消息发送前         |        否        |
| `chat.send.after_send`         | 消息发送后         |        否        |

### 阻塞模式示例（修改消息内容）

```javascript
// 权限：hook.register
// 在消息处理前，将链接替换为网页内容，让 LLM 能"看到"链接内容
yara.hook.register("chat.receive.before_process", function(event) {
  var msg = event.message;
  if (msg.content && msg.content.includes("https://")) {
    var urlMatch = msg.content.match(/https?:\/\/\S+/);
    if (urlMatch) {
      var response = yara.http.get(urlMatch[0]);
      if (response.status === 200) {
        var pageContent = response.body.substring(0, 2000);
        return {
          allowContinue: true,
          modifiedData: {
            content: "[用户分享了链接，内容如下]\n" + pageContent
          }
        };
      }
    }
  }
  return { allowContinue: true };
}, { mode: "blocking", order: "early" });
```

### 观察模式示例（不修改流程）

```javascript
// 权限：hook.register
// 静默记录所有消息，不干扰处理流程
yara.hook.register("chat.replyer.after_response", function(event) {
  var msg = event.message;
  yara.logger.info("群 " + msg.groupId + " 中 " + msg.senderName + " 发送了消息: " + msg.content);
  // 观察模式不需要返回值（或返回 { allowContinue: true }）
}, { mode: "observe" });
```

### 中断处理示例

```javascript
// 检测到敏感词时直接中断，不进入 LLM 处理
yara.hook.register("chat.receive.before_process", function(event) {
  var config = yara.config.getFile() || {};
  var blockedWords = config.blockedWords || [];
  for (var i = 0; i < blockedWords.length; i++) {
    if (event.message.content.indexOf(blockedWords[i]) !== -1) {
      var groupId = event.message.groupId || "default";
      yara.send.text(groupId, "消息包含敏感内容，已被拦截");
      return { allowContinue: false, action: "abort" };
    }
  }
  return { allowContinue: true };
}, { mode: "blocking", errorPolicy: "log" });
```

### Hook 配置参数

| 参数            | 值          | 说明                                        |
| ------------- | ---------- | ----------------------------------------- |
| `mode`        | `blocking` | 阻塞模式：返回值影响后续处理，`allowContinue: false` 可中断 |
| `mode`        | `observe`  | 观察模式：仅观察不干预，返回值被忽略                        |
| `order`       | `early`    | 优先执行（在所有同类型 Hook 之前）                      |
| `order`       | `normal`   | 正常顺序（默认）                                  |
| `order`       | `late`     | 延后执行（在所有同类型 Hook 之后）                      |
| `errorPolicy` | `abort`    | 出错时中止整个处理流程                               |
| `errorPolicy` | `skip`     | 出错时跳过此 Hook，继续处理                          |
| `errorPolicy` | `log`      | 出错时记录日志，继续处理（默认）                          |
| `timeoutMs`   | 毫秒数        | 超时时间，默认 8000ms                            |

***

## 事件处理器开发

事件处理器让插件可以响应系统级事件（如消息到达、启动、停止等），通过 `yara.eventHandler.register()` 注册。

### 支持的事件类型

| 事件类型                     | 触发时机         |
| ------------------------ | ------------ |
| `ON_START`               | 系统启动时        |
| `ON_STOP`                | 系统停止时        |
| `ON_MESSAGE_PRE_PROCESS` | 消息预处理前       |
| `ON_MESSAGE`             | 消息到达时        |
| `ON_PLAN`                | Planner 规划阶段 |
| `POST_LLM`               | LLM 调用后      |
| `AFTER_LLM`              | LLM 完整处理后    |
| `POST_SEND_PRE_PROCESS`  | 消息发送预处理      |
| `POST_SEND`              | 消息发送后        |
| `AFTER_SEND`             | 消息发送完成后      |

### 完整示例

```javascript
// 注册事件处理器（事件类型用 YaraEvents 常量或字符串）
// 权限：event_handler.register

// 注册消息事件处理器
yara.eventHandler.register("onMessage", "ON_MESSAGE", function(eventData) {
  yara.logger.info("收到消息: " + eventData.senderName + " - " + eventData.content);

  // 统计消息字数
  var wordCount = eventData.content ? eventData.content.length : 0;
  if (wordCount > 100) {
    yara.logger.info("长消息检测: " + wordCount + " 字");
  }

  return { handled: true };
});

// 注册启动事件处理器
yara.eventHandler.register("onStart", "ON_START", function(eventData) {
  yara.logger.info("系统已启动，插件开始工作");
  return { handled: true };
});
```

### eventHandler.register() 参数说明

`yara.eventHandler.register(name, eventType, handler, options)` 的字段：

| 字段                 | 类型     | 必需 | 说明                            |
| ------------------ | ------ | -- | ----------------------------- |
| `name`             | string | 是  | 事件处理器名称（唯一标识）                 |
| `description`      | string | 否  | 描述文字                          |
| `eventType`        | string | 是  | 监听的事件类型，见上表                   |
| `interceptMessage` | bool   | 否  | 是否拦截消息（设为 true 时消息不再传递给后续处理器） |
| `weight`           | int    | 否  | 权重，数字越小越先执行，默认 100            |

### 事件对象 (eventData) 常用字段

| 字段           | 类型     | 说明           |
| ------------ | ------ | ------------ |
| `content`    | string | 消息文本内容       |
| `senderName` | string | 发送者昵称        |
| `senderId`   | string | 发送者 ID       |
| `groupId`    | string | 群组 ID        |
| `platform`   | string | 平台标识（如 "qq"） |
| `messageId`  | string | 消息唯一 ID      |
| `timestamp`  | number | 消息时间戳        |

***

## 插件白名单认证

插件系统提供了一个白名单认证功能，插件开发者可以选择使用。在配置文件中添加 `auth` 节即表示启用白名单，插件注册的工具只会在白名单中的群聊/私聊里暴露给 Planner，避免无关群聊中 Planner 看到不相关的工具，减少判断异常和节省 token。

### 如何使用

在插件的配置文件（如 `config.yaml` 或 `config.json`）中添加 `auth` 节：

```yaml
# config.yaml 示例
auth:
  allowed_groups:
    - "qq:群号1"
    - "qq:群号2"
  allowed_private:
    - "qq:用户ID"
```

```json
// config.json 示例
{
  "auth": {
    "allowed_groups": ["qq:群号1", "qq:群号2"],
    "allowed_private": []
  }
}
```

| 字段                | 类型    | 说明                                                            |
| ----------------- | ----- | ------------------------------------------------------------- |
| `allowed_groups`  | array | 允许的群聊，格式: `平台:群号`。不写 `auth` 节时所有群聊可用，写了 `auth` 但留空表示不授权任何群聊   |
| `allowed_private` | array | 允许的私聊，格式: `平台:用户ID`。不写 `auth` 节时所有私聊可用，写了 `auth` 但留空表示不授权任何私聊 |

规则：**不配置** **`auth`** **节 = 全群可用；配置了** **`auth`** **节 = 严格按白名单来，没有开关。**

系统会在每次处理消息时自动读取这个配置，过滤掉未授权插件的工具。如果需要在 Web UI 的插件配置页中编辑白名单，可在 `plugin.json` 的 `config.sections` 中声明 `auth` 配置节；不声明也能正常工作，只是前端无法直接修改。

### 插件代码中配合检查

工具的过滤由系统自动完成，但命令（Command）和工具处理函数中也建议加上校验：

```javascript
// 检查当前群聊是否在白名单内
function checkGroupAuth(groupId, platform) {
  var config = yara.config.getFile();
  // 没有 auth 节 = 不限制
  if (!config.auth) return true;

  var allowedGroups = config.auth.allowed_groups || [];
  if (allowedGroups.length === 0) return false;

  var groupKey = platform + ":" + groupId;
  for (var i = 0; i < allowedGroups.length; i++) {
    if (allowedGroups[i] === groupKey) return true;
  }
  return false;
}

// 在命令处理器中使用
yara.command.register("mycmd", "/mycmd", function(match, context) {
  if (!checkGroupAuth(context.groupId, context.platform)) {
    return "本插件未授权在当前群聊使用";
  }
  // 正常处理...
});
```

### 适用场景

- 群管类插件想只在特定群使用，避免无关群聊中 Planner 看到群管工具

- 新插件先在少数群测试，稳定后再放开

- 按群授权，控制使用范围

### 注意

- 不配置 `auth` 节的插件在所有群聊中正常可用

- 配置了 `auth` 则严格按白名单：`allowed_groups` 为空表示不授权任何群聊

- 工具过滤在 Planner 获取工具列表时生效，不影响工具的注册和执行

- 建议在工具处理函数中也加上权限校验（如 `checkGroupPermission`），作为防御性编程的第二道防线

***

## 最佳实践

### 1. 最小权限原则

只声明插件实际需要的权限。

```json
{ "permissions": ["send.text", "http.request"] }
```

### 2. 错误处理

```javascript
function onLoad() {
  try {
    var resp = yara.http.get("https://api.example.com/data");
    if (resp.status !== 200) {
      yara.logger.warn("API 返回异常状态: " + resp.status);
      return;
    }
    var data = JSON.parse(resp.body);
  } catch (e) {
    yara.logger.error("插件初始化失败: " + e.message);
  }
}
```

### 3. 资源清理

```javascript
function onUnload() {
  // 事件订阅和 Hook 会被系统自动清理
  // 手动创建的 TCP/RCON 连接等资源需要自行释放
}
```

### 4. 使用 context 获取群聊信息

```javascript
yara.command.register("mycmd", "/mycmd", function(match, context) {
  // context.platform: 触发命令的平台，如 "qq", "telegram"
  // context.groupId: 触发命令的群组ID
  var groupId = context ? context.groupId : "default";
  yara.send.text(groupId, "Hello!");
  return "done";
});
```

### 5. 配置文件自动生成

**重要：插件不应预写配置文件，配置文件应由插件首次运行时自动生成，且应包含示例参数值。**

```javascript
// 在 onLoad() 或首次使用时自动生成配置文件
// 默认配置应包含示例值，方便用户理解和修改
var DEFAULT_CONFIG = [
  "# 插件配置文件",
  "# 此文件由插件首次运行时自动生成，可自行修改",
  "",
  "api_key: \"your-api-key-here\"",
  "max_results: 10",
  "enable_cache: true",
  ""
].join("\n");

function ensureConfig() {
  try {
    var config = yara.config.getFile();
    if (config && Object.keys(config).length > 0) {
      return config;
    }
  } catch (e) {
    // 文件不存在，下面生成
  }

  yara.logger.info("配置文件不存在，自动生成...");
  yara.file.write("config.yaml", DEFAULT_CONFIG);
  return yara.config.getFile();
}
```

**JSON 格式配置示例（推荐用于有复杂嵌套结构的插件）：**

```javascript
var DEFAULT_CONFIG = {
  plugin: {
    enabled: true,
    config_version: "1.0.0"
  },
  api: {
    endpoint: "https://api.example.com",
    apiKey: "your-api-key-here"
  },
  features: {
    enableFeatureA: true,
    enableFeatureB: false
  },
  permissions: {
    adminUsers: [
      "qq:你的QQ号"
    ],
    allowedGroups: [
      "qq:群号"
    ]
  },
  limits: {
    maxResults: 10,
    timeoutSeconds: 30
  }
};

function ensureConfig() {
  try {
    var existing = yara.config.getFile();
    if (existing && Object.keys(existing).length > 0) {
      return existing;
    }
  } catch (e) {
    // 文件不存在，下面生成
  }

  yara.logger.info("配置文件不存在，自动生成...");
  yara.file.write("config.json", JSON.stringify(DEFAULT_CONFIG, null, 2));
  return DEFAULT_CONFIG;
}
```

**规则：**

- 插件目录只提交 `plugin.json` 和 `index.js`，不提交 `config.yaml` / `config.toml` / `config.json` 等配置文件

- 配置文件在 `onLoad()` 或首次命令调用时通过 `yara.file.write()` 自动生成到插件根目录

- **默认配置必须包含示例参数值**（如 `"qq:你的QQ号"`、`"your-api-key-here"`），不要用空字符串或空数组，方便用户知道应该填什么格式

- **`plugin.json`** **中每个参数的** **`description`** **必须包含示例格式**，详见上方 [配置参数描述规范](#配置参数描述规范-️) 章节

- `plugins/` 目录默认在 `.gitignore` 中，插件不会被推送到仓库

- **插件通过** **`plugin.json`** **的** **`config.configFile`** **指定唯一的配置文件**（如 `config.yaml` / `config.toml` / `config.json`），格式由 `config.type` 声明。插件运行时（`yara.config`）与 Web 配置页都读写这一个文件，二者保持一致。旧插件未声明 `configFile` 时，才按 `config.yaml` > `config.yml` > `config.toml` > `config.json` 的优先级读取

### 6. 插件图标

插件可以在目录中放置图标文件，Web 面板会自动识别并显示：

| 图标文件                     | 优先级 | 说明   |
| ------------------------ | --- | ---- |
| `icon.webp`              | 1   | 推荐格式 |
| `icon.png`               | 2   | 通用格式 |
| `icon.svg`               | 3   | 矢量图标 |
| `icon.jpg` / `icon.jpeg` | 4   | 备选   |

图标会在插件列表和配置页中显示。如果插件没有提供图标，系统会自动分配一个默认图标。

### 7. 前端配置页

Web 面板为每个插件提供了独立的配置页面（`/plugins/{id}/config`），支持两种编辑模式：

| 模式        | 说明                                                                                        |
| --------- | ----------------------------------------------------------------------------------------- |
| **可视化模式** | 基于 `plugin.json` 中 `config.sections` 定义的 schema 渲染表单，支持 string/number/boolean/object 字段类型 |
| **源代码模式** | 直接编辑配置文件的原始文本，适用于无 schema 或复杂配置场景                                                         |

配置页会自动检测未保存的更改，保存后触发插件热更新。

### 8. 插件 ID 命名规范

使用反向域名格式：`com.example.my-plugin`

### 9. 版本号规范

严格遵循语义化版本：`1.0.0`（不含 v 前缀）

### 10. 热重载规则与运行时数据

系统通过文件监听实现插件热更新，**不同类型的文件变化触发不同行为**：

| 文件变化                                                               | 系统行为                                                               |
| ------------------------------------------------------------------ | ------------------------------------------------------------------ |
| `index.js` / `plugin.json` 等代码与清单文件                                | 卸载并重新加载插件（500ms 防抖）                                                |
| 配置文件（`config.yaml` / `config.yml` / `config.toml` / `config.json`） | 调用 `onConfigUpdate` 热更新配置，**不重载插件**（未实现 `onConfigUpdate` 时退化为整体重载） |
| `data/` 目录（含其下所有文件）的任何写入                                           | **完全忽略**，不触发任何操作                                                   |
| 临时/备份文件（`~` 结尾、`.` 开头、`.backup` / `.bak` 后缀）                       | 完全忽略                                                               |

**因此插件可以放心把运行时数据（计数、缓存、下载的图片等）写入** **`data/`** **目录**，这些写入不会打断正在执行的后台异步任务，也不会触发重载：

```javascript
// 写入运行数据（不会触发重载）
yara.file.writeData("daily_count.json", JSON.stringify({ date: "2026-08-16", count: 3 }));
yara.http.download(url, "result.png"); // 下载文件保存在 data/ 目录
```

> 注意：`data/` 是运行时数据目录，可能被清理；需要持久化的配置应放在插件根目录的 `config.yaml` 中。

***

## LTP3 包标准

LTP（Lunar Tool Package）是月华智能体的工具包/前端页面标准，通过 `metadata.json` 描述包元信息，按标签（tags）分类管理。

### 目录结构

```
com.yaraflow.mc-status/
├── metadata.json       # 包元信息（必需）
├── plugin.json         # YaraFlow 插件清单
├── index.js            # 插件主逻辑
├── index.html          # 插件介绍页（琉璃管理页展示）
├── script.js           # 介绍页脚本
├── styles.css          # 介绍页样式
├── config.yaml         # 配置文件（自动生成，放插件根目录）
└── data/               # 插件运行时数据目录（计数器/缓存/下载文件等；热重载监听会忽略此目录的写入）
```

### metadata.json 规范

```json
{
  "id": "package_name",
  "title": "包标题",
  "description": "包描述",
  "tags": ["LTP3"],
  "url": "/file/read/package/package_name/index.html",
  "icon": "/file/read/package/package_name/icon.webp",
  "tools": []
}
```

| 字段            | 类型        | 必需    | 说明                                        |
| ------------- | --------- | ----- | ----------------------------------------- |
| `id`          | string    | 是     | 包唯一标识，与目录名一致                              |
| `title`       | string    | 是     | 包显示名称                                     |
| `description` | string    | 是     | 包功能描述                                     |
| `tags`        | string\[] | 是     | 归属标签：`LTP2`（月华插件）、`LTP3`（语瞳插件）、`LTPX`（通用） |
| `url`         | string    | 否     | 插件介绍/文档页面路径，在琉璃管理页中点击可查看插件详情              |
| `icon`        | string    | 否     | 图标路径，默认使用系统默认图标                           |
| `tools`       | array     | 工具型必需 | OpenAI function tool 定义数组，格式见下方           |

### 工具定义（tools 字段）

```json
{
  "tools": [
    {
      "type": "function",
      "function": {
        "name": "tool_name",
        "description": "工具描述",
        "parameters": {
          "type": "object",
          "properties": {
            "param_name": {
              "type": "string",
              "enum": ["option1", "option2"],
              "description": "参数描述"
            }
          },
          "required": ["param_name"]
        }
      }
    }
  ]
}
```

### 前端页面规范

前端型 LTP 包（标签含 `LTP3`）的 HTML/CSS/JS 必须严格遵循 [frontend-development-guide.md](./frontend-development-guide.md) 的规范，包括：

- 玻璃拟态 (Glassmorphism) 设计

- 三层背景结构（`bg-layer` → 玻璃效果 → 内容面板）

- CSS 变量驱动双主题（`:root` + `.dark-mode`）

- Font Awesome 6.4.0 图标

- 标题格式：`『 星月智能 』模块名`

- 三断点响应式（1024px / 768px / 480px）

- 禁用外部依赖，仅使用标准依赖库

### 标签体系

| 标签     | 归属 | 说明                                      |
| ------ | -- | --------------------------------------- |
| `LTP2` | 月华 | 月华智能体的工具包/插件                            |
| `LTP3` | 语瞳 | 语瞳（YaraFlow）的插件，可以是 YaraFlow JS 插件或前端页面 |
| `LTPX` | 通用 | 月华和语瞳共用的通用组件/工具                         |

