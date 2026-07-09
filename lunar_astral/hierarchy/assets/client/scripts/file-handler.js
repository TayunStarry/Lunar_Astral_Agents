import { createFilePreview, getFileCategory, revokeAllFilePreviews, getVideoThumbnail, formatFileSize, fileToRawBase64, getAudioFormat } from './file.js';
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
		}
		else if (preview.type === 'video') {
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
		}
		else if (preview.type === 'audio') {
			const icon = document.createElement('i');
			icon.className = 'fas fa-music';
			icon.style.cssText = 'font-size: 24px; color: white; display: flex; align-items: center; justify-content: center; height: 100%;';
			item.appendChild(icon);
		}
		else {
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
	 *
	 * 注意：音频文件不会上传到服务器，而是后续在 buildFileContentBlocks 中
	 * 直接以 base64 编码形式嵌入到消息内容中。
	 *
	 * 关键：音频的 previewUrl 使用独立创建的 blob URL，不复用 preview.url，
	 * 否则发送消息后 clearFilePreviews() 撤销 preview.url 会导致历史记录中
	 * 的音频播放器无法播放。
	 */
	async processFileUpload(preview) {
		const category = getFileCategory(preview.file);

		// 音频文件：跳过服务器上传，仅返回元数据，base64 编码在内容块构建阶段完成
		if (category === 'audio') {
			// 为历史记录中的播放器创建独立的 blob URL，生命周期与消息绑定而非与预览绑定
			const independentBlobUrl = URL.createObjectURL(preview.file);
			console.log(`[Audio:processFileUpload] fileName=${preview.name}, fileSize=${preview.file.size}字节, mime=${preview.file.type || '(unknown)'}, blobUrl=${independentBlobUrl}`);
			return {
				fileUrl: null,
				category,
				fileName: preview.name,
				fileSize: preview.file.size,
				previewUrl: independentBlobUrl,
			};
		}

		const saveResult = await saveFile(preview.file, true);
		const fileUrl = `${window.location.origin}/file/read/${saveResult.filename}`;

		return {
			fileUrl,
			category,
			fileName: preview.name,
			fileSize: preview.file.size
		};
	}

	/**
	 * 构建文件内容块（用于OpenAI消息格式）
	 *
	 * 音频文件使用 input_audio 类型，input_audio.data 字段为纯 base64 字符串
	 * （不带 "data:audio/xxx;base64," 前缀）。
	 *
	 * 这是 llama.cpp OpenAI 兼容 API 的硬性要求：
	 * - image_url 类型只接受 data:image/* 前缀的 data URL，audio MIME 会触发
	 *   "Invalid uri format" 错误
	 * - input_audio 类型期望 input_audio.data 为原始 base64 字符串
	 *   （accept_base64_uri=false，跳过 data URI 解析，直接 base64 解码）
	 *
	 * 输出格式：
	 * { type: 'input_audio', input_audio: { data: '<纯base64>' } }
	 */
	async buildFileContentBlocks(fileResults) {
		const contentBlocks = [];
		const userContentParts = [];
		const uploadedFileUrls = [];
		const audioPreviewUrls = [];

		for (const res of fileResults) {
			if (res.category === 'audio') {
				// 音频文件：读取为纯 base64 后以 input_audio 形式加入消息内容
				const file = this.previews.find(p => p.name === res.fileName)?.file;
				if (file) {
					try {
						const base64Data = await fileToRawBase64(file);
						// 获取音频格式：llama.cpp 严格要求 input_audio.format 为 "wav" 或 "mp3"
						const audioFormat = getAudioFormat(file);
						// 校验：必须是纯 base64 字符串，不能包含 "data:" 前缀（llama.cpp 会直接 base64_decode）
						const hasDataPrefix = base64Data.startsWith('data:');
						const headPreview = base64Data.slice(0, 32);
						const tailPreview = base64Data.slice(-16);
						console.log(`[Audio:buildFileContentBlocks] fileName=${res.fileName}, fileSize=${file.size}字节, mime=${file.type || '(unknown)'}, format=${audioFormat || '(不支持!)'}, base64Len=${base64Data.length}, hasDataPrefix=${hasDataPrefix}(应为false), head=${headPreview}..., tail=...${tailPreview}, previewUrl=${res.previewUrl || '(none)'}`);
						if (hasDataPrefix) {
							console.warn(`[Audio:buildFileContentBlocks] 警告: base64 数据包含 data: 前缀，llama.cpp 将无法解码！请检查 fileToRawBase64 实现`);
						}
						if (!audioFormat) {
							console.error(`[Audio:buildFileContentBlocks] 音频格式不支持: ${res.fileName}，llama.cpp 仅支持 wav/mp3`);
							this.showError(`音频 ${res.fileName} 格式不被 llama.cpp 支持，仅支持 wav 和 mp3`);
							userContentParts.push(`[不支持的音频格式: ${res.fileName}]`);
							continue;
						}
						contentBlocks.push({
							type: 'input_audio',
							input_audio: { data: base64Data, format: audioFormat }
						});
						userContentParts.push(`[音频: ${res.fileName}]`);
						// 保留 blob URL 用于本地消息历史预览播放
						if (res.previewUrl) {
							audioPreviewUrls.push(res.previewUrl);
						}
					} catch (err) {
						console.error(`[Audio:buildFileContentBlocks] 读取音频文件失败: ${res.fileName}`, err);
						this.showError(`无法读取音频 ${res.fileName}，未加入上下文`);
						userContentParts.push(`[无法读取音频: ${res.fileName}]`);
					}
				} else {
					console.warn(`[Audio:buildFileContentBlocks] 未找到文件对象: ${res.fileName}，跳过该音频`);
				}
				continue;
			}

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

		return { contentBlocks, userContentParts, uploadedFileUrls, audioPreviewUrls };
	}
}
