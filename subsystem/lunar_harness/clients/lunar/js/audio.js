// ============================================================
//  星月智能 · 消息终端 — 音频播放队列（TTS）
// ============================================================

// ---------- 音频播放队列（TTS） ----------
class AudioQueueManager {
    constructor() {
        this.audioContext = null;
        this.currentSource = null;
        this.queue = [];
        this.playing = false;
    }

    enqueue(audioBase64) {
        if (!audioBase64) return;
        this.queue.push(audioBase64);
        if (!this.playing) this.playNext();
    }

    playNext() {
        if (this.queue.length === 0) {
            this.playing = false;
            this.currentSource = null;
            return;
        }
        this.playing = true;
        const base64 = this.queue.shift();
        try {
            const arrayBuffer = this.base64ToArrayBuffer(base64);
            this.decodeAndPlay(arrayBuffer);
        } catch (err) {
            console.warn('音频处理失败', err);
            this.playNext();
        }
    }

    decodeAndPlay(arrayBuffer) {
        if (!this.audioContext) {
            this.audioContext = new (window.AudioContext || window.webkitAudioContext)();
        }
        if (this.audioContext.state === 'suspended') {
            this.audioContext.resume();
        }
        this.audioContext.decodeAudioData(
            arrayBuffer,
            (buffer) => this.playAudioBuffer(buffer),
            (err) => {
                console.warn('音频解码失败', err);
                this.playNext();
            }
        );
    }

    playAudioBuffer(audioBuffer) {
        const source = this.audioContext.createBufferSource();
        source.buffer = audioBuffer;
        source.connect(this.audioContext.destination);
        this.currentSource = source;
        source.onended = () => {
            this.currentSource = null;
            this.playNext();
        };
        source.start();
    }

    stop() {
        if (this.currentSource) {
            try {
                this.currentSource.onended = null;
                this.currentSource.stop();
            } catch (e) { /* 已停止 */ }
            this.currentSource = null;
        }
        this.queue = [];
        this.playing = false;
    }

    base64ToArrayBuffer(base64) {
        const binary = atob(base64);
        const bytes = new Uint8Array(binary.length);
        for (let i = 0; i < binary.length; i++) {
            bytes[i] = binary.charCodeAt(i);
        }
        return bytes.buffer;
    }
}
const AudioQueue = new AudioQueueManager();

