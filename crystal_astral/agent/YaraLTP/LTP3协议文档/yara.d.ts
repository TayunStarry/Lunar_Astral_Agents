/**
 * YaraFlow 插件沙箱全局类型定义（yara.d.ts）
 *
 * 用途：让开发者在 IDE（VS Code 等）里编写插件 JS 时获得自动补全与类型提示（一键填充），
 * 事件名 / Hook 点不再需要记忆裸字符串。
 *
 * 用法：在插件 JS 文件顶部添加引用指令（插件目录结构 plugins/<插件ID>/index.js 时）：
 *   /// <reference path="../yara.d.ts" />
 * 或者把本文件复制到插件目录后用 ./yara.d.ts。
 *
 * 说明：
 * - `YaraEvents` / `YaraHooks` 是运行时会注入沙箱的全局常量对象，插件里可直接使用，
 *   例如 `yara.event.subscribe(YaraEvents.ON_MESSAGE, ...)`。
 * - `YaraEventName` / `YaraHookPoint` 是字符串联合类型，直接写字符串字面量也有补全提示。
 * - 生命周期函数 `onLoad` / `onUnload` / `onConfigUpdate` 由插件自行定义，这里只声明签名。
 */

// ─────────────────────────────────────────────────────────────
// 事件名常量（运行时会注入全局 YaraEvents）
// ─────────────────────────────────────────────────────────────
declare const YaraEvents: {
  readonly ON_START: 'ON_START'
  readonly ON_STOP: 'ON_STOP'
  readonly ON_MESSAGE_PRE_PROCESS: 'ON_MESSAGE_PRE_PROCESS'
  readonly ON_MESSAGE: 'ON_MESSAGE'
  readonly ON_PLAN: 'ON_PLAN'
  readonly POST_LLM: 'POST_LLM'
  readonly AFTER_LLM: 'AFTER_LLM'
  readonly POST_SEND_PRE_PROCESS: 'POST_SEND_PRE_PROCESS'
  readonly POST_SEND: 'POST_SEND'
  readonly AFTER_SEND: 'AFTER_SEND'
}

type YaraEventName = (typeof YaraEvents)[keyof typeof YaraEvents]

// ─────────────────────────────────────────────────────────────
// Hook 点常量（运行时会注入全局 YaraHooks）
// ─────────────────────────────────────────────────────────────
declare const YaraHooks: {
  // 聊天消息链
  readonly CHAT_RECEIVE_BEFORE_PROCESS: 'chat.receive.before_process'
  readonly CHAT_RECEIVE_AFTER_PROCESS: 'chat.receive.after_process'
  // 命令执行链
  readonly CHAT_COMMAND_BEFORE_EXECUTE: 'chat.command.before_execute'
  readonly CHAT_COMMAND_AFTER_EXECUTE: 'chat.command.after_execute'
  // 表情包链
  readonly EMOJI_CHAT_BEFORE_SELECT: 'emoji.chat.before_select'
  readonly EMOJI_CHAT_AFTER_SELECT: 'emoji.chat.after_select'
  readonly EMOJI_REGISTER_AFTER_BUILD_DESCRIPTION: 'emoji.register.after_build_description'
  readonly EMOJI_REGISTER_AFTER_BUILD_EMOTION: 'emoji.register.after_build_emotion'
  // 发送服务链
  readonly SEND_SERVICE_AFTER_BUILD_MESSAGE: 'send_service.after_build_message'
  readonly SEND_SERVICE_BEFORE_SEND: 'send_service.before_send'
  readonly SEND_SERVICE_AFTER_SEND: 'send_service.after_send'
  // 规划器链
  readonly CHAT_PLANNER_BEFORE_REQUEST: 'chat.planner.before_request'
  readonly CHAT_PLANNER_AFTER_RESPONSE: 'chat.planner.after_response'
  // 回复器链
  readonly CHAT_REPLYER_BEFORE_REQUEST: 'chat.replyer.before_request'
  readonly CHAT_REPLYER_BEFORE_MODEL_REQUEST: 'chat.replyer.before_model_request'
  readonly CHAT_REPLYER_AFTER_RESPONSE: 'chat.replyer.after_response'
  // 黑话链
  readonly JARGON_QUERY_BEFORE_SEARCH: 'jargon.query.before_search'
  readonly JARGON_QUERY_AFTER_SEARCH: 'jargon.query.after_search'
  readonly JARGON_EXTRACT_BEFORE_PERSIST: 'jargon.extract.before_persist'
  readonly JARGON_INFERENCE_BEFORE_FINALIZE: 'jargon.inference.before_finalize'
  // 表达方式链
  readonly EXPRESSION_SELECT_BEFORE_SELECT: 'expression.select.before_select'
  readonly EXPRESSION_SELECT_AFTER_SELECTION: 'expression.select.after_selection'
  readonly EXPRESSION_LEARN_AFTER_EXTRACT: 'expression.learn.after_extract'
  readonly EXPRESSION_LEARN_BEFORE_UPSERT: 'expression.learn.before_upsert'
}

type YaraHookPoint = (typeof YaraHooks)[keyof typeof YaraHooks]

// Hook 注册选项
interface YaraHookOptions {
  mode?: 'blocking' | 'observe'
  order?: 'early' | 'normal' | 'late'
  errorPolicy?: 'abort' | 'skip' | 'log'
  timeoutMs?: number
}

// ─────────────────────────────────────────────────────────────
// 通用类型
// ─────────────────────────────────────────────────────────────

/** 二进制数据：字符串（UTF-8 文本）/ 整数数组（兼容旧插件）/ Uint8Array / ArrayBuffer */
type Bytes = string | number[] | Uint8Array | ArrayBuffer

interface ApiError {
  error: string
}

/** 聊天消息对象（事件/Hook 回调参数） */
interface YaraMessage {
  id: string
  senderId: string
  senderName: string
  groupId: string
  content: string
  isAtMe?: boolean
  hasImage?: boolean
  image_urls?: string[]
  timestamp: number
  platform: string
}

/** 命令/工具处理器上下文 */
interface YaraContext {
  platform: string
  groupId: string
  messageId?: string
  userId?: string
  senderName?: string
}

// ─────────────────────────────────────────────────────────────
// API 接口
// ─────────────────────────────────────────────────────────────

interface YaraLogger {
  info(message: string, ...fields: unknown[]): void
  warn(message: string, ...fields: unknown[]): void
  error(message: string, ...fields: unknown[]): void
  debug(message: string, ...fields: unknown[]): void
}

interface YaraSend {
  /** 发送文本消息，成功返回 true */
  text(groupId: string, content: string): boolean
  /** 发送图片（URL / base64） */
  image(groupId: string, imageData: string): boolean
  /** 发送表情包 */
  emoji(groupId: string, emojiData: unknown): boolean
  /** 发送图文混合消息：segments 形如 [{ type: "text", content }, { type: "image", content }] */
  hybrid(groupId: string, segments: Array<{ type: string; content: string }>): boolean
}

interface YaraEvent {
  /** 订阅事件（如 YaraEvents.ON_MESSAGE） */
  subscribe(topic: YaraEventName | string, callback: (eventData: YaraMessage & Record<string, unknown>) => void): void
  /** 发布事件 */
  publish(topic: YaraEventName | string, payload?: unknown): void
}

interface YaraHook {
  /** 注册消息处理链 Hook，hookType 见 YaraHooks 常量 */
  register(
    hookType: YaraHookPoint | string,
    handler: (event: {
      type: string
      message?: YaraMessage
      context?: Record<string, unknown>
    }) => void | {
      allowContinue?: boolean
      action?: 'abort'
      logSuffix?: string
      modifiedData?: { content?: string; senderName?: string }
    },
    options?: YaraHookOptions
  ): void
}

interface YaraCommand {
  /**
   * 注册指令
   * @param name 指令名称（唯一标识）
   * @param pattern 正则模式（可带 /.../ 分隔符）
   * @param handler (match: RegExpMatchArray, context: YaraContext) => void
   * @param options { aliases: string[] }
   */
  register(
    name: string,
    pattern: string,
    handler: (match: RegExpMatchArray & Record<string, string>, context?: YaraContext) => unknown,
    options?: { aliases?: string[] }
  ): void
}

interface YaraHttpResponse {
  status: number
  statusText: string
  body: string
  headers: Record<string, string>
}

interface YaraHttp {
  get(url: string, headers?: Record<string, string>): YaraHttpResponse | ApiError
  post(url: string, body?: unknown, headers?: Record<string, string>): YaraHttpResponse | ApiError
  /** 下载文件到插件 data/ 目录，savePath 相对 data/（可选，默认取 URL 文件名） */
  download(url: string, savePath?: string): { path: string } | ApiError
}

interface YaraNetworkSocket {
  send(data: Bytes): { success: true } | ApiError
  sendString(data: string): { success: true } | ApiError
  /** 返回整数数组（兼容旧插件），可用 yara.encoding.base64Encode 转 base64 */
  receive(timeoutSec?: number): number[] | ApiError
  receiveString(timeoutSec?: number): string | ApiError
  close(): { success: true } | ApiError
}

interface YaraNetworkUdpSocket extends YaraNetworkSocket {
  localAddr?: string
  sendTo(data: Bytes, host: string, port: number): { success: true } | ApiError
  sendToString(data: string, host: string, port: number): { success: true } | ApiError
  receiveFrom(timeoutSec?: number): { data: number[]; host: string; port: number } | ApiError
  receiveFromString(timeoutSec?: number): { data: string; host: string; port: number } | ApiError
}

interface YaraNetwork {
  resolveDNS(hostname: string, timeoutSec?: number): string[] | ApiError
  resolveSRV(service: string, proto: string, hostname: string, timeoutSec?: number): { target: string; port: number } | ApiError
  tcpConnect(host: string, port: number, timeoutSec?: number): YaraNetworkSocket | ApiError
  udpConnect(host: string, port: number, timeoutSec?: number): YaraNetworkUdpSocket | ApiError
  udpListen(host?: string, port?: number): YaraNetworkUdpSocket | ApiError
}

interface YaraPlatform {
  sendCommand(command: string, params: Record<string, unknown>): { success: boolean; error?: string }
  getName(): string
  getGroupId(): string
  lookupUser(groupId: string, name: string): string | null
}

interface YaraEncoding {
  base64Encode(data: Bytes): string
  base64Decode(str: string): number[]
  hexEncode(data: Bytes): string
  hexDecode(str: string): number[]
  urlEncode(str: string): string
  urlDecode(str: string): string
  utf8Encode(str: string): number[]
  utf8Decode(bytes: Bytes): string
}

interface YaraTime {
  now(): number
  nowMs(): number
  format(timestamp: number, layout: string): string
  formatDuration(seconds: number): string
  parse(str: string, layout: string): number | null
  /** 同步睡眠指定毫秒数（可被插件超时安全打断），用于延时操作 */
  sleep(ms: number): void
}

interface YaraCrypto {
  md5(data: Bytes): string
  sha1(data: Bytes): string
  sha256(data: Bytes): string
  hmacSha1(key: Bytes, data: Bytes): string
  hmacSha256(key: Bytes, data: Bytes): string
  ed25519Sign(privateKey: string, data: Bytes): string
  generateJWT(claims: Record<string, unknown>, privateKey: string, keyID?: string): string
}

interface YaraModel {
  chat(params: Record<string, unknown>): unknown
  chatWithConfig(params: Record<string, unknown>): unknown
  chatWithTask(params: Record<string, unknown>): unknown
  chatWithTools(params: Record<string, unknown>): unknown
  embed(params: Record<string, unknown>): unknown
  getConfig(taskType: string): unknown
  getAllConfigs(): unknown
  getAvailableConfigs(): unknown
  listTasks(): unknown
  getAvailableModels(): unknown
}

interface YaraConfig {
  /** 读取插件配置文件（YAML/JSON/TOML，由 plugin.json 的 config.configFile 指定） */
  getFile(): Record<string, unknown> | undefined
  /** 将配置对象序列化后写回配置文件 */
  setFile(config: Record<string, unknown>): void
}

interface YaraDatabase {
  queryMessages(opts: { platform?: string; groupID?: string; limit?: number }): unknown
  searchMessages(opts: { platform?: string; groupID?: string; query?: string; limit?: number; offset?: number }): unknown
  getUserMessages(opts: { platform?: string; userID?: string; limit?: number }): unknown
  getUserInfo(opts: { platform?: string; userID?: string }): unknown
}

interface YaraFile {
  read(path: string): string | undefined
  write(path: string, content: string): void
  readData(path: string): string | undefined
  writeData(path: string, content: string): void
  listData(dir?: string): string[]
  getDataPath(): string
}

interface YaraToolParam {
  name: string
  type: string
  description: string
  required?: boolean
  default?: unknown
  enumValues?: string[]
}

interface YaraToolDefinition {
  name?: string
  description: string
  briefDescription?: string
  detailedDescription?: string
  parameters?: YaraToolParam[]
  coreTool?: boolean
  visibility?: 'visible' | 'hidden' | 'deferred'
  toolType?: 'agent' | 'autonomous' | 'core'
  timeoutSeconds?: number
  async?: boolean
}

interface YaraTool {
  /**
   * 注册 Agent 工具（LLM 可调用）
   * @param name 工具名称
   * @param definition 工具定义（description / parameters 等）
   * @param handler (params: any, context: YaraContext) => any
   */
  register(name: string, definition: YaraToolDefinition, handler: (params: Record<string, unknown>, context?: YaraContext) => unknown): void
  /** 注册自主运行工具（hidden + autonomous，通过 Hook 触发） */
  registerAutonomous(definition: {
    name: string
    description?: string
    hookType?: YaraHookPoint | string
    pattern?: string
    timeoutSeconds?: number
    handler: (message?: YaraMessage) => unknown
  }): void
  /** 获取当前插件已注册的工具定义 */
  getDefinitions(): YaraToolDefinition[]
}

interface YaraImage {
  /** 获取主程序已缓存/可下载的图片（按 URL），返回 base64，失败返回 null */
  getCached(url: string): string | null
  /** 读取并校验插件目录内图片文件，有效返回 base64，否则 null */
  loadValid(path: string): string | null
  /** 校验 base64 是否为有效图片头 */
  isImage(base64: string): boolean
}

interface YaraAsync {
  run(
    taskFn: (task: Record<string, unknown>) => unknown,
    options?: { timeout?: number; onProgress?: (data: unknown) => void; onComplete?: (result: unknown) => void; onError?: (err: string) => void }
  ): { taskId: string; status: string; timeout: number }
  reportProgress(taskId: string, data: unknown): void
}

interface YaraApi {
  register(name: string, handler: (params: Record<string, unknown>) => unknown, options?: { description?: string; version?: string; public?: boolean }): void
  call(qualifiedName: string, params?: Record<string, unknown>): unknown
}

interface YaraEventHandler {
  register(
    name: string,
    eventType: YaraEventName | string,
    handler: (eventData: YaraMessage & Record<string, unknown>) => void,
    options?: { interceptMessage?: boolean; weight?: number; description?: string }
  ): void
}

interface YaraLLMProvider {
  register(clientType: string, handler: (params: Record<string, unknown>) => unknown, options?: { name?: string; description?: string; version?: string }): void
}

interface YaraEmojiInfo {
  id?: string
  hash?: string
  emotion?: string
  description?: string
  url?: string
  [key: string]: unknown
}

interface YaraEmoji {
  getRandom(): YaraEmojiInfo | null
  getByEmotion(emotion: string): YaraEmojiInfo | null
  getAll(): YaraEmojiInfo[]
  getCount(): number
  getEmotions(): string[]
  getInfo(...args: unknown[]): YaraEmojiInfo | null
}

interface YaraKnowledgeEntry {
  id: number
  content: string
  tags: string[]
  source: string
  createdAt: number
  updatedAt: number
}

interface YaraKnowledge {
  search(opts: { query: string; limit?: number }): { entries: YaraKnowledgeEntry[]; total: number }
}

// ─────────────────────────────────────────────────────────────
// 全局 yara 对象
// ─────────────────────────────────────────────────────────────
interface YaraAPI {
  logger: YaraLogger
  send: YaraSend
  event: YaraEvent
  hook: YaraHook
  command: YaraCommand
  http: YaraHttp
  network: YaraNetwork
  platform: YaraPlatform
  encoding: YaraEncoding
  time: YaraTime
  crypto: YaraCrypto
  model: YaraModel
  config: YaraConfig
  database: YaraDatabase
  file: YaraFile
  tool: YaraTool
  image: YaraImage
  async: YaraAsync
  api: YaraApi
  eventHandler: YaraEventHandler
  llmProvider: YaraLLMProvider
  emoji: YaraEmoji
  knowledge: YaraKnowledge
}

declare const yara: YaraAPI

// ─────────────────────────────────────────────────────────────
// 生命周期函数（由插件自行定义，这里仅声明签名）
// ─────────────────────────────────────────────────────────────
declare function onLoad(): void
declare function onUnload(): void
declare function onConfigUpdate(scope: string, config: Record<string, unknown>, version: string): void
