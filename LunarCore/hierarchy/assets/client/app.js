import { WebSocketClient } from './websocket.js';
import { randomBorderColor, clearContainer } from './utils.js';
import {
	createFilePreview,
	getFileType,
	isTextFile,
	revokeFilePreview,
	revokeAllFilePreviews,
	getVideoThumbnail
} from './fileUtils.js';
import { saveFile, sendMessages } from './api.js';
import { Live2D, EmotionalStateEnum } from './live2dManager.js';
import { renderMessage, renderAllMessages } from './messageRenderer.js';

/**
 * 核心应用类
 *
 * 管理整个聊天应用的生命周期、UI交互、WebSocket通信等核心功能
 */
class LunarCoreApp {
	/** WebSocket客户端 */
	wsClient = null;
	/** 历史消息 */
	historyMessages = [];
	/** 文件预览 */
	filePreviews = [];
	/** 是否打开弹窗 */
	isModalOpen = false;
	/** 是否加载中 */
	isLoading = false;
	/** 消息输入框 */
	messageInput = null;
	/** 发送按钮 */
	sendButton = null;
	/** 文件预览区域 */
	filePreviewArea = null;
	/** 弹窗遮罩层 */
	modalOverlay = null;
	/** 弹窗主体 */
	modalBody = null;
	/** 错误提示 */
	errorToast = null;
	/** TTS 音频上下文 */
	ttsAudioContext = null;
	/** 当前 TTS 音频源 */
	currentTtsSource = null;
	
	/** 构造函数 */
	constructor() {
		this.initElements();
		this.initEventListeners();
		this.initWebSocket();
		this.initLive2D();
	}
	
	/** 初始化元素 */
	initElements() {
		this.messageInput = document.getElementById('messageInput');
		this.sendButton = document.getElementById('sendButton');
		this.filePreviewArea = document.getElementById('filePreviewArea');
		this.modalOverlay = document.getElementById('modalOverlay');
		this.modalBody = document.getElementById('modalBody');
		this.errorToast = document.getElementById('errorToast');
		
		document.getElementById('chatHistoryButton')?.addEventListener('click', () => this.toggleModal());
		document.getElementById('modalClose')?.addEventListener('click', () => this.toggleModal());
		this.modalOverlay?.addEventListener('click', (e) => {
			if (e.target === this.modalOverlay) this.toggleModal();
		});
	}
	
	/** 初始化事件监听器 */
	initEventListeners() {
		this.sendButton?.addEventListener('click', () => this.handleSend());
		this.messageInput?.addEventListener('keydown', (e) => {
			if (e.key === 'Enter' && !e.shiftKey) {
				e.preventDefault();
				this.handleSend();
			}
		});
		this.messageInput?.addEventListener('input', () => { this.autoResizeTextarea(); });
		this.setupDragAndDrop();
	}
	
	/** 初始化拖拽事件 */
	setupDragAndDrop() {
		const inputContainer = document.getElementById('mainContainerPanel');
		['dragenter', 'dragover', 'dragleave', 'drop'].forEach(eventName => {
			inputContainer?.addEventListener(eventName, (e) => { e.preventDefault(); e.stopPropagation(); });
		});
		['dragenter', 'dragover'].forEach(eventName => {
			inputContainer?.addEventListener(eventName, () => {
				inputContainer.classList.add('drag-over');
			});
		});
		['dragleave', 'drop'].forEach(eventName => {
			inputContainer?.addEventListener(eventName, () => {
				inputContainer.classList.remove('drag-over');
			});
		});
		inputContainer?.addEventListener('drop', async (e) => {
			const files = e.dataTransfer?.files;
			if (files) {
				await this.handleFileSelect(Array.from(files));
			}
		});
	}
	
	/**
	 * 处理文件选择
	 *
	 * @param {File[]} files - 文件数组
	 *
	 * @returns {Promise<void>}
	 */
	async handleFileSelect(files) {
		for (const file of files) {
			const preview = createFilePreview(file);
			this.filePreviews.push(preview);
			this.renderFilePreview(preview);
		}
	}
	
	/**
	 * 渲染文件预览项
	 *
	 * @param {FilePreview} preview - 文件预览对象
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
			icon.className = 'fas fa-file-alt';
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
		
		this.filePreviewArea?.appendChild(item);
	}
	
	/**
	 * 移除文件预览项
	 *
	 * @param {FilePreview} preview - 文件预览对象
	 * @param {HTMLElement} item - 预览项DOM元素
	 */
	removeFilePreview(preview, item) {
		const index = this.filePreviews.indexOf(preview);
		if (index > -1) {
			this.filePreviews.splice(index, 1);
		}
		if (preview.url.startsWith('blob:')) {
			URL.revokeObjectURL(preview.url);
		}
		item.remove();
	}
	
	/**
	 * 自动调整文本框高度
	 */
	autoResizeTextarea() {
		if (this.messageInput) {
			this.messageInput.style.height = 'auto';
			this.messageInput.style.height = Math.min(this.messageInput.scrollHeight, 120) + 'px';
			Live2D.reloadContainer();
		}
	}
	
	/**
	 * 初始化WebSocket连接
	 */
	initWebSocket() {
		const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
		const wsUrl = `${protocol}//localhost:36797/ws`;
		this.wsClient = new WebSocketClient(wsUrl);
		this.wsClient.onConnect(() => {
			console.log('WebSocket connected');
			this.setLoadingState(false);
		});
		this.wsClient.onMessage((message) => {
			this.handleWebSocketMessage(message);
		});
		this.wsClient.onError((error) => {
			console.error('WebSocket error:', error);
			this.showError('连接错误，请刷新页面');
		});
		this.wsClient.connect();
	}
	
	/**
	 * 处理WebSocket消息
	 *
	 * @param {WebSocketMessage} message - WebSocket消息
	 *
	 * @returns {Promise<void>}
	 */
	async handleWebSocketMessage(message) {
		switch (message.type) {
			case 'context':
				if (message.data.type === 'response' || message.data.type === 'active') {
					const content = message.data.content || '';
					this.processTTS(content);
					await this.handleAssistantMessage(content);
				}
				break;
			case 'image':
				if (message.data.images) {
					for (const imageBase64 of message.data.images) {
						const byteString = atob(imageBase64);
						const mimeType = 'image/jpeg';
						const ab = new ArrayBuffer(byteString.length);
						const ia = new Uint8Array(ab);
						for (let i = 0; i < byteString.length; i++) {
							ia[i] = byteString.charCodeAt(i);
						}
						const blob = new Blob([ab], { type: mimeType });
						const file = new File([blob], `assistant_${Date.now()}.jpg`, { type: 'image/jpeg' });
						const fileUrl = `${window.location.origin}/read/${file.name}`;
						await this.handleAssistantMessage('', fileUrl);
					}
				}
				break;
			case 'error':
				this.showError(message.data.content || '发生错误');
				this.setLoadingState(false);
				Live2D.setStateWithTimeout(EmotionalStateEnum.IDLE);
				break;
		}
	}
	
	/**
	 * 清理文本，用于语音合成
	 * 
	 * @param {string} text - 原始文本
	 * @returns {string} - 清理后的文本，适合TTS朗读
	 */
	cleanTextForTTS(text) {
		if (!text) return '';
		
		let processed = text;
		
		processed = processed.replace(/```[\s\S]*?```/g, '');
		processed = processed.replace(/`[^`]*`/g, '');
		processed = processed.replace(/!\[.*?\]\(.*?\)/g, '');
		processed = processed.replace(/\[.*?\]\(.*?\)/g, '');
		processed = processed.replace(/<[^>]*>/g, '');
		processed = processed.replace(/[\u{1F600}-\u{1F64F}\u{1F300}-\u{1F5FF}\u{1F680}-\u{1F6FF}\u{1F900}-\u{1F9FF}\u{2600}-\u{26FF}\u{2700}-\u{27BF}\u{FE00}-\u{FE0F}\u{1F1E0}-\u{1F1FF}\u{200D}\u{20E3}\u{FE0F}]/gu, '');
		processed = processed.replace(/\*/g, '');
		processed = processed.replace(/\r?\n/g, ' ');
		processed = processed.replace(/\（[^）]*\）/g, '');
		processed = processed.replace(/\([^)]*\)/g, '');
		
		const allowed = '\\u4e00-\\u9fff' + 'a-zA-Z0-9' + '\\s' + '\uFF0C\u3002\uFF1F\uFF1A' + '\u201C\u201D\u2018\u2019' + '\u300A\u300B' + ',.\'\"?:!';
		const whitelist = new RegExp(`[^${allowed}]`, 'g');
		processed = processed.replace(whitelist, '');
		processed = processed.replace(/\s+/g, ' ');
		return processed.trim();
	}
	
	/**
	 * 文本转语音
	 * @param {string} text - 原始消息文本
	 */
	async processTTS(text) {
		const processedText = this.cleanTextForTTS(text);
		
		let audioBlob;
		try {
			const response = await fetch('./voice_template.wav');
			if (!response.ok) throw new Error('Failed to fetch voice template');
			audioBlob = await response.blob();
		} catch (err) {
			console.error('TTS: 语音模板加载失败', err);
			this.showError('语音模板加载失败');
			return;
		}
		
		const formData = new FormData();
		formData.append('text', processedText);
		formData.append('demo_id', 'demo-1');
		formData.append('prompt_audio', audioBlob, 'voice_template.wav');
		formData.append('max_new_frames', '800');
		formData.append('voice_clone_max_text_tokens', '200');
		formData.append('attn_implementation', 'eager');
		formData.append('do_sample', '1');
		formData.append('text_temperature', '1.0');
		formData.append('text_top_p', '1.0');
		formData.append('text_top_k', '50');
		formData.append('audio_temperature', '0.8');
		formData.append('audio_top_p', '0.95');
		formData.append('audio_top_k', '25');
		formData.append('audio_repetition_penalty', '1.2');
		formData.append('seed', '16384');
		formData.append('tts_max_batch_size', '0');
		formData.append('codec_max_batch_size', '0');
		formData.append('enable_text_normalization', '0');
		formData.append('enable_normalize_tts_text', '0');
		formData.append('cpu_threads', '8');
		
		try {
			const apiUrl = '/audio/generate';
			const res = await fetch(apiUrl, {
				method: 'POST',
				body: formData
			});
			if (!res.ok) throw new Error(`TTS API 状态异常: ${res.status}`);
			const data = await res.json();
			
			if (data.audio_base64) {
				const arrayBuffer = this.base64ToArrayBuffer(data.audio_base64);
				this.playAudioBuffer(arrayBuffer);
			} else {
				console.warn('TTS: 响应中没有 audio_base64');
			}
		} catch (err) {
			console.error('TTS 请求失败:', err);
			this.showError('语音生成失败');
		}
	}
	
	/**
	 * 解码音频Buffer并播放
	 * @param {ArrayBuffer} arrayBuffer 
	 */
	playAudioBuffer(arrayBuffer) {
		if (!this.ttsAudioContext) {
			this.ttsAudioContext = new (window.AudioContext || window.webkitAudioContext)();
		}
		
		if (this.currentTtsSource) {
			try {
				this.currentTtsSource.stop();
			} catch (e) {}
			this.currentTtsSource = null;
		}
		
		this.ttsAudioContext.decodeAudioData(
			arrayBuffer,
			(audioBuffer) => {
				const source = this.ttsAudioContext.createBufferSource();
				source.buffer = audioBuffer;
				source.connect(this.ttsAudioContext.destination);
				this.currentTtsSource = source;
				source.onended = () => {
					this.currentTtsSource = null;
				};
				source.start();
			},
			(err) => {
				console.error('TTS: decodeAudioData failed', err);
				this.showError('音频解码失败');
			}
		);
	}
	
	/**
	 * 将 Base64 字符串转换为 ArrayBuffer
	 * @param {string} base64 
	 * @returns {ArrayBuffer}
	 */
	base64ToArrayBuffer(base64) {
		const binaryString = atob(base64);
		const len = binaryString.length;
		const bytes = new Uint8Array(len);
		for (let i = 0; i < len; i++) {
			bytes[i] = binaryString.charCodeAt(i);
		}
		return bytes.buffer;
	}
	
	/**
	 * 处理助手消息
	 *
	 * @param {string} content - 消息内容
	 * @param {string} [imageUrl] - 图片URL（可选）
	 *
	 * @returns {Promise<void>}
	 */
	async handleAssistantMessage(content, imageUrl) {
		const assistantMessage = {
			role: 'assistant',
			content: content,
			imageUrl: imageUrl || undefined,
			timestamp: Date.now(),
		};
		this.historyMessages.push(assistantMessage);
		
		if (this.isModalOpen) {
			await renderMessage(assistantMessage, this.modalBody);
		}
		this.setLoadingState(false);
		Live2D.setStateWithTimeout(EmotionalStateEnum.IDLE);
	}
	
	/**
	 * 异步保存并提取文件信息
	 * @param {FilePreview} preview
	 * @returns {Promise<{fileUrl: string, textContent?: string, isText: boolean, fileName: string}>}
	 */
	async processFileUpload(preview) {
		const saveResult = await saveFile(preview.file, true);
		const fileUrl = `${window.location.origin}/read/${saveResult.filename}`;
		let textContent = null;
		let isText = false;
		
		if (isTextFile(preview.file)) {
			isText = true;
			try {
				const rawText = await preview.file.text();
				const MAX_TEXT_LEN = 50000;
				if (rawText.length > MAX_TEXT_LEN) {
					textContent = rawText.slice(0, MAX_TEXT_LEN) + '\n\n[文件内容过长，已截断]';
					this.showError(`文件 ${preview.file.name} 内容超过限制，仅截取前 ${MAX_TEXT_LEN} 字符`);
				} else {
					textContent = rawText;
				}
			} catch (err) {
				console.error(`读取文本文件失败: ${preview.file.name}`, err);
				this.showError(`无法读取文件 ${preview.file.name}，未加入上下文`);
			}
		}
		return { fileUrl, textContent, isText, fileName: preview.name };
	}
	
	/**
	 * 处理发送消息
	 *
	 * @returns {Promise<void>}
	 */
	async handleSend() {
		const text = this.messageInput?.value.trim();
		const hasFiles = this.filePreviews.length > 0;
		
		if (!text && !hasFiles) return;
		
		this.setLoadingState(true);
		Live2D.setEmotionState(EmotionalStateEnum.AWAIT);
		
		try {
			const openAIMessages = [];
			const contentBlocks = [];
			const uploadedFileUrls = [];
			
			const filePromises = this.filePreviews.map(preview => this.processFileUpload(preview));
			const fileResults = await Promise.all(filePromises);
			
			for (const res of fileResults) {
				if (res.isText && res.textContent) {
					const textBlock = `【文件 ${res.fileName}】\n内容：\n\`\`\`\n${res.textContent}\n\`\`\`\n访问链接：${res.fileUrl}`;
					contentBlocks.push({ type: 'text', text: textBlock });
					uploadedFileUrls.push(res.fileUrl);
				} else if (!res.isText) {
					contentBlocks.push({ type: 'image_url', image_url: { url: res.fileUrl } });
					uploadedFileUrls.push(res.fileUrl);
				} else {
					contentBlocks.push({ type: 'text', text: `【文件 ${res.fileName}】无法读取内容，访问链接：${res.fileUrl}` });
					uploadedFileUrls.push(res.fileUrl);
				}
			}
			
			if (text) {
				contentBlocks.push({ type: 'text', text: text });
			}
			
			const userContent = contentBlocks.length > 0 ? contentBlocks : text;
			openAIMessages.push({ role: 'user', content: userContent });
			
			const userMessage = {
				role: 'user',
				content: text || (hasFiles ? '[文件]' : ''),
				timestamp: Date.now(),
			};
			if (uploadedFileUrls.length > 0) {
				userMessage.imageUrls = uploadedFileUrls;
			}
			this.historyMessages.push(userMessage);
			
			if (this.isModalOpen) {
				await renderMessage(userMessage, this.modalBody);
			}
			
			await sendMessages(openAIMessages);
			
			this.messageInput.value = '';
			this.clearFilePreviews();
			this.autoResizeTextarea();
		} catch (error) {
			console.error('Send error:', error);
			this.showError(error instanceof Error ? error.message : '发送失败');
			this.setLoadingState(false);
			Live2D.setStateWithTimeout(EmotionalStateEnum.IDLE);
		}
	}
	
	/**
	 * 清空所有文件预览
	 */
	clearFilePreviews() {
		revokeAllFilePreviews(this.filePreviews);
		this.filePreviews = [];
		if (this.filePreviewArea) {
			this.filePreviewArea.innerHTML = '';
		}
	}
	
	/**
	 * 设置加载状态
	 *
	 * @param {boolean} loading - 是否加载中
	 */
	setLoadingState(loading) {
		this.isLoading = loading;
		if (this.sendButton) {
			this.sendButton.disabled = loading;
			if (loading) {
				this.sendButton.innerHTML = '<span class="loading-indicator"></span>';
			} else {
				this.sendButton.innerHTML = '<i class="fas fa-paper-plane"></i>';
			}
		}
	}
	
	/**
	 * 切换弹窗显示状态
	 */
	toggleModal() {
		this.isModalOpen = !this.isModalOpen;
		if (this.isModalOpen) {
			this.modalOverlay?.classList.add('visible');
			if (this.modalBody) {
				this.modalBody.innerHTML = '';
			}
			renderAllMessages(this.modalBody, this.historyMessages, false);
		} else {
			this.modalOverlay?.classList.remove('visible');
		}
	}
	
	/**
	 * 显示错误提示
	 *
	 * @param {string} message - 错误信息
	 */
	showError(message) {
		if (this.errorToast) {
			this.errorToast.textContent = message;
			this.errorToast.classList.add('visible');
			setTimeout(() => {
				this.errorToast?.classList.remove('visible');
			}, 3000);
		}
	}
	
	/**
	 * 初始化Live2D
	 *
	 * @returns {Promise<void>}
	 */
	async initLive2D() {
		try {
			await Live2D.init();
		} catch (error) {
			console.error('Failed to initialize Live2D:', error);
			this.showError('Live2D 初始化失败');
		}
	}
}

// 启动应用
document.addEventListener('DOMContentLoaded', () => {
	window.app = new LunarCoreApp();
});