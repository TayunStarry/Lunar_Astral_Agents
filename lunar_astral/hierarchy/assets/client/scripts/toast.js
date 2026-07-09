/**
 * 通用悬浮消息组件 (Toast)
 *
 * 采用固定定位的悬浮框显示各类状态消息，不干扰页面原有布局。
 * 支持五种消息类型，具备进出场动画、自动消失和手动关闭功能。
 *
 * 消息类型：
 * - info     : 蓝色信息提示，3秒后自动消失
 * - success  : 绿色成功提示，3秒后自动消失
 * - warning  : 黄色警告提示，5秒后自动消失
 * - error    : 红色错误提示，5秒后自动消失
 * - voice    : 语音识别常驻状态，手动关闭或调用 dismissVoice()
 */

// ==== 消息类型配置 ====
const TYPE_CONFIG = {
    info:    { icon: 'fa-info-circle',    color: 'var(--primary-color)', autoDismiss: 3000 },
    success: { icon: 'fa-check-circle',   color: 'var(--success-color)', autoDismiss: 3000 },
    warning: { icon: 'fa-exclamation-triangle', color: 'var(--status-fbbf24)', autoDismiss: 5000 },
    error:   { icon: 'fa-times-circle',   color: 'var(--error-color)',   autoDismiss: 5000 },
    voice:   { icon: 'fa-microphone',     color: 'var(--success-color)', autoDismiss: 0 },
};

export class ToastManager {
    /** @type {HTMLElement|null} */
    container = null;
    /** @type {string|null} 当前常驻语音消息的ID */
    voiceToastId = null;

    /**
     * 初始化容器（由外部在 DOM 就绪后调用一次）
     *
     * @param {string} containerId - toast 容器的 DOM id
     */
    init(containerId = 'toastContainer') {
        this.container = document.getElementById(containerId);
        if (!this.container) {
            this.container = document.createElement('div');
            this.container.id = containerId;
            this.container.className = 'toast-container';
            document.body.appendChild(this.container);
        }
    }

    /**
     * 显示一条 toast 消息
     *
     * @param {string} message  - 消息文本
     * @param {'info'|'success'|'warning'|'error'|'voice'} type - 消息类型
     * @param {number} [duration] - 自定义持续时间（毫秒），0 表示常驻不消失
     * @returns {string} toast 元素 ID，可用于手动关闭
     */
    show(message, type = 'info', duration) {
        if (!this.container) {
            this.init();
        }

        const config = TYPE_CONFIG[type] || TYPE_CONFIG.info;
        const autoDismiss = duration !== undefined ? duration : config.autoDismiss;
        const id = `toast-${Date.now()}-${Math.random().toString(36).slice(2, 7)}`;

        const toast = document.createElement('div');
        toast.id = id;
        toast.className = `toast-item toast-${type}`;
        toast.style.setProperty('--toast-color', config.color);
        toast.innerHTML = `
            <i class="fas ${config.icon} toast-icon"></i>
            <span class="toast-message">${this.escapeHtml(message)}</span>
            <button class="toast-close" title="关闭">
                <i class="fas fa-times"></i>
            </button>
        `;

        // 关闭按钮事件
        toast.querySelector('.toast-close')?.addEventListener('click', () => {
            this.dismiss(id);
        });

        // 如果已有同类型 voice toast，先关闭旧的
        if (type === 'voice' && this.voiceToastId) {
            this.dismiss(this.voiceToastId);
        }

        this.container.appendChild(toast);

        // 入场动画：延迟一帧以触发 transition
        requestAnimationFrame(() => {
            toast.classList.add('toast-visible');
        });

        // 自动消失
        if (autoDismiss > 0) {
            setTimeout(() => {
                this.dismiss(id);
            }, autoDismiss);
        } else if (type === 'voice') {
            this.voiceToastId = id;
        }

        return id;
    }

    /**
     * 关闭指定 toast
     *
     * @param {string} id - toast 元素 ID
     */
    dismiss(id) {
        const toast = document.getElementById(id);
        if (!toast) return;

        if (this.voiceToastId === id) {
            this.voiceToastId = null;
        }

        toast.classList.remove('toast-visible');
        toast.classList.add('toast-leaving');

        // 动画结束后移除元素
        const onTransitionEnd = () => {
            toast.removeEventListener('transitionend', onTransitionEnd);
            if (toast.parentNode) {
                toast.parentNode.removeChild(toast);
            }
        };
        toast.addEventListener('transitionend', onTransitionEnd);

        // 兜底：500ms 后强制移除
        setTimeout(() => {
            if (toast.parentNode) {
                toast.parentNode.removeChild(toast);
            }
        }, 500);
    }

    /**
     * 关闭语音识别常驻消息
     */
    dismissVoice() {
        if (this.voiceToastId) {
            this.dismiss(this.voiceToastId);
        }
    }

    /**
     * 显示信息提示
     */
    info(message, duration) { return this.show(message, 'info', duration); }

    /**
     * 显示成功提示
     */
    success(message, duration) { return this.show(message, 'success', duration); }

    /**
     * 显示警告提示
     */
    warning(message, duration) { return this.show(message, 'warning', duration); }

    /**
     * 显示错误提示
     */
    error(message, duration) { return this.show(message, 'error', duration); }

    /**
     * 显示语音识别常驻状态
     */
    voice(message = '语音识别中...') { return this.show(message, 'voice', 0); }

    /**
     * HTML 转义
     */
    escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }
}

/** 全局 Toast 单例 */
export const Toast = new ToastManager();