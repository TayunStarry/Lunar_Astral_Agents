// ============================================================
// 『 星月智能 』LTP9 引擎可视化测试（蓝图编辑器 v2）
// 核心基础：常量 / 全局状态 / DOM 引用 / 基础工具函数
// ============================================================

// ==== 常量 ====
const FLOW_FILE = 'database/lunar.engine_manager_ltp9.graph.json'; // 画布持久化到本地文件（local_data/database/）
const WS_MAX_RETRY = 5;
const WS_RETRY_INTERVAL = 3000;
const NODE_W = 200;
const HEADER_H = 34;  // 节点头部高度，端口从下方起排
const PORT_STEP = 22; // 端口纵向间距
// 判定回执的有效类型：只有这些类型 + 匹配 request_id 才算真实回执
// （/ws 集线器会把客户端发出的请求原样广播回显，若不区分类型会把"发送的请求"误当作回执）
const ACK_TYPES = { 'ltp9/result': 1, 'ltp9/test_ack': 1, 'ltp9/stats_ack': 1, 'ltp9/broadcast_ack': 1, 'ltp9/pong': 1 };

// ==== 状态 ====
const state = {
    ws: null, retry: 0, connected: false,
    seq: 0, running: false, abort: false,
    nodes: [], links: [], selectedNodeId: null,
    selected: new Set(),  // Ctrl+多选的节点 id（封装复合节点用）
    execViews: [],     // 复合节点子图执行视图栈（隔离的 {nodes,links,active}，支持嵌套与主图并行）
    pending: new Map(),
    evWaiters: [],   // 事件等待者（事件启动）{topic,res,_t}
    clocks: [],      // 时钟定时器句柄（时钟启动）
    active: 0,          // 全局挂起/执行中的激活任务数（含排队与等待中）；子图另有自己的计数（见 execViews），
                        // 子图完成判定只看所属视图的计数：全局计数会被其它并行分支的完成事件干扰

    callCount: 0,       // 本次运行周期已执行节点次数
    maxCalls: 300,      // 单次运行最大调用次数（可用「调用上限」节点动态调整）
    limitedNotified: false, // 达到上限只提示一次
    doneWaiters: [],    // 子图执行完成回调栈（复合节点内部执行用）
    filter: 'all', unread: 0, log: [],
    linkDrag: { from: null, x: 0, y: 0 },
    catalog: { plugins: [], events: [], exports: {}, tools: {}, agentPlugins: [] },
    modalNodeId: null // 当前浮窗编辑的节点（catalog 刷新后重渲染用）
};

// ==== DOM ====
const $ = id => document.getElementById(id);
const modal = $('modal'), modalTop = $('modalTop'), modalBody = $('modalBody');
const canvas = $('canvas'), linksSvg = $('linksSvg'), nodesLayer = $('nodesLayer');

function toast(msg, type) {
    const el = document.createElement('div');
    el.className = 'toast' + (type ? ' ' + type : '');
    el.textContent = msg; $('toastWrap').appendChild(el);
    setTimeout(() => { el.style.opacity = '0'; setTimeout(() => el.remove(), 300); }, 2200);
}
function sleep(ms) { return new Promise(r => setTimeout(r, ms)); }
function newId() { return 'n' + Date.now() + '_' + (++state.seq); }
function seg() { return 'r' + Date.now() + '_' + (++state.seq); }
function nodeById(id) {
    for (let i = state.execViews.length - 1; i >= 0; i--) {
        const n = state.execViews[i].nodes.find(x => x.id === id);
        if (n) return n;
    }
    return state.nodes.find(n => n.id === id);
}
// 节点所属执行视图：复合节点子图执行期间压栈 execViews，子图节点查其视图、主图节点查全局；
// 连线查找同样按所属视图隔离，保证复合节点与主图其他分支真正并行互不干扰。
// 视图（含主图 state 自身）都带 active 计数：挂起任务按所属视图计数，子图完成只看本视图归零。
function viewOf(id) {
    for (const v of state.execViews) if (v.nodes.some(n => n.id === id)) return v;
    return state;
}
function incomingLinks(id) { return viewOf(id).links.filter(l => l.to.node === id); }
function outgoingLinks(id) { return viewOf(id).links.filter(l => l.from.node === id); }

function defaultParams(type) {
    const p = {};
    (metaOf(type).fields || []).forEach(f => { if (f.def !== undefined) p[f.key] = f.def; });
    return p;
}