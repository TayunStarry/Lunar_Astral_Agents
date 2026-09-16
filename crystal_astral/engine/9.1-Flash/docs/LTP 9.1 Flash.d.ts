// ═══════════════════════════════════════════════════════════
// 类型定义
// ═══════════════════════════════════════════════════════════

/** 记忆命中项 */
interface MemoryHit {
    /** 记忆正文 */
    content: string
    /** 相似度 */
    similarity: number
}

/** 事件负载 */
interface EventLoad<P = unknown> {
    /** 事件主题 */
    type: string
    /** 事件数据 */
    payload: P
}

/** 放行 */
interface PassResult { }

/** 拦截：短路后续订阅器 */
interface InterceptResult {
    intercept: true
}

/** 改写：替换事件负载 */
interface ModifyResult {
    modifiedData: {
        content?: string
        senderName?: string
        [key: string]: unknown
    }
}

/** 撤回：事件不再派发 */
interface CancelResult {
    cancel: true
}

/** 订阅器可返回的结果（推荐 `{ return: 业务结果 }` 回传月华） */
type HandlerResult = PassResult | InterceptResult | ModifyResult | CancelResult | Record<string, unknown>

/** 默认事件主题（仅类型层，运行时直接用字符串） */
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

/** OpenAI 兼容消息 */
interface PostMessage {
    /** 消息角色 */
    role: 'user' | 'assistant' | 'system' | 'tool'
    /** 消息内容 */
    content: string | Array<TextItem | ImageItem>
}

/** 文本项 */
interface TextItem {
    type: 'text'
    /** 文本内容 */
    text: string
}

/** 图片项 */
interface ImageItem {
    type: 'image_url'
    /** 图片地址 */
    image_url: {
        url: string
    }
}

/** 月华记忆检索记录 */
interface RagRecord {
    /** 记录 ID */
    id: string
    /** 记录角色 */
    role: string
    /** 记录正文 */
    content?: string
    /** 记录图片（base64） */
    image?: string
    /** 相似度 */
    similarity: number
}

/** 月华音乐作品详情 */
interface MusicPieceDetail {
    /** 作品标题 */
    title: string
    /** 乐器列表（逗号分隔） */
    instruments: string
    /** 演奏速度（BPM） */
    tempo: number
    /** 段落结构 */
    structure: string
    /** 调式 */
    key: string
    /** 拍号 */
    meter: string
    /** ABC 乐谱长度（小节） */
    abcLength: number
}

/** 月华绘画作品详情 */
interface PaintingDetail {
    /** 工具名称 */
    toolName: string
    /** 提示词摘要 */
    promptSummary: string
    /** 表情 */
    expression?: string
    /** 姿势 */
    posture?: string
    /** 环境 */
    environment?: string
}

/** 月华事件契约：payload 为送入数据，return 为应回传的业务结果（`{ return: ... }`，空则月华走默认处理） */
interface EventContract {
    /** 收到消息前：改写未读消息与待处理视频 */
    message_received_before: {
        payload: {
            /** 未读消息上下文 */
            messages: PostMessage[]
            /** 待处理视频地址 */
            videos: string[]
        }
        /** 给出的字段替换月华对应队列 */
        return: {
            messages?: PostMessage[]
            videos?: string[]
        }
    }
    /** 执行计划前：改写到期计划正文 */
    execution_schedule_before: {
        payload: {
            /** 计划正文 */
            plan: string
        }
        return: {
            plan?: string
        }
    }
    /** 观看视频前：直接给出解析结果，跳过月华本地逐帧解析 */
    watch_video_before: {
        payload: {
            /** 用户随视频提出的需求 */
            userNeeds?: string
            /** 待处理视频地址 */
            videoUrls: string[]
        }
        /** 解析结果消息数组，追加进月华未读上下文 */
        return: PostMessage[]
    }
    /** 读取文件前：改写 `(#文件)` 引用的消息原文 */
    read_file_before: {
        payload: {
            /** 消息原文 */
            text: string
        }
        return: {
            text?: string
        }
    }
    /** 做出行动前：改写行动块数组 */
    take_action_before: {
        payload: {
            /** 行动块数组 */
            actions: string[]
        }
        return: {
            actions?: string[]
        }
    }
    /** 表达情感前：改写表情匹配参考文本 */
    express_emotions_before: {
        payload: {
            /** 正文参考文本 */
            text: string
        }
        return: {
            text?: string
        }
    }
    /** 构建记忆前：替换本次检索记录 */
    build_memory_before: {
        payload: {
            /** 检索所用用户消息 */
            userMessages: string[]
            /** 检索到的记忆碎片 */
            records: RagRecord[]
        }
        /** 替换后的检索记录，直接用于摘要 */
        return: RagRecord[]
    }
    /** 演奏音乐前：替换音乐作品详情 */
    play_music_before: {
        payload: {
            /** 音乐作品详情 */
            pieces: MusicPieceDetail[]
        }
        return: MusicPieceDetail[]
    }
    /** 绘制画作前：替换绘画作品详情 */
    draw_painting_before: {
        payload: {
            /** 绘画作品详情 */
            paintings: PaintingDetail[]
        }
        return: PaintingDetail[]
    }
    /** 执行搜索前：改写搜索查询语句 */
    execute_search_before: {
        payload: {
            /** 搜索查询语句 */
            query: string
        }
        return: {
            query?: string
        }
    }
}

/** 月华事件主题 */
type EventTopic = keyof EventContract

/** 月华事件回传：`{ return }` 即把业务结果交回月华 */
interface ReturnResult<R> {
    /** 业务结果，格式见 EventContract 对应事件 */
    return: R
}

/** fetch 响应体 */
interface FetchResponse {
    /** 状态码 */
    status: number
    /** 响应头（键名小写） */
    headers: Record<string, string>
    /** 以文本读取响应体 */
    text(): Promise<string>
    /** 解析为 JSON */
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

// ═══════════════════════════════════════════════════════════
// 引擎能力
// ═══════════════════════════════════════════════════════════

/** 广播事件（需要权限: allow-signal） */
export const signal: {
    /**
     * 在引擎中广播一条内容
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
    /**
     * 注册广播事件订阅器回调
     * 
     * @param handler 收到广播时执行；data 为广播内容
     * 
     * @returns 订阅 id，用于后续 unsubscribe 退订
     */
    subscribe(handler: (data: unknown) => void): number
    /**
     * 按订阅 id 退订广播事件订阅器回调
     * 
     * @param id subscribe 返回的订阅 id
     */
    unsubscribe(id: number): void
}
/** 文件能力（需要权限: allow-file） */
export const file: {
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
/** 向量记忆库（需要权限: allow-memory） */
export const memory: {
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
    search(v: { query: string; limit?: number }): Array<MemoryHit>
    /**
     * 按查询语义检索图片
     * 
     * @param query 检索文本
     * 
     * @param opts { limit? } 最多返回条数
     * 
     * @returns { success, results: [{ image, similarity }] }
     */
    searchImage(query: string, opts?: { limit?: number }): { success: boolean; results?: Array<{ image: string; similarity: number }>; error?: string }
    /**
     * 往 stickers 集合添加一张图片（base64；标签由记忆库 LLM 自动生成）
     * 
     * @param image 待添加的图片 base64
     */
    storeImage(image: string): { success: boolean; id?: string; error?: string }
    /** 基于查询语义随机返回一张图片 */
    randomImage(query?: string): { success: boolean; image?: string; error?: string }
}
/** SQLite 数据库（需要权限: allow-database） */
export const database: {
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
/** 插件配置（需要权限: allow-config） */
export const config: {
    /**
     * 读取配置
     * 
     * @returns config.yaml 引擎启动时解析注入的对象；无配置时返回 undefined
     */
    read(): Record<string, unknown> | undefined
    /**
     * 编写配置
     * 
     * @param v 要写入的配置对象；更新内存并同步写回 config.yaml
     */
    write(v: Record<string, unknown>): void
}
/** 编解码工具（需要权限: allow-certificate） */
export const encoding: {
    /** base64 编码；data 为 UTF-8 字符串 */
    base64Encode(data: any): string
    /** base64 解码；失败返回 { error } */
    base64Decode(s: string): any
    /** URL 编码（查询转义） */
    urlEncode(s: string): string
    /** URL 解码；失败抛错 */
    urlDecode(s: string): string
    /**
     * @param key 加解密密钥字符串
     * 
     * @param content 要加密的明文
     * 
     * @returns 加密后的密文
     */
    lunarEncoder(key: string, content: string): string
    /**
     * @param key 加解密密钥字符串
     * 
     * @param cipher 要解密的密文
     * 
     * @returns 解密后的原文
     */
    lunarDecoder(key: string, cipher: string): string
}
/** 哈希工具（需要权限: allow-certificate） */
export const hash: {
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
export const image: {
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
/** 模型能力（需要权限: allow-agent） */
export const agent: {
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
    chat(messages: Array<PostMessage>, opts?: { temperature?: number; max_tokens?: number; system?: string; tools?: unknown[] }): { success: boolean; text?: string; tool_calls?: unknown[]; error?: string }
    /**
     * 计算文本的嵌入向量（读取 lunar_config.json 的 agent.embedding 字段的文本嵌入模型）
     * 
     * @param input 单条字符串或字符串数组；也兼容 opts.text / opts.texts
     * 
     * @returns 单条返回 { success:true, embedding: number[] }；多条返回 { success:true, embeddings: number[][] }；失败返回 failure
     */
    embed(input: string | string[], opts?: { text?: string; texts?: string[] }): { success: boolean; embedding?: number[]; embeddings?: number[][]; error?: string }
    /**
     * 调用前端智能体（Mini-LTP / Node-LTP，需要权限: allow-agent）
     * 以自然语言驱动目标前端智能体（同步阻塞，返回其执行文本）
     * 
     * @param pkgId 目标插件目录名（插件 ID）
     * 
     * @param text 要交给智能体的自然语言输入
     * 
     * @returns 智能体执行结果文本；目标包不存在时抛出「xxx 包拒绝响应」
     */
    synergy: (pkgId: string, text: string) => string
}
/** 事件处理 */
export const event: {
    /**
     * 订阅某主题事件，注册一个回调
     * 
     * @param topic 事件主题字符串，与 E 端发起的 topic 对应
     * 
     * @param handler 收到事件时的回调；同步返回 StarHandlerResult（intercept/modifiedData/cancel）或业务结果对象
     * 
     * @param priority 可选订阅优先级，仅支持正整数（0 开始，0 为最高优先级）；未设置则为纯时间顺序
     * 
     * @typeParam T 月华事件主题
     * 
     * @returns 订阅 id，用于后续 unsubscribe 退订
     */
    subscribe<T extends EventTopic>(topic: T, handler: (e: EventLoad<EventContract[T]['payload']>) => ReturnResult<EventContract[T]['return']> | HandlerResult, priority?: number): number
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
/** 时间戳 */
export const time: {
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
export const exportFunction: <F extends (...args: any[]) => any>(name: string, fn: F) => void
/**
 * 跨包调用（allow-call）：调用目标插件导出的函数
 * 
 * @param pkgId 目标插件目录名（插件 ID）
 * 
 * @param name 目标插件已 export 的函数名
 * 
 * @param args 传给该函数的实参数组
 * 
 * @typeParam T 返回结果的类型
 * 
 * @returns 目标函数返回值；目标包不存在或未导出时抛出「xxx 包拒绝响应」
 */
export const callFunction: <T = unknown>(pkgId: string, name: string, args: T) => T
/** 同步休眠 */
export const sleep: (ms: number) => void
/** 同步 HTTP 请求 (需要权限: allow-network)*/
export const http: {
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
/** 裸 TCP/UDP/DNS（需要权限: allow-network；覆盖 WebSocket 替代不了的非 HTTP 场景） */
export const network: {
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
export const command: {
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
export const async: {
    /** 启动异步子任务，返回 taskId；taskFn 收到 (task)，task.id 即 taskId */
    run(taskFn: (task: { id: number; data?: unknown }) => unknown, options?: { timeout?: number; data?: unknown }): { success: boolean; text?: string; error?: string }
    /** 上报任务进度 */
    reportProgress(taskId: number, progress: unknown): { success: boolean; error?: string }
    /** 查询任务状态 */
    getStatus(taskId: number): { success: boolean; taskId?: number; status?: string; progress?: unknown; error?: string }
    /** 枚举全部任务状态 */
    list(): Array<{ taskId: number; status: string; progress?: unknown }>
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
/**
 * WebSocket 客户端（全局 `WebSocket`，沙箱主动连服务器；需要权限: allow-network）
 * 
 * @param url 目标 WebSocket 地址（ws:// 或 wss://）
 */
//@ts-ignore
declare class WebSocket {
    /** 构造函数；@param url 目标地址 */
    constructor(url: string)
    /** 连接状态：0=CONNECTING 1=OPEN 2=CLOSING 3=CLOSED */
    readyState: number
    /** 连接成功打开后的回调 */
    onopen: (() => void) | null
    /** 收到消息时的回调；参数为 { data: string } */
    onmessage: ((ev: { data: string }) => void) | null
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