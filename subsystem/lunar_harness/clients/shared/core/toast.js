/**
 * 星月智能 · 共享 Toast 控制器
 *
 * 统一队列、生命周期、自动消失与语音常驻逻辑；具体视觉（容器位置、
 * 圆角、玻璃效果、进出场动画）由两端各自的 CSS 决定。
 *
 * 端差异全部通过构造参数表达：
 * - 桌面端：单类名 `toast`、无关闭按钮、图标内联间距、固定 2.8s
 * - 移动端：`toast-item toast-{type}`、带关闭按钮、按类型区分时长
 */

export const TOAST_TYPES = {
	info: { icon: 'fa-info-circle', color: 'var(--primary-color)', autoDismiss: 3000 },
	success: { icon: 'fa-check-circle', color: 'var(--success-color)', autoDismiss: 3000 },
	warning: { icon: 'fa-exclamation-triangle', color: 'var(--status-fbbf24)', autoDismiss: 5000 },
	error: { icon: 'fa-times-circle', color: 'var(--error-color)', autoDismiss: 5000 },
	voice: { icon: 'fa-microphone', color: 'var(--success-color)', autoDismiss: 0 }
};

export class ToastController {
	constructor({
		containerId = 'toastContainer',
		containerClass = 'toast-container',
		baseClass = 'toast-item',
		typeClassPrefix = 'toast-',
		types = TOAST_TYPES,
		showIcon = true,
		showClose = true,
		iconStyle = '',
		enterClass = 'toast-visible',
		leaveClass = 'toast-leaving',
		colorVariable = '--toast-color',
		removeDelay = 500
	} = {}) {
		this.containerId = containerId;
		this.containerClass = containerClass;
		this.baseClass = baseClass;
		this.typeClassPrefix = typeClassPrefix;
		this.types = types;
		this.showIcon = showIcon;
		this.showClose = showClose;
		this.iconStyle = iconStyle;
		this.enterClass = enterClass;
		this.leaveClass = leaveClass;
		this.colorVariable = colorVariable;
		this.removeDelay = removeDelay;
		/** @type {HTMLElement|null} */
		this.container = null;
		/** @type {string|null} 当前常驻语音消息的 ID */
		this.voiceToastId = null;
	}

	/** 初始化容器（由外部在 DOM 就绪后调用一次；show 时也会懒初始化） */
	init(containerId = this.containerId) {
		this.containerId = containerId;
		this.container = document.getElementById(containerId);
		if (!this.container) {
			this.container = document.createElement('div');
			this.container.id = containerId;
			this.container.className = this.containerClass;
			document.body.appendChild(this.container);
		}
		return this.container;
	}

	/**
	 * 显示一条提示
	 *
	 * @param {string} message 消息文本
	 * @param {string} [type] 消息类型
	 * @param {number} [duration] 持续时间（毫秒），0 表示常驻；缺省用类型配置
	 * @returns {string} toast 元素 ID
	 */
	show(message, type = 'info', duration) {
		if (!this.container) this.init();

		const config = this.types[type] || this.types.info;
		const autoDismiss = duration !== undefined ? duration : config.autoDismiss;
		const id = `toast-${Date.now()}-${Math.random().toString(36).slice(2, 7)}`;

		const toast = document.createElement('div');
		toast.id = id;
		toast.className = this.typeClassPrefix ? `${this.baseClass} ${this.typeClassPrefix}${type}` : this.baseClass;
		if (config.color && this.colorVariable) {
			toast.style.setProperty(this.colorVariable, config.color);
		}

		if (this.showIcon) {
			const icon = document.createElement('i');
			icon.className = `fas ${config.icon} toast-icon`;
			if (this.iconStyle) icon.setAttribute('style', this.iconStyle);
			toast.appendChild(icon);
		}
		const messageEl = document.createElement('span');
		messageEl.className = 'toast-message';
		messageEl.textContent = String(message ?? '');
		toast.appendChild(messageEl);

		if (this.showClose) {
			const close = document.createElement('button');
			close.className = 'toast-close';
			close.title = '关闭';
			close.innerHTML = '<i class="fas fa-times"></i>';
			close.addEventListener('click', () => this.dismiss(id));
			toast.appendChild(close);
		}

		// 同类型 voice toast 只保留一条
		if (type === 'voice' && this.voiceToastId) {
			this.dismiss(this.voiceToastId);
		}

		this.container.appendChild(toast);

		// 入场动画：延迟一帧触发 transition
		if (this.enterClass) {
			requestAnimationFrame(() => toast.classList.add(this.enterClass));
		}

		if (autoDismiss > 0) {
			setTimeout(() => this.dismiss(id), autoDismiss);
		} else if (type === 'voice') {
			this.voiceToastId = id;
		}

		return id;
	}

	/** 关闭指定 toast */
	dismiss(id) {
		const toast = document.getElementById(id);
		if (!toast) return;

		if (this.voiceToastId === id) this.voiceToastId = null;

		if (this.enterClass) toast.classList.remove(this.enterClass);
		if (this.leaveClass) toast.classList.add(this.leaveClass);

		const remove = () => {
			toast.removeEventListener('transitionend', onTransitionEnd);
			if (toast.parentNode) toast.parentNode.removeChild(toast);
		};
		const onTransitionEnd = () => remove();
		toast.addEventListener('transitionend', onTransitionEnd);

		// 兜底：动画未触发时按时移除
		setTimeout(remove, this.removeDelay);
	}

	dismissVoice() {
		if (this.voiceToastId) this.dismiss(this.voiceToastId);
	}

	info(message, duration) { return this.show(message, 'info', duration); }

	success(message, duration) { return this.show(message, 'success', duration); }

	warning(message, duration) { return this.show(message, 'warning', duration); }

	error(message, duration) { return this.show(message, 'error', duration); }

	voice(message = '语音识别中...') { return this.show(message, 'voice', 0); }
}
