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

// ---------- 靠近显示机制 ----------
/** 靠近显示：默认隐藏，鼠标接近元素时显示，离开 1.5 秒后隐藏 */
function setupProximityReveal() {
    const targets = [topControls, scrollControls];
    const THRESHOLD = 120;   // 靠近阈值（像素）
    const HIDE_DELAY = 1500; // 离开后隐藏延迟（毫秒）
    const timers = new Map();

    const reveal = (el) => el.classList.add('revealed');
    const conceal = (el) => {
        el.classList.remove('revealed');
        timers.delete(el);
    };
    const scheduleHide = (el) => {
        if (timers.has(el)) return;
        timers.set(el, setTimeout(() => conceal(el), HIDE_DELAY));
    };

    // 鼠标移动：靠近元素显示位置则滑入，离开则开始滑出计时
    document.addEventListener('mousemove', (e) => {
        for (const el of targets) {
            // 元素显示位置的视口坐标（offset 系列不受 translateX 位移影响，保证隐藏时检测基准不变）
            const pr = (el.offsetParent || document.body).getBoundingClientRect();
            const left = pr.left + el.offsetLeft;
            const top = pr.top + el.offsetTop;
            const right = left + el.offsetWidth;
            const bottom = top + el.offsetHeight;
            // 鼠标到元素包围盒的最近距离（鼠标在盒内时为 0）
            const dx = Math.max(left - e.clientX, 0, e.clientX - right);
            const dy = Math.max(top - e.clientY, 0, e.clientY - bottom);
            const dist = Math.hypot(dx, dy);
            if (dist < THRESHOLD) {
                clearTimeout(timers.get(el));
                timers.delete(el);
                reveal(el);
            } else {
                scheduleHide(el);
            }
        }
    });

    // 搜索框聚焦时保持顶部控制区显示（纯键盘搜索不受影响）
    const searchInput = document.getElementById('searchInput');
    if (searchInput) {
        searchInput.addEventListener('focusin', () => {
            clearTimeout(timers.get(topControls));
            timers.delete(topControls);
            reveal(topControls);
        });
        searchInput.addEventListener('focusout', () => scheduleHide(topControls));
    }

    // 用户跳转面板打开时保持滚动控制区显示
    document.addEventListener('click', (e) => {
        if (!userJumpPanel.hidden) {
            clearTimeout(timers.get(scrollControls));
            timers.delete(scrollControls);
            reveal(scrollControls);
        } else {
            scheduleHide(scrollControls);
        }
    });
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
