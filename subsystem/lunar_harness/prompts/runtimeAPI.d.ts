// ═══════════════════════════════════════════════════════════
// 月华私有 JS 运行时 · API 契约（索引与全局基础设施）
// ═══════════════════════════════════════════════════════════
//
// 文件首行含 `//Engine: Lunar Harness` 的脚本在月华私有 JS 运行时执行：
// 同步执行，须在运行时长内返回。时长由 run_project / run_script 的 timeout_sec
// （5-3600 秒）或首行 `timeout=<数值><单位>` 指定，默认 120 秒。
// 首行示例：`//Engine: Lunar Harness | timeout=300s`
//
// 契约按能力拆分，按需读取对应文件即可（无需一次加载全部类型）：
//   commonAPI.d.ts    公共类型（消息 / 工具调用 / 事件回调结果）
//   eventAPI.d.ts     事件订阅（event.*）与月华事件契约
//   fileAPI.d.ts      文件读写（file.*）
//   imageAPI.d.ts     图像处理（image.*）
//   videoAPI.d.ts     视频抽帧（video.*）
//   memoryAPI.d.ts    记忆库（memory.*）
//   databaseAPI.d.ts  SQLite 单一共享库（database.*）
//   encodingAPI.d.ts  编解码与哈希签名（encoding.* / hash.*）
//   agentAPI.d.ts     模型调用（agent.*）
//   networkAPI.d.ts   同步 HTTP 与裸套接字（http.* / network.*）
//
// 本文件收录全局基础设施：console / 定时器 / fetch / WebSocket / time。

/** 同步 fetch 结果（统一 Result 信封；body 为 JSON 时自动解析为对象，否则为文本） */
interface FetchResult {
	/** 请求是否成功发出（传输层；HTTP 4xx/5xx 仍为 true，看 status/ok 判断） */
	success: boolean
	/** HTTP 状态码（传输失败时为 0） */
	status: number
	/** 2xx 便捷标记 */
	ok?: boolean
	/** 最终 URL（跟随重定向后） */
	url?: string
	/** 响应头（键名小写，多值逗号合并） */
	headers?: Record<string, string>
	/** 响应体（JSON 自动解析为对象，否则原始文本） */
	body?: unknown
	/** 错误信息（传输层） */
	error?: string
}

/** fetch 请求配置 */
interface FetchInit {
	/** 请求方法 */
	method?: string
	/** 请求头 */
	headers?: Record<string, string>
	/** 请求体 */
	body?: string
}

/** 时间戳 */
declare const time: {
	/** 秒级时间戳 */
	now(): number
	/** 毫秒级时间戳 */
	nowMs(): number
}

/**
 * 导出脚本函数，供宿主调用
 *
 * 单文件脚本以 `exportFunction('main', main)` 注册入口；宿主运行脚本后会调用导出的 main。
 * 函数须同步返回普通值，异步等待请用 setTimeout 等定时器完成。
 *
 * @param name 导出的函数名
 * @param fn 函数实现本体
 */
declare const exportFunction: <F extends (...args: any[]) => any>(name: string, fn: F) => void

/**
 * 发起同步网络请求（阻塞直到返回）
 *
 * @param input 请求 URL
 * @param init 请求配置
 * @returns 统一 Result 信封；body 为 JSON 时自动解析
 */
declare function fetch(input: string, init?: FetchInit): FetchResult

/**
 * WebSocket 客户端（全局 `WebSocket`，沙箱主动连服务器）
 *
 * @param url 目标地址（ws:// 或 wss://）
 */
//@ts-ignore
declare class WebSocket {
	constructor(url: string)
	/** 连接状态：0=CONNECTING 1=OPEN 2=CLOSING 3=CLOSED */
	readyState: number
	/** 连接成功打开后的回调 */
	onopen: (() => void) | null
	/** 收到消息时的回调；文本帧 data 为 string，二进制帧 data 为 ArrayBuffer */
	onmessage: ((ev: { data: string | ArrayBuffer }) => void) | null
	/** 发生错误时的回调 */
	onerror: ((ev: { error: string }) => void) | null
	/** 连接关闭后的回调 */
	onclose: (() => void) | null
	/** 发送文本数据 */
	send(data: string): void
	/** 关闭连接 */
	close(): void
}

/**
 * 延时执行一次回调
 *
 * @param cb 到点后执行的回调
 * @param ms 延时毫秒数
 * @param a 传给 cb 的额外实参
 * @returns 定时器 id，用于 clearTimeout 取消
 */
declare function setTimeout(cb: (...a: any[]) => void, ms?: number, ...a: any[]): number

/**
 * 周期重复执行回调
 *
 * @param cb 每间隔执行的回调
 * @param ms 间隔毫秒数
 * @param a 传给 cb 的额外实参
 * @returns 定时器 id，用于 clearInterval 取消
 */
declare function setInterval(cb: (...a: any[]) => void, ms?: number, ...a: any[]): number

/**
 * 在事件循环空闲时尽快执行回调
 *
 * @param cb 要尽快执行的回调
 * @returns 任务 id，用于 clearImmediate 取消
 */
declare function setImmediate(cb: (...a: any[]) => void): number

/** 取消 setTimeout 定时器；@param id setTimeout 返回的定时器 id */
declare function clearTimeout(id: number): void

/** 取消 setInterval 定时器；@param id setInterval 返回的定时器 id */
declare function clearInterval(id: number): void

/** 取消 setImmediate 任务；@param id setImmediate 返回的任务 id */
declare function clearImmediate(id: number): void

/** 控制台输出（写入引擎日志） */
//@ts-ignore
declare const console: {
	/** 通用输出；@param a 任意个要打印的值 */
	log(...a: any[]): void
	/** 信息输出；@param a 任意个要打印的值 */
	info(...a: any[]): void
	/** 警告输出；@param a 任意个要打印的值 */
	warn(...a: any[]): void
	/** 错误输出；@param a 任意个要打印的值 */
	error(...a: any[]): void
	/** 调试输出；@param a 任意个要打印的值 */
	debug(...a: any[]): void
}
