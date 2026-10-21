import type { FileListItem, ProxyFetchConfig } from './config/config';
import type { MediaSegment, MediaFrameGroup, ResizeImageResult, ResizeImageResults, GenerateImageParams, GenerateImageResult } from './config/image';
import type { MultimodalMessage } from './config/model';
import type { TTSParams } from './config/tool';
import type { ScreenshotParams } from './config/screenshot';

declare global {
    /**
     * 在磁盘中保存文件
     * 
     * @param {string} fileName 文件名
     * 
     * @param {boolean} overwrite 是否覆盖已存在的文件
     * 
     * @param {*} fileData 文件数据
     * 
     * @returns {[string, string, Error | null]} 包含保存结果的元组，[保存的文件名, 保存的文件路径, 错误信息]
     */
    function saveFile(fileName: string, overwrite: boolean, fileData: Blob | File | FormData | string | Uint8Array): [string, string, Error | null];
    /**
     * 计算数据的 SHA-256 哈希，截取前 16 位十六进制（与前端文件哈希命名约定一致）
     *
     * 入参与 upscaleImage 一致：字节数据（Uint8Array 等）直接使用，
     * 地址类（HTTP(S) URL、data URI、本地路径）由 Go 侧下载/解码后计算
     *
     * @param {Uint8Array | string} data 数据内容或来源地址
     *
     * @returns {[string, Error | null]} 包含哈希结果的元组，[16位十六进制哈希, 错误信息]
     */
    function hashBytes(data: Uint8Array | string): [string, Error | null];
    /**
     * 从磁盘中读取文件
     * 
     * @param {string} filePath 文件路径
     * 
     * @returns {[string, number, string, Error | null]} 包含读取结果的元组，[文件内容(base64编码), 文件大小, MIME类型, 错误信息]
     */
    function readFile(filePath: string): [string, number, string, Error | null];
    /**
     * 解码 base64 为字节数组（二进制专用）
     *
     * `atob` 返回 Go 字符串，goja 按 UTF-8 转 JS 字符串会把 PNG/JPEG 等二进制中的
     * 非法 UTF-8 字节替换成 U+FFFD，落盘后文件损坏；图片、音频等二进制内容必须用本函数。
     *
     * @param {string} data base64 文本（可带 data URI 前缀由调用方自行剥离）
     *
     * @returns {[ArrayBuffer, Error | null]} 包含解码结果的元组，[字节缓冲区, 错误信息]
     */
    function base64Decode(data: string): [ArrayBuffer, Error | null];
    /**
     * 以 UTF-8 文本读取文件（不经过 base64，适合直接解析 JSON/文本缓存）
     *
     * @param {string} filePath 文件路径
     *
     * @returns {[string, Error | null]} 包含读取结果的元组，[文件文本, 错误信息]
     */
    function readTextFile(filePath: string): [string, Error | null];
    /**
     * 获取指定目录下的所有文件列表
     * 
     * @param {string} path 目录路径
     * 
     * @returns {[FileListItem[], Error | null]} 包含文件列表的元组，[文件列表, 错误信息]
     */
    function fileList(path: string): [FileListItem[], Error | null];
    /**
     * 查看嵌入式文件系统中的文件内容
     * 
     * @param {string} filePath 文件路径
     * 
     * @returns {[string, Error | null]} 包含文件内容的元组，[文件内容, 错误信息]
     */
    function fileView(filePath: string): [string, Error | null];
    /**
     * 读取指定知识库表的全部条目（JSON 落盘格式 [key,text][]）
     *
     * 一个表对应 local_data/database/knowledge 下的一个 JSON 文件，数据按键去重。
     *
     * @param {string} table 表名（即 JSON 文件名，不含扩展名）
     *
     * @returns {[Array<[string, string]>, Error | null]} 包含条目数组的元组，[[key, text],...]
     */
    function knowledgeLoad(table: string): [Array<[string, string]>, Error | null];
    /**
     * 将指定知识库表的条目数组写回对应 JSON 文件（覆写，自动建目录）
     *
     * @param {string} table 表名（即 JSON 文件名，不含扩展名）
     *
     * @param {Array<[string, string]>} entries [key, text] 条目数组
     *
     * @returns {[boolean, Error | null]} 包含写入结果的元组，[是否成功, 错误信息]
     */
    function knowledgeSave(table: string, entries: Array<[string, string]>): [boolean, Error | null];
    /**
     * 获取系统访问URL
     * 
     * @returns {[string, Error | null]} 包含系统URL的元组，[系统URL, 错误信息]
     */
    function url(): [string, Error | null];
    /**
     * 获取当前地址信息
     * 
     * @returns {[string[], Error | null]} 包含地址信息的元组，[省份, 城市, 错误信息]
     */
    function address(): [string[], Error | null];
    /**
     * 网络请求代理函数
     * 
     * @param {ProxyFetchConfig} config 请求配置对象
     * 
     * @returns {[any, Error | null]} 包含响应结果的元组，[响应结果, 错误信息]
     */
    function syncFetch(config: ProxyFetchConfig): [any, Error | null];
    /**
     * 将视频本地化并写入 llama-server 媒体目录，供感知者以 file:// 引用观看
     *
     * 输入支持 HTTP(S) URL、data:video/xxx;base64 URI 与本地文件路径。
     * 超过 80 秒的视频自动按 80 秒分段（长边超限时重编码归一化，否则流复制切分），切片一律不含音轨。
     *
     * @param {string} inputFile 视频地址（URL / data URI / 本地路径）
     *
     * @returns {[MediaSegment[], Error | null]} 包含媒体片段列表的元组，[{file, start, end}]
     */
    function videoMedia(inputFile: string): [MediaSegment[], Error | null];
    /**
     * 将视频抽帧为图片序列帧（序列帧解读模式）
     *
     * 供不支持 llama-server「媒体目录 + file:// 引用」机制的架构（如云端 OpenAI 兼容 API）使用：
     * 在本地用 FFmpeg 抽帧并编码为 JPEG data URI，直接以多个 image_url 提交，无需服务端解码视频。
     * 分组与 file 模式一致（每 80 秒一组），组内按 1fps 采样，视觉 token 量级与 file 模式相当。
     *
     * @param {string} inputFile 视频地址（URL / data URI / 本地路径）
     *
     * @returns {[MediaFrameGroup[], Error | null]} 包含分组采样帧列表的元组，[{start, end, frames:[{image, time}]}]
     */
    function videoFrames(inputFile: string): [MediaFrameGroup[], Error | null];
    /**
     * 将音频本地化并转换为 16kHz 单声道 WAV
     *
     * 输入支持 HTTP(S) URL、data:audio/xxx;base64 URI 与本地文件路径
     *
     * @param {string} inputFile 音频地址（URL / data URI / 本地路径）
     *
     * @returns {[string, Error | null]} 包含转换结果的元组，[WAV音频的base64编码, 错误信息]
     */
    function audioWav(inputFile: string): [string, Error | null];
    /**
     * 判断图片是否为动态图（多帧 GIF / APNG / 动态 WebP）
     *
     * 入参支持字节数据与来源地址（HTTP(S) URL、data:image URI、本地文件路径），
     * 传入地址时由 Go 侧下载/解码后判定，无需 TypeScript 先取回字节
     *
     * 注意：宿主**不提供** resizeImage —— 思维链不允许自己缩放图片。
     * 图片进入模型上下文的唯一形态是视觉读取管线产物（/file/miniature/ 或 visionRead），
     * 缩放与尺寸约束在 Go 侧统一完成（见 docs/adr/0008）。
     *
     * @param {Blob | File | string | Uint8Array} imgData 图片数据或地址
     *
     * @returns {[boolean, Error | null]} 是否动态图与错误信息
     */
    function isAnimatedImage(imgData: Blob | File | string | Uint8Array): [boolean, Error | null];
    /**
     * 视觉读取：按「约束尺寸 + 句柄叠印」管线生成思维链消费副本，直接返回 data URI
     *
     * 与 /file/miniature 端点共用同一变换实现（缩放到 vision_max_side、
     * 右上角叠印句柄标签、按内容哈希盘上缓存、动图原样直通）。原图不被修改。
     *
     * 这是 base64 形态的视觉副本入口：把同一 VisionCopy 管线产出的副本交给无法回取
     * 本机 URL 的云端模型；产物仍是 ≤ vision_max_side 的副本，不是原图。
     * 不提供 resizeImage，也不要 readResource 再去拼 data URI
     * （图片资源的 readResource 已改为回视觉副本，读不到原图）。
     *
     * @param {string} relPath 源图相对 LocalDir 路径（multimedia/ 下）
     * @param {string} [tag] 叠印句柄（如 @I.003；缺省时按路径反查资源目录）
     * @param {number} [side] 副本最大边长（缺省取配置 vision_max_side）
     *
     * @returns {[string, Error | null, number, number]} [data URI, 错误信息, 副本宽, 副本高]
     */
    function visionRead(relPath: string, tag?: string, side?: number): [string, Error | null, number, number];
    /**
     * 将动态图直接解码为图片帧序列
     *
     * 不再转码为慢放视频：长边约束到 640，按内容感知规则（循环裁剪、运动加密、去重）摘取至多 64 帧，
     * 编码为 JPEG data URI 交给模型理解。
     *
     * @param {Blob | File | string | Uint8Array} imgData 动态图数据或地址
     *
     * @returns {[MediaFrameGroup[], Error | null]} 包含分组帧列表的元组，[{start, end, frames:[{image, time}]}]
     */
    function animatedImageToFrames(imgData: Blob | File | string | Uint8Array): [MediaFrameGroup[], Error | null];
    /**
     * 生成图片
     * 
     * @param {GenerateImageParams} params 图片生成参数
     * 
     * @returns {[GenerateImageResult, Error | null]} 包含生成图片结果的元组，[生成图片结果, 错误信息]
     */
    function generateImage(params: GenerateImageParams): [GenerateImageResult, Error | null];
    /**
     * 对已有图片执行 RealESRGAN 超分辨率放大（4x）
     *
     * 输入支持字节数据与来源地址（HTTP(S) URL、data:image URI、本地路径），
     * 地址类入参由 Go 侧下载/解码后经 sd-cli -M upscale 管线放大
     *
     * @param {Blob | File | string | Uint8Array} imgData 图片数据或地址
     *
     * @returns {[GenerateImageResult, Error | null]} 包含超分结果的元组，[结果(base64/尺寸), 错误信息]
     */
    function upscaleImage(imgData: Blob | File | string | Uint8Array): [GenerateImageResult, Error | null];
    /**
     * 查询当前绘图模型方案（状态记录在 Go 层，与图像工坊前端共享）
     *
     * @returns {[string, Error | null]} [方案键名（如 turbo / quality；未配置方案时为空串）, 错误信息]
     */
    function getDiffusionScheme(): [string, Error | null];
    /**
     * 切换绘图模型方案（状态记录在 Go 层，与图像工坊前端共享）
     *
     * 只影响此后创建的生成任务；已入队任务按入队时的方案快照执行。
     *
     * @param {string} scheme 方案键名（如 turbo / quality）
     *
     * @returns {[string, Error | null]} [切换后的方案键名, 错误信息]
     */
    function setDiffusionScheme(scheme: string): [string, Error | null];
    /**
     * 拉取上下文
     * 
     * @returns {MultimodalMessage[]} 上下文消息列表
     */
    function pullContext(): MultimodalMessage[];
    /**
     * 未读消息条数（只读，不消费队列）
     *
     * 生活链调度用：必须查宿主队列而不是 `pullContext()`——后者会把消息取走，
     * 等于替对话链消费了本轮消息。
     *
     * @returns 宿主统一未读队列当前长度
     */
    function unreadCount(): number;
    /**
     * 推送上下文
     * 
     * @param {string} msgType 消息类型（text=文本 / music=乐谱 / action=动作）
     * 
     * @param {string} data 消息内容
     *
     * @param {string} audio 语音数据（base64 WAV；空字符串表示不合成）
     *
     * @param {boolean} [auto=false] 是否为快速应答（QQ 桥接转发但不占用应答份额）
     */
    function pushContext(msgType: string, data: string, audio: string, auto?: boolean): boolean;
    /**
     * 推送图片
     * 
     * @param {string[]} imageData 图片 URL 列表（/file/read/... 或 http(s)://）
     * @param {boolean} [isSticker=false] 是否为表情包（前端据此决定角标显示）
     *
     * @returns {string[]} 登记句柄并做哈希唯一性归一后的最终 URL 列表
     */
    function pushImage(imageData: string[], isSticker?: boolean): string[];
    /**
     * 获取缓存的智能体3D位置（由前端遥测数据更新）
     *
     * @returns {{ x: number, y: number, z: number }} 智能体当前位置
     */
    function getAgentPosition(): { x: number; y: number; z: number };
    /**
     * 向引擎直接发送命令（绕过前端转发，Agent → StudioHub → 引擎）
     *
     * @param {string} type 消息类型（如 'action', 'movement', 'mouse_tracking'）
     * @param {string} payload JSON 字符串格式的 payload 数据
     * @returns {boolean} 是否成功
     */
    function sendToEngine(type: string, payload: string): boolean;
    /**
     * 查询引擎当前可用的动作列表
     * 
     * 返回引擎 ACTION_DEFINITIONS 中已注册的动作定义，
     * 数据来源于引擎广播的 animation_list 消息中携带的 actionDefinitions 字段
     *
     * @returns {string} JSON 字符串，格式为 {"actions":[{"name":"荡秋千","mouseTracking":true},...],"updated_at":...}
     *                   引擎未就绪或缓存为空时返回 "{}"
     */
    function getAvailableActions(): string;
    /**
     * 在引擎 animation_list 缓存中为动作文本匹配最合适的动作名
     *
     * 匹配在 Go 侧完成：动作词表只存在于 AnimationListCache，不写记忆库。
     * 语义匹配优先；嵌入不可用时退回字面包含匹配。
     *
     * @param {string} query 动作/情绪文本（如“轻轻晃了晃脑袋”）
     * @returns {string} 动作名；没有可匹配动作时返回空串
     */
    function matchAction(query: string): string;
    /** 记忆库条目（查询/列举/按 ID 读取共用） */
    interface MemoryEntry {
        /** 条目 ID */
        id: string;
        /** 记忆类型：text / image / audio */
        type: string;
        /** 消息角色（图片记忆通常为 image） */
        role: string;
        /** 内容所属分组集合名 */
        collection?: string;
        /** 写入来源标记（插件 / 内部调用等） */
        source?: string;
        /** 文本内容（音频记忆为描述） */
        content?: string;
        /** 媒体读取地址 */
        url?: string;
        /** 旧版图片内联数据（仅历史数据兼容） */
        base64?: string;
        /** 内容向量余弦相似度（列举/按 ID 读取为 0） */
        similarity: number;
        /** 入库 Unix 秒时间戳（旧数据无此字段） */
        timestamp?: number;
    }
    /**
     * 初始化记忆库实例：一次性加载全部集合，补齐默认分组与集合名向量
     *
     * 模型配置与月华共用 lunar_config.json 的 agent 分组；不再按集合启动。
     *
     * @returns {[boolean, Error | null]} 包含初始化结果的元组，[是否成功, 错误信息]
     */
    function memoryInit(): [boolean, Error | null];
    /**
     * 列出记忆库全部分组集合名（管理/调试用途，不参与检索参数）
     *
     * @returns {[string[], Error | null]} 包含集合清单的元组，[集合名数组, 错误信息]
     */
    function memoryListCollections(): [string[], Error | null];
    /**
     * 添加文本记忆（内容自动归入最相关的分组集合）
     *
     * @param {string} role 消息角色 (user/assistant/system/tool)
     *
     * @param {string} content 消息文本内容
     *
     * @returns {[string, Error | null]} 包含操作结果的元组，[文档ID, 错误信息]
     */
    function memoryAdd(role: string, content: string): [string, Error | null];
    /**
     * 添加文本记忆，携带调用方提供的显式标签与来源标记
     *
     * @param {string} role 消息角色 (user/assistant/system/tool)
     *
     * @param {string} content 消息文本内容
     *
     * @param {string[]} tags 显式标签数组（以标签文本计算嵌入与归组）
     *
     * @param {string} [source] 来源标记（插件 / 内部调用等），仅作溯源信息
     *
     * @returns {[string, Error | null]} 包含操作结果的元组，[文档ID, 错误信息]
     */
    function memoryAddWithTags(role: string, content: string, tags: string[], source?: string): [string, Error | null];
    /**
     * 在全部集合中检索指定类型的相关记忆
     *
     * 默认快速检索：先查与查询含义最匹配的集合（集合名向量），命中后再查第 2~5
     * 匹配的集合；首集合未命中则回退扫描其余全部集合。
     *
     * @param {string} queryText 查询文本
     *
     * @param {string} mediaType 返回类型：text / image / audio
     *
     * @param {number} topK 返回的最相关结果数量
     *
     * @param {boolean} [forceGlobal] 强制全局检索（跳过集合名路由，扫描全部集合）
     *
     * @returns {[MemoryEntry[], Error | null]} 包含查询结果的元组，结果按相似度降序排列
     */
    function memoryQuery(queryText: string, mediaType: string, topK: number, forceGlobal?: boolean): [MemoryEntry[], Error | null];
    /**
     * 在全部集合中按关键词 / 正则检索指定类型的记忆（不调用嵌入服务）
     *
     * @param {string} mediaType 返回类型：text / image / audio
     *
     * @param {string[]} terms 关键词数组（全部命中）或正则片段
     *
     * @param {boolean} useRegex 是否按正则匹配（否则为大小写不敏感的子串 AND 匹配）
     *
     * @param {number} limit 返回条数上限
     *
     * @returns {[MemoryEntry[], Error | null]} 命中结果（按入库时间倒序；similarity 固定为 0）与错误信息
     */
    function memoryKeywordSearch(mediaType: string, terms: string[], useRegex: boolean, limit: number): [MemoryEntry[], Error | null];
    /**
     * 按 ID 返回一条记忆（跨全部集合查找）
     *
     * @param {string} id 记忆条目 ID
     *
     * @returns {[MemoryEntry | null, Error | null]} 包含查询结果的元组，未找到时结果为 null
     */
    function memoryGet(id: string): [MemoryEntry | null, Error | null];
    /**
     * 逐条列举记忆条目（跨全部集合）
     *
     * @param {string} [mediaType] 返回类型；空或省略表示全部类型
     *
     * @param {number} [limit] 返回条数上限；<=0 表示返回全部
     *
     * @param {number} [offset] 跳过条数
     *
     * @returns {[MemoryEntry[], Error | null]} 包含条目列表的元组
     */
    function memoryList(mediaType?: string, limit?: number, offset?: number): [MemoryEntry[], Error | null];
    /**
     * 按 ID 删除记忆（跨全部集合查找）
     *
     * @param {string} id 要删除的记忆 ID
     *
     * @returns {[boolean, Error | null]} 包含操作结果的元组，[是否成功, 错误信息]
     */
    function memoryDelete(id: string): [boolean, Error | null];
    /**
     * 添加图片记忆（LLM 自动生成标签并归组）
     *
     * @param {string} base64Image 图片 base64 数据（data:image/...;base64,...）
     *
     * @param {string} [orientation] 图片识别取向标识（auto/emotion/text/color/appearance/species/posture/custom），默认 auto
     *
     * @param {string} [custom] 自定义识别取向参考文本（仅 orientation 为 custom 时使用）
     *
     * @returns {[string, Error | null]} 包含操作结果的元组，[图片文档ID, 错误信息]
     */
    function memoryAddImage(base64Image: string, orientation?: string, custom?: string): [string, Error | null];
    /**
     * 添加音频记忆（按描述文本归组）
     *
     * @param {string} base64Audio 音频 base64 数据（data:audio/...;base64,...）
     *
     * @param {string} [description] 音频描述（用于嵌入与归组）
     *
     * @returns {[string, Error | null]} 包含操作结果的元组，[音频文档ID, 错误信息]
     */
    function memoryAddAudio(base64Audio: string, description?: string): [string, Error | null];
    /**
     * 文本转语音生成
     * 
     * 接收文本输入参数，调用TTS合成引擎生成音频数据，进行Base64编码后通过WebSocket广播至所有已连接的客户端
     * 
     * @param {string} text 要合成的文本内容
     * 
     * @param {TTSParams} [params] 可选的合成参数配置
     * 
     * @returns {[string, Error | null]} 包含合成结果的元组，[音频数据(Base64编码的WAV), 错误信息]
     */
    function tts(text: string, params?: TTSParams): [string, Error | null];
    /**
     * 执行屏幕截图（内部已集成图片压缩缩放处理）
     * 
     * Go 层统一完成截图捕获 + 图片压缩缩放 + base64 编码，
     * 返回的 base64 字段格式为 "data:image/[format];base64,[data]"
     *
     * @param {ScreenshotParams} [params] 截图参数对象，字段对齐 CaptureRequest
     *   mode / display_index / offset_x / offset_y / width / height /
     *   region_x / region_y / region_w / region_h / format / quality / scale
     *
     * @returns {[ResizeImageResults, Error | null]} 包含处理结果的元组，[结果对象数组(base64/format/width/height), 错误信息]
     */
    function screenshotCapture(params?: ScreenshotParams): [ResizeImageResults, Error | null];
    /**
     * 获取所有显示器信息
     *
     * @returns {[Array<{index: number, x: number, y: number, width: number, height: number}>, Error | null]} 包含显示器信息的元组，[显示器列表, 错误信息]
     */
    function screenshotGetDisplays(): [Array<{ index: number; x: number; y: number; width: number; height: number }>, Error | null];
    /**
     * 获取琉璃（远程 LTPX）聚合工具链
     * 启动与每次工具调用后调用：进程内直读工具链并写入内部缓存，返回聚合工具与可调用应用清单
     *
     * @returns {string} JSON 字符串：{ tool?: {name, description, parameters}, tools?: Array<{name, description}> }（未就绪时无 tool）
     */
    function getLTPXRemoteStatus(): string;
    /**
     * 转发工具调用到琉璃（进程内 Broker）
     *
     * @param {string} toolName 聚合工具名（use_the_program）
     * @param {string} argumentsJSON 工具参数 JSON 字符串（{tool, instruction}）
     *
     * @returns {Promise<string>} 异步返回文本结果；失败时 resolve 以「【工具调用失败】」开头的错误文本
     */
    function callLTPXRemoteTool(toolName: string, argumentsJSON: string): Promise<string>;
    /**
     * 同步推送事件并接收 LTP9 插件处理结果（事件触发点调用）
     *
     * 把「xx事件发生前」的原始负载推送到 LTP9 引擎派发面，同步阻塞等待插件处理完成。
     * 无插件订阅 / 插件未改写 / 未回传业务结果时 returned=false
     * （调用方应回退本地默认处理）。
     *
     * @param {string} topic 事件主题（如 message_received_before）
     * @param {string} payloadJSON 事件负载 JSON 字符串（可为 "{}"）
     *
     * @returns {string} JSON 字符串：{ modified: boolean, data: any, returned: boolean, return: any }
     */
    function ltpInteractEvent(topic: string, payloadJSON: string): string;
    /**
     * 导出智能体上下文（TUI「数据导出」调用；由 Go 侧经运行时触发本函数）
     *
     * 双向桥：宿主请求 → 本函数序列化上下文为 JSON 文本 → 由宿主落盘为 .json 文件。
     * 之所以由 JS 侧产出内容：智能体上下文（messages）只存在于本运行时内。
     *
     * @param {string} argsJSON 请求参数 JSON，形如 {"agent":"painter","includeImages":false}；
     *                          agent 取 'lunar'（月华主上下文/未读队列）或子智能体名
     *                          （dialogue/painter/musician/perceiver/description）
     *
     * @returns {[string, Error | null]} 包含导出结果的元组，[上下文 JSON 文本, 错误信息]
     */
    function dumpForJS(argsJSON: string): [string, Error | null];
    /**
     * 将调试内容写入本地文件（覆写模式）
     * 用于各子智能体导出上下文快照
     *
     * @param {string} filePath 输出文件的绝对路径
     * @param {string} content 要写入的字符串内容
     *
     * @returns {[string, Error | null]} 包含导出结果的元组，[文件路径, 错误信息]
     */
    function saveDebugFile(filePath: string, content: string): [string, Error | null];
    /**
     * 通知宿主「本轮模型调用成功」（清零连续失败计数，取消待执行的重载）
     *
     * 宿主未注册该全局时（离线脚本 / 旧版本）调用会被忽略，不影响主流程。
     */
    function notifyAgentIdle(): void;
    /**
     * 通知宿主「本轮应答失败」
     *
     * 宿主按连续失败次数决定是否自动重载月华引擎（goja lunar_loop）；
     * 单次失败只记日志，连续失败才动手，避免一次网络抖动就重置上下文。
     *
     * @param {string} reason 可读的失败原因（已由 JS 侧从 error 中提取）
     */
    function notifyAgentFailure(reason: string): void;
    /**
     * 取出宿主为「引擎重载」暂存的上下文快照（一次性：取走即清空）
     *
     * 只在重载后的恢复流程里调用；没有待恢复快照时返回空串。
     *
     * @returns {string} 快照 JSON 文本（见 agentSystem.snapshotContext 的产物）
     */
    function takeEngineSnapshot(): string;

    // =========================================================================
    // 统一资源归档层（句柄 / 资源目录 / 内容仓库，见 docs/adr/0001-统一资源寻址与三层存储.md）
    // =========================================================================

    /** 资源类型码：D 文档 / I 图片 / V 视频 / A 音频 / T 表格 */
    type ResourceKindCode = 'D' | 'I' | 'V' | 'A' | 'T';

    /** 统一入库入参（五类来源归一：path / url / data / bytes / workspace） */
    interface ResourceAdmitInput {
        /** 来源类型：path（LocalDir 内文件 / 本机服务地址）/ url（远程）/ data（data URI）/ bytes（字节）/ workspace（工作区文件，只读收录） */
        sourceKind: 'path' | 'url' | 'data' | 'bytes' | 'workspace';
        /** 来源值（路径 / 地址 / data URI） */
        source?: string;
        /** 原始字节（sourceKind 为 bytes 时使用，或由 source 解析；data 为 base64 字符串时自动解码） */
        data?: string | Uint8Array;
        /** 建议文件名（可空：path 来源自动取基名） */
        name?: string;
        /** 类型码建议值（可空：按扩展名推断） */
        kindHint?: ResourceKindCode;
        /** 来源语义：client-upload / channel / tool-output / engine-temp */
        origin?: 'client-upload' | 'channel' | 'tool-output' | 'engine-temp';
        /** 保留等级建议值：normal（A 必存）/ soft（B 产出）/ ephemeral（C 参考） */
        retain?: 'normal' | 'soft' | 'ephemeral';
        /** 建议 MIME（可空：按扩展名推断） */
        mime?: string;
        /** 按内容哈希去重（媒体默认开启） */
        dedup?: boolean;
        /** 强制写入指定目录：true 时跳过内容哈希复用 */
        mustStore?: boolean;
    }

    /** 入库结果 */
    interface ResourceAdmitResult {
        /** 规范句柄（形如 @I.007） */
        handle: string;
        /** 类型码 */
        kind: ResourceKindCode;
        /** 资源序号 */
        seq: number;
        /** 资源目录自增主键（写摘要 / 段落表时使用） */
        id: number;
        /** 内容哈希（sha16） */
        content_hash: string;
        /** 相对 LocalDir 的物理路径（内容仓库位置，可交给图片处理管线取回） */
        blob_path: string;
        /** 规范 MIME */
        mime: string;
        /** 字节数 */
        size_bytes: number;
        /** 原始文件名 */
        name: string;
        /** 是否复用了既有 blob（内容去重命中） */
        deduped: boolean;
        /** blob 状态：active / lost */
        state: 'active' | 'lost';
    }

    /** 资源取回结果 */
    interface ResourceReadResult {
        /** 请求的句柄（含锚点） */
        handle: string;
        /** 类型码 */
        kind: ResourceKindCode;
        /** 展示标题 */
        title: string;
        /** 文本内容（文档 / 表格；携带锚点时为其完整行区间） */
        content?: string;
        /** 二进制内容 base64（图片 / 视频 / 音频本体） */
        base64?: string;
        /** 规范 MIME */
        mime: string;
        /** 内容仓库路径（相对 LocalDir，仅供 Go 侧图片管线使用；不得拼 image_url） */
        blob_path: string;
        /** blob 已丢失：句柄有效但本体不可取，应降级为描述 */
        lost: boolean;
        /** 内容按上限被截断 */
        truncated: boolean;
        /** 命中的章节标题（携带 § 锚点时） */
        section_title?: string;
        /** 实际返回的行区间（1 基，含端点） */
        line_start?: number;
        line_end?: number;
        /** 资源总行数（文本文档） */
        total_lines?: number;
    }

    /** 资源目录条目 */
    interface ResourceEntry {
        id: number;
        kind: ResourceKindCode;
        seq: number;
        /** 规范句柄，形如 @D.003 */
        handle: string;
        name: string;
        ext: string;
        mime: string;
        content_hash: string;
        blob_path: string;
        size_bytes: number;
        width?: number;
        height?: number;
        duration_ms?: number;
        title: string;
        summary: string;
        tags: string[];
        origin: string;
        retain: 'normal' | 'soft' | 'ephemeral';
        state: 'active' | 'lost';
        section_count: number;
        created_at: number;
        used_at: number;
    }

    /** 资源段落（索引单元，不进句柄文法） */
    interface ResourceSection {
        id: number;
        resource_id: number;
        /** 章节序号（句柄 |§n 使用） */
        ordinal: number;
        /** 标题层级 1..6（无标题文本为 0） */
        level: number;
        title: string;
        /** 层级路径（如「一、功能 / 1.2 用法」） */
        path: string;
        /** 正文起始行（1 基） */
        line_start: number;
        /** 正文结束行（含） */
        line_end: number;
        /** 正文字数 */
        chars: number;
    }

    /** 段落表写入草稿 */
    interface ResourceSectionDraft {
        ordinal: number;
        level: number;
        title: string;
        path: string;
        lineStart: number;
        lineEnd: number;
        chars: number;
    }

    /** 跨资源搜索命中 */
    interface ResourceSearchHit {
        handle: string;
        kind: ResourceKindCode;
        title: string;
        summary: string;
        /** 命中章节（文档） */
        sections?: ResourceSection[];
        /** 命中位置：card / section */
        matched_in: string;
    }

    /**
     * 统一入库：归一来源（本地文件 / 远程 URL / data URI / 字节）→ 落盘 → 登记资源目录 → 返回句柄
     *
     * 这是全模态资源进入系统的唯一通道：前端已落盘的图片只登记不复制（零拷贝），
     * QQ 等渠道仅给 URL 的图片会下载后按内容哈希落盘。哈希为内容 SHA-256 前 16 位。
     *
     * @param {ResourceAdmitInput} input 入库入参
     *
     * @returns {[ResourceAdmitResult | null, Error | null]} 包含入库结果的元组
     */
    function resourceAdmit(input: ResourceAdmitInput): [ResourceAdmitResult | null, Error | null];

    /**
     * 文档按「documents/原文件名」入库或刷新
     *
     * 同一路径复用同一句柄：内容变化时只刷新哈希、体积与段落表，
     * 避免群聊里反复发送同一份文本文件时不断产生新的 @D 序号。
     *
     * @param {string} path 相对 LocalDir 的路径（必须在 documents/ 下）
     * @param {string} name 原始文件名
     *
     * @returns {[ResourceAdmitResult | null, Error | null]} 包含入库结果的元组
     */
    function resourceAdmitDocument(path: string, name: string): [ResourceAdmitResult | null, Error | null];

    /**
     * 按句柄取回资源内容（带锚点时按锚点整段展开）
     *
     * blob 已丢失时不报错，返回 lost=true，由调用方降级为描述或大纲。
     *
     * @param {string} handle 资源句柄，形如 @D.003 / @D.003|§2 / @I.007 / @D.003|L120-168
     *
     * @param {number} [maxBytes] 单次取回字节上限（缺省 256KB）
     *
     * @returns {[ResourceReadResult | null, Error | null]} 包含取回结果的元组
     */
    function resourceRead(handle: string, maxBytes?: number): [ResourceReadResult | null, Error | null];

    /**
     * 读取资源大纲（段落表）；段落表为空且 blob 在位时按标题现场补建
     *
     * @param {string} handle 资源句柄
     *
     * @returns {[ResourceSection[] | null, string, Error | null]} 元组 [段落数组, 规范句柄, 错误信息]
     */
    function resourceOutline(handle: string): [ResourceSection[] | null, string, Error | null];

    /**
     * 列出资源目录（越靠前越新）
     *
     * @param {string} [kind] 类型码过滤（空表示全部）
     *
     * @param {number} [limit] 条数上限（缺省 50）
     *
     * @returns {[ResourceEntry[] | null, Error | null]} 包含资源列表的元组
     */
    function resourceList(kind?: string, limit?: number): [ResourceEntry[] | null, Error | null];

    /**
     * 跨资源搜索（标题 / 摘要 / 标签 / 文件名 / 段落标题子串匹配）
     *
     * 定位到「哪份资源的哪一节」，取正文请随后调用 resourceRead。
     *
     * @param {string} query 搜索关键词
     *
     * @param {string} [kind] 类型码过滤
     *
     * @param {number} [limit] 条数上限（缺省 10）
     *
     * @returns {[ResourceSearchHit[] | null, Error | null]} 包含命中列表的元组
     */
    function resourceSearch(query: string, kind?: string, limit?: number): [ResourceSearchHit[] | null, Error | null];

    /**
     * 资源目录统计（条数 / 体积 / 按类型与状态分布）
     *
     * @returns {[object | null, Error | null]} 包含统计对象的元组
     */
    function resourceStats(): [{
        total: number;
        by_kind: Record<string, number>;
        by_state: Record<string, number>;
        by_retain: Record<string, number>;
        bytes: number;
    } | null, Error | null];

    /**
     * 写入资源摘要 / 描述（图片视觉描述、文档概要）；tags 传空数组表示保留原标签
     *
     * @param {number} id 资源目录主键
     *
     * @param {string} summary 摘要文本
     *
     * @param {string[]} [tags] 标签数组
     *
     * @returns {[boolean, Error | null]} 包含操作结果的元组
     */
    function resourceSetSummary(id: number, summary: string, tags?: string[]): [boolean, Error | null];

    /**
     * 全量替换资源段落表（入库与迁移重建使用）
     *
     * @param {number} id 资源目录主键
     *
     * @param {ResourceSectionDraft[]} sections 段落草稿数组
     *
     * @returns {[boolean, Error | null]} 包含操作结果的元组
     */
    function resourceSetSections(id: number, sections: ResourceSectionDraft[]): [boolean, Error | null];

    /**
     * 显式删除资源（目录行 + 独占 blob；仍被其它目录行引用时保留实体）
     *
     * @param {string} handle 资源句柄
     *
     * @returns {[boolean, Error | null]} 包含操作结果的元组
     */
    function resourceDelete(handle: string): [boolean, Error | null];

    /**
     * 取回句柄文法（Go 侧为唯一权威定义，TypeScript 与前端不得各写一套）
     *
     * @returns {{pattern: string, handle: string, section: string, line: string, seqWidth: number, typeCodes: Record<string, string>}}
     */
    function resourceGrammar(): {
        pattern: string;
        handle: string;
        section: string;
        line: string;
        seqWidth: number;
        typeCodes: Record<string, string>;
    };
}