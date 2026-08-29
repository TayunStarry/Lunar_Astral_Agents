// ============================================================
//  星月智能 · 消息终端 — 消息媒体块构建（图片 / 视频 / 音频 / 乐谱）
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
        const audio = document.createElement('audio');
        audio.className = 'message-audio-player';
        audio.controls = true;
        audio.src = att.src;
        return audio;
    }
    return null;
}

function buildMusicCard(msg) {
    const card = document.createElement('div');
    card.className = 'music-card';
    const header = document.createElement('div');
    header.className = 'music-card-header';
    const title = document.createElement('div');
    title.className = 'music-card-title';
    title.innerHTML = '<i class="fas fa-music"></i> 乐谱';
    const playBtn = document.createElement('button');
    playBtn.className = 'music-play-btn';
    playBtn.innerHTML = '<i class="fas fa-play"></i> 播放';
    playBtn.addEventListener('click', () => renderMusicScore(msg.abcNotation));
    header.appendChild(title);
    header.appendChild(playBtn);
    const preview = document.createElement('div');
    preview.className = 'music-abc-preview';
    preview.textContent = msg.abcNotation || '';
    card.appendChild(header);
    card.appendChild(preview);
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
