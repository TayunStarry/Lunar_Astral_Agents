import type { KeyFrame, FileListItem, KnowledgeRequest, BatchResult, ProxyFetchConfig, ResizeImageResult, GenerateImageParams, GenerateImageResult, MultimodalMessage, TTSParams } from './index';

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
    function saveFile(fileName: string, overwrite: boolean, fileData: Blob | File | FormData | string): [string, string, Error | null];
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
     * 执行知识库请求
     *
     * @param {KnowledgeRequest} request 知识库请求对象
     *
     * @returns {[BatchResult, Error | null]} 包含知识库操作结果的元组，[操作结果, 错误信息]
     */
    function knowledge(request: KnowledgeRequest): [BatchResult, Error | null];
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
     * 提取视频关键帧
     * 
     * @param {string} inputFile 视频文件路径
     * 
     * @param {string} cacheDir 缓存目录
     * 
     * @returns {[KeyFrame[], Error | null]} 包含关键帧列表的元组，[关键帧列表, 错误信息]
     */
    function keyframe(inputFile: string, cacheDir: string): [KeyFrame[], Error | null];
    /**
     * 调整图片大小
     * 
     * @param {Blob | File | FormData | string | Uint8Array} imgData 图片数据
     * 
     * @returns {[ResizeImageResult, Error | null]} 包含调整后的图片结果的元组，[调整后的图片结果, 错误信息]
     */
    function resizeImage(imgData: Blob | File | FormData | string | Uint8Array): [ResizeImageResult, Error | null];
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
     * @param {string} msgType 消息类型
     * 
     * @param {string} data 消息内容
     */
    function pushContext(msgType: string, data: string, audio: string): boolean;
    /**
     * 推送图片
     * 
     * @param {string[]} imageData 图片数据列表（base64编码）
     */
    function pushImage(imageData: string[]): boolean;
    /**
     * 初始化记忆库实例并创建指定集合
     *
     * @param {string} baseURL 嵌入模型服务的基础URL (e.g. http://localhost:36789/v1)
     *
     * @param {string} apiKey API密钥
     *
     * @param {string} modelName 嵌入模型名称（集合级锁定）
     *
     * @param {string} collectionName 集合名称（探针定维度）
     *
     * @returns {[boolean, Error | null]} 包含初始化结果的元组，[是否成功, 错误信息]
     */
    function memoryInit(baseURL: string, apiKey: string, modelName: string, collectionName: string): [boolean, Error | null];
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
     * 从指定集合查询相关消息
     *
     * @param {string} collectionName 集合名称
     *
     * @param {string} queryText 查询文本
     *
     * @param {number} topK 返回的最相关结果数量
     *
     * @returns {[Array<{id: string, role: string, content: string, similarity: number}>, Error | null]} 包含查询结果的元组，结果按相似度降序排列
     */
    function memoryQuery(collectionName: string, queryText: string, topK: number): [Array<{ id: string, role: string, content: string, similarity: number }>, Error | null];
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
     * 初始化网络检索子系统
     *
     * @param {string} baseURL LLM 服务基础 URL
     *
     * @param {string} apiKey LLM API 密钥
     *
     * @param {string} model LLM 模型名称
     *
     * @param {number} maxTokens 最大生成 token 数
     *
     * @param {number} temperature 生成温度
     *
     * @returns {[boolean, Error | null]} 包含初始化结果的元组，[是否成功, 错误信息]
     */
    function webSearchInit(baseURL: string, apiKey: string, model: string, maxTokens: number, temperature: number): [boolean, Error | null];
    /**
     * 执行网页搜索
     *
     * @param {string} query 搜索查询
     *
     * @returns {[string, Error | null]} 包含搜索结果的元组，[搜索结果文本, 错误信息]
     */
    function webSearchWebpage(query: string): [string, Error | null];
    /**
     * 执行轻量摘要
     *
     * @param {string} query 搜索查询
     *
     * @returns {[string, Error | null]} 包含搜索结果的元组，[搜索结果文本, 错误信息]
     */
    function webSearchSimple(query: string): [string, Error | null];
    /**
     * 执行深度研究（子问题拆解 + 并行搜索 + URL去重 + 综合报告）
     *
     * @param {string} query 搜索查询
     *
     * @returns {[string, Error | null]} 包含搜索结果的元组，[搜索结果文本, 错误信息]
     */
    function webSearchDepth(query: string): [string, Error | null];
    /**
	 * 检查网络检索子系统是否已初始化
	 *
	 * @returns {boolean} 是否已初始化
	 */
	function webSearchIsReady(): boolean;
	/**
	 * 执行大会辩论式深度研究
	 * 需先调用 webSearchSetMemoryProvider 设置记忆库提供者
	 *
	 * @param {string} query 搜索查询
	 *
	 * @returns {[string, Error | null]} 包含搜索结果的元组，[搜索结果文本, 错误信息]
	 */
	function webSearchAssembly(query: string): [string, Error | null];
	/**
	 * 处理消息中的链接，提取URL并替换为摘要
	 * 对网页链接抓取内容并总结，对图片链接使用视觉模型识别，对下载链接调用下载回调
	 *
	 * @param {string} query 包含链接的消息文本
	 *
	 * @returns {[string, string[], Error | null]} 包含处理结果的元组，[替换后的文本, 链接描述列表, 错误信息]
	 */
	function webSearchProcessLinks(query: string): [string, string[], Error | null];
	/**
	 * 设置记忆库提供者（供大会辩论的守旧派使用）
	 * 自动使用内置记忆库实例，无需参数
	 *
	 * @returns {[boolean, Error | null]} 包含设置结果的元组，[是否成功, 错误信息]
	 */
	function webSearchSetMemoryProvider(): [boolean, Error | null];
	/**
	 * 设置下载回调函数
	 * 配置后，processLinks 遇到下载链接时会自动下载文件到指定目录
	 *
	 * @param {string} downloadDir 下载目标目录
	 * @param {string} groupID 下载目标群组ID
	 *
	 * @returns {[boolean, Error | null]} 包含设置结果的元组，[是否成功, 错误信息]
	 */
	function webSearchSetDownloadFunc(downloadDir: string, groupID: string): [boolean, Error | null];
    /**
     * 执行屏幕截图（内部已集成图片压缩缩放处理）
     * 
     * Go 层统一完成截图捕获 + 图片压缩缩放 + base64 编码，
     * 返回的 base64 字段格式为 "data:image/[format];base64,[data]"
     *
     * @param {number} displayIndex 显示器索引（-1 表示所有显示器，0 表示主显示器）
     *
     * @param {string} [region] 截图区域，格式为 "x,y,width,height"
     *
     * @param {string} [scale] 缩放参数，如 "0.5" 或 "800,600"
     *
     * @param {string} [format] 图片格式，"png" 或 "jpg"
     *
     * @param {number} [quality] JPEG 质量 1-100
     *
     * @returns {[ResizeImageResult, Error | null]} 包含处理结果的元组，[结果对象(base64/format/width/height), 错误信息]
     */
    function screenshotCapture(displayIndex: number, region?: string, scale?: string, format?: string, quality?: number): [ResizeImageResult, Error | null];
    /**
     * 获取所有显示器信息
     *
     * @returns {[Array<{index: number, x: number, y: number, width: number, height: number}>, Error | null]} 包含显示器信息的元组，[显示器列表, 错误信息]
     */
    function screenshotGetDisplays(): [Array<{ index: number; x: number; y: number; width: number; height: number }>, Error | null];
    /**
     * 获取 LTPX 工具状态
     * 返回当前已加载工具列表和待处理的加载/卸载操作
     * 
     * @returns {string} JSON 字符串，包含 loaded、pendingLoads、pendingUnloads
     */
    function getLTPXToolStatus(): string;
    /**
     * 处理 LTPX 工具变更（加载/卸载）
     * 在 goja 事件循环中执行工具的注册和注销
     * 
     * @param {string} statusJSON 工具状态 JSON 字符串
     * 
     * @returns {boolean} 是否处理成功
     */
    function processLTPXChanges(statusJSON: string): boolean;
	    /**
	     * 初始化学习者智能体
	     *
	     * @param {string} baseURL LLM 服务基础 URL
	     * @param {string} apiKey LLM API 密钥
	     * @param {string} model LLM 模型名称
	     * @param {number} maxTokens 最大生成 token 数
	     * @param {number} temperature 生成温度
	     * @param {string} embeddingURL 嵌入服务基础 URL
	     * @param {string} embeddingKey 嵌入服务 API 密钥
	     * @param {string} embeddingModel 嵌入模型名称
	     *
	     * @returns {[boolean, Error | null]} 包含初始化结果的元组，[是否成功, 错误信息]
	     */
	    function learnerInit(baseURL: string, apiKey: string, model: string, maxTokens: number, temperature: number, embeddingURL: string, embeddingKey: string, embeddingModel: string): [boolean, Error | null];
	    /**
		     * 执行学习者研究
		     *
		     * @param {string} dialogueJSON 对话历史消息的 JSON 字符串
		     * @param {string} unreadJSON 未读消息的 JSON 字符串
		     * @param {string} intentHint 意图提示 ("memory" | "search" | "balanced" | "ambiguous")
		     *
		     * @returns {[string, Error | null]} 包含研究结果的元组，[研究报告文本, 错误信息]
		     */
		    function learnerExecute(dialogueJSON: string, unreadJSON: string, intentHint: string): [string, Error | null];
	    /**
	     * 检查学习者智能体是否已初始化
	     *
	     * @returns {boolean} 是否已初始化
	     */
	    function learnerIsReady(): boolean;
	    /**
	     * 导出学习者 Go 层运行时上下文到文件（覆写模式）
	     * 包含搜索结果、策略评估、辩论状态、记忆匹配等 Go 层完整数据
	     *
	     * @param {string} dialogueJSON 对话历史消息的 JSON 字符串
	     * @param {string} unreadJSON 未读消息的 JSON 字符串
	     * @param {string} intentHint 意图提示
	     * @param {string} outputPath 输出文件路径
	     *
	     * @returns {[string, Error | null]} 包含导出结果的元组，[文件路径, 错误信息]
	     */
	    function learnerDumpContext(dialogueJSON: string, unreadJSON: string, intentHint: string, outputPath: string): [string, Error | null];
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