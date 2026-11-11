// dialog.js 轻提示与模态弹窗（重命名 / 移动 / 删除 / 快捷键）。

const root = () => document.getElementById('modal-root');

// TOAST_LIFE_MS 单条提示存活时长；TOAST_MAX_VISIBLE 同时可见条数上限
const TOAST_LIFE_MS = 2600;
const TOAST_MAX_VISIBLE = 3;
const TOAST_EXIT_MS = 200;

// toast 轻提示：固定定位、不参与文档流，自动消失、不阻塞操作。
//
// 为什么改成堆叠而不是单条覆盖：后端进度推送（阶段、失败原因、完成摘要）
// 都走这里，单条覆盖会让后到的提示把前一条直接顶掉。堆叠 + 条数上限
// 既保证每条都看得见，又不会在屏幕上堆积出一片文字。
export function toast(message, type = '') {
    const container = document.getElementById('toast-root');
    const text = String(message == null ? '' : message).trim();
    if (!container || !text) return;

    // 同文案去重：连续相同的提示只把计时往后推，不重复叠一颗。
    const last = container.lastElementChild;
    if (last && last.dataset.text === text && last.dataset.type === type) {
        last.classList.add('visible');
        restartToastTimer(last);
        return;
    }

    const el = document.createElement('div');
    el.className = `toast${type ? ` ${type}` : ''}`;
    el.dataset.text = text;
    el.dataset.type = type;
    el.textContent = text;
    container.append(el);
    while (container.childElementCount > TOAST_MAX_VISIBLE) {
        container.firstElementChild.remove();
    }
    // 下一帧再加 visible，保证入场过渡生效（同步添加会被合并掉）。
    requestAnimationFrame(() => el.classList.add('visible'));
    restartToastTimer(el);
}

// restartToastTimer 重排一条提示的消失计时。
function restartToastTimer(el) {
    if (el.toastTimer) clearTimeout(el.toastTimer);
    el.toastTimer = setTimeout(() => {
        el.classList.remove('visible');
        el.toastExitTimer = setTimeout(() => el.remove(), TOAST_EXIT_MS);
    }, TOAST_LIFE_MS);
}

// ===== 运行态浮窗 =====
// RUN_STATUS_HIDE_MS 终态停留时长：任务结束后浮窗再停留一会儿，然后自动收起。
const RUN_STATUS_HIDE_MS = 6000;
const RUN_STATUS_EXIT_MS = 200;

let runStatusKey = '';
let runStatusDismissed = '';
let runStatusHideTimer = 0;

// updateRunStatus 维护那一条常驻运行态浮窗。
//
// 为什么用浮窗而不是页内文本：创作 / 演绎任务高频推送阶段与百分比，
// 写进表单会撑开布局、挤动按钮；浮窗固定定位、不占文档流，页面结构保持恒定。
export function updateRunStatus(payload) {
    const card = document.getElementById('run-status');
    if (!card || !payload) return;
    const key = String(payload.key || '');
    if (key !== runStatusKey) {
        runStatusKey = key;
        runStatusDismissed = '';
    }
    // 用户手动收起后同一任务不再自动弹回；换任务（key 变化）才会重新出现。
    if (key && key === runStatusDismissed) return;

    clearTimeout(runStatusHideTimer);
    const tone = payload.tone === 'success' || payload.tone === 'error' ? payload.tone : '';
    card.classList.remove('is-success', 'is-error');
    if (tone) card.classList.add(`is-${tone}`);
    card.classList.remove('hidden');

    const title = document.getElementById('run-status-title');
    const stage = document.getElementById('run-status-stage');
    const percent = document.getElementById('run-status-percent');
    const bar = document.getElementById('run-status-bar');
    const detail = document.getElementById('run-status-detail');
    if (title) title.textContent = payload.title || '运行中';
    if (stage) stage.textContent = payload.stage || '';
    const progress = Math.max(0, Math.min(100, Math.round(Number(payload.progress) || 0)));
    if (percent) percent.textContent = `${progress}%`;
    if (bar) {
        bar.classList.toggle('is-indeterminate', progress <= 0 && !payload.terminal);
        bar.style.width = progress > 0 ? `${progress}%` : '';
    }
    if (detail) {
        const text = String(payload.detail || '').trim();
        detail.textContent = text;
        detail.classList.toggle('hidden', !text);
    }

    // 下一帧再加 visible，保证从隐藏状态出现时入场过渡生效。
    requestAnimationFrame(() => card.classList.add('visible'));
    if (payload.terminal) {
        runStatusHideTimer = setTimeout(() => hideRunStatus(true), RUN_STATUS_HIDE_MS);
    }
}

// dismissRunStatus 用户手动收起浮窗，并记住该任务不再自动弹回。
export function dismissRunStatus() {
    hideRunStatus(true);
}

// clearRunStatus 静默收起浮窗：提交失败这类没有任务可跟踪的场景用它，
// 不记入已收起名单，用户重试时浮窗还能正常出现。
export function clearRunStatus() {
    hideRunStatus(false);
}

// hideRunStatus 收起浮窗；remember 为真时记下当前任务，避免下一次推送又弹回来。
function hideRunStatus(remember) {
    clearTimeout(runStatusHideTimer);
    const card = document.getElementById('run-status');
    if (!card) return;
    if (remember) runStatusDismissed = runStatusKey;
    card.classList.remove('visible');
    setTimeout(() => {
        // 期间若来了新任务并重新显示，就不要再补上 hidden。
        if (!card.classList.contains('visible')) card.classList.add('hidden');
    }, RUN_STATUS_EXIT_MS);
}

// openModal 打开一个模态弹窗。
function openModal({ title, body, actions }) {
    const mask = document.createElement('div');
    mask.className = 'modal-mask';
    const modal = document.createElement('div');
    modal.className = 'modal';
    const head = document.createElement('div');
    head.className = 'modal-head';
    const titleEl = document.createElement('span');
    titleEl.textContent = title;
    const closeBtn = document.createElement('button');
    closeBtn.className = 'icon-btn';
    closeBtn.innerHTML = '<i class="fas fa-times"></i>';
    head.append(titleEl, closeBtn);
    const bodyEl = document.createElement('div');
    bodyEl.className = 'modal-body';
    if (typeof body === 'string') bodyEl.textContent = body;
    else bodyEl.append(body);
    const actionsEl = document.createElement('div');
    actionsEl.className = 'modal-actions';
    actionsEl.append(...actions);
    modal.append(head, bodyEl, actionsEl);
    mask.append(modal);
    root().replaceChildren(mask);

    const close = () => {
        mask.remove();
        document.removeEventListener('keydown', onKey, true);
    };
    const onKey = (event) => {
        if (event.key === 'Escape') {
            event.stopPropagation();
            close();
        }
    };
    document.addEventListener('keydown', onKey, true);
    closeBtn.addEventListener('click', close);
    mask.addEventListener('click', (event) => {
        if (event.target === mask) close();
    });
    return { close, bodyEl };
}

// confirmDialog 确认弹窗。
export function confirmDialog(title, message, confirmText = '确认') {
    return new Promise((resolve) => {
        const cancel = document.createElement('button');
        cancel.className = 'btn-glass';
        cancel.textContent = '取消';
        const ok = document.createElement('button');
        ok.className = 'btn-glass primary';
        ok.textContent = confirmText;
        const modal = openModal({ title, body: message, actions: [cancel, ok] });
        cancel.addEventListener('click', () => {
            modal.close();
            resolve(false);
        });
        ok.addEventListener('click', () => {
            modal.close();
            resolve(true);
        });
    });
}

// promptDialog 单行输入弹窗（返回 null 表示取消）。
export function promptDialog(title, label, value, confirmText = '确定') {
    return new Promise((resolve) => {
        const wrap = document.createElement('div');
        const labelEl = document.createElement('label');
        labelEl.textContent = label;
        const input = document.createElement('input');
        input.className = 'modal-input';
        input.value = value || '';
        input.spellcheck = false;
        wrap.append(labelEl, input);

        const cancel = document.createElement('button');
        cancel.className = 'btn-glass';
        cancel.textContent = '取消';
        const ok = document.createElement('button');
        ok.className = 'btn-glass primary';
        ok.textContent = confirmText;

        const modal = openModal({ title, body: wrap, actions: [cancel, ok] });
        const finish = (result) => {
            modal.close();
            resolve(result);
        };
        cancel.addEventListener('click', () => finish(null));
        ok.addEventListener('click', () => {
            const text = input.value.trim();
            if (!text) {
                toast('名称不能为空', 'error');
                return;
            }
            finish(text);
        });
        input.addEventListener('keydown', (event) => {
            if (event.key === 'Enter') {
                event.preventDefault();
                ok.click();
            }
        });
        setTimeout(() => {
            input.focus();
            input.select();
        }, 30);
    });
}

// folderDialog 目标目录选择（返回 null 表示取消）。
export function folderDialog(dirs, currentPath) {
    return new Promise((resolve) => {
        const list = document.createElement('div');
        list.className = 'folder-list';
        let chosen = currentPath || dirs[0] || '';
        dirs.forEach((dir) => {
            const item = document.createElement('button');
            item.className = 'folder-item';
            if (dir === chosen) item.classList.add('active');
            item.innerHTML = `<i class="fas fa-folder"></i><span></span>`;
            item.querySelector('span').textContent = dir;
            item.addEventListener('click', () => {
                chosen = dir;
                list.querySelectorAll('.folder-item').forEach((el) => el.classList.remove('active'));
                item.classList.add('active');
            });
            list.append(item);
        });
        const cancel = document.createElement('button');
        cancel.className = 'btn-glass';
        cancel.textContent = '取消';
        const ok = document.createElement('button');
        ok.className = 'btn-glass primary';
        ok.textContent = '移动到此处';
        const modal = openModal({ title: '移动文件', body: list, actions: [cancel, ok] });
        cancel.addEventListener('click', () => {
            modal.close();
            resolve(null);
        });
        ok.addEventListener('click', () => {
            modal.close();
            resolve(chosen);
        });
    });
}

// helpDialog 键鼠交互原语速查。
export function helpDialog(items) {
    const table = document.createElement('div');
    table.className = 'change-list';
    items.forEach(([key, desc]) => {
        const row = document.createElement('div');
        row.className = 'detail-row';
        const k = document.createElement('span');
        k.className = 'k';
        k.textContent = key;
        const v = document.createElement('span');
        v.className = 'v';
        v.textContent = desc;
        row.append(k, v);
        table.append(row);
    });
    const close = document.createElement('button');
    close.className = 'btn-glass primary';
    close.textContent = '知道了';
    const modal = openModal({ title: '键鼠操作', body: table, actions: [close] });
    close.addEventListener('click', modal.close);
}
