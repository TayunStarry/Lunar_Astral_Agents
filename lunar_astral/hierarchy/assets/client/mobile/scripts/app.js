import { WebSocketClient } from './socket.js';
import { AudioQueue } from './tts.js';
import { VoiceChat } from './voice.js';
import { Toast } from './toast.js';
import { sendMessages } from './fetch.js';
import { initMusicRenderer, renderMusicScore, playRenderedAudio } from './music_renderer.js';
import { initUserName, getUserNamePrefix } from './username.js';

class LunarCoreApp {
	wsClient = null;
	isLoading = false;
	messageInput = null;
	sendButton = null;
	errorToast = null;
	voiceBtn = null;

	constructor() {
		// 加载后端用户名（lunar_config.json 的 server.user_name）
		initUserName();
		this.initElements();
		this.initVoiceChat();
		this.initEventListeners();
		this.initWebSocket();
		this.setupRendererFrame();
		initMusicRenderer();
	}

	initElements() {
		this.messageInput = document.getElementById('messageInput');
		this.sendButton = document.getElementById('sendButton');
		this.errorToast = document.getElementById('errorToast');
		this.voiceBtn = document.getElementById('voiceBtn');
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
		this.messageInput?.addEventListener('input', () => { this.autoResizeTextarea(); });
	}

	autoResizeTextarea() {
		if (this.messageInput) {
			this.messageInput.style.height = 'auto';
			this.messageInput.style.height = Math.min(this.messageInput.scrollHeight, 120) + 'px';
		}
	}

	initWebSocket() {
		// 与电脑端保持一致：WebSocket 连接后端服务端口
		const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
		const wsUrl = `${protocol}//${window.location.hostname}:36789/ws`;
		this.wsClient = new WebSocketClient(wsUrl);
		this.wsClient.onMessage((message) => this.handleWebSocketMessage(message));
		this.wsClient.onError((error) => { console.error('WebSocket error:', error); this.showError('连接错误，请刷新页面'); });
		this.wsClient.connect();
	}

	/** 渲染引擎 iframe：指向智能体所在后端（36789），与主 WebSocket 连接保持一致（与电脑端 main.js 一致） */
	setupRendererFrame() {
		const frame = document.getElementById('rendererFrame');
		if (!frame || !frame.dataset.src) return;
		frame.src = `${frame.dataset.src}&ws=${window.location.hostname}:36789`;
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
				for (const imageBase64 of images) {
					const imageUrl = imageBase64.startsWith('data:') || imageBase64.startsWith('http')
						? imageBase64
						: 'data:image/jpeg;base64,' + imageBase64;
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

	/** 处理 context 类型消息：不显示文本内容，仅处理语音播报与音乐 */
	async handleContextMessage(data) {
		const subType = data.type || 'text';
		const content = data.content || '';
		const audio = data.audio || '';

		// 音乐创作消息
		if (subType === 'music') {
			if (content) renderMusicScore(content);
			return;
		}
		// 后端渲染音频就绪（FluidSynth + SoundFont）
		if (subType === 'music_audio') {
			try {
				const audioData = JSON.parse(content || '{}');
				if (audioData.type === 'audio_ready' && audioData.audio_url) {
					playRenderedAudio(audioData.audio_url, audioData.file_name);
				}
			} catch (e) {
				console.warn('[音乐] 音频数据解析失败:', e);
			}
			return;
		}

		// 文本响应不显示，仅播报携带的 TTS 语音
		if (audio) {
			AudioQueue.enqueue(audio);
			VoiceChat.onAudioPlaybackChange();
		}
		this.setLoadingState(false);
	}

	async handleSend() {
		const text = this.messageInput?.value.trim();
		if (!text) return;
		if (this.isLoading) return;

		this.setLoadingState(true);

		try {
			// QQ 适配器标准格式：发送给月华的文本带用户名前缀（与电脑端一致）
			await sendMessages([{ role: 'user', content: getUserNamePrefix() + text }]);
			this.messageInput.value = '';
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
			this.sendButton.innerHTML = loading
				? '<span class="loading-indicator"></span>'
				: '<i class="fas fa-paper-plane"></i>';
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
