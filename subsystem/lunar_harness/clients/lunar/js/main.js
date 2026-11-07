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
    stopVoiceListening(false);
    AudioQueue.stop();
    disconnectWebSocket();
    document.querySelectorAll('.echarts-container').forEach(c => {
        if (c._echartsInstance) c._echartsInstance.dispose();
    });
}

// ---------- 初始化 ----------
/** 渲染引擎 iframe：按配置选择月岩引擎（lunar.rendering_engine）、Live2D 引擎层
 *  （lunar.live2d_engine）或 GIF 动图引擎（lunar.gif_engine）；缺省回退月岩。
 *  WsBridge 未传 ws 参数时默认连接同源 /ws，代理场景下自动走代理隧道 */
function setupRendererFrame() {
    window.LunarShared.renderer.applyRendererFrame(document.getElementById('rendererFrame'), characterRenderer);
}

async function init() {
    // 共享层缺失时由守卫渲染可见错误层并中止初始化，避免半死状态
    if (!(await window.waitForLunarShared())) return;
    loadTheme();
    setupThemeToggle();
    loadVoiceAutoPlay();
    await initUserName();
    await loadCharacterRenderer();
    setupVoiceToggle();
    setupTabEvents();
    setupSearchEvents();
    setupDragEvents();
    setupInputEvents();
    setupVoiceInput();
    setupQuoteEvents();
    setupMessageAreaDelegation();
    setupDrawboard();
    setupCapture();
    setupScrollControls();
    setupCornerToggle();
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
