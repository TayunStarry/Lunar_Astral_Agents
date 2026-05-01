import { encodeFilePath } from './utils.js';

/**
 * 创建文件预览对象
 *
 * @param {File} file - 文件对象
 *
 * @returns {FilePreview} - 文件预览对象
 */
export function createFilePreview(file) {
	return { file, url: URL.createObjectURL(file), type: getFileType(file), name: file.name };
}

/**
 * 获取文件类型
 *
 * @param {File} file - 文件对象
 *
 * @returns {'image' | 'video' | 'text'} - 文件类型
 */
export function getFileType(file) {
	if (file.type.startsWith('image/')) return 'image';
	if (file.type.startsWith('video/')) return 'video';
	return 'text';
}

/**
 * 判断文件是否为媒体文件（图片或视频）
 *
 * @param {File} file - 文件对象
 *
 * @returns {boolean} - 是否为媒体文件
 */
export function isMediaFile(file) {
	return file.type.startsWith('image/') || file.type.startsWith('video/');
}

/**
 * 判断文件是否为纯文本类文件（通过 MIME 或扩展名）
 * @param {File} file - 文件对象
 * @returns {boolean}
 */
export function isTextFile(file) {
	/** 常见文本 MIME 类型 */
	const textMimeTypes = [
		'text/plain', 'text/html', 'text/css', 'text/javascript', 'text/markdown',
		'text/xml', 'text/csv', 'text/calendar', 'text/yaml', 'text/x-yaml',
		'application/json', 'application/javascript', 'application/xml', 'application/yaml',
		'application/typescript', 'application/x-httpd-php', 'application/rtf'
	];
	
	if (textMimeTypes.includes(file.type)) return true;
	
	/** 配置文件扩展名列表 */
	const configExtensions = ['ini', 'cfg', 'conf', 'properties', 'gitignore', 'dockerignore', 'editorconfig', 'code-workspace'];
	/** 数据文件扩展名列表 */
	const dataExtensions = ['json', 'xml', 'yaml', 'yml', 'toml', 'csv'];
	/** 文档文件扩展名列表 */
	const docExtensions = ['txt', 'md', 'log'];
	/** Web前端文件扩展名列表 */
	const webExtensions = ['html', 'htm', 'css', 'scss', 'js', 'ts', 'jsx', 'tsx'];
	/** 后端/脚本语言文件扩展名列表 */
	const scriptExtensions = ['php', 'py', 'rb', 'pl', 'sh', 'bash', 'ps1', 'bat', 'cmd', 'r'];
	/** 编译型语言文件扩展名列表 */
	const compiledExtensions = ['java', 'c', 'cpp', 'h', 'hpp', 'go', 'rs', 'swift', 'kt', 'dart', 'lua', 'sql'];
	/** 所有文本文件扩展名列表 */
	const textExtensions = [...configExtensions, ...dataExtensions, ...docExtensions, ...webExtensions, ...scriptExtensions, ...compiledExtensions];
	
	const fileExtension = file.name.slice(file.name.lastIndexOf('.') + 1).toLowerCase();
	return textExtensions.includes(fileExtension);
}

/**
 * 根据文件类型决定存储目录前缀
 * @param {File} file - 文件对象
 * @returns {'document/' | 'images/'}
 */
export function getStoragePrefix(file) {
	return isMediaFile(file) ? 'images/' : 'document/';
}

/**
 * 撤销文件预览对象的URL
 *
 * @param {FilePreview} preview - 文件预览对象
 */
export function revokeFilePreview(preview) {
	if (preview.url.startsWith('blob:')) {
		URL.revokeObjectURL(preview.url);
	}
}

/**
 * 撤销所有文件预览对象的URL
 *
 * @param {FilePreview[]} previews - 文件预览对象数组
 */
export function revokeAllFilePreviews(previews) {
	previews.forEach(revokeFilePreview);
}

/**
 * 获取视频缩略图
 *
 * @param {File} file - 视频文件对象
 *
 * @returns {Promise<string>} - 视频缩略图的Base64编码字符串
 */
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

/**
 * 计算文件的SHA-256哈希值（16位）
 * @param {File} file - 文件对象
 * @returns {Promise<string>} - 文件的SHA-256哈希值（16位）
 */
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