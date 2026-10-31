// ============================================================
//  星月智能 · 消息终端 — 消息元素渲染（分类徽标 / 操作按钮 / 主体结构）
// ============================================================

// ---------- 消息渲染 ----------
const CATEGORY_META = {
    text: { label: '文本', icon: 'fa-align-left', cls: 'badge-text' },
    file: { label: '文件', icon: 'fa-file-lines', cls: 'badge-file' },
    image: { label: '图片', icon: 'fa-image', cls: 'badge-image' },
    voice: { label: '语音', icon: 'fa-volume-high', cls: 'badge-voice' },
    music: { label: '乐谱', icon: 'fa-music', cls: 'badge-music' },
    action: { label: '行动', icon: 'fa-person-running', cls: 'badge-action' }
};

/**
 * 从用户消息正文中拆出资源引用句柄，返回纯文本与引用条目
 *
 * 前端只下发规范句柄（@D.003 / @I.007 …），用户气泡里的句柄应渲染为
 * 可点击的引用按钮，并显示句柄对应的文件名 —— 用户需要一眼看出
 * "这句话是对哪份资料说的"，而不是看到一串裸文本。
 *
 * @param content 用户消息正文
 * @returns plain：去掉引用标记后的正文；refs：按出现顺序的引用条目
 */
function extractUserRefs(content) {
    const pattern = /@[DIVAGTU]\.\d{3,}(?:\|(?:§\d+|L\d+-\d+))?/g;
    const refs = [];
    const plain = String(content || '').replace(pattern, match => {
        refs.push({ id: match, label: match.slice(1).replace(/\|\S+$/, '') });
        return '';
    });
    return { plain: plain.replace(/\s+/g, ' ').trim(), refs };
}

/** 构建行内引用按钮（用户气泡内联展示；点击把该引用载入输入框） */
function buildInlineRefPill(ref) {
    const pill = document.createElement('span');
    // 复用既有的行内引用样式（与历史上 [#file.ext] 内联引用同一套视觉）
    pill.className = 'file-ref-inline';
    pill.title = `点击载入引用 ${ref.id}`;
    const icon = document.createElement('i');
    icon.className = 'fas fa-file-alt';
    const name = document.createElement('span');
    // 展示句柄对应的文件名；解析完成前先显示句柄，避免空白
    name.textContent = resourceNameCache.get(baseResourceHandle(ref.id)) || ref.id;
    name.title = ref.id;
    resolveResourceName(ref.id).then(label => {
        if (label) name.textContent = label;
    });
    pill.appendChild(icon);
    pill.appendChild(name);
    pill.addEventListener('click', () => addFileReference(ref.id));
    return pill;
}

/**
 * 渲染用户消息正文：文件引用渲染为按钮，其余文本按 Markdown 渲染
 *
 * @param el 消息元素
 * @param content 用户消息正文
 * @returns 渲染完成的 Promise
 */
function fillUserMessageContent(el, content) {
    const contentDiv = el.querySelector('.markdown-content');
    if (!contentDiv) return Promise.resolve(el);
    const { plain, refs } = extractUserRefs(content);
    if (refs.length === 0) {
        if (!plain) return Promise.resolve(el);
        return fillMarkdownContent(el, plain);
    }
    const line = document.createElement('div');
    line.className = 'message-ref-line';
    refs.forEach(ref => line.appendChild(buildInlineRefPill(ref)));
    if (plain) {
        const textSpan = document.createElement('span');
        textSpan.className = 'message-ref-text';
        textSpan.textContent = plain;
        line.appendChild(textSpan);
    }
    contentDiv.appendChild(line);
    return Promise.resolve(el);
}

function buildCategoryBadges(categories) {
    return (categories || []).map(cat => {
        const meta = CATEGORY_META[cat];
        if (!meta) return '';
        return `<span class="message-category-badge ${meta.cls}"><i class="fas ${meta.icon}"></i>${meta.label}</span>`;
    }).join('');
}

function buildActionsPanel(msg) {
    const panel = document.createElement('div');
    panel.className = 'message-actions-panel';
    const quoteBtn = document.createElement('button');
    quoteBtn.className = 'chat-action-button quote_message_button';
    quoteBtn.title = '引用这条消息';
    quoteBtn.innerHTML = '<i class="fas fa-quote-left"></i>';
    quoteBtn.addEventListener('click', () => toggleQuoteMessage(msg));
    panel.appendChild(quoteBtn);
    const copyBtn = document.createElement('button');
    copyBtn.className = 'chat-action-button copy_message_button';
    copyBtn.title = '复制';
    copyBtn.innerHTML = '<i class="fas fa-copy"></i>';
    copyBtn.addEventListener('click', () => copyMessage(msg));
    panel.appendChild(copyBtn);
    return panel;
}

/** 构建消息气泡内的引用块（QQ 风格：发送者 + 内容预览，支持多条） */
function buildQuoteBlock(quotes) {
    const wrap = document.createElement('div');
    wrap.className = 'message-quote';
    const header = document.createElement('div');
    header.className = 'message-quote-header';
    header.innerHTML = `<i class="fas fa-quote-left"></i> 引用 ${quotes.length} 条消息`;
    wrap.appendChild(header);
    quotes.forEach(quoted => {
        const item = document.createElement('div');
        item.className = 'message-quote-item';
        const sender = document.createElement('span');
        sender.className = 'message-quote-sender';
        sender.textContent = quoted.sender || (quoted.role === 'assistant' ? ASSISTANT_NAME : backendUserName);
        const text = document.createElement('span');
        text.className = 'message-quote-text';
        let preview = quoted.text ? '[语音] ' + quoted.text : (quoted.content || '');
        preview = String(preview).replace(/\s+/g, ' ').trim();
        if (!preview) preview = quoted.categories && quoted.categories.includes('image') ? '[图片]' : '[消息]';
        text.textContent = preview.length > 120 ? preview.slice(0, 120) + '…' : preview;
        item.appendChild(sender);
        item.appendChild(text);
        if (quoted.id) {
            item.title = '点击定位到被引用的消息';
            item.addEventListener('click', () => {
                if (typeof jumpToMessage === 'function') jumpToMessage(quoted.id);
            });
        }
        wrap.appendChild(item);
    });
    return wrap;
}

/** 仅用于前端展示：隐藏缓存正文里的 `[用户: 名字]:` 前缀（模型上下文仍保留） */
function stripUserPrefix(text) {
    return String(text || '').replace(/^\[用户:\s*[^\]]*\]\s*[:：]\s*/u, '');
}

function computeSearchText(msg) {
    const parts = [];
    if (msg.sender) parts.push(msg.sender);
    if (msg.content) parts.push(msg.role === 'user' ? stripUserPrefix(msg.content) : msg.content);
    if (msg.imageLabel) parts.push(msg.imageLabel);
    if (msg.abcNotation) parts.push(msg.abcNotation);
    if (msg.attachments && msg.attachments.length) {
        msg.attachments.forEach(att => { if (att.label) parts.push(att.label); });
    }
    return parts.join(' ').toLowerCase();
}

function renderMessageElement(msg) {
    const el = document.createElement('div');
    el.className = `message ${msg.role === 'user' ? 'user-message' : 'assistant-message'}`;
    el.dataset.id = msg.id;
    el.dataset.categories = (msg.categories || ['text']).join(',');
    el.dataset.searchText = computeSearchText(msg);
    if (msg.categories && msg.categories.includes('action')) el.classList.add('action-message');
    if (typeof isMessageQuoted === 'function' && isMessageQuoted(msg.id)) el.classList.add('quote-selected');

    // 用户气泡名优先用缓存里的实际发送者（QQ 私聊昵称 / 前端配置名）；正文不带前缀
    const displayName = msg.role === 'user' ? (msg.sender || backendUserName) : ASSISTANT_NAME;
    const header = document.createElement('div');
    header.className = 'message-header';
    header.innerHTML = `
        <span class="header-name">${escapeHtml(displayName)}</span>
        ${buildCategoryBadges(msg.categories)}
        <span class="header-time">${msg.timestamp ? new Date(msg.timestamp).toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit', second: '2-digit' }) : getTimeString()}</span>
    `;
    el.appendChild(header);

    // 引用块（被引用消息快照，支持多条）
    if (msg.quotes && msg.quotes.length) el.appendChild(buildQuoteBlock(msg.quotes));

    // 图片块
    if (msg.imageSrc) el.appendChild(buildImageBlock(msg));

    // 视频块
    if (msg.videoSrc) el.appendChild(buildVideoBlock(msg));

    // 音频文件块
    if (msg.audioSrc) el.appendChild(buildAudioFileBlock(msg));

    // 文本内容容器
    const contentDiv = document.createElement('div');
    contentDiv.className = 'markdown-content';
    el.appendChild(contentDiv);

    // 资源收录引用图标（按钮）：点击把句柄载入输入框
    if (msg.fileRef) el.appendChild(buildFileRefBlock(msg.fileRef, msg.fileLabel));

    // 多附件（用户拖入的图片/视频/音频）
    if (msg.attachments && msg.attachments.length) {
        msg.attachments.forEach(att => {
            const block = buildAttachmentBlock(att);
            if (block) el.appendChild(block);
        });
    }

    // 乐谱卡片
    if (msg.abcNotation) el.appendChild(buildMusicCard(msg));

    // 语音重播按钮
    if (msg.audio) el.appendChild(buildAudioReplay(msg));

    // 操作按钮
    el.appendChild(buildActionsPanel(msg));

    messageArea.appendChild(el);

    // 用户消息：文件引用渲染为可点击按钮，其余文本按 Markdown 渲染（展示时隐藏发送者前缀）
    if (msg.role === 'user' && msg.content) return fillUserMessageContent(el, stripUserPrefix(msg.content));
    // 助手消息：正文已被服务端置换为句柄 + 文件名，连同收录图标一起展示
    if (msg.content) return fillMarkdownContent(el, msg.content);
    return Promise.resolve(el);
}
