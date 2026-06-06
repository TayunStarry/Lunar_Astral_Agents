import { createFilePreview, getFileCategory, revokeAllFilePreviews, getVideoThumbnail, formatFileSize } from './file.js';
import { saveFile } from './fetch.js';

/**
 * 文件预览管理器
 *
 * 负责文件的选择、预览渲染、删除、上传处理和清理。
 */
export class FilePreviewManager {
	previews = [];
	previewArea = null;
	showError = null;

	/**
	 * @param {HTMLElement} previewArea - 文件预览区域DOM元素
	 * @param {(msg: string) => void} showError - 错误提示回调
	 */
	constructor(previewArea, showError) {
		this.previewArea = previewArea;
		this.showError = showError;
	}

	/**
	 * 处理文件选择
	 */
	async handleFileSelect(files) {
		for (const file of files) {
			const category = getFileCategory(file);
			if (category === 'other') {
				this.showError(`文件 ${file.name} 不在允许的类型白名单中，将只发送文件名和大小`);
			}
			const preview = createFilePreview(file);
			this.previews.push(preview);
			this.renderFilePreview(preview);
		}
	}

	/**
	 * 渲染文件预览项
	 */
	renderFilePreview(preview) {
		const item = document.createElement('div');
		item.className = 'file-preview-item';
		item.dataset.name = preview.name;

		if (preview.type === 'image') {
			const img = document.createElement('img');
			img.src = preview.url;
			item.appendChild(img);
		} else if (preview.type === 'video') {
			const video = document.createElement('video');
			video.src = preview.url;
			video.muted = true;
			item.appendChild(video);
			getVideoThumbnail(preview.file).then(thumbnail => {
				const img = document.createElement('img');
				img.src = thumbnail;
				item.insertBefore(img, video);
				video.style.display = 'none';
			}).catch(e => {
				console.warn('Failed to get video thumbnail:', e);
			});
		} else {
			const icon = document.createElement('i');
			icon.className = preview.type === 'text' ? 'fas fa-file-alt' : 'fas fa-file';
			icon.style.cssText = 'font-size: 24px; color: white; display: flex; align-items: center; justify-content: center; height: 100%;';
			item.appendChild(icon);
		}

		const label = document.createElement('div');
		label.className = 'file-label';
		label.textContent = preview.name;
		item.appendChild(label);

		const removeBtn = document.createElement('button');
		removeBtn.className = 'remove-btn';
		removeBtn.innerHTML = '<i class="fas fa-times"></i>';
		removeBtn.onclick = () => this.removeFilePreview(preview, item);
		item.appendChild(removeBtn);

		this.previewArea?.appendChild(item);
	}

	/**
	 * 删除文件预览项
	 */
	removeFilePreview(preview, item) {
		const index = this.previews.indexOf(preview);
		if (index > -1) {
			this.previews.splice(index, 1);
		}
		if (preview.url.startsWith('blob:')) {
			URL.revokeObjectURL(preview.url);
		}
		item.remove();
	}

	/**
	 * 清理所有文件预览
	 */
	clearFilePreviews() {
		revokeAllFilePreviews(this.previews);
		this.previews = [];
		if (this.previewArea) {
			this.previewArea.innerHTML = '';
		}
	}

	/**
	 * 处理文件上传，返回上传结果
	 */
	async processFileUpload(preview) {
		const saveResult = await saveFile(preview.file, true);
		const fileUrl = `${window.location.origin}/file/read/${saveResult.filename}`;
		const category = getFileCategory(preview.file);

		return {
			fileUrl,
			category,
			fileName: preview.name,
			fileSize: preview.file.size
		};
	}

	/**
	 * 构建文件内容块（用于OpenAI消息格式）
	 */
	async buildFileContentBlocks(fileResults) {
		const contentBlocks = [];
		const userContentParts = [];
		const uploadedFileUrls = [];

		for (const res of fileResults) {
			uploadedFileUrls.push(res.fileUrl);

			if (res.category === 'text') {
				const file = this.previews.find(p => p.name === res.fileName)?.file;
				if (file) {
					try {
						const rawText = await file.text();
						const MAX_TEXT_LEN = 50000;
						let textContent = rawText;
						if (rawText.length > MAX_TEXT_LEN) {
							textContent = rawText.slice(0, MAX_TEXT_LEN) + '\n\n[文件内容过长，已截断]';
							this.showError(`文件 ${res.fileName} 内容超过限制，仅截取前 ${MAX_TEXT_LEN} 字符`);
						}
						const textBlock = `【文件 ${res.fileName}】\n内容：\n\`\`\`\n${textContent}\n\`\`\`\n访问链接：${res.fileUrl}`;
						contentBlocks.push({ type: 'text', text: textBlock });
						userContentParts.push(`[文本文件: ${res.fileName}]`);
					} catch (err) {
						console.error(`读取文本文件失败: ${res.fileName}`, err);
						this.showError(`无法读取文件 ${res.fileName}，未加入上下文`);
						contentBlocks.push({ type: 'text', text: `【文件 ${res.fileName}】无法读取内容，访问链接：${res.fileUrl}` });
						userContentParts.push(`[无法读取: ${res.fileName}]`);
					}
				}
			} else if (res.category === 'image') {
				contentBlocks.push({ type: 'image_url', image_url: { url: res.fileUrl } });
				userContentParts.push(`[图片: ${res.fileName}]`);
			} else if (res.category === 'video') {
				contentBlocks.push({ type: 'image_url', image_url: { url: res.fileUrl } });
				userContentParts.push(`[视频: ${res.fileName}]`);
			} else {
				const fileSize = formatFileSize(res.fileSize);
				contentBlocks.push({ type: 'text', text: `【文件 ${res.fileName}】\n大小：${fileSize}\n访问链接：${res.fileUrl}` });
				userContentParts.push(`[文件: ${res.fileName} (${fileSize})]`);
			}
		}

		return { contentBlocks, userContentParts, uploadedFileUrls };
	}
}
