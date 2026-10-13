// ============================================================
//  星月智能 · 消息终端 — 通用工具（ID / 时间 / HTML 转义 / 提示 / 滚动 / 剪贴板）
// ============================================================

// ---------- 通用工具 ----------
function generateId() {
    // UUID v4 风格 ID：msg- 前缀 + 标准 v4 UUID（随机数取自 crypto.getRandomValues）
    const bytes = new Uint8Array(16);
    crypto.getRandomValues(bytes);
    bytes[6] = (bytes[6] & 0x0f) | 0x40; // 版本位 → v4
    bytes[8] = (bytes[8] & 0x3f) | 0x80; // 变体位 → RFC 4122
    const hex = Array.from(bytes, b => b.toString(16).padStart(2, '0')).join('');
    const uuid = `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`;
    return `msg-${uuid}`;
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
