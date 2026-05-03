/* ============================================================
   ui.js - Toast 提示与状态管理
   ============================================================ */

export let toastContainer;
export let connectionStatus;

export function initUI(container, status) {
    toastContainer = container;
    connectionStatus = status;
}

// ==================== Toast 提示 ====================
export function showToast(message, type = 'info', duration = 3000) {
    const toast = document.createElement('div');
    toast.className = `toast ${type}`;
    toast.textContent = message;
    toastContainer.appendChild(toast);
    setTimeout(() => toast.remove(), duration + 400);
}

// ==================== 状态指示 ====================
export function setStatus(status) {
    connectionStatus.className = 'status-badge';
    if (status === 'sending') {
        connectionStatus.classList.add('sending');
        connectionStatus.innerHTML = '<i class="fas fa-circle"></i> 发送中...';
    } else if (status === 'error') {
        connectionStatus.classList.add('error');
        connectionStatus.innerHTML = '<i class="fas fa-circle"></i> 连接失败';
    } else {
        connectionStatus.innerHTML = '<i class="fas fa-circle"></i> 就绪';
    }
}
