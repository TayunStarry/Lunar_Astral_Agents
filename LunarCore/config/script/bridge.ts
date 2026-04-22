import { KeyFrame, FileListItem, DatabaseRequest, BatchResult, ProxyFetchConfig, ResizeImageResult, GenerateImageParams, GenerateImageResult } from '../index';

// 声明全局函数，使用不同的名称以避免冲突
declare function _SaveFile(fileName: string, overwrite: boolean, fileData: Blob | File | FormData | string): [string, string, Error | null];
declare function _ReadFile(filePath: string): [Blob, number, string, Error | null];
declare function _GetFileList(path: string): [FileListItem[], Error | null];
declare function _ExecuteDatabaseRequest(request: DatabaseRequest): [BatchResult, Error | null];
declare function _QueryCurrentAddress(): [string[], Error | null];
declare function _GetSystemUrl(): [string, Error | null];
declare function _VideoKeyframeExtraction(inputFile: string, cacheDir: string): [KeyFrame[], Error | null];
declare function _ProxyFetch(config: ProxyFetchConfig): Promise<[any, Error | null]>;
declare function _ResizeImage(imgData: Blob | File | FormData | string | Uint8Array): [ResizeImageResult, Error | null];
declare function _GenerateImage(params: GenerateImageParams): Promise<[GenerateImageResult, Error | null]>;
declare function _waiter(ms: number): Promise<[boolean, Error | null]>;
declare function _log(message: string): Promise<[boolean, Error | null]>;

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
export function SaveFile(fileName: string, overwrite: boolean, fileData: Blob | File | FormData | string): [string, string, Error | null] {
    return _SaveFile(fileName, overwrite, fileData);
}

/**
 * 从磁盘中读取文件
 * 
 * @param {string} filePath 文件路径
 * 
 * @returns {[Blob, number, string, Error | null]} 包含读取结果的元组，[文件内容, 文件大小, MIME类型, 错误信息]
 * */
export function ReadFile(filePath: string): [Blob, number, string, Error | null] {
    return _ReadFile(filePath);
}

/**
 * 获取指定目录下的所有文件列表
 * 
 * @param {string} path 目录路径
 * 
 * @returns {[FileListItem[], Error | null]} 包含文件列表的元组，[文件列表, 错误信息]
 * */
export function GetFileList(path: string): [FileListItem[], Error | null] {
    return _GetFileList(path);
}

/**
 * 执行数据库请求
 * 
 * @param {DatabaseRequest} request 数据库请求对象
 * 
 * @returns {[BatchResult, Error | null]} 包含数据库操作结果的元组，[操作结果, 错误信息]
 * */
export function ExecuteDatabaseRequest(request: DatabaseRequest): [BatchResult, Error | null] {
    return _ExecuteDatabaseRequest(request);
}

/**
 * 获取当前地址信息
 * 
 * @returns {[string[], Error | null]} 包含地址信息的元组，[省份, 城市, 错误信息]
 * */
export function QueryCurrentAddress(): [string[], Error | null] {
    return _QueryCurrentAddress();
}

/**
 * 获取系统访问URL
 * 
 * @returns {[string, Error | null]} 包含系统URL的元组，[系统URL, 错误信息]
 * */
export function GetSystemUrl(): [string, Error | null] {
    return _GetSystemUrl();
}

/**
 * 提取视频关键帧
 * 
 * @param {string} inputFile 视频文件路径
 * 
 * @param {string} cacheDir 缓存目录
 * 
 * @returns {[KeyFrame[], Error | null]} 包含关键帧列表的元组，[关键帧列表, 错误信息]
 * */
export function VideoKeyframeExtraction(inputFile: string, cacheDir: string): [KeyFrame[], Error | null] {
    return _VideoKeyframeExtraction(inputFile, cacheDir);
}

/**
 * 网络请求代理函数
 * 
 * @param {ProxyFetchConfig} config 请求配置对象
 * 
 * @returns {Promise<[any, Error | null]>} 包含响应结果的元组，[响应结果, 错误信息]
 * */
export async function ProxyFetch(config: ProxyFetchConfig): Promise<[any, Error | null]> {
    return await _ProxyFetch(config);
}

/**
 * 缩放图片
 * 
 * @param {Blob | File | FormData | string | Uint8Array} imgData 图片数据
 * 
 * @returns {[ResizeImageResult, Error | null]} 包含缩放结果的元组，[缩放结果, 错误信息]
 * */
export function ResizeImage(imgData: Blob | File | FormData | string | Uint8Array): [ResizeImageResult, Error | null] {
    return _ResizeImage(imgData);
}


/**
 * 生成图片
 * 
 * @param {GenerateImageParams} params 图片生成参数
 * 
 * @returns {Promise<[GenerateImageResult, Error | null]>} 包含生成结果的元组，[生成结果, 错误信息]
 * */
export async function GenerateImage(params: GenerateImageParams): Promise<[GenerateImageResult, Error | null]> {
    return await _GenerateImage(params);
}

/**
 * 等待指定毫秒数后返回true
 * 
 * @param {number} ms 毫秒数
 * 
 * @returns {Promise<[boolean, Error | null]>} 包含等待结果的元组，[true, 错误信息]
 * */
export async function Waiter(ms: number): Promise<[boolean, Error | null]> {
    return await _waiter(ms);
}

// 存储定时器ID和对应的清除函数
const timers = new Map<number, () => void>();
let nextTimerId = 1;

/**
 * 在指定的毫秒数后执行一次函数
 * 
 * @param {Function} callback 要执行的函数
 * @param {number} delay 延迟的毫秒数
 * @param {...any} args 传递给函数的参数
 * 
 * @returns {number} 定时器ID，可用于 clearTimeout
 */
export function setTimeout(callback: Function, delay: number, ...args: any[]): number {
    const timerId = nextTimerId++;
    let cancelled = false;
    
    // 存储清除函数
    timers.set(timerId, () => {
        cancelled = true;
    });
    
    // 执行定时器
    (async () => {
        await _waiter(delay);
        
        if (!cancelled) {
            callback(...args);
        }
        
        // 执行完毕后移除定时器
        timers.delete(timerId);
    })();
    
    return timerId;
}

/**
 * 取消由 setTimeout 设置的定时器
 * 
 * @param {number} timerId 由 setTimeout 返回的定时器ID
 */
export function clearTimeout(timerId: number): void {
    const clearFn = timers.get(timerId);
    if (clearFn) {
        clearFn();
        timers.delete(timerId);
    }
}

/**
 * 按照指定的毫秒数重复执行函数
 * 
 * @param {Function} callback 要执行的函数
 * @param {number} interval 间隔的毫秒数
 * @param {...any} args 传递给函数的参数
 * 
 * @returns {number} 定时器ID，可用于 clearInterval
 */
export function setInterval(callback: Function, interval: number, ...args: any[]): number {
    const timerId = nextTimerId++;
    let cancelled = false;
    
    // 存储清除函数
    timers.set(timerId, () => {
        cancelled = true;
    });
    
    // 执行定时器
    (async () => {
        while (!cancelled) {
            await _waiter(interval);
            
            if (!cancelled) {
                callback(...args);
            }
        }
        
        // 执行完毕后移除定时器
        timers.delete(timerId);
    })();
    
    return timerId;
}

/**
 * 取消由 setInterval 设置的定时器
 * 
 * @param {number} timerId 由 setInterval 返回的定时器ID
 */
export function clearInterval(timerId: number): void {
    const clearFn = timers.get(timerId);
    if (clearFn) {
        clearFn();
        timers.delete(timerId);
    }
}

/**
 * 在Go中使用log模块打印字符串
 * 
 * @param {string} message 要打印的消息
 * 
 * @returns {Promise<[boolean, Error | null]>} 包含打印结果的元组，[true, 错误信息]
 * */
export async function Log(message: string): Promise<[boolean, Error | null]> {
    return await _log(message);
}