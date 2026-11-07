/**
 * 星月智能 · 桌面端 / 移动端共享音频播放队列（TTS）
 *
 * 顺序播放收到的 Base64 WAV；decodeAndPlay / 队列 / 停止逻辑完全共用。
 * 移动端开启 enhance 走「高通 → 低架 → 高架 → 增益」处理链并支持音色参数；
 * 桌面端 enhance=false，保持直连输出。
 */

export class AudioQueueManager {
	/**
	 * @param {object} [options]
	 * @param {boolean} [options.enhance=false] 是否启用移动端的音频增强链路
	 * @param {()=>void} [options.onIdle] 队列播空时回调（移动端用于通知语音识别）
	 * @param {(error:Error)=>void} [options.onError]
	 */
	constructor(options = {}) {
		this.audioContext = null;
		this.currentSource = null;
		this.queue = [];
		this.playing = false;
		this.playedCount = 0;
		this.errorCount = 0;
		this.enhance = !!options.enhance;
		this.onIdle = options.onIdle || null;
		this.onError = options.onError || null;
		this.audioSettings = {
			noiseReduction: 0.15,
			playbackSpeed: 1.0,
			trebleBoost: 0.0,
			bassCut: 0.0
		};
	}

	enqueue(audioBase64) {
		if (!audioBase64) {
			console.warn('AudioQueue: 收到空音频数据，已忽略');
			return;
		}
		this.queue.push(audioBase64);
		if (!this.playing) this.playNext();
	}

	playNext() {
		if (this.queue.length === 0) {
			this.playing = false;
			this.currentSource = null;
			this.onIdle?.();
			return;
		}
		this.playing = true;
		const audioBase64 = this.queue.shift();
		try {
			this.decodeAndPlay(this.base64ToArrayBuffer(audioBase64));
		} catch (err) {
			console.error('AudioQueue: 音频数据处理失败:', err);
			this.errorCount++;
			this.onError?.(err);
			this.playNext();
		}
	}

	decodeAndPlay(arrayBuffer) {
		if (!this.audioContext) {
			this.audioContext = new (window.AudioContext || window.webkitAudioContext)();
		}
		if (this.audioContext.state === 'suspended') this.audioContext.resume();
		this.audioContext.decodeAudioData(
			arrayBuffer,
			buffer => this.playAudioBuffer(buffer),
			err => {
				console.error('AudioQueue: 音频解码失败:', err);
				this.errorCount++;
				this.onError?.(err);
				this.playNext();
			}
		);
	}

	playAudioBuffer(audioBuffer) {
		const source = this.audioContext.createBufferSource();
		source.buffer = audioBuffer;

		if (!this.enhance) {
			source.connect(this.audioContext.destination);
		} else {
			source.playbackRate.value = this.audioSettings.playbackSpeed;
			const gainNode = this.audioContext.createGain();
			gainNode.gain.value = 1.0;

			const noiseGateFilter = this.audioContext.createBiquadFilter();
			noiseGateFilter.type = 'highpass';
			noiseGateFilter.frequency.value = 80 + (this.audioSettings.noiseReduction * 200);
			noiseGateFilter.Q.value = 0.5;

			const lowShelfFilter = this.audioContext.createBiquadFilter();
			lowShelfFilter.type = 'lowshelf';
			lowShelfFilter.frequency.value = 200;
			lowShelfFilter.gain.value = -this.audioSettings.bassCut * 20;

			const highShelfFilter = this.audioContext.createBiquadFilter();
			highShelfFilter.type = 'highshelf';
			highShelfFilter.frequency.value = 3000;
			highShelfFilter.gain.value = this.audioSettings.trebleBoost * 15;

			source.connect(noiseGateFilter);
			noiseGateFilter.connect(lowShelfFilter);
			lowShelfFilter.connect(highShelfFilter);
			highShelfFilter.connect(gainNode);
			gainNode.connect(this.audioContext.destination);
		}

		this.currentSource = source;
		source.onended = () => {
			this.currentSource = null;
			this.playedCount++;
			this.playNext();
		};
		source.start();
	}

	stop() {
		if (this.currentSource) {
			try {
				this.currentSource.onended = null;
				this.currentSource.stop();
			} catch { /* 忽略已停止的源 */ }
			this.currentSource = null;
		}
		this.queue = [];
		this.playing = false;
	}

	getStatus() {
		return {
			queueLength: this.queue.length,
			playing: this.playing,
			playedCount: this.playedCount,
			errorCount: this.errorCount
		};
	}

	setAudioSetting(setting, value) {
		if (Object.prototype.hasOwnProperty.call(this.audioSettings, setting)) {
			this.audioSettings[setting] = Math.max(0, Math.min(1, value));
		}
	}

	base64ToArrayBuffer(base64) {
		const binary = atob(base64);
		const bytes = new Uint8Array(binary.length);
		for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i);
		return bytes.buffer;
	}
}

/** 兼容各端直接 import 单例的写法（可选） */
export function createAudioQueue(options) {
	return new AudioQueueManager(options);
}
