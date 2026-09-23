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
    // 删除消息：立即保存到本地，并刷新 1 分钟持久化判定计时
    schedulePersist(true);
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
    // 不再在每条消息到达时立即写盘：由 1 分钟周期判定器在内容变化时统一落盘，
    // 避免流式回复期间的高频磁盘写入；删除消息则立即保存（见 deleteMessage）
}

// ---------- 消息切片存储（database/message/fragment_xxxx.json，每片至多 50 条） ----------
// 计算第 index 个切片的存储路径（fragment_0001.json 起）
function fragmentPath(index) {
    return `${MESSAGE_DIR_PATH}/fragment_${String(index).padStart(4, '0')}.json`;
}

// 写入单个切片文件，失败仅记录不抛出，保证其余切片的写入/清空不被中断
async function writeFragmentFile(path, data) {
    try {
        const res = await fetch('/file/write', {
            method: 'POST',
            headers: {
                'X-File-Name': encodeFilePath(path),
                'X-Overwrite': 'true'
            },
            body: data
        });
        if (!res.ok) throw new Error(`HTTP ${res.status}`);
        return true;
    } catch (e) {
        console.warn(`切片写入失败: ${path}`, e);
        return false;
    }
}

// 将消息按 50 条一片切分写入数据库切片目录；
// 消息减少（删除/清空）后，超出当前片数的旧切片逐个覆写为空数组，避免下次加载读到残留消息
async function persistMessages() {
    const fragmentCount = Math.max(1, Math.ceil(messages.length / MESSAGE_FRAGMENT_LIMIT));
    let allWritten = true;
    for (let i = 1; i <= fragmentCount; i++) {
        const slice = messages.slice((i - 1) * MESSAGE_FRAGMENT_LIMIT, i * MESSAGE_FRAGMENT_LIMIT);
        if (!await writeFragmentFile(fragmentPath(i), JSON.stringify(slice))) allWritten = false;
    }
    // 清空全部残留切片：逐个覆写为空数组，某片失败不影响其余切片
    for (let i = fragmentCount + 1; i <= persistedFragmentCount; i++) {
        if (!await writeFragmentFile(fragmentPath(i), '[]')) allWritten = false;
    }
    if (fragmentCount > persistedFragmentCount) persistedFragmentCount = fragmentCount;
    // 仅在全部写入成功时刷新快照，否则保留旧快照让周期判定器重试
    if (allWritten) lastPersistSnapshot = JSON.stringify(messages);
}

// 启动 1 分钟周期的持久化判定：循环检查消息内容相对上次保存是否有变化
function startPersistWatcher() {
    if (saveTimer) clearInterval(saveTimer);
    saveTimer = setInterval(checkMessagesAndPersist, PERSIST_INTERVAL_MS);
}

// 每 1 分钟判定一次：消息内容与上次保存不同则触发保存到磁盘
function checkMessagesAndPersist() {
    const data = JSON.stringify(messages);
    if (data === lastPersistSnapshot) return;
    persistMessages();
}

// 改动后的统一入口：刷新 1 分钟判定计时；immediate 为 true 时立即保存到本地（删除消息场景）
function schedulePersist(immediate) {
    startPersistWatcher();
    if (immediate) persistMessages();
}

// 按顺序逐一加载聊天记录切片；缺失或无法读取/解析的切片跳过，
// 连续缺失达到上限或扫描数量达到兜底上限即认为加载完毕
async function loadPersistedMessages() {
    // 以当前 messages（含无文件时的空态）作为上一次落盘基线
    lastPersistSnapshot = JSON.stringify(messages);
    // 启动 1 分钟周期的持久化判定
    startPersistWatcher();
    const loaded = [];
    let misses = 0;
    for (let index = 1; index <= MESSAGE_LOAD_MAX_SCANS && misses < MESSAGE_LOAD_MAX_MISSES; index++) {
        let list = null;
        try {
            const res = await fetch('/file/read/' + fragmentPath(index));
            if (res.ok) {
                const parsed = await res.json();
                if (Array.isArray(parsed)) list = parsed;
            }
        } catch (e) {
            // 读取或解析失败：按缺失处理，跳过该切片
        }
        if (list) {
            misses = 0;
            if (index > persistedFragmentCount) persistedFragmentCount = index;
            loaded.push(...list.filter(m => m && typeof m === 'object'));
        } else {
            misses++;
        }
    }
    if (!loaded.length) return;
    // 超出保留上限的历史直接丢弃（不做旧格式迁移）
    messages = loaded.length > MAX_PERSISTED_MESSAGES ? loaded.slice(-MAX_PERSISTED_MESSAGES) : loaded;
    // 加载后刷新基线，避免首轮周期把旧内容误判为变更而重复写盘
    lastPersistSnapshot = JSON.stringify(messages);
    const renders = messages.map(msg => renderMessageElement(msg));
    updateEmptyState();
    // 等待所有消息的 markdown/mermaid 异步渲染完成后再滚动到底部
    await Promise.all(renders);
    scrollToBottom(false);
    // 懒加载图片进入视口后异步加载会改变高度，延迟补滚确保到达真实底部
    setTimeout(() => scrollToBottom(false), 250);
    applyFilters();
}
