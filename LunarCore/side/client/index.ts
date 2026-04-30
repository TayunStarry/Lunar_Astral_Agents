/// <reference path="./global.d.ts" />
import { Message, FilePreview, WebSocketMessage, MessageContentBlock } from './types';
import { WebSocketClient } from './websocket';
import { Live2D, EmotionalStateEnum } from './live2d';
import { renderMessage, renderAllMessages } from './renderer';
import { createFilePreview, isMediaFile, revokeAllFilePreviews, saveFile, sendMessages, getVideoThumbnail } from './file-handler';

/**
 * 核心应用类
 *
 * 管理整个聊天应用的生命周期、UI交互、WebSocket通信等核心功能
 */
export class LunarCoreApp {
	/** WebSocket客户端 */
	private wsClient: WebSocketClient | null = null;
	/** 历史消息 */
	private historyMessages: Message[] = [];
	/** 文件预览 */
	private filePreviews: FilePreview[] = [];
	/** 是否打开弹窗 */
	private isModalOpen = false;
	/** 是否加载中 */
	private isLoading = false;
	/** 消息输入框 */
	private messageInput: HTMLTextAreaElement | null = null;
	/** 发送按钮 */
	private sendButton: HTMLButtonElement | null = null;
	/** 文件预览区域 */
	private filePreviewArea: HTMLElement | null = null;
	/** 聊天历史面板 */
	private chatHistoryPanel: HTMLElement | null = null;
	/** 弹窗遮罩层 */
	private modalOverlay: HTMLElement | null = null;
	/** 弹窗主体 */
	private modalBody: HTMLElement | null = null;
	/** 错误提示 */
	private errorToast: HTMLElement | null = null;
	/** 构造函数 */
	constructor() { this.initElements(); this.initEventListeners(); this.initWebSocket(); this.initLive2D(); }
	/** 初始化元素 */
	private initElements(): void {
		// 初始化 消息输入框 元素
		this.messageInput = document.getElementById('messageInput') as HTMLTextAreaElement;
		// 初始化 发送按钮 元素
		this.sendButton = document.getElementById('sendButton') as HTMLButtonElement;
		// 初始化 文件预览区域 元素
		this.filePreviewArea = document.getElementById('filePreviewArea');
		// 初始化 聊天历史面板 元素
		this.chatHistoryPanel = document.getElementById('chatHistoryPanel');
		// 初始化 弹窗遮罩层 元素
		this.modalOverlay = document.getElementById('modalOverlay');
		// 初始化 弹窗主体 元素
		this.modalBody = document.getElementById('modalBody');
		// 初始化 错误提示 元素
		this.errorToast = document.getElementById('errorToast');
		// 初始化 聊天历史按钮 元素
		document.getElementById('chatHistoryButton')?.addEventListener('click', () => this.toggleModal());
		// 初始化 弹窗关闭按钮 元素
		document.getElementById('modalClose')?.addEventListener('click', () => this.toggleModal());
		// 初始化 弹窗遮罩层 点击事件 元素
		this.modalOverlay?.addEventListener('click', (e) => { if (e.target === this.modalOverlay) this.toggleModal(); });
	}
	/** 初始化事件监听器 */
	private initEventListeners(): void {
		// 初始化 发送按钮 点击事件 元素
		this.sendButton?.addEventListener('click', () => this.handleSend());
		// 初始化 消息输入框 键盘事件 元素
		this.messageInput?.addEventListener('keydown',
			(e) => {
				if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); this.handleSend(); }
			}
		);
		// 初始化 消息输入框 输入事件 元素
		this.messageInput?.addEventListener('input', () => { this.autoResizeTextarea(); });
		// 初始化 消息输入框 拖拽事件 元素
		this.setupDragAndDrop();
	}
	/** 初始化拖拽事件 */
	private setupDragAndDrop(): void {
		const inputContainer = document.getElementById('inputContainer');
		['dragenter', 'dragover', 'dragleave', 'drop'].forEach(
			eventName => {
				inputContainer?.addEventListener(eventName, (e) => { e.preventDefault(); e.stopPropagation(); });
			}
		);
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
	private async handleFileSelect(files: File[]): Promise<void> {
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
	private renderFilePreview(preview: FilePreview): void {
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
	private removeFilePreview(preview: FilePreview, item: HTMLElement): void {
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
	private autoResizeTextarea(): void {
		if (this.messageInput) {
			this.messageInput.style.height = 'auto';
			this.messageInput.style.height = Math.min(this.messageInput.scrollHeight, 120) + 'px';
		}
	}

	/**
	 * 初始化WebSocket连接
	 */
	private initWebSocket(): void {
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
	private async handleWebSocketMessage(message: WebSocketMessage): Promise<void> {
		switch (message.type) {
			case 'context':
				if (message.data.type === 'response' || message.data.type === 'active') {
					await this.handleAssistantMessage(message.data.content || '');
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
						const saveResult = await saveFile(file, true);
						const fileUrl = `${window.location.origin}/read/${saveResult.filename}`;
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
	 * 处理助手消息
	 *
	 * @param {string} content - 消息内容
	 * @param {string} [imageUrl] - 图片URL（可选）
	 *
	 * @returns {Promise<void>}
	 */
	private async handleAssistantMessage(content: string, imageUrl?: string): Promise<void> {
		const assistantMessage: Message = {
			role: 'assistant',
			content: content,
			imageUrl: imageUrl || undefined,
			timestamp: Date.now(),
		};

		this.historyMessages.push(assistantMessage);
		await renderMessage(assistantMessage, this.chatHistoryPanel!);

		if (this.isModalOpen) {
			await renderMessage(assistantMessage, this.modalBody!);
		}

		this.setLoadingState(false);
		Live2D.setStateWithTimeout(EmotionalStateEnum.IDLE);
	}

	/**
	 * 处理发送消息
	 *
	 * @returns {Promise<void>}
	 */
	private async handleSend(): Promise<void> {
		const text = this.messageInput?.value.trim();
		const hasFiles = this.filePreviews.length > 0;

		if (!text && !hasFiles) {
			return;
		}

		this.setLoadingState(true);
		Live2D.setEmotionState(EmotionalStateEnum.AWAIT);

		try {
			const openAIMessages: { role: string; content: string | MessageContentBlock[] }[] = [];
			let userContent: string | MessageContentBlock[];
			const uploadedImageUrls: string[] = [];

			if (hasFiles) {
				const contentBlocks: MessageContentBlock[] = [];

				for (const preview of this.filePreviews) {
					if (isMediaFile(preview.file)) {
						const saveResult = await saveFile(preview.file, false);
						const fileUrl = `${window.location.origin}/read/${saveResult.filename}`;

						if (preview.type === 'image') {
							contentBlocks.push({
								type: 'image_url',
								image_url: { url: fileUrl },
							});
							uploadedImageUrls.push(fileUrl);
						} else if (preview.type === 'video') {
							contentBlocks.push({
								type: 'image_url',
								image_url: { url: fileUrl },
							});
							uploadedImageUrls.push(fileUrl);
						}
					}
				}

				if (text) {
					contentBlocks.push({ type: 'text', text });
				}

				userContent = contentBlocks;
			} else {
				userContent = text!;
			}

			openAIMessages.push({
				role: 'user',
				content: userContent,
			});

			const userMessage: Message = {
				role: 'user',
				content: text || (hasFiles ? '[文件]' : ''),
				timestamp: Date.now(),
			};

			if (uploadedImageUrls.length > 0) {
				userMessage.imageUrls = uploadedImageUrls;
			}

			this.historyMessages.push(userMessage);
			await renderMessage(userMessage, this.chatHistoryPanel!);

			if (this.isModalOpen) {
				await renderMessage(userMessage, this.modalBody!);
			}

			await sendMessages(openAIMessages);

			this.messageInput!.value = '';
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
	private clearFilePreviews(): void {
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
	private setLoadingState(loading: boolean): void {
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
	private toggleModal(): void {
		this.isModalOpen = !this.isModalOpen;

		if (this.isModalOpen) {
			this.modalOverlay?.classList.add('visible');
			if (this.modalBody) {
				this.modalBody.innerHTML = '';
			}
			renderAllMessages(this.modalBody!, this.historyMessages, false);
		} else {
			this.modalOverlay?.classList.remove('visible');
		}
	}

	/**
	 * 显示错误提示
	 *
	 * @param {string} message - 错误信息
	 */
	private showError(message: string): void {
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
	private async initLive2D(): Promise<void> {
		try {
			await Live2D.init();
		} catch (error) {
			console.error('Failed to initialize Live2D:', error);
			this.showError('Live2D 初始化失败');
		}
	}
}

document.addEventListener('DOMContentLoaded', () => { window.app = new LunarCoreApp(); });
