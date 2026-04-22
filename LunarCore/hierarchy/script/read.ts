import { calculateFileHash, toBtoaString } from '../index';
import { SaveFile, ReadFile, GetFileList } from '../../config/index';

/** 文件列表项属性 */
export interface FileListItem {
    /** 文件或目录名称 */
    name: string;
    /** 文件大小（字节） */
    size: number;
    /** 是否为目录 */
    isDir: boolean;
    /** 最后修改时间，格式：YYYY-MM-DD HH:mm:ss */
    lastModified: string;
    /** 文件的完整路径 */
    path: string;
}

/**
 * 同步从指定路径读取文件内容，并对内容进行格式化处理
 *
 * 支持控制是否移除换行符，最终会将多个连续空白字符替换为单个空格
 *
 * @param {string} path - 文件的路径
 *
 * @param {boolean} [removeNewLines=false] - 是否剔除换行符，默认不剔除
 *
 * @returns {string} - 解析并格式化后的文件内容
 */
export function getFileContent(path: string, removeNewLines: boolean = false): string {
    /** 从磁盘读取文件内容 */
    let [content, size, mimeType, err] = ReadFile(path);
    // 检查读取是否成功
    if (err) throw err;
    // 根据参数决定是否移除换行符
    if (removeNewLines) return String(content).replace(/[\r\n]+/g, '');
    // 将多个连续的空格或制表符替换为单个空格，并返回处理结果
    return String(content).replace(/[ \t]+/g, ' ');
};

/**
 * 异步函数，用于将图片文件保存到服务器，使用内容哈希作为文件名
 *
 * @param {File} file - 需要保存的图片文件对象
 *
 * @returns {Promise<string>} - 保存成功后返回图片的读取路径，失败则抛出错误
 */
export async function saveImageToServer(file: File): Promise<string> {
    try {
        /** 计算文件的SHA-256哈希值（取前16个字符，保持较短长度） */
        const fileHash = await calculateFileHash(file);
        /** 获取文件扩展名 */
        const fileExtension = file.name.slice(file.name.lastIndexOf('.')).toLowerCase();
        /** 使用哈希值 + 扩展名作为新文件名 */
        const newFileName = `${fileHash}${fileExtension}`;
        /** 将包含图片文件名的路径进行 Base64 编码，用于设置请求头中的文件名 */
        const base64FileName = toBtoaString('images/' + newFileName);
        /** 向服务器发送 POST 请求，尝试保存图片文件 */
        const [_, __, err] = SaveFile(base64FileName, true, file);
        // 检查响应是否成功，若失败则抛出错误
        if (!err) throw err;
        // 保存成功，返回图片的读取路径
        return `/read/images/${newFileName}`;
    }
    catch (error) {
        if (!(error instanceof Error)) return '';
        // 保存失败，返回空字符串
        return `${error.name} | ${error.message} | ${error.stack}`;
    }
};

/**
 * 异步尝试获取文件内容并调用回调函数处理
 *
 * @param {RequestInfo | URL} url - 文件的 URL 地址
 *
 * @param {string} [initializeContent='{}'] - 初始化内容，默认值为空 JSON 字符串
 *
 * @param {(content: string) => any} [callback] - 处理文件内容的回调函数，可选，默认使用 JSON.parse
 *
 * @returns {Promise<any>} - 回调函数处理后的结果
 *
 * @throws {Error} - 当获取 文件内容失败或回调函数处理出错时抛出错误
 */
export async function fetchDocumentCallback(url: RequestInfo | URL, initializeContent: string = '{}', callback?: (content: string) => any): Promise<any> {
    /** 默认回调函数：尝试将文本解析为 JSON */
    const defaultCallback = (content: string) => JSON.parse(content);
    /** 应用回调函数，默认使用默认回调 */
    const applyCallback = callback ?? defaultCallback;
    /** 统一兜底逻辑：当文件不存在或读取失败时，保存默认内容并返回 */
    const fallback = async () => {
        SaveFile(url.toString(), true, initializeContent)
        return applyCallback(initializeContent);
    };
    try {
        /** 拆分文件路径 */
        const filePath = url.toString().split(/[\/\\]/);
        /** 获取文件列表 */
        const [fileList, err1] = GetFileList(filePath.slice(0, -1).join('/'));
        // 检查文件列表响应是否成功
        if (!err1) return await fallback();
        /** 检查文件是否存在且不是目录 */
        const exists = fileList.some(item => item.name === filePath[filePath.length - 1] && !item.isDir);
        // 检查文件是否存在
        if (!exists) return await fallback();
        /** 读取文件内容 */
        const [content, size, mimeType, err2] = ReadFile(url.toString());
        // 检查文件内容响应是否成功
        if (!err2) return await fallback();
        /** 解析文件内容为文本 */
        const text = String(content);
        // 检查文件内容是否为空
        if (!text) return await fallback();
        // 执行回调函数处理文件内容
        return applyCallback(text);
    }
    // 任何异常都走兜底逻辑
    catch (error) {
        if (error instanceof Error) return await fallback();
    }
};