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

    const response = await fetch('/file/write', {
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
 * 上传并按原文件名入库为文档资源，返回规范句柄
 *
 * 与电脑端 send.js 的 admitDocumentFile 同语义：正文不进上下文，
 * 消息里只带 @D 句柄，月华按需读取（解析不了的文件同样保留句柄）。
 * @param {File} file - 文件对象
 * @returns {Promise<{handle: string, name: string}>}
 */
export async function admitDocumentFile(file) {
    const fileName = String(file.name || 'file').replace(/[\\/:*?"<>|\u0000-\u001f]/g, '_').trim() || 'file';
    const path = `documents/${fileName}`;
    const saved = await fetch('/file/write', {
        method: 'POST',
        headers: {
            'X-File-Name': encodeFilePath(path),
            'X-Overwrite': 'true',
        },
        body: file,
    });
    if (!saved.ok) throw new Error('文件上传失败');
    // 服务端会把不可读二进制归位到 unknown/，入库必须按实际落点路径
    const savedInfo = await saved.json().catch(() => ({}));
    const storedPath = String(savedInfo.rel_path || path);
    const admitted = await fetch('/file/resource/admit', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ path: storedPath, name: fileName }),
    });
    if (!admitted.ok) throw new Error('资源入库失败');
    const data = await admitted.json();
    const resource = data.resource || {};
    const handle = String(resource.handle || '');
    if (!handle) throw new Error('资源入库未返回句柄');
    return { handle, name: String(resource.name || fileName) };
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
