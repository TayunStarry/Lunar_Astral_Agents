import { WebSocketClient } from './socket.js';
import { Live2D, EmotionalStateEnum } from './live2d.js';
import { renderMessage, renderAllMessages } from './chat.js';
import { AudioQueue } from './tts.js';
import { VoiceChat } from './voice.js';
import { Toast } from './toast.js';
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
		this.modelIntel = document.getElementById('modelIntel');
		const touchInteraction = document.getElementById('touchInteraction');
		const touchRipple = document.getElementById('touchRipple');
		this.voiceChatItem = document.getElementById('voiceChatItem');

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

		// 功能菜单下拉
		this.initFunctionMenu();

		// 模态框关闭
		document.getElementById('modalClose')?.addEventListener('click', () => this.toggleModal());
		this.modalOverlay?.addEventListener('click', (e) => {
			if (e.target === this.modalOverlay) this.toggleModal();
		});

		// 点击页面其他地方关闭下拉菜单
		document.addEventListener('click', (e) => {
			const menuWrapper = document.getElementById('functionMenuWrapper');
			const dropdown = document.getElementById('functionMenuDropdown');
			if (menuWrapper && dropdown && !menuWrapper.contains(e.target)) {
				dropdown.classList.remove('visible');
			}
		});

		// 初始化语音识别
		this.initVoiceChat();
	}

	initFunctionMenu() {
		const menuButton = document.getElementById('functionMenuButton');
		const dropdown = document.getElementById('functionMenuDropdown');
		const chatHistoryItem = document.getElementById('chatHistoryItem');
		const voiceChatItemEl = document.getElementById('voiceChatItem');
		const fileImportItem = document.getElementById('fileImportItem');
		const fileImportInput = document.getElementById('fileImportInput');

		// 菜单按钮：切换下拉菜单
		menuButton?.addEventListener('click', (e) => {
			e.stopPropagation();
			dropdown?.classList.toggle('visible');
		});

		// 聊天记录
		chatHistoryItem?.addEventListener('click', (e) => {
			e.stopPropagation();
			dropdown?.classList.remove('visible');
			this.toggleModal();
		});

		// 语音对话
		voiceChatItemEl?.addEventListener('click', (e) => {
			e.stopPropagation();
			dropdown?.classList.remove('visible');
			const enabled = VoiceChat.toggle();
			this.updateVoiceChatUI(enabled);
		});

		// 文件导入
		fileImportItem?.addEventListener('click', (e) => {
			e.stopPropagation();
			dropdown?.classList.remove('visible');
			fileImportInput?.click();
		});

		// 文件导入输入处理
		fileImportInput?.addEventListener('change', async (e) => {
			const files = e.target.files;
			if (files && files.length > 0) {
				await this.fileManager.handleFileSelect(Array.from(files));
			}
			// 重置input以允许重复选择同一文件
			fileImportInput.value = '';
		});
	}

	initVoiceChat() {
		// 临时识别结果回调：实时显示在 Live2D 区域
		VoiceChat.onInterimResult((text) => {
			if (this.modelIntel && text) {
				this.modelIntel.textContent = text;
				this.modelIntel.classList.add('visible');
			}
		});

		// 最终识别结果回调：自动填入输入框并发送
		VoiceChat.onResult((text) => {
			if (this.messageInput && text) {
				this.messageInput.value = text;
				this.autoResizeTextarea();
				// 恢复语音识别提示文字
				if (this.modelIntel && VoiceChat.enabled) {
					this.modelIntel.textContent = '语音识别中...';
				}
				// 自动触发消息发送
				this.handleSend();
			}
		});

		// 语音状态变更回调：更新UI
		VoiceChat.onStatusChange((enabled) => {
			this.updateVoiceChatUI(enabled);
		});

		// 语音错误回调：显示友好提示
		VoiceChat.onError((errorType, reason) => {
			switch (errorType) {
				case 'not-allowed':
					Toast.error('麦克风权限被拒绝，请在浏览器设置中允许麦克风访问');
					break;
				case 'unsupported':
					Toast.warning(reason || '当前环境不支持语音识别');
					break;
			}
		});
	}

	updateVoiceChatUI(enabled) {
		// 更新语音对话菜单项样式
		if (this.voiceChatItem) {
			if (enabled) {
				this.voiceChatItem.classList.add('voice-active');
			} else {
				this.voiceChatItem.classList.remove('voice-active');
			}
		}

		// 控制 Live2D 区域的语音识别状态显示
		if (this.modelIntel) {
			if (enabled) {
				this.modelIntel.textContent = '语音识别中...';
				this.modelIntel.classList.add('visible', 'listening');
			} else {
				this.modelIntel.classList.remove('visible', 'listening');
				this.modelIntel.textContent = '';
			}
		}
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
		this.wsClient.onConnect(() => this.setLoadingState(false));
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
					/** 获取音频数据 */
					const audio = message.data.audio || undefined;
					// 渲染消息到历史记录
					await this.handleAssistantMessage(content, undefined, audio);
					// 如果包含音频数据，加入播放队列
					if (audio) {
						AudioQueue.enqueue(audio);
						// 通知语音识别：音频即将播放
						VoiceChat.onAudioPlaybackChange();
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

	async handleAssistantMessage(content, imageUrl, audio) {
		const assistantMessage = {
			role: 'assistant',
			content: content,
			imageUrl: imageUrl || undefined,
			audio: audio,
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
