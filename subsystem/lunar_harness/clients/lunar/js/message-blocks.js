// ============================================================
//  星月智能 · 消息终端 — 消息媒体块构建（共享层适配）
//  纯 DOM 结构在 /shared/core/message-view.js，
//  这里只注入桌面端特有的预览 / 播放 / 音乐卡行为。
// ============================================================

function buildImageBlock(msg) {
    return window.LunarShared.messageView.buildImageBlock({
        src: msg.imageSrc,
        label: msg.imageLabel || '图片',
        onPreview: (src, label) => {
            if (typeof window.previewImage === 'function') window.previewImage(src, label);
        }
    });
}

function buildVideoBlock(msg) {
    return window.LunarShared.messageView.buildVideoBlock({ src: msg.videoSrc });
}

function buildAudioFileBlock(msg) {
    return window.LunarShared.messageView.buildAudioBlock({ src: msg.audioSrc });
}

function buildAttachmentBlock(att) {
    return window.LunarShared.messageView.buildAttachmentBlock(att, {
        onPreview: (src, label) => {
            if (typeof window.previewImage === 'function') window.previewImage(src, label);
        }
    });
}

/** 语音转写块：展示句柄与转写文本，替代已取消的音频落盘播放 */
function buildVoiceTranscriptBlock(att) {
    return window.LunarShared.messageView.buildVoiceTranscriptBlock(att);
}

/** 音乐卡片：后端推送的音频 + 共享播放器（播放/暂停、波形、进度、时长） */
function buildMusicCard(msg) {
    const audioUrl = String(msg.musicAudio || msg.audio || '');
    return window.LunarShared.messageView.buildMusicCard(msg, {
        createOptions: {
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
        },
        // 历史消息重渲染时保持上次的收起状态，避免「关了又出现」
        collapseWhenDismissed: true
    });
}

/** 语音重播胶囊：播放图标 + 装饰波形 + 文案 + 真实时长，点击逻辑沿用 AudioQueue */
function buildAudioReplay(msg) {
    return window.LunarShared.messageView.buildAudioReplay({
        audio: msg.audio,
        onReplay: (audio) => AudioQueue.enqueue(audio)
    });
}
