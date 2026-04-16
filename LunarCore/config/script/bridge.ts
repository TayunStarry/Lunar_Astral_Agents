import { KeyFrame, FileListItem, DatabaseRequest, BatchResult, ProxyFetchConfig, ResizeImageResult, GenerateImageParams, GenerateImageResult } from '../index';

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
 * */
export declare function SaveFile(fileName: string, overwrite: boolean, fileData: Blob | File | FormData | string): [string, string, Error | null];

/**
 * 从磁盘中读取文件
 * 
 * @param {string} filePath 文件路径
 * 
 * @returns {[Blob, number, string, Error | null]} 包含读取结果的元组，[文件内容, 文件大小, MIME类型, 错误信息]
 * */
export declare function ReadFile(filePath: string): [Blob, number, string, Error | null];

/**
 * 获取指定目录下的所有文件列表
 * 
 * @param {string} path 目录路径
 * 
 * @returns {[FileListItem[], Error | null]} 包含文件列表的元组，[文件列表, 错误信息]
 * */
export declare function GetFileList(path: string): [FileListItem[], Error | null];

/**
 * 执行数据库请求
 * 
 * @param {DatabaseRequest} request 数据库请求对象
 * 
 * @returns {[BatchResult, Error | null]} 包含数据库操作结果的元组，[操作结果, 错误信息]
 * */
export declare function ExecuteDatabaseRequest(request: DatabaseRequest): [BatchResult, Error | null];

/**
 * 获取当前地址信息
 * 
 * @returns {[string[], Error | null]} 包含地址信息的元组，[省份, 城市, 错误信息]
 * */
export declare function QueryCurrentAddress(): [string[], Error | null];

/**
 * 获取系统访问URL
 * 
 * @returns {[string, Error | null]} 包含系统URL的元组，[系统URL, 错误信息]
 * */
export declare function GetSystemUrl(): [string, Error | null];

/**
 * 提取视频关键帧
 * 
 * @param {string} inputFile 视频文件路径
 * 
 * @param {string} cacheDir 缓存目录
 * 
 * @returns {[KeyFrame[], Error | null]} 包含关键帧列表的元组，[关键帧列表, 错误信息]
 * */
export declare function VideoKeyframeExtraction(inputFile: string, cacheDir: string): [KeyFrame[], Error | null];

/**
 * 网络请求代理函数
 * 
 * @param {ProxyFetchConfig} config 请求配置对象
 * 
 * @returns {Promise<[any, Error | null]>} 包含响应结果的元组，[响应结果, 错误信息]
 * */
export declare function ProxyFetch(config: ProxyFetchConfig): Promise<[any, Error | null]>;

/**
 * 缩放图片
 * 
 * @param {Blob | File | FormData | string | Uint8Array} imgData 图片数据
 * 
 * @returns {[ResizeImageResult, Error | null]} 包含缩放结果的元组，[缩放结果, 错误信息]
 * */
export declare function ResizeImage(imgData: Blob | File | FormData | string | Uint8Array): [ResizeImageResult, Error | null];


/**
 * 生成图片
 * 
 * @param {GenerateImageParams} params 图片生成参数
 * 
 * @returns {Promise<[GenerateImageResult, Error | null]>} 包含生成结果的元组，[生成结果, 错误信息]
 * */
export declare function GenerateImage(params: GenerateImageParams): Promise<[GenerateImageResult, Error | null]>;