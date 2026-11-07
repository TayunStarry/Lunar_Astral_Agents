/**
 * 星月智能 · 桌面端 / 移动端共享连续语音对话
 *
 * AEC + VAD 自动录音发送机制完全一致；各端通过 onUtterance 注入发送逻辑：
 * - 桌面端：本地历史渲染、引用块、离线提示、句柄回填
 * - 移动端：直接发送 Standard 消息
 */
import { fileToRawBase64 } from './utils.js';

const TARGET_RATE = 16000;
const FRAME_SIZE = 1024;
const START_FRAMES = 3;
const END_SILENCE_MS = 800;
const TAIL_KEEP_MS = 200;
const MIN_SPEECH_MS = 250;
const MAX_SPEECH_MS = 30000;
const PREROLL_FRAMES = 5;
const ABS_THRESHOLD = 0.012;
const NOISE_FACTOR = 2.2;
const AI_RESUME_DELAY_MS = 350;
const REPLY_COOLDOWN_MS = 30000;

export class VoiceChatManager {
	/**
	 * @param {object} options
	 * @param {import('./audio-queue.js').AudioQueueManager} options.audioQueue
	 * @param {(payload:{file:File,wavBlob:Blob,base64Data:string,sampleRate:number})=>Promise<boolean|void>} options.onUtterance
	 *        返回 false 表示未真正发送（如离线），不进入等待回应状态
	 * @param {(state:'idle'|'listening'|'capturing')=>void} [options.onStatusChange]
	 * @param {(type:string, message?:string)=>void} [options.onError]
	 */
	constructor(options = {}) {
		this.audioQueue = options.audioQueue;
		this.onUtterance = options.onUtterance;
		this.onStatusChangeCallback = options.onStatusChange || null;
		this.onErrorCallback = options.onError || null;

		this.enabled = false;
		this.capturing = false;
		this.sending = false;
		this.awaitingReply = false;
		this.lastSentAt = 0;

		this.stream = null;
		this.audioContext = null;
		this.source = null;
		this.processor = null;
		this.muteGain = null;

		this.preroll = [];
		this.frames = [];
		this.aboveCount = 0;
		this.silenceMs = 0;
		this.speechMs = 0;
		this.voicedMs = 0;
		this.noiseFloor = 0.004;
		this.resumeAt = 0;
	}

	async toggle() {
		if (this.enabled) await this.disable();
		else await this.enable();
		return this.enabled;
	}

	async enable() {
		if (this.enabled) return;
		if (!navigator.mediaDevices || !window.AudioContext) {
			this.onErrorCallback?.('unsupported', '当前环境不支持连续语音');
			return;
		}
		try {
			this.stream = await navigator.mediaDevices.getUserMedia({
				audio: { echoCancellation: true, noiseSuppression: true, autoGainControl: true }
			});
		} catch {
			this.onErrorCallback?.('not-allowed', '麦克风权限被拒绝，无法开启连续语音');
			return;
		}
		try {
			this.audioContext = new AudioContext({ sampleRate: TARGET_RATE });
		} catch {
			this.audioContext = new AudioContext();
		}
		if (this.audioContext.state === 'suspended') await this.audioContext.resume();

		this.source = this.audioContext.createMediaStreamSource(this.stream);
		this.processor = this.audioContext.createScriptProcessor(FRAME_SIZE, 1, 1);
		this.muteGain = this.audioContext.createGain();
		this.muteGain.gain.value = 0; // 采集链路静音，避免自激啸叫
		this.processor.onaudioprocess = e => this.handleFrame(e.inputBuffer.getChannelData(0));
		this.source.connect(this.processor);
		this.processor.connect(this.muteGain);
		this.muteGain.connect(this.audioContext.destination);

		// 设备被拔出 / 权限被回收时自动退出监听
		this.stream.getAudioTracks().forEach(track => {
			track.addEventListener('ended', () => this.disable());
		});

		this.enabled = true;
		this.noiseFloor = 0.004;
		this.resumeAt = 0;
		this.preroll = [];
		this.resetUtterance();
		this.notifyStatusChange('listening');
	}

	async disable() {
		if (!this.enabled && !this.stream) return;
		this.enabled = false;
		this.resetUtterance();
		this.preroll = [];
		if (this.processor) {
			this.processor.onaudioprocess = null;
			this.processor.disconnect();
			this.processor = null;
		}
		if (this.source) {
			this.source.disconnect();
			this.source = null;
		}
		if (this.muteGain) {
			this.muteGain.disconnect();
			this.muteGain = null;
		}
		if (this.stream) {
			this.stream.getTracks().forEach(track => track.stop());
			this.stream = null;
		}
		if (this.audioContext) {
			this.audioContext.close();
			this.audioContext = null;
		}
		this.notifyStatusChange('idle');
	}

	/** 月华回应感知：外部收到助手消息时调用，解除语音发送限制 */
	notifyAssistantReply() {
		this.awaitingReply = false;
	}

	/** 音频播放状态变化（逐帧检测已覆盖，保留兼容） */
	onAudioPlaybackChange() { /* no-op */ }

	onStatusChange(callback) { this.onStatusChangeCallback = callback; }
	onError(callback) { this.onErrorCallback = callback; }

	notifyStatusChange(state) {
		this.onStatusChangeCallback?.(state);
	}

	resetUtterance() {
		this.capturing = false;
		this.frames = [];
		this.aboveCount = 0;
		this.silenceMs = 0;
		this.speechMs = 0;
		this.voicedMs = 0;
	}

	frameMs() {
		return (FRAME_SIZE / (this.audioContext ? this.audioContext.sampleRate : TARGET_RATE)) * 1000;
	}

	isAssistantSpeaking() {
		if (this.audioQueue?.getStatus) {
			const status = this.audioQueue.getStatus();
			if (status.playing || status.queueLength > 0) return true;
		}
		return !!(this.audioQueue && (this.audioQueue.playing || this.audioQueue.currentSource));
	}

	handleFrame(samples) {
		if (!this.enabled) return;
		const frameMs = this.frameMs();
		const rms = this.frameRms(samples);

		if (this.isAssistantSpeaking()) {
			if (this.capturing) {
				this.resetUtterance();
				this.notifyStatusChange('listening');
			}
			this.resumeAt = Date.now() + AI_RESUME_DELAY_MS;
			return;
		}
		if (Date.now() < this.resumeAt) return;

		const threshold = Math.max(ABS_THRESHOLD, this.noiseFloor * NOISE_FACTOR);
		const voiced = rms > threshold;

		if (!this.capturing) {
			if (!voiced) this.noiseFloor = this.noiseFloor * 0.98 + rms * 0.02;
			this.aboveCount = voiced ? this.aboveCount + 1 : 0;
			this.preroll.push(new Float32Array(samples));
			if (this.preroll.length > PREROLL_FRAMES) this.preroll.shift();
			if (this.aboveCount >= START_FRAMES) this.beginUtterance();
			return;
		}

		this.frames.push(new Float32Array(samples));
		this.speechMs += frameMs;
		this.silenceMs = voiced ? 0 : this.silenceMs + frameMs;
		if (voiced) this.voicedMs += frameMs;
		if (this.silenceMs >= END_SILENCE_MS || this.speechMs >= MAX_SPEECH_MS) {
			this.finishUtterance(frameMs);
		}
	}

	frameRms(samples) {
		let sum = 0;
		for (let i = 0; i < samples.length; i++) sum += samples[i] * samples[i];
		return Math.sqrt(sum / samples.length);
	}

	beginUtterance() {
		this.capturing = true;
		this.frames = this.preroll.slice();
		this.preroll = [];
		this.aboveCount = 0;
		this.silenceMs = 0;
		this.speechMs = 0;
		this.voicedMs = START_FRAMES * this.frameMs();
		this.notifyStatusChange('capturing');
	}

	finishUtterance(frameMs) {
		const sampleRate = this.audioContext ? this.audioContext.sampleRate : TARGET_RATE;
		this.capturing = false;
		const dropFrames = Math.max(0, Math.floor((this.silenceMs - TAIL_KEEP_MS) / frameMs));
		if (dropFrames > 0) this.frames.splice(Math.max(this.frames.length - dropFrames, 0), dropFrames);
		const frames = this.frames;
		const voicedMs = this.voicedMs;
		this.resetUtterance();
		this.preroll = [];
		this.notifyStatusChange(this.enabled ? 'listening' : 'idle');
		if (voicedMs < MIN_SPEECH_MS) return;
		this.sendUtterance(frames, sampleRate);
	}

	canSendNow() {
		return !this.awaitingReply || (Date.now() - this.lastSentAt) >= REPLY_COOLDOWN_MS;
	}

	async sendUtterance(frames, sampleRate) {
		if (this.sending) return;
		if (!this.canSendNow()) {
			this.onErrorCallback?.('rate-limited');
			return;
		}
		if (!this.onUtterance) {
			this.onErrorCallback?.('send-failed', '未配置语音发送逻辑');
			return;
		}
		this.sending = true;
		try {
			const wavBlob = this.encodeWavBlob(this.flattenFrames(frames), sampleRate);
			const stamp = new Date().toTimeString().slice(0, 8).replace(/:/g, '');
			const file = new File([wavBlob], `语音_${stamp}.wav`, { type: 'audio/wav' });
			const base64Data = await fileToRawBase64(file);
			const sent = await this.onUtterance({ file, wavBlob, base64Data, sampleRate });
			if (sent !== false) {
				this.lastSentAt = Date.now();
				this.awaitingReply = true;
			}
		} catch (err) {
			this.onErrorCallback?.('send-failed', err?.message || '语音发送失败');
		} finally {
			this.sending = false;
		}
	}

	flattenFrames(frames) {
		const total = frames.reduce((n, f) => n + f.length, 0);
		const samples = new Float32Array(total);
		let offset = 0;
		for (const frame of frames) {
			samples.set(frame, offset);
			offset += frame.length;
		}
		return samples;
	}

	encodeWavBlob(samples, sampleRate) {
		const buffer = new ArrayBuffer(44 + samples.length * 2);
		const view = new DataView(buffer);
		this.writeString(view, 0, 'RIFF');
		view.setUint32(4, 36 + samples.length * 2, true);
		this.writeString(view, 8, 'WAVE');
		this.writeString(view, 12, 'fmt ');
		view.setUint32(16, 16, true);
		view.setUint16(20, 1, true);
		view.setUint16(22, 1, true);
		view.setUint32(24, sampleRate, true);
		view.setUint32(28, sampleRate * 2, true);
		view.setUint16(32, 2, true);
		view.setUint16(34, 16, true);
		this.writeString(view, 36, 'data');
		view.setUint32(40, samples.length * 2, true);
		let offset = 44;
		for (let i = 0; i < samples.length; i++, offset += 2) {
			const s = Math.max(-1, Math.min(1, samples[i]));
			view.setInt16(offset, s < 0 ? s * 0x8000 : s * 0x7FFF, true);
		}
		return new Blob([view], { type: 'audio/wav' });
	}

	writeString(view, offset, str) {
		for (let i = 0; i < str.length; i++) {
			view.setUint8(offset + i, str.charCodeAt(i));
		}
	}
}
