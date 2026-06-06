import type { KeyFrame, FileListItem, DatabaseRequest, BatchResult, ProxyFetchConfig, ResizeImageResult, GenerateImageParams, GenerateImageResult, MultimodalMessage, TTSParams } from './index';

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
     * 执行数据库请求
     * 
     * @param {DatabaseRequest} request 数据库请求对象
     * 
     * @returns {[BatchResult, Error | null]} 包含数据库操作结果的元组，[操作结果, 错误信息]
     */
    function database(request: DatabaseRequest): [BatchResult, Error | null];
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
     * 初始化 chromem-go 向量数据库
     * 
     * @param {string} baseURL 嵌入模型服务的基础URL (e.g. http://localhost:36789/v1)
     * 
     * @param {string} apiKey API密钥
     * 
     * @param {string} modelName 嵌入模型名称
     * 
     * @returns {[boolean, Error | null]} 包含初始化结果的元组，[是否成功, 错误信息]
     */
    function chromemInit(baseURL: string, apiKey: string, modelName: string): [boolean, Error | null];
    /**
     * 向 chromem-go 向量数据库添加消息
     * 
     * @param {string} role 消息角色 (user/assistant/system/tool)
     * 
     * @param {string} content 消息文本内容
     * 
     * @returns {[boolean, Error | null]} 包含操作结果的元组，[是否成功, 错误信息]
     */
    function chromemAdd(role: string, content: string): [boolean, Error | null];
    /**
     * 从 chromem-go 向量数据库查询相关消息
     * 
     * @param {string} queryText 查询文本
     * 
     * @param {number} topK 返回的最相关结果数量
     * 
     * @returns {[Array<{id: string, role: string, content: string}>, Error | null]} 包含查询结果的元组
     */
    function chromemQuery(queryText: string, topK: number): [Array<{ id: string, role: string, content: string }>, Error | null];
    /**
     * 从 chromem-go 向量数据库删除指定消息
     * 
     * @param {string} id 要删除的消息ID
     * 
     * @returns {[boolean, Error | null]} 包含操作结果的元组，[是否成功, 错误信息]
     */
    function chromemDelete(id: string): [boolean, Error | null];
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
}