import type { KeyFrame, FileListItem, DatabaseRequest, BatchResult, ProxyFetchConfig, ResizeImageResult, GenerateImageParams, GenerateImageResult } from './index';

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
}