/**
 * 移动端文件能力：分类、哈希、大小、音频格式与 base64 来自共享层，
 * 这里只保留移动端独有的缩略图 / 预览 / data URL 读取。
 */
import {
	getFileCategory as sharedGetFileCategory,
	getStoragePrefix as sharedGetStoragePrefix,
	calculateFileHash as sharedCalculateFileHash,
	getAudioFormat as sharedGetAudioFormat,
	formatFileSize as sharedFormatFileSize
} from '/shared/core/files.js';

export { fileToRawBase64 } from '/shared/core/utils.js';

/** 移动端沿用 16 位短哈希命名 */
export function calculateFileHash(file) {
	return sharedCalculateFileHash(file, 16);
}

/** 移动端沿用两位小数、不取整的显示习惯 */
export function formatFileSize(bytes) {
	return sharedFormatFileSize(bytes, { precision: 2, compact: false });
}

export const getFileCategory = sharedGetFileCategory;
export const getStoragePrefix = sharedGetStoragePrefix;
export const getAudioFormat = sharedGetAudioFormat;

/** 纯 MIME 分类（移动端文件选择器的类型标记） */
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
	return sharedGetFileCategory(file) === 'text';
}

export function isAllowedTextFile(file) {
	return sharedGetFileCategory(file) === 'text';
}

export function isAllowedImageFile(file) {
	return sharedGetFileCategory(file) === 'image';
}

export function isAllowedVideoFile(file) {
	return sharedGetFileCategory(file) === 'video';
}

export function isAllowedAudioFile(file) {
	return sharedGetFileCategory(file) === 'audio';
}

export function createFilePreview(file) {
	return { file, url: URL.createObjectURL(file), type: sharedGetFileCategory(file), name: file.name };
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

/**
 * 根据文件名扩展名推断音频 MIME 类型
 * @param {File} file
 * @returns {string}
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
		oga: 'audio/ogg'
	};
	return mimeMap[ext] || 'audio/wav';
}

/** 将文件读取为 base64 编码的 data URL，可按指定 MIME 重建前缀 */
export function fileToBase64DataURL(file, mimeType) {
	return new Promise((resolve, reject) => {
		const reader = new FileReader();
		reader.onload = () => {
			const result = reader.result;
			if (typeof result !== 'string') {
				reject(new Error('Failed to read file as data URL'));
				return;
			}
			if (result.startsWith('data:')) {
				if (mimeType) {
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
