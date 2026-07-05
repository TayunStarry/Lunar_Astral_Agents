// ==== 基座层应用（函数式架构） ====
// 职责：iframe 集成、操纵层互斥切换、BroadcastChannel 协调

// ==== 常量 ====
const CHANNEL_NAME = 'integrated-studio-bus';
const PANELS = ['physics', 'animation', 'movement', 'elements', 'assets'];
const SOURCE_BASE = 'base';

// ==== DOM 引用聚合 ====
const elements = {
    tabs: document.querySelectorAll('.tab-btn'),
    panelIframes: document.querySelectorAll('.area-panel iframe'),
    panelLoading: document.querySelector('.panel-loading'),
    btnMode: document.getElementById('btn-mode'),
    btnTheme: document.getElementById('btn-theme'),
    btnHelp: document.getElementById('btn-help'),
    helpModal: document.getElementById('help-modal'),
    helpClose: document.querySelector('.modal-close'),
    helpOverlay: document.getElementById('help-modal'),
    toast: document.getElementById('toast'),
};

// ==== 全局状态 ====
const state = {
    activePanel: 'animation',          // 当前激活的操纵层（Q6.2 默认动画）
    currentMode: 'editor',             // 当前引擎视图模式（Q5）
    readyPanels: new Set(),            // 已就绪的 panel iframe（Q6.5 握手）
    channel: new BroadcastChannel(CHANNEL_NAME),
    toastTimer: null,
};

// ==== 初始化 ====
function init() {
    bindEvents();
    // 初始激活动画面板（已在 HTML 中通过 class="active" + hidden 属性预设）
    notifyEngineActivePanel();
}

function bindEvents() {
    // Tab 切换
    elements.tabs.forEach(btn => {
        btn.addEventListener('click', () => switchPanel(btn.dataset.panel));
    });

    // 工具按钮
    elements.btnMode?.addEventListener('click', toggleMode);
    elements.btnTheme?.addEventListener('click', toggleTheme);
    elements.btnHelp?.addEventListener('click', showHelp);

    // 模态框关闭
    elements.helpClose?.addEventListener('click', hideHelp);
    elements.helpOverlay?.addEventListener('click', (e) => {
        if (e.target === elements.helpOverlay) hideHelp();
    });

    // ESC 关闭模态框
    document.addEventListener('keydown', (e) => {
        if (e.key === 'Escape' && !elements.helpOverlay.hasAttribute('hidden')) {
            hideHelp();
        }
    });

    // BroadcastChannel 监听
    state.channel.onmessage = handleChannelMessage;
}

// ==== 操纵层互斥切换（Q6.4） ====
function switchPanel(name) {
    if (!PANELS.includes(name) || name === state.activePanel) return;
    state.activePanel = name;

    // 1. 更新 tab 按钮状态
    elements.tabs.forEach(btn => {
        btn.classList.toggle('active', btn.dataset.panel === name);
    });

    // 2. 切换 iframe 可见性
    elements.panelIframes.forEach(f => {
        const isActive = f.dataset.panel === name;
        f.classList.toggle('active', isActive);
        if (isActive) f.removeAttribute('hidden');
        else f.setAttribute('hidden', '');
    });

    // 3. 若面板未就绪，显示加载占位
    if (!state.readyPanels.has(name)) {
        showLoading(true);
        const checkReady = setInterval(() => {
            if (state.readyPanels.has(name)) {
                clearInterval(checkReady);
                showLoading(false);
            }
        }, 100);
    } else {
        showLoading(false);
    }

    // 4. 通知引擎当前活跃面板（仅用于日志/调试，不影响引擎行为）
    notifyEngineActivePanel();
}

function showLoading(show) {
    if (!elements.panelLoading) return;
    if (show) elements.panelLoading.removeAttribute('hidden');
    else elements.panelLoading.setAttribute('hidden', '');
}

function notifyEngineActivePanel() {
    state.channel.postMessage({
        type: 'active_panel_changed',
        source: SOURCE_BASE,
        payload: { panel: state.activePanel },
        timestamp: Date.now(),
    });
}

// ==== 模式切换（Q5.3） ====
function toggleMode() {
    state.currentMode = state.currentMode === 'editor' ? 'embedded' : 'editor';
    updateModeButton();
    state.channel.postMessage({
        type: 'mode_changed',
        source: SOURCE_BASE,
        payload: { mode: state.currentMode },
        timestamp: Date.now(),
    });
    showToast(`已切换至${state.currentMode === 'editor' ? '编辑器' : '嵌入式'}视图`);
}

function updateModeButton() {
    if (!elements.btnMode) return;
    const isEditor = state.currentMode === 'editor';
    const icon = elements.btnMode.querySelector('i');
    const text = elements.btnMode.querySelector('span');
    if (icon) icon.className = isEditor ? 'fas fa-eye' : 'fas fa-eye-slash';
    if (text) text.textContent = isEditor ? '编辑器视图' : '嵌入式视图';
}

// ==== 主题切换 ====
function toggleTheme() {
    document.body.classList.toggle('dark-mode');
    const isDark = document.body.classList.contains('dark-mode');
    const icon = elements.btnTheme?.querySelector('i');
    if (icon) icon.className = isDark ? 'fas fa-sun' : 'fas fa-moon';
    // 广播主题变更给引擎和所有面板
    state.channel.postMessage({
        type: 'theme_changed',
        source: SOURCE_BASE,
        payload: { dark: isDark },
        timestamp: Date.now(),
    });
    showToast(isDark ? '已切换至暗色主题' : '已切换至亮色主题');
}

// ==== 帮助模态框 ====
function showHelp() {
    elements.helpOverlay?.removeAttribute('hidden');
}

function hideHelp() {
    elements.helpOverlay?.setAttribute('hidden', '');
}

// ==== Toast 工具 ====
function showToast(message) {
    if (!elements.toast) return;
    elements.toast.textContent = message;
    elements.toast.classList.add('visible');
    clearTimeout(state.toastTimer);
    state.toastTimer = setTimeout(() => {
        elements.toast.classList.remove('visible');
    }, 3000);
}

// ==== BroadcastChannel 消息处理 ====
function handleChannelMessage(event) {
    const msg = event.data;
    if (!msg || msg.source === SOURCE_BASE) return;  // 忽略自己发的

    switch (msg.type) {
        case 'panel_ready':
            // panel iframe 加载完成握手（Q6.5）
            state.readyPanels.add(msg.source.replace('-panel', ''));
            // 若当前激活的面板正好就绪，隐藏 loading
            if (state.readyPanels.has(state.activePanel)) {
                showLoading(false);
            }
            break;

        case 'body_click':
            // 引擎报告点击模型部位（Q3.4 body_click 消息）
            handleBodyClick(msg.payload);
            break;

        case 'telemetry':
        case 'molang_value':
        case 'mode_changed':
        case 'active_panel_changed':
            // 这些消息由其他 iframe 直接监听处理，基座层无需介入
            break;

        default:
            console.debug('[base] 未识别的消息类型:', msg.type);
    }
}

function handleBodyClick(payload) {
    if (payload?.partName) {
        showToast(`点击部位：${payload.partName}`);
    }
}

// ==== 启动 ====
document.addEventListener('DOMContentLoaded', init);
