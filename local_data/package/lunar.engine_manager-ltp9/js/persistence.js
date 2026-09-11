// ==== 持久化 / 示例 / 清空 ====
// 画布以 JSON 文件形式保存到 local_data/database/（同项目数据库目录）
function graphJson() {
    const ids = new Set(state.nodes.map(n => n.id)); // 保存时过滤无效连线，防止坏线入库
    return JSON.stringify({
        nodes: state.nodes.map(n => ({ id: n.id, type: n.type, x: n.x, y: n.y, params: n.params, enabled: n.enabled })),
        links: state.links.filter(l => l && l.from && ids.has(l.from.node) && l.to && ids.has(l.to.node)).map(l => ({ id: l.id, from: l.from, to: l.to }))
    }, null, 2);
}
async function save() {
    try {
        const resp = await fetch('/file/write', {
            method: 'POST',
            headers: {
                'X-File-Name': btoa(unescape(encodeURIComponent(FLOW_FILE))),
                'X-Overwrite': 'true'
            },
            body: new Blob([graphJson()], { type: 'application/json' })
        });
        if (!resp.ok) { toast('保存失败: ' + resp.status, 'error'); return; }
        toast('画布已保存 → ' + FLOW_FILE, 'success');
    } catch (e) { toast('保存异常: ' + e.message, 'error'); }
}
function applyGraph(g) {
    state.nodes = (g.nodes || []).map(n => Object.assign({ enabled: true, running: false, _s: 'pending', outValue: null }, n));
    state.links = g.links || [];
    sanitizeGraph();
    renderGraph();
}
// 清洗无效连线：端点节点已不存在（历史编辑残留的 undefined 指针）会让激活链抛异常，加载/封装/运行前统一剔除
function sanitizeGraph() {
    const ids = new Set(state.nodes.map(n => n.id));
    const before = state.links.length;
    state.links = state.links.filter(l => l && l.from && ids.has(l.from.node) && l.to && ids.has(l.to.node));
    if (state.links.length !== before) { toast('已清理 ' + (before - state.links.length) + ' 条无效连线', 'error'); renderLinks(); }
}
async function load() {
    try {
        const resp = await fetch('/file/read/' + FLOW_FILE);
        if (!resp.ok) { toast('没有已保存的画布（' + FLOW_FILE + '）', 'error'); return; }
        const g = await resp.json();
        applyGraph(g); toast('已加载画布', 'success');
    } catch (e) { toast('加载失败: ' + e.message, 'error'); }
}
function clearGraph() { if (!state.nodes.length) return; if (!confirm('确定清空画布？')) return; state.nodes = []; state.links = []; renderGraph(); closeModal(); closeLogModal(); toast('已清空'); }
function demoGraph() {
    state.nodes = []; state.links = [];
    const cx = 60, cy = 60, gapX = 240, gapY = 150;
    const defs = [
        { type: 'start', x: cx, y: cy },
        { type: 'stats', x: cx, y: cy + gapY },
        { type: 'wait', x: cx + gapX, y: cy },
        { type: 'crypto', x: cx + gapX, y: cy + gapY },
        { type: 'llm', x: cx + gapX * 2, y: cy },
        { type: 'broadcast_all', x: cx + gapX * 2, y: cy + gapY },
        { type: 'transform', x: cx + gapX * 3, y: cy }
    ];
    defs.forEach(d => state.nodes.push({ id: newId(), type: d.type, x: d.x, y: d.y, params: defaultParams(d.type), running: false, _s: 'pending', outValue: null }));
    const lk = (a, b, ap, bp) => state.links.push({ id: 'l' + seg(), from: { node: state.nodes[a].id, port: ap }, to: { node: state.nodes[b].id, port: bp } });
    // 时钟链：start → stats → wait → crypto → llm → broadcast
    lk(0, 1, 'gate', 'gate');
    lk(1, 2, 'rows', 'gate');
    lk(2, 3, 'gate', 'gate');
    lk(3, 4, 'text', 'gate');
    lk(4, 5, 'answer', 'gate');
    // 时钟：start → 格式转换；同时 stats 结果经信号线（rows→in1）喂入 格式转换
    lk(0, 6, 'gate', 'gate');
    lk(1, 6, 'rows', 'in1');
    renderGraph();
}
function ensureLinkByIdx(a, b, ap, bp) { /* 兼容预留 */ }