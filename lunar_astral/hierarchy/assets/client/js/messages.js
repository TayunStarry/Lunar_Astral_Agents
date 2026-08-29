// ============================================================
//  星月智能 · 消息终端 — 消息增删 / 持久化 / 文件引用
// ============================================================

// ---------- 文件引用（阅读者智能体） ----------
/** 构建消息内可点击的文件图标块：点击将引用 `#fileName.ext` 载入输入框 */
function buildFileRefBlock(refId) {
    const pill = document.createElement('div');
    pill.className = 'file-ref-pill';
    pill.title = `点击载入文件引用 ${refId}`;
    const icon = document.createElement('i');
    icon.className = 'fas fa-file-alt';
    const name = document.createElement('span');
    name.className = 'file-ref-name';
    name.textContent = refId;
    pill.appendChild(icon);
    pill.appendChild(name);
    pill.addEventListener('click', () => addFileReference(refId));
    return pill;
}

/** 将文件引用加入输入框（去重），并渲染图标条 */
function addFileReference(refId) {
    if (!refId) return;
    if (!referencedFiles.includes(refId)) referencedFiles.push(refId);
    renderFileRefChips();
    messageInput.focus();
}

function removeFileReference(index) {
    referencedFiles.splice(index, 1);
    renderFileRefChips();
}

/** 渲染输入框上方的文件引用图标条（输入框仍显示为文件图标） */
function renderFileRefChips() {
    fileRefChips.innerHTML = '';
    if (!referencedFiles.length) {
        fileRefChips.hidden = true;
        return;
    }
    fileRefChips.hidden = false;
    referencedFiles.forEach((refId, idx) => {
        const chip = document.createElement('div');
        chip.className = 'file-ref-chip';
        const icon = document.createElement('i');
        icon.className = 'fas fa-file-alt';
        const name = document.createElement('span');
        name.className = 'file-ref-chip-name';
        name.textContent = refId;
        name.title = refId;
        const removeBtn = document.createElement('button');
        removeBtn.className = 'pending-attachment-remove';
        removeBtn.title = '移除引用';
        removeBtn.innerHTML = '<i class="fas fa-times"></i>';
        removeBtn.addEventListener('click', () => removeFileReference(idx));
        chip.appendChild(icon);
        chip.appendChild(name);
        chip.appendChild(removeBtn);
        fileRefChips.appendChild(chip);
    });
}

// ---------- 消息增删与持久化 ----------
async function copyMessage(msg) {
    let text = msg.content || '';
    if (!text && msg.imageSrc) text = msg.imageSrc;
    if (!text && msg.abcNotation) text = msg.abcNotation;
    const ok = await copyToClipboard(text);
    showToast(ok ? '已复制' : '复制失败', ok ? 'success' : 'error');
}

function deleteMessage(id) {
    const el = messageArea.querySelector(`.message[data-id="${id}"]`);
    if (el) el.remove();
    messages = messages.filter(m => m.id !== id);
    updateEmptyState();
    schedulePersist();
    showToast('消息已删除', 'info');
}

function addMessage(msg) {
    messages.push(msg);
    if (messages.length > MAX_PERSISTED_MESSAGES) {
        const removed = messages.shift();
        const oldEl = messageArea.querySelector(`.message[data-id="${removed.id}"]`);
        if (oldEl) oldEl.remove();
    }
    renderMessageElement(msg);
    updateEmptyState();
    scrollToBottom(true);
    applyFilters();
    schedulePersist();
}

async function persistMessages() {
    const data = JSON.stringify(messages, null, 2);
    try {
        await fetch('/file/write', {
            method: 'POST',
            headers: {
                'X-File-Name': encodeFilePath(MESSAGES_FILE_PATH),
                'X-Overwrite': 'true'
            },
            body: data
        });
    } catch (e) {
        console.warn('消息持久化失败', e);
    }
}

function schedulePersist() {
    clearTimeout(saveTimer);
    saveTimer = setTimeout(persistMessages, 800);
}

async function loadPersistedMessages() {
    try {
        const res = await fetch('/file/read/database/messages.json');
        if (!res.ok) return;
        const list = await res.json();
        if (!Array.isArray(list)) return;
        messages = list;
        const renders = list.map(msg => renderMessageElement(msg));
        updateEmptyState();
        // 等待所有消息的 markdown/mermaid 异步渲染完成后再滚动到底部
        await Promise.all(renders);
        scrollToBottom(false);
        // 懒加载图片进入视口后异步加载会改变高度，延迟补滚确保到达真实底部
        setTimeout(() => scrollToBottom(false), 250);
        applyFilters();
    } catch (e) {
        // 无持久化文件或读取失败，从空状态开始
    }
}
