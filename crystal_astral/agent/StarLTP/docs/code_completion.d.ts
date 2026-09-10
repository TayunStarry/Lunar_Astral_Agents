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

/**
 * 事件回调可返回的结果：放行 / 拦截 / 改写 / 撤回 / 任意业务结果对象（非 Promise）
 * 
 * 月华事件（xxx_before 系列）的推荐返回为 StarMoonReturnResult（`{ return: 业务结果 }`）。
 */
type StarHandlerResult = StarPassResult | StarInterceptResult | StarModifyResult | StarCancelResult | Record<string, unknown>

/**
 * 默认事件主题
 * 
 * 仅作类型层枚举（供编辑器补全与类型推断）；引擎沙箱不注入该对象，
 * 运行时请直接使用等值字符串（如 `'watch_video_before'`）。
 */
declare enum defaultEventTopic {
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
	BUILD_MEMORY_BEFORE = 'build_memory_before',
	/** 演奏音乐前 */
	PLAY_MUSIC_BEFORE = 'play_music_before',
	/** 绘制画作前 */
	DRAW_PAINTING_BEFORE = 'draw_painting_before',
	/** 执行搜索前 */
	EXECUTE_SEARCH_BEFORE = 'execute_search_before',
}

// ═══════════════════════════════════════════════════════════
// 月华事件（xxx_before）标准接口
// ═══════════════════════════════════════════════════════════

/** 月华消息（与月华对话上下文中的消息结构一致） */
interface StarPostMessage {
	/** 消息角色 */
	role: 'user' | 'assistant' | 'system' | 'tool'
	/** 消息内容：纯文本字符串，或多模态内容项数组 */
	content: string | Array<StarTextItem | StarImageItem>
}

/** 多模态消息中的文本项 */
interface StarTextItem {
	/** 内容项类型 */
	type: 'text'
	/** 文本内容 */
	text: string
}

/** 多模态消息中的图片项 */
interface StarImageItem {
	/** 内容项类型 */
	type: 'image_url'
	/** 图片地址（base64 data URL 或 http 链接） */
	image_url: {
		/** 图片的 URL 地址 */
		url: string
	}
}

/** 月华长期记忆检索记录（构建记忆前事件中送入/取出的碎片） */
interface StarRagRecord {
	/** 记录唯一标识 */
	id: string
	/** 记录角色 */
	role: string
	/** 记录正文 */
	content?: string
	/** 记录图片（base64） */
	image?: string
	/** 与本次查询的相似度 */
	similarity: number
}

/** 月华音乐作品详情（演奏音乐前事件） */
interface StarMusicPieceDetail {
	/** 作品标题 */
	title: string
	/** 使用的乐器列表，多个乐器用逗号分隔 */
	instruments: string
	/** 演奏速度（BPM） */
	tempo: number
	/** 段落结构描述 */
	structure: string
	/** 调式，如 C大调、a小调 */
	key: string
	/** 拍号，如 4/4、3/4 */
	meter: string
	/** ABC 乐谱长度（小节） */
	abcLength: number
}

/** 月华绘画作品详情（绘制画作前事件） */
interface StarPaintingDetail {
	/** 工具名称：self_portrait / diffusion_generation */
	toolName: string
	/** 正向提示词摘要（自画像固定为「自画像」） */
	promptSummary: string
	/** 表情（自画像专用） */
	expression?: string
	/** 姿势（自画像专用） */
	posture?: string
	/** 环境（自画像专用） */
	environment?: string
}

/**
 * 月华事件契约：payload 为月华送入该事件的数据格式，return 为订阅器应回传的业务结果格式。
 * 
 * 订阅器返回 `{ return: <return 格式的数据> }` 即把业务结果交回月华；
 * 不返回（或 return 为空）时月华走本地默认处理。
 */
interface StarMoonEventContract {
	/** 收到消息前：月华把待消费的未读消息与视频交给插件改写 */
	message_received_before: {
		/** 事件负载：当前未读消息与待处理视频 */
		payload: {
			/** 未读消息上下文 */
			messages: StarPostMessage[]
			/** 待处理视频地址列表 */
			videos: string[]
		}
		/** 回传：给到的数组字段将替换月华对应队列（未给出的字段保持不变） */
		return: {
			/** 替换后的未读消息上下文 */
			messages?: StarPostMessage[]
			/** 替换后的待处理视频地址列表 */
			videos?: string[]
		}
	}
	/** 执行计划前：到期计划即将写入月华上下文 */
	execution_schedule_before: {
		/** 事件负载：到期计划正文 */
		payload: {
			/** 计划正文 */
			plan: string
		}
		/** 回传：替换后的计划正文 */
		return: {
			/** 替换后的计划正文 */
			plan?: string
		}
	}
	/** 观看视频前：插件可直接给出视频解析结果，月华将追加进上下文并跳过本地逐帧解析 */
	watch_video_before: {
		/** 事件负载：待处理视频与用户需求 */
		payload: {
			/** 用户随视频提出的需求（可为空） */
			userNeeds?: string
			/** 待处理视频地址列表 */
			videoUrls: string[]
		}
		/** 回传：视频解析结果消息数组，按顺序追加进月华未读上下文 */
		return: StarPostMessage[]
	}
	/** 读取文件前：待解析 `(#文件)` 引用的消息正文 */
	read_file_before: {
		/** 事件负载：待解析引用的消息原文 */
		payload: {
			/** 消息原文 */
			text: string
		}
		/** 回传：替换后的消息原文 */
		return: {
			/** 替换后的消息原文 */
			text?: string
		}
	}
	/** 做出行动前：月华解析出的行动块即将交给行动者 */
	take_action_before: {
		/** 事件负载：行动块数组 */
		payload: {
			/** 行动块数组 */
			actions: string[]
		}
		/** 回传：替换后的行动块数组 */
		return: {
			/** 替换后的行动块数组 */
			actions?: string[]
		}
	}
	/** 表达情感前：用于匹配表情包的正文参考文本 */
	express_emotions_before: {
		/** 事件负载：正文参考文本 */
		payload: {
			/** 正文参考文本 */
			text: string
		}
		/** 回传：替换后的表情参考文本 */
		return: {
			/** 替换后的表情参考文本 */
			text?: string
		}
	}
	/** 构建记忆前：插件可替换本次检索记录，再交由月华做摘要 */
	build_memory_before: {
		/** 事件负载：用户消息与检索记录 */
		payload: {
			/** 本次检索所用的用户消息 */
			userMessages: string[]
			/** 月华检索到的长期记忆碎片 */
			records: StarRagRecord[]
		}
		/** 回传：替换后的检索记录数组（直接用于摘要） */
		return: StarRagRecord[]
	}
	/** 演奏音乐前：插件可替换作品详情，再交由月华汇总报幕 */
	play_music_before: {
		/** 事件负载：本次创作的音乐作品详情 */
		payload: {
			/** 音乐作品详情数组 */
			pieces: StarMusicPieceDetail[]
		}
		/** 回传：替换后的音乐作品详情数组（直接用于汇总） */
		return: StarMusicPieceDetail[]
	}
	/** 绘制画作前：插件可替换作品详情，再交由月华汇总报幕 */
	draw_painting_before: {
		/** 事件负载：本次绘制的作品详情 */
		payload: {
			/** 绘画作品详情数组 */
			paintings: StarPaintingDetail[]
		}
		/** 回传：替换后的绘画作品详情数组（直接用于汇总） */
		return: StarPaintingDetail[]
	}
	/** 执行搜索前：插件可改写搜索查询语句 */
	execute_search_before: {
		/** 事件负载：研究需求原文 */
		payload: {
			/** 搜索查询语句 */
			query: string
		}
		/** 回传：替换后的查询语句 */
		return: {
			/** 替换后的查询语句 */
			query?: string
		}
	}
}

/** 月华事件主题（取值与 defaultEventTopic 的月华事件成员一致） */
type StarMoonEventTopic = keyof StarMoonEventContract

/** 月华事件回传结果：返回该对象即把业务结果交回月华（月华事件的推荐返回格式） */
interface StarMoonReturnResult<R> {
	/** 回传月华的业务结果；格式见 StarMoonEventContract 对应事件 */
	return: R
}

/** 事件订阅器（同主题可重复订阅） */
interface StarEventBus {
	/**
	 * 订阅月华事件（xxx_before 系列）：按事件名推断负载格式与推荐返回格式
	 * 
	 * @param topic 月华事件主题（运行时用等值字符串，如 'watch_video_before'；类型层亦可用 defaultEventTopic.WATCH_VIDEO_BEFORE 补全）
	 * 
	 * @param handler 收到事件时的回调；e.payload 即该事件的负载格式，
	 *                推荐返回 { return: 业务结果 } 交回月华（格式见 StarMoonEventContract）
	 * 
	 * @param priority 可选订阅优先级，仅支持正整数（0 开始，0 为最高优先级）；未设置则为纯时间顺序
	 * 
	 * @typeParam T 月华事件主题
	 * 
	 * @returns 订阅 id，用于后续 unsubscribe 退订
	 */
	subscribe<T extends StarMoonEventTopic>(
		topic: T,
		handler: (e: StarEvent<StarMoonEventContract[T]['payload']>) => StarMoonReturnResult<StarMoonEventContract[T]['return']> | StarHandlerResult,
		priority?: number
	): number
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
	/**
	 * 插件主动在事件总线上发布一个事件，派发给其它订阅该主题的插件订阅器，并转发给宿主供外部客户端消费
	 * 
	 * @param topic 事件主题字符串
	 * 
	 * @param payload 事件负载（任意可 JSON 化数据）
	 */
	publish(topic: string, payload?: unknown): void
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
	 * @returns 成功返回 { success:true, id }；失败返回 { success:false, error }
	 */
	store(v: { content: string; tags?: string[] }): { success: boolean; id?: number; error?: string }
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
	 * @returns 成功返回 { success:true, rows:行数组 }；每行为「列名 → 值」的对象；失败含 error
	 */
	query(sql: string, params?: unknown[]): { success: boolean; rows: Array<Record<string, unknown>>; error?: string }

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

/** 编解码工具（常驻基础能力，不参与 allow-* 开关） */
interface StarEncoding {
	/** base64 编码；data 为 UTF-8 字符串 */
	base64Encode(data: string): string
	/** base64 解码；失败返回 { error } */
	base64Decode(s: string): string
	/** 十六进制编码 */
	hexEncode(data: string): string
	/** 十六进制解码；失败抛错 */
	hexDecode(s: string): string
	/** URL 编码（查询转义） */
	urlEncode(s: string): string
	/** URL 解码；失败抛错 */
	urlDecode(s: string): string
	/** UTF-8「编码」（LTP9 内部即 UTF-8 字符串，直通返回） */
	utf8Encode(s: string): string
	/** UTF-8「解码」（直通返回） */
	utf8Decode(s: string): string
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
	/**
	 * MD5 摘要
	 * 
	 * @param data 输入字符串
	 * 
	 * @returns 小写十六进制摘要
	 */
	md5(data: string): string
	/**
	 * SHA-1 摘要
	 * 
	 * @param data 输入字符串
	 * 
	 * @returns 小写十六进制摘要
	 */
	sha1(data: string): string
	/**
	 * SHA-256 摘要
	 * 
	 * @param data 输入字符串
	 * 
	 * @returns 小写十六进制摘要
	 */
	sha256(data: string): string
	/**
	 * HMAC-SHA1
	 * 
	 * @param key 密钥
	 * @param data 输入字符串
	 * 
	 * @returns 小写十六进制 MAC
	 */
	hmacSha1(key: string, data: string): string
	/**
	 * HMAC-SHA256
	 * 
	 * @param key 密钥
	 * @param data 输入字符串
	 * 
	 * @returns 小写十六进制 MAC
	 */
	hmacSha256(key: string, data: string): string
	/**
	 * Ed25519 签名
	 * 
	 * @param privKeyPemOrSeed Ed25519 私钥（PEM(PKCS8) 或 32/64 字节原始字节）
	 * @param data 待签名数据
	 * 
	 * @returns base64url 签名；失败时抛出
	 */
	ed25519Sign(privKeyPemOrSeed: string, data: string): string
	/**
	 * 生成 EdDSA (Ed25519) JWT
	 * 
	 * @param claims 令牌负载对象
	 * @param privKeyPemOrSeed Ed25519 私钥（PEM(PKCS8) 或 32/64 字节原始字节）
	 * @param kid 可选；非空时写入 JWT 头
	 * 
	 * @returns JWT 字符串（header.payload.signature）
	 */
	generateJWT(claims: Record<string, unknown>, privKeyPemOrSeed: string, kid?: string): string
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
	 * @param opts 可选参数 { temperature?, max_tokens?, system?, tools? }
	 *             system  为系统提示词字符串（自动前置为一条 system 消息）
	 *             tools   为给模型提供的函数/工具定义数组
	 * 
	 * @returns 成功返回 { success:true, text: 回复内容, tool_calls?: 模型函数调用 }；
	 *          模型走函数调用时 content 为空，tool_calls 原样透传（供 AtoA 接头）；失败返回 failure
	 */
	chat(messages: Array<{ role: string; content: string }>, opts?: { temperature?: number; max_tokens?: number; system?: string; tools?: unknown[] }): { success: boolean; text?: string; tool_calls?: unknown[]; error?: string }
	/**
	 * 计算文本的嵌入向量（读取 lunar_config.json 的 agent.embedding 字段的文本嵌入模型）
	 * 
	 * @param input 单条字符串或字符串数组；也兼容 opts.text / opts.texts
	 * 
	 * @returns 单条返回 { success:true, embedding: number[] }；多条返回 { success:true, embeddings: number[][] }；失败返回 failure
	 */
	embed(input: string | string[], opts?: { text?: string; texts?: string[] }): { success: boolean; embedding?: number[]; embeddings?: number[][]; error?: string }
}

/** Agent 工具注册（需要权限: allow-agent；供 LLM tool_calls / AtoA 接头调用） */
interface StarTool {
	/**
	 * 注册一个 LLM 可调用的函数工具
	 * 
	 * @param name 工具名（唯一标识，供 LLM tool_calls 与宿主 CallTool 精确匹配）
	 * @param definition 工具定义 { description?, parameters? }；parameters 为参数定义数组
	 * @param handler (params) 处理器，params 为 LLM 调用工具时的参数对象；返回任意业务结果
	 */
	register(name: string, definition: { description?: string; parameters?: unknown[] }, handler: (params: Record<string, unknown>) => unknown): { success: boolean; error?: string }
	/**
	 * 获取当前插件已注册的工具定义（含 name/description/parameters）
	 */
	getDefinitions(): Array<{ name: string; description?: string; parameters?: unknown[] }>
}

/** 平台上下文（需要权限: allow-send；由宿主注入 platformResolver 解析） */
interface StarPlatform {
	/** 获取当前平台名称 */
	getName(): { success: boolean; value?: unknown; error?: string }
	/** 获取当前群 ID */
	getGroupId(): { success: boolean; value?: unknown; error?: string }
	/** 按群 ID + 昵称解析用户（未接入时返回 error） */
	lookupUser(groupId: string, name: string): { success: boolean; value?: unknown; error?: string }
}

/** 表情包（需要权限: allow-memory；内部复用项目记忆库 stickers 集合） */
interface StarEmoji {
	/**
	 * 按查询语义检索表情包图片
	 * 
	 * @param query 检索文本
	 * @param opts { limit? } 最多返回条数
	 * 
	 * @returns { success, results: [{ image, similarity }] }
	 */
	search(query: string, opts?: { limit?: number }): { success: boolean; results?: Array<{ image: string; similarity: number }>; error?: string }
	/**
	 * 往 stickers 集合添加一张表情包图片（base64；标签由记忆库 LLM 自动生成）
	 * 
	 * @param image 待添加的图片 base64
	 */
	store(image: string): { success: boolean; id?: string; error?: string }
	/** 基于查询语义随机返回一张表情包图片 */
	random(query?: string): { success: boolean; image?: string; error?: string }
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
	/** Agent 工具注册（需要权限: allow-agent；供 LLM tool_calls / AtoA 接头调用） */
	tool: StarTool
	/** 平台上下文（需要权限: allow-send；由宿主注入 platformResolver 解析） */
	platform: StarPlatform
	/** 表情包（需要权限: allow-memory；内部复用项目记忆库 stickers 集合） */
	emoji: StarEmoji
	/** 发送到会话（需要权限: allow-send） */
	send: StarSend
	/** WebSocket 服务端（需要权限: allow-socket） */
	ws: StarWs
	/** 同步 HTTP（需要权限: allow-network；engine.http.get/post/download，阻塞式、返回普通对象，非 Promise） */
	http: StarHttpSync
	/** 同步休眠（engine.sleep(ms)，阻塞式；常驻基础能力） */
	sleep(ms: number): void
	/** 插件配置（常驻基础能力，不参与 allow-* 开关） */
	config: StarConfig
	/** 编解码工具（常驻基础能力，不参与 allow-* 开关） */
	encoding: StarEncoding
	/** 裸 TCP/UDP/DNS（需要权限: allow-network） */
	network: StarNetwork
	/** 指令系统（常驻基础能力） */
	command: StarCommand
	/** 异步子任务（常驻基础能力） */
	async: StarAsync
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
	/**
	 * 同步下载任意文件到插件数据目录并落盘
	 * 
	 * @param url 文件直链
	 * @param savePath 相对插件数据目录的保存路径；为空时按 URL 末段命名
	 * 
	 * @returns { success:true, path: 相对路径, size: 字节数 }；失败返回 { success:false, error }
	 */
	download(url: string, savePath?: string): { success: boolean; path?: string; size?: number; error?: string }
}

/** 网络套接字（engine.network connect/listen 返回值；读取阻塞带超时） */
interface StarNetSocket {
	/** 以文本写入；返回 { success } 或 { success:false, error } */
	send(data: string): { success: boolean; error?: string }
	/** 阻塞读取一段文本（带超时，秒）；返回 { success, data }，UDP 额外含 host/port */
	receive(timeoutSec?: number): { success: boolean; data?: string; host?: string; port?: number; error?: string }
	/** 向指定主机/端口发送 UDP 数据（udpListen 场景） */
	sendTo(host: string, port: number, data: string): { success: boolean; error?: string }
	/** 关闭套接字 */
	close(): { success: boolean; error?: string }
}

/** 裸 TCP/UDP/DNS（需要权限: allow-network；覆盖 WebSocket 替代不了的非 HTTP 场景） */
interface StarNetwork {
	/** 解析主机名的全部地址 */
	resolveDNS(hostname: string, timeoutSec?: number): { success: boolean; addresses?: string[]; error?: string }
	/** 解析 SRV 记录 */
	resolveSRV(service: string, proto: string, hostname: string, timeoutSec?: number): { success: boolean; targets?: Array<{ target: string; port: number }>; error?: string }
	/** 建立 TCP 连接，返回套接字 */
	tcpConnect(host: string, port: number, timeoutSec?: number): StarNetSocket
	/** 建立已「连接」的 UDP 套接字 */
	udpConnect(host: string, port: number, timeoutSec?: number): StarNetSocket
	/** 在指定端口监听 UDP */
	udpListen(host?: string, port?: number): StarNetSocket
}

/** 指令系统（常驻基础能力，宿主经 Go 侧 Command/CommandAll 触发） */
interface StarCommand {
	/**
	 * 注册一条指令
	 * 
	 * @param name 指令名（唯一标识；别名一并登记）
	 * @param pattern 正则（可带 /.../ 分隔符）；**留空**时仅按指令名/别名精确匹配，
	 *                不参与正则回退（避免空正则劫持未匹配文本）
	 * @param handler (match, context) 处理器；match 为捕获组数组，context 为可选上下文
	 * @param options { aliases?: string[] } 别名列表，与 name 一并作精确匹配
	 */
	register(name: string, pattern: string, handler: (match: string[], context?: Record<string, unknown>) => unknown, options?: { aliases?: string[] }): { success: boolean; error?: string }
}

/** 异步子任务（常驻基础能力） */
interface StarAsync {
	/** 启动异步子任务，返回 taskId；taskFn 收到 (task)，task.id 即 taskId */
	run(taskFn: (task: { id: number; data?: unknown }) => unknown, options?: { timeout?: number; data?: unknown }): { success: boolean; text?: string; error?: string }
	/** 上报任务进度 */
	reportProgress(taskId: number, progress: unknown): { success: boolean; error?: string }
	/** 查询任务状态 */
	getStatus(taskId: number): { success: boolean; taskId?: number; status?: string; progress?: unknown; error?: string }
	/** 枚举全部任务状态 */
	list(): Array<{ taskId: number; status: string; progress?: unknown }>
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

//@ts-ignore
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

// ═══════════════════════════════════════════════════════════
// 生命周期（execute.js 可选定义）
// ═══════════════════════════════════════════════════════════

/** 插件加载完成时由引擎回调；无需参数与返回值 */
declare function onLoad(): void
/** 插件卸载前由引擎回调；无需参数与返回值 */
declare function onUnload(): void
/**
 * 配置回写（engine.config.setFile）后由引擎回调
 * 
 * @param scope 配置作用域（当前固定为 "plugin"）
 * @param config 更新后的配置对象
 * @param version 配置版本（当前为空字符串）
 */
declare function onConfigUpdate(scope: string, config: Record<string, unknown>, version: string): void