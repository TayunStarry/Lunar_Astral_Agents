// ============================================================
//  星月智能 · 消息终端 — 音频播放队列 + 乐谱播放器桥接
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

// ---------- 音乐播放器 iframe 桥接（BroadcastChannel） ----------
const musicChannel = new BroadcastChannel('lunar-astral-music');
let musicIframe = null;
let musicReady = false;
const musicPendingQueue = [];

function initMusicRenderer() {
    if (document.getElementById('music-renderer-frame')) return;
    musicIframe = document.createElement('iframe');
    musicIframe.id = 'music-renderer-frame';
    musicIframe.src = '/file/read/package/music_libs/music_renderer.html';
    musicIframe.allow = 'autoplay';
    musicIframe.style.cssText = 'position:fixed;inset:0;width:100%;height:100%;border:none;z-index:1000;pointer-events:none;background:transparent;display:none;';
    document.body.appendChild(musicIframe);

    musicChannel.onmessage = (event) => {
        const msg = event.data;
        if (!msg || !msg.type) return;
        switch (msg.type) {
            case 'ready':
                musicReady = true;
                musicChannel.postMessage({ type: 'theme', darkMode: document.body.classList.contains('dark-mode') });
                while (musicPendingQueue.length) musicChannel.postMessage(musicPendingQueue.shift());
                break;
            case 'closed':
                hideMusicIframe();
                break;
            case 'state':
                if (msg.playing || msg.paused) showMusicIframe();
                break;
        }
    };
}

function postMusicMessage(msg) {
    if (!musicReady) {
        musicPendingQueue.push(msg);
        return;
    }
    musicChannel.postMessage(msg);
}

function showMusicIframe() {
    if (musicIframe) {
        musicIframe.style.display = 'block';
        musicIframe.style.pointerEvents = 'auto';
    }
}

function hideMusicIframe() {
    if (musicIframe) {
        musicIframe.style.display = 'none';
        musicIframe.style.pointerEvents = 'none';
    }
}

function renderMusicScore(abcNotation) {
    if (!abcNotation) return;
    showMusicIframe();
    postMusicMessage({ type: 'render', abcNotation });
}

function playRenderedAudio(audioUrl, fileName) {
    if (!audioUrl) return;
    showMusicIframe();
    postMusicMessage({ type: 'play_audio', audioUrl, fileName });
}
