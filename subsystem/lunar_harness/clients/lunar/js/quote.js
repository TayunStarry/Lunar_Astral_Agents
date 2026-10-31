// ============================================================
//  星月智能 · 消息终端 — 消息引用（QQ 风格）
// ============================================================
//
// 交互与 QQ 对齐：
//   - 消息悬浮操作条提供「引用」按钮，点击即选中（再次点击取消）；
//   - 支持同时选中多条消息，按选择顺序在输入框上方形成引用条；
//   - 发送后引用随消息进入服务端上下文，音频被引用时才会读取其转写句柄。

const MAX_QUOTED_MESSAGES = 5;
const QUOTE_PREVIEW_LIMIT = 64;

/** 取消息中的音频附件（悬空句柄或转写文本） */
function messageAudioAttachment(msg) {
    if (!msg || !msg.attachments) return null;
    return msg.attachments.find(att => att && att.type === 'audio' && (att.handle || att.text || att.src)) || null;
}

/** 生成引用预览：正文 → 语音转写 → 图片/视频 → 兜底 */
function quoteMessagePreview(msg) {
    if (!msg) return '[消息]';
    const audio = messageAudioAttachment(msg);
    if (audio && audio.text) return truncateQuotePreview('[语音] ' + audio.text);
    let text = msg.role === 'user' ? stripUserPrefix(msg.content || '') : (msg.content || '');
    text = String(text).replace(/\s+/g, ' ').trim();
    if (text) return truncateQuotePreview(text);
    if (msg.attachments && msg.attachments.length) {
        const imageCount = msg.attachments.filter(att => att && att.type === 'image').length;
        const videoCount = msg.attachments.filter(att => att && att.type === 'video').length;
        if (imageCount) return `[图片 ×${imageCount}]`;
        if (videoCount) return `[视频 ×${videoCount}]`;
        if (audio) return '[语音]';
    }
    if (msg.imageSrc) return '[图片]';
    if (msg.videoSrc) return '[视频]';
    if (msg.abcNotation) return '[乐谱]';
    return '[消息]';
}

function truncateQuotePreview(text) {
    const runes = Array.from(String(text || ''));
    if (runes.length <= QUOTE_PREVIEW_LIMIT) return runes.join('');
    return runes.slice(0, QUOTE_PREVIEW_LIMIT).join('') + '…';
}

/** 消息 → 引用快照（随消息发送给服务端，作为模型上下文） */
function quoteSnapshotOfMessage(msg) {
    const audio = messageAudioAttachment(msg);
    return {
        id: msg.id || '',
        role: msg.role || 'user',
        sender: msg.role === 'user' ? (msg.sender || backendUserName) : ASSISTANT_NAME,
        content: msg.role === 'user' ? stripUserPrefix(msg.content || '') : (msg.content || ''),
        categories: msg.categories || [],
        audio_handle: (audio && audio.handle) || msg.audioHandle || '',
        text: (audio && audio.text) || ''
    };
}

/** 当前选择是否包含某条消息 */
function isMessageQuoted(id) {
    return quotedMessages.some(msg => msg && msg.id === id);
}

/** 选中 / 取消选中一条消息（QQ 多选语义：重复点击即取消） */
function toggleQuoteMessage(msg) {
    if (!msg) return;
    const index = quotedMessages.findIndex(item => item && item.id === msg.id);
    if (index >= 0) {
        quotedMessages.splice(index, 1);
    } else {
        if (quotedMessages.length >= MAX_QUOTED_MESSAGES) {
            showToast(`最多同时引用 ${MAX_QUOTED_MESSAGES} 条消息`, 'info');
            return;
        }
        quotedMessages.push(msg);
    }
    syncQuoteSelection();
    renderQuoteTray();
    if (messageInput) messageInput.focus();
}

/** 同步消息气泡的选中态 */
function syncQuoteSelection() {
    messageArea.querySelectorAll('.message').forEach(el => {
        el.classList.toggle('quote-selected', isMessageQuoted(el.dataset.id));
    });
}

/** 渲染输入框上方的引用条 */
function renderQuoteTray() {
    if (!quoteTray) return;
    quoteTrayList.innerHTML = '';
    if (!quotedMessages.length) {
        quoteTray.hidden = true;
        if (quoteTrayTitle) quoteTrayTitle.innerHTML = '<i class="fas fa-quote-left"></i> 引用消息';
        return;
    }
    quoteTray.hidden = false;
    if (quoteTrayTitle) {
        quoteTrayTitle.innerHTML = `<i class="fas fa-quote-left"></i> 引用消息（${quotedMessages.length}）`;
    }
    quotedMessages.forEach((msg, index) => {
        const chip = document.createElement('div');
        chip.className = 'quote-chip';
        const head = document.createElement('span');
        head.className = 'quote-chip-sender';
        head.textContent = (msg.role === 'user' ? (msg.sender || backendUserName) : ASSISTANT_NAME);
        const preview = document.createElement('span');
        preview.className = 'quote-chip-text';
        preview.textContent = quoteMessagePreview(msg);
        const remove = document.createElement('button');
        remove.className = 'quote-chip-remove';
        remove.title = '取消引用这条消息';
        remove.innerHTML = '<i class="fas fa-times"></i>';
        remove.addEventListener('click', () => {
            quotedMessages.splice(index, 1);
            syncQuoteSelection();
            renderQuoteTray();
        });
        chip.appendChild(head);
        chip.appendChild(preview);
        chip.appendChild(remove);
        quoteTrayList.appendChild(chip);
    });
}

/** 清空全部引用 */
function clearQuotedMessages() {
    quotedMessages = [];
    syncQuoteSelection();
    renderQuoteTray();
}

/** 构造发送用引用内容块；无引用时返回 null */
function buildQuoteContentBlock() {
    if (!quotedMessages.length) return null;
    return {
        type: 'quote',
        quote: { messages: quotedMessages.map(quoteSnapshotOfMessage) }
    };
}

/** 把服务端返回的音频悬空句柄回填到刚发送的本地消息 */
function applyAudioHandlesToMessage(messageId, handles) {
    if (!messageId || !Array.isArray(handles) || !handles.length) return;
    const msg = messages.find(item => item && item.id === messageId);
    if (!msg || !msg.attachments) return;
    let cursor = 0;
    msg.attachments.forEach(att => {
        if (att && att.type === 'audio' && cursor < handles.length) {
            const handle = handles[cursor];
            if (handle) att.handle = handle;
            cursor++;
        }
    });
}

/** 引用条交互与快捷键 */
function setupQuoteEvents() {
    if (quoteClearBtn) quoteClearBtn.addEventListener('click', clearQuotedMessages);
    document.addEventListener('keydown', (e) => {
        if (e.key === 'Escape' && quotedMessages.length) clearQuotedMessages();
    });
}
