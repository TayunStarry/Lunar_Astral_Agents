// ============================================================
//  星月智能 · 消息终端 — 连续语音对话（AEC + VAD 自动录音发送）
// ============================================================

// ---------- 连续语音参数 ----------
const VOICE_TARGET_RATE = 16000;       // 目标采样率（音频理解标准输入）
const VOICE_FRAME_SIZE = 1024;         // VAD 分析帧长
const VOICE_START_FRAMES = 3;          // 连续超阈值帧数 → 判定说话开始
const VOICE_END_SILENCE_MS = 800;      // 静音持续时长 → 判定说话结束
const VOICE_TAIL_KEEP_MS = 200;        // 结束判定后保留的尾部静音
const VOICE_MIN_SPEECH_MS = 250;       // 有效人声最短时长（滤除瞬时噪声）
const VOICE_MAX_SPEECH_MS = 30000;     // 单次语音最长时长（超时强制结束）
const VOICE_PREROLL_FRAMES = 5;        // 说话起点前保留的缓冲帧，避免吃掉开头
const VOICE_ABS_THRESHOLD = 0.012;     // 绝对能量阈值下限
const VOICE_NOISE_FACTOR = 2.2;        // 相对本底噪声的倍数阈值
const VOICE_AI_RESUME_DELAY_MS = 350;  // AI 播放结束后恢复监听的延迟
const VOICE_REPLY_COOLDOWN_MS = 30000; // 未收到回应时，两次语音发送的最小间隔

// ---------- 连续语音状态 ----------
let voiceListening = false;    // 是否处于持续监听
let voiceCapturing = false;    // 是否正在采集用户语音
let voiceSending = false;      // 是否正在发送语音
let voiceStream = null;        // 麦克风媒体流
let voiceContext = null;       // 采集用 AudioContext
let voiceSource = null;        // 媒体流源节点
let voiceProcessor = null;     // PCM 采集处理节点
let voiceMuteGain = null;      // 静音增益节点（保证处理节点运行且不外放）
let voicePreroll = [];         // 说话起点前的缓冲帧
let voiceFrames = [];          // 当前语音帧
let voiceAboveCount = 0;       // 连续超阈值帧计数
let voiceSilenceMs = 0;        // 已累计静音时长
let voiceSpeechMs = 0;         // 已累计语音时长
let voiceVoicedMs = 0;         // 已累计人声时长（用于过滤瞬时噪声）
let voiceNoiseFloor = 0.004;   // 自适应本底噪声
let voiceResumeAt = 0;         // AI 播放结束后可恢复监听的时间戳
let voiceLastSentAt = 0;       // 上次语音发送的时间戳
let voiceAwaitingReply = false; // 是否仍在等待月华对上次语音的回应

// ---------- 语音输入入口 ----------
function setupVoiceInput() {
    if (!voiceBtn) return;
    voiceBtn.addEventListener('click', () => {
        if (voiceListening) stopVoiceListening(true);
        else startVoiceListening();
    });
}

// 开启连续语音：申请带 AEC 的麦克风并常驻，之后自动检测用户语音
async function startVoiceListening() {
    if (voiceListening) return;
    if (!navigator.mediaDevices || !window.AudioContext) {
        showToast('当前环境不支持连续语音', 'error');
        return;
    }
    try {
        // 声学回声消除 + 噪声抑制 + 自动增益：从采集源头抑制 AI 外放回灌
        voiceStream = await navigator.mediaDevices.getUserMedia({
            audio: { echoCancellation: true, noiseSuppression: true, autoGainControl: true }
        });
    } catch (err) {
        showToast('麦克风权限被拒绝，无法开启连续语音', 'error');
        return;
    }
    try {
        voiceContext = new AudioContext({ sampleRate: VOICE_TARGET_RATE });
    } catch (err) {
        // 部分环境不支持指定采样率，退回默认采样率（WAV 按实际采样率编码）
        voiceContext = new AudioContext();
    }
    if (voiceContext.state === 'suspended') await voiceContext.resume();

    voiceSource = voiceContext.createMediaStreamSource(voiceStream);
    voiceProcessor = voiceContext.createScriptProcessor(VOICE_FRAME_SIZE, 1, 1);
    voiceMuteGain = voiceContext.createGain();
    voiceMuteGain.gain.value = 0;              // 采集链路静音，避免自激啸叫
    voiceProcessor.onaudioprocess = (e) => handleVoiceFrame(e.inputBuffer.getChannelData(0));
    voiceSource.connect(voiceProcessor);
    voiceProcessor.connect(voiceMuteGain);
    voiceMuteGain.connect(voiceContext.destination);

    // 设备被拔出 / 权限被回收时自动退出监听
    voiceStream.getAudioTracks().forEach(track => {
        track.addEventListener('ended', () => stopVoiceListening(false));
    });

    voiceListening = true;
    voiceNoiseFloor = 0.004;
    voiceResumeAt = 0;
    voicePreroll = [];
    resetVoiceUtterance();
    updateVoiceUI('listening');
    showToast('连续语音已开启，直接说话即可', 'success');
}

// 关闭连续语音：释放采集链路与麦克风
function stopVoiceListening(notify) {
    if (!voiceListening && !voiceStream) return;
    voiceListening = false;
    resetVoiceUtterance();
    voicePreroll = [];
    if (voiceProcessor) {
        voiceProcessor.onaudioprocess = null;
        voiceProcessor.disconnect();
        voiceProcessor = null;
    }
    if (voiceSource) {
        voiceSource.disconnect();
        voiceSource = null;
    }
    if (voiceMuteGain) {
        voiceMuteGain.disconnect();
        voiceMuteGain = null;
    }
    if (voiceStream) {
        voiceStream.getTracks().forEach(track => track.stop());
        voiceStream = null;
    }
    if (voiceContext) {
        voiceContext.close();
        voiceContext = null;
    }
    updateVoiceUI('idle');
    if (notify) showToast('连续语音已关闭', 'info');
}

// 重置当前一轮语音采集状态
function resetVoiceUtterance() {
    voiceCapturing = false;
    voiceFrames = [];
    voiceAboveCount = 0;
    voiceSilenceMs = 0;
    voiceSpeechMs = 0;
    voiceVoicedMs = 0;
}

// 单帧时长（毫秒），采样率取实际采集上下文
function voiceFrameMs() {
    return (VOICE_FRAME_SIZE / (voiceContext ? voiceContext.sampleRate : VOICE_TARGET_RATE)) * 1000;
}

// 按钮状态：idle 待机 / listening 持续监听 / capturing 正在采集用户语音
function updateVoiceUI(state) {
    if (!voiceBtn) return;
    voiceBtn.classList.toggle('listening', state === 'listening');
    voiceBtn.classList.toggle('recording', state === 'capturing');
    if (state === 'capturing') {
        voiceBtn.title = '正在聆听你的声音…';
        voiceBtn.innerHTML = '<i class="fas fa-microphone-lines"></i>';
    } else if (state === 'listening') {
        voiceBtn.title = '连续语音对话中（点击关闭）';
        voiceBtn.innerHTML = '<i class="fas fa-headset"></i>';
    } else {
        voiceBtn.title = '开启连续语音对话';
        voiceBtn.innerHTML = '<i class="fas fa-microphone"></i>';
    }
}

// ---------- 语音活动检测（VAD） ----------
// AI 播放识别：AI 正在说话时一律不采集，避免把自身外放的声音当成用户输入
function isAssistantSpeaking() {
    return !!(AudioQueue && (AudioQueue.playing || AudioQueue.currentSource));
}

// 逐帧判定：说话开始 → 采集 → 说话结束
function handleVoiceFrame(samples) {
    if (!voiceListening) return;
    const frameMs = voiceFrameMs();
    const rms = frameRms(samples);

    if (isAssistantSpeaking()) {
        // AI 说话期间：丢弃进行中的采集，并在播放结束后留出恢复延迟
        if (voiceCapturing) {
            resetVoiceUtterance();
            updateVoiceUI('listening');
        }
        voiceResumeAt = Date.now() + VOICE_AI_RESUME_DELAY_MS;
        return;
    }
    if (Date.now() < voiceResumeAt) return;

    const threshold = Math.max(VOICE_ABS_THRESHOLD, voiceNoiseFloor * VOICE_NOISE_FACTOR);
    const voiced = rms > threshold;

    if (!voiceCapturing) {
        // 未进入语音：维护起点前缓冲 + 自适应本底噪声
        if (!voiced) voiceNoiseFloor = voiceNoiseFloor * 0.98 + rms * 0.02;
        voiceAboveCount = voiced ? voiceAboveCount + 1 : 0;
        voicePreroll.push(new Float32Array(samples));
        if (voicePreroll.length > VOICE_PREROLL_FRAMES) voicePreroll.shift();
        if (voiceAboveCount >= VOICE_START_FRAMES) beginVoiceUtterance();
        return;
    }

    // 采集中：累计帧与静音 / 人声时长，静音足够久或超时即结束
    voiceFrames.push(new Float32Array(samples));
    voiceSpeechMs += frameMs;
    voiceSilenceMs = voiced ? 0 : voiceSilenceMs + frameMs;
    if (voiced) voiceVoicedMs += frameMs;
    if (voiceSilenceMs >= VOICE_END_SILENCE_MS || voiceSpeechMs >= VOICE_MAX_SPEECH_MS) {
        finishVoiceUtterance(frameMs);
    }
}

// 帧能量（RMS）
function frameRms(samples) {
    let sum = 0;
    for (let i = 0; i < samples.length; i++) sum += samples[i] * samples[i];
    return Math.sqrt(sum / samples.length);
}

// 说话开始：把起点前缓冲一并纳入，避免吃掉开头音节
function beginVoiceUtterance() {
    voiceCapturing = true;
    voiceFrames = voicePreroll.slice();
    voicePreroll = [];
    voiceAboveCount = 0;
    voiceSilenceMs = 0;
    voiceSpeechMs = 0;
    voiceVoicedMs = VOICE_START_FRAMES * voiceFrameMs();
    updateVoiceUI('capturing');
}

// 说话结束：裁剪多余尾部静音后自动发送
function finishVoiceUtterance(frameMs) {
    const sampleRate = voiceContext ? voiceContext.sampleRate : VOICE_TARGET_RATE;
    voiceCapturing = false;
    const dropFrames = Math.max(0, Math.floor((voiceSilenceMs - VOICE_TAIL_KEEP_MS) / frameMs));
    if (dropFrames > 0) voiceFrames.splice(Math.max(voiceFrames.length - dropFrames, 0), dropFrames);
    const frames = voiceFrames;
    const voicedMs = voiceVoicedMs;
    resetVoiceUtterance();
    voicePreroll = [];
    updateVoiceUI(voiceListening ? 'listening' : 'idle');
    if (voicedMs < VOICE_MIN_SPEECH_MS) return;   // 过短视为环境噪声，直接丢弃
    sendVoiceUtterance(frames, sampleRate);
}

// ---------- 自动发送 ----------
// 月华回应感知：WebSocket 收到助手消息时调用，解除语音发送限制
function notifyVoiceAssistantReply() {
    voiceAwaitingReply = false;
}

// 发送资格判定：已收到月华回应，或距上次发送已达冷却时长，方可再次发送
function canSendVoiceNow() {
    return !voiceAwaitingReply || (Date.now() - voiceLastSentAt) >= VOICE_REPLY_COOLDOWN_MS;
}

// PCM → WAV → 直接推送给 AI，无需用户点击发送
async function sendVoiceUtterance(frames, sampleRate) {
    if (voiceSending) return;
    // 节流：月华尚未回应且距上次发送不足 30 秒时，丢弃本次语音并提示
    if (!canSendVoiceNow()) {
        showToast('月华还没回应上一条语音，稍后再说吧', 'info');
        return;
    }
    voiceSending = true;
    try {
        const wavBlob = encodeWavBlob(flattenFrames(frames), sampleRate);
        const stamp = new Date().toTimeString().slice(0, 8).replace(/:/g, '');
        const file = new File([wavBlob], `语音_${stamp}.wav`, { type: 'audio/wav' });
        const base64Data = await fileToRawBase64(file);

        // 本地历史展示（独立 blob URL，避免被清理撤销）
        addMessage({
            id: generateId(),
            role: 'user',
            categories: ['voice'],
            content: '',
            attachments: [{ type: 'audio', src: URL.createObjectURL(file), label: file.name }],
            timestamp: Date.now()
        });
        schedulePersist();

        if (!backendConnected) {
            showToast('离线模式：语音仅本地渲染', 'info');
            return;
        }
        // QQ 适配器标准格式：[用户: xxx]: [语音] 前缀 + 音频内容块（统一数组保持时序）
        await sendMessages([{
            role: 'user',
            content: [
                { type: 'text', text: buildUserNamePrefix() + '[语音] ' },
                { type: 'input_audio', input_audio: { data: base64Data, format: 'wav' } }
            ]
        }]);
        // 发送成功后进入等待回应状态，直至收到月华回应或冷却期满
        voiceLastSentAt = Date.now();
        voiceAwaitingReply = true;
    } catch (err) {
        showToast('语音发送失败：' + (err.message || err), 'error');
    } finally {
        voiceSending = false;
    }
}

// 合并多帧 PCM 为连续采样
function flattenFrames(frames) {
    const total = frames.reduce((n, f) => n + f.length, 0);
    const samples = new Float32Array(total);
    let offset = 0;
    for (const frame of frames) {
        samples.set(frame, offset);
        offset += frame.length;
    }
    return samples;
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
