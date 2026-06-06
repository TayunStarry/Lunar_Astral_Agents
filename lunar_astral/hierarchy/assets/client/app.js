import { WebSocketClient } from './socket.js';
import { Live2D, EmotionalStateEnum } from './live2d.js';
import { renderMessage, renderAllMessages } from './chat.js';
import { AudioQueue } from './tts.js';
import { TouchInteractionHandler } from './touch.js';
import { FilePreviewManager } from './file-handler.js';
import { sendMessages } from './fetch.js';

class LunarCoreApp {
	wsClient = null;
	historyMessages = [];
	isModalOpen = false;
	isLoading = false;
	messageInput = null;
	sendButton = null;
	modalOverlay = null;
	modalBody = null;
	errorToast = null;
	fileManager = null;
	touchHandler = null;

	constructor() {
		this.initElements();
		this.initEventListeners();
		this.initWebSocket();
		this.initLive2D();
	}

	initElements() {
		this.messageInput = document.getElementById('messageInput');
		this.sendButton = document.getElementById('sendButton');
		const filePreviewArea = document.getElementById('filePreviewArea');
		this.modalOverlay = document.getElementById('modalOverlay');
		this.modalBody = document.getElementById('modalBody');
		this.errorToast = document.getElementById('errorToast');
		const touchInteraction = document.getElementById('touchInteraction');
		const touchRipple = document.getElementById('touchRipple');

		// 初始化文件预览管理器
		this.fileManager = new FilePreviewManager(filePreviewArea, (msg) => this.showError(msg));

		// 初始化触摸交互处理器
		this.touchHandler = new TouchInteractionHandler(
			touchInteraction,
			touchRipple,
			{
				setLoadingState: (loading) => this.setLoadingState(loading),
				showError: (msg) => this.showError(msg),
				isLoading: () => this.isLoading,
			}
		);

		document.getElementById('chatHistoryButton')?.addEventListener('click', () => this.toggleModal());
		document.getElementById('modalClose')?.addEventListener('click', () => this.toggleModal());
		this.modalOverlay?.addEventListener('click', (e) => {
			if (e.target === this.modalOverlay) this.toggleModal();
		});
	}

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
				await this.fileManager.handleFileSelect(Array.from(files));
			}
		});
	}

	autoResizeTextarea() {
		if (this.messageInput) {
			this.messageInput.style.height = 'auto';
			this.messageInput.style.height = Math.min(this.messageInput.scrollHeight, 120) + 'px';
			Live2D.reloadContainer();
		}
	}

	initWebSocket() {
		const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
		const wsUrl = `${protocol}//${window.location.host}/ws`;
		this.wsClient = new WebSocketClient(wsUrl);
		this.wsClient.onConnect(() => { console.log('WebSocket connected'); this.setLoadingState(false); });
		this.wsClient.onMessage((message) => this.handleWebSocketMessage(message));
		this.wsClient.onError((error) => { console.error('WebSocket error:', error); this.showError('连接错误，请刷新页面'); });
		this.wsClient.connect();
	}

	async handleWebSocketMessage(message) {
		switch (message.type) {
			case 'context':
				// 判断是否为响应或活动消息
				if (message.data.type === 'response' || message.data.type === 'active') {
					/** 获取消息的文本内容 */
					const content = message.data.content || '';
					// 渲染消息到历史记录
					await this.handleAssistantMessage(content, undefined);
					// 如果包含音频数据，加入播放队列
					if (message.data.audio) {
						AudioQueue.enqueue(message.data.audio);
					}
				}
				break;

			case 'image':
				// 判断是否有图片信息
				if (!message.data.images) break;
				// 遍历图片信息，执行图片渲染
				for (const imageBase64 of message.data.images) {
					// 渲染图片到历史记录
					await this.handleAssistantMessage('', 'data:image/jpeg;base64,' + imageBase64);
				}
				break;

			case 'error':
				this.showError(message.data.content || '发生错误');
				this.setLoadingState(false);
				Live2D.setStateWithTimeout(EmotionalStateEnum.IDLE);
				break;
		}
	}

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

	async handleSend() {
		const text = this.messageInput?.value.trim();
		const hasFiles = this.fileManager.previews.length > 0;

		if (!text && !hasFiles) return;

		this.setLoadingState(true);
		Live2D.setEmotionState(EmotionalStateEnum.AWAIT);

		try {
			const openAIMessages = [];

			// 上传文件并构建内容块
			const filePromises = this.fileManager.previews.map(preview => this.fileManager.processFileUpload(preview));
			const fileResults = await Promise.all(filePromises);
			const { contentBlocks, userContentParts, uploadedFileUrls } = await this.fileManager.buildFileContentBlocks(fileResults);

			if (text) {
				contentBlocks.push({ type: 'text', text: text });
			}

			const userContent = contentBlocks.length > 0 ? contentBlocks : text;
			openAIMessages.push({ role: 'user', content: userContent });

			const userMessage = {
				role: 'user',
				content: text || (userContentParts.length > 0 ? userContentParts.join(' ') : ''),
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
			this.fileManager.clearFilePreviews();
			this.autoResizeTextarea();
		} catch (error) {
			console.error('Send error:', error);
			this.showError(error instanceof Error ? error.message : '发送失败');
			this.setLoadingState(false);
			Live2D.setStateWithTimeout(EmotionalStateEnum.IDLE);
		}
	}

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

	showError(message) {
		if (this.errorToast) {
			this.errorToast.textContent = message;
			this.errorToast.classList.add('visible');
			setTimeout(() => { this.errorToast?.classList.remove('visible'); }, 3000);
		}
	}

	async initLive2D() {
		try {
			await Live2D.init();
		} catch (error) {
			console.error('Failed to initialize Live2D:', error);
			this.showError('Live2D 初始化失败');
		}
	}
}

document.addEventListener('DOMContentLoaded', () => {
	window.app = new LunarCoreApp();
});
