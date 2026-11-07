// ============================================================
//  星月智能 · 消息终端 — 通用工具（ID / 时间 / HTML 转义 / 提示 / 滚动 / 剪贴板）
// ============================================================

// ---------- 通用工具 ----------
function generateId() {
    return window.LunarShared.utils.generateId();
}

function getTimeString() {
    return window.LunarShared.utils.formatTime(undefined, true);
}

function escapeHtml(str) {
    return window.LunarShared.utils.escapeHtml(str);
}

// ---------- Toast（共享队列 / 生命周期，桌面端沿用胶囊视觉） ----------
const desktopToast = window.LunarShared ? new window.LunarShared.ToastController({
    baseClass: 'toast',
    // 桌面端一直只有单一胶囊样式，不加类型类名
    typeClassPrefix: '',
    types: {
        info: { icon: 'fa-circle-info', color: '', autoDismiss: 2800 },
        success: { icon: 'fa-check-circle', color: '', autoDismiss: 2800 },
        error: { icon: 'fa-exclamation-triangle', color: '', autoDismiss: 2800 }
    },
    showClose: false,
    iconStyle: 'margin-right:8px;',
    // 进出场由 feedback.css 的 .toast 动画负责
    enterClass: '',
    leaveClass: '',
    // 桌面端由 CSS 动画负责淡出，到点直接移除（保持原有 2800ms 生命周期）
    removeDelay: 0,
    colorVariable: null
}) : null;

function showToast(msg, type = 'info') {
    if (!desktopToast) return;
    desktopToast.show(msg, type);
}

function updateEmptyState() {
    emptyState.classList.toggle('hidden', messages.length > 0);
}

function scrollToBottom(smooth = true) {
    messageArea.scrollTo({ top: messageArea.scrollHeight, behavior: smooth ? 'smooth' : 'auto' });
}

function scrollToTop(smooth = true) {
    messageArea.scrollTo({ top: 0, behavior: smooth ? 'smooth' : 'auto' });
}

// ---------- 剪贴板 / 文件路径编码 ----------
async function copyToClipboard(text) {
    return window.LunarShared.utils.copyToClipboard(text);
}

function encodeFilePath(path) {
    return window.LunarShared.utils.encodeFilePath(path);
}
