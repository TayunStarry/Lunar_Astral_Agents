/**
 * 模态框处理模块
 * 负责通用操作模态框（确认 / 输入）的封装与快捷方法
 */

/**
 * 当前编辑的文件
 * @type {Object|null}
 */
let currentEditFile = null;

/**
 * 操作模态框配置类型枚举
 * @enum {string}
 */
const ActionModalType = {
    CONFIRM: 'confirm',
    PROMPT: 'prompt'
};

/**
 * 通用操作模态框 — 封装 show / hide / 事件绑定
 * 返回 Promise，confirm 模式下 resolve(true/false)，prompt 模式下 resolve(string|null)
 *
 * @param {Object} options - 配置项
 * @param {string} options.type   - 'confirm' | 'prompt'
 * @param {string} options.title  - 标题
 * @param {string} options.message - 描述文字（confirm 模式必填）
 * @param {string} [options.label] - 输入框标签（prompt 模式）
 * @param {string} [options.defaultValue] - 输入框默认值（prompt 模式）
 * @param {string} [options.hint] - 输入框提示文字（prompt 模式）
 * @param {string} [options.icon] - Font Awesome 图标类名（'fa-question-circle'）
 * @param {string} [options.iconType] - 图标风格: 'info' | 'danger' | 'warning'
 * @param {string} [options.confirmText] - 确认按钮文字
 * @param {string} [options.confirmClass] - 确认按钮额外 CSS 类 ('btn-primary', 'btn-danger')
 * @param {string} [options.cancelText] - 取消按钮文字
 * @param {Function} [options.onValidate] - (value: string) => string|null  校验函数，返回错误信息
 * @returns {Promise<boolean|string|null>}
 */
function showActionModal(options) {
    const {
        type,
        title,
        message = '',
        label = '',
        defaultValue = '',
        hint = '',
        icon = 'fa-question-circle',
        iconType = 'info',
        confirmText = '确认',
        confirmClass = 'btn-primary',
        cancelText = '取消',
        onValidate = null
    } = options;

    return new Promise((resolve) => {
        const modal = document.getElementById('action-modal');
        const modalContent = modal.querySelector('.action-modal-content');
        const closeBtn = document.getElementById('action-modal-close');
        const iconEl = document.getElementById('action-modal-icon');
        const iconInner = iconEl.querySelector('i');
        const titleEl = document.getElementById('action-modal-title');
        const messageEl = document.getElementById('action-modal-message');
        const inputGroup = document.getElementById('action-modal-input-group');
        const labelEl = document.getElementById('action-modal-label');
        const inputEl = document.getElementById('action-modal-input');
        const hintEl = document.getElementById('action-modal-hint');
        const cancelBtn = document.getElementById('action-modal-cancel');
        const confirmBtn = document.getElementById('action-modal-confirm');

        // ---- 清除上一次的状态 ----
        let resolved = false;
        inputEl.value = '';
        inputEl.classList.remove('error');
        hintEl.textContent = '';
        hintEl.classList.remove('error');
        // 重置动画（移除后重排触发）
        modalContent.style.animation = 'none';
        void modalContent.offsetWidth;
        modalContent.style.animation = '';

        /**
         * 安全 resolve，防止重复关闭
         */
        function finalize(value) {
            if (resolved) return;
            resolved = true;
            modal.classList.remove('show');
            resolve(value);
        }

        // ---- 填充 UI ----
        // 图标
        iconEl.className = 'action-modal-icon';
        if (iconType === 'danger') iconEl.classList.add('danger');
        else if (iconType === 'warning') iconEl.classList.add('warning');
        else iconEl.classList.add('info');
        iconInner.className = `fas ${icon}`;

        titleEl.textContent = title;
        messageEl.textContent = message;

        // 输入区域
        if (type === ActionModalType.PROMPT) {
            inputGroup.style.display = 'block';
            labelEl.textContent = label;
            inputEl.value = defaultValue;
            hintEl.textContent = hint;
            inputEl.classList.remove('error');
            hintEl.classList.remove('error');
        } else {
            inputGroup.style.display = 'none';
        }

        // 按钮
        cancelBtn.innerHTML = `<i class="fas fa-times"></i> ${cancelText}`;
        confirmBtn.className = `btn ${confirmClass}`;
        confirmBtn.innerHTML = `<i class="fas fa-check"></i> ${confirmText}`;

        // ---- 事件绑定 ----
        /**
         * 处理确认
         */
        function handleConfirm() {
            if (type === ActionModalType.PROMPT) {
                const value = inputEl.value.trim();
                if (onValidate) {
                    const error = onValidate(value);
                    if (error) {
                        inputEl.classList.add('error');
                        hintEl.textContent = error;
                        hintEl.classList.add('error');
                        inputEl.focus();
                        return;
                    }
                }
                finalize(value || null);
            } else {
                finalize(true);
            }
        }

        /**
         * 处理取消
         */
        function handleCancel() {
            finalize(type === ActionModalType.PROMPT ? null : false);
        }

        // 绑定事件
        confirmBtn.onclick = handleConfirm;
        cancelBtn.onclick = handleCancel;
        closeBtn.onclick = handleCancel;

        // 点击遮罩关闭
        modal.onclick = (e) => {
            if (e.target === modal) handleCancel();
        };

        // 键盘支持
        modal.onkeydown = null; // 清除旧监听器
        const keydownHandler = (e) => {
            if (e.key === 'Enter') {
                e.preventDefault();
                handleConfirm();
            } else if (e.key === 'Escape') {
                e.preventDefault();
                handleCancel();
            }
        };
        modal.addEventListener('keydown', keydownHandler, { once: false });

        // 清理键盘监听器（在关闭时）
        const cleanupKeydown = () => {
            modal.removeEventListener('keydown', keydownHandler);
        };
        const origFinalize = finalize;
        finalize = (value) => {
            cleanupKeydown();
            origFinalize(value);
        };

        // ---- 显示 ----
        modal.classList.add('show');

        // 聚焦输入框或确认按钮
        if (type === ActionModalType.PROMPT) {
            requestAnimationFrame(() => {
                inputEl.focus();
                // 选中默认值文本方便替换
                if (defaultValue) inputEl.select();
            });
        } else {
            requestAnimationFrame(() => confirmBtn.focus());
        }
    });
}

/**
 * 快捷确认模态框
 * @param {string} title - 标题
 * @param {string} message - 描述文字
 * @param {'danger'|'warning'|'info'} [type='danger'] - 风格
 * @returns {Promise<boolean>}
 */
function showConfirmModal(title, message, type = 'danger') {
    const iconMap = {
        danger: { icon: 'fa-exclamation-triangle', iconType: 'danger', confirmClass: 'btn-danger', confirmText: '确认删除' },
        warning: { icon: 'fa-exclamation-circle', iconType: 'warning', confirmClass: 'btn-accent', confirmText: '确认' },
        info: { icon: 'fa-info-circle', iconType: 'info', confirmClass: 'btn-primary', confirmText: '确认' }
    };
    const cfg = iconMap[type] || iconMap.info;

    return showActionModal({
        type: ActionModalType.CONFIRM,
        title,
        message,
        icon: cfg.icon,
        iconType: cfg.iconType,
        confirmText: cfg.confirmText,
        confirmClass: cfg.confirmClass
    });
}

/**
 * 快捷输入模态框
 * @param {string} title - 标题
 * @param {string} label - 输入框标签
 * @param {string} [defaultValue=''] - 默认值
 * @param {string} [hint=''] - 输入提示
 * @param {Function} [onValidate] - 校验函数 (value) => errorString|null
 * @returns {Promise<string|null>} 用户输入值，取消时返回 null
 */
function showPromptModal(title, label, defaultValue = '', hint = '', onValidate = null) {
    return showActionModal({
        type: ActionModalType.PROMPT,
        title,
        label,
        defaultValue,
        hint,
        icon: 'fa-pen-to-square',
        iconType: 'info',
        confirmText: '确认',
        confirmClass: 'btn-primary',
        onValidate
    });
}
