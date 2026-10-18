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
const ACK_TYPES = { 'ltp9/result': 1, 'ltp9/test_ack': 1, 'ltp9/stats_ack': 1, 'ltp9/broadcast_ack': 1, 'ltp9/pong': 1, 'ltp9/bp_ack': 1 };

// ==== 状态 ====
const state = {
    ws: null, retry: 0, connected: false,
    seq: 0, running: false,
    bpRunId: null,     // 当前运行标识（后端分配；多标签页按 run_id 过滤推送）
    nodes: [], links: [], selectedNodeId: null,
    selected: new Set(),  // Ctrl+多选的节点 id（封装复合节点用）
    pending: new Map(),
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
    return state.nodes.find(n => n.id === id);
}
// 扁平节点/连线查找：调度已迁移至后端，复合子图节点（~cN 重映射 ID）仅存在于引擎侧，
// 前端只维护主图节点与连线
function incomingLinks(id) {
    return state.links.filter(l => l.to.node === id);
}
function outgoingLinks(id) {
    return state.links.filter(l => l.from.node === id);
}

function defaultParams(type) {
    const p = {};
    (metaOf(type).fields || []).forEach(f => { if (f.def !== undefined) p[f.key] = f.def; });
    return p;
}