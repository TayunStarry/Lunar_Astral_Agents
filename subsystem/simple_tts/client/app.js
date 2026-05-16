const API_BASE = window.location.origin;
const STATUS_INDICATOR = document.getElementById('status-indicator');
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

let uploadedRefAudioPath = null;
let audioContext = null;
let animationId = null;
let currentAudioBase64 = null;

function init() {
    checkServerStatus();
    setupEventListeners();
    TEXT_INPUT.focus();
    DOWNLOAD_BTN.disabled = true;
}

async function checkServerStatus() {
    try {
        const response = await fetch(`${API_BASE}/health`, { method: 'GET' });
        if (response.ok) {
            STATUS_INDICATOR.className = 'status-indicator online';
            STATUS_INDICATOR.querySelector('.status-text').textContent = '在线';
        }
    } catch (error) {
        STATUS_INDICATOR.className = 'status-indicator offline';
        STATUS_INDICATOR.querySelector('.status-text').textContent = '离线';
    }
}

function setupEventListeners() {
    TEXT_INPUT.addEventListener('input', handleTextInput);
    
    UPLOAD_AREA.addEventListener('click', () => AUDIO_UPLOAD.click());
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
    
    AUDIO_PLAYER.volume = VOLUME_SLIDER.value / 100;
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
            showStatus('参考音频已上传', 'success');
        } else {
            showStatus('上传失败: ' + result.error, 'error');
        }
    } catch (error) {
        showStatus('上传失败: ' + error.message, 'error');
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
        showStatus('请输入要合成的文本', 'error');
        TEXT_INPUT.focus();
        return;
    }
    
    SYNTHESIZE_BTN.disabled = true;
    showStatus('正在合成语音，请稍候...', 'info');
    startWaveformAnimation();
    
    try {
        const requestBody = {
            text: text
        };
        
        if (uploadedRefAudioPath) {
            requestBody.ref_audio = uploadedRefAudioPath;
        }
        
        const response = await fetch(`${API_BASE}/tts/`, {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify(requestBody)
        });
        
        const result = await response.json();
        
        if (result.success) {
            loadAudioFromBase64(result.audio);
            showStatus('语音合成成功！', 'success');
            AUDIO_PLAYER_CONTAINER.classList.remove('hidden');
        } else {
            showStatus('合成失败: ' + result.error, 'error');
        }
    } catch (error) {
        showStatus('合成失败: ' + error.message, 'error');
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
    AUDIO_PLAYER.play().then(() => {
        PLAY_BTN.classList.add('hidden');
        PAUSE_BTN.classList.remove('hidden');
    }).catch(error => {
        console.error('播放失败:', error);
        showStatus('播放失败', 'error');
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
    const seekTime = (PROGRESS_BAR.value / 100) * AUDIO_PLAYER.duration;
    AUDIO_PLAYER.currentTime = seekTime;
}

function handleVolumeChange() {
    AUDIO_PLAYER.volume = VOLUME_SLIDER.value / 100;
}

function handleTimeUpdate() {
    if (AUDIO_PLAYER.duration) {
        const progress = (AUDIO_PLAYER.currentTime / AUDIO_PLAYER.duration) * 100;
        PROGRESS_BAR.value = progress;
        TIME_DISPLAY.textContent = `${formatTime(AUDIO_PLAYER.currentTime)} / ${formatTime(AUDIO_PLAYER.duration)}`;
    }
}

function handleMetadataLoaded() {
    TIME_DISPLAY.textContent = `0:00 / ${formatTime(AUDIO_PLAYER.duration)}`;
}

function handleAudioEnded() {
    resetPlayerControls();
}

function downloadAudio() {
    if (!currentAudioBase64) {
        showStatus('没有可下载的音频', 'error');
        return;
    }
    
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
    
    showStatus('音频已下载', 'success');
}

function formatTime(seconds) {
    const mins = Math.floor(seconds / 60);
    const secs = Math.floor(seconds % 60);
    return `${mins}:${secs.toString().padStart(2, '0')}`;
}

function showStatus(message, type) {
    STATUS_MESSAGE.textContent = message;
    STATUS_MESSAGE.className = `status-message ${type}`;
    
    if (type !== 'info') {
        setTimeout(() => {
            STATUS_MESSAGE.className = 'status-message';
        }, 3000);
    }
}

function startWaveformAnimation() {
    if (animationId) return;
    
    WAVEFORM.innerHTML = '';
    const barCount = 50;
    
    for (let i = 0; i < barCount; i++) {
        const bar = document.createElement('div');
        bar.className = 'waveform-bar';
        bar.style.animationDelay = `${i * 0.05}s`;
        bar.style.height = '8px';
        WAVEFORM.appendChild(bar);
    }
    
    animationId = true;
}

function stopWaveformAnimation() {
    if (!animationId) return;
    
    WAVEFORM.innerHTML = '';
    animationId = null;
}

document.addEventListener('DOMContentLoaded', init);

setInterval(checkServerStatus, 5000);
