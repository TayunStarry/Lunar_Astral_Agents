// ============================================================
//  星月智能 · 消息终端 — 通用工具（ID / 时间 / HTML 转义 / 提示 / 滚动 / 剪贴板）
// ============================================================

// ---------- 通用工具 ----------
function generateId() {
    return `msg-${Date.now()}-${Math.random().toString(36).slice(2, 10)}`;
}

function getTimeString() {
    return new Date().toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit', second: '2-digit' });
}

function escapeHtml(str) {
    const div = document.createElement('div');
    div.textContent = str == null ? '' : String(str);
    return div.innerHTML;
}

function showToast(msg, type = 'info') {
    const toast = document.createElement('div');
    toast.className = 'toast';
    const icons = { info: 'fa-circle-info', success: 'fa-check-circle', error: 'fa-exclamation-triangle' };
    toast.innerHTML = `<i class="fas ${icons[type] || icons.info}" style="margin-right:8px;"></i>${escapeHtml(msg)}`;
    toastContainer.appendChild(toast);
    setTimeout(() => toast.remove(), 2800);
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
    try {
        await navigator.clipboard.writeText(text || '');
        return true;
    } catch {
        return false;
    }
}

function encodeFilePath(path) {
    // 路径为 ASCII，直接 base64 编码
    return btoa(unescape(encodeURIComponent(path)));
}
