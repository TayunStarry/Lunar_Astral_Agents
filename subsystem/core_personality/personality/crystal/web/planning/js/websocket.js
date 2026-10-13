// ==== WebSocket ====
function wsUrl() { const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:'; return proto + '//' + window.location.host + '/ws?bus=1'; }
function connect() {
    try { state.ws = new WebSocket(wsUrl()); } catch (e) { scheduleReconnect(); return; }
    state.ws.onopen = () => { state.connected = true; state.retry = 0; renderWs(); toast('已连接引擎', 'success'); send({ type: 'ltp9/probe' }, '探测'); };
    state.ws.onmessage = ev => handleMessage(ev.data);
    state.ws.onclose = () => {
        state.connected = false; renderWs(); scheduleReconnect();
        // 后端重启/断线会丢失运行实例（bp_done 不会到来），复位运行态
        if (state.running) { state.running = false; state.bpRunId = null; setRunBtn(false); }
    };
    state.ws.onerror = () => { try { state.ws.close(); } catch (e) {} };
}
function scheduleReconnect() { if (state.retry < WS_MAX_RETRY) { state.retry++; renderWs(); setTimeout(connect, WS_RETRY_INTERVAL); } else $('wsText').textContent = '连接失败'; }
function renderWs() {
    const dot = $('wsDot'), txt = $('wsText');
    if (state.connected) { dot.className = 'status-dot on'; txt.textContent = '已连接'; }
    else if (state.retry > 0 && state.retry < WS_MAX_RETRY) { dot.className = 'status-dot off'; txt.textContent = `重连中(${state.retry}/${WS_MAX_RETRY})`; }
    else if (state.retry >= WS_MAX_RETRY) { dot.className = 'status-dot off'; txt.textContent = '连接失败'; }
    else { dot.className = 'status-dot'; txt.textContent = '未连接'; }
}

// ==== 信封发送 / 等待 ====
function send(env, label) {
    if (!state.ws || state.ws.readyState !== WebSocket.OPEN) { toast('尚未连接引擎', 'error'); return null; }
    env.request_id = seg();
    const p = new Promise((resolve) => state.pending.set(env.request_id, { resolve }));
    addLog({ dir: 'send', type: env.type, label: label || '', req: env.request_id, env, isError: false });
    try { state.ws.send(JSON.stringify(env)); } catch (e) { toast('发送失败: ' + e.message, 'error'); state.pending.delete(env.request_id); return null; }
    return p;
}
// 等待回执并带超时：仅在真正收到匹配 request_id 的 ack 时才算完成（默认最长 5 分钟，供画图等慢操作）
function sendWait(env, label, ms) {
    const p = send(env, label);
    if (!p) return Promise.resolve(null);
    return new Promise(res => {
        const t = setTimeout(() => { state.pending.delete(env.request_id); res(null); }, ms || 300000);
        p.then(v => { clearTimeout(t); res(v); });
    });
}
function handleMessage(raw) {
    let m; try { m = JSON.parse(raw); } catch (e) { return; }
    if (!m || typeof m.type !== 'string') return;
    const isAck = !!ACK_TYPES[m.type];
    if (m.request_id && state.pending.has(m.request_id)) {
        if (isAck) { const p = state.pending.get(m.request_id); state.pending.delete(m.request_id); p.resolve(m); }
        else { return; } // 自己发送的请求被集线器回显，不是回执，忽略
    }
    if (!m.type.startsWith('ltp9/')) return;
    // 蓝图执行推送（bp_node/bp_exec/bp_done）：委托后端执行客户端处理（bp_ack 已由 pending 机制消费）
    if (m.type.startsWith('ltp9/bp_')) { handleBpMessage(m); return; }
    // 引擎真实注册信息（probe/stats 回执携带 plugins/agent_plugins）：刷新下拉动态选项
    if (Array.isArray(m.plugins) || Array.isArray(m.agent_plugins)) updateCatalog(m.plugins, m.agent_plugins);
    const sm = summarize(m);
    addLog({ dir: isAck ? 'recv' : 'recv', type: m.type, label: '', req: m.request_id || '', env: m, isError: sm.isError, summary: sm.text });
}
// updateCatalog 用引擎回执的插件状态填充动态选项：LTP9包ID / 已订阅事件主题 / 各包导出函数 / Mini-LTP·Node-LTP 智能体包
function updateCatalog(plugins, agentPlugins) {
    const c = { plugins: [], events: [], exports: {} };
    const seen = new Set();
    (plugins || []).forEach(pl => {
        if (!pl || !pl.id) return;
        c.plugins.push(pl.id);
        c.exports[pl.id] = (pl.exports || []).slice().sort();
        (pl.events || []).forEach(t => seen.add(t));
    });
    c.plugins.sort();
    c.events = [...seen].sort();
    c.agentPlugins = (agentPlugins || []).slice().sort();
    state.catalog = c;
    // 浮窗打开中：重渲染参数表单，让新选项立即生效
    if (state.modalNodeId) { const n = nodeById(state.modalNodeId); if (n) openNodeModal(n); }
}
function summarize(m) {
    let text = m.type.replace('ltp9/', ''); let isError = false;
    switch (m.type) {
        case 'ltp9/result': {
            const r = m.result;
            if (r && r.error) { text = 'LTP9 错误: ' + r.error; isError = true; }
            else if (r && r.value && r.value.ok === false) { text = '调用失败: ' + (r.value.text || safeJson(r.value)); isError = true; }
            else if (r && r.summary) text = `事件回执 subscribed=${r.summary.subscribed}` + (r.summary.intercepted ? ' · intercepted' : '') + (r.summary.canceled ? ' · canceled' : '');
            else text = '结果: ' + safeJson(r);
            break;
        }
        case 'ltp9/test_ack': {
            const a = m.action || '';
            if (m.ok === false) { text = `测试[${a}] 失败: ${m.error || ''}`; isError = true; }
            else text = `测试[${a}]: ` + safeJson(m.value);
            break;
        }
        case 'ltp9/stats_ack': case 'ltp9/pong':
            text = '引擎在线 · 插件 ' + (m.plugins && m.plugins.length ? m.plugins.length : '?') + ' 个'; break;
        case 'ltp9/broadcast_ack': text = '广播已发送'; break;
        default: text = safeJson(m);
    }
    return { text, isError };
}
function safeJson(v) { try { const s = JSON.stringify(v); return s === undefined ? String(v) : (s.length > 220 ? s.slice(0, 220) + '…' : s); } catch (e) { return String(v); } }
function escHtml(s) { return String(s).replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c])); }
// （coreOut/extractOut 已随调度循环迁移至引擎 blueprint_nodes.go 的 bpCoreOut，前端不再需要）