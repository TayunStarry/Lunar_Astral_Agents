// ============================================================
//  星月智能 · 消息终端 — 语音输入（麦克风录音 → WAV 待发送附件）
// ============================================================

// ---------- 录音状态 ----------
let mediaRecorder = null;          // MediaRecorder 实例
let recordChunks = [];             // 录音数据块
let recordStream = null;           // 麦克风媒体流
const SAMPLE_RATE = 16000;         // 目标采样率（音频理解标准输入）

// ---------- 语音输入 ----------
function setupVoiceInput() {
    if (!voiceBtn) return;
    voiceBtn.addEventListener('click', () => {
        if (mediaRecorder && mediaRecorder.state === 'recording') {
            mediaRecorder.stop();
        } else {
            startRecording();
        }
    });
}

// 开始录音：请求麦克风权限并启动 MediaRecorder
async function startRecording() {
    if (!navigator.mediaDevices || !window.MediaRecorder) {
        showToast('当前环境不支持录音', 'error');
        return;
    }
    try {
        recordStream = await navigator.mediaDevices.getUserMedia({ audio: true });
    } catch (err) {
        showToast('麦克风权限被拒绝，无法录音', 'error');
        return;
    }
    try {
        mediaRecorder = new MediaRecorder(recordStream);
    } catch (err) {
        showToast('录音启动失败', 'error');
        recordStream.getTracks().forEach(track => track.stop());
        return;
    }
    recordChunks = [];
    mediaRecorder.ondataavailable = (e) => {
        if (e.data && e.data.size > 0) recordChunks.push(e.data);
    };
    mediaRecorder.onstop = () => finalizeRecording();
    mediaRecorder.start();
    // 更新按钮为录音中状态
    voiceBtn.classList.add('recording');
    voiceBtn.title = '停止录音';
    voiceBtn.innerHTML = '<i class="fas fa-stop"></i>';
    showToast('开始录音，再次点击结束', 'info');
}

// 结束录音：释放媒体流，将录音转为 WAV 待发送附件
function finalizeRecording() {
    // 恢复按钮状态
    voiceBtn.classList.remove('recording');
    voiceBtn.title = '语音输入';
    voiceBtn.innerHTML = '<i class="fas fa-microphone"></i>';
    // 释放麦克风
    if (recordStream) {
        recordStream.getTracks().forEach(track => track.stop());
        recordStream = null;
    }
    if (!recordChunks.length) {
        showToast('未录制到音频内容', 'error');
        return;
    }
    const blob = new Blob(recordChunks, { type: mediaRecorder.mimeType || 'audio/webm' });
    recordChunks = [];
    convertToWav(blob)
        .then(wavFile => {
            addPendingFiles([wavFile]);
            showToast('录音已添加到待发送附件', 'success');
        })
        .catch(err => {
            console.warn('录音转换失败', err);
            showToast('录音转换失败', 'error');
        });
}

// 将录音 Blob 解码并重采样为 16kHz 单声道 WAV File
async function convertToWav(blob) {
    const arrayBuffer = await blob.arrayBuffer();
    const context = new (window.AudioContext || window.webkitAudioContext)();
    const audioBuffer = await context.decodeAudioData(arrayBuffer);
    context.close();
    // 用离线上下文重采样为 16kHz 单声道
    const frames = Math.ceil(audioBuffer.duration * SAMPLE_RATE);
    const offline = new OfflineAudioContext(1, Math.max(frames, 1), SAMPLE_RATE);
    const source = offline.createBufferSource();
    source.buffer = audioBuffer;
    source.connect(offline.destination);
    source.start(0);
    const rendered = await offline.startRendering();
    const wavBlob = encodeWavBlob(rendered.getChannelData(0), SAMPLE_RATE);
    const stamp = new Date().toTimeString().slice(0, 8).replace(/:/g, '');
    return new File([wavBlob], `录音_${stamp}.wav`, { type: 'audio/wav' });
}

// 将 PCM 浮点采样编码为 16 位 WAV Blob
function encodeWavBlob(samples, sampleRate) {
    const buffer = new ArrayBuffer(44 + samples.length * 2);
    const view = new DataView(buffer);
    // RIFF 头
    writeString(view, 0, 'RIFF');
    view.setUint32(4, 36 + samples.length * 2, true);
    writeString(view, 8, 'WAVE');
    // fmt 块（16位 PCM 单声道）
    writeString(view, 12, 'fmt ');
    view.setUint32(16, 16, true);
    view.setUint16(20, 1, true);
    view.setUint16(22, 1, true);
    view.setUint32(24, sampleRate, true);
    view.setUint32(28, sampleRate * 2, true);
    view.setUint16(32, 2, true);
    view.setUint16(34, 16, true);
    // data 块
    writeString(view, 36, 'data');
    view.setUint32(40, samples.length * 2, true);
    let offset = 44;
    for (let i = 0; i < samples.length; i++, offset += 2) {
        const s = Math.max(-1, Math.min(1, samples[i]));
        view.setInt16(offset, s < 0 ? s * 0x8000 : s * 0x7FFF, true);
    }
    return new Blob([view], { type: 'audio/wav' });
}

function writeString(view, offset, str) {
    for (let i = 0; i < str.length; i++) {
        view.setUint8(offset + i, str.charCodeAt(i));
    }
}
