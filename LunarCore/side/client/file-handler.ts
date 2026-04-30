import { FilePreview, SaveFileResponse } from './types';
import { encodeFileName } from './utils';

/**
 * 创建文件预览对象
 * 
 * @param {File} file - 文件对象
 * 
 * @returns {FilePreview} - 文件预览对象
 */
export function createFilePreview(file: File): FilePreview {
	return { file, url: URL.createObjectURL(file), type: getFileType(file), name: file.name };
}

/**
 * 获取文件类型
 * 
 * @param {File} file - 文件对象
 * 
 * @returns {'image' | 'video' | 'text'} - 文件类型
 */
export function getFileType(file: File): 'image' | 'video' | 'text' {
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
export function isMediaFile(file: File): boolean {
	return file.type.startsWith('image/') || file.type.startsWith('video/');
}

/**
 * 撤销文件预览对象的URL
 * 
 * @param {FilePreview} preview - 文件预览对象
 */
export function revokeFilePreview(preview: FilePreview): void {
	if (preview.url.startsWith('blob:')) {
		URL.revokeObjectURL(preview.url);
	}
}

/**
 * 撤销所有文件预览对象的URL
 * 
 * @param {FilePreview[]} previews - 文件预览对象数组
 */
export function revokeAllFilePreviews(previews: FilePreview[]): void {
	previews.forEach(revokeFilePreview);
}

/**
 * 获取视频缩略图
 * 
 * @param {File} file - 视频文件对象
 * 
 * @returns {Promise<string>} - 视频缩略图的Base64编码字符串
 */
export async function getVideoThumbnail(file: File): Promise<string> {
	function execute(resolve: (value: string) => void, reject: (reason?: any) => void) {
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
			}
			else reject(new Error('Failed to get video context'));
		};
		video.onerror = () => { reject(new Error('Failed to load video')); };
		video.src = URL.createObjectURL(file);
	}
	return new Promise(execute);
}

/**
 * 保存文件到服务器
 * 
 * @param {File} file - 文件对象
 * @param {boolean} [overwrite=false] - 是否覆盖已存在文件
 * 
 * @returns {Promise<SaveFileResponse>} - 保存文件的响应对象
 */
export async function saveFile(file: File, overwrite: boolean = false): Promise<SaveFileResponse> {
	const encodedFileName = encodeFileName(file.name);
	const response = await fetch('/save',
		{
			method: 'POST',
			headers: {
				'X-File-Name': encodedFileName,
				'X-Overwrite': overwrite.toString(),
				'Content-Length': file.size.toString(),
			},
			body: file,
		}
	);
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
export async function sendMessages(messages: { role: string; content: unknown }[]): Promise<{ success: boolean; length: number }> {
	const response = await fetch('/write/message',
		{
			method: 'POST',
			headers: {
				'Content-Type': 'application/json',
			},
			body: JSON.stringify({ messages }),
		}
	);
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
export async function fetchLive2DSetting(): Promise<{ name?: string; url?: string; scale?: number; x?: number; y?: number; autoInteract?: boolean }> {
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
