class TTSManager {
    constructor() {
        this.audioContext = null;
        this.currentSource = null;

        // 音频处理设置配置对象
        this.audioSettings = {
            // 噪声抑制强度，范围 0-1，值越大过滤的低频噪声越多
            noiseReduction: 0.15,
            // 播放速度倍率，1.0 为正常速度
            playbackSpeed: 1.0,
            // 高音增强强度，范围 0-1
            trebleBoost: 0.0,
            // 低音削减强度，范围 0-1
            bassCut: 0.0
        };

        // 噪声门滤波器，用于减少背景噪声
        this.noiseGateFilter = null;
        // 高频搁架滤波器，用于增强高音
        this.highShelfFilter = null;
        // 低频搁架滤波器，用于削减低音
        this.lowShelfFilter = null;
        // 增益节点，用于控制音量
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
        const allowed = '\\u4e00-\\u9fff' + 'a-zA-Z0-9' + '\\s' + '\uFF0C\u3002\uFF1F\uFF1A' + '\u201C\u201D\u2018\u2019' + '\u300A\u300B' + ',.\'\"?:!';
        const whitelist = new RegExp(`[^${allowed}]`, 'g');
        processed = processed.replace(whitelist, '，');
        processed = processed.replace(/\s+/g, ' ');
        return processed.trim();
    }

    async generateAndPlay(text) {
        const processedText = this.cleanTextForTTS(text);
        if (!processedText) return null;
        const formData = new FormData();
        // 要转换为语音的文本内容
        formData.append('text', processedText);
        // 演示ID，用于标识不同的语音合成演示
        formData.append('demo_id', 'demo-30');
        // 最大生成帧数，控制音频生成的最大长度
        formData.append('max_new_frames', '800');
        // 语音克隆时文本的最大token数量
        formData.append('voice_clone_max_text_tokens', '75');
        // 注意力机制实现方式，eager表示使用即时计算模式
        formData.append('attn_implementation', 'eager');
        // 是否启用采样，1表示启用随机采样
        formData.append('do_sample', '1');
        // 文本生成的温度参数，控制随机性，1.0为标准值
        formData.append('text_temperature', '1.0');
        // 文本生成的top-p采样阈值，1.0表示不限制
        formData.append('text_top_p', '1.0');
        // 文本生成的top-k采样值，限制候选词数量
        formData.append('text_top_k', '50');
        // 音频生成的温度参数，0.8表示较低的随机性
        formData.append('audio_temperature', '0.8');
        // 音频生成的top-p采样阈值，0.95表示较高的累积概率
        formData.append('audio_top_p', '0.95');
        // 音频生成的top-k采样值，25限制候选音频特征
        formData.append('audio_top_k', '25');
        // 音频生成的重复惩罚系数，1.2表示中等程度的惩罚
        formData.append('audio_repetition_penalty', '1.2');
        // 随机种子，用于保证结果可复现
        formData.append('seed', '82340927390');
        // TTS模型的最大批处理大小，1表示单条处理
        formData.append('tts_max_batch_size', '1');
        // 编解码器的最大批处理大小，0表示自动
        formData.append('codec_max_batch_size', '0');
        // 是否启用文本规范化，1表示启用
        formData.append('enable_text_normalization', '1');
        // 是否启用TTS文本标准化，1表示启用
        formData.append('enable_normalize_tts_text', '1');
        // CPU线程数，8表示使用8个线程进行计算
        formData.append('cpu_threads', '8');
        try {
            const apiUrl = '/audio/generate';
            const res = await fetch(apiUrl, { method: 'POST', body: formData });
            if (!res.ok) throw new Error(`TTS API 状态异常: ${res.status}`);
            const data = await res.json();
            if (data.audio_base64) {
                const arrayBuffer = this.base64ToArrayBuffer(data.audio_base64);
                this.playAudioBuffer(arrayBuffer);
                return data.audio_base64;
            }
            else console.warn('TTS: 响应中没有 audio_base64');
        }
        catch (err) {
            console.error('TTS 请求失败:', err);
            this.showError('语音生成失败');
        }
        return null;
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
            } catch (e) { }
            this.currentSource = null;
        }
    }
}
class Qwen3 extends TTSManager {
    constructor() {
        super();
        this.refText = "";
        this.modelReady = false;
    }


    arrayBufferToBase64(buffer) {
        const bytes = new Uint8Array(buffer);
        let binary = '';
        for (let i = 0; i < bytes.length; i++) {
            binary += String.fromCharCode(bytes[i]);
        }
        return btoa(binary);
    }

    async checkModelAvailable() {
        try {
            const res = await fetch('/qwen_tts/models');
            return res.ok;
        } catch {
            return false;
        }
    }

    async generateAndPlay(text) {
        const processedText = this.cleanTextForTTS(text);
        if (!processedText) return null;
        const requestBody = {
            // 模型名称，指定使用的TTS模型
            model_name: "Qwen/Qwen3-TTS-12Hz-0.6B-Base",
            // 要转换为语音的文本内容
            text: processedText,
            // 语言设置，auto表示自动检测
            language: "auto",
            // 参考音频的Base64编码，用于声音克隆
            ref_audio_base64: this.refAudioBase64,
            // 参考音频对应的文本内容
            ref_text: this.refText,
            // 是否分段生成音频
            segment_gen: true,
            // 是否启用采样
            do_sample: true,
            // 采样时的top-k值，控制候选词数量
            top_k: 50,
            // 采样时的top-p值，控制累积概率阈值
            top_p: 1.0,
            // 采样温度，控制随机性，0表示确定性输出
            temperature: 0.8,
            // 重复惩罚系数，0表示不惩罚
            repetition_penalty: 1.2,
            // 子说话人是否启用采样
            subtalker_dosample: true,
            // 子说话人top-k值
            subtalker_top_k: 25,
            // 子说话人top-p值
            subtalker_top_p: 0.95,
            // 子说话人采样温度
            subtalker_temperature: 0.8,
            // 最大生成token数，0表示使用默认值
            max_new_tokens: 2048
        };
        try {
            const apiUrl = '/qwen_tts/voice-clone';
            const res = await fetch(apiUrl, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(requestBody) });
            if (!res.ok) throw new Error(`Qwen3 API 状态异常: ${res.status}`);
            const data = await res.json();
            if (data.audio_base64) {
                const arrayBuffer = this.base64ToArrayBuffer(data.audio_base64);
                this.playAudioBuffer(arrayBuffer);
                return data.audio_base64;
            }
            else console.warn('响应中没有 audio_base64');
        }
        catch (err) {
            console.error('Qwen3 TTS 请求失败:', err);
            this.showError('语音生成失败');
        }
        return null;
    }
}

async function initTTS() {
    try {
        const res = await fetch('/qwen_tts/models');
        if (res.ok) {
            return new Qwen3();
        }
    } catch { }
    return new TTSManager();
}

export let TTS;

initTTS().then(instance => {
    TTS = instance;
});