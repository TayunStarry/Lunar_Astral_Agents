import { WebSocketClient } from './socket.js';
import { renderMessage, renderAllMessages } from './chat.js';
import { AudioQueue } from './tts.js';
import { VoiceChat } from './voice.js';
import { Toast } from './toast.js';
import { FilePreviewManager } from './file-handler.js';
import { sendMessages } from './fetch.js';
import { initMusicRenderer, renderMusicScore } from './music_renderer.js';

const MAX_HISTORY_MESSAGES = 40;

// ==== 3D渲染器广播频道 ====
const rendererChannel = new BroadcastChannel('lunar-astral-renderer');

// ==== 触摸提示词生成（3D模型点击→AI对话） ====
const TOUCH_PROMPTS = {
	'头部': ['轻轻摸了摸头', '拍了拍脑袋', '揉了揉头发'],
	'头发': ['拨弄了一下头发', '轻轻拉了拉发丝', '梳理着头发'],
	'胸部': ['轻轻抱了一下', '拍了拍胸口', '靠在怀里'],
	'右臂': ['握住了右手', '轻轻碰了碰右臂', '拉起右手'],
	'左臂': ['握住了左手', '轻轻碰了碰左臂', '拉起左手'],
	'右手': ['握住了右手', '和右手十指相扣', '轻轻抚摸右手'],
	'左手': ['握住了左手', '和左手十指相扣', '轻轻抚摸左手'],
	'右腿': ['碰了碰右腿', '轻轻捏了捏右腿'],
	'左腿': ['碰了碰左腿', '轻轻捏了捏左腿'],
};

function generateTouchPrompt(part) {
	const prompts = TOUCH_PROMPTS[part];
	if (!prompts) return `触碰了${part}`;
	return prompts[Math.floor(Math.random() * prompts.length)];
}

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

	constructor() {
		this.initElements();
		this.initEventListeners();
		this.initWebSocket();
		this.initRendererChannel();
		initMusicRenderer();
	}

	initElements() {
		this.messageInput = document.getElementById('messageInput');
		this.sendButton = document.getElementById('sendButton');
		const filePreviewArea = document.getElementById('filePreviewArea');
		this.modalOverlay = document.getElementById('modalOverlay');
		this.modalBody = document.getElementById('modalBody');
		this.errorToast = document.getElementById('errorToast');
		this.modelIntel = document.getElementById('modelIntel');
		this.voiceChatItem = document.getElementById('voiceChatItem');

		// 初始化文件预览管理器
		this.fileManager = new FilePreviewManager(filePreviewArea, (msg) => this.showError(msg));

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
			fileImportInput.value = '';
		});
	}

	initVoiceChat() {
		// 临时识别结果回调：实时显示在模型区域
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
				if (this.modelIntel && VoiceChat.enabled) {
					this.modelIntel.textContent = '语音识别中...';
				}
				this.handleSend();
			}
		});

		// 语音状态变更回调
		VoiceChat.onStatusChange((enabled) => {
			this.updateVoiceChatUI(enabled);
		});

		// 语音错误回调
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
		if (this.voiceChatItem) {
			if (enabled) {
				this.voiceChatItem.classList.add('voice-active');
			} else {
				this.voiceChatItem.classList.remove('voice-active');
			}
		}
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

	// ==== 3D渲染器广播频道初始化 ====
	initRendererChannel() {
		rendererChannel.onmessage = (event) => {
			const msg = event.data;
			if (!msg || !msg.type) return;

			switch (msg.type) {
				case 'body_click':
					// 3D模型身体部位被点击 → 生成触摸提示词 → 发送给AI
					this.handleBodyClick(msg.part, msg.boneName);
					break;
				case 'action_started':
					console.log(`[主客户端] 动作已执行: ${msg.action}`);
					break;
				case 'movement_complete':
					console.log('[主客户端] 位移完成');
					break;
			}
		};
	}

	// ==== 处理3D模型点击 ====
	handleBodyClick(part, boneName) {
		const prompt = generateTouchPrompt(part);
		// 直接作为用户消息发送（不在聊天记录中显示触摸提示词）
		this.sendTouchPrompt(prompt);
	}

	async sendTouchPrompt(prompt) {
		try {
			const openAIMessages = [{ role: 'user', content: prompt }];
			await sendMessages(openAIMessages);
		} catch (error) {
			console.error('触摸提示词发送失败:', error);
		}
	}

	// ==== 向3D渲染器发送指令 ====
	sendRendererCommand(command) {
		rendererChannel.postMessage(command);
	}

	async handleWebSocketMessage(message) {
		switch (message.type) {
			case 'context':
				// 检查是否为动作/位移指令（特殊type前缀）
				// 注意：pushContext 的 data 参数是 JSON 字符串，存在 message.data.content 中
				if (message.data.type === 'action') {
					const inner = message.data.content ? JSON.parse(message.data.content) : {};
					this.sendRendererCommand({
						type: 'action',
						action: inner.action
					});
					break;
				}
				if (message.data.type === 'movement') {
					const inner = message.data.content ? JSON.parse(message.data.content) : {};
					this.sendRendererCommand({
						type: 'movement',
						position: inner.position,
						resumeTracking: inner.resumeTracking
					});
					break;
				}
				if (message.data.type === 'mouse_tracking') {
					const inner = message.data.content ? JSON.parse(message.data.content) : {};
					this.sendRendererCommand({
						type: 'mouse_tracking',
						enabled: inner.enabled
					});
					break;
				}

				// 正常响应/活动消息
				if (message.data.type === 'response' || message.data.type === 'active') {
					const content = message.data.content || '';
					const audio = message.data.audio || undefined;
					await this.handleAssistantMessage(content, undefined, audio);
					if (audio) {
						AudioQueue.enqueue(audio);
						VoiceChat.onAudioPlaybackChange();
					}
				}

				// 音乐创作消息
				if (message.data.type === 'music') {
					const abcNotation = message.data.content || '';
					if (abcNotation) {
						renderMusicScore(abcNotation);
					}
				}
				break;

			case 'image':
				if (!message.data.images) break;
				for (const imageBase64 of message.data.images) {
					const imageUrl = imageBase64.startsWith('data:') ? imageBase64 : 'data:image/jpeg;base64,' + imageBase64;
					await this.handleAssistantMessage('', imageUrl);
				}
				break;

			case 'error':
				this.showError(message.data.content || '发生错误');
				this.setLoadingState(false);
				break;
		}
	}

	/** 将消息加入历史记录 */
	addToHistory(message) {
		this.historyMessages.push(message);
		while (this.historyMessages.length > MAX_HISTORY_MESSAGES) {
			this.historyMessages.shift();
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
		this.addToHistory(assistantMessage);

		if (this.isModalOpen) {
			await renderMessage(assistantMessage, this.modalBody);
		}
		this.setLoadingState(false);
	}

	async handleSend() {
		const text = this.messageInput?.value.trim();
		const hasFiles = this.fileManager.previews.length > 0;

		if (!text && !hasFiles) return;

		this.setLoadingState(true);

		try {
			const openAIMessages = [];

			const filePromises = this.fileManager.previews.map(preview => this.fileManager.processFileUpload(preview));
			const fileResults = await Promise.all(filePromises);
			const { contentBlocks, userContentParts, uploadedFileUrls, audioPreviewUrls } = await this.fileManager.buildFileContentBlocks(fileResults);

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
			if (audioPreviewUrls.length > 0) {
				userMessage.audioUrls = audioPreviewUrls;
			}
			this.addToHistory(userMessage);

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
}

document.addEventListener('DOMContentLoaded', () => {
	window.app = new LunarCoreApp();
});
