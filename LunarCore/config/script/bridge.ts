import { KeyFrame, FileListItem, DatabaseRequest, BatchResult, ProxyFetchConfig, ResizeImageResult, GenerateImageParams, GenerateImageResult } from '../index';

// 声明全局函数，使用不同的名称以避免冲突
declare function AdapterSaveFile(fileName: string, overwrite: boolean, fileData: Blob | File | FormData | string): [string, string, Error | null];
declare function AdapterReadFile(filePath: string): [Blob, number, string, Error | null];
declare function AdapterGetFileList(path: string): [FileListItem[], Error | null];
declare function AdapterExecuteDatabaseRequest(request: DatabaseRequest): [BatchResult, Error | null];
declare function AdapterQueryCurrentAddress(): [string[], Error | null];
declare function AdapterGetSystemUrl(): [string, Error | null];
declare function AdapterVideoKeyframeExtraction(inputFile: string, cacheDir: string): [KeyFrame[], Error | null];
declare function AdapterProxyFetch(config: ProxyFetchConfig): [any, Error | null];
declare function AdapterResizeImage(imgData: Blob | File | FormData | string | Uint8Array): [ResizeImageResult, Error | null];
declare function AdapterGenerateImage(params: GenerateImageParams): [GenerateImageResult, Error | null];
declare function AdapterWaiter(ms: number): [string, Error | null];
declare function AdapterLog(message: string): [string, Error | null];

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
    return AdapterSaveFile(fileName, overwrite, fileData);
}

/**
 * 从磁盘中读取文件
 * 
 * @param {string} filePath 文件路径
 * 
 * @returns {[Blob, number, string, Error | null]} 包含读取结果的元组，[文件内容, 文件大小, MIME类型, 错误信息]
 * */
export function ReadFile(filePath: string): [Blob, number, string, Error | null] {
    return AdapterReadFile(filePath);
}

/**
 * 获取指定目录下的所有文件列表
 * 
 * @param {string} path 目录路径
 * 
 * @returns {[FileListItem[], Error | null]} 包含文件列表的元组，[文件列表, 错误信息]
 * */
export function GetFileList(path: string): [FileListItem[], Error | null] {
    return AdapterGetFileList(path);
}

/**
 * 执行数据库请求
 * 
 * @param {DatabaseRequest} request 数据库请求对象
 * 
 * @returns {[BatchResult, Error | null]} 包含数据库操作结果的元组，[操作结果, 错误信息]
 * */
export function ExecuteDatabaseRequest(request: DatabaseRequest): [BatchResult, Error | null] {
    return AdapterExecuteDatabaseRequest(request);
}

/**
 * 获取当前地址信息
 * 
 * @returns {[string[], Error | null]} 包含地址信息的元组，[省份, 城市, 错误信息]
 * */
export function QueryCurrentAddress(): [string[], Error | null] {
    return AdapterQueryCurrentAddress();
}

/**
 * 获取系统访问URL
 * 
 * @returns {[string, Error | null]} 包含系统URL的元组，[系统URL, 错误信息]
 * */
export function GetSystemUrl(): [string, Error | null] {
    return AdapterGetSystemUrl();
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
    return AdapterVideoKeyframeExtraction(inputFile, cacheDir);
}



/**
 * 网络请求代理函数
 * 
 * @param {ProxyFetchConfig} config 请求配置对象
 * 
 * @returns {Promise<[any, Error | null]>} 包含响应结果的元组，[响应结果, 错误信息]
 * */
export function ProxyFetch(config: ProxyFetchConfig): [any, Error | null] {
    return AdapterProxyFetch(config);
}

/**
 * 缩放图片
 * 
 * @param {Blob | File | FormData | string | Uint8Array} imgData 图片数据
 * 
 * @returns {[ResizeImageResult, Error | null]} 包含缩放结果的元组，[缩放结果, 错误信息]
 * */
export function ResizeImage(imgData: Blob | File | FormData | string | Uint8Array): [ResizeImageResult, Error | null] {
    return AdapterResizeImage(imgData);
}


/**
 * 生成图片
 * 
 * @param {GenerateImageParams} params 图片生成参数
 * 
 * @returns {[GenerateImageResult, Error | null]} 包含生成结果的元组，[生成结果, 错误信息]
 * */
export function GenerateImage(params: GenerateImageParams): [GenerateImageResult, Error | null] {
    return AdapterGenerateImage(params);
}

/**
 * 等待指定毫秒数后返回消息
 * 
 * @param {number} ms 毫秒数
 * 
 * @returns {[string, Error | null]} 包含等待结果的元组，[消息, 错误信息]
 * */
export function Waiter(ms: number): [string, Error | null] {
    return AdapterWaiter(ms);
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
        await AdapterWaiter(delay);
        
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
            await AdapterWaiter(interval);
            
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
export async function Log(message: string): Promise<[string, Error | null]> {
    return await AdapterLog(message);
}