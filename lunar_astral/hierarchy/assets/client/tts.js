/**
 * 基于队列的音频播放系统
 *
 * 当接收到WebSocket推送的音频数据时立即加入播放队列，
 * 确保音频播放的顺序性，必须等待当前音频播放完成后才能开始播放队列中的下一个音频。
 * 提供队列状态管理，包括队列长度监控、播放状态指示和异常处理机制。
 */

class AudioQueueManager {
    /** @type {AudioContext|null} Web Audio API上下文 */
    audioContext = null;
    /** @type {AudioBufferSourceNode|null} 当前正在播放的音频源节点 */
    currentSource = null;
    /** @type {string[]} 待播放的音频Base64队列 */
    queue = [];
    /** @type {boolean} 是否正在播放 */
    playing = false;
    /** @type {number} 已播放的音频数量 */
    playedCount = 0;
    /** @type {number} 播放错误计数 */
    errorCount = 0;

    /** 音频处理参数 */
    audioSettings = {
        noiseReduction: 0.15,
        playbackSpeed: 1.0,
        trebleBoost: 0.0,
        bassCut: 0.0
    };

    /**
     * 将Base64编码的WAV音频数据加入播放队列
     *
     * @param {string} audioBase64 - Base64编码的WAV音频数据
     */
    enqueue(audioBase64) {
        if (!audioBase64) {
            console.warn('AudioQueue: 收到空音频数据，已忽略');
            return;
        }
        this.queue.push(audioBase64);

        // 如果当前未在播放，启动播放流程
        if (!this.playing) {
            this.playNext();
        }
    }

    /**
     * 播放队列中的下一个音频
     *
     * 顺序播放机制：仅当当前无音频播放时才从队列头部取出并播放，
     * 播放完成后自动递归调用自身处理队列中的下一个音频。
     */
    playNext() {
        // 队列为空，停止播放
        if (this.queue.length === 0) {
            this.playing = false;
            this.currentSource = null;
            // 通知语音识别：音频播放完成
            this.notifyVoiceChat();
            return;
        }

        this.playing = true;

        // 从队列头部取出音频数据
        const audioBase64 = this.queue.shift();

        try {
            const arrayBuffer = this.base64ToArrayBuffer(audioBase64);
            this.decodeAndPlay(arrayBuffer);
        } catch (err) {
            console.error('AudioQueue: 音频数据处理失败:', err);
            this.errorCount++;
            // 出错时继续播放队列中的下一个
            this.playNext();
        }
    }

    /**
     * 解码音频数据并播放
     *
     * @param {ArrayBuffer} arrayBuffer - WAV音频的ArrayBuffer
     */
    decodeAndPlay(arrayBuffer) {
        if (!this.audioContext) {
            this.audioContext = new (window.AudioContext || window.webkitAudioContext)();
        }

        // 恢复被浏览器挂起的AudioContext
        if (this.audioContext.state === 'suspended') {
            this.audioContext.resume();
        }

        this.audioContext.decodeAudioData(
            arrayBuffer,
            (audioBuffer) => {
                this.playAudioBuffer(audioBuffer);
            },
            (err) => {
                console.error('AudioQueue: 音频解码失败:', err);
                this.errorCount++;
                // 解码失败，继续播放下一个
                this.playNext();
            }
        );
    }

    /**
     * 播放AudioBuffer，构建音频处理链路
     *
     * 音频处理链路：Source → NoiseGate → LowShelf → HighShelf → Gain → Destination
     * 播放完成后自动触发队列中下一个音频的播放。
     *
     * @param {AudioBuffer} audioBuffer - 解码后的音频缓冲区
     */
    playAudioBuffer(audioBuffer) {
        const source = this.audioContext.createBufferSource();
        source.buffer = audioBuffer;
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

        this.currentSource = source;

        source.onended = () => {
            this.currentSource = null;
            this.playedCount++;
            // 当前音频播放完成，自动播放队列中的下一个
            this.playNext();
        };

        source.start();
    }

    /**
     * 停止当前播放并清空队列
     */
    stop() {
        // 停止当前播放
        if (this.currentSource) {
            try {
                this.currentSource.onended = null; // 防止触发playNext
                this.currentSource.stop();
            } catch (e) { /* 忽略已停止的源 */ }
            this.currentSource = null;
        }

        // 清空队列
        const clearedCount = this.queue.length;
        this.queue = [];
        this.playing = false;
    }

    /**
     * 获取队列状态信息
     *
     * @returns {{ queueLength: number, playing: boolean, playedCount: number, errorCount: number }}
     */
    getStatus() {
        return {
            queueLength: this.queue.length,
            playing: this.playing,
            playedCount: this.playedCount,
            errorCount: this.errorCount
        };
    }

    /**
     * 通知语音识别模块音频播放状态变化
     * 使用动态import避免循环依赖
     */
    notifyVoiceChat() {
        // 延迟导入以避免循环依赖
        import('./voice.js').then(({ VoiceChat }) => {
            VoiceChat.onAudioPlaybackChange();
        }).catch(() => {
            // voice.js 可能未加载，忽略
        });
    }

    /**
     * 设置音频参数
     *
     * @param {string} setting - 参数名称
     * @param {number} value - 参数值（0-1范围）
     */
    setAudioSetting(setting, value) {
        if (this.audioSettings.hasOwnProperty(setting)) {
            this.audioSettings[setting] = Math.max(0, Math.min(1, value));
        }
    }

    /**
     * 将Base64字符串转换为ArrayBuffer
     *
     * @param {string} base64 - Base64编码的字符串
     * @returns {ArrayBuffer} 解码后的ArrayBuffer
     */
    base64ToArrayBuffer(base64) {
        const binaryString = atob(base64);
        const len = binaryString.length;
        const bytes = new Uint8Array(len);
        for (let i = 0; i < len; i++) {
            bytes[i] = binaryString.charCodeAt(i);
        }
        return bytes.buffer;
    }
}

/** 全局音频播放队列单例 */
export const AudioQueue = new AudioQueueManager();
