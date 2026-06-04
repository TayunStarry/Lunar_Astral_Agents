const API_BASE = window.location.origin;

// DOM Elements
const TEXT_INPUT = document.getElementById('text-input');
const CHAR_COUNT = document.getElementById('char-count');
const UPLOAD_AREA = document.getElementById('upload-area');
const AUDIO_UPLOAD = document.getElementById('audio-upload');
const UPLOAD_CONTENT = document.getElementById('upload-content');
const FILE_INFO = document.getElementById('file-info');
const FILE_NAME = document.getElementById('file-name');
const REMOVE_FILE_BTN = document.getElementById('remove-file');
const SYNTHESIZE_BTN = document.getElementById('synthesize-btn');
const STATUS_MESSAGE = document.getElementById('status-message');
const AUDIO_PLAYER_CONTAINER = document.getElementById('audio-player-container');
const AUDIO_PLAYER = document.getElementById('audio-player');
const PLAY_PAUSE_BTN = document.getElementById('play-pause-btn');
const ICON_PLAY = PLAY_PAUSE_BTN.querySelector('.icon-play');
const ICON_PAUSE = PLAY_PAUSE_BTN.querySelector('.icon-pause');
const STOP_BTN = document.getElementById('stop-btn');
const PROGRESS_BAR = document.getElementById('progress-bar');
const TIME_DISPLAY = document.getElementById('time-display');
const VOLUME_SLIDER = document.getElementById('volume-slider');
const WAVEFORM = document.getElementById('waveform');
const DOWNLOAD_BTN = document.getElementById('download-btn');
const STREAM_BTN = document.getElementById('stream-btn');
const CHUNKFRAMES_INPUT = document.getElementById('chunkframes-input');
const STREAM_BADGE = document.getElementById('stream-badge');
const TEMPERATURE_INPUT = document.getElementById('temperature-input');
const TEMPERATURE_VALUE = document.getElementById('temperature-value');
const TOPK_INPUT = document.getElementById('topk-input');
const TOPP_INPUT = document.getElementById('topp-input');
const TOPP_VALUE = document.getElementById('topp-value');
const MAXTOKENS_INPUT = document.getElementById('maxtokens-input');
const REPETITION_INPUT = document.getElementById('repetition-input');
const REPETITION_VALUE = document.getElementById('repetition-value');
const THREADS_INPUT = document.getElementById('threads-input');
const DISABLE_CACHE_TOGGLE = document.getElementById('disable-cache-toggle');

// EQ DOM Elements
const EQ_LOW_SLIDER = document.getElementById('eq-low-slider');
const EQ_LOW_MODE = document.getElementById('eq-low-mode');
const EQ_LOW_MULTI = document.getElementById('eq-low-multi');
const EQ_LOW_VALUE = document.getElementById('eq-low-value');
const EQ_MID_SLIDER = document.getElementById('eq-mid-slider');
const EQ_MID_MODE = document.getElementById('eq-mid-mode');
const EQ_MID_MULTI = document.getElementById('eq-mid-multi');
const EQ_MID_VALUE = document.getElementById('eq-mid-value');
const EQ_HIGH_SLIDER = document.getElementById('eq-high-slider');
const EQ_HIGH_MODE = document.getElementById('eq-high-mode');
const EQ_HIGH_MULTI = document.getElementById('eq-high-multi');
const EQ_HIGH_VALUE = document.getElementById('eq-high-value');
const EQ_RESET_BTN = document.getElementById('eq-reset-btn');
const EQ_VIS_LOW = document.getElementById('eq-vis-low');
const EQ_VIS_MID = document.getElementById('eq-vis-mid');
const EQ_VIS_HIGH = document.getElementById('eq-vis-high');

// State variables
let uploadedRefAudioPath = null;
let animationId = null;
let waveInterval = null;
let currentAudioBase64 = null;
let streamWs = null;
let isStreaming = false;
let audioBufferQueue = [];
let isWebAudioPlaying = false;
let webAudioCtx = null;
let scheduledNodes = [];
let nextStartTime = 0;
let streamSampleRate = 24000;
let streamGainNode = null;
let streamTotalSamples = 0;
let streamStartTime = 0;
let streamCachedChunks = [];
let streamReceivedChunkIndex = 0;
let streamExpectedTotalChunks = 0;
let streamIsPaused = false;
let streamPlaybackPosition = 0;

// EQ state
let eqCtx = null;
let eqLowFilter = null;
let eqMidFilter = null;
let eqHighFilter = null;
let eqSourceNode = null;

function init() {
    setupEventListeners();
    TEXT_INPUT.focus();
    DOWNLOAD_BTN.disabled = true;
    ICON_PLAY.classList.remove('hidden');
    ICON_PAUSE.classList.add('hidden');
    if (AUDIO_PLAYER) AUDIO_PLAYER.volume = VOLUME_SLIDER.value / 100;
    updateEQVisualizer();
}

function setupEventListeners() {
    TEXT_INPUT.addEventListener('input', handleTextInput);

    UPLOAD_AREA.addEventListener('click', (e) => {
        if (!e.target.closest('#remove-file') && !e.target.closest('#file-info')) {
            AUDIO_UPLOAD.click();
        }
    });
    AUDIO_UPLOAD.addEventListener('change', handleFileSelect);
    UPLOAD_AREA.addEventListener('dragover', handleDragOver);
    UPLOAD_AREA.addEventListener('dragleave', handleDragLeave);
    UPLOAD_AREA.addEventListener('drop', handleDrop);

    REMOVE_FILE_BTN.addEventListener('click', removeUploadedFile);
    SYNTHESIZE_BTN.addEventListener('click', synthesizeSpeech);
    STREAM_BTN.addEventListener('click', synthesizeStream);

    PLAY_PAUSE_BTN.addEventListener('click', togglePlayPause);
    STOP_BTN.addEventListener('click', stopAudio);
    PROGRESS_BAR.addEventListener('input', handleSeek);
    VOLUME_SLIDER.addEventListener('input', handleVolumeChange);
    DOWNLOAD_BTN.addEventListener('click', downloadAudio);

    AUDIO_PLAYER.addEventListener('timeupdate', handleTimeUpdate);
    AUDIO_PLAYER.addEventListener('ended', handleAudioEnded);
    AUDIO_PLAYER.addEventListener('loadedmetadata', handleMetadataLoaded);

    TEMPERATURE_INPUT.addEventListener('input', () => {
        TEMPERATURE_VALUE.textContent = TEMPERATURE_INPUT.value;
    });
    TOPP_INPUT.addEventListener('input', () => {
        TOPP_VALUE.textContent = TOPP_INPUT.value;
    });
    REPETITION_INPUT.addEventListener('input', () => {
        REPETITION_VALUE.textContent = REPETITION_INPUT.value;
    });

    // EQ event listeners
    EQ_LOW_SLIDER.addEventListener('input', () => { updateEQ(); updateEQVisualizer(); });
    EQ_LOW_MODE.addEventListener('change', () => { updateEQ(); updateEQVisualizer(); });
    EQ_LOW_MULTI.addEventListener('change', () => { updateEQ(); updateEQVisualizer(); });
    EQ_MID_SLIDER.addEventListener('input', () => { updateEQ(); updateEQVisualizer(); });
    EQ_MID_MODE.addEventListener('change', () => { updateEQ(); updateEQVisualizer(); });
    EQ_MID_MULTI.addEventListener('change', () => { updateEQ(); updateEQVisualizer(); });
    EQ_HIGH_SLIDER.addEventListener('input', () => { updateEQ(); updateEQVisualizer(); });
    EQ_HIGH_MODE.addEventListener('change', () => { updateEQ(); updateEQVisualizer(); });
    EQ_HIGH_MULTI.addEventListener('change', () => { updateEQ(); updateEQVisualizer(); });
    EQ_RESET_BTN.addEventListener('click', () => { resetEQ(); updateEQVisualizer(); });
}

function togglePlayPause() {
    if (!AUDIO_PLAYER.src) {
        showStatus('没有可播放的音频', 'error');
        return;
    }
    if (AUDIO_PLAYER.paused) {
        AUDIO_PLAYER.play().then(() => {
            ICON_PLAY.classList.add('hidden');
            ICON_PAUSE.classList.remove('hidden');
        }).catch(error => {
            console.error('播放失败:', error);
            showStatus('播放失败，请重试', 'error');
        });
    } else {
        AUDIO_PLAYER.pause();
        ICON_PLAY.classList.remove('hidden');
        ICON_PAUSE.classList.add('hidden');
    }
}

function handleTextInput() {
    CHAR_COUNT.textContent = TEXT_INPUT.value.length;
}

function handleDragOver(e) {
    e.preventDefault();
    UPLOAD_AREA.classList.add('dragging');
}

function handleDragLeave() {
    UPLOAD_AREA.classList.remove('dragging');
}

async function handleDrop(e) {
    e.preventDefault();
    UPLOAD_AREA.classList.remove('dragging');
    const files = e.dataTransfer.files;
    if (files.length > 0) {
        await uploadAudioFile(files[0]);
    }
}

function handleFileSelect(e) {
    const file = e.target.files[0];
    if (file) {
        uploadAudioFile(file);
    }
}

async function uploadAudioFile(file) {
    const validExts = ['.wav'];
    const ext = '.' + file.name.split('.').pop().toLowerCase();
    if (!validExts.includes(ext)) {
        showStatus('仅支持 WAV 格式音频文件', 'error');
        AUDIO_UPLOAD.value = '';
        return;
    }

    const formData = new FormData();
    formData.append('audio', file);

    try {
        showStatus('正在上传音频...', 'info');
        const response = await fetch(`${API_BASE}/upload/`, {
            method: 'POST',
            body: formData
        });
        const result = await response.json();

        if (result.success) {
            uploadedRefAudioPath = result.path;
            FILE_NAME.textContent = result.name;
            FILE_INFO.classList.remove('hidden');
            UPLOAD_CONTENT.classList.add('hidden');
            showStatus('参考音频已就绪', 'success');
        } else {
            showStatus('上传失败: ' + (result.error || '未知错误'), 'error');
            AUDIO_UPLOAD.value = '';
        }
    } catch (error) {
        showStatus('上传失败: ' + error.message, 'error');
        AUDIO_UPLOAD.value = '';
    }
}

function removeUploadedFile(e) {
    e.stopPropagation();
    uploadedRefAudioPath = null;
    AUDIO_UPLOAD.value = '';
    FILE_INFO.classList.add('hidden');
    UPLOAD_CONTENT.classList.remove('hidden');
}

async function synthesizeSpeech() {
    const text = TEXT_INPUT.value.trim();

    if (!text) {
        showStatus('请填写需要转换的文本', 'error');
        TEXT_INPUT.focus();
        return;
    }

    SYNTHESIZE_BTN.disabled = true;
    STREAM_BTN.disabled = true;
    showStatus('正在生成语音，请稍后...', 'info');
    startWaveformAnimation();

    try {
        const requestBody = { text: text };
        if (uploadedRefAudioPath) {
            requestBody.ref_audio = uploadedRefAudioPath;
        }

        const temperature = parseFloat(TEMPERATURE_INPUT.value);
        const topK = parseInt(TOPK_INPUT.value, 10);
        const topP = parseFloat(TOPP_INPUT.value);
        const maxTokens = parseInt(MAXTOKENS_INPUT.value, 10);
        const repetitionPenalty = parseFloat(REPETITION_INPUT.value);
        const threads = parseInt(THREADS_INPUT.value, 10);

        if (temperature !== 0.8) requestBody.temperature = temperature;
        if (topK > 0) requestBody.top_k = topK;
        if (topP !== 0.9) requestBody.top_p = topP;
        if (maxTokens > 0) requestBody.max_tokens = maxTokens;
        if (repetitionPenalty !== 1.1) requestBody.repetition_penalty = repetitionPenalty;
        if (threads > 0) requestBody.threads = threads;

        requestBody.disable_cache = DISABLE_CACHE_TOGGLE.checked;

        const response = await fetch(`${API_BASE}/tts`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(requestBody)
        });

        const result = await response.json();

        if (result.success && result.audio) {
            loadAudioFromBase64(result.audio);
            showStatus('合成成功，点击播放', 'success');
        } else {
            showStatus('合成失败: ' + (result.error || '服务端错误'), 'error');
        }
    } catch (error) {
        showStatus('网络错误: ' + error.message, 'error');
    } finally {
        SYNTHESIZE_BTN.disabled = false;
        STREAM_BTN.disabled = false;
        stopWaveformAnimation();
    }
}

async function synthesizeStream() {
    const text = TEXT_INPUT.value.trim();

    if (!text) {
        showStatus('请填写需要转换的文本', 'error');
        TEXT_INPUT.focus();
        return;
    }

    if (isStreaming) {
        stopStream();
        return;
    }

    SYNTHESIZE_BTN.disabled = true;
    STREAM_BTN.disabled = true;
    showStatus('正在连接流式输出...', 'info');
    startWaveformAnimation();

    audioBufferQueue = [];
    streamCachedChunks = [];
    streamReceivedChunkIndex = 0;
    streamExpectedTotalChunks = 0;
    streamIsPaused = false;
    streamPlaybackPosition = 0;
    currentAudioBase64 = null;

    const wsProtocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const wsUrl = `${wsProtocol}//${window.location.host}/tts/stream`;

    streamWs = new WebSocket(wsUrl);

    streamWs.onopen = () => {
        const requestBody = { text: text };
        if (uploadedRefAudioPath) {
            requestBody.ref_audio = uploadedRefAudioPath;
        }

        const chunkFrames = parseInt(CHUNKFRAMES_INPUT.value, 10) || 50;
        requestBody.chunk_frames = chunkFrames;

        const temperature = parseFloat(TEMPERATURE_INPUT.value);
        const topK = parseInt(TOPK_INPUT.value, 10);
        const topP = parseFloat(TOPP_INPUT.value);
        const maxTokens = parseInt(MAXTOKENS_INPUT.value, 10);
        const repetitionPenalty = parseFloat(REPETITION_INPUT.value);
        const threads = parseInt(THREADS_INPUT.value, 10);

        if (temperature !== 0.8) requestBody.temperature = temperature;
        if (topK > 0) requestBody.top_k = topK;
        if (topP !== 0.9) requestBody.top_p = topP;
        if (maxTokens > 0) requestBody.max_tokens = maxTokens;
        if (repetitionPenalty !== 1.1) requestBody.repetition_penalty = repetitionPenalty;
        if (threads > 0) requestBody.threads = threads;

        requestBody.disable_cache = DISABLE_CACHE_TOGGLE.checked;

        streamWs.send(JSON.stringify(requestBody));
        showStatus('流式传输中...', 'info');
    };

    streamWs.onmessage = (event) => {
        try {
            const data = JSON.parse(event.data);
            handleStreamMessage(data);
        } catch (e) {
            console.error('解析流式消息失败:', e);
        }
    };

    streamWs.onerror = (error) => {
        console.error('WebSocket 错误:', error);
        showStatus('流式连接错误', 'error');
        stopStream();
    };

    streamWs.onclose = () => {
        if (isStreaming) {
            stopStream();
        }
    };
}

function handleStreamMessage(data) {
    switch (data.type) {
        case 'audio_chunk':
            if (data.audio) {
                const expectedIndex = streamReceivedChunkIndex + 1;
                if (data.chunk_index && data.chunk_index !== expectedIndex) {
                    console.warn('[Stream] Chunk index mismatch: expected ' + expectedIndex + ', got ' + data.chunk_index);
                }
                streamReceivedChunkIndex = data.chunk_index || streamReceivedChunkIndex + 1;
                streamCachedChunks.push(data.audio);
                if (data.sample_rate) streamSampleRate = data.sample_rate;
                if (data.is_final && data.total_chunks) {
                    streamExpectedTotalChunks = data.total_chunks;
                }
                if (streamIsPaused) {
                    return;
                }
                audioBufferQueue.push(data.audio);
                if (!isWebAudioPlaying) {
                    initWebAudioPlayback();
                } else {
                    scheduleNextChunk();
                }
            }
            break;

        case 'final':
            streamExpectedTotalChunks = data.total_chunks || streamReceivedChunkIndex;
            showStatus('流式合成完成', 'success');
            finalizeStreamAudio();
            break;

        case 'error':
            showStatus('流式合成失败: ' + (data.error || '未知错误'), 'error');
            stopStream();
            break;
    }
}

function decodeBase64ToFloat32(base64Audio) {
    const binaryString = atob(base64Audio);
    const bytes = new Uint8Array(binaryString.length);
    for (let i = 0; i < binaryString.length; i++) {
        bytes[i] = binaryString.charCodeAt(i);
    }

    if (bytes.length < 2) return { samples: [], sampleRate: 24000 };

    const view = new DataView(bytes.buffer);
    const numSamples = bytes.length / 2;

    const samples = new Float32Array(numSamples);
    for (let i = 0; i < numSamples; i++) {
        const int16 = view.getInt16(i * 2, true);
        samples[i] = int16 / 32768.0;
    }

    return { samples, sampleRate: streamSampleRate };
}

function initWebAudioContext() {
    if (!webAudioCtx) {
        webAudioCtx = new (window.AudioContext || window.webkitAudioContext)();
        streamGainNode = webAudioCtx.createGain();
        streamGainNode.gain.value = VOLUME_SLIDER.value / 100;
        streamGainNode.connect(webAudioCtx.destination);
    }
    if (webAudioCtx.state === 'suspended') {
        webAudioCtx.resume();
    }
}

function initWebAudioPlayback() {
    initWebAudioContext();

    isWebAudioPlaying = true;
    isStreaming = true;
    nextStartTime = webAudioCtx.currentTime + 0.05;
    streamStartTime = nextStartTime;
    streamTotalSamples = 0;
    scheduledNodes = [];

    STREAM_BTN.classList.add('active');
    STREAM_BADGE.classList.remove('hidden');
    STREAM_BTN.querySelector('span').textContent = '停止流式';
    const playerCard = AUDIO_PLAYER_CONTAINER;
    if (playerCard) playerCard.classList.add('streaming');

    scheduleNextChunk();
    startStreamMonitor();
}

function scheduleNextChunk() {
    if (!isWebAudioPlaying || audioBufferQueue.length === 0) return;

    const base64Audio = audioBufferQueue.shift();
    const { samples, sampleRate } = decodeBase64ToFloat32(base64Audio);

    if (samples.length === 0) {
        scheduleNextChunk();
        return;
    }

    try {
        const audioBuffer = webAudioCtx.createBuffer(1, samples.length, sampleRate);
        audioBuffer.getChannelData(0).set(samples);

        const source = webAudioCtx.createBufferSource();
        source.buffer = audioBuffer;
        source.connect(streamGainNode);
        source.start(nextStartTime);

        scheduledNodes.push({ source, buffer: audioBuffer });

        nextStartTime += samples.length / sampleRate;

        source.onended = () => {
            const idx = scheduledNodes.findIndex(n => n.source === source);
            if (idx !== -1) scheduledNodes.splice(idx, 1);

            if (isWebAudioPlaying && audioBufferQueue.length > 0) {
                scheduleNextChunk();
            } else if (isWebAudioPlaying && audioBufferQueue.length === 0 && scheduledNodes.length === 0) {
                if (!isStreaming) {
                    completeStreamPlayback();
                }
            }
        };
    } catch (e) {
        console.error('调度音频块失败:', e);
        if (audioBufferQueue.length > 0) {
            setTimeout(() => scheduleNextChunk(), 10);
        }
    }
}

function startStreamMonitor() {
    const monitor = setInterval(() => {
        if (!isWebAudioPlaying) {
            clearInterval(monitor);
            return;
        }

        if (!isStreaming && audioBufferQueue.length === 0 && scheduledNodes.length === 0) {
            clearInterval(monitor);
            completeStreamPlayback();
        }
    }, 200);
}

function completeStreamPlayback() {
    isWebAudioPlaying = false;
    nextStartTime = 0;
    streamTotalSamples = 0;

    stopStreamUI();
}

function stopStream() {
    if (streamWs) {
        streamWs.close();
        streamWs = null;
    }

    if (webAudioCtx) {
        scheduledNodes.forEach(node => {
            try { node.source.stop(); } catch (e) {}
        });
        scheduledNodes = [];
    }

    audioBufferQueue = [];
    isWebAudioPlaying = false;
    isStreaming = false;
    nextStartTime = 0;
    streamTotalSamples = 0;

    stopStreamUI();
}

function stopStreamUI() {
    STREAM_BTN.classList.remove('active');
    STREAM_BADGE.classList.add('hidden');
    STREAM_BTN.querySelector('span').textContent = '流式输出';
    const playerCard = AUDIO_PLAYER_CONTAINER;
    if (playerCard) playerCard.classList.remove('streaming');

    SYNTHESIZE_BTN.disabled = false;
    STREAM_BTN.disabled = false;
    stopWaveformAnimation();
}

function finalizeStreamAudio() {
    isStreaming = false;
    if (streamWs) {
        streamWs.close();
        streamWs = null;
    }

    if (streamCachedChunks.length > 0) {
        const completeWavBase64 = convertPCMChunksToWav(streamCachedChunks);
        currentAudioBase64 = completeWavBase64;
        loadAudioFromBase64(completeWavBase64);
        DOWNLOAD_BTN.disabled = false;
    }
}

function convertPCMChunksToWav(chunks) {
    let totalPCMSamples = 0;
    const decodedChunks = [];

    for (const chunk of chunks) {
        const { samples } = decodeBase64ToFloat32(chunk);
        decodedChunks.push(samples);
        totalPCMSamples += samples.length;
    }

    const allSamples = new Float32Array(totalPCMSamples);
    let offset = 0;
    for (const samples of decodedChunks) {
        allSamples.set(samples, offset);
        offset += samples.length;
    }

    const wavBytes = encodePCMToWAV(allSamples, streamSampleRate);
    return arrayBufferToBase64(wavBytes.buffer);
}

function arrayBufferToBase64(buffer) {
    const bytes = new Uint8Array(buffer);
    let binary = '';
    for (let i = 0; i < bytes.byteLength; i++) {
        binary += String.fromCharCode(bytes[i]);
    }
    return btoa(binary);
}

function encodePCMToWAV(samples, sampleRate) {
    const numSamples = samples.length;
    const byteRate = sampleRate * 2;
    const blockAlign = 2;
    const dataSize = numSamples * 2;
    const fileSize = 36 + dataSize;
    const buffer = new ArrayBuffer(44 + dataSize);
    const view = new DataView(buffer);

    writeString(view, 0, 'RIFF');
    view.setUint32(4, fileSize, true);
    writeString(view, 8, 'WAVE');
    writeString(view, 12, 'fmt ');
    view.setUint32(16, 16, true);
    view.setUint16(20, 1, true);
    view.setUint16(22, 1, true);
    view.setUint32(24, sampleRate, true);
    view.setUint32(28, byteRate, true);
    view.setUint16(32, blockAlign, true);
    view.setUint16(34, 16, true);
    writeString(view, 36, 'data');
    view.setUint32(40, dataSize, true);

    for (let i = 0; i < numSamples; i++) {
        const sample = Math.max(-1, Math.min(1, samples[i]));
        const val = sample < 0 ? sample * 32768 : sample * 32767;
        view.setInt16(44 + i * 2, val, true);
    }

    return new Uint8Array(buffer);
}

function writeString(view, offset, str) {
    for (let i = 0; i < str.length; i++) {
        view.setUint8(offset + i, str.charCodeAt(i));
    }
}

function loadAudioFromBase64(base64Audio) {
    const audioSrc = 'data:audio/wav;base64,' + base64Audio;
    AUDIO_PLAYER.src = audioSrc;
    AUDIO_PLAYER.load();
    resetPlayerControls();
    currentAudioBase64 = base64Audio;
    DOWNLOAD_BTN.disabled = false;
}

function resetPlayerControls() {
    ICON_PLAY.classList.remove('hidden');
    ICON_PAUSE.classList.add('hidden');
    PROGRESS_BAR.value = 0;
    TIME_DISPLAY.textContent = '0:00 / 0:00';
}

function stopAudio() {
    AUDIO_PLAYER.pause();
    AUDIO_PLAYER.currentTime = 0;
    resetPlayerControls();
}

function handleSeek() {
    if (!AUDIO_PLAYER.duration || !isFinite(AUDIO_PLAYER.duration)) return;
    const seekTime = (PROGRESS_BAR.value / 100) * AUDIO_PLAYER.duration;
    AUDIO_PLAYER.currentTime = seekTime;
}

function handleVolumeChange() {
    if (AUDIO_PLAYER) AUDIO_PLAYER.volume = VOLUME_SLIDER.value / 100;
    if (streamGainNode) streamGainNode.gain.value = VOLUME_SLIDER.value / 100;
}

function handleTimeUpdate() {
    if (AUDIO_PLAYER.duration && isFinite(AUDIO_PLAYER.duration)) {
        const progress = (AUDIO_PLAYER.currentTime / AUDIO_PLAYER.duration) * 100;
        PROGRESS_BAR.value = progress;
        TIME_DISPLAY.textContent = formatTime(AUDIO_PLAYER.currentTime) + ' / ' + formatTime(AUDIO_PLAYER.duration);
    }
}

function handleMetadataLoaded() {
    if (AUDIO_PLAYER.duration && isFinite(AUDIO_PLAYER.duration)) {
        TIME_DISPLAY.textContent = '0:00 / ' + formatTime(AUDIO_PLAYER.duration);
    } else {
        TIME_DISPLAY.textContent = '0:00 / 0:00';
    }
    PROGRESS_BAR.value = 0;
}

function handleAudioEnded() {
    resetPlayerControls();
}

function downloadAudio() {
    if (!currentAudioBase64) {
        showStatus('没有可下载的音频', 'error');
        return;
    }
    try {
        const byteCharacters = atob(currentAudioBase64);
        const byteNumbers = new Array(byteCharacters.length);
        for (let i = 0; i < byteCharacters.length; i++) {
            byteNumbers[i] = byteCharacters.charCodeAt(i);
        }
        const byteArray = new Uint8Array(byteNumbers);
        const blob = new Blob([byteArray], { type: 'audio/wav' });
        const url = URL.createObjectURL(blob);
        const a = document.createElement('a');
        a.href = url;
        a.download = 'tts_' + Date.now() + '.wav';
        document.body.appendChild(a);
        a.click();
        document.body.removeChild(a);
        URL.revokeObjectURL(url);
        showStatus('音频已保存', 'success');
    } catch (err) {
        console.error('下载失败', err);
        showStatus('下载失败，音频数据异常', 'error');
    }
}

function formatTime(seconds) {
    if (isNaN(seconds) || !isFinite(seconds)) return '0:00';
    const mins = Math.floor(seconds / 60);
    const secs = Math.floor(seconds % 60);
    return mins + ':' + String(secs).padStart(2, '0');
}

function showStatus(message, type) {
    STATUS_MESSAGE.textContent = message;
    STATUS_MESSAGE.className = 'status-toast ' + type + ' show';
    if (type !== 'info') {
        setTimeout(() => {
            STATUS_MESSAGE.classList.remove('show');
        }, 2800);
    }
}

function startWaveformAnimation() {
    if (animationId) return;
    if (!WAVEFORM) return;
    WAVEFORM.innerHTML = '';
    const barCount = 40;
    const bars = [];
    for (let i = 0; i < barCount; i++) {
        const bar = document.createElement('div');
        bar.className = 'waveform-bar';
        bar.style.animationDelay = (i * 0.04) + 's';
        bars.push(bar);
        WAVEFORM.appendChild(bar);
    }

    waveInterval = setInterval(() => {
        bars.forEach((bar, i) => {
            const baseHeight = 6 + Math.sin(Date.now() * 0.005 + i * 0.4) * 10;
            const randomBoost = Math.random() * 15;
            const h = Math.max(4, Math.min(55, baseHeight + randomBoost));
            bar.style.height = h + 'px';
        });
    }, 120);

    animationId = true;
}

function stopWaveformAnimation() {
    if (waveInterval) {
        clearInterval(waveInterval);
        waveInterval = null;
    }
    if (!animationId) return;
    if (WAVEFORM) WAVEFORM.innerHTML = '';
    animationId = null;
}

// ========== EQ Functions ==========

function initEQ() {
    if (eqCtx) return;

    eqCtx = new (window.AudioContext || window.webkitAudioContext)();

    eqLowFilter = eqCtx.createBiquadFilter();
    eqLowFilter.type = 'lowshelf';
    eqLowFilter.frequency.value = 200;
    eqLowFilter.gain.value = 0;

    eqMidFilter = eqCtx.createBiquadFilter();
    eqMidFilter.type = 'peaking';
    eqMidFilter.frequency.value = 1000;
    eqMidFilter.Q.value = 1;
    eqMidFilter.gain.value = 0;

    eqHighFilter = eqCtx.createBiquadFilter();
    eqHighFilter.type = 'highshelf';
    eqHighFilter.frequency.value = 8000;
    eqHighFilter.gain.value = 0;

    eqLowFilter.connect(eqMidFilter);
    eqMidFilter.connect(eqHighFilter);
    eqHighFilter.connect(eqCtx.destination);

    if (eqCtx.state === 'suspended') {
        eqCtx.resume();
    }
}

function connectEQToAudioElement() {
    if (!eqCtx) {
        initEQ();
    }

    if (!eqSourceNode && AUDIO_PLAYER) {
        eqSourceNode = eqCtx.createMediaElementSource(AUDIO_PLAYER);
        eqSourceNode.connect(eqLowFilter);
    }
}

function getEQValue(slider, mode, multi) {
    const sliderVal = parseFloat(slider.value);
    const multiplier = parseInt(multi.value, 10);
    const value = sliderVal * multiplier;
    return mode.value === 'cut' ? -value : value;
}

function updateEQ() {
    const lowGain = getEQValue(EQ_LOW_SLIDER, EQ_LOW_MODE, EQ_LOW_MULTI);
    const midGain = getEQValue(EQ_MID_SLIDER, EQ_MID_MODE, EQ_MID_MULTI);
    const highGain = getEQValue(EQ_HIGH_SLIDER, EQ_HIGH_MODE, EQ_HIGH_MULTI);

    EQ_LOW_VALUE.textContent = (lowGain > 0 ? '+' : '') + lowGain.toFixed(1) + 'dB';
    EQ_MID_VALUE.textContent = (midGain > 0 ? '+' : '') + midGain.toFixed(1) + 'dB';
    EQ_HIGH_VALUE.textContent = (highGain > 0 ? '+' : '') + highGain.toFixed(1) + 'dB';

    if (eqCtx) {
        if (eqLowFilter) eqLowFilter.gain.value = lowGain;
        if (eqMidFilter) eqMidFilter.gain.value = midGain;
        if (eqHighFilter) eqHighFilter.gain.value = highGain;
    }
}

function updateEQVisualizer() {
    if (!EQ_VIS_LOW || !EQ_VIS_MID || !EQ_VIS_HIGH) return;

    const lowGain = getEQValue(EQ_LOW_SLIDER, EQ_LOW_MODE, EQ_LOW_MULTI);
    const midGain = getEQValue(EQ_MID_SLIDER, EQ_MID_MODE, EQ_MID_MULTI);
    const highGain = getEQValue(EQ_HIGH_SLIDER, EQ_HIGH_MODE, EQ_HIGH_MULTI);

    const baselineY = 237;
    const minH = 15;
    const maxH = 200;
    const lowH = Math.max(minH, Math.min(maxH, 20 + (lowGain / 36) * 180));
    const midH = Math.max(minH, Math.min(maxH, 20 + (midGain / 36) * 180));
    const highH = Math.max(minH, Math.min(maxH, 20 + (highGain / 36) * 180));

    EQ_VIS_LOW.setAttribute('y', baselineY - lowH);
    EQ_VIS_LOW.setAttribute('height', lowH);
    EQ_VIS_MID.setAttribute('y', baselineY - midH);
    EQ_VIS_MID.setAttribute('height', midH);
    EQ_VIS_HIGH.setAttribute('y', baselineY - highH);
    EQ_VIS_HIGH.setAttribute('height', highH);
}

function resetEQ() {
    EQ_LOW_SLIDER.value = 0;
    EQ_LOW_MODE.value = 'boost';
    EQ_LOW_MULTI.value = '3';
    EQ_MID_SLIDER.value = 0;
    EQ_MID_MODE.value = 'boost';
    EQ_MID_MULTI.value = '3';
    EQ_HIGH_SLIDER.value = 0;
    EQ_HIGH_MODE.value = 'boost';
    EQ_HIGH_MULTI.value = '3';

    EQ_LOW_VALUE.textContent = '0dB';
    EQ_MID_VALUE.textContent = '0dB';
    EQ_HIGH_VALUE.textContent = '0dB';

    if (eqLowFilter) eqLowFilter.gain.value = 0;
    if (eqMidFilter) eqMidFilter.gain.value = 0;
    if (eqHighFilter) eqHighFilter.gain.value = 0;
}

// Override play function to ensure EQ is connected
const originalPlay = AUDIO_PLAYER.play;
AUDIO_PLAYER.play = function() {
    connectEQToAudioElement();
    return originalPlay.apply(AUDIO_PLAYER, arguments);
};

// Initialize application
document.addEventListener('DOMContentLoaded', init);