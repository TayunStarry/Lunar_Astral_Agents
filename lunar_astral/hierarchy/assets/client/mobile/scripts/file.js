import { encodeFilePath } from './util.js';

const ALLOWED_TEXT_EXTENSIONS = [
    'txt', 'md', 'json', 'xml', 'yaml', 'yml', 'toml', 'csv',
    'html', 'htm', 'css', 'scss', 'js', 'ts', 'jsx', 'tsx',
    'php', 'py', 'rb', 'pl', 'sh', 'bash', 'ps1', 'bat', 'cmd', 'r',
    'java', 'c', 'cpp', 'h', 'hpp', 'go', 'rs', 'swift', 'kt', 'dart', 'lua', 'sql',
    'ini', 'cfg', 'conf', 'properties', 'gitignore', 'dockerignore', 'editorconfig', 'code-workspace',
    'log', 'rtf'
];

const ALLOWED_TEXT_MIME_TYPES = [
    'text/plain', 'text/html', 'text/css', 'text/javascript', 'text/markdown',
    'text/xml', 'text/csv', 'text/calendar', 'text/yaml', 'text/x-yaml',
    'application/json', 'application/javascript', 'application/xml', 'application/yaml',
    'application/typescript', 'application/x-httpd-php', 'application/rtf'
];

const ALLOWED_IMAGE_EXTENSIONS = ['jpg', 'jpeg', 'png', 'gif', 'bmp', 'webp', 'svg'];
const ALLOWED_VIDEO_EXTENSIONS = ['mp4', 'webm', 'ogg', 'mov', 'avi', 'flv'];
const ALLOWED_AUDIO_EXTENSIONS = ['wav', 'mp3', 'm4a', 'flac', 'aac', 'opus', 'weba', 'wma', 'aiff', 'oga'];

export function isAllowedTextFile(file) {
    if (ALLOWED_TEXT_MIME_TYPES.includes(file.type)) return true;
    const ext = file.name.slice(file.name.lastIndexOf('.') + 1).toLowerCase();
    return ALLOWED_TEXT_EXTENSIONS.includes(ext);
}

export function isAllowedImageFile(file) {
    if (file.type.startsWith('image/')) return true;
    const ext = file.name.slice(file.name.lastIndexOf('.') + 1).toLowerCase();
    return ALLOWED_IMAGE_EXTENSIONS.includes(ext);
}

export function isAllowedVideoFile(file) {
    if (file.type.startsWith('video/')) return true;
    const ext = file.name.slice(file.name.lastIndexOf('.') + 1).toLowerCase();
    return ALLOWED_VIDEO_EXTENSIONS.includes(ext);
}

export function isAllowedAudioFile(file) {
    if (file.type.startsWith('audio/')) return true;
    const ext = file.name.slice(file.name.lastIndexOf('.') + 1).toLowerCase();
    return ALLOWED_AUDIO_EXTENSIONS.includes(ext);
}

export function getFileCategory(file) {
    if (isAllowedImageFile(file)) return 'image';
    if (isAllowedVideoFile(file)) return 'video';
    if (isAllowedAudioFile(file)) return 'audio';
    if (isAllowedTextFile(file)) return 'text';
    return 'other';
}

export function createFilePreview(file) {
    return { file, url: URL.createObjectURL(file), type: getFileCategory(file), name: file.name };
}

export function getFileType(file) {
    if (file.type.startsWith('image/')) return 'image';
    if (file.type.startsWith('video/')) return 'video';
    if (file.type.startsWith('audio/')) return 'audio';
    return 'text';
}

export function isMediaFile(file) {
    return file.type.startsWith('image/') || file.type.startsWith('video/') || file.type.startsWith('audio/');
}

export function isTextFile(file) {
    if (ALLOWED_TEXT_MIME_TYPES.includes(file.type)) return true;
    const ext = file.name.slice(file.name.lastIndexOf('.') + 1).toLowerCase();
    return ALLOWED_TEXT_EXTENSIONS.includes(ext);
}

export function getStoragePrefix(file) {
    return isMediaFile(file) ? 'images/' : 'documents/';
}

export function revokeFilePreview(preview) {
    if (preview.url.startsWith('blob:')) {
        URL.revokeObjectURL(preview.url);
    }
}

export function revokeAllFilePreviews(previews) {
    previews.forEach(revokeFilePreview);
}

export async function getVideoThumbnail(file) {
    return new Promise((resolve, reject) => {
        const video = document.createElement('video');
        video.preload = 'metadata';
        video.muted = true;
        video.onloadeddata = () => { video.currentTime = 1; };
        video.onseeked = () => {
            const canvas = document.createElement('canvas');
            canvas.width = video.videoWidth;
            canvas.height = video.videoHeight;
            const ctx = canvas.getContext('2d');
            if (ctx) {
                ctx.drawImage(video, 0, 0);
                resolve(canvas.toDataURL('image/jpeg'));
            } else {
                reject(new Error('Failed to get video context'));
            }
        };
        video.onerror = () => { reject(new Error('Failed to load video')); };
        video.src = URL.createObjectURL(file);
    });
}

export async function calculateFileHash(file) {
    return new Promise((resolve) => {
        const reader = new FileReader();
        reader.onload = async function (e) {
            try {
                const arrayBuffer = e.target?.result;
                const hashBuffer = await crypto.subtle.digest('SHA-256', arrayBuffer);
                const hashArray = Array.from(new Uint8Array(hashBuffer));
                const fullHash = hashArray.map(b => b.toString(16).padStart(2, '0')).join('');
                const shortHash = fullHash.substring(0, 16);
                resolve(shortHash);
            } catch {
                resolve(encodeFilePath(file.name).slice(-16));
            }
        };
        reader.onerror = async (error) => {
            if (error instanceof Error) {
                console.error(error.message);
            }
            resolve(encodeFilePath(file.name).slice(-16));
        };
        reader.readAsArrayBuffer(file);
    });
}

export function formatFileSize(bytes) {
    if (bytes === 0) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
}

/**
 * 根据文件名扩展名推断音频 MIME 类型
 * @param {File} file - 文件对象
 * @returns {string} - MIME 类型，默认 audio/wav
 */
export function getAudioMimeType(file) {
    if (file.type && file.type.startsWith('audio/')) return file.type;
    const ext = file.name.slice(file.name.lastIndexOf('.') + 1).toLowerCase();
    const mimeMap = {
        wav: 'audio/wav',
        mp3: 'audio/mpeg',
        m4a: 'audio/mp4',
        flac: 'audio/flac',
        aac: 'audio/aac',
        opus: 'audio/opus',
        weba: 'audio/webm',
        wma: 'audio/x-ms-wma',
        aiff: 'audio/aiff',
        oga: 'audio/ogg',
    };
    return mimeMap[ext] || 'audio/wav';
}

/**
 * 获取 llama.cpp input_audio 所需的格式标识
 *
 * llama.cpp 的 OpenAI 兼容端点严格要求 input_audio.format 为 "wav" 或 "mp3"，
 * 其他格式会触发 "input_audio.format must be either 'wav' or 'mp3'" 错误。
 *
 * @param {File} file - 文件对象
 * @returns {'wav' | 'mp3' | null} - 返回 'wav'/'mp3'，不支持的格式返回 null
 */
export function getAudioFormat(file) {
    const ext = file.name.slice(file.name.lastIndexOf('.') + 1).toLowerCase();
    if (ext === 'wav') return 'wav';
    if (ext === 'mp3') return 'mp3';
    return null;
}

/**
 * 将文件读取为 base64 编码的 data URL
 * @param {File} file - 文件对象
 * @param {string} [mimeType] - 可选 MIME 类型，默认根据文件推断
 * @returns {Promise<string>} - data URL，格式为 data:<mime>;base64,<base64>
 */
export function fileToBase64DataURL(file, mimeType) {
    return new Promise((resolve, reject) => {
        const reader = new FileReader();
        reader.onload = () => {
            const result = reader.result;
            if (typeof result !== 'string') {
                reject(new Error('Failed to read file as data URL'));
                return;
            }
            // 若已有正确的 data URL 前缀，直接返回；否则按指定 MIME 重建
            if (result.startsWith('data:')) {
                if (mimeType) {
                    // 替换前缀中的 MIME 类型为指定值
                    const commaIdx = result.indexOf(',');
                    const base64 = result.slice(commaIdx + 1);
                    resolve(`data:${mimeType};base64,${base64}`);
                } else {
                    resolve(result);
                }
            } else {
                reject(new Error('Unexpected file read result'));
            }
        };
        reader.onerror = () => reject(reader.error || new Error('FileReader error'));
        reader.readAsDataURL(file);
    });
}

/**
 * 将文件读取为纯 base64 字符串（不带 data URL 前缀）
 *
 * 用于 llama.cpp 的 input_audio 类型，该类型期望 input_audio.data 字段
 * 直接为原始 base64 编码字符串，不能包含 "data:audio/xxx;base64," 前缀。
 *
 * @param {File} file - 文件对象
 * @returns {Promise<string>} - 纯 base64 字符串
 */
export function fileToRawBase64(file) {
    return new Promise((resolve, reject) => {
        const reader = new FileReader();
        reader.onload = () => {
            const result = reader.result;
            if (typeof result !== 'string') {
                reject(new Error('Failed to read file as data URL'));
                return;
            }
            if (!result.startsWith('data:')) {
                reject(new Error('Unexpected file read result'));
                return;
            }
            const commaIdx = result.indexOf(',');
            if (commaIdx < 0) {
                reject(new Error('Invalid data URL format'));
                return;
            }
            resolve(result.slice(commaIdx + 1));
        };
        reader.onerror = () => reject(reader.error || new Error('FileReader error'));
        reader.readAsDataURL(file);
    });
}