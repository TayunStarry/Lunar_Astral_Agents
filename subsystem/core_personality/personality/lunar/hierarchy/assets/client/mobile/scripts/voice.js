import { AudioQueue } from './tts.js';
import { sendMessages } from './fetch.js';
import { fileToRawBase64 } from './file.js';
import { getUserNamePrefix } from './username.js';

/**
 * 连续语音对话管理器（与电脑端 voice-record.js 机制一致）
 *
 * 声学回声消除（AEC）+ VAD 自动录音发送：
 * - getUserMedia 申请带 echoCancellation / noiseSuppression / autoGainControl 的麦克风，
 *   从采集源头抑制月华语音外放回灌
 * - 常驻监听：VAD 逐帧检测说话开始 / 结束，自动裁剪静音并编码 WAV 发送
 * - 月华播放语音期间暂停采集，避免把自身外放当成用户输入
 * - 回应感知：收到月华回应（WS context / image）前，两次语音发送间隔不足冷却时长时丢弃
 */

// ==== 连续语音参数（与电脑端保持一致） ====
const TARGET_RATE = 16000;        // 目标采样率（音频理解标准输入）
const FRAME_SIZE = 1024;          // VAD 分析帧长
const START_FRAMES = 3;           // 连续超阈值帧数 → 判定说话开始
const END_SILENCE_MS = 800;       // 静音持续时长 → 判定说话结束
const TAIL_KEEP_MS = 200;         // 结束判定后保留的尾部静音
const MIN_SPEECH_MS = 250;        // 有效人声最短时长（滤除瞬时噪声）
const MAX_SPEECH_MS = 30000;      // 单次语音最长时长（超时强制结束）
const PREROLL_FRAMES = 5;         // 说话起点前保留的缓冲帧，避免吃掉开头
const ABS_THRESHOLD = 0.012;      // 绝对能量阈值下限
const NOISE_FACTOR = 2.2;         // 相对本底噪声的倍数阈值
const AI_RESUME_DELAY_MS = 350;   // AI 播放结束后恢复监听的延迟
const REPLY_COOLDOWN_MS = 30000;  // 未收到回应时，两次语音发送的最小间隔

export class VoiceChatManager {
    /** 是否处于持续监听 */
    enabled = false;
    /** 是否正在采集用户语音 */
    capturing = false;
    /** 是否正在发送语音 */
    sending = false;
    /** 是否仍在等待月华对上次语音的回应 */
    awaitingReply = false;
    /** 上次语音发送的时间戳 */
    lastSentAt = 0;

    // 采集链路
    stream = null;
    audioContext = null;
    source = null;
    processor = null;
    muteGain = null;

    // VAD 状态
    preroll = [];
    frames = [];
    aboveCount = 0;
    silenceMs = 0;
    speechMs = 0;
    voicedMs = 0;
    noiseFloor = 0.004;
    resumeAt = 0;

    // 回调
    onStatusChangeCallback = null;
    onErrorCallback = null;

    // ==== 公共接口 ====

    /**
     * 切换连续语音状态
     *
     * @returns {Promise<boolean>} 切换后的启用状态
     */
    async toggle() {
        if (this.enabled) {
            await this.disable();
        } else {
            await this.enable();
        }
        return this.enabled;
    }

    /** 开启连续语音：申请带 AEC 的麦克风并常驻监听 */
    async enable() {
        if (this.enabled) return;
        if (!navigator.mediaDevices || !window.AudioContext) {
            if (this.onErrorCallback) this.onErrorCallback('unsupported', '当前环境不支持连续语音');
            return;
        }
        try {
            // 声学回声消除 + 噪声抑制 + 自动增益：从采集源头抑制月华外放回灌
            this.stream = await navigator.mediaDevices.getUserMedia({
                audio: { echoCancellation: true, noiseSuppression: true, autoGainControl: true }
            });
        } catch (err) {
            if (this.onErrorCallback) this.onErrorCallback('not-allowed', '麦克风权限被拒绝，无法开启连续语音');
            return;
        }
        try {
            this.audioContext = new AudioContext({ sampleRate: TARGET_RATE });
        } catch (err) {
            // 部分环境不支持指定采样率，退回默认采样率（WAV 按实际采样率编码）
            this.audioContext = new AudioContext();
        }
        if (this.audioContext.state === 'suspended') await this.audioContext.resume();

        this.source = this.audioContext.createMediaStreamSource(this.stream);
        this.processor = this.audioContext.createScriptProcessor(FRAME_SIZE, 1, 1);
        this.muteGain = this.audioContext.createGain();
        this.muteGain.gain.value = 0;   // 采集链路静音，避免自激啸叫
        this.processor.onaudioprocess = (e) => this.handleFrame(e.inputBuffer.getChannelData(0));
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

    /** 关闭连续语音：释放采集链路与麦克风 */
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

    /** 月华回应感知：WebSocket 收到助手消息时由外部调用，解除语音发送限制 */
    notifyAssistantReply() {
        this.awaitingReply = false;
    }

    /**
     * 音频播放状态变化回调（tts.js 播放队列结束时动态调用）
     * 采集侧逐帧检测月华是否在说话，此处无需额外处理，保留仅为兼容调用方
     */
    onAudioPlaybackChange() { /* 逐帧检测已覆盖 */ }

    // ==== 回调注册 ====

    /** 注册状态变更回调（state: idle | listening | capturing） */
    onStatusChange(callback) {
        this.onStatusChangeCallback = callback;
    }

    /** 注册错误回调（errorType: 'not-allowed' | 'unsupported'） */
    onError(callback) {
        this.onErrorCallback = callback;
    }

    /** 通知状态变更 */
    notifyStatusChange(state) {
        if (this.onStatusChangeCallback) {
            this.onStatusChangeCallback(state);
        }
    }

    // ==== 语音活动检测（VAD） ====

    /** 重置当前一轮语音采集状态 */
    resetUtterance() {
        this.capturing = false;
        this.frames = [];
        this.aboveCount = 0;
        this.silenceMs = 0;
        this.speechMs = 0;
        this.voicedMs = 0;
    }

    /** 单帧时长（毫秒），采样率取实际采集上下文 */
    frameMs() {
        return (FRAME_SIZE / (this.audioContext ? this.audioContext.sampleRate : TARGET_RATE)) * 1000;
    }

    /** 月华播放识别：AI 正在说话时一律不采集，避免把自身外放的声音当成用户输入 */
    isAssistantSpeaking() {
        const status = AudioQueue.getStatus();
        return status.playing || status.queueLength > 0;
    }

    /** 逐帧判定：说话开始 → 采集 → 说话结束 */
    handleFrame(samples) {
        if (!this.enabled) return;
        const frameMs = this.frameMs();
        const rms = this.frameRms(samples);

        if (this.isAssistantSpeaking()) {
            // 月华说话期间：丢弃进行中的采集，并在播放结束后留出恢复延迟
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
            // 未进入语音：维护起点前缓冲 + 自适应本底噪声
            if (!voiced) this.noiseFloor = this.noiseFloor * 0.98 + rms * 0.02;
            this.aboveCount = voiced ? this.aboveCount + 1 : 0;
            this.preroll.push(new Float32Array(samples));
            if (this.preroll.length > PREROLL_FRAMES) this.preroll.shift();
            if (this.aboveCount >= START_FRAMES) this.beginUtterance();
            return;
        }

        // 采集中：累计帧与静音 / 人声时长，静音足够久或超时即结束
        this.frames.push(new Float32Array(samples));
        this.speechMs += frameMs;
        this.silenceMs = voiced ? 0 : this.silenceMs + frameMs;
        if (voiced) this.voicedMs += frameMs;
        if (this.silenceMs >= END_SILENCE_MS || this.speechMs >= MAX_SPEECH_MS) {
            this.finishUtterance(frameMs);
        }
    }

    /** 帧能量（RMS） */
    frameRms(samples) {
        let sum = 0;
        for (let i = 0; i < samples.length; i++) sum += samples[i] * samples[i];
        return Math.sqrt(sum / samples.length);
    }

    /** 说话开始：把起点前缓冲一并纳入，避免吃掉开头音节 */
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

    /** 说话结束：裁剪多余尾部静音后自动发送 */
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
        if (voicedMs < MIN_SPEECH_MS) return;   // 过短视为环境噪声，直接丢弃
        this.sendUtterance(frames, sampleRate);
    }

    // ==== 自动发送 ====

    /** 发送资格判定：已收到月华回应，或距上次发送已达冷却时长，方可再次发送 */
    canSendNow() {
        return !this.awaitingReply || (Date.now() - this.lastSentAt) >= REPLY_COOLDOWN_MS;
    }

    /** PCM → WAV → 直接推送给月华，无需用户点击发送 */
    async sendUtterance(frames, sampleRate) {
        if (this.sending) return;
        // 节流：月华尚未回应且距上次发送不足 30 秒时，丢弃本次语音并提示（与电脑端一致）
        if (!this.canSendNow()) {
            if (this.onErrorCallback) this.onErrorCallback('rate-limited');
            return;
        }
        this.sending = true;
        try {
            const wavBlob = this.encodeWavBlob(this.flattenFrames(frames), sampleRate);
            const base64Data = await fileToRawBase64(wavBlob);
            // QQ 适配器标准格式：[用户: xxx]: [语音] 前缀 + 音频内容块（统一数组保持时序）
            await sendMessages([{
                role: 'user',
                content: [
                    { type: 'text', text: getUserNamePrefix() + '[语音] ' },
                    { type: 'input_audio', input_audio: { data: base64Data, format: 'wav' } }
                ]
            }]);
            // 发送成功后进入等待回应状态，直至收到月华回应或冷却期满
            this.lastSentAt = Date.now();
            this.awaitingReply = true;
        } catch (err) {
            if (this.onErrorCallback) this.onErrorCallback('send-failed', '语音发送失败');
        } finally {
            this.sending = false;
        }
    }

    /** 合并多帧 PCM 为连续采样 */
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

    /** 将 PCM 浮点采样编码为 16 位 WAV Blob */
    encodeWavBlob(samples, sampleRate) {
        const buffer = new ArrayBuffer(44 + samples.length * 2);
        const view = new DataView(buffer);
        // RIFF 头
        this.writeString(view, 0, 'RIFF');
        view.setUint32(4, 36 + samples.length * 2, true);
        this.writeString(view, 8, 'WAVE');
        // fmt 块（16位 PCM 单声道）
        this.writeString(view, 12, 'fmt ');
        view.setUint32(16, 16, true);
        view.setUint16(20, 1, true);
        view.setUint16(22, 1, true);
        view.setUint32(24, sampleRate, true);
        view.setUint32(28, sampleRate * 2, true);
        view.setUint16(32, 2, true);
        view.setUint16(34, 16, true);
        // data 块
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

/** 全局连续语音对话单例 */
export const VoiceChat = new VoiceChatManager();
