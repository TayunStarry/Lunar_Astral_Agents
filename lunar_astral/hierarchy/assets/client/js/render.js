// ============================================================
//  星月智能 · 消息终端 — 消息元素渲染（分类徽标 / 操作按钮 / 主体结构）
// ============================================================

// ---------- 消息渲染 ----------
const CATEGORY_META = {
    text: { label: '文本', icon: 'fa-align-left', cls: 'badge-text' },
    image: { label: '图片', icon: 'fa-image', cls: 'badge-image' },
    voice: { label: '语音', icon: 'fa-volume-high', cls: 'badge-voice' },
    music: { label: '乐谱', icon: 'fa-music', cls: 'badge-music' },
    action: { label: '行动', icon: 'fa-person-running', cls: 'badge-action' }
};

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
    const copyBtn = document.createElement('button');
    copyBtn.className = 'chat-action-button copy_message_button';
    copyBtn.title = '复制';
    copyBtn.innerHTML = '<i class="fas fa-copy"></i>';
    copyBtn.addEventListener('click', () => copyMessage(msg));
    const delBtn = document.createElement('button');
    delBtn.className = 'chat-action-button delete_message_button';
    delBtn.title = '删除';
    delBtn.innerHTML = '<i class="fas fa-trash"></i>';
    delBtn.addEventListener('click', () => deleteMessage(msg.id));
    panel.appendChild(copyBtn);
    panel.appendChild(delBtn);
    return panel;
}

function computeSearchText(msg) {
    const parts = [];
    if (msg.content) parts.push(msg.content);
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

    const displayName = msg.role === 'user' ? USER_NAME : ASSISTANT_NAME;
    const header = document.createElement('div');
    header.className = 'message-header';
    header.innerHTML = `
        <span class="header-name">${escapeHtml(displayName)}</span>
        ${buildCategoryBadges(msg.categories)}
        <span class="header-time">${msg.timestamp ? new Date(msg.timestamp).toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit', second: '2-digit' }) : getTimeString()}</span>
    `;
    el.appendChild(header);

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

    // 文件导入引用图标（按钮）：点击载入输入框
    if (msg.fileRef) el.appendChild(buildFileRefBlock(msg.fileRef));

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

    if (msg.content && !msg.fileRef) return fillMarkdownContent(el, msg.content);
    return Promise.resolve(el);
}
