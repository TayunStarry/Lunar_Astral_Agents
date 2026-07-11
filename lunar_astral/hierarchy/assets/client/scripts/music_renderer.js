/**
 * 音乐渲染器 - iframe 桥接模块
 *
 * 负责管理音乐渲染器 iframe 的生命周期，
 * 通过 BroadcastChannel API 实现与独立音乐播放页面之间的双向通讯。
 *
 * 通讯协议：
 *   发送（主应用 → iframe）：
 *     { type: 'render', abcNotation: string }  加载并渲染乐谱
 *     { type: 'close' }                         关闭播放器
 *     { type: 'theme', darkMode: boolean }      切换主题
 *   接收（iframe → 主应用）：
 *     { type: 'ready' }                         iframe 加载完成
 *     { type: 'closed' }                        播放器已关闭
 *     { type: 'error', message: string }        错误通知
 *     { type: 'state', playing, paused, ... }   播放状态变更
 */

// ==== 广播频道 ====
const channel = new BroadcastChannel('lunar-astral-music');

// ==== iframe 管理 ====
let iframe = null;
let isReady = false;
/** 乐谱渲染请求队列（iframe 未就绪时缓存） */
const pendingQueue = [];

/**
 * 向 iframe 发送消息
 * @param {object} msg - 消息对象
 */
function postToIframe(msg) {
    channel.postMessage(msg);
}

/**
 * 创建音乐渲染器 iframe 并挂载到页面
 */
export function initMusicRenderer() {
    if (document.getElementById('music-renderer-frame')) return;

    iframe = document.createElement('iframe');
    iframe.id = 'music-renderer-frame';
    iframe.src = '../music_renderer.html';
    iframe.allow = 'autoplay';
    iframe.style.cssText = `
        position: fixed;
        inset: 0;
        width: 100%;
        height: 100%;
        border: none;
        z-index: 1000;
        pointer-events: none;
        background: transparent;
        display: none;
    `;
    document.body.appendChild(iframe);

    // 监听 iframe 消息
    channel.onmessage = (event) => {
        const msg = event.data;
        if (!msg || !msg.type) return;

        switch (msg.type) {
            case 'ready':
                isReady = true;
                // 同步当前主题
                syncTheme();
                // 处理缓存的请求
                flushPendingQueue();
                break;
            case 'closed':
                hideIframe();
                break;
            case 'error':
                console.error('[音乐渲染器]', msg.message);
                break;
            case 'state':
                // 播放状态变更时显示 iframe
                if (msg.playing || msg.paused) {
                    showIframe();
                }
                break;
        }
    };
}

/**
 * 加载并渲染 ABC 记谱法乐谱
 * @param {string} abcNotation - ABC 记谱法乐谱文本
 */
export function renderMusicScore(abcNotation) {
    if (!abcNotation || typeof abcNotation !== 'string') {
        console.warn('[音乐渲染器] 无效的乐谱数据');
        return;
    }

    if (!isReady) {
        pendingQueue.push({ type: 'render', abcNotation });
        return;
    }

    showIframe();
    postToIframe({ type: 'render', abcNotation });
}

/**
 * 播放后端渲染的 WAV 音频
 * 当 FluidSynth + SoundFont 渲染完成时，前端收到音频 URL 直接播放
 * @param {string} audioUrl - WAV 音频文件 URL
 * @param {string} fileName - 文件名
 */
export function playRenderedAudio(audioUrl, fileName) {
    if (!audioUrl) {
        console.warn('[音乐渲染器] 无效的音频 URL');
        return;
    }

    if (!isReady) {
        pendingQueue.push({ type: 'play_audio', audioUrl, fileName });
        return;
    }

    showIframe();
    postToIframe({ type: 'play_audio', audioUrl, fileName });
}

/**
 * 关闭音乐播放器
 */
export function closeMusicRenderer() {
    postToIframe({ type: 'close' });
}

/**
 * 同步主题到 iframe
 */
export function syncMusicRendererTheme(darkMode) {
    if (!isReady) {
        pendingQueue.push({ type: 'theme', darkMode });
        return;
    }
    postToIframe({ type: 'theme', darkMode });
}

// ==== 内部方法 ====

function showIframe() {
    if (iframe) {
        iframe.style.display = 'block';
        iframe.style.pointerEvents = 'auto';
    }
}

function hideIframe() {
    if (iframe) {
        iframe.style.display = 'none';
        iframe.style.pointerEvents = 'none';
    }
}

function syncTheme() {
    const darkMode = document.body.classList.contains('dark-mode');
    postToIframe({ type: 'theme', darkMode });
}

function flushPendingQueue() {
    while (pendingQueue.length > 0) {
        const msg = pendingQueue.shift();
        postToIframe(msg);
    }
}

/**
 * 销毁音乐渲染器
 */
export function destroyMusicRenderer() {
    if (iframe) {
        iframe.remove();
        iframe = null;
    }
    isReady = false;
    channel.onmessage = null;
    pendingQueue.length = 0;
}
