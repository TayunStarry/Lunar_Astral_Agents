const API_BASE = window.location.origin;

// DOM 元素
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
const PLAY_BTN = document.getElementById('play-btn');
const PAUSE_BTN = document.getElementById('pause-btn');
const STOP_BTN = document.getElementById('stop-btn');
const PROGRESS_BAR = document.getElementById('progress-bar');
const TIME_DISPLAY = document.getElementById('time-display');
const VOLUME_SLIDER = document.getElementById('volume-slider');
const WAVEFORM = document.getElementById('waveform');
const DOWNLOAD_BTN = document.getElementById('download-btn');

// 状态变量
let uploadedRefAudioPath = null;
let animationId = null;
let currentAudioBase64 = null;

// 初始化 (已移除健康检查与轮询)
function init() {
    setupEventListeners();
    TEXT_INPUT.focus();
    DOWNLOAD_BTN.disabled = true;
    // 初始状态：播放按钮可见，暂停按钮隐藏
    PLAY_BTN.classList.remove('hidden');
    PAUSE_BTN.classList.add('hidden');
    // 音量同步
    if (AUDIO_PLAYER) AUDIO_PLAYER.volume = VOLUME_SLIDER.value / 100;
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

    PLAY_BTN.addEventListener('click', playAudio);
    PAUSE_BTN.addEventListener('click', pauseAudio);
    STOP_BTN.addEventListener('click', stopAudio);
    PROGRESS_BAR.addEventListener('input', handleSeek);
    VOLUME_SLIDER.addEventListener('input', handleVolumeChange);
    DOWNLOAD_BTN.addEventListener('click', downloadAudio);

    AUDIO_PLAYER.addEventListener('timeupdate', handleTimeUpdate);
    AUDIO_PLAYER.addEventListener('ended', handleAudioEnded);
    AUDIO_PLAYER.addEventListener('loadedmetadata', handleMetadataLoaded);
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
            // 上传失败时清空文件选择器，让用户可重试
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
    showStatus('🎙️ 正在生成语音，请稍后...', 'info');
    startWaveformAnimation();

    try {
        const requestBody = { text: text };
        if (uploadedRefAudioPath) {
            requestBody.ref_audio = uploadedRefAudioPath;
        }

        const response = await fetch(`${API_BASE}/tts/`, {
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
        stopWaveformAnimation();
    }
}

function loadAudioFromBase64(base64Audio) {
    const audioSrc = `data:audio/wav;base64,${base64Audio}`;
    AUDIO_PLAYER.src = audioSrc;
    AUDIO_PLAYER.load();
    resetPlayerControls();
    currentAudioBase64 = base64Audio;
    DOWNLOAD_BTN.disabled = false;
}

function playAudio() {
    if (!AUDIO_PLAYER.src) {
        showStatus('没有可播放的音频', 'error');
        return;
    }
    AUDIO_PLAYER.play().then(() => {
        PLAY_BTN.classList.add('hidden');
        PAUSE_BTN.classList.remove('hidden');
    }).catch(error => {
        console.error('播放失败:', error);
        showStatus('播放失败，请重试', 'error');
        // 播放失败时恢复按钮状态
        PLAY_BTN.classList.remove('hidden');
        PAUSE_BTN.classList.add('hidden');
    });
}

function pauseAudio() {
    AUDIO_PLAYER.pause();
    PLAY_BTN.classList.remove('hidden');
    PAUSE_BTN.classList.add('hidden');
}

function stopAudio() {
    AUDIO_PLAYER.pause();
    AUDIO_PLAYER.currentTime = 0;
    resetPlayerControls();
}

function resetPlayerControls() {
    PLAY_BTN.classList.remove('hidden');
    PAUSE_BTN.classList.add('hidden');
    PROGRESS_BAR.value = 0;
    TIME_DISPLAY.textContent = '0:00 / 0:00';
}

function handleSeek() {
    if (!AUDIO_PLAYER.duration || !isFinite(AUDIO_PLAYER.duration)) return;
    const seekTime = (PROGRESS_BAR.value / 100) * AUDIO_PLAYER.duration;
    AUDIO_PLAYER.currentTime = seekTime;
}

function handleVolumeChange() {
    if (AUDIO_PLAYER) AUDIO_PLAYER.volume = VOLUME_SLIDER.value / 100;
}

function handleTimeUpdate() {
    if (AUDIO_PLAYER.duration && isFinite(AUDIO_PLAYER.duration)) {
        const progress = (AUDIO_PLAYER.currentTime / AUDIO_PLAYER.duration) * 100;
        PROGRESS_BAR.value = progress;
        TIME_DISPLAY.textContent = `${formatTime(AUDIO_PLAYER.currentTime)} / ${formatTime(AUDIO_PLAYER.duration)}`;
    }
}

function handleMetadataLoaded() {
    if (AUDIO_PLAYER.duration && isFinite(AUDIO_PLAYER.duration)) {
        TIME_DISPLAY.textContent = `0:00 / ${formatTime(AUDIO_PLAYER.duration)}`;
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
        a.download = `tts_${Date.now()}.wav`;
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
    if (isNaN(seconds) || !isFinite(seconds)) return "0:00";
    const mins = Math.floor(seconds / 60);
    const secs = Math.floor(seconds % 60);
    return `${mins}:${secs.toString().padStart(2, '0')}`;
}

function showStatus(message, type) {
    STATUS_MESSAGE.textContent = message;
    STATUS_MESSAGE.className = `status-toast ${type} show`;
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
    for (let i = 0; i < barCount; i++) {
        const bar = document.createElement('div');
        bar.className = 'waveform-bar';
        bar.style.animationDelay = `${i * 0.04}s`;
        bar.style.height = '6px';
        WAVEFORM.appendChild(bar);
    }
    animationId = true;
}

function stopWaveformAnimation() {
    if (!animationId) return;
    if (WAVEFORM) WAVEFORM.innerHTML = '';
    animationId = null;
}

// 启动应用
document.addEventListener('DOMContentLoaded', init);