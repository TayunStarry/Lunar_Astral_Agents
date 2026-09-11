// ============================================================
//  星月智能 · 消息终端 — 用户发言跳转 / 滚动控制 / 靠近显示
// ============================================================

// ---------- 用户发言跳转 ----------
function summarizeUserMessage(msg) {
    if (msg.content && msg.content.trim()) {
        const text = msg.content.replace(/\s+/g, ' ').trim();
        return text.length > 30 ? text.slice(0, 30) + '…' : text;
    }
    if (msg.attachments && msg.attachments.length) {
        const imgCount = msg.attachments.filter(a => a.type === 'image').length;
        if (imgCount) return `[图片 ×${imgCount}]`;
        return '[附件]';
    }
    if (msg.imageSrc) return '[图片]';
    return '[无文本]';
}

function renderUserJumpList() {
    userJumpList.innerHTML = '';
    const userMsgs = messages.filter(m => m.role === 'user');
    if (!userMsgs.length) {
        userJumpList.innerHTML = '<div class="user-jump-empty">暂无用户发言</div>';
        return;
    }
    userMsgs.forEach(m => {
        const item = document.createElement('button');
        item.className = 'user-jump-item';
        const time = m.timestamp
            ? new Date(m.timestamp).toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' })
            : '';
        item.innerHTML = `<span class="user-jump-time">${escapeHtml(time)}</span><span class="user-jump-summary">${escapeHtml(summarizeUserMessage(m))}</span>`;
        item.addEventListener('click', () => {
            jumpToMessage(m.id);
            closeUserJumpPanel();
        });
        userJumpList.appendChild(item);
    });
}

function jumpToMessage(id) {
    const el = messageArea.querySelector(`.message[data-id="${id}"]`);
    if (!el) return;
    el.style.display = ''; // 即使被标签/搜索过滤隐藏，也临时显示目标消息
    el.scrollIntoView({ block: 'start', behavior: 'smooth' });
    flashMessageBorder(el);
}

// 边框闪烁（直接操作 box-shadow，不触发消息入场动画，避免气泡跳动）
function flashMessageBorder(el) {
    if (el._flashTimer) {
        clearInterval(el._flashTimer);
        el._flashTimer = null;
    }
    let on = true;
    let count = 0;
    el.style.boxShadow = '0 0 0 3px rgba(157, 107, 255, 0.85)';
    el._flashTimer = setInterval(() => {
        on = !on;
        count++;
        el.style.boxShadow = on ? '0 0 0 3px rgba(157, 107, 255, 0.85)' : '0 0 0 0 rgba(157, 107, 255, 0)';
        if (count >= 6) {
            clearInterval(el._flashTimer);
            el._flashTimer = null;
            el.style.boxShadow = '';
        }
    }, 220);
}

function openUserJumpPanel() {
    renderUserJumpList();
    userJumpPanel.hidden = false;
    jumpUserBtn.classList.add('active');
}

function closeUserJumpPanel() {
    userJumpPanel.hidden = true;
    jumpUserBtn.classList.remove('active');
}

function toggleUserJumpPanel() {
    if (userJumpPanel.hidden) openUserJumpPanel();
    else closeUserJumpPanel();
}

// ---------- 右下角控制区收起 / 展开 ----------
/** 左侧矩形按钮切换控制区内容的显隐（内容从右向左滑入/滑出） */
function setupCornerToggle() {
    const open = () => {
        cornerContent.classList.add('open');
        cornerToggle.classList.add('active');
    };
    const close = () => {
        cornerContent.classList.remove('open');
        cornerToggle.classList.remove('active');
    };

    cornerToggle.addEventListener('click', () =>
        cornerContent.classList.contains('open') ? close() : open()
    );
}

function setupScrollControls() {
    scrollTopBtn.addEventListener('click', () => scrollToTop(true));
    scrollBottomBtn.addEventListener('click', () => scrollToBottom(true));
    jumpUserBtn.addEventListener('click', toggleUserJumpPanel);
    document.addEventListener('click', (e) => {
        if (!userJumpPanel.hidden && !userJumpPanel.contains(e.target) && e.target !== jumpUserBtn && !jumpUserBtn.contains(e.target)) {
            closeUserJumpPanel();
        }
    });
    document.addEventListener('keydown', (e) => {
        if (e.key === 'Escape' && !userJumpPanel.hidden) closeUserJumpPanel();
    });
}
