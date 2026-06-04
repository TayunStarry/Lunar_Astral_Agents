import { WebSocketClient } from './socket.js';
import { createFilePreview, getFileCategory, revokeAllFilePreviews, getVideoThumbnail, formatFileSize } from './file.js';
import { saveFile, sendMessages } from './fetch.js';
import { Live2D, EmotionalStateEnum } from './live2d.js';
import { renderMessage, renderAllMessages } from './chat.js';
import { TTS } from './tts.js';

class LunarCoreApp {
	wsClient = null;
	historyMessages = [];
	filePreviews = [];
	isModalOpen = false;
	isLoading = false;
	messageInput = null;
	sendButton = null;
	filePreviewArea = null;
	modalOverlay = null;
	modalBody = null;
	errorToast = null;
	touchInteraction = null;
	touchRipple = null;
	touchStartTime = 0;
	touchStartPos = { x: 0, y: 0 };

	constructor() {
		this.initElements();
		this.initEventListeners();
		this.initWebSocket();
		this.initLive2D();
	}

	initElements() {
		this.messageInput = document.getElementById('messageInput');
		this.sendButton = document.getElementById('sendButton');
		this.filePreviewArea = document.getElementById('filePreviewArea');
		this.modalOverlay = document.getElementById('modalOverlay');
		this.modalBody = document.getElementById('modalBody');
		this.errorToast = document.getElementById('errorToast');
		this.touchInteraction = document.getElementById('touchInteraction');
		this.touchRipple = document.getElementById('touchRipple');

		document.getElementById('chatHistoryButton')?.addEventListener('click', () => this.toggleModal());
		document.getElementById('modalClose')?.addEventListener('click', () => this.toggleModal());
		this.modalOverlay?.addEventListener('click', (e) => {
			if (e.target === this.modalOverlay) this.toggleModal();
		});

		// 初始化触摸交互机制
		this.initTouchInteraction();
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
				await this.handleFileSelect(Array.from(files));
			}
		});
	}

	async handleFileSelect(files) {
		for (const file of files) {
			const category = getFileCategory(file);
			if (category === 'other') {
				this.showError(`文件 ${file.name} 不在允许的类型白名单中，将只发送文件名和大小`);
			}
			const preview = createFilePreview(file);
			this.filePreviews.push(preview);
			this.renderFilePreview(preview);
		}
	}

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

		this.filePreviewArea?.appendChild(item);
	}

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

	async handleWebSocketMessage(message) {
		switch (message.type) {
			case 'context':
				if (message.data.type === 'response' || message.data.type === 'active') {
					const content = message.data.content || '';
					console.log('content', content);
					const audioBase64 = await TTS.generateAndPlay(content);
					await this.handleAssistantMessage(content, undefined, audioBase64);
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
						const fileUrl = `${window.location.origin}/file/read/${file.name}`;
						await this.handleAssistantMessage('', fileUrl, '');
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

	async handleAssistantMessage(content, imageUrl, audioBase64) {
		const assistantMessage = {
			role: 'assistant',
			content: content,
			imageUrl: imageUrl || undefined,
			audioBase64: audioBase64 || undefined,
			timestamp: Date.now(),
		};
		this.historyMessages.push(assistantMessage);

		if (this.isModalOpen) {
			await renderMessage(assistantMessage, this.modalBody);
		}
		this.setLoadingState(false);
		Live2D.setStateWithTimeout(EmotionalStateEnum.IDLE);
	}

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
			const userContentParts = [];

			const filePromises = this.filePreviews.map(preview => this.processFileUpload(preview));
			const fileResults = await Promise.all(filePromises);

			for (const res of fileResults) {
				uploadedFileUrls.push(res.fileUrl);

				if (res.category === 'text') {
					const file = this.filePreviews.find(p => p.name === res.fileName)?.file;
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
			this.clearFilePreviews();
			this.autoResizeTextarea();
		} catch (error) {
			console.error('Send error:', error);
			this.showError(error instanceof Error ? error.message : '发送失败');
			this.setLoadingState(false);
			Live2D.setStateWithTimeout(EmotionalStateEnum.IDLE);
		}
	}

	clearFilePreviews() {
		revokeAllFilePreviews(this.filePreviews);
		this.filePreviews = [];
		if (this.filePreviewArea) {
			this.filePreviewArea.innerHTML = '';
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

	/**
	 * 初始化触摸交互机制
	 *
	 * 在Live2D模型区域上覆盖一层透明触摸层，统一检测用户对整个模型区域的点击操作。
	 * 支持PC端（click）和移动端（touchstart/touchend）事件。
	 * 点击时生成涟漪反馈动画，并触发AI交流交互流程。
	 */
	initTouchInteraction() {
		if (!this.touchInteraction) return;

		// PC端：点击事件
		this.touchInteraction.addEventListener('click', (e) => {
			// 如果是从触摸事件转换过来的click，跳过（避免重复触发）
			if (e.detail === 0) return;
			this.handleTouchInteraction(e.clientX, e.clientY);
		});

		// 移动端：触摸事件（使用touchstart+touchend防止滑动误触）
		this.touchInteraction.addEventListener('touchstart', (e) => {
			const touch = e.touches[0];
			this.touchStartTime = Date.now();
			this.touchStartPos = { x: touch.clientX, y: touch.clientY };
		}, { passive: true });

		this.touchInteraction.addEventListener('touchend', (e) => {
			// 判断是否为快速点击（非滑动操作）
			const duration = Date.now() - this.touchStartTime;
			const touch = e.changedTouches[0];
			const dx = touch.clientX - this.touchStartPos.x;
			const dy = touch.clientY - this.touchStartPos.y;
			const distance = Math.sqrt(dx * dx + dy * dy);

			// 只有短时间（<500ms）且短距离（<20px）的触摸才算点击
			if (duration < 500 && distance < 20) {
				this.handleTouchInteraction(touch.clientX, touch.clientY);
			}
		});
	}

	/**
	 * 处理触摸交互（统一入口）
	 *
	 * 在触摸位置生成涟漪动画反馈，并构造触摸提示词触发AI交流。
	 *
	 * @param {number} clientX - 点击位置的水平坐标
	 * @param {number} clientY - 点击位置的垂直坐标
	 */
	handleTouchInteraction(clientX, clientY) {
		// 加载中时忽略触摸
		if (this.isLoading) return;

		// 触发涟漪反馈动画
		this.triggerRippleEffect(clientX, clientY);

		// 触发AI交流交互流程
		this.triggerTouchDialogue();
	}

	/**
	 * 触发涟漪反馈动画
	 *
	 * 在点击位置生成一个扩散的圆形紫色涟漪动画，提供视觉反馈。
	 * 涟漪使用CSS动画实现，动画结束后自动清理样式。
	 *
	 * @param {number} clientX - 点击位置的水平坐标
	 * @param {number} clientY - 点击位置的垂直坐标
	 */
	triggerRippleEffect(clientX, clientY) {
		if (!this.touchRipple) return;

		// 获取触摸层相对视口的位置
		const rect = this.touchInteraction.getBoundingClientRect();
		const rippleX = clientX - rect.left;
		const rippleY = clientY - rect.top;

		// 设置涟漪位置
		this.touchRipple.style.left = rippleX + 'px';
		this.touchRipple.style.top = rippleY + 'px';

		// 移除上一次动画状态（通过强制重绘重置动画）
		this.touchRipple.classList.remove('active');
		void this.touchRipple.offsetWidth; // 强制重绘
		this.touchRipple.classList.add('active');

		// 动画结束后移除active类
		const onAnimationEnd = () => {
			this.touchRipple?.classList.remove('active');
			this.touchRipple?.removeEventListener('animationend', onAnimationEnd);
		};
		this.touchRipple.addEventListener('animationend', onAnimationEnd);
	}

	/**
	 * 触摸提示词素材库
	 *
	 * 四要素组合: <力量> + <动作> + <部位> + <态度>
	 * 格式: "<力量><动作>了<部位>, 请做出<态度>的反应"
	 */
	static TOUCH_PROMPT_CONFIG = {
		/** 力量等级（5级，从轻到重） */
		force: [
			{ value: '轻轻', weight: 3 },
			{ value: '温柔', weight: 3 },
			{ value: '稍微', weight: 2 },
			{ value: '用力', weight: 1 },
			{ value: '使劲', weight: 1 },
		],
		/** 核心动作 */
		action: [
			{ value: '摸', weight: 3 },
			{ value: '揉', weight: 2 },
			{ value: '捏', weight: 2 },
			{ value: '挠', weight: 2 },
			{ value: '拍', weight: 2 },
			{ value: '戳', weight: 1 },
			{ value: '抚', weight: 2 },
			{ value: '弹', weight: 1 },
		],
		/** 身体部位与物品 */
		part: [
			{ value: '头发', weight: 3 },
			{ value: '头顶', weight: 3 },
			{ value: '脸颊', weight: 2 },
			{ value: '发梢', weight: 2 },
			{ value: '胸部', weight: 1 },
			{ value: '腹部', weight: 1 },
			{ value: '大腿', weight: 1 },
			{ value: '小腿', weight: 1 },
			{ value: '脚', weight: 1 },
			{ value: '手', weight: 2 },
			{ value: '大臂', weight: 1 },
			{ value: '小臂', weight: 1 },
			{ value: '裙子', weight: 1 },
			{ value: '外套', weight: 1 },
		],
		/** 态度/情绪 */
		attitude: [
			{ value: '好奇', weight: 2 },
			{ value: '疑惑', weight: 2 },
			{ value: '不适', weight: 1 },
			{ value: '高兴', weight: 3 },
			{ value: '害羞', weight: 2 },
			{ value: '生气', weight: 1 },
			{ value: '惊讶', weight: 2 },
			{ value: '享受', weight: 1 },
		],
		/**
		 * 不合法组合规则
		 * 键为"力量|部位"，当随机生成的结果命中时重新生成
		 */
		excludeCombinations: new Set([
			'用力|脸颊', '使劲|脸颊',  // 敏感部位不能用重手
			'用力|胸部', '使劲|胸部',
			'用力|腹部', '使劲|腹部',
			'捏|裙子', '揉|裙子', '挠|裙子', '戳|裙子', '弹|裙子',  // 衣物不适合捏揉
			'捏|外套', '揉|外套', '挠|外套', '戳|外套', '弹|外套',
			'弹|胸部', '弹|腹部', '弹|大腿',  // 弹不适合大面积部位
		]),
	};

	/**
	 * 加权随机选择
	 *
	 * @param {Array<{ value: string; weight: number }>} items - 带权重的选项数组
	 * @returns {string} - 选中的值
	 */
	static weightedRandom(items) {
		const totalWeight = items.reduce((sum, item) => sum + item.weight, 0);
		let random = Math.random() * totalWeight;
		for (const item of items) {
			random -= item.weight;
			if (random <= 0) {
				return item.value;
			}
		}
		return items[items.length - 1].value;
	}

	/**
	 * 生成随机触摸提示词
	 *
	 * 从素材库中随机组合 <力量> + <动作> + <部位> + <态度>,
	 * 通过排除规则过滤不合法组合, 确保生成质量。
	 *
	 * @returns {string} - 格式化的触摸提示词
	 */
	static generateTouchPrompt() {
		const config = LunarCoreApp.TOUCH_PROMPT_CONFIG;
		let force, action, part, attitude;
		let attempts = 0;
		const maxAttempts = 50;

		// 循环生成直到获得合法组合
		do {
			force = LunarCoreApp.weightedRandom(config.force);
			action = LunarCoreApp.weightedRandom(config.action);
			part = LunarCoreApp.weightedRandom(config.part);
			attitude = LunarCoreApp.weightedRandom(config.attitude);
			attempts++;
		} while (
			(config.excludeCombinations.has(`${force}|${part}`) ||
				config.excludeCombinations.has(`${action}|${part}`)) &&
			attempts < maxAttempts
		);

		// 格式: "<力量><动作>了< 部位>, 请做出<态度>的反应"
		return `${force}${action}了${part}, 请做出${attitude}的反应`;
	}
	/**
	 * 触发触摸对话流程
	 *
	 * 构造触摸交互提示词，发送至后端AI模型，触发角色对触摸的回应。
	 * 复用现有的消息发送和WebSocket响应处理流程。
	 */
	async triggerTouchDialogue() {
		this.setLoadingState(true);
		Live2D.setEmotionState(EmotionalStateEnum.AWAIT);

		try {
			// 随机生成触摸提示词，直接发送至后端，不在前端聊天记录中显示
			const touchPrompt = LunarCoreApp.generateTouchPrompt();
			const openAIMessages = [{ role: 'user', content: touchPrompt }];

			// 发送消息到后端（不添加到前端聊天记录中）
			await sendMessages(openAIMessages);
		} catch (error) {
			console.error('Touch interaction error:', error);
			this.showError('触摸交互失败');
			this.setLoadingState(false);
			Live2D.setStateWithTimeout(EmotionalStateEnum.IDLE);
		}
	}
}

document.addEventListener('DOMContentLoaded', () => {
	window.app = new LunarCoreApp();
});