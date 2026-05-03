import { calculateFileHash, getStoragePrefix } from './file.js';
import { encodeFilePath } from './util.js';

/**
 * 保存文件到服务器
 * @param {File} file - 文件对象
 * @param {boolean} [overwrite=false] - 是否覆盖已存在文件
 * @returns {Promise<SaveFileResponse>} - 保存文件的响应对象
 */
export async function saveFile(file, overwrite = false) {
    const prefix = getStoragePrefix(file);
    const fileHash = await calculateFileHash(file);
    const fileExtension = file.name.slice(file.name.lastIndexOf('.')).toLowerCase();
    const newFileName = `${fileHash}${fileExtension}`;
    const encodedFileName = encodeFilePath(prefix + newFileName);

    const response = await fetch('/save', {
        method: 'POST',
        headers: {
            'X-File-Name': encodedFileName,
            'X-Overwrite': overwrite.toString(),
        },
        body: file,
    });

    if (!response.ok) {
        const errorData = await response.json().catch(() => ({ message: 'Upload failed' }));
        throw new Error(errorData.message || `HTTP error! status: ${response.status}`);
    }
    return response.json();
}

/**
 * 发送消息到服务器
 *
 * @param {Array<{ role: string; content: unknown }>} messages - 消息数组
 *
 * @returns {Promise<{ success: boolean; length: number }>} - 服务器响应对象
 */
export async function sendMessages(messages) {
    const response = await fetch('/write/message', {
        method: 'POST',
        headers: {
            'Content-Type': 'application/json',
        },
        body: JSON.stringify({ messages }),
    });

    if (!response.ok) {
        const errorData = await response.json().catch(() => ({ message: 'Request failed' }));
        throw new Error(errorData.message || `HTTP error! status: ${response.status}`);
    }
    return response.json();
}

/**
 * 从服务器获取Live2D设置
 *
 * @returns {Promise<{ name?: string; url?: string; scale?: number; x?: number; y?: number; autoInteract?: boolean }>} - Live2D设置对象
 */
export async function fetchLive2DSetting() {
    try {
        const response = await fetch('/read/resources/live2d/setting.json');
        if (!response.ok) {
            throw new Error(`HTTP error! status: ${response.status}`);
        }
        const rawText = await response.text();
        const jsonText = rawText
            .replace(/\/\/.*$/gm, '')
            .replace(/\/\*[\s\S]*?\*\//g, '')
            .replace(/'/g, '"');
        const setting = JSON.parse(jsonText);
        return setting;
    }
    catch (error) {
        console.error('Failed to fetch Live2D setting:', error);
        return {};
    }
}