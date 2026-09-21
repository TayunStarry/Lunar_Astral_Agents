/**
 * crystal_astral LTP9（engine.*）插件沙箱全局类型定义（engine.d.ts）
 *
 * 用途：让开发者在 IDE（VS Code 等）里编写 LTP9 插件 JS（execute.js）时获得自动补全与类型提示。
 * 用法：把本文件复制到插件目录后以 ./engine.d.ts 引用：
 *   /// <reference path="engine.d.ts" />
 *
 * 说明：
 * - 插件只能使用 `engine.*` 命名空间、全局 `console`，以及顶层生命周期函数 onLoad/onUnload/onConfigUpdate。
 * - 同一插件的所有 JS 执行为串行（引擎按插件独立事件循环），耗时工作请用 `engine.async.run` / `engine.sleep`。
 * - 所有 API 为同步阻塞，返回普通对象（非 Promise）；通用返回结构 `rwResult = { success, text?, error?, tool_calls? }`。
 * - 能力按 `permissions.key` 中的 allow-* 门控：未授权的命名空间在 JS 中呈 `undefined`，调用即 `TypeError`。
 * - 恒常基础能力（不参与开关，始终注入）：config / event / time / sleep / encoding / command / async / export、全局 console。
 * - 模型垄断：engine.llm 忽略插件传入的 baseUrl/apiKey/model/taskType/model_list 等参数，
 *   一律强制使用琉璃 lunar_config.json 中 agent 字段定义的模型（对话→AgentMultimodal，嵌入→AgentEmbedding）。
 */

/** 通用返回结构（多数 engine.* 的统一返回）。 */
interface RwResult {
  success: boolean;
  text?: string;
  error?: string;
  /** engine.llm.chat 响应中的工具调用（OpenAI 兼容 tool_calls 原样透传）。 */
  tool_calls?: unknown[];
}

/** 网络响应（engine.http）。 */
interface HttpResult {
  status: number;
  body: string;
  error?: string;
}

/** 文件读取返回。 */
interface FileContent extends RwResult { text?: string }

/** 嵌入返回。 */
interface EmbedResult extends RwResult {
  embedding?: number[];
  embeddings?: number[][];
}

/** 事件载荷（engine.event 订阅回调入参）。 */
interface LTP9Event {
  type: string;
  payload: unknown;
}

/**
 * 工具 handler 的调用上下文（宿主构造）。
 * image_urls / image_descriptions：`[图片N]` 对应 `image_urls[N-1]`，`[图片N：描述]` 对应 `image_descriptions[N-1]`。
 */
interface ToolContext {
  sessionId?: string;
  platform?: string;
  groupId?: string;
  userId?: string;
  senderName?: string;
  messageId?: string;
  replyMessageId?: string;
  messageIDMapping?: Record<string, string>;
  image_urls?: string[];
  image_descriptions?: string[];
  isGroup?: boolean;
}

/** 命令 handler 的 match 对象：match[0]=完整匹配，match[1..]=分组；命名分组可经 match.groups.<name> 访问。 */
interface CommandMatch extends Array<string> {
  groups?: Record<string, string>;
}

// =====================================================================
// engine.* 命名空间
// =====================================================================

declare const engine: {
  /** 配置（常驻能力）。 */
  config: {
    /** 返回 config.yaml 解析后的对象；无配置或解析失败时为空对象。 */
    getFile(): Record<string, unknown>;
    /** 把对象序列化为 YAML 回写 config.yaml 并触发 onConfigUpdate("plugin", config, "")；只接收对象。 */
    setFile(v: Record<string, unknown>): void;
    /** 声明配置节/字段的中文名、类型、说明（供宿主 WebUI 渲染配置表单）。 */
    registerSchema(sections: ConfigSectionDef[]): void;
  };

  /** 事件总线（常驻能力）。 */
  event: {
    /** 订阅主题，返回订阅 id（供 unsubscribe 退订）。priority 0 最高，越小越先执行；可省略。 */
    subscribe(topic: string, cb: (event: LTP9Event) => unknown, priority?: number): number;
    /** 按订阅 id 退订。 */
    unsubscribe(topic: string, id: number): void;
    /** 发布事件：异步分发给其它插件订阅器 + 转发宿主 outbound；跳过发起插件自身订阅器。 */
    publish(topic: string, payload: unknown): void;
  };

  /** 时间与休眠（常驻能力）。 */
  time: {
    /** 当前 Unix 时间戳（秒）。 */
    now(): number;
    /** 当前 Unix 时间戳（毫秒）。 */
    nowMs(): number;
  };
  /** 同步阻塞指定毫秒（常驻能力）。 */
  sleep(ms: number): void;

  /** 日志：沙箱全局 console（见下方声明），输出走琉璃 LoggerGeneral。 */
  logger: never; // 仅供类型占位；实际用全局 console

  // ---- allow-file ----
  file?: {
    /** 读取文件；返回 {success, text} 对象，内容在 .text。路径相对插件 data/ 目录。 */
    read(path: string): FileContent;
    /** 写文件；返回 {success} 或 {success:false, error}。 */
    write(path: string, data: string): RwResult;
    /** 删除文件；返回 {success} 或 {success:false, error}。 */
    delete(path: string): RwResult;
  };

  /** 图片（allow-file；下载另需 allow-network）：校验与 base64 转换在 Go 侧完成（按真实字节校验 PNG/JPEG/GIF/WebP/BMP）。 */
  image?: {
    /** 读取 data/ 下图片并转 base64。 */
    loadValid(path: string): RwResult;
    /** 下载网络图片到 data/ 并返回 base64。 */
    download(url: string, fileName: string): RwResult;
  };

  // ---- allow-network ----
  http?: {
    get(url: string, headers?: Record<string, string>): HttpResult;
    post(url: string, body: string, headers?: Record<string, string>): HttpResult;
    /** 下载到 data/；返回 {success, path, size}。savePath 留空按 URL 末段命名。 */
    download(url: string, savePath?: string): { success: boolean; path?: string; size?: number; error?: string };
  };

  network?: {
    resolveDNS(host: string, timeoutSec?: number): { success: boolean; addresses?: string[]; error?: string };
    resolveSRV(service: string, proto: string, host: string, timeoutSec?: number): { success: boolean; targets?: { target: string; port: number }[]; error?: string };
    tcpConnect(host: string, port: number, timeoutSec?: number): NetSocket;
    udpConnect(host: string, port: number, timeoutSec?: number): NetSocket;
    udpListen(host: string, port: number): NetSocket;
  };

  /** 编解码（常驻能力）。 */
  encoding: {
    base64Encode(data: string): string;
    base64Decode(s: string): string;
    hexEncode(data: string): string;
    hexDecode(s: string): string;
    urlEncode(s: string): string;
    urlDecode(s: string): string;
    utf8Encode(s: string): number[];
    utf8Decode(v: number[] | Uint8Array | ArrayBuffer | string): string;
  };

  // ---- allow-agent ----
  /** LLM（模型垄断：忽略 params 与设置，强制 agent 字段模型）。 */
  llm?: {
    chat(messages: { role: string; content: string }[], opts?: LlmChatOpts): RwResult;
    embed(input: string | string[], opts?: Record<string, unknown>): EmbedResult;
    /** 等价 chat，opts.messages 必填（可带 tools）。 */
    chatWithTools(opts: LlmChatOpts): RwResult;
    /** 等价 chat；opts.messages 必填。 */
    chatWithConfig(opts: Record<string, unknown>): RwResult;
    /** 返回指定任务的模型池配置概要（模型垄断下恒为 agent 单模型）。 */
    getConfig(taskType: string): Record<string, unknown>;
    /** 全部任务的模型池配置概要（模型垄断下每任务相同）。 */
    getAllConfigs(): Record<string, unknown>;
    /** 任务列表：["replyer","planner","tool_use","vlm","voice","embedding"]。 */
    listTasks(): string[];
    /** 可用模型列表（模型垄断下为 [agent 对话模型名]）。 */
    getAvailableModels(): string[];
    /** 最近 n 条调用记录（agent 模型）。 */
    getCallRecords(n: number): unknown[];
    /** 熔断器状态（agent 单模型）。 */
    getCircuitBreakers(): unknown;
  };

  tool?: {
    register(name: string, definition: { description: string; parameters?: ToolParam[] }, handler: (params: Record<string, unknown>, context: ToolContext) => unknown): RwResult;
    getDefinitions(): { name: string; description: string; parameters?: ToolParam[] }[];
  };

  /** 调用前端智能体（Mini-LTP / Node-LTP 包）。 */
  agent?(id: string): { run(text: string): unknown };

  // ---- allow-send ----
  send?: {
    text(target: string, text: string): RwResult;
    image(target: string, b64: string): RwResult;
    /** 图文混合分段：图片段 {type:"image", content:b64}（兼容 image 字段），文本段 {type:"text", content}。 */
    hybrid(target: string, segs: { type: string; content: string }[]): RwResult;
  };

  platform?: {
    getName(): unknown;
    getGroupId(): unknown;
    lookupUser(groupId: string, name: string): unknown;
    sendCommand(command: string, args?: Record<string, unknown>): RwResult;
  };

  // ---- allow-signal ----
  signal?: {
    all(payload: unknown): void;
    target(id: string, payload: unknown): void;
  };
  frontEvent?: {
    signal: {
      subscribe(cb: (payload: unknown) => void): number;
      unsubscribe(id: number): void;
    };
  };

  // ---- allow-call / 常驻 export ----
  export(name: string, fn: (...args: unknown[]) => unknown): void;
  call?(id: string): { run(fnName: string, args: unknown[]): unknown };
  /** 跨插件调用导出函数（allow-call）。 */

  // ---- allow-certificate ----
  crypto?: {
    signJWT(claims: Record<string, unknown>, secret: string, algorithm: string, kid?: string): string;
    generateJWT(claims: Record<string, unknown>, privKey: string, kid?: string): string;
    md5(data: string): string;
    sha1(data: string): string;
    sha256(data: string): string;
    hmacSha1(key: string, data: string): string;
    hmacSha256(key: string, data: string): string;
    ed25519Sign(privKey: string, data: string): string;
  };
  /** 加解密（复用琉璃 LunarDecoder）。 */
  encoder?(key: string, content: string): string;
  decoder?(key: string, cipher: string): string;

  // ---- allow-database（未接入：返回"未接入数据库(host 未注入)"占位错误） ----
  database?: {
    query(sql: string, params?: unknown[]): RwResult;
    exec(sql: string, params?: unknown[]): RwResult;
  };

  // ---- allow-memory（桥接琉璃记忆库 FileManager） ----
  memory?: {
    /** 写入记忆库（ltp9_memory 集合；记忆库未初始化时返回"未接入"）。 */
    store(v: Record<string, unknown>): RwResult;
    /** 语义检索记忆库。 */
    search(v: Record<string, unknown>): RwResult;
  };

  /** 表情包（allow-memory）：复用记忆库 stickers（image 型）集合；search/random 已接入，store 未接入。 */
  emoji?: {
    search(query: string, opts?: { limit?: number }): RwResult;
    store(image: string): RwResult;
    random(query: string): RwResult;
  };

  // ---- allow-socket ----
  ws?: {
    expose(path: string, handler: (msg: string) => string): RwResult;
    publish(data: string): RwResult;
  };

  // ---- 常驻 command / async ----
  command: {
    register(name: string, pattern: string, handler: (match: CommandMatch, context: Record<string, unknown>) => unknown, options?: { aliases?: string[] }): RwResult;
  };
  async: {
    /** 启动后台任务；返回 {success, text: taskId}。taskFn 收到 { id, data }。 */
    run(taskFn: (task: { id: number; data?: unknown }) => unknown, opts?: { timeout?: number; data?: unknown }): RwResult;
    reportProgress(id: number, progress: unknown): RwResult;
    getStatus(id: number): RwResult;
    list(): { taskId: number; status: string; progress: unknown }[];
  };
};

// =====================================================================
// engine.* 相关子类型
// =====================================================================

interface ConfigFieldDef {
  name: string;
  /** 中文名（插件自声明）。 */
  label: string;
  /** boolean / integer / number / string / array / object。 */
  type: string;
  description: string;
  default?: unknown;
}

interface ConfigSectionDef {
  name: string;
  /** 中文节名。 */
  label: string;
  description: string;
  /** 展示顺序（小者在前）。 */
  order: number;
  fields: ConfigFieldDef[];
}

interface ToolParam {
  name: string;
  type: string;
  description: string;
  required?: boolean;
}

/** engine.llm.chat / chatWithTools 的 opts。 */
interface LlmChatOpts {
  /** 任务类型（模型垄断下仅作名义，不参与选模型）。 */
  taskType?: string;
  /** 采样参数（temperature/max_tokens 生效；baseUrl/apiKey/model 被忽略——模型垄断）。 */
  temperature?: number;
  max_tokens?: number;
  /** 系统提示词（前置为一条 system 消息）。 */
  system?: string;
  /** 工具/函数定义数组（透传）。 */
  tools?: unknown[];
  messages?: { role: string; content: string }[];
  [k: string]: unknown;
}

/** 裸 TCP/UDP 套接字（engine.network）。 */
interface NetSocket {
  send(data: string | number[] | Uint8Array | ArrayBuffer): RwResult;
  receive(timeoutSec?: number): { success: boolean; data?: number[]; host?: string; port?: number; error?: string };
  receiveString(timeoutSec?: number): { success: boolean; data?: string; error?: string };
  sendTo(host: string, port: number, data: string | number[] | Uint8Array | ArrayBuffer): RwResult;
  close(): RwResult;
}

// =====================================================================
// 全局 console（输出走琉璃 LoggerGeneral）
// =====================================================================
//@ts-ignore
declare const console: {
  log(...args: unknown[]): void;
  info(...args: unknown[]): void;
  warn(...args: unknown[]): void;
  error(...args: unknown[]): void;
  debug(...args: unknown[]): void;
};

// =====================================================================
// 生命周期钩子（在 execute.js 顶层定义即生效）
// =====================================================================

/** 插件加载完成后调用（可做初始化 / 条件注册工具）。 */
declare function onLoad(): void;
/** 插件卸载时调用（清理资源）。 */
declare function onUnload(): void;
/** config.yaml 经 engine.config.setFile 回写后触发（fire-and-forget）。 */
declare function onConfigUpdate(scope: string, config: Record<string, unknown>, version: string): void;