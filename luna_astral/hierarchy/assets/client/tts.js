class TTSManager {
    allowLoading = true;
    maxTextLength = 5000;
    streamQueue = [];
    isStreaming = false;
    streamAbortController = null;

    constructor() {
        this.audioContext = null;
        this.currentSource = null;

        this.audioSettings = {
            noiseReduction: 0.15,
            playbackSpeed: 1.0,
            trebleBoost: 0.0,
            bassCut: 0.0
        };

        this.noiseGateFilter = null;
        this.highShelfFilter = null;
        this.lowShelfFilter = null;
        this.gainNode = null;
    }

    cleanTextForTTS(text) {
        if (!text) return '';
        let processed = text;
        processed = processed.replace(/```[\s\S]*?```/g, '');
        processed = processed.replace(/`[^`]*`/g, '');
        processed = processed.replace(/!\[.*?\]\(.*?\)/g, '');
        processed = processed.replace(/\[.*?\]\(.*?\)/g, '');
        processed = processed.replace(/<[^>]*>/g, '');
        processed = processed.replace(/[\u{1F600}-\u{1F64F}\u{1F300}-\u{1F5FF}\u{1F680}-\u{1F6FF}\u{1F900}-\u{1F9FF}\u{2600}-\u{26FF}\u{2700}-\u{27BF}\u{FE00}-\u{FE0F}\u{1F1E0}-\u{1F1FF}\u{200D}\u{20E3}\u{FE0F}]/gu, '');
        processed = processed.replace(/\*/g, '');
        processed = processed.replace(/\r?\n/g, ' ');
        processed = processed.replace(/\（[^）]*\）/g, '');
        processed = processed.replace(/\([^)]*\)/g, '');
        const allowed = '\\u4e00-\\u9fff' + 'a-zA-Z0-9' + '\\s' + '\uFF0C\u3002\uFF1F\uFF1A\uFF01\uFF1B\u3001\u2014\u2026\u300A\u300B\u201C\u201D\u2018\u2019\uFF08\uFF09\u3010\u3011' + ',.\'\"?:!';
        const whitelist = new RegExp(`[^${allowed}]`, 'g');
        processed = processed.replace(whitelist, '，');
        processed = processed.replace(/\s+/g, ' ');
        return processed.trim();
    }

    async generateAndPlay(text) {
        if (!this.allowLoading) return null;

        const processedText = this.cleanTextForTTS(text);
        if (!processedText) {
            console.warn('TTS: 清理后文本为空');
            return null;
        }

        if (processedText.length > this.maxTextLength) {
            console.warn(`TTS: 文本长度超过限制 (${processedText.length} > ${this.maxTextLength})`);
            this.showError('文本过长，请缩短后重试');
            return null;
        }

        return await this.streamGenerateAndPlay(processedText);
    }

    async streamGenerateAndPlay(text) {
        if (!this.allowLoading) return null;

        try {
            if (this.streamAbortController) {
                this.streamAbortController.abort();
            }
            this.streamAbortController = new AbortController();

            const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
            const wsUrl = `${protocol}//${window.location.host}/qwen_tts/stream`;

            return new Promise((resolve, reject) => {
                const ws = new WebSocket(wsUrl);
                const audioChunks = [];
                let finalAudio = null;

                ws.onopen = () => {
                    ws.send(JSON.stringify({
                        text: text,
                        chunk_frames: 50
                    }));
                };

                ws.onmessage = (event) => {
                    try {
                        const response = JSON.parse(event.data);

                        if (response.type === 'audio_chunk') {
                            audioChunks.push(response.audio);
                            this.playPCMChunk(response.audio, response.sample_rate);
                        } else if (response.type === 'final') {
                            finalAudio = response.audio;
                            this.isStreaming = false;
                            this.processStreamQueue();
                            ws.close();
                            resolve(finalAudio);
                        } else if (response.type === 'error') {
                            this.isStreaming = false;
                            this.showError(response.error || '流式TTS错误');
                            this.processStreamQueue();
                            ws.close();
                            resolve(null);
                        }
                    } catch (e) {
                        console.error('TTS: 解析WebSocket消息失败', e);
                    }
                };

                ws.onerror = (error) => {
                    console.error('TTS WebSocket错误:', error);
                    this.isStreaming = false;
                    this.showError('流式连接失败');
                    this.processStreamQueue();
                    resolve(null);
                };

                ws.onclose = () => {
                    if (!finalAudio && audioChunks.length === 0) {
                        this.isStreaming = false;
                        this.processStreamQueue();
                        resolve(null);
                    }
                };
            });
        } catch (err) {
            console.error('TTS 流式请求失败:', err);
            this.isStreaming = false;
            this.showError(err.message || '流式语音生成失败');
            this.processStreamQueue();
            return null;
        }
    }

    playPCMChunk(base64Data, sampleRate) {
        if (!this.audioContext) {
            this.audioContext = new (window.AudioContext || window.webkitAudioContext)();
        }

        if (this.audioContext.state === 'suspended') {
            this.audioContext.resume();
        }

        const binaryString = atob(base64Data);
        const len = binaryString.length;
        const bytes = new Uint8Array(len);
        for (let i = 0; i < len; i++) {
            bytes[i] = binaryString.charCodeAt(i);
        }

        const pcmData = new Int16Array(bytes.buffer);
        const float32Data = new Float32Array(pcmData.length);
        for (let i = 0; i < pcmData.length; i++) {
            float32Data[i] = pcmData[i] / 32768.0;
        }

        const audioBuffer = this.audioContext.createBuffer(1, float32Data.length, sampleRate);
        audioBuffer.getChannelData(0).set(float32Data);

        const source = this.audioContext.createBufferSource();
        source.buffer = audioBuffer;
        source.playbackRate.value = this.audioSettings.playbackSpeed;

        const gainNode = this.audioContext.createGain();
        gainNode.gain.value = 1.0;

        const highpass = this.audioContext.createBiquadFilter();
        highpass.type = 'highpass';
        highpass.frequency.value = 80 + (this.audioSettings.noiseReduction * 200);
        highpass.Q.value = 0.5;

        const lowshelf = this.audioContext.createBiquadFilter();
        lowshelf.type = 'lowshelf';
        lowshelf.frequency.value = 200;
        lowshelf.gain.value = -this.audioSettings.bassCut * 20;

        const highshelf = this.audioContext.createBiquadFilter();
        highshelf.type = 'highshelf';
        highshelf.frequency.value = 3000;
        highshelf.gain.value = this.audioSettings.trebleBoost * 15;

        source.connect(highpass);
        highpass.connect(lowshelf);
        lowshelf.connect(highshelf);
        highshelf.connect(gainNode);
        gainNode.connect(this.audioContext.destination);

        source.start();
    }

    queueStreamRequest(text, callback) {
        this.streamQueue.push({ text, callback });
        if (!this.isStreaming) {
            this.processStreamQueue();
        }
    }

    async processStreamQueue() {
        if (this.streamQueue.length === 0) {
            this.isStreaming = false;
            return;
        }

        this.isStreaming = true;
        const { text, callback } = this.streamQueue.shift();

        const result = await this.streamGenerateAndPlay(text);
        if (callback) {
            callback(result);
        }

        this.processStreamQueue();
    }

    async generateAndPlay(text) {
        if (!this.allowLoading) return null;

        const processedText = this.cleanTextForTTS(text);
        if (!processedText) {
            console.warn('TTS: 清理后文本为空');
            return null;
        }

        if (processedText.length > this.maxTextLength) {
            console.warn(`TTS: 文本长度超过限制 (${processedText.length} > ${this.maxTextLength})`);
            this.showError('文本过长，请缩短后重试');
            return null;
        }

        return await this.streamGenerateAndPlay(processedText);
    }

    playAudioBuffer(arrayBuffer) {
        if (!this.audioContext) {
            this.audioContext = new (window.AudioContext || window.webkitAudioContext)();
        }

        if (this.currentSource) {
            try {
                this.currentSource.stop();
            } catch (e) { }
            this.currentSource = null;
        }

        this.audioContext.decodeAudioData(
            arrayBuffer,
            (audioBuffer) => {
                const source = this.audioContext.createBufferSource();
                source.buffer = audioBuffer;
                source.playbackRate.value = this.audioSettings.playbackSpeed;

                this.gainNode = this.audioContext.createGain();
                this.gainNode.gain.value = 1.0;

                this.noiseGateFilter = this.audioContext.createBiquadFilter();
                this.noiseGateFilter.type = 'highpass';
                this.noiseGateFilter.frequency.value = 80 + (this.audioSettings.noiseReduction * 200);
                this.noiseGateFilter.Q.value = 0.5;

                this.lowShelfFilter = this.audioContext.createBiquadFilter();
                this.lowShelfFilter.type = 'lowshelf';
                this.lowShelfFilter.frequency.value = 200;
                this.lowShelfFilter.gain.value = -this.audioSettings.bassCut * 20;

                this.highShelfFilter = this.audioContext.createBiquadFilter();
                this.highShelfFilter.type = 'highshelf';
                this.highShelfFilter.frequency.value = 3000;
                this.highShelfFilter.gain.value = this.audioSettings.trebleBoost * 15;

                source.connect(this.noiseGateFilter);
                this.noiseGateFilter.connect(this.lowShelfFilter);
                this.lowShelfFilter.connect(this.highShelfFilter);
                this.highShelfFilter.connect(this.gainNode);
                this.gainNode.connect(this.audioContext.destination);

                this.currentSource = source;
                source.onended = () => {
                    this.currentSource = null;
                };
                source.start();
            },
            (err) => {
                console.error('TTS: decodeAudioData failed', err);
                this.showError('音频解码失败');
            }
        );
    }

    base64ToArrayBuffer(base64) {
        const binaryString = atob(base64);
        const len = binaryString.length;
        const bytes = new Uint8Array(len);
        for (let i = 0; i < len; i++) {
            bytes[i] = binaryString.charCodeAt(i);
        }
        return bytes.buffer;
    }

    setAudioSetting(setting, value) {
        if (this.audioSettings.hasOwnProperty(setting)) {
            this.audioSettings[setting] = Math.max(0, Math.min(1, value));
        }
    }

    showError(message) {
        const errorToast = document.getElementById('errorToast');
        if (!errorToast) return;
        errorToast.textContent = message;
        errorToast.classList.add('visible');
        setTimeout(() => errorToast?.classList.remove('visible'), 3000);
        this.allowLoading = false;
    }

    stop() {
        if (this.currentSource) {
            try {
                this.currentSource.stop();
            } catch (e) { }
            this.currentSource = null;
        }
        if (this.streamAbortController) {
            this.streamAbortController.abort();
            this.streamAbortController = null;
        }
        this.streamQueue = [];
        this.isStreaming = false;
    }
}

export const TTS = new TTSManager();
