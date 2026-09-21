// ============================================================
// 『 星月智能 』LTP9 引擎可视化测试（蓝图编辑器 v2）
// 核心基础：常量 / 全局状态 / DOM 引用 / 基础工具函数
// ============================================================

// ==== 常量 ====
const GRAPH_DIR = 'database/engine_graph'; // 画布存放目录（local_data/database/engine_graph/，多画布）
const COMPOSITES_FILE = 'database/custom_node.json'; // 保存的复合节点库（「节点模块 → 我的复合」）
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
    seq: 0, running: false, abort: false, singleRun: false,
    nodes: [], links: [], selectedNodeId: null,
    selected: new Set(),  // Ctrl+多选的节点 id（封装复合节点用）
    pool: new Map(),   // 执行池：nodeId → 在池激活数（池内节点并行独立执行，无层级）
    compRuns: [],      // 复合节点扁平运行时实例（{index,links,active,done}；子图节点直接进入全局执行池，非层级视图）
    compSeq: 0,        // 复合实例节点 ID 重映射序号
    pending: new Map(),
    evWaiters: [],   // 事件等待者（事件启动）{topic,res,_t}
    clocks: [],      // 时钟定时器句柄（时钟启动）
    active: 0,          // 执行池内挂起/执行中的激活任务数（含排队与等待中）

    callCount: 0,       // 本次运行已执行的节点数量
    maxCalls: 300,      // 单次运行最大节点执行数量（可用「调用上限」节点动态调整）
    limitedNotified: false, // 达到上限只提示一次
    filter: 'all', unread: 0, log: [],
    linkDrag: { from: null, x: 0, y: 0 },
    catalog: { plugins: [], events: [], exports: {}, agentPlugins: [] },
    modalNodeId: null, // 当前浮窗编辑的节点（catalog 刷新后重渲染用）
    canvasName: null, // 当前画布名称（对应 engine_graph/<名称>.json）
    canvases: [], // 已存在的画布列表 [{name,size,modified}]
    savedComposites: [] // 保存到节点模块的复合节点 [{name, params}]
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
    for (let i = state.compRuns.length - 1; i >= 0; i--) {
        const n = state.compRuns[i].index.get(id);
        if (n) return n;
    }
    return state.nodes.find(n => n.id === id);
}
// 扁平节点/连线查找：主图与复合节点子图节点同处一个执行命名空间（子图节点经 ID 重映射后无冲突），
// 所有连线统一为先后顺序连线，无任何视图层级。
function incomingLinks(id) {
    let out = state.links.filter(l => l.to.node === id);
    for (const inst of state.compRuns) out = out.concat(inst.links.filter(l => l.to.node === id));
    return out;
}
function outgoingLinks(id) {
    let out = state.links.filter(l => l.from.node === id);
    for (const inst of state.compRuns) out = out.concat(inst.links.filter(l => l.from.node === id));
    return out;
}

function defaultParams(type) {
    const p = {};
    (metaOf(type).fields || []).forEach(f => { if (f.def !== undefined) p[f.key] = f.def; });
    return p;
}