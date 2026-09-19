import type { FileListItem, ProxyFetchConfig } from './config/config';
import type { MediaSegment, ResizeImageResult, ResizeImageResults, GenerateImageParams, GenerateImageResult } from './config/image';
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
     * 入参与 resizeImage 一致：字节数据（Uint8Array 等）直接使用，
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
     * 超过 60 秒的视频自动按 60 秒分段（FFmpeg 流复制切分）。
     *
     * @param {string} inputFile 视频地址（URL / data URI / 本地路径）
     *
     * @returns {[MediaSegment[], Error | null]} 包含媒体片段列表的元组，[{file, start, end}]
     */
    function videoMedia(inputFile: string): [MediaSegment[], Error | null];
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
     * 拉取语音/音频URL
     *
     * @returns {string[]} 语音/音频URL列表
     */
    function pullAudioUrl(): string[];
    /**
     * 标准图片处理：解码 → 格式校验/转码 → 等比例缩放到 1024 → 输出 base64
     *
     * 入参与 isAnimatedImage / animatedImageToVideo 一致，可传字节数据或来源地址
     * （HTTP(S) URL、data:image URI、本地路径），地址类入参由 Go 侧下载/解码。
     * 返回恒为单元素数组：动态图在此按首帧静态降级，动态图理解请先经
     * isAnimatedImage 判定并走 animatedImageToVideo 的视频链路
     *
     * @param {Blob | File | FormData | string | Uint8Array} imgData 图片数据或来源地址
     *
     * @returns {[ResizeImageResults, Error | null]} 包含处理结果数组的元组，[处理结果数组, 错误信息]
     */
    function resizeImage(imgData: Blob | File | FormData | string | Uint8Array): [ResizeImageResults, Error | null];
    /**
     * 判断图片是否为动态图（多帧 GIF / APNG / 动态 WebP）
     *
     * 入参支持字节数据与来源地址（HTTP(S) URL、data:image URI、本地文件路径），
     * 传入地址时由 Go 侧下载/解码后判定，无需 TypeScript 先取回字节
     *
     * @param {Blob | File | string | Uint8Array} imgData 图片数据或地址
     *
     * @returns {[boolean, Error | null]} 是否动态图与错误信息
     */
    function isAnimatedImage(imgData: Blob | File | string | Uint8Array): [boolean, Error | null];
    /**
     * 将动态图编码为慢放视频并写入 llama-server 媒体目录
     *
     * 动态图多为数秒，直接交给固定抽帧频率的 llama-server 只能采到一两帧，
     * 因此转码时按目标采样点数做等比例慢放，再以 file:// 交给感知者理解
     *
     * @param {Blob | File | string | Uint8Array} imgData 动态图数据或地址
     *
     * @returns {[MediaSegment[], Error | null]} 包含媒体片段列表的元组，[{file, start, end}]
     */
    function animatedImageToVideo(imgData: Blob | File | string | Uint8Array): [MediaSegment[], Error | null];
    /**
     * 生成图片
     * 
     * @param {GenerateImageParams} params 图片生成参数
     * 
     * @returns {[GenerateImageResult, Error | null]} 包含生成图片结果的元组，[生成图片结果, 错误信息]
     */
    function generateImage(params: GenerateImageParams): [GenerateImageResult, Error | null];
    /**
     * 拉取上下文
     * 
     * @returns {MultimodalMessage[]} 上下文消息列表
     */
    function pullContext(): MultimodalMessage[];
    /**
     * 拉取视频URL
     * 
     * @returns {string[]} 视频URL列表
     */
    function pullVideoUrl(): string[];
    /**
     * 推送上下文
     * 
     * @param {string} msgType 消息类型（text=文本 / music=乐谱 / action=动作）
     * 
     * @param {string} data 消息内容
     */
    function pushContext(msgType: string, data: string, audio: string): boolean;
    /**
     * 推送图片
     * 
     * @param {string[]} imageData 图片数据列表（base64编码）
     * @param {boolean} [isSticker=false] 是否为表情包（前端据此决定角标显示）
     */
    function pushImage(imageData: string[], isSticker?: boolean): boolean;
    /**
     * 获取缓存的智能体3D位置（由前端遥测数据更新）
     *
     * @returns {{ x: number, y: number, z: number }} 智能体当前位置
     */
    function getAgentPosition(): { x: number; y: number; z: number };
    /**
     * 将3D引擎事件推送到AI上下文
     *
     * @param {string} eventType 事件类型
     * @param {string} data 事件数据JSON字符串
     * @returns {boolean} 是否成功
     */
    function pushAgentEvent(eventType: string, data: string): boolean;
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
     * 初始化记忆库实例并创建/打开指定集合（v5 文本/图片混合存储，仅需集合名）
     * 模型配置从 lunar_config.json 的 memory 配置组读取
     *
     * @param {string} collectionName 集合名称
     *
     * @returns {[boolean, Error | null]} 包含初始化结果的元组，[是否成功, 错误信息]
     */
    function memoryInit(collectionName: string): [boolean, Error | null];
    /**
     * 向指定集合添加消息
     *
     * @param {string} collectionName 集合名称
     *
     * @param {string} role 消息角色 (user/assistant/system/tool)
     *
     * @param {string} content 消息文本内容
     *
     * @returns {[boolean, Error | null]} 包含操作结果的元组，[是否成功, 错误信息]
     */
    function memoryAdd(collectionName: string, role: string, content: string): [boolean, Error | null];
    /**
     * 向指定集合添加消息，携带调用方提供的显式标签（跳过 LLM 标签生成）
     *
     * @param {string} collectionName 集合名称
     *
     * @param {string} role 消息角色 (user/assistant/system/tool)
     *
     * @param {string} content 消息文本内容
     *
     * @param {string[]} tags 显式标签数组（如启发式提取的 Function/Class 名）
     *
     * @returns {[string, Error | null]} 包含操作结果的元组，[文档ID, 错误信息]
     */
    function memoryAddWithTags(collectionName: string, role: string, content: string, tags: string[]): [string, Error | null];
    /**
     * 从指定集合查询相关消息
     *
     * @param {string} collectionName 集合名称
     *
     * @param {string} queryText 查询文本
     *
     * @param {number} topK 返回的最相关结果数量
     *
     * @returns {[Array<{id: string, role: string, content?: string, image?: string, similarity: number, timestamp?: number}>, Error | null]} 包含查询结果的元组，结果按相似度降序排列（text 集合返回 content，image 集合返回 image；timestamp 为入库 Unix 秒时间戳，旧数据无此字段）
     */
    function memoryQuery(collectionName: string, queryText: string, topK: number): [Array<{ id: string, role: string, content?: string, image?: string, similarity: number, timestamp?: number }>, Error | null];
    /**
     * 从指定集合删除消息
     *
     * @param {string} collectionName 集合名称
     *
     * @param {string} id 要删除的消息ID
     *
     * @returns {[boolean, Error | null]} 包含操作结果的元组，[是否成功, 错误信息]
     */
    function memoryDelete(collectionName: string, id: string): [boolean, Error | null];
    /**
     * 向 image 类型集合添加图片文档（LLM 自动生成标签）
     *
     * @param {string} collectionName 集合名称
     *
     * @param {string} base64Image 图片 base64 数据（data:image/...;base64,...）
     *
     * @param {string} [orientation] 图片识别取向标识（auto/emotion/text/color/appearance/species/posture/custom），默认 auto
     *
     * @param {string} [custom] 自定义识别取向参考文本（仅 orientation 为 custom 时使用）
     *
     * @returns {[string, Error | null]} 包含操作结果的元组，[图片文档ID, 错误信息]
     */
    function memoryAddImage(collectionName: string, base64Image: string, orientation?: string, custom?: string): [string, Error | null];
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
     * 获取琉璃（远程 LTPX）在线状态与最新工具链
     * 思考链起点调用：主动向琉璃心跳，若在线则拉取最新工具链写入内部缓存并返回
     *
     * @returns {string} JSON 字符串：{ online: boolean, url: string, tools: [{name,description,app_id,parameters}] }
     */
    function getLTPXRemoteStatus(): string;
    /**
     * 转发工具调用到琉璃（远程 LTPX）
     *
     * @param {string} toolName 琉璃工具名
     * @param {string} argumentsJSON 工具参数 JSON 字符串
     *
     * @returns {Promise<string>} 异步返回琉璃的文本结果；失败时 resolve 以「【琉璃工具调用失败】」开头的错误文本
     */
    function callLTPXRemoteTool(toolName: string, argumentsJSON: string): Promise<string>;
    /**
     * 清空内部缓存的琉璃工具链（琉璃离线时调用）
     *
     * @returns {boolean} 是否成功
     */
    function clearLTPXRemoteTools(): boolean;
    /**
     * 同步推送事件到琉璃并接收插件处理结果（事件触发点调用）
     *
     * 把「xx事件发生前」的原始负载推送到琉璃 /ltpx/event，同步阻塞等待琉璃 LTP9
     * 插件处理完成。琉璃离线 / 无插件订阅 / 插件未回传业务结果时 returned=false
     * （调用方应回退本地默认处理）。
     *
     * @param {string} topic 事件主题（如 message_received_before）
     * @param {string} payloadJSON 事件负载 JSON 字符串（可为 "{}"）
     *
     * @returns {string} JSON 字符串：{ online: boolean, modified: boolean, data: any, returned: boolean, return: any }
     */
    function ltpInteractEvent(topic: string, payloadJSON: string): string;
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
}