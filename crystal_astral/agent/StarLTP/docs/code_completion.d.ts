/**
 * StarLTP 插件层类型声明（code_completion.d.ts）
 *
 * 在 execute.js 顶部引用：
 *   /// <reference path="./code_completion.d.ts" />
 *
 * 插件运行于每插件独立 goja 沙箱，统一入口为回调注册（事件订阅）。
 * 宿主接口一律经全局 `engine` 获得；能力是否可用由 `permissions.key` 中的 `allow-*` 决定。
 * 注意：LTP9 接口全部为**同步**调用（返回普通值，非 Promise）；网络请用 `engine.http`（同步阻塞）。
 */

// ═══════════════════════════════════════════════════════════
// 事件回调结果
// ═══════════════════════════════════════════════════════════

/**
 * 事件负载（event.subscribe 回调参数）
 * 
 * @see StarEventBus.subscribe
 * 
 * @typeParam P 事件负载的具体类型
 */
interface StarEvent<P = unknown> {
	/** 事件主题（来电者提供的 topic 字符串） */
	type: string
	/** 事件附带的数据 */
	payload: P
}

/** 放行：不返回任何标记，事件继续派发给后续订阅器 */
interface StarPassResult { }

/**
 * 拦截：返回该对象以标记本次事件已被消费，短路该插件后续订阅器
 * @property intercept 固定为 true 才表示拦截
 */
interface StarInterceptResult {
	intercept: true
}

/**
 * 改写：返回该对象以替换事件参数，供后续订阅器与客户端读取
 * 
 * @property modifiedData 替换后的字段；content 与 senderName 为常用项，其余可自定义扩展
 */
interface StarModifyResult {
	modifiedData: {
		content?: string
		senderName?: string
		[key: string]: unknown
	}
}

/** 撤回：返回该对象以标记本次事件已被撤回，不再派发（含后续订阅器与其他插件） */
interface StarCancelResult {
	cancel: true
}

/** 事件回调可返回的结果：放行 / 拦截 / 改写 / 撤回 / 任意业务结果对象（非 Promise） */
type StarHandlerResult = StarPassResult | StarInterceptResult | StarModifyResult | StarCancelResult | Record<string, unknown>

/** 默认事件主题 */
export enum defaultEventTopic {
	/** 收到消息前 */
	MESSAGE_RECEIVED_BEFORE = 'message_received_before',
	/** 执行计划前 */
	EXECUTION_SCHEDULE_BEFORE = 'execution_schedule_before',
	/** 观看视频前 */
	WATCH_VIDEO_BEFORE = 'watch_video_before',
	/** 读取文件前 */
	READ_FILE_BEFORE = 'read_file_before',
	/** 做出行动前 */
	TAKE_ACTION_BEFORE = 'take_action_before',
	/** 表达情感前 */
	EXPRESS_EMOTIONS_BEFORE = 'express_emotions_before',
	/** 构建记忆前 */
	BUILD_MEMORIES_BEFORE = 'build_memories_before',
	/** 演奏音乐前 */
	PLAY_MUSIC_BEFORE = 'play_music_before',
	/** 绘制画作前 */
	DRAW_PAINTING_BEFORE = 'draw_painting_before',
	/** 执行搜索前 */
	EXECUTE_SEARCH_BEFORE = 'execute_search_before',
}

/** 事件订阅器（同主题可重复订阅） */
interface StarEventBus {
	/**
	 * 订阅某主题事件，注册一个回调
	 * 
	 * @param topic 事件主题字符串，与 E 端发起的 topic 对应
	 * 
	 * @param handler 收到事件时的回调；同步返回 StarHandlerResult（intercept/modifiedData/cancel）或业务结果对象
	 * 
	 * @param priority 可选订阅优先级，仅支持正整数（0 开始，0 为最高优先级）；未设置则为纯时间顺序
	 * 
	 * @typeParam P 该主题下事件负载的具体类型
	 * 
	 * @returns 订阅 id，用于后续 unsubscribe 退订
	 */
	subscribe<P = unknown>(topic: defaultEventTopic | string, handler: (e: StarEvent<P>) => StarHandlerResult, priority?: number): number
	/**
	 * 按订阅 id 退订某个事件
	 * 
	 * @param topic 事件主题字符串
	 * 
	 * @param id subscribe 返回的订阅 id
	 */
	unsubscribe(topic: string, id: number): void
}
/**
 * 事件订阅器派发顺序：优先级高的（数值小）排在前面；同优先级按订阅时间顺序；
 * 未设置优先级的订阅排在所有设置者之后并按时间顺序。
 */

// ═══════════════════════════════════════════════════════════
// engine 各能力
// ═══════════════════════════════════════════════════════════

/** 广播接收：engine.signal 的落点（allow-signal） */
interface StarFrontSignal {
	/**
	 * 注册前端信号订阅回调（engine.signal.all/target 触发）
	 * 
	 * @param handler 收到广播时执行；data 为广播内容
	 * 
	 * @returns 订阅 id，用于后续 unsubscribe 退订
	 */
	subscribe(handler: (data: unknown) => void): number
	/**
	 * 按订阅 id 退订前端信号
	 * 
	 * @param id subscribe 返回的订阅 id
	 */
	unsubscribe(id: number): void
}

/** 文件能力（allow-file） */
interface StarFile {
	/**
	 * 写文件
	 * 
	 * @param path 文件相对路径，限定在插件数据目录内
	 * 
	 * @param data 要写入的 UTF-8 文本内容
	 * 
	 * @returns 成功返回 { success:true }；失败（如路径越界）返回 { success:false, error }
	 */
	write(path: string, data: string): { success: boolean; error?: string }
	/**
	 * 读文件
	 * 
	 * @param path 文件相对路径，限定在插件数据目录内
	 * 
	 * @returns 成功返回 { success:true, text: 文件内容 }；不存在或读取失败返回 { success:false, error }
	 */
	read(path: string): { success: boolean; text?: string; error?: string }
	/**
	 * 删除文件
	 * 
	 * @param path 文件相对路径，限定在插件数据目录内
	 * 
	 * @returns 成功返回 { success:true }；失败返回 { success:false, error }
	 */
	delete(path: string): { success: boolean; error?: string }
}

/** 记忆库单条命中 */
interface StarMemoryHit {
	/** 记忆正文 */
	content: string
	/** 与本次查询的相似度（0 ~ 1） */
	similarity: number
}

/** 向量记忆库（allow-memory） */
interface StarMemory {
	/**
	 * 写入一条记忆
	 * 
	 * @param v 记忆内容；content 为正文，tags 为可检索的标签列表
	 * 
	 * @returns 写入成功返回 true
	 */
	store(v: { content: string; tags?: string[] }): boolean
	/**
	 * 按语义搜索记忆
	 * 
	 * @param v 查询参数；query 为待检索的文本，limit 最多返回条数（默认 5，上限 50）
	 * 
	 * @returns 相似度降序的命中列表；每项含 content 与 similarity
	 */
	search(v: { query: string; limit?: number }): Array<StarMemoryHit>
}

/** SQLite 数据库（allow-database） */
interface StarDatabase {
	/**
	 * 执行 SELECT 查询
	 * 
	 * @param sql SELECT 语句
	 * 
	 * @param params 绑定参数（? 占位符依次对应）
	 * 
	 * @returns 行数组；每行为「列名 → 值」的对象
	 */
	query(sql: string, params?: unknown[]): Array<Record<string, unknown>>

	/**
	 * 执行写语句
	 * 
	 * @param sql INSERT/UPDATE/DELETE 等写语句
	 * 
	 * @param params 绑定参数（? 占位符依次对应）
	 * 
	 * @returns 含 success 与受影响行数 rows_affected 的结果
	 */
	exec(sql: string, params?: unknown[]): { success: boolean; rows_affected: number }
}

/** 插件配置（config.yaml） */
interface StarConfig {
	/**
	 * 读取配置
	 * 
	 * @returns config.yaml 引擎启动时解析注入的对象；无配置时返回 undefined
	 */
	getFile(): Record<string, unknown> | undefined
	/**
	 * 回写配置
	 * 
	 * @param v 要写入的配置对象；更新内存并同步写回 config.yaml
	 */
	setFile(v: Record<string, unknown>): void
}

/** JWT 签名（需要权限: allow-certificate） */
interface StarCrypto {
	/**
	 * 生成 JWT 令牌
	 * 
	 * @param claims 令牌负载对象
	 * @param secret HS256 时为其 HMAC 密钥；EdDSA 时为 Ed25519 PEM(PKCS8) 私钥
	 * @param algorithm 算法，默认 HS256；可选 HS256 / EdDSA(Ed25519) / none
	 * @param kid 可选；非空时写入 JWT 头（多密钥按 Key ID 选择签名密钥）
	 * 
	 * @returns JWT 字符串（header.payload.signature）
	 */
	signJWT(claims: Record<string, unknown>, secret: string, algorithm?: string, kid?: string): string
}

/** 图像处理（需要权限: allow-file；下载另需 allow-network） */
interface StarImage {
	/**
	 * 读取插件数据目录内图片并校验，返回 base64
	 * 
	 * @param path 相对插件数据目录的路径
	 * 
	 * @returns 成功返回 { success:true, text: base64 }；非图片返回 failure
	 */
	loadValid(path: string): { success: boolean; text?: string; error?: string }
	/**
	 * 下载图片到插件数据目录并校验，返回 base64
	 * 
	 * @param url 图片直链
	 * @param fileName 相对数据目录的保存文件名
	 * 
	 * @returns 成功返回 { success:true, text: base64 }；失败返回 failure
	 */
	download(url: string, fileName: string): { success: boolean; text?: string; error?: string }
}

/** LLM 对话（需要权限: allow-agent；读取 lunar_config.json 的 agent 字段） */
interface StarLLM {
	/**
	 * 调用 v1/chat/completions 生成回复文本
	 * 
	 * @param messages OpenAI 消息数组 [{role, content}]
	 * @param opts 可选参数 { temperature?, max_tokens? }
	 * 
	 * @returns 成功返回 { success:true, text: 回复内容 }；失败返回 failure
	 */
	chat(messages: Array<{ role: string; content: string }>, opts?: { temperature?: number; max_tokens?: number }): { success: boolean; text?: string; error?: string }
}

/** 发送到会话（需要权限: allow-send；通道由宿主注入） */
interface StarSend {
	/**
	 * 发送文本
	 * 
	 * @param target 目标（如群 ID）
	 * @param text 文本内容
	 */
	text(target: string, text: string): { success: boolean; error?: string }
	/**
	 * 发送图片
	 * 
	 * @param target 目标（如群 ID）
	 * @param b64 图片 base64
	 */
	image(target: string, b64: string): { success: boolean; error?: string }
	/**
	 * 发送混合分段消息（text/image 段数组）
	 * 
	 * @param target 目标（如群 ID）
	 * @param segs 分段数组 [{ type: 'text', content } | { type: 'image', content: base64 }]
	 */
	hybrid(target: string, segs: Array<Record<string, unknown>>): { success: boolean; error?: string }
}

/** WebSocket 服务端（需要权限: allow-socket；传输由宿主注入 wsBridge） */
interface StarWs {
	/**
	 * 挂载一个 WebSocket 端点，返回访问地址
	 * 
	 * @param path 挂载路径（如 "/ws/flow"）
	 * @param handler 收到消息时的回调，返回值为回给客户端的文本
	 * 
	 * @returns 成功返回 { success:true, path: 访问地址 }
	 */
	expose(path: string, handler: (msg: string) => string): { success: boolean; path?: string; error?: string }
	/**
	 * 向本插件挂载的 WebSocket 路径广播数据
	 * 
	 * @param data 要广播的文本
	 */
	publish(data: string): { success: boolean; error?: string }
}

/** 宿主引擎（execute.js 内使用的全局 `engine`） */
interface StarEngine {
	/** 事件订阅器总线（客户端经 StarLTP.Emit 发起） */
	event: StarEventBus
	/** 前端事件：接收 engine.signal 广播（allow-signal） */
	frontEvent: { signal: StarFrontSignal }
	/** 广播（allow-signal） */
	signal: {
		/**
		 * 向所有插件广播一条内容
		 * 
		 * @param payload 要广播的内容（任意可 JSON 化数据）
		 */
		all(payload: unknown): void
		/**
		 * 仅向目标插件广播一条内容
		 * 
		 * @param pkgId 目标插件目录名（插件 ID）
		 * 
		 * @param payload 要广播的内容(任意可 JSON 化数据)
		 */
		target(pkgId: string, payload: unknown): void
	}
	/** 时间戳 */
	time: {
		/**
		 * 当前时间
		 * 
		 * @returns 秒级时间戳
		 */
		now(): number
		/**
		 * 当前时间
		 * 
		 * @returns 毫秒级时间戳
		 */
		nowMs(): number
	}
	/**
	 * 导出插件能力，供其它插件经 engine.call 调用
	 * 
	 * @param name 导出的函数名（供调用方 engine.call(pkgId).run(name, args) 使用）
	 * 
	 * @param fn 函数实现本体（同步，返回普通值；异步请用 engine.sleep/engine.http 阻塞式完成）
	 */
	export<F extends (...args: any[]) => any>(name: string, fn: F): void
	/**
	 * 跨包调用（allow-call）：调用目标插件导出的函数
	 * 
	 * @param pkgId 目标插件目录名（插件 ID）
	 */
	call(pkgId: string): {
		/**
		 * 执行目标插件导出的函数（同步阻塞，返回其值）
		 * 
		 * @param fnName 目标插件已 export 的函数名
		 * 
		 * @param args 传给该函数的实参数组
		 * 
		 * @typeParam T 返回结果的类型
		 * 
		 * @returns 目标函数返回值；目标包不存在或未导出时抛出「xxx 包拒绝响应」
		 */
		run<T = unknown>(fnName: string, args?: unknown[]): T
	}
	/**
	 * 调用前端智能体（Mini-LTP / Node-LTP，需要权限: allow-agent）
	 * 
	 * @param pkgId 目标插件目录名（插件 ID）
	 */
	agent(pkgId: string): {
		/**
		 * 以自然语言驱动目标前端智能体（同步阻塞，返回其执行文本）
		 * 
		 * @param text 要交给智能体的自然语言输入
		 * 
		 * @returns 智能体执行结果文本；目标包不存在时抛出「xxx 包拒绝响应」
		 */
		run(text: string): string
	}
	/**
	 * 加密（需要权限: allow-certificate）
	 * 
	 * @param key 加解密密钥字符串
	 * 
	 * @param content 要加密的明文
	 * 
	 * @returns 加密后的密文
	 */
	encoder(key: string, content: string): string
	/**
	 * 解密（需要权限: allow-certificate）	
	 * 
	 * @param key 加解密密钥字符串
	 * 
	 * @param cipher 要解密的密文
	 * 
	 * @returns 解密后的原文
	 */
	decoder(key: string, cipher: string): string
	/** 文件读写删（需要权限: allow-file） */
	file: StarFile
	/** 向量记忆库（需要权限: allow-memory） */
	memory: StarMemory
	/** SQLite 数据库（需要权限: allow-database） */
	database: StarDatabase
	/** JWT 签名（需要权限: allow-certificate） */
	crypto: StarCrypto
	/** 图像处理（需要权限: allow-file；下载另需 allow-network） */
	image: StarImage
	/** LLM 对话（需要权限: allow-agent） */
	llm: StarLLM
	/** 发送到会话（需要权限: allow-send） */
	send: StarSend
	/** WebSocket 服务端（需要权限: allow-socket） */
	ws: StarWs
	/** 同步 HTTP（需要权限: allow-network；engine.http.get/post，阻塞式、返回普通对象，非 Promise） */
	http: StarHttpSync
	/** 同步休眠（engine.sleep(ms)，阻塞式；常驻基础能力） */
	sleep(ms: number): void
	/** 插件配置（常驻基础能力，不参与 allow-* 开关） */
	config: StarConfig
}

/** 同步 HTTP（非 Promise，返回普通对象） */
interface StarHttpSync {
	/**
	 * 同步 GET；阻塞直到返回
	 * 
	 * @param url 请求地址
	 * @param headers 可选请求头
	 * 
	 * @returns { status, body }；出错时含 error
	 */
	get(url: string, headers?: Record<string, string>): { status: number; body: string; error?: string }
	/**
	 * 同步 POST；阻塞直到返回
	 * 
	 * @param url 请求地址
	 * @param body 请求体（JSON 字符串）
	 * @param headers 可选请求头
	 * 
	 * @returns { status, body }；出错时含 error
	 */
	post(url: string, body: string, headers?: Record<string, string>): { status: number; body: string; error?: string }
}

/** 宿主引擎全局对象 */
declare const engine: StarEngine

// ═══════════════════════════════════════════════════════════
// 沙箱宿主全局
// ═══════════════════════════════════════════════════════════

/** fetch 响应体 */
interface FetchResponse {
	/** HTTP 状态码 */
	status: number
	/** 响应头（键名小写） */
	headers: Record<string, string>
	/** 以文本读取响应体 */
	text(): Promise<string>
	/** 将响应体解析为 JSON */
	json(): Promise<unknown>
}

/** fetch 请求配置 */
interface FetchInit {
	/** 请求方法 */
	method?: string;
	/** 请求头 */
	headers?: Record<string, string>;
	/** 请求体 */
	body?: string;
}

/**
 * 发起网络请求（Node 风格 fetch，返回 Promise；需要权限: allow-network）
 * 
 * @param input 请求 URL
 * 
 * @param init 请求配置；method 为请求方法，headers 为请求头，body 为请求体
 * 
 * @returns 解析完成的 fetch 响应（含 status/headers/text()/json()）
 * 
 * @remarks 若需要**同步**（非 Promise）网络调用，请改用 `engine.http.get/post`。
 */
declare function fetch(input: string, init?: FetchInit): Promise<FetchResponse>

/** WebSocket 消息事件 */
interface StarWSMessage {
	/** 收到的消息数据 */
	data: unknown
}

/**
 * WebSocket 客户端（全局 `WebSocket`，沙箱主动连服务器；需要权限: allow-network）
 * 
 * @param url 目标 WebSocket 地址（ws:// 或 wss://）
 */
declare class StarWebSocket {
	/** 构造函数；@param url 目标地址 */
	constructor(url: string)
	/** 连接状态：0=CONNECTING 1=OPEN 2=CLOSING 3=CLOSED */
	readyState: number
	/** 连接成功打开后的回调 */
	onopen: (() => void) | null
	/** 收到消息时的回调；参数为 StarWSMessage，含 data */
	onmessage: ((ev: StarWSMessage) => void) | null
	/** 发生错误时的回调 */
	onerror: ((ev: { error: string }) => void) | null
	/** 连接关闭后的回调 */
	onclose: (() => void) | null
	/** 发送文本数据 */
	send(data: string): void
	/** 关闭连接 */
	close(): void
}

/** WebSocket 构造函数全局别名 */
declare const WebSocket: typeof StarWebSocket

/**
 * 延时执行一次回调
 * 
 * @param cb 到点后执行的回调
 * 
 * @param ms 延时毫秒数
 * 
 * @param a 传给 cb 的额外实参
 * 
 * @returns 定时器 id，用于 clearTimeout 取消
 */
declare function setTimeout(cb: (...a: any[]) => void, ms?: number, ...a: any[]): number

/**
 * 周期重复执行回调
 * 
 * @param cb 每间隔执行的回调
 * 
 * @param ms 间隔毫秒数
 * 
 * @param a 传给 cb 的额外实参
 * 
 * @returns 定时器 id，用于 clearInterval 取消
 */
declare function setInterval(cb: (...a: any[]) => void, ms?: number, ...a: any[]): number

/**
 * 在事件循环空闲时尽快执行回调
 * 
 * @param cb 要尽快执行的回调
 * 
 * @returns 任务 id，用于 clearImmediate 取消
 */
declare function setImmediate(cb: (...a: any[]) => void): number

/**
 * 取消 setTimeout 定时器
 * 
 * @param id setTimeout 返回的定时器 id
 */
declare function clearTimeout(id: number): void

/**
 * 取消 setInterval 定时器
 * 
 * @param id setInterval 返回的定时器 id
 */
declare function clearInterval(id: number): void

/**
 * 取消 setImmediate 任务
 * 
 * @param id setImmediate 返回的任务 id
 */
declare function clearImmediate(id: number): void

/** 控制台输出 */
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

// ═══════════════════════════════════════════════════════════
// 生命周期（execute.js 可选定义）
// ═══════════════════════════════════════════════════════════

/** 插件加载完成时由引擎回调；无需参数与返回值 */
declare function onLoad(): void
/** 插件卸载前由引擎回调；无需参数与返回值 */
declare function onUnload(): void