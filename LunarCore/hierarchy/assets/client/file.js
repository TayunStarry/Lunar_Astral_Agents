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

export function getFileCategory(file) {
    if (isAllowedImageFile(file)) return 'image';
    if (isAllowedVideoFile(file)) return 'video';
    if (isAllowedTextFile(file)) return 'text';
    return 'other';
}

export function createFilePreview(file) {
    return { file, url: URL.createObjectURL(file), type: getFileCategory(file), name: file.name };
}

export function getFileType(file) {
    if (file.type.startsWith('image/')) return 'image';
    if (file.type.startsWith('video/')) return 'video';
    return 'text';
}

export function isMediaFile(file) {
    return file.type.startsWith('image/') || file.type.startsWith('video/');
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