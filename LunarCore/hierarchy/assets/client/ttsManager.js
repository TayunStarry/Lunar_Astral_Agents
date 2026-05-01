class TTSManager {
    constructor() {
        this.audioContext = null;
        this.currentSource = null;
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

        const allowed = '\\u4e00-\\u9fff' + 'a-zA-Z0-9' + '\\s' + '\uFF0C\u3002\uFF1F\uFF1A' + '\u201C\u201D\u2018\u2019' + '\u300A\u300B' + ',.\'\"?:!';
        const whitelist = new RegExp(`[^${allowed}]`, 'g');
        processed = processed.replace(whitelist, '');
        processed = processed.replace(/\s+/g, ' ');
        return processed.trim();
    }

    async generateAndPlay(text) {
        const processedText = this.cleanTextForTTS(text);
        if (!processedText) return;

        let audioBlob;
        try {
            const response = await fetch('./voice_template.wav');
            if (!response.ok) throw new Error('Failed to fetch voice template');
            audioBlob = await response.blob();
        } catch (err) {
            console.error('TTS: 语音模板加载失败', err);
            this.showError('语音模板加载失败');
            return;
        }

        const formData = new FormData();
        formData.append('text', processedText);
        formData.append('demo_id', 'demo-1');
        formData.append('prompt_audio', audioBlob, 'voice_template.wav');
        formData.append('max_new_frames', '800');
        formData.append('voice_clone_max_text_tokens', '200');
        formData.append('attn_implementation', 'eager');
        formData.append('do_sample', '1');
        formData.append('text_temperature', '1.0');
        formData.append('text_top_p', '1.0');
        formData.append('text_top_k', '50');
        formData.append('audio_temperature', '0.8');
        formData.append('audio_top_p', '0.95');
        formData.append('audio_top_k', '25');
        formData.append('audio_repetition_penalty', '1.2');
        formData.append('seed', '16384');
        formData.append('tts_max_batch_size', '0');
        formData.append('codec_max_batch_size', '0');
        formData.append('enable_text_normalization', '0');
        formData.append('enable_normalize_tts_text', '0');
        formData.append('cpu_threads', '8');

        try {
            const apiUrl = '/audio/generate';
            const res = await fetch(apiUrl, {
                method: 'POST',
                body: formData
            });
            if (!res.ok) throw new Error(`TTS API 状态异常: ${res.status}`);
            const data = await res.json();

            if (data.audio_base64) {
                const arrayBuffer = this.base64ToArrayBuffer(data.audio_base64);
                this.playAudioBuffer(arrayBuffer);
            } else {
                console.warn('TTS: 响应中没有 audio_base64');
            }
        } catch (err) {
            console.error('TTS 请求失败:', err);
            this.showError('语音生成失败');
        }
    }

    playAudioBuffer(arrayBuffer) {
        if (!this.audioContext) {
            this.audioContext = new (window.AudioContext || window.webkitAudioContext)();
        }

        if (this.currentSource) {
            try {
                this.currentSource.stop();
            } catch (e) {}
            this.currentSource = null;
        }

        this.audioContext.decodeAudioData(
            arrayBuffer,
            (audioBuffer) => {
                const source = this.audioContext.createBufferSource();
                source.buffer = audioBuffer;
                source.connect(this.audioContext.destination);
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

    showError(message) {
        const errorToast = document.getElementById('errorToast');
        if (errorToast) {
            errorToast.textContent = message;
            errorToast.classList.add('visible');
            setTimeout(() => {
                errorToast?.classList.remove('visible');
            }, 3000);
        }
    }

    stop() {
        if (this.currentSource) {
            try {
                this.currentSource.stop();
            } catch (e) {}
            this.currentSource = null;
        }
    }
}

export const TTS = new TTSManager();