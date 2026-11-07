/**
 * 星月智能 · 桌面端 / 移动端共享文件能力
 *
 * 统一两端的文件分类、哈希命名与保存。
 * 仅移动端使用的缩略图能力留在 mobile/scripts/file.js。
 */
import { ENDPOINTS } from './constants.js';
import { encodeFilePath, formatFileSize } from './utils.js';

/** 文本类扩展名（两端并集） */
export const TEXT_EXTENSIONS = [
	'txt', 'md', 'markdown', 'json', 'xml', 'yaml', 'yml', 'csv', 'log', 'rtf',
	'js', 'mjs', 'cjs', 'ts', 'tsx', 'jsx', 'css', 'scss', 'less', 'html', 'htm',
	'go', 'rs', 'c', 'h', 'cpp', 'hpp', 'cc', 'cxx', 'cs', 'java', 'kt', 'kts', 'swift', 'dart',
	'py', 'pyw', 'rb', 'php', 'pl', 'lua', 'sh', 'bash', 'ps1', 'bat', 'cmd',
	'sql', 'toml', 'ini', 'conf', 'cfg', 'properties', 'env', 'gradle', 'cmake', 'dockerfile',
	'gitignore', 'dockerignore', 'editorconfig', 'code-workspace'
];

/** 图片扩展名 */
export const IMAGE_EXTENSIONS = ['jpg', 'jpeg', 'png', 'gif', 'webp', 'bmp', 'svg', 'avif', 'ico', 'tif', 'tiff', 'heic', 'heif'];

/** 视频扩展名 */
export const VIDEO_EXTENSIONS = ['mp4', 'mov', 'avi', 'mkv', 'flv', 'wmv', 'm4v', 'mpeg', 'mpg', '3gp'];

/** 音频扩展名 */
export const AUDIO_EXTENSIONS = ['mp3', 'wav', 'ogg', 'oga', 'm4a', 'aac', 'flac', 'opus', 'weba', 'webm', 'amr', 'wma', 'aiff'];

/** 取小写扩展名（不含点） */
export function getFileExtension(fileName) {
	const name = String(fileName || '');
	const idx = name.lastIndexOf('.');
	return idx >= 0 ? name.slice(idx + 1).toLowerCase() : '';
}

/** 文件分类：image / video / audio / text / other */
export function getFileCategory(file) {
	if (!file) return 'other';
	const mime = String(file.type || '').toLowerCase();
	const ext = getFileExtension(file.name);
	if (mime.startsWith('image/') || IMAGE_EXTENSIONS.includes(ext)) return 'image';
	if (mime.startsWith('video/') || VIDEO_EXTENSIONS.includes(ext)) return 'video';
	if (mime.startsWith('audio/') || AUDIO_EXTENSIONS.includes(ext)) return 'audio';
	if (mime.startsWith('text/') || TEXT_EXTENSIONS.includes(ext)) return 'text';
	return 'other';
}

/** 存储目录（带结尾斜杠，与 X-File-Name 路径拼接一致） */
export function getStoragePrefix(categoryOrFile) {
	const category = typeof categoryOrFile === 'string'
		? categoryOrFile
		: getFileCategory(categoryOrFile);
	return category === 'image' || category === 'video' || category === 'audio'
		? 'multimedia/'
		: 'documents/';
}

/** 音频格式标识：音频不再落盘，任何格式都以内联音频下发，直接取扩展名 */
export function getAudioFormat(file) {
	const ext = getFileExtension(file?.name);
	return ext || 'wav';
}

/** 计算 SHA-256 摘要；length>0 时只取前 N 位（两端历史命名均为 16 位） */
export async function calculateFileHash(file, length = 0) {
	try {
		const buffer = await file.arrayBuffer();
		const digest = await crypto.subtle.digest('SHA-256', buffer);
		const fullHash = Array.from(new Uint8Array(digest), b => b.toString(16).padStart(2, '0')).join('');
		return length > 0 ? fullHash.slice(0, length) : fullHash;
	} catch {
		const fallback = encodeFilePath(file.name).replace(/[^a-z0-9]/gi, '');
		return length > 0 ? fallback.slice(-length) : fallback;
	}
}

/**
 * 以「内容哈希 + 原扩展名」写入文件
 * @param {File} file
 * @param {{overwrite?:boolean, hashLength?:number}} [options]
 * @returns {Promise<{relPath:string, url:string, hash:string, fileName:string, response:object}>}
 */
export async function writeHashedFile(file, { overwrite = false, hashLength = 16 } = {}) {
	const hash = await calculateFileHash(file, hashLength);
	const ext = file.name.includes('.') ? file.name.slice(file.name.lastIndexOf('.')).toLowerCase() : '.bin';
	const fileName = `${hash}${ext}`;
	const relPath = `${getStoragePrefix(file)}${fileName}`;
	const headers = { 'X-File-Name': encodeFilePath(relPath) };
	if (overwrite) headers['X-Overwrite'] = 'true';
	const resp = await fetch(ENDPOINTS.writeFile, { method: 'POST', headers, body: file });
	if (!resp.ok) throw new Error('文件上传失败');
	const response = await resp.json().catch(() => ({}));
	// 二进制会被服务端归位到 unknown/，读取地址必须用实际落点路径
	const finalPath = String(response.rel_path || response.filename || relPath);
	return {
		relPath: finalPath,
		url: `/file/read/${finalPath}`,
		hash,
		fileName,
		response
	};
}

export { formatFileSize };
