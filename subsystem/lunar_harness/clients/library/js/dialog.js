// dialog.js 提示条与模态弹窗（重命名 / 移动 / 删除 / 快捷键）。

const root = () => document.getElementById('modal-root');

// toast 顶部提示。
export function toast(message, type = '') {
    const el = document.getElementById('toast');
    el.className = `toast visible ${type}`.trim();
    el.textContent = message;
    clearTimeout(toast.timer);
    toast.timer = setTimeout(() => {
        el.classList.remove('visible');
    }, 2200);
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
