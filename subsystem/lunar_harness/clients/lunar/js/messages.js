// ============================================================
//  星月智能 · 消息终端 — 消息增删 / 持久化 / 文件引用
// ============================================================

// ---------- 资源引用（阅读者智能体） ----------
/** 构建消息内可点击的资源图标块：点击将该资源句柄载入输入框 */
function buildFileRefBlock(refId, label) {
    const pill = document.createElement('div');
    pill.className = 'file-ref-pill';
    pill.title = `点击载入资源引用 ${refId}`;
    const icon = document.createElement('i');
    icon.className = 'fas fa-file-alt';
    const name = document.createElement('span');
    name.className = 'file-ref-name';
    // 展示句柄对应的文件名，悬停显示规范句柄
    name.textContent = label || resourceNameCache.get(baseResourceHandle(refId)) || refId;
    name.title = refId;
    if (!label) {
        resolveResourceName(refId).then(resolved => {
            if (resolved) name.textContent = resolved;
        });
    }
    pill.appendChild(icon);
    pill.appendChild(name);
    pill.addEventListener('click', () => addFileReference(refId));
    return pill;
}

/** 将资源句柄加入输入框（去重），并渲染图标条 */
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
        // 展示句柄对应的文件名，悬停显示规范句柄
        name.textContent = resourceNameCache.get(baseResourceHandle(refId)) || refId;
        name.title = refId;
        resolveResourceName(refId).then(resolved => {
            if (resolved) name.textContent = resolved;
        });
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

// ---------- 消息增删 ----------
async function copyMessage(msg) {
    let text = msg.role === 'user' ? stripUserPrefix(msg.content) : (msg.content || '');
    if (!text && msg.imageSrc) text = msg.imageSrc;
    if (!text && msg.abcNotation) text = msg.abcNotation;
    const ok = await copyToClipboard(text);
    showToast(ok ? '已复制' : '复制失败', ok ? 'success' : 'error');
}

function addMessage(msg) {
    messages.push(msg);
    if (messages.length > MAX_CACHED_MESSAGES) {
        const removed = messages.shift();
        const oldEl = messageArea.querySelector(`.message[data-id="${removed.id}"]`);
        if (oldEl) oldEl.remove();
    }
    renderMessageElement(msg);
    updateEmptyState();
    scrollToBottom(true);
    applyFilters();
    // 前端不再落盘：聊天记录由思维链写入 database/messages.json，页面刷新时统一读取
}

// ---------- 消息读取（仅刷新时从缓存文件同步） ----------

// 读取消息文件并解析为消息数组；缺失或无法读取/解析时返回 null
async function readMessageFile(path) {
    try {
        const res = await fetch('/file/read/' + path);
        if (!res.ok) return null;
        const parsed = await res.json();
        return Array.isArray(parsed) ? parsed.filter(m => m && typeof m === 'object') : null;
    } catch (e) {
        // 读取或解析失败：按缺失处理
        return null;
    }
}

// 加载聊天记录：读取思维链维护的缓存文件，仅保留最新 MAX_CACHED_MESSAGES 条
async function loadPersistedMessages() {
    const loaded = await readMessageFile(MESSAGE_FILE_PATH);
    if (!loaded || !loaded.length) return;
    // 超出保留上限的历史直接丢弃（仅保留最新消息）
    messages = loaded.length > MAX_CACHED_MESSAGES ? loaded.slice(-MAX_CACHED_MESSAGES) : loaded;
    const renders = messages.map(msg => renderMessageElement(msg));
    updateEmptyState();
    // 等待所有消息的 markdown/mermaid 异步渲染完成后再滚动到底部
    await Promise.all(renders);
    scrollToBottom(false);
    // 懒加载图片进入视口后异步加载会改变高度，延迟补滚确保到达真实底部
    setTimeout(() => scrollToBottom(false), 250);
    applyFilters();
}
