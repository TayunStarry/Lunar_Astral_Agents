// ============================================================
// 『 星月智能 』LTP3 引擎管理器
// 左栏：蛇形节点网格（上方）+ 选中节点详情（下方）
// 右栏：引擎状态 / 插件列表 / 引擎回显卡片
// 协议：/ws 集线器 ltp3/* 信封（见 engine/YaraLTP/docs/client-dev-guide.md）
// ============================================================

// ==== LTP3 协议枚举（权威源：crystal_astral/agent/YaraLTP/variable.go） ====
const YARA_HOOKS = [
    'chat.receive.before_process', 'chat.receive.after_process',
    'chat.command.before_execute', 'chat.command.after_execute',
    'emoji.chat.before_select', 'emoji.chat.after_select',
    'emoji.register.after_build_description', 'emoji.register.after_build_emotion',
    'send_service.after_build_message', 'send_service.before_send', 'send_service.after_send',
    'chat.planner.before_request', 'chat.planner.after_response',
    'chat.replyer.before_request', 'chat.replyer.before_model_request', 'chat.replyer.after_response',
    'jargon.query.before_search', 'jargon.query.after_search',
    'jargon.extract.before_persist', 'jargon.inference.before_finalize',
    'expression.select.before_select', 'expression.select.after_selection',
    'expression.learn.after_extract', 'expression.learn.before_upsert'
];

const YARA_EVENTS = [
    'ON_START', 'ON_STOP', 'ON_MESSAGE_PRE_PROCESS', 'ON_MESSAGE', 'ON_PLAN',
    'POST_LLM', 'AFTER_LLM', 'POST_SEND_PRE_PROCESS', 'POST_SEND', 'AFTER_SEND'
];

const MANAGE_ACTIONS = ['list', 'scan', 'reload', 'reload_one', 'unload_one'];

// 模块元信息（类型 → 展示名 / 图标 / 主题色 / 说明）
const NODE_META = {
    hook:    { label: '钩子', icon: 'fa-link',            color: '#7db4ff', desc: '在钩子点发布消息' },
    event:   { label: '事件', icon: 'fa-bolt',            color: '#d6a7ff', desc: '发布系统/自定义事件' },
    command: { label: '指令', icon: 'fa-terminal',        color: '#5eead4', desc: '调用插件注册的指令' },
    tool:    { label: '工具', icon: 'fa-wrench',          color: '#fcd34d', desc: '调用插件注册的工具' },
    manage:  { label: '管理', icon: 'fa-cog',             color: '#fca5a5', desc: '引擎/插件管理动作' },
    ping:    { label: '探测', icon: 'fa-heartbeat',       color: '#67e8f9', desc: '探测引擎在线状态' },
    delay:   { label: '等待', icon: 'fa-hourglass-half',  color: '#cbd5e1', desc: '流程中等待指定毫秒' }
};

const FLOW_KEY = 'lunar.engine_manager.flow.v1';
const WS_MAX_RETRY = 5;
const WS_RETRY_INTERVAL = 3000;

// ==== DOM 引用 ====
const $ = (id) => document.getElementById(id);
const nodeGrid = $('nodeGrid');
const flowEmpty = $('flowEmpty');
const flowStatus = $('flowStatus');
const detailModal = $('detailModal');
const detailModalTop = $('detailModalTop');
const detailModalContent = $('detailModalContent');
const palette = $('palette');
const runFlowBtn = $('runFlowBtn');
const saveFlowBtn = $('saveFlowBtn');
const loadFlowBtn = $('loadFlowBtn');
const clearFlowBtn = $('clearFlowBtn');
const reconnectBtn = $('reconnectBtn');
const pingBtn = $('pingBtn');
const listBtn = $('listBtn');
const scanBtn = $('scanBtn');
const reloadAllBtn = $('reloadAllBtn');
const wsDot = $('wsDot');
const wsState = $('wsState');
const engineName = $('engineName');
const pluginCount = $('pluginCount');
const pluginCountBadge = $('pluginCountBadge');
const pluginList = $('pluginList');
const logFilters = $('logFilters');
const clearLogBtn = $('clearLogBtn');
const logList = $('logList');
const flowSaved = $('flowSaved');
const toastWrap = $('toastWrap');

// ==== 状态 ====
const state = {
    nodes: [],          // 流程节点
    selectedId: null,   // 当前选中节点 id（详情面板只显示它）
    ws: null,
    wsRetry: 0,
    connected: false,
    running: false,
    seq: 0,
    requests: new Map(), // request_id → { label, time }（用于回执配对）
    log: [],             // 回显日志
    filter: 'all',       // all / recv / send / error
    plugins: []
};

// ==== 通用工具 ====
function toast(msg, type) {
    const el = document.createElement('div');
    el.className = 'toast' + (type ? ' ' + type : '');
    el.textContent = msg;
    toastWrap.appendChild(el);
    setTimeout(() => {
        el.style.transition = 'opacity 0.3s';
        el.style.opacity = '0';
        setTimeout(() => el.remove(), 300);
    }, 2200);
}

function sleep(ms) {
    return new Promise(resolve => setTimeout(resolve, ms));
}

function parseJSON(text, label) {
    const t = (text || '').trim();
    if (!t) return {};
    try {
        return JSON.parse(t);
    } catch (e) {
        toast((label || 'JSON') + ' 解析失败: ' + e.message, 'error');
        return undefined;
    }
}

function newRequestId() {
    return 'em_' + Date.now() + '_' + (++state.seq);
}

// ==== WebSocket 客户端（/ws 集线器） ====
function wsUrl() {
    const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    return proto + '//' + window.location.host + '/ws';
}

function connect() {
    try {
        state.ws = new WebSocket(wsUrl());
    } catch (e) {
        console.error('WebSocket 创建失败:', e);
        scheduleReconnect();
        return;
    }
    state.ws.onopen = () => {
        state.connected = true;
        state.wsRetry = 0;
        updateStatusUI();
        toast('已连接 LTP3 引擎', 'success');
        sendEnvelope({ type: 'ltp3/ping' }, '探测');
        sendEnvelope({ type: 'ltp3/manage', action: 'list' }, '插件列表');
    };
    state.ws.onmessage = (ev) => handleMessage(ev.data);
    state.ws.onclose = () => {
        state.connected = false;
        updateStatusUI();
        scheduleReconnect();
    };
    state.ws.onerror = () => {
        try { state.ws.close(); } catch (e) { }
    };
}

function scheduleReconnect() {
    if (state.wsRetry < WS_MAX_RETRY) {
        state.wsRetry++;
        updateStatusUI();
        setTimeout(connect, WS_RETRY_INTERVAL);
    } else {
        wsState.textContent = '连接失败';
    }
}

// 发送 ltp3/* 信封（自动附加 request_id 并记录用于回执配对）
function sendEnvelope(partial, label) {
    if (!state.ws || state.ws.readyState !== WebSocket.OPEN) {
        toast('尚未连接引擎', 'error');
        return false;
    }
    const request_id = newRequestId();
    const env = Object.assign({ request_id }, partial);
    state.requests.set(request_id, { label: label || partial.type, time: Date.now() });
    addLog({
        dir: 'send',
        type: partial.type,
        req: request_id,
        isError: false,
        data: env,
        summary: summarizeOut(env, label)
    });
    try {
        state.ws.send(JSON.stringify(env));
    } catch (e) {
        toast('发送失败: ' + e.message, 'error');
        return false;
    }
    return true;
}

// ==== 消息处理（引擎 → 客户端） ====
function handleMessage(raw) {
    let m;
    try { m = JSON.parse(raw); } catch (e) { return; }
    if (!m || typeof m.type !== 'string' || !m.type.startsWith('ltp3/')) return;

    const own = m.request_id ? state.requests.get(m.request_id) : null;
    const sm = summarizeIn(m);
    addLog({
        dir: 'recv',
        type: m.type,
        req: m.request_id || '',
        isOwn: !!own,
        isError: sm.isError,
        data: m,
        summary: sm.text
    });

    switch (m.type) {
        case 'ltp3/pong':
            engineName.textContent = m.engine || 'LTP3';
            pluginCount.textContent = m.plugins != null ? m.plugins : '—';
            break;
        case 'ltp3/manage_ack':
            if (!m.ok) toast('管理动作失败: ' + (m.message || m.action), 'error');
            if (Array.isArray(m.plugins)) renderPlugins(m.plugins);
            break;
        case 'ltp3/lifecycle':
            // 插件加载/卸载广播 → 自动刷新插件列表
            sendEnvelope({ type: 'ltp3/manage', action: 'list' }, '插件列表(刷新)');
            break;
    }
}

// 出站信封的摘要
function summarizeOut(env, label) {
    const t = env.type.replace('ltp3/', '');
    let brief = '';
    if (env.hook) brief = env.hook;
    else if (env.event) brief = env.event;
    else if (env.command) brief = env.command;
    else if (env.tool) brief = env.tool;
    else if (env.action) brief = env.action + (env.id ? ' · ' + env.id : '');
    return (label ? label + ' · ' : '') + t + (brief ? ' → ' + brief : '');
}

// 入站消息的摘要与错误标记
function summarizeIn(m) {
    const t = m.type.replace('ltp3/', '');
    let text = t;
    let isError = !!m.error || (m.summary && m.summary.errored > 0);
    switch (m.type) {
        case 'ltp3/hook_result':
            text = `钩子「${m.hook}」subscribed=${m.summary.subscribed} errored=${m.summary.errored}`;
            break;
        case 'ltp3/event_ack':
            text = `事件「${m.event}」subscribed=${m.subscribed}`;
            break;
        case 'ltp3/command_result':
            text = `指令「${m.command}」subscribed=${m.summary.subscribed}`;
            break;
        case 'ltp3/tool_result':
            text = `工具「${m.tool}」subscribed=${m.summary.subscribed} errored=${m.summary.errored}`;
            break;
        case 'ltp3/manage_ack':
            text = `管理 ${m.action} ok=${m.ok}` + (m.message ? ' · ' + m.message : '') + (m.plugins ? ` · 插件 ${m.plugins.length} 个` : '');
            break;
        case 'ltp3/pong':
            text = `引擎 ${m.engine} 在线 · 插件 ${m.plugins} 个`;
            break;
        case 'ltp3/send':
            text = `插件发言(${m.kind}) ${m.content || m.image || m.emoji || ''}`;
            break;
        case 'ltp3/lifecycle':
            text = `生命周期 ${m.event} · ${m.title || m.plugin || ''}`;
            break;
        case 'ltp3/error':
            text = m.error || '未知错误';
            isError = true;
            break;
    }
    return { text, isError };
}

// ==== 回显日志（卡片网格） ====
function addLog(entry) {
    entry.time = new Date();
    state.log.unshift(entry);
    if (state.log.length > 500) state.log.pop();
    renderLog();
}

function renderLog() {
    logList.innerHTML = '';
    const items = state.log.filter(e => {
        if (state.filter === 'all') return true;
        if (state.filter === 'recv') return e.dir === 'recv';
        if (state.filter === 'send') return e.dir === 'send';
        if (state.filter === 'error') return e.isError;
        return true;
    });
    if (items.length === 0) {
        logList.innerHTML = '<div class="empty-tip" style="grid-column:1/-1;">暂无记录</div>';
        return;
    }
    items.forEach(entry => logList.appendChild(buildLogEntry(entry)));
}

function buildLogEntry(entry) {
    const el = document.createElement('div');
    el.className = 'log-entry' + (entry.isError ? ' error' : '');

    // 头部：类型徽标 + 时间 + 标记
    const head = document.createElement('div');
    head.className = 'log-head';

    const badge = document.createElement('span');
    badge.className = 'log-type ' + typeClass(entry);
    badge.textContent = entry.type.replace('ltp3/', '');
    head.appendChild(badge);

    const time = document.createElement('span');
    time.className = 'log-time';
    const d = entry.time;
    time.textContent = `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}:${String(d.getSeconds()).padStart(2, '0')}`;
    head.appendChild(time);

    const tag = document.createElement('span');
    tag.className = 'log-tag';
    if (entry.dir === 'send') {
        tag.textContent = '发送';
    } else if (entry.isError) {
        tag.textContent = '错误';
        tag.style.background = 'rgba(248,113,113,0.3)';
        tag.style.color = '#fca5a5';
    } else {
        tag.textContent = entry.isOwn ? '回执' : '广播';
    }
    head.appendChild(tag);

    el.appendChild(head);

    // 摘要
    const summary = document.createElement('div');
    summary.className = 'log-summary';
    summary.textContent = entry.summary;
    summary.title = entry.summary;
    el.appendChild(summary);

    // 提示：点击查看完整详情
    const hint = document.createElement('div');
    hint.className = 'log-json-hint';
    hint.innerHTML = '<i class="fas fa-expand"></i> 点击查看完整 JSON';
    el.appendChild(hint);

    // 整卡点击 → 弹出详情模态框
    el.addEventListener('click', () => openLogModal(entry));
    return el;
}

function typeClass(entry) {
    if (entry.isError) return 't-error';
    if (entry.dir === 'send') return 't-send';
    if (entry.isOwn) return 't-response';
    return 't-broadcast';
}

// ==== 插件列表 ====
function renderPlugins(plugins) {
    state.plugins = plugins || [];
    pluginCountBadge.textContent = state.plugins.length;
    pluginCount.textContent = state.plugins.length;
    pluginList.innerHTML = '';
    if (state.plugins.length === 0) {
        pluginList.innerHTML = '<div class="empty-tip">暂无插件（引擎每 3 秒自动对账包目录中的 LTP3 包）</div>';
        return;
    }
    state.plugins.forEach(p => {
        const row = document.createElement('div');
        row.className = 'plugin-row';

        const badge = document.createElement('span');
        badge.className = 'plugin-badge ' + (p.loaded ? 'loaded' : 'error');
        badge.title = p.loaded ? '已加载' : (p.error || '未加载');
        row.appendChild(badge);

        const id = document.createElement('span');
        id.className = 'plugin-id';
        id.textContent = p.id;
        id.title = p.id;
        row.appendChild(id);

        const title = document.createElement('span');
        title.className = 'plugin-title';
        title.textContent = (p.title || '') + (p.dir_name && p.dir_name !== p.id ? ' · ' + p.dir_name : '') + (p.error ? ' · ' + p.error : '');
        title.title = p.title || p.id;
        row.appendChild(title);

        const actions = document.createElement('div');
        actions.className = 'plugin-actions';

        const reload = document.createElement('button');
        reload.textContent = '重载';
        reload.title = '重载插件 ' + p.id;
        reload.addEventListener('click', () => sendEnvelope({ type: 'ltp3/manage', action: 'reload_one', id: p.id }, '重载插件'));

        const unload = document.createElement('button');
        unload.className = 'unload';
        unload.textContent = '卸载';
        unload.title = '卸载插件 ' + p.id;
        unload.addEventListener('click', () => sendEnvelope({ type: 'ltp3/manage', action: 'unload_one', id: p.id }, '卸载插件'));

        actions.appendChild(reload);
        actions.appendChild(unload);
        row.appendChild(actions);
        pluginList.appendChild(row);
    });
}

// ==== 引擎状态 UI ====
function updateStatusUI() {
    if (state.connected) {
        wsDot.className = 'status-dot on';
        wsState.textContent = '已连接';
    } else if (state.wsRetry > 0 && state.wsRetry < WS_MAX_RETRY) {
        wsDot.className = 'status-dot off';
        wsState.textContent = `重连中(${state.wsRetry}/${WS_MAX_RETRY})`;
    } else if (state.wsRetry >= WS_MAX_RETRY) {
        wsDot.className = 'status-dot off';
        wsState.textContent = '连接失败';
    } else {
        wsDot.className = 'status-dot';
        wsState.textContent = '未连接';
    }
}

function updateSavedBadge() {
    flowSaved.textContent = localStorage.getItem(FLOW_KEY) ? '有' : '无';
}

// ==== 流程节点（可视化编程） ====
// configured：是否被设置过（false → 卡片显示红色边框提示）
function createNode(type) {
    const node = { id: 'n' + Date.now() + '_' + (++state.seq), type, enabled: true, configured: false };
    switch (type) {
        case 'hook':
            node.hook = 'chat.receive.before_process';
            node.payload = '{\n  "id": "m1",\n  "groupId": "g1",\n  "senderName": "测试员",\n  "content": "今天天气如何",\n  "platform": "qq"\n}';
            node.context = '{}';
            break;
        case 'event':
            node.event = 'ON_MESSAGE';
            node.payload = '{\n  "content": "示例事件载荷"\n}';
            break;
        case 'command':
            node.command = '/天气 上海';
            node.context = '{\n  "groupId": "g1"\n}';
            break;
        case 'tool':
            node.tool = 'get_weather';
            node.payload = '{\n  "city": "北京"\n}';
            node.context = '{\n  "groupId": "g1"\n}';
            break;
        case 'manage':
            node.action = 'list';
            node.id = '';
            break;
        case 'delay':
            node.ms = 500;
            break;
    }
    return node;
}

// ==== 节点视图：蛇形网格 ====
function gridCols() {
    const w = nodeGrid.clientWidth || 420;
    const cardMin = 150;
    const gap = 10;
    const cols = Math.max(2, Math.floor((w + gap) / (cardMin + gap)));
    return Math.min(cols, 8);
}

function renderNodeGrid() {
    const cols = gridCols();
    nodeGrid.style.gridTemplateColumns = 'repeat(' + cols + ', minmax(0, 1fr))';

    // 空流程：显示空态提示（flowEmpty 被 innerHTML 清空后需重新挂回）
    if (state.nodes.length === 0) {
        nodeGrid.innerHTML = '';
        nodeGrid.appendChild(flowEmpty);
        flowEmpty.style.display = 'block';
        return;
    }
    flowEmpty.style.display = 'none';
    nodeGrid.innerHTML = '';

    state.nodes.forEach((node, i) => {
        const row = Math.floor(i / cols);
        const colInRow = i % cols;
        const col = row % 2 === 0 ? colInRow : cols - 1 - colInRow; // 蛇形：偶数行左→右，奇数行右→左
        const card = buildNodeCard(node, i);
        card.style.gridColumn = String(col + 1);
        card.style.gridRow = String(row + 1);
        nodeGrid.appendChild(card);
    });
}

// 节点卡片（矩形，带状态：选中/停用/未设置/运行中）
function buildNodeCard(node, idx) {
    const meta = NODE_META[node.type] || { label: node.type, icon: 'fa-cube', color: '#9d6bff' };
    const card = document.createElement('div');
    card.className = 'node-card';
    card.dataset.id = node.id;
    card.style.setProperty('--node-color', meta.color);
    card.classList.toggle('selected', node.id === state.selectedId);
    card.classList.toggle('disabled', !node.enabled);
    card.classList.toggle('unset', !node.configured);

    const top = document.createElement('div');
    top.className = 'node-card-top';
    const icon = document.createElement('span');
    icon.className = 'node-card-icon';
    icon.innerHTML = '<i class="fas ' + meta.icon + '"></i>';
    const idxSpan = document.createElement('span');
    idxSpan.className = 'node-card-idx';
    idxSpan.textContent = '#' + (idx + 1);
    top.appendChild(icon);
    top.appendChild(idxSpan);
    card.appendChild(top);

    const title = document.createElement('div');
    title.className = 'node-card-title';
    title.textContent = meta.label;
    title.title = meta.desc;
    card.appendChild(title);

    const foot = document.createElement('div');
    foot.className = 'node-card-foot';
    const tag = createStateTag('node-state', node);
    foot.appendChild(tag);
    const arrowCls = flowArrowClass(node, idx);
    if (arrowCls) {
        const arrow = document.createElement('i');
        arrow.className = 'fas flow-arrow ' + arrowCls;
        arrow.title = '流程流向';
        foot.appendChild(arrow);
    }
    card.appendChild(foot);

    card.addEventListener('click', () => openNodeModal(node.id));
    return card;
}

// 蛇形流向箭头：行中向右/向左，行尾向下
function flowArrowClass(node, idx) {
    const cols = gridCols();
    if (idx === state.nodes.length - 1) return '';
    const row = Math.floor(idx / cols);
    const colInRow = idx % cols;
    if (colInRow === cols - 1) return 'fa-arrow-down';
    return row % 2 === 0 ? 'fa-arrow-right' : 'fa-arrow-left';
}

// 状态标签（已设置 / 未设置 / 已停用），baseClass 区分 node-state 与 detail-state 样式
function createStateTag(baseClass, node) {
    const tag = document.createElement('span');
    tag.dataset.base = baseClass;
    applyStateTag(tag, node);
    return tag;
}

function applyStateTag(tag, node) {
    tag.className = tag.dataset.base || 'node-state';
    if (!node.enabled) {
        tag.classList.add('off');
        tag.textContent = '已停用';
    } else if (!node.configured) {
        tag.classList.add('unset');
        tag.textContent = '未设置';
    } else {
        tag.classList.add('on');
        tag.textContent = '已设置';
    }
}

// 点击节点卡片 → 弹出该节点的详情编辑模态框（并高亮选中）
function openNodeModal(id) {
    const node = state.nodes.find(n => n.id === id);
    if (!node) return;
    state.selectedId = id;
    renderNodeGrid(); // 更新选中高亮

    // 顶栏
    detailModalTop.innerHTML = '';
    const meta = NODE_META[node.type] || { label: node.type, icon: 'fa-cube', color: '#9d6bff' };
    const title = document.createElement('span');
    title.className = 'modal-title';
    title.innerHTML = '<i class="fas ' + meta.icon + '" style="color:' + meta.color + '"></i> 节点详情 · #' + (state.nodes.indexOf(node) + 1) + ' ' + meta.label;
    detailModalTop.appendChild(title);
    detailModalTop.appendChild(createCloseBtn());

    // 内容：详情编辑器
    detailModalContent.innerHTML = '';
    const editor = document.createElement('div');
    editor.style.cssText = 'display:flex;flex-direction:column;gap:10px;';
    buildNodeEditor(editor, node);
    detailModalContent.appendChild(editor);

    openModal();
}

// ==== 详情模态框控制 ====
function openModal() {
    detailModal.classList.add('open');
}

function closeModal() {
    detailModal.classList.remove('open');
    detailModalTop.innerHTML = '';
    detailModalContent.innerHTML = '';
}

function createCloseBtn() {
    const btn = document.createElement('button');
    btn.className = 'modal-close';
    btn.innerHTML = '&times;';
    btn.title = '关闭';
    btn.addEventListener('click', closeModal);
    return btn;
}

// ==== 引擎回显详情模态框 ====
function openLogModal(entry) {
    // 顶栏：类型徽标 + 时间 + 标记
    detailModalTop.innerHTML = '';
    const title = document.createElement('span');
    title.className = 'modal-title';

    const badge = document.createElement('span');
    badge.className = 'log-type ' + typeClass(entry);
    badge.textContent = entry.type.replace('ltp3/', '');
    title.appendChild(badge);

    const d = entry.time;
    const time = document.createElement('span');
    time.className = 'log-time';
    time.textContent = `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}:${String(d.getSeconds()).padStart(2, '0')}`;
    title.appendChild(time);

    const tag = document.createElement('span');
    tag.className = 'log-tag';
    if (entry.dir === 'send') tag.textContent = '发送';
    else if (entry.isError) { tag.textContent = '错误'; tag.style.background = 'rgba(248,113,113,0.3)'; tag.style.color = '#fca5a5'; }
    else tag.textContent = entry.isOwn ? '回执' : '广播';
    title.appendChild(tag);

    detailModalTop.appendChild(title);
    detailModalTop.appendChild(createCloseBtn());

    // 内容：摘要 + 完整 JSON
    detailModalContent.innerHTML = '';
    const summary = document.createElement('div');
    summary.className = 'modal-summary';
    summary.textContent = entry.summary;
    detailModalContent.appendChild(summary);
    const pre = document.createElement('pre');
    pre.className = 'modal-json';
    pre.textContent = JSON.stringify(entry.data, null, 2);
    detailModalContent.appendChild(pre);

    openModal();
}

// 就地更新某张卡片的状态样式（字段编辑触发，避免整卡重建丢失焦点）
function updateCardStatus(node) {
    const card = nodeGrid.querySelector('.node-card[data-id="' + node.id + '"]');
    if (!card) return;
    card.classList.toggle('disabled', !node.enabled);
    card.classList.toggle('unset', !node.configured);
    const tag = card.querySelector('[data-base="node-state"]');
    if (tag) applyStateTag(tag, node);
}

function markConfigured(node) {
    if (!node.configured) {
        node.configured = true;
        updateCardStatus(node);
        // 若该节点详情正显示在模态框中，同步刷新其状态标签
        const tag = detailModalContent.querySelector('[data-base="detail-state"]');
        if (tag && node.id === state.selectedId) applyStateTag(tag, node);
    }
}

// ==== 节点详情编辑器（模态框内容，只渲染传入节点） ====
function buildNodeEditor(container, node) {
    const meta = NODE_META[node.type] || { label: node.type, icon: 'fa-cube', color: '#9d6bff' };
    const idx = state.nodes.indexOf(node);

    const header = document.createElement('div');
    header.className = 'detail-header';
    header.style.setProperty('--node-color', meta.color);

    const icon = document.createElement('span');
    icon.className = 'node-icon';
    icon.innerHTML = '<i class="fas ' + meta.icon + '"></i>';
    header.appendChild(icon);

    const title = document.createElement('span');
    title.className = 'detail-title';
    title.textContent = '#' + (idx + 1) + ' ' + meta.label;
    title.title = meta.desc;
    header.appendChild(title);

    header.appendChild(createStateTag('detail-state', node));

    // 启用开关
    const toggle = document.createElement('label');
    toggle.className = 'node-toggle';
    const cb = document.createElement('input');
    cb.type = 'checkbox';
    cb.checked = node.enabled;
    cb.addEventListener('change', () => {
        node.enabled = cb.checked;
        renderNodeGrid();
        buildNodeEditorBody(container, node); // 刷新状态标签（无文本输入，不会丢焦点）
    });
    const slider = document.createElement('span');
    toggle.appendChild(cb);
    toggle.appendChild(slider);
    header.appendChild(toggle);

    // 运行（仅该模块）
    const run = document.createElement('button');
    run.className = 'node-btn node-run';
    run.title = '仅运行该模块';
    run.innerHTML = '<i class="fas fa-play"></i>';
    run.addEventListener('click', () => runNode(node, idx));
    header.appendChild(run);

    // 复制
    const dup = document.createElement('button');
    dup.className = 'node-btn';
    dup.title = '复制模块';
    dup.innerHTML = '<i class="fas fa-copy"></i>';
    dup.addEventListener('click', () => {
        const copy = JSON.parse(JSON.stringify(node));
        copy.id = 'n' + Date.now() + '_' + (++state.seq);
        state.nodes.splice(idx + 1, 0, copy);
        state.selectedId = copy.id;
        renderNodeGrid();
        openNodeModal(copy.id);
    });
    header.appendChild(dup);

    // 删除
    const del = document.createElement('button');
    del.className = 'node-btn node-del';
    del.title = '删除模块';
    del.innerHTML = '<i class="fas fa-times"></i>';
    del.addEventListener('click', () => {
        state.nodes.splice(idx, 1);
        if (state.selectedId === node.id) state.selectedId = null;
        renderNodeGrid();
        closeModal();
    });
    header.appendChild(del);

    container.appendChild(header);
    buildNodeEditorBody(container, node);
}

// 编辑器字段区（独立函数便于启用开关切换后只重建字段区保留顶栏样式）
function buildNodeEditorBody(container, node) {
    const old = container.querySelector('.detail-body');
    if (old) old.remove();
    const body = document.createElement('div');
    body.className = 'detail-body';
    buildNodeFields(body, node);
    container.appendChild(body);
}

// ==== 字段构建器（事件监听直接就地更新 node 字段，避免输入时重建丢焦点） ====
function addField(container, label, control, opts) {
    const wrap = document.createElement('div');
    wrap.className = 'field';
    const lab = document.createElement('label');
    lab.className = 'field-label';
    lab.textContent = label;
    if (opts && opts.required) {
        const req = document.createElement('span');
        req.className = 'req';
        req.textContent = '*';
        lab.appendChild(req);
    }
    wrap.appendChild(lab);
    wrap.appendChild(control);
    container.appendChild(wrap);
    return wrap;
}

function textInput(value, onInput, placeholder) {
    const input = document.createElement('input');
    input.type = 'text';
    input.value = value || '';
    input.placeholder = placeholder || '';
    input.addEventListener('input', () => onInput(input.value));
    return input;
}

// 可输入 + 候选列表（datalist）下拉框：允许自定义值，同时给出协议枚举提示。
// 带可见的下拉箭头与占位提示，可读性优于原生 select（所有选择型字段统一使用）。
function comboInput(options, value, onInput, placeholder) {
    const id = 'dl_' + (++state.seq);
    const wrap = document.createElement('div');
    wrap.className = 'combo-wrap';

    const input = document.createElement('input');
    input.type = 'text';
    input.className = 'combo-input';
    input.value = value || '';
    input.placeholder = placeholder || '';
    input.spellcheck = false;
    input.setAttribute('list', id);
    input.addEventListener('input', () => onInput(input.value));

    // 下拉箭头（指示可展开候选；Chromium 对 input[list] 原生自带箭头，这里再补一个统一视觉）
    const caret = document.createElement('i');
    caret.className = 'fas fa-chevron-down combo-caret';

    const dl = document.createElement('datalist');
    dl.id = id;
    options.forEach(opt => {
        const o = document.createElement('option');
        o.value = opt;
        dl.appendChild(o);
    });

    wrap.appendChild(input);
    wrap.appendChild(caret);
    wrap.appendChild(dl);
    return wrap;
}

function jsonInput(value, onInput, rows) {
    const ta = document.createElement('textarea');
    ta.value = value || '';
    ta.rows = rows || 3;
    ta.spellcheck = false;
    ta.addEventListener('input', () => onInput(ta.value));
    return ta;
}

function numInput(value, onInput) {
    const input = document.createElement('input');
    input.type = 'number';
    input.value = value != null ? value : '';
    input.min = '0';
    input.step = '100';
    input.addEventListener('input', () => onInput(parseInt(input.value, 10) || 0));
    return input;
}

// 各类型节点的字段（首次编辑任一字段即视为「已设置」）
function buildNodeFields(body, node) {
    const mark = () => markConfigured(node);
    switch (node.type) {
        case 'hook':
            addField(body, '钩子点', comboInput(YARA_HOOKS, node.hook, v => { node.hook = v; mark(); }, '选择或输入钩子点'), { required: true });
            addField(body, '消息载荷 payload（JSON）', jsonInput(node.payload, v => { node.payload = v; mark(); }, 4));
            addField(body, '上下文 context（JSON）', jsonInput(node.context, v => { node.context = v; mark(); }, 2));
            break;
        case 'event':
            addField(body, '事件名', comboInput(YARA_EVENTS, node.event, v => { node.event = v; mark(); }, '选择或输入事件名'), { required: true });
            addField(body, '事件载荷 payload（JSON）', jsonInput(node.payload, v => { node.payload = v; mark(); }, 3));
            break;
        case 'command':
            addField(body, '指令文本（名称或正则匹配整段）', textInput(node.command, v => { node.command = v; mark(); }, '/天气 上海'), { required: true });
            addField(body, '上下文 context（JSON）', jsonInput(node.context, v => { node.context = v; mark(); }, 2));
            break;
        case 'tool':
            addField(body, '工具名', textInput(node.tool, v => { node.tool = v; mark(); }, 'get_weather'), { required: true });
            addField(body, '工具参数 payload（JSON）', jsonInput(node.payload, v => { node.payload = v; mark(); }, 3));
            addField(body, '上下文 context（JSON）', jsonInput(node.context, v => { node.context = v; mark(); }, 2));
            break;
        case 'manage':
            addField(body, '管理动作', comboInput(MANAGE_ACTIONS, node.action, v => { node.action = v; mark(); }, '选择或输入管理动作'), { required: true });
            addField(body, '目标插件 id（reload_one / unload_one 时填写）', textInput(node.id, v => { node.id = v; mark(); }, 'com.example.hello'));
            break;
        case 'ping': {
            const hint = document.createElement('div');
            hint.className = 'field-hint';
            hint.textContent = '发送 ltp3/ping 探测引擎在线状态与已加载插件数';
            body.appendChild(hint);
            break;
        }
        case 'delay':
            addField(body, '等待毫秒', numInput(node.ms, v => { node.ms = v; mark(); }));
            break;
    }
}

// ==== 信封构建与执行 ====
function buildEnvelope(node) {
    switch (node.type) {
        case 'hook': {
            const payload = parseJSON(node.payload, '钩子载荷');
            if (payload === undefined) return null;
            const ctx = parseJSON(node.context || '{}', '钩子上下文');
            if (ctx === undefined) return null;
            if (!(node.hook || '').trim()) { toast('请选择钩子点', 'error'); return null; }
            return { type: 'ltp3/hook', hook: node.hook.trim(), payload, context: ctx };
        }
        case 'event': {
            const payload = parseJSON(node.payload || '{}', '事件载荷');
            if (payload === undefined) return null;
            if (!(node.event || '').trim()) { toast('请填写事件名', 'error'); return null; }
            return { type: 'ltp3/event', event: node.event.trim(), payload };
        }
        case 'command': {
            if (!(node.command || '').trim()) { toast('请填写指令文本', 'error'); return null; }
            const ctx = parseJSON(node.context || '{}', '指令上下文');
            if (ctx === undefined) return null;
            return { type: 'ltp3/command', command: node.command.trim(), context: ctx };
        }
        case 'tool': {
            if (!(node.tool || '').trim()) { toast('请填写工具名', 'error'); return null; }
            const payload = parseJSON(node.payload || '{}', '工具参数');
            if (payload === undefined) return null;
            const ctx = parseJSON(node.context || '{}', '工具上下文');
            if (ctx === undefined) return null;
            return { type: 'ltp3/tool', tool: node.tool.trim(), payload, context: ctx };
        }
        case 'manage':
            return { type: 'ltp3/manage', action: node.action, ...((node.id || '').trim() ? { id: node.id.trim() } : {}) };
        case 'ping':
            return { type: 'ltp3/ping' };
        case 'delay':
            return null;
    }
    return null;
}

function runNode(node, idx) {
    const env = buildEnvelope(node);
    if (!env) return;
    const card = nodeGrid.querySelector('.node-card[data-id="' + node.id + '"]');
    if (card) card.classList.add('running');
    sendEnvelope(env, NODE_META[node.type].label + ' #' + ((idx != null ? idx : state.nodes.indexOf(node)) + 1));
    setTimeout(() => { if (card) card.classList.remove('running'); }, 1200);
}

async function runFlow() {
    if (state.running) return;
    const targets = state.nodes.filter(n => n.enabled);
    if (targets.length === 0) {
        toast('流程为空，请先添加模块', 'error');
        return;
    }
    if (!state.connected) {
        toast('尚未连接引擎', 'error');
        return;
    }
    state.running = true;
    runFlowBtn.disabled = true;
    for (let i = 0; i < targets.length; i++) {
        const n = targets[i];
        const card = nodeGrid.querySelector('.node-card[data-id="' + n.id + '"]');
        if (card) card.classList.add('running');
        flowStatus.textContent = `运行中 ${i + 1}/${targets.length}`;
        if (n.type === 'delay') {
            await sleep(Math.max(0, n.ms || 500));
        } else {
            runNode(n, state.nodes.indexOf(n));
        }
        if (i < targets.length - 1) await sleep(350); // 模块间留出回执间隔
        if (card) card.classList.remove('running');
    }
    flowStatus.textContent = '完成';
    setTimeout(() => { if (flowStatus.textContent === '完成') flowStatus.textContent = ''; }, 2500);
    state.running = false;
    runFlowBtn.disabled = false;
}

// ==== 流程保存 / 加载 / 清空 ====
function saveFlow() {
    localStorage.setItem(FLOW_KEY, JSON.stringify(state.nodes));
    updateSavedBadge();
    toast('流程已保存到本地', 'success');
}

function loadFlow() {
    const raw = localStorage.getItem(FLOW_KEY);
    if (!raw) {
        toast('没有已保存的流程', 'error');
        return;
    }
    try {
        const arr = JSON.parse(raw);
        if (!Array.isArray(arr)) throw new Error('格式错误');
        state.nodes = arr;
        state.selectedId = null;
        closeModal();
        renderNodeGrid();
        toast('已加载保存的流程', 'success');
    } catch (e) {
        toast('加载失败: ' + e.message, 'error');
    }
}

function clearFlow() {
    if (state.nodes.length === 0) return;
    if (!confirm('确定清空当前流程画布？')) return;
    state.nodes = [];
    state.selectedId = null;
    closeModal();
    renderNodeGrid();
    toast('流程已清空');
}

// 演示流程（首次进入且无本地流程时预置；示例节点视为已设置）
function demoFlow() {
    const types = ['ping', 'manage', 'tool', 'hook', 'event', 'delay', 'ping'];
    return types.map(type => {
        const n = createNode(type);
        n.configured = true;
        return n;
    });
}

// ==== 事件绑定 ====
function bindEvents() {
    // 调色板：点击 / 拖入节点网格
    palette.querySelectorAll('.palette-chip').forEach(chip => {
        chip.addEventListener('click', () => {
            const n = createNode(chip.dataset.type);
            state.nodes.push(n);
            renderNodeGrid();
            openNodeModal(n.id);
        });
        chip.addEventListener('dragstart', (e) => {
            try { e.dataTransfer.setData('text/plain', chip.dataset.type); } catch (err) { }
            e.dataTransfer.effectAllowed = 'copy';
        });
    });

    nodeGrid.addEventListener('dragover', (e) => { e.preventDefault(); e.dataTransfer.dropEffect = 'copy'; });
    nodeGrid.addEventListener('drop', (e) => {
        e.preventDefault();
        const type = e.dataTransfer.getData('text/plain');
        if (NODE_META[type]) {
            const n = createNode(type);
            state.nodes.push(n);
            renderNodeGrid();
            openNodeModal(n.id);
        }
    });

    // 窗口尺寸变化时重排蛇形网格
    window.addEventListener('resize', () => renderNodeGrid());

    // 详情模态框：点击遮罩关闭 / ESC 关闭
    detailModal.addEventListener('click', (e) => {
        if (e.target === detailModal) closeModal();
    });
    document.addEventListener('keydown', (e) => {
        if (e.key === 'Escape') closeModal();
    });

    runFlowBtn.addEventListener('click', runFlow);
    saveFlowBtn.addEventListener('click', saveFlow);
    loadFlowBtn.addEventListener('click', loadFlow);
    clearFlowBtn.addEventListener('click', clearFlow);

    // 右栏状态按钮
    reconnectBtn.addEventListener('click', () => {
        state.wsRetry = 0;
        if (state.ws) { try { state.ws.close(); } catch (e) { } }
        connect();
    });
    pingBtn.addEventListener('click', () => sendEnvelope({ type: 'ltp3/ping' }, '探测'));
    listBtn.addEventListener('click', () => sendEnvelope({ type: 'ltp3/manage', action: 'list' }, '插件列表'));
    scanBtn.addEventListener('click', () => sendEnvelope({ type: 'ltp3/manage', action: 'scan' }, '强制对账'));
    reloadAllBtn.addEventListener('click', () => sendEnvelope({ type: 'ltp3/manage', action: 'reload' }, '全部重载'));

    // 日志过滤与清空
    logFilters.addEventListener('click', (e) => {
        const chip = e.target.closest('.filter-chip');
        if (!chip) return;
        logFilters.querySelectorAll('.filter-chip').forEach(c => c.classList.remove('active'));
        chip.classList.add('active');
        state.filter = chip.dataset.filter;
        renderLog();
    });
    clearLogBtn.addEventListener('click', () => {
        state.log = [];
        renderLog();
    });
}

// ==== 启动 ====
function init() {
    // 优先加载本地保存的流程，否则预置演示流程
    const raw = localStorage.getItem(FLOW_KEY);
    if (raw) {
        try {
            const arr = JSON.parse(raw);
            if (Array.isArray(arr)) state.nodes = arr;
        } catch (e) { state.nodes = demoFlow(); }
    } else {
        state.nodes = demoFlow();
    }
    updateSavedBadge();
    bindEvents();
    renderNodeGrid();
    updateStatusUI();
    connect();
}

init();
