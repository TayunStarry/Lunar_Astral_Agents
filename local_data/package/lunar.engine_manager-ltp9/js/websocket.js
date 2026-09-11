// ==== WebSocket ====
function wsUrl() { const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:'; return proto + '//' + window.location.host + '/ws'; }
function connect() {
    try { state.ws = new WebSocket(wsUrl()); } catch (e) { scheduleReconnect(); return; }
    state.ws.onopen = () => { state.connected = true; state.retry = 0; renderWs(); toast('已连接引擎', 'success'); send({ type: 'ltp9/probe' }, '探测'); };
    state.ws.onmessage = ev => handleMessage(ev.data);
    state.ws.onclose = () => { state.connected = false; renderWs(); scheduleReconnect(); };
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
    // 引擎真实注册信息（probe/stats 回执携带 plugins/agent_plugins）：刷新下拉动态选项
    if (Array.isArray(m.plugins) || Array.isArray(m.agent_plugins)) updateCatalog(m.plugins, m.agent_plugins);
    const sm = summarize(m);
    addLog({ dir: isAck ? 'recv' : 'recv', type: m.type, label: '', req: m.request_id || '', env: m, isError: sm.isError, summary: sm.text });
}
// updateCatalog 用引擎回执的插件状态填充动态选项：LTP9包ID / 已订阅事件主题 / 各包导出函数 / Mini-LTP·Node-LTP 智能体包
function updateCatalog(plugins, agentPlugins) {
    const c = { plugins: [], events: [], exports: {}, tools: {} };
    const seen = new Set();
    (plugins || []).forEach(pl => {
        if (!pl || !pl.id) return;
        c.plugins.push(pl.id);
        c.exports[pl.id] = (pl.exports || []).slice().sort();
        c.tools[pl.id] = (pl.tools || []).slice().sort();
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
function extractOut(ack) {
    if (!ack) return null;
    if (ack.value !== undefined) return ack.value;
    if (ack.result !== undefined) return ack.result;
    return ack;
}
// 提取节点核心输出（供下游直接使用，无需每个节点后跟格式转换）：
//  - ltp9/test_ack：剥掉标准信封 + probeResult 的 {ok,value} 包装，直接给出要传递的数据
//  - ltp9/result（工具等回执）：取 result.value；ltp9/stats_ack：取 plugins
//  - 核心对象若含节点声明输出字段（out.k）或引擎约定主值字段（text/body），直接传裸值（字符串等），
//    如 qwen_asr{text} → 纯文本、kokoro_tts{audio} → 纯 base64 音频、llm{text} → 纯应答
//  - 失败信封原样保留，便于在日志/详情中看到错误
//  - 事件触发(event)/跨包调用(call)/插件状态(stats)：回执非明确结果格式，输出原始信息不简化
function coreOut(ack, node) {
    if (!ack) return null;
    if (node && (node.type === 'event' || node.type === 'call' || node.type === 'stats')) return ack;
    if (ack.ok === false) return ack;
    let v = ack;
    if (ack.type === 'ltp9/test_ack') v = ack.value;
    else if (ack.type === 'ltp9/result') v = (ack.result && typeof ack.result === 'object' && 'value' in ack.result) ? ack.result.value : (ack.result !== undefined ? ack.result : ack);
    else if (ack.type === 'ltp9/stats_ack') v = ack.plugins !== undefined ? ack.plugins : ack;
    let guard = 0;
    while (v && typeof v === 'object' && !Array.isArray(v) && v.ok === true && 'value' in v && guard++ < 8) v = v.value;
    if (v && typeof v === 'object' && !Array.isArray(v)) {
        // 事件触发节点：订阅器 return 的业务结果优先，其次改写后的 data
        if (node && node.type === 'event' && (v.returned || v.data !== undefined)) {
            v = (v.returned && v.return !== undefined) ? v.return : v.data;
        } else {
            const meta = metaOf(node ? node.type : '');
            const cand = [];
            if (meta.out && meta.out.k && meta.out.k !== 'gate') cand.push(meta.out.k);
            cand.push('text', 'body'); // rwResult(text) 与 http {status,body} 引擎约定
            for (const k of cand) {
                if (k in v && (typeof v[k] === 'string' || typeof v[k] === 'number') && v[k] !== '') { v = v[k]; break; }
            }
        }
    }
    return v;
}