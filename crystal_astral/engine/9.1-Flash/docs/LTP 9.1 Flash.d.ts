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

/** OpenAI 兼容消息 */
interface PostMessage {
    /** 消息角色 */
    role: 'user' | 'assistant' | 'system' | 'tool'
    /** 消息内容 */
    content: string | Array<TextItem | ImageItem>
    /** assistant 消息携带的函数调用（OpenAI 兼容 tool_calls） */
    tool_calls?: ToolCall[]
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

/** OpenAI 兼容函数工具定义（agent.chat 的 opts.tools 项） */
interface ChatTool {
    type: 'function'
    /** 函数定义体 */
    function: {
        /** 函数名 */
        name: string
        /** 函数描述 */
        description?: string
        /** JSON Schema 形式的参数定义 */
        parameters?: Record<string, unknown>
    }
}

/** OpenAI 兼容模型函数调用（agent.chat 返回的 tool_calls 项） */
interface ToolCall {
    /** 调用 id（回传 tool 消息时填写 tool_call_id） */
    id: string
    type: 'function'
    /** 调用详情 */
    function: {
        /** 函数名 */
        name: string
        /** JSON 字符串形式的实参 */
        arguments: string
    }
}

/** 工具结果消息（role='tool'，回传某次函数调用的执行结果） */
interface ToolMessage {
    role: 'tool'
    /** 对应 ToolCall.id */
    tool_call_id: string
    /** 执行结果文本 */
    content: string
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

/** 默认事件主题常量（单源派生自 EventContract，避免双份维护；仅类型层，运行时直接用字符串） */
declare const defaultEventTopic: { readonly [K in EventTopic]: K }

/** 月华事件回传：`{ return }` 即把业务结果交回月华 */
interface ReturnResult<R> {
    /** 业务结果，格式见 EventContract 对应事件 */
    return: R
}

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
    error?: string
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

/** 数据库事务作用域对象（transaction 回调入参，操作落在当前事务内） */
interface DatabaseTx {
    /** 事务内 SELECT，返回 { success, rows } */
    query(sql: string, params?: unknown[]): { success: boolean; rows: Array<Record<string, unknown>>; error?: string }
    /** 事务内写语句，返回 { success, rows_affected } */
    exec(sql: string, params?: unknown[]): { success: boolean; rows_affected: number; error?: string }
}

/** 命名空间数据库作用域（独立 <name>.db 文件，真隔离） */
interface DatabaseScope {
    /** SELECT 查询，返回 { success, rows } */
    query(sql: string, params?: unknown[]): { success: boolean; rows: Array<Record<string, unknown>>; error?: string }
    /** 写语句，返回 { success, rows_affected } */
    exec(sql: string, params?: unknown[]): { success: boolean; rows_affected: number; error?: string }
    /** 同步事务，语义同 database.transaction */
    transaction(fn: (tx: DatabaseTx) => unknown): { success: boolean; result?: unknown; error?: string }
    /** 按名称迁移，语义同 database.migrate（记录在本库 _migrations 表） */
    migrate(name: string, upSql: string): { success: boolean; applied?: boolean; error?: string }
}
/**
 * 跨包调用结果（判别联合：成功携带 result，失败携带 error）
 */
type CallResult<R> = { success: true; result: R } | { success: false; error: string }
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
     * @returns { success, results: 相似度降序的命中列表 }，每项含 content 与 similarity
     */
    search(v: { query: string; limit?: number }): { success: boolean; results?: Array<MemoryHit>; error?: string }
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
/** SQLite 数据库（需要权限: allow-database）。query/exec/transaction/migrate 操作共享库 knowledge.db */
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
    exec(sql: string, params?: unknown[]): { success: boolean; rows_affected: number; error?: string }
    /**
     * 同步事务：回调内执行 query/exec，全部成功则提交
     * 
     * @param fn 事务回调，接收 { query, exec }（绑定到事务）；正常返回即提交，
     *          抛异常或返回 Promise 则回滚（事务内禁异步）
     * 
     * @returns { success, result: fn返回值 } 或 { success:false, error }
     */
    transaction(fn: (tx: DatabaseTx) => unknown): { success: boolean; result?: unknown; error?: string }
    /**
     * 按名称迁移：_migrations 表未记录则在事务内执行 upSql（支持多语句，禁含 BEGIN/COMMIT）并记录
     * 
     * @param name 迁移名（唯一标识）
     * @param upSql 建表/改表 SQL
     * 
     * @returns { success, applied }（applied=false 表示此前已应用，本次跳过）
     */
    migrate(name: string, upSql: string): { success: boolean; applied?: boolean; error?: string }
    /**
     * 打开命名空间作用域：对应独立 <name>.db 文件（名称仅允许字母数字-_）
     * 
     * @param name 命名空间名
     * 
     * @returns 作用域对象 { query, exec, transaction, migrate }；名称非法时抛出
     */
    namespace(name: string): DatabaseScope
}
/** 插件配置（需要权限: allow-config） */
export const config: {
    /**
     * 读取配置
     * 
     * @returns { success, config? }；config 为 config.yaml 引擎启动时解析注入的对象，无配置时省略
     */
    read(): { success: boolean; config?: Record<string, unknown> }
    /**
     * 编写配置
     * 
     * @param v 要写入的配置对象；更新内存并同步写回 config.yaml
     */
    write(v: Record<string, unknown>): void
}
/** 编解码工具（需要权限: allow-certificate）。可失败方法统一 Result 风格，不抛异常 */
export const encoding: {
    /** base64 编码；data 为 UTF-8 字符串 */
    base64Encode(data: any): string
    /** base64 解码；返回 { success, text?, error? } */
    base64Decode(s: string): { success: boolean; text?: string; error?: string }
    /** URL 编码（查询转义） */
    urlEncode(s: string): string
    /** URL 解码；返回 { success, text?, error? } */
    urlDecode(s: string): { success: boolean; text?: string; error?: string }
    /**
     * 加密
     * 
     * @param key 加解密密钥字符串
     * 
     * @param content 要加密的明文
     * 
     * @returns { success, text: 密文, error? }
     */
    lunarEncoder(key: string, content: string): { success: boolean; text?: string; error?: string }
    /**
     * 解密
     * 
     * @param key 加解密密钥字符串
     * 
     * @param cipher 要解密的密文
     * 
     * @returns { success, text: 原文, error? }
     */
    lunarDecoder(key: string, cipher: string): { success: boolean; text?: string; error?: string }
}
/** 哈希工具（需要权限: allow-certificate）。签名类方法统一 Result 风格 */
export const hash: {
    /**
     * 生成 JWT 令牌
     * 
     * @param claims 令牌负载对象
     * @param secret HS256 时为其 HMAC 密钥；EdDSA 时为 Ed25519 PEM(PKCS8) 私钥
     * @param algorithm 算法，默认 HS256；可选 HS256 / EdDSA(Ed25519) / none
     * @param kid 可选；非空时写入 JWT 头（多密钥按 Key ID 选择签名密钥）
     * 
     * @returns { success, text: JWT 字符串, error? }
     */
    signJWT(claims: Record<string, unknown>, secret: string, algorithm?: string, kid?: string): { success: boolean; text?: string; error?: string }
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
     * @returns { success, text: base64url 签名, error? }
     */
    ed25519Sign(privKeyPemOrSeed: string, data: string): { success: boolean; text?: string; error?: string }
    /**
     * 生成 EdDSA (Ed25519) JWT
     * 
     * @param claims 令牌负载对象
     * @param privKeyPemOrSeed Ed25519 私钥（PEM(PKCS8) 或 32/64 字节原始字节）
     * @param kid 可选；非空时写入 JWT 头
     * 
     * @returns { success, text: JWT 字符串, error? }
     */
    generateJWT(claims: Record<string, unknown>, privKeyPemOrSeed: string, kid?: string): { success: boolean; text?: string; error?: string }
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
    /**
     * 缩放图片并按需重编码（宽超限时可选等比缩放 / 指定精确宽高）
     *
     * @param path 相对插件数据目录的路径，或 data:image/...;base64 内联数据
     * @param opts 可选参数：
     *             max_dim 等比缩放到不超过该边长（默认 0=不缩放）
     *             width|height 精确目标宽高（两者需同时给出，与 max_dim 互斥）
     *             format 输出编码格式（jpeg/png/webp，默认保留源格式；PNG 保透明）
     *             quality jpeg/webp 质量（1-100，默认 90）
     *
     * @returns 成功返回 { success:true, text: data URI, width, height, format }；失败返回 failure
     */
    resize(path: string, opts?: { max_dim?: number; width?: number; height?: number; format?: 'jpeg' | 'png' | 'webp'; quality?: number }): { success: boolean; text?: string; width?: number; height?: number; format?: string; error?: string }
    /**
     * 仅重编码图片（编码格式化，不改尺寸）
     *
     * @param path 相对插件数据目录的路径，或 data:image/...;base64 内联数据
     * @param format 目标编码格式（jpeg/png/webp）
     * @param opts 可选参数 { quality? }；jpeg/webp 质量（1-100，默认 90）
     *
     * @returns 成功返回 { success:true, text: data URI, width, height, format }；失败返回 failure
     */
    convert(path: string, format: 'jpeg' | 'png' | 'webp', opts?: { quality?: number }): { success: boolean; text?: string; width?: number; height?: number; format?: string; error?: string }
}
/** 视频抽帧（需要权限: allow-file；http(s)/data URI 源运行时另需 allow-network） */
export const video: {
    /**
     * 从视频抽取多帧并编码为图片
     *
     * @param source 相对插件数据目录的视频路径，或 http(s) URL，或 data:video/...;base64 URI
     * @param opts 可选参数（采样方式三选一，互斥优先级：times > count > fps）：
     *             times 显式时间点（秒）数组，精确抽取
     *             count 等距采样帧数（1-60，默认不启用）
     *             fps   均匀抽帧频率（默认 5），超 60 帧自动等距抽样
     *             dedup 相邻帧相似度去重（默认 true）
     *             max_dim 帧等比缩放的边长上限（默认 640，超出则缩放）
     *             format 帧编码格式（jpeg/png/webp，默认 jpeg）
     *             quality jpeg/webp 质量（1-100，默认 85）
     *
     * @returns 成功返回 { success:true, frames:[{ data:dataURI, timestamp, width, height, format, index }], count }；失败返回 failure
     */
    frames(source: string, opts?: { times?: number[]; count?: number; fps?: number; dedup?: boolean; max_dim?: number; format?: 'jpeg' | 'png' | 'webp'; quality?: number }): { success: boolean; frames?: Array<{ data: string; timestamp: string; width: number; height: number; format: string; index: number }>; count?: number; skipped?: number; error?: string }
}
/** 模型能力（需要权限: allow-agent） */
export const agent: {
    /**
     * 调用 v1/chat/completions 生成回复文本
     * 
     * @param messages OpenAI 消息数组（PostMessage / ToolMessage）
     * @param opts 可选参数 { temperature?, max_tokens?, system?, tools? }
     *             system  为系统提示词字符串（自动前置为一条 system 消息）
     *             tools   为给模型提供的函数工具定义（OpenAI 兼容 ChatTool[]）
     * 
     * @returns 成功返回 { success:true, text: 回复内容, tool_calls?: ToolCall[] }；
     *          模型走函数调用时 content 为空，tool_calls 原样透传（供 AtoA 接头）；失败返回 failure
     */
    chat(messages: Array<PostMessage | ToolMessage>, opts?: { temperature?: number; max_tokens?: number; system?: string; tools?: ChatTool[] }): { success: boolean; text?: string; tool_calls?: ToolCall[]; error?: string }
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
     * @returns { success, text: 执行结果文本, error? }；目标包不存在/拒绝响应时 success=false
     */
    synergy: (pkgId: string, text: string) => { success: boolean; text?: string; error?: string }
    /**
     * 调用进程内 Web-LTP 网络搜索，返回自然语言搜索报告
     *（双权限：绑定需 allow-agent，执行需 allow-network）
     *
     * @param instruction 自然语言搜索指令（可含「前N页/个」等数量要求）
     *
     * @returns { success, text: 搜索报告, error? }；无 allow-network 时 success=false
     */
    search(instruction: string): { success: boolean; text?: string; error?: string }
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
    subscribe(topic: string, handler: (e: any[]) => HandlerResult, priority?: number): number
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
 * @param args 传给该函数的实参数组（按 exportFunction 实现的形参顺序展开）
 * 
 * @typeParam A 实参数组元组类型
 * 
 * @typeParam R 目标函数返回值类型
 * 
 * @returns { success:true, result } 或 { success:false, error }（目标包不存在或未导出）
 */
export const callFunction: <A extends unknown[], R = unknown>(pkgId: string, name: string, args: A) => CallResult<R>
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
     * @returns { success, status, body }；传输失败时 success=false 含 error
     */
    get(url: string, headers?: Record<string, string>): { success: boolean; status: number; body: string; error?: string }
    /**
     * 同步 POST；阻塞直到返回
     * 
     * @param url 请求地址
     * @param body 请求体（JSON 字符串）
     * @param headers 可选请求头
     * 
     * @returns { success, status, body }；传输失败时 success=false 含 error
     */
    post(url: string, body: string, headers?: Record<string, string>): { success: boolean; status: number; body: string; error?: string }
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
    /** 枚举全部任务状态，返回 { success, tasks } */
    list(): { success: boolean; tasks?: Array<{ taskId: number; status: string; progress?: unknown }>; error?: string }
}
/**
 * 发起网络请求（同步 fetch，阻塞直到返回；需要权限: allow-network）
 * 
 * @param input 请求 URL
 * 
 * @param init 请求配置；method 为请求方法，headers 为请求头，body 为请求体
 * 
 * @returns 统一 Result 信封 FetchResult（body 为 JSON 时自动解析为对象，否则为文本）
 */
declare function fetch(input: string, init?: FetchInit): FetchResult
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