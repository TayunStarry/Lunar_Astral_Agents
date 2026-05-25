class TTSManager {
    allowLoading = true;
    maxTextLength = 5000;

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
        processed = processed.replace(/<think>[\s\S]*?<\/think>/gi, '');
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

        try {
            const res = await fetch('/tts', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ text: processedText })
            });

            if (!res.ok) {
                const errorData = await res.json().catch(() => null);
                const errorMsg = errorData?.error || errorData?.message || `请求失败 (${res.status})`;
                throw new Error(errorMsg);
            }

            const data = await res.json();

            if (!data.success) {
                throw new Error(data.error || data.message || '语音生成失败');
            }

            if (!data.audio) {
                throw new Error('响应中缺少音频数据');
            }

            const arrayBuffer = this.base64ToArrayBuffer(data.audio);
            this.playAudioBuffer(arrayBuffer);
            return data.audio;
        } catch (err) {
            console.error('TTS 请求失败:', err);
            this.showError(err.message || '语音生成失败');
            return null;
        }
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
    }
}

export const TTS = new TTSManager();
