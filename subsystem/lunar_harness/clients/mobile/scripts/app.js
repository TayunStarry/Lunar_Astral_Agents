import { WebSocketClient } from './socket.js';
import { AudioQueue } from './tts.js';
import { VoiceChat } from './voice.js';
import { Toast } from './toast.js';
import { sendMessages } from './fetch.js';
import { initUserName, getUserNamePrefix } from './username.js';
import { setupRendererFrame as setupSharedRendererFrame } from '/shared/core/renderer.js';
import { FilePreviewManager } from './file-manager.js';
import { HistoryView } from './history-view.js';
import { createViewSwitcher, maybeShowHistoryHint } from './view-switch.js';
import { extractWebpageEnvelopes } from '/shared/core/media.js';
import { formatFileSize } from './file.js';

class LunarCoreApp {
	wsClient = null;
	isLoading = false;
	messageInput = null;
	sendButton = null;
	errorToast = null;
	voiceBtn = null;
	fileImportBtn = null;
	fileInput = null;
	fileManager = null;
	historyView = null;
	viewSwitcher = null;

	constructor() {
		// 确保称呼可用：优先读取浏览器缓存，缺失时显示覆盖层
		initUserName();
		this.initElements();
		this.initVoiceChat();
		this.initEventListeners();
		this.initHistoryView();
		this.initWebSocket();
		this.setupRendererFrame();
		this.restoreLatestMusic();
	}

	initElements() {
		this.messageInput = document.getElementById('messageInput');
		this.sendButton = document.getElementById('sendButton');
		this.errorToast = document.getElementById('errorToast');
		this.voiceBtn = document.getElementById('voiceBtn');
		this.fileImportBtn = document.getElementById('fileImportBtn');
		this.fileInput = document.getElementById('fileInput');

		// 附件管理器：气泡预览区 + 错误提示 + 按钮状态联动
		this.fileManager = new FilePreviewManager(
			document.getElementById('filePreviewArea'),
			(msg) => this.showError(msg),
			() => this.updateActionButton()
		);
	}

	// ==== 历史记录视图（双击输入框切换，数据与桌面端同源） ====
	initHistoryView() {
		const panel = document.getElementById('mainContainerPanel');
		const historyView = document.getElementById('historyView');
		this.historyView = new HistoryView({
			scrollEl: document.getElementById('historyScroll'),
			listEl: document.getElementById('historyList'),
			statusEl: document.getElementById('historyStatus')
		});
		this.viewSwitcher = createViewSwitcher({
			panel,
			input: this.messageInput,
			onShow: () => {
				historyView?.setAttribute('aria-hidden', 'false');
				this.historyView.open();
			},
			onHide: () => historyView?.setAttribute('aria-hidden', 'true')
		});
		// 首次进入的一次性提示（localStorage 记忆，非按钮）
		maybeShowHistoryHint(document.getElementById('historyHint'));
	}

	// ==== 连续语音对话（AEC + VAD 自动录音发送） ====
	initVoiceChat() {
		// 语音状态变更：更新麦克风按钮与模型区状态标识
		VoiceChat.onStatusChange((state) => this.updateVoiceUI(state));

		// 语音错误回调
		VoiceChat.onError((errorType, reason) => {
			switch (errorType) {
				case 'not-allowed':
					Toast.error('麦克风权限被拒绝，请在浏览器设置中允许麦克风访问');
					break;
				case 'unsupported':
					Toast.warning(reason || '当前环境不支持连续语音');
					break;
				case 'send-failed':
					Toast.error(reason || '语音发送失败');
					break;
				case 'rate-limited':
					Toast.info('月华还没回应上一条语音，稍后再说吧', 2500);
					break;
			}
		});

		this.voiceBtn?.addEventListener('click', async () => {
			const enabled = await VoiceChat.toggle();
			if (enabled) {
				Toast.success('连续语音已开启，直接说话即可', 2500);
			} else {
				Toast.info('连续语音已关闭', 2000);
			}
		});
	}

	/** 麦克风按钮状态：idle 待机 / listening 持续监听 / capturing 正在采集用户语音（仅按钮呈现，无气泡提示） */
	updateVoiceUI(state) {
		if (!this.voiceBtn) return;
		this.voiceBtn.classList.toggle('listening', state === 'listening');
		this.voiceBtn.classList.toggle('capturing', state === 'capturing');
		if (state === 'capturing') {
			this.voiceBtn.title = '正在聆听你的声音…';
			this.voiceBtn.innerHTML = '<i class="fas fa-microphone-lines"></i>';
		} else if (state === 'listening') {
			this.voiceBtn.title = '连续语音对话中（点击关闭）';
			this.voiceBtn.innerHTML = '<i class="fas fa-headset"></i>';
		} else {
			this.voiceBtn.title = '开启连续语音对话';
			this.voiceBtn.innerHTML = '<i class="fas fa-microphone"></i>';
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
		// 输入内容变化：实时切换「文件导入 / 发送」按钮
		this.messageInput?.addEventListener('input', () => {
			this.autoResizeTextarea();
			this.updateActionButton();
		});

		// 文件导入按钮 → 触发系统文件选择器
		this.fileImportBtn?.addEventListener('click', () => this.fileInput?.click());
		this.fileInput?.addEventListener('change', (e) => {
			const files = e.target.files;
			if (files && files.length) this.fileManager.handleFileSelect(Array.from(files));
			e.target.value = '';
		});

		// 初始为「空状态」：显示文件导入按钮
		this.updateActionButton();
	}

	/**
	 * 输入框空状态时显示文件导入按钮，有内容（文字或待发送附件）时显示发送按钮
	 */
	updateActionButton() {
		const hasText = !!this.messageInput?.value.trim();
		const hasFiles = !!this.fileManager && this.fileManager.previews.length > 0;
		const showSend = hasText || hasFiles || this.isLoading;
		this.sendButton?.toggleAttribute('hidden', !showSend);
		this.fileImportBtn?.toggleAttribute('hidden', showSend);
	}

	autoResizeTextarea() {
		if (this.messageInput) {
			this.messageInput.style.height = 'auto';
			this.messageInput.style.height = Math.min(this.messageInput.scrollHeight, 120) + 'px';
		}
	}

	initWebSocket() {
		// 与电脑端保持一致：WebSocket 连接同源 /ws（HTTP 直连时为后端端口，HTTPS 代理时走代理隧道）
		// channel=message-terminal 为该终端声明的接入频道（供控制台在线探针识别）
		const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
		const wsUrl = `${protocol}//${window.location.host}/ws?channel=message-terminal`;
		this.wsClient = new WebSocketClient(wsUrl);
		this.wsClient.onMessage((message) => this.handleWebSocketMessage(message));
		this.wsClient.onError((error) => { console.error('WebSocket error:', error); this.showError('连接错误，请刷新页面'); });
		this.wsClient.connect();
	}

	/**
	 * 渲染引擎 iframe：按 config/general.json 的 system.character_renderer 选择
	 * 月岩引擎（lunar.rendering_engine）、Live2D 引擎层（lunar.live2d_engine）
	 * 或 GIF 动图引擎（lunar.gif_engine）。
	 * WsBridge 连接同源 /ws，与主 WebSocket 连接保持一致（与电脑端 main.js 一致）；
	 * 配置读取失败或取值非法时回退 bedrock。
	 */
	async setupRendererFrame() {
		await setupSharedRendererFrame('rendererFrame');
	}

	async handleWebSocketMessage(message) {
		switch (message.type) {
			case 'context':
			case 'image':
				// 月华回应感知：助手侧消息到达即解除连续语音的发送限制
				VoiceChat.notifyAssistantReply();
				break;
		}

		switch (message.type) {
			case 'context':
				await this.handleContextMessage(message.data || {});
				break;

			case 'image': {
				// 图片消息推送：直接使用 multimedia_preview 组件弹窗显示（缩放/拖拽/重置）
				const data = message.data || {};
				const images = data.images || [];
				const label = data.sticker ? '表情包' : '图片';
				for (const image of images) {
					// 图片统一以 URL 传递：/file/read/... 与 http(s):// 直接使用；其余走服务读取端点
					const imageUrl = (image.startsWith('data:') || image.startsWith('http') || image.startsWith('/'))
						? image
						: '/file/read/' + image.replace(/^\.?\//, '');
					// 先进入历史列表，再弹预览
					this.historyView?.appendLive({
						role: 'assistant', categories: ['image'], content: '',
						imageSrc: imageUrl, imageLabel: label, timestamp: Date.now()
					});
					if (window.previewImage) {
						window.previewImage(imageUrl, label);
					}
				}
				break;
			}

			case 'engine':
				// 引擎控制消息：仅供引擎侧消费，不在响应展示区显示
				break;

			case 'error':
				this.showError(message.data?.content || '发生错误');
				this.setLoadingState(false);
				break;
		}
	}

	/**
	 * 处理 context 类型消息：先渲染文本，再异步播报。
	 * 文字渲染不依赖任何音频事件（decode/start/ended），音频失败也不影响已显示的文本。
	 */
	async handleContextMessage(data) {
		const subType = data.type || 'text';
		const content = data.content || '';
		const audio = data.audio || '';

		// 音乐创作消息：后端已渲染音频，统一交给共享播放器展示（播放器 + 波形）
		if (subType === 'music') {
			this.showMusic(data);
			this.historyView?.appendLive({
				role: 'assistant', categories: ['music'], content: '', audio,
				musicTitle: data.title || '', musicSummary: data.summary || '',
				musicDuration: Number(data.duration) || 0, timestamp: Date.now()
			});
			this.setLoadingState(false);
			return;
		}
		// 文本先落到历史视图（与桌面端 addMessage 同序），再做播报
		if (content) {
			this.historyView?.appendLive({
				role: 'assistant',
				categories: audio ? ['text', 'voice'] : ['text'],
				content,
				audio,
				timestamp: Date.now()
			});
		}
		if (audio) {
			AudioQueue.enqueue(audio);
			VoiceChat.onAudioPlaybackChange();
		}
		this.setLoadingState(false);
	}

	/** 展示后端推送的音乐：直接复用共享播放器（与电脑端同一套播放器 + 波形） */
	showMusic(data) {
		const dock = document.getElementById('musicDock');
		if (!dock || !window.LunarMusicPlayer) return;
		const audioUrl = String(data.audio || '');
		dock.innerHTML = '';
		dock.appendChild(window.LunarMusicPlayer.createCard({
			title: data.title || '月华演奏曲',
			summary: data.summary || '',
			duration: Number(data.duration) || 0,
			audio: audioUrl,
			onClose: () => {
				// 用户手动收起：记住这首，刷新/重开页面不再自动复活
				window.LunarMusicPlayer.dismiss(audioUrl);
				dock.hidden = true;
				dock.innerHTML = '';
			}
		}));
		dock.hidden = false;
	}

	/** 启动时恢复最近一条音乐记录（手机端无消息列表，重开页面仍可继续播放） */
	async restoreLatestMusic() {
		try {
			const res = await fetch('/file/read/database/messages.json');
			if (!res.ok) return;
			const records = await res.json();
			if (!Array.isArray(records)) return;
			if (!window.LunarMusicPlayer) return;
			const latest = records.slice().reverse().find(item => item
				&& Array.isArray(item.categories) && item.categories.includes('music')
				&& item.audio
				&& !window.LunarMusicPlayer.isDismissed(item.audio));
			if (!latest) return;
			this.showMusic({
				title: latest.musicTitle || '',
				summary: latest.musicSummary || '',
				duration: latest.musicDuration,
				audio: latest.audio
			});
		} catch (e) {
			// 历史文件不可用时静默：不影响实时推送
		}
	}

	async handleSend() {
		const text = this.messageInput?.value.trim();
		const hasFiles = !!this.fileManager && this.fileManager.previews.length > 0;
		if (!text && !hasFiles) return;
		if (this.isLoading) return;

		this.setLoadingState(true);

		// ==== 乐观渲染：先上屏，再上传/落库（不等服务端回执） ====
		const localId = `live-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
		const previews = this.fileManager ? this.fileManager.previews.slice() : [];
		const categories = new Set();
		if (text) categories.add('text');
		const optimisticAttachments = [];
		for (const preview of previews) {
			if (preview.type === 'image' || preview.type === 'video' || preview.type === 'audio') {
				// 独立 blob URL：清空待发送预览时不会连带撤销已上屏消息的图片
				optimisticAttachments.push({
					type: preview.type,
					src: URL.createObjectURL(preview.file),
					label: preview.name
				});
			} else {
				// 非媒体文件：本地气泡保留文件名与大小（信封内对应 name / bytes）
				optimisticAttachments.push({
					type: 'file',
					label: preview.name,
					size: formatFileSize(preview.file.size)
				});
			}
			if (preview.type === 'image' || preview.type === 'video') categories.add('image');
			else if (preview.type === 'audio') categories.add('voice');
			else categories.add('file');
		}
		this.historyView?.appendLive({
			id: localId,
			role: 'user',
			categories: categories.size ? Array.from(categories) : ['text'],
			content: text,
			attachments: optimisticAttachments.length ? optimisticAttachments : undefined,
			status: 'sending',
			timestamp: Date.now()
		});

		try {
			// 附件上传并构建内容信封（图片/视频/音频/文本文件）
			let contentBlocks = [];
			if (hasFiles) {
				const results = await Promise.all(
					this.fileManager.previews.map(p => this.fileManager.processFileUpload(p))
				);
				contentBlocks = await this.fileManager.buildFileContentBlocks(results);
			}

			// 输入框里的 URL 由前端抽成 webpage 信封，正文保持纯文本
			const textSplit = await extractWebpageEnvelopes(text);
			contentBlocks.push(...textSplit.envelopes);

			// QQ 适配器标准格式：主文本带用户名前缀（与电脑端一致）
			let content;
			if (contentBlocks.length) {
				content = [
					...(textSplit.text ? [{ type: 'text', text: getUserNamePrefix() + textSplit.text }] : []),
					...contentBlocks,
				];
			} else {
				content = getUserNamePrefix() + textSplit.text;
			}

			await sendMessages([{ role: 'user', content }]);

			// 发送成功后清空输入与待发送附件
			this.historyView?.updateLiveStatus(localId, 'sent');
			this.messageInput.value = '';
			this.autoResizeTextarea();
			this.fileManager.clearFilePreviews();
		} catch (error) {
			console.error('Send error:', error);
			// 失败：消息气泡标记失败，输入与附件保留，用户可直接重试
			this.historyView?.updateLiveStatus(localId, 'failed');
			this.showError(error instanceof Error ? error.message : '发送失败');
		} finally {
			this.setLoadingState(false);
		}
	}

	setLoadingState(loading) {
		this.isLoading = loading;
		if (this.sendButton) {
			this.sendButton.disabled = loading;
			this.sendButton.innerHTML = loading
				? '<span class="loading-indicator"></span>'
				: '<i class="fas fa-paper-plane"></i>';
		}
		this.updateActionButton();
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
