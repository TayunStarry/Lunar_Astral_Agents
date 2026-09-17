// ==== 回显日志 + 日志侧边栏 ====
function addLog(entry) { entry.time = new Date(); state.log.unshift(entry); if (state.log.length > 400) state.log.pop(); state.unread++; updateFab(); renderLog(); }
function updateFab() { const fab = $('logFab'), cnt = $('logCount'); if (state.unread > 0) fab.classList.add('glow'); else fab.classList.remove('glow'); cnt.textContent = state.unread > 99 ? '99+' : String(state.unread); }
function closeLogModal() { $('logSidebar').classList.remove('open'); }
function toggleLogSidebar() { const s = $('logSidebar'); s.classList.toggle('open'); if (s.classList.contains('open')) { closeNodeModalPanel(); state.unread = 0; updateFab(); renderLog(); } } // 两个侧栏互斥，打开日志时收起节点模块

function renderLog() {
    const list = $('logList'); list.innerHTML = '';
    const items = state.log.filter(en => {
        if (state.filter === 'all') return true;
        if (state.filter === 'send') return en.dir === 'send';
        if (state.filter === 'recv') return en.dir === 'recv';
        if (state.filter === 'error') return en.isError;
        return true;
    });
    if (!items.length) { list.innerHTML = '<div class="empty-tip">暂无日志</div>'; return; }
    items.forEach(en => {
        const el = document.createElement('div'); el.className = 'log-entry' + (en.isError ? ' error' : '');
        const head = document.createElement('div'); head.className = 'log-head';
        const badge = document.createElement('span'); badge.className = 'log-type ' + (en.isError ? 't-error' : (en.dir === 'send' ? 't-send' : 't-response'));
        badge.textContent = en.type; head.appendChild(badge);
        const time = document.createElement('span'); time.className = 'log-time';
        const d = en.time; time.textContent = `${String(d.getHours()).padStart(2,'0')}:${String(d.getMinutes()).padStart(2,'0')}:${String(d.getSeconds()).padStart(2,'0')}`; head.appendChild(time);
        const tag = document.createElement('span'); tag.className = 'log-tag'; tag.textContent = en.dir === 'send' ? '发送' : '回执'; head.appendChild(tag);
        el.appendChild(head);
        const sum = document.createElement('div'); sum.className = 'log-summary'; sum.textContent = en.summary || (en.label ? en.label + ' · ' + en.type : en.type); el.appendChild(sum);
        const hint = document.createElement('div'); hint.className = 'log-json-hint'; hint.innerHTML = '<i class="fas fa-expand"></i> 查看 JSON'; el.appendChild(hint);
        el.addEventListener('click', () => openDetail('日志详情', en.env || en));
        list.appendChild(el);
    });
}