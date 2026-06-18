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
const THEME_TOGGLE = document.getElementById('theme-toggle');

// Batch (全文朗读) DOM Elements
const BATCH_BTN = document.getElementById('batch-btn');
const BATCH_MODAL = document.getElementById('batch-modal');
const BATCH_CLOSE_BTN = document.getElementById('batch-close-btn');
const BATCH_UPLOAD_AREA = document.getElementById('batch-upload-area');
const BATCH_FILE_INPUT = document.getElementById('batch-file-input');
const BATCH_UPLOAD_CONTENT = document.getElementById('batch-upload-content');
const BATCH_FILE_INFO = document.getElementById('batch-file-info');
const BATCH_FILE_NAME = document.getElementById('batch-file-name');
const BATCH_CHUNK_COUNT = document.getElementById('batch-chunk-count');
const BATCH_REMOVE_FILE = document.getElementById('batch-remove-file');
const BATCH_START_INDEX = document.getElementById('batch-start-index');
const BATCH_START_BTN = document.getElementById('batch-start-btn');
const BATCH_STOP_BTN = document.getElementById('batch-stop-btn');
const BATCH_QUEUE = document.getElementById('batch-queue');
const QUEUE_PROGRESS = document.getElementById('queue-progress');
const QUEUE_PROGRESS_BAR = document.getElementById('queue-progress-bar');
const QUEUE_CURRENT_TEXT = document.getElementById('queue-current-text');
const QUEUE_LIST = document.getElementById('queue-list');

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

// Batch (全文朗读) state
let batchChunks = [];           // 清洗切片后的文本数组
let batchQueue = [];            // 队列项 [{text, status: 'pending'|'active'|'done'|'cancelled'}]
let batchRunning = false;       // 是否正在合成
let batchAborted = false;       // 是否已中断
let batchCurrentIndex = -1;     // 当前正在合成的索引
let batchProcessedCount = 0;    // 已完成的条目数

function init() {
    setupEventListeners();
    initTheme();
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

    // Batch (全文朗读) event listeners
    BATCH_BTN.addEventListener('click', openBatchModal);
    BATCH_CLOSE_BTN.addEventListener('click', closeBatchModal);
    BATCH_MODAL.addEventListener('click', (e) => { if (e.target === BATCH_MODAL) closeBatchModal(); });
    BATCH_UPLOAD_AREA.addEventListener('click', (e) => {
        if (!e.target.closest('#batch-remove-file') && !e.target.closest('#batch-file-info')) {
            BATCH_FILE_INPUT.click();
        }
    });
    BATCH_FILE_INPUT.addEventListener('change', handleBatchFileSelect);
    BATCH_UPLOAD_AREA.addEventListener('dragover', handleBatchDragOver);
    BATCH_UPLOAD_AREA.addEventListener('dragleave', handleBatchDragLeave);
    BATCH_UPLOAD_AREA.addEventListener('drop', handleBatchDrop);
    BATCH_REMOVE_FILE.addEventListener('click', removeBatchFile);
    BATCH_START_BTN.addEventListener('click', startBatchSynthesis);
    BATCH_STOP_BTN.addEventListener('click', stopBatchSynthesis);
    BATCH_START_INDEX.addEventListener('input', validateStartIndex);

    // 暗色模式切换
    THEME_TOGGLE.addEventListener('click', toggleTheme);
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
            startWaveformAnimation();
        }).catch(error => {
            console.error('播放失败:', error);
            showStatus('播放失败，请重试', 'error');
        });
    } else {
        AUDIO_PLAYER.pause();
        ICON_PLAY.classList.remove('hidden');
        ICON_PAUSE.classList.add('hidden');
        stopWaveformAnimation();
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

    try {
        showStatus('正在上传音频...', 'info');
        const arrayBuffer = await file.arrayBuffer();
        // 计算文件内容的校验码，取前8位作为文件名
        const hashBuffer = await crypto.subtle.digest('SHA-256', arrayBuffer);
        const hashArray = Array.from(new Uint8Array(hashBuffer));
        const hashPrefix = hashArray.slice(0, 4).map(b => b.toString(16).padStart(2, '0')).join('');
        const saveFileName = 'audios/' + hashPrefix + ext;
        const encodedFileName = btoa(unescape(encodeURIComponent(saveFileName)));

        const response = await fetch(`${API_BASE}/file/write`, {
            method: 'POST',
            headers: { 'X-File-Name': encodedFileName, 'X-Overwrite': 'true' },
            body: arrayBuffer
        });

        if (!response.ok) {
            const errorText = await response.text();
            showStatus('上传失败: ' + errorText, 'error');
            AUDIO_UPLOAD.value = '';
            return;
        }

        const result = await response.json();
        uploadedRefAudioPath = result.path;
        FILE_NAME.textContent = file.name;
        FILE_INFO.classList.remove('hidden');
        UPLOAD_CONTENT.classList.add('hidden');
        showStatus('参考音频已就绪', 'success');
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
            try { node.source.stop(); } catch (e) { }
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
    stopWaveformAnimation();
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
    stopWaveformAnimation();
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
        bar.style.height = '6px';
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
AUDIO_PLAYER.play = function () {
    connectEQToAudioElement();
    return originalPlay.apply(AUDIO_PLAYER, arguments);
};

// ========== Batch (全文朗读) Functions ==========

/**
 * 清洗文本，去除Markdown标记、行内代码、HTML标签、表情符号等不适合语音合成的内容
 * 与 server_side/model/narrator.ts 中的 cleanTextForTTS 逻辑完全一致
 *
 * @param {string} text - 待清洗文本
 * @returns {string} 清洗后的文本
 */
function cleanTextForTTS(text) {
    if (!text) return '';
    let processed = text;
    // 移除行内代码
    processed = processed.replace(/`[^`]*`/g, '');
    // 移除图片标记 ![alt](url)
    processed = processed.replace(/!\[.*?\]\(.*?\)/g, '');
    // 移除链接标记 [text](url)，保留链接文字
    processed = processed.replace(/\[([^\]]*)\]\(.*?\)/g, '$1');
    // 移除HTML标签
    processed = processed.replace(/<[^>]*>/g, '');
    // 移除emoji表情符号
    processed = processed.replace(/[\u{1F600}-\u{1F64F}\u{1F300}-\u{1F5FF}\u{1F680}-\u{1F6FF}\u{1F900}-\u{1F9FF}\u{2600}-\u{26FF}\u{2700}-\u{27BF}\u{FE00}-\u{FE0F}\u{1F1E0}-\u{1F1FF}\u{200D}\u{20E3}\u{FE0F}]/gu, '');
    // 移除星号（Markdown加粗/斜体标记）
    processed = processed.replace(/\*/g, '');
    // 将换行符替换为空格
    processed = processed.replace(/\r?\n/g, ' ');
    // 移除中文括号内的内容（通常是注释或补充说明）
    processed = processed.replace(/\（[^）]*\）/g, '');
    // 移除英文括号内的内容
    processed = processed.replace(/\([^)]*\)/g, '');
    // 白名单过滤：仅保留中文、英文、数字、常用中英文标点
    const allowed = '\\u4e00-\\u9fff' + 'a-zA-Z0-9' + '\\s_~\\-' + '\uFF0C\u3002\uFF1F\uFF1A\uFF01\uFF1B\u3001\u2014\u2026\u300A\u300B\u201C\u201D\u2018\u2019\uFF08\uFF09\u3010\u3011' + ',.\'\"?:!;';
    const whitelist = new RegExp(`[^${allowed}]`, 'g');
    processed = processed.replace(whitelist, '\uFF0C');
    // 合并多余空格
    processed = processed.replace(/\s+/g, ' ');
    return processed.trim();
}

/**
 * 将清洗后的文本进行二级智能分句
 * 与 server_side/model/narrator.ts 中的 splitSentences 逻辑完全一致
 *
 * @param {string} text - 清洗后的文本
 * @returns {string[]} 句子数组
 */
function splitSentences(text) {
    if (!text) return [];

    const LEVEL1_PUNCT = /[。？！—～?!]/;
    const LEVEL2_PUNCT = /[，,、；;]/;
    const MAX_LENGTH = 35;

    function splitByPunct(source, punctRegex) {
        const result = [];
        let start = 0;

        for (let i = 0; i < source.length; i++) {
            if (punctRegex.test(source[i])) {
                let end = i + 1;
                while (end < source.length && punctRegex.test(source[end])) {
                    end++;
                }
                const fragment = source.slice(start, end).trim();
                if (fragment.length > 0) {
                    result.push(fragment);
                }
                start = end;
                i = end - 1;
            }
        }

        if (start < source.length) {
            const fragment = source.slice(start).trim();
            if (fragment.length > 0) {
                result.push(fragment);
            }
        }

        return result;
    }

    // 一级切片
    const level1 = splitByPunct(text, LEVEL1_PUNCT);

    // 二级切片：对超过35字符的片段进行逗号切分
    const result = [];
    for (const fragment of level1) {
        if (fragment.length <= MAX_LENGTH) {
            result.push(fragment);
            continue;
        }

        let remaining = fragment;
        while (remaining.length > MAX_LENGTH) {
            let splitPos = -1;
            for (let i = Math.min(remaining.length - 1, MAX_LENGTH - 1); i >= 0; i--) {
                if (LEVEL2_PUNCT.test(remaining[i])) {
                    let end = i + 1;
                    while (end < remaining.length && LEVEL2_PUNCT.test(remaining[end])) {
                        end++;
                    }
                    splitPos = end;
                    break;
                }
            }

            // 无逗号可切：强制按 MAX_LENGTH 长度切分，避免产生超长片段
            if (splitPos === -1) {
                splitPos = MAX_LENGTH;
            }

            const slice = remaining.slice(0, splitPos).trim();
            if (slice.length > 0) {
                result.push(slice);
            }
            remaining = remaining.slice(splitPos);
        }

        const tail = remaining.trim();
        if (tail.length > 0) {
            result.push(tail);
        }
    }

    return result;
}

/**
 * 提取思考区内容（<think>...</think>）并从文本中移除
 * @param {string} text
 * @returns {[string[], string]} [思考区数组, 移除后的文本]
 */
function extractThinkingBlocks(text) {
    const blocks = [];
    const regex = /<think>([\s\S]*?)<\/think>/gi;
    let match;
    while ((match = regex.exec(text)) !== null) {
        const content = match[1].trim();
        if (content.length > 0) {
            blocks.push(content);
        }
    }
    const remaining = text.replace(/<think>[\s\S]*?<\/think>/gi, '');
    return [blocks, remaining];
}

/**
 * 提取代码块内容（```...```）并从文本中移除
 * @param {string} text
 * @returns {[string[], string]} [代码块数组, 移除后的文本]
 */
function extractCodeBlocks(text) {
    const blocks = [];
    const codeBlockRegex = /```[a-zA-Z0-9+#-]*[\s\S]*?```/g;
    let match;
    while ((match = codeBlockRegex.exec(text)) !== null) {
        blocks.push(match[0]);
    }
    const remaining = text.replace(/```[a-zA-Z0-9+#-]*[\s\S]*?```/g, '');
    return [blocks, remaining];
}

/**
 * 完整的文本解析流程：思考区提取 → 代码块提取 → 清洗 → 切片
 * @param {string} rawText
 * @returns {string[]} 清洗切片后的文本数组
 */
function parseContent(rawText) {
    if (!rawText) return [];
    const [, textAfterThinking] = extractThinkingBlocks(rawText);
    const [, textAfterCode] = extractCodeBlocks(textAfterThinking);
    const cleanedText = cleanTextForTTS(textAfterCode);
    return splitSentences(cleanedText);
}

// ========== Modal 控制 ==========

function openBatchModal() {
    BATCH_MODAL.classList.remove('hidden');
}

function closeBatchModal() {
    if (batchRunning) {
        showStatus('正在合成中，请先中断合成', 'error');
        return;
    }
    BATCH_MODAL.classList.add('hidden');
}

// ========== 文件处理 ==========

function handleBatchDragOver(e) {
    e.preventDefault();
    BATCH_UPLOAD_AREA.classList.add('dragging');
}

function handleBatchDragLeave() {
    BATCH_UPLOAD_AREA.classList.remove('dragging');
}

async function handleBatchDrop(e) {
    e.preventDefault();
    BATCH_UPLOAD_AREA.classList.remove('dragging');
    const files = e.dataTransfer.files;
    if (files.length > 0) {
        await processBatchFile(files[0]);
    }
}

function handleBatchFileSelect(e) {
    const file = e.target.files[0];
    if (file) {
        processBatchFile(file);
    }
}

/**
 * 验证并处理上传的文件
 */
async function processBatchFile(file) {
    const validExts = ['.txt', '.md'];
    const ext = '.' + file.name.split('.').pop().toLowerCase();
    if (!validExts.includes(ext)) {
        showStatus('仅支持 TXT 或 MD 格式文件，当前文件类型: ' + ext, 'error');
        BATCH_FILE_INPUT.value = '';
        return;
    }

    // 限制文件大小（5MB）
    const maxSize = 5 * 1024 * 1024;
    if (file.size > maxSize) {
        showStatus('文件过大，最大支持 5MB', 'error');
        BATCH_FILE_INPUT.value = '';
        return;
    }

    try {
        const rawText = await readFileAsText(file);
        if (!rawText || rawText.trim().length === 0) {
            showStatus('文件内容为空', 'error');
            return;
        }

        // 使用与 narrator.ts 一致的解析流程
        batchChunks = parseContent(rawText);

        if (batchChunks.length === 0) {
            showStatus('文件解析后无可合成的文本段落', 'warning');
            return;
        }

        // 更新UI
        BATCH_FILE_NAME.textContent = file.name;
        BATCH_CHUNK_COUNT.textContent = batchChunks.length + ' 段';
        BATCH_FILE_INFO.classList.remove('hidden');
        BATCH_UPLOAD_CONTENT.classList.add('hidden');

        // 更新起始段落的最大值
        BATCH_START_INDEX.max = batchChunks.length;
        BATCH_START_INDEX.value = 1;

        // 启用开始按钮
        BATCH_START_BTN.disabled = false;

        showStatus('文件已加载，共 ' + batchChunks.length + ' 个段落', 'success');
    } catch (error) {
        showStatus('文件读取失败: ' + error.message, 'error');
        BATCH_FILE_INPUT.value = '';
    }
}

/**
 * 使用 FileReader 读取文件内容
 */
function readFileAsText(file) {
    return new Promise((resolve, reject) => {
        const reader = new FileReader();
        reader.onload = (e) => resolve(e.target.result);
        reader.onerror = () => reject(new Error('无法读取文件'));
        reader.readAsText(file, 'UTF-8');
    });
}

function removeBatchFile(e) {
    e.stopPropagation();
    batchChunks = [];
    BATCH_FILE_INPUT.value = '';
    BATCH_FILE_INFO.classList.add('hidden');
    BATCH_UPLOAD_CONTENT.classList.remove('hidden');
    BATCH_START_BTN.disabled = true;
    BATCH_START_INDEX.value = 1;
    BATCH_START_INDEX.max = 1;
    BATCH_QUEUE.classList.add('hidden');
}

// ========== 起始段落校验 ==========

function validateStartIndex() {
    const val = parseInt(BATCH_START_INDEX.value, 10);
    if (isNaN(val) || val < 1) {
        BATCH_START_INDEX.value = 1;
    } else if (batchChunks.length > 0 && val > batchChunks.length) {
        BATCH_START_INDEX.value = batchChunks.length;
    }
}

// ========== 合成队列状态渲染 ==========

function renderQueue() {
    QUEUE_LIST.innerHTML = '';
    const visibleCount = Math.min(batchQueue.length, 30); // 最多显示30条

    for (let i = 0; i < visibleCount; i++) {
        const item = batchQueue[batchQueue.length - visibleCount + i];
        const el = document.createElement('div');
        el.className = 'queue-item queue-item-' + item.status;
        el.innerHTML = '<span class="queue-item-icon"></span><span class="queue-item-text">' + escapeHtml(item.text) + '</span>';
        QUEUE_LIST.appendChild(el);
    }

    // 更新进度文本
    const total = batchQueue.length;
    const done = batchProcessedCount;
    QUEUE_PROGRESS.textContent = done + ' / ' + total;

    // 更新进度条
    const pct = total > 0 ? (done / total * 100) : 0;
    QUEUE_PROGRESS_BAR.style.width = pct + '%';

    // 更新当前文本
    if (batchCurrentIndex >= 0 && batchCurrentIndex < batchQueue.length) {
        QUEUE_CURRENT_TEXT.textContent = '\u25B6 ' + batchQueue[batchCurrentIndex].text;
    } else {
        QUEUE_CURRENT_TEXT.textContent = '';
    }
}

function escapeHtml(text) {
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
}

// ========== 批量合成主逻辑 ==========

/** 小延迟辅助函数 */
function sleep(ms) {
    return new Promise(resolve => setTimeout(resolve, ms));
}

/**
 * 批量合成入口
 * producer 收到一条音频后立刻请求下一条，不等播放结束
 * consumer 通过 <audio> 元素串行播放，走常规合成EQ链路（MediaElementSource）
 */
async function startBatchSynthesis() {
    if (batchChunks.length === 0) {
        showStatus('请先导入文件', 'error');
        return;
    }

    validateStartIndex();
    const startIndex = parseInt(BATCH_START_INDEX.value, 10) - 1;

    if (startIndex < 0 || startIndex >= batchChunks.length) {
        showStatus('起始段落超出范围', 'error');
        return;
    }

    // 构建队列（从指定段落开始）
    batchQueue = batchChunks.slice(startIndex).map(text => ({
        text: text,
        status: 'pending',
        _audio: null  // 存放返回的 base64 音频
    }));

    batchRunning = true;
    batchAborted = false;
    batchCurrentIndex = 0;
    batchProcessedCount = 0;

    // 更新UI状态
    BATCH_START_BTN.disabled = true;
    BATCH_STOP_BTN.disabled = false;
    BATCH_UPLOAD_AREA.style.pointerEvents = 'none';
    BATCH_UPLOAD_AREA.style.opacity = '0.5';
    BATCH_START_INDEX.disabled = true;
    BATCH_QUEUE.classList.remove('hidden');

    SYNTHESIZE_BTN.disabled = true;
    STREAM_BTN.disabled = true;

    showStatus('批量合成开始，共 ' + batchQueue.length + ' 段', 'info');
    renderQueue();

    const total = batchQueue.length;
    let fetchIdx = 0;   // 下一条要请求的索引

    /**
     * producer — 持续请求合成，收到音频塞入队列即请求下一条
     */
    const producer = async () => {
        console.log('[Batch] producer 启动, total:', total);
        while (fetchIdx < total && !batchAborted) {
            const i = fetchIdx;
            console.log('[Batch] producer 请求第', i, '段');
            batchQueue[i].status = 'fetching';
            renderQueue();

            try {
                batchQueue[i]._audio = await fetchSingleChunk(batchQueue[i].text);
                console.log('[Batch] producer 第', i, '段完成, 立即请求下一段');
            } catch (err) {
                console.error('[Batch] 段落' + (i + 1) + '合成失败:', err);
                batchQueue[i].status = 'cancelled';
                batchProcessedCount++;
            }
            fetchIdx++;
            if (!batchAborted) renderQueue();
        }
        console.log('[Batch] producer 结束, fetchIdx:', fetchIdx);
    };

    /**
     * consumer — 串行播放已合成的音频（通过 <audio> 元素 → EQ MediaElementSource）
     */
    const consumer = async () => {
        for (let i = 0; i < total; i++) {
            if (batchAborted) break;

            // 等待当前索引的音频合成完成（如果尚未完成）
            while (!batchAborted && !batchQueue[i]._audio && batchQueue[i].status !== 'cancelled') {
                await sleep(80);
            }
            if (batchAborted) break;

            // 已标记为取消的跳过
            if (batchQueue[i].status === 'cancelled') continue;

            batchCurrentIndex = i;
            batchQueue[i].status = 'active';
            renderQueue();

            // 通过 audio 元素播放（走常规合成EQ链路）
            await playBatchAudio(batchQueue[i]._audio);

            if (batchAborted) {
                batchQueue[i].status = 'cancelled';
                break;
            }

            batchQueue[i].status = 'done';
            batchProcessedCount++;
            renderQueue();
        }
    };

    // 并行启动 producer 和 consumer
    await Promise.all([producer(), consumer()]);

    // 完成或中断后的清理
    finishBatchSynthesis();
}

/**
 * 发送单条文本到后端合成，返回 base64 音频数据（不播放）
 * 与常规合成使用同一 /tts 端点
 */
async function fetchSingleChunk(text) {
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
        return result.audio;
    }
    throw new Error(result.error || '服务端合成失败');
}

/**
 * 通过 <audio> 元素播放一段 base64 WAV 音频
 * 音频自动经过 MediaElementSource → EQ 滤镜链，无需额外处理
 * 换源时先 load() 确保浏览器解码完成再播放，避免连续切换时卡片
 *
 * @param {string} base64Audio - base64编码的WAV音频
 * @returns {Promise<void>} 播放完毕resolve
 */
function playBatchAudio(base64Audio) {
    return new Promise((resolve) => {
        let resolved = false;

        const cleanup = () => {
            AUDIO_PLAYER.removeEventListener('ended', onEnded);
            AUDIO_PLAYER.removeEventListener('pause', onPause);
            AUDIO_PLAYER.removeEventListener('loadeddata', onLoadedData);
        };

        const finish = () => {
            if (resolved) return;
            resolved = true;
            cleanup();
            resolve();
        };

        const onEnded = () => finish();
        // 中断合成时会 pause()，此时也应释放阻塞
        const onPause = () => {
            if (batchAborted) finish();
        };

        // 使用 loadeddata 兜底：数据已加载可播放
        const onLoadedData = () => {
            AUDIO_PLAYER.removeEventListener('loadeddata', onLoadedData);
            AUDIO_PLAYER.play().catch(() => finish());
        };

        AUDIO_PLAYER.addEventListener('ended', onEnded);
        AUDIO_PLAYER.addEventListener('pause', onPause);
        AUDIO_PLAYER.addEventListener('loadeddata', onLoadedData);
        AUDIO_PLAYER.src = 'data:audio/wav;base64,' + base64Audio;
        AUDIO_PLAYER.load();

        // data URL 为同步加载，直接尝试播放（ most reliable for data: URIs ）
        AUDIO_PLAYER.play().catch(() => {
            // 若立即播放失败，等待 loadeddata 事件兜底
        });
    });
}

function stopBatchSynthesis() {
    if (!batchRunning) return;

    batchAborted = true;
    showStatus('正在中断合成...', 'info');

    // 停止当前正在播放的音频，让 playBatchAudio 的 ended 监听 resolve
    AUDIO_PLAYER.pause();
    AUDIO_PLAYER.currentTime = 0;

    // 将未完成（包括 fetching）的队列项标记为取消
    for (let i = batchCurrentIndex; i < batchQueue.length; i++) {
        if (batchQueue[i].status === 'pending' || batchQueue[i].status === 'fetching') {
            batchQueue[i].status = 'cancelled';
        }
    }
    renderQueue();
}

function finishBatchSynthesis() {
    batchRunning = false;
    batchCurrentIndex = -1;

    // 恢复UI状态
    BATCH_START_BTN.disabled = false;
    BATCH_STOP_BTN.disabled = true;
    BATCH_UPLOAD_AREA.style.pointerEvents = '';
    BATCH_UPLOAD_AREA.style.opacity = '';
    BATCH_START_INDEX.disabled = false;

    SYNTHESIZE_BTN.disabled = false;
    STREAM_BTN.disabled = false;

    if (batchAborted) {
        showStatus('批量合成已中断，已完成 ' + batchProcessedCount + ' 段', 'info');
    } else {
        showStatus('批量合成完成，共 ' + batchProcessedCount + ' 段', 'success');
    }
}

// ========== 暗色模式 ==========

function initTheme() {
    const saved = localStorage.getItem('theme');
    if (saved === 'dark') {
        document.body.classList.add('dark-mode');
        updateThemeIcon(true);
    } else if (saved === 'light') {
        document.body.classList.remove('dark-mode');
        updateThemeIcon(false);
    } else {
        // 跟随系统偏好
        const prefersDark = window.matchMedia('(prefers-color-scheme: dark)').matches;
        if (prefersDark) {
            document.body.classList.add('dark-mode');
            updateThemeIcon(true);
        } else {
            updateThemeIcon(false);
        }
    }

    // 监听系统主题变化
    window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', (e) => {
        if (!localStorage.getItem('theme')) {
            if (e.matches) {
                document.body.classList.add('dark-mode');
                updateThemeIcon(true);
            } else {
                document.body.classList.remove('dark-mode');
                updateThemeIcon(false);
            }
        }
    });
}

function toggleTheme() {
    const isDark = document.body.classList.toggle('dark-mode');
    updateThemeIcon(isDark);
    localStorage.setItem('theme', isDark ? 'dark' : 'light');
}

function updateThemeIcon(isDark) {
    const icon = THEME_TOGGLE.querySelector('i');
    if (isDark) {
        icon.className = 'fas fa-sun';
    } else {
        icon.className = 'fas fa-moon';
    }
}

// Initialize application
document.addEventListener('DOMContentLoaded', init);