// ============================================================
//  星月智能 · 消息终端 — 事件委托 / 清理 / 初始化入口
// ============================================================

// ---------- 事件委托 ----------
function setupMessageAreaDelegation() {
    messageArea.addEventListener('click', (e) => {
        const summary = e.target.closest('.think-summary');
        if (summary) {
            summary.parentElement.classList.toggle('open');
        }
    });
}

// ---------- 清理 ----------
function cleanup() {
    manualClose = true;
    if (reconnectTimer) clearTimeout(reconnectTimer);
    AudioQueue.stop();
    if (ws) {
        ws.onclose = null;
        ws.close();
        ws = null;
    }
    document.querySelectorAll('.echarts-container').forEach(c => {
        if (c._echartsInstance) c._echartsInstance.dispose();
    });
    if (musicIframe) {
        musicIframe.remove();
        musicIframe = null;
    }
    musicChannel.close();
}

// ---------- 初始化 ----------
/** 渲染引擎 iframe：指向智能体所在后端（36789），与主 WebSocket 连接保持一致 */
function setupRendererFrame() {
    const frame = document.getElementById('rendererFrame');
    if (!frame || !frame.dataset.src) return;
    frame.src = `${frame.dataset.src}&ws=${window.location.hostname}:36789`;
}

async function init() {
    loadTheme();
    setupThemeToggle();
    loadVoiceAutoPlay();
    setupVoiceToggle();
    setupTabEvents();
    setupSearchEvents();
    setupDragEvents();
    setupInputEvents();
    setupMessageAreaDelegation();
    setupDrawboard();
    setupCapture();
    setupScrollControls();
    setupProximityReveal();
    initMusicRenderer();
    setupRendererFrame();
    await ensureMarked();
    initMermaid();
    await loadPersistedMessages();
    updateEmptyState();
    connectWebSocket();
    window.addEventListener('beforeunload', cleanup);
}

if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init);
} else {
    init();
}
