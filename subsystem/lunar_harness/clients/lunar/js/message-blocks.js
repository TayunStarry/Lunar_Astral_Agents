// ============================================================
//  星月智能 · 消息终端 — 消息媒体块构建（图片 / 视频 / 音频 / 音乐）
// ============================================================

function buildImageBlock(msg) {
    const grid = document.createElement('div');
    grid.className = 'image-grid';
    const container = document.createElement('div');
    container.className = 'labeled-image-container';
    container.style.setProperty('--image-label', `'${msg.imageLabel || '图片'}'`);
    const img = document.createElement('img');
    img.src = msg.imageSrc;
    img.alt = msg.imageLabel || '图片';
    img.loading = 'lazy';
    img.addEventListener('click', () => {
        if (typeof window.previewImage === 'function') window.previewImage(msg.imageSrc, msg.imageLabel || '图片');
    });
    container.appendChild(img);
    grid.appendChild(container);
    return grid;
}

function buildVideoBlock(msg) {
    const container = document.createElement('div');
    container.className = 'video-container';
    const video = document.createElement('video');
    video.src = msg.videoSrc;
    video.controls = true;
    video.playsInline = true;
    container.appendChild(video);
    return container;
}

function buildAudioFileBlock(msg) {
    const audio = document.createElement('audio');
    audio.className = 'message-audio-player';
    audio.controls = true;
    audio.src = msg.audioSrc;
    return audio;
}

function buildAttachmentBlock(att) {
    if (!att) return null;
    if (att.type === 'image') {
        const grid = document.createElement('div');
        grid.className = 'image-grid';
        const container = document.createElement('div');
        container.className = 'labeled-image-container';
        container.style.setProperty('--image-label', `'${att.label || '图片'}'`);
        const img = document.createElement('img');
        img.src = att.src;
        img.alt = att.label || '图片';
        img.loading = 'lazy';
        img.addEventListener('click', () => {
            if (typeof window.previewImage === 'function') window.previewImage(att.src, att.label || '图片');
        });
        container.appendChild(img);
        grid.appendChild(container);
        return grid;
    }
    if (att.type === 'video') {
        const container = document.createElement('div');
        container.className = 'video-container';
        const video = document.createElement('video');
        video.src = att.src;
        video.controls = true;
        video.playsInline = true;
        container.appendChild(video);
        return container;
    }
    if (att.type === 'audio') {
        // 音频不再持久化：历史消息只有悬空句柄 + 转写文本，不渲染播放器
        if (!att.src) return buildVoiceTranscriptBlock(att);
        const audio = document.createElement('audio');
        audio.className = 'message-audio-player';
        audio.controls = true;
        audio.src = att.src;
        return audio;
    }
    return null;
}

/** 语音转写块：展示句柄与转写文本，替代已取消的音频落盘播放 */
function buildVoiceTranscriptBlock(att) {
    const wrap = document.createElement('div');
    wrap.className = 'voice-transcript';
    const head = document.createElement('div');
    head.className = 'voice-transcript-head';
    head.innerHTML = `<i class="fas fa-microphone"></i> ${escapeHtml(att.label || '语音')}`;
    if (att.handle) head.title = att.handle;
    const text = document.createElement('div');
    text.className = 'voice-transcript-text';
    text.textContent = att.text || '（这段语音没有可用的转写文本）';
    wrap.appendChild(head);
    wrap.appendChild(text);
    return wrap;
}

/** 音乐卡片：后端推送的音频 + 共享播放器（播放/暂停、波形、进度、时长） */
function buildMusicCard(msg) {
    const audioUrl = String(msg.musicAudio || msg.audio || '');
    const card = LunarMusicPlayer.createCard({
        title: msg.musicTitle || '月华演奏曲',
        summary: msg.musicSummary || '',
        duration: msg.musicDuration,
        audio: audioUrl,
        // 桌面端点击卡片本体即可在完整卡片与迷你条之间切换
        toggleOnCardClick: true,
        // 桌面端折叠为迷你条，保留播放器实例与进度，不打断正在播放的音频
        pauseOnClose: false,
        onClose: (target) => {
            LunarMusicPlayer.dismiss(audioUrl);
            target.setCollapsed(true);
        },
        onExpand: () => {
            LunarMusicPlayer.undismiss(audioUrl);
        }
    });
    // 历史消息重渲染时保持上次的收起状态，避免「关了又出现」
    if (LunarMusicPlayer.isDismissed(audioUrl)) card.setCollapsed(true);
    return card;
}

function buildAudioReplay(msg) {
    const btn = document.createElement('button');
    btn.className = 'audio-replay-btn';
    btn.title = '重播语音';
    btn.innerHTML = '<i class="fas fa-volume-up"></i> 重播语音';
    btn.addEventListener('click', () => {
        AudioQueue.enqueue(msg.audio);
        btn.classList.add('replaying');
        setTimeout(() => btn.classList.remove('replaying'), 600);
    });
    return btn;
}
