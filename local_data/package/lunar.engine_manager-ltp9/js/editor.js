// ==== 节点/连线 增删查 ====
function addNode(type, x, y) { const n = { id: newId(), type, x, y, params: defaultParams(type), running: false, _s: 'pending', outValue: null }; state.nodes.push(n); renderGraph(); return n; }
function removeNode(id) {
    state.nodes = state.nodes.filter(n => n.id !== id);
    state.links = state.links.filter(l => l.from.node !== id && l.to.node !== id);
    renderGraph();
}
// 复制节点：参数深拷贝 + 向右下偏移放置，并把原节点进/出连线一并复制到副本
function duplicateNode(id) {
    const n = nodeById(id); if (!n) return null;
    // 复合节点副本需重建子图节点 ID（避免两个副本并行执行时子图互相串扰），同时复制原节点进/出连线
    const params = n.type === 'composite' ? cloneCompositeParams(n.params) : JSON.parse(JSON.stringify(n.params || {}));
    const copy = { id: newId(), type: n.type, x: n.x + 34, y: n.y + 34, params, running: false, _s: 'pending', outValue: null, enabled: n.enabled };
    state.nodes.push(copy);
    state.links.slice().forEach(l => {
        if (l.from.node === id) state.links.push({ id: 'l' + seg(), from: { node: copy.id, port: l.from.port }, to: { node: l.to.node, port: l.to.port } });
        else if (l.to.node === id) state.links.push({ id: 'l' + seg(), from: { node: l.from.node, port: l.from.port }, to: { node: copy.id, port: l.to.port } });
    });
    renderGraph();
    toast('已复制节点', 'success');
    return copy;
}
function removeLink(id) { state.links = state.links.filter(l => l.id !== id); renderLinks(); }
function ensureLink(from, to) {
    if (!from || !to || from.node === to.node) return;
    if (state.links.some(l => l.from.node === from.node && l.from.port === from.port && l.to.node === to.node && l.to.port === to.port)) return;
    state.links.push({ id: 'l' + seg(), from, to }); renderLinks();
}

// ==== 自定义复合节点：Ctrl+多选 → 封装 / 展开 ====
// 复合节点把一段子流程打包为单节点（params.subgraph 存内部节点/连线快照，inMap/outMap 记端口映射），
// 执行时换入子图复用全局执行器（runCompositeNode），「展开」可随时还原为原始节点再编辑。
function clearSelection() {
    state.selected.clear();
    nodesLayer.querySelectorAll('.node.selected').forEach(el => el.classList.remove('selected'));
}
function toggleSelectNode(id) {
    if (state.selected.has(id)) state.selected.delete(id); else state.selected.add(id);
    const el = nodesLayer.querySelector('.node[data-id="' + id + '"]');
    if (el) el.classList.toggle('selected', state.selected.has(id));
}
// 封装：选中节点 → 单个复合节点。外部入线映射为输入端口，外部出线映射为单输出端口（多出口需先汇聚）。
function encapsulateSelected() {
    sanitizeGraph(); // 先剔除无效连线，避免坏线被快照进复合节点
    const ids = [...state.selected];
    if (!ids.length) { toast('请先 Ctrl+点击 选中要封装的节点（至少 1 个）', 'error'); return; }
    const name = prompt('复合节点名称', '复合节点');
    if (name === null) return; // 取消
    const set = new Set(ids);
    // 1. 内部节点/连线快照（坐标以选区左上角为原点，便于展开时就近还原）
    let minX = Infinity, minY = Infinity;
    ids.forEach(id => { const n = nodeById(id); if (n) { minX = Math.min(minX, n.x); minY = Math.min(minY, n.y); } });
    const innerNodes = ids.map(id => {
        const n = nodeById(id);
        return { id: n.id, type: n.type, x: n.x - minX, y: n.y - minY, params: JSON.parse(JSON.stringify(n.params || {})) };
    });
    const innerLinks = state.links.filter(l => set.has(l.from.node) && set.has(l.to.node))
        .map(l => ({ id: l.id, from: { node: l.from.node, port: l.from.port }, to: { node: l.to.node, port: l.to.port } }));
    // 2. 边界输入：外部 → 选内；gate 口入线映射为复合节点的时钟线，其余每个内部目标端口映射一个输入端口
    const inMap = []; const seenIn = new Set();
    state.links.filter(l => !set.has(l.from.node) && set.has(l.to.node)).forEach(l => {
        if (l.to.port === 'gate') return;
        const key = l.to.node + '::' + l.to.port;
        if (seenIn.has(key)) return; seenIn.add(key);
        const t = nodeById(l.to.node);
        const pm = t && inPortsOf(t.type, t).find(p => p.k === l.to.port);
        inMap.push({ k: 'in' + (inMap.length + 1), label: (pm && pm.label) || l.to.port, node: l.to.node, port: l.to.port });
    });
    // 3. 边界输出：选内 → 外部，按 (节点,端口) 归组；复合节点为单出口模型，多于一组时要求先汇聚
    const outGroups = [];
    state.links.filter(l => set.has(l.from.node) && !set.has(l.to.node) && l.from.port !== 'gate').forEach(l => {
        let g = outGroups.find(x => x.node === l.from.node && x.port === l.from.port);
        if (!g) outGroups.push({ node: l.from.node, port: l.from.port });
    });
    if (outGroups.length > 1) { toast('选区内有 ' + outGroups.length + ' 个对外输出点，请先汇聚为一路再封装', 'error'); return; }
    const outMap = outGroups.length ? outGroups[0] : null;
    // 4. 建复合节点（放在选区中心附近）
    const comp = {
        id: newId(), type: 'composite',
        x: Math.max(10, Math.round((minX + 80) / 5) * 5), y: Math.max(10, Math.round((minY + 40) / 5) * 5),
        params: { label: name || '复合节点', hint: name || '封装的子流程', subgraph: { nodes: innerNodes, links: innerLinks }, inMap, outMap },
        running: false, _s: 'pending', outValue: null, enabled: true
    };
    // 5. 重接外部连线 → 复合节点端口；移除选区内节点与内部连线
    state.nodes = state.nodes.filter(n => !set.has(n.id));
    state.links = state.links.filter(l => !(set.has(l.from.node) && set.has(l.to.node)));
    state.links.forEach(l => {
        if (!set.has(l.from.node) && set.has(l.to.node)) {
            if (l.to.port === 'gate') { l.to = { node: comp.id, port: 'gate' }; return; }
            const m = inMap.find(x => x.node === l.to.node && x.port === l.to.port);
            if (m) l.to = { node: comp.id, port: m.k };
        } else if (set.has(l.from.node) && !set.has(l.to.node)) {
            l.from = { node: comp.id, port: l.from.port === 'gate' ? 'gate' : 'out' };
        }
    });
    state.nodes.push(comp);
    clearSelection();
    renderGraph();
    toast('已封装为复合节点「' + (name || '复合节点') + '」（' + innerNodes.length + ' 节点 / ' + innerLinks.length + ' 连线）', 'success');
}
// 展开：复合节点 → 还原内部节点，并把外部连线接回内部端点
function unpackComposite(node) {
    sanitizeGraph();
    const sg = node.params.subgraph || {};
    if (!sg.nodes || !sg.nodes.length) { removeNode(node.id); toast('复合节点为空，已移除', 'success'); return; }
    const restored = sg.nodes.map(n => ({
        id: n.id, type: n.type,
        x: Math.max(10, node.x + (n.x || 0)), y: Math.max(10, node.y + (n.y || 0)),
        params: JSON.parse(JSON.stringify(n.params || {})), running: false, _s: 'pending', outValue: null, enabled: true
    }));
    const inMap = node.params.inMap || [], outMap = node.params.outMap || null;
    state.links.forEach(l => {
        if (l.to.node === node.id && l.to.port !== 'gate') {
            const m = inMap.find(x => x.k === l.to.port);
            if (m) l.to = { node: m.node, port: m.port };
        } else if (l.from.node === node.id && l.from.port === 'out' && outMap) {
            l.from = { node: outMap.node, port: outMap.port };
        }
    });
    state.nodes = state.nodes.filter(n => n.id !== node.id);
    restored.forEach(n => state.nodes.push(n));
    (sg.links || []).forEach(l => state.links.push({ id: 'l' + seg(), from: { node: l.from.node, port: l.from.port }, to: { node: l.to.node, port: l.to.port } }));
    clearSelection();
    renderGraph();
    toast('已展开复合节点（' + restored.length + ' 节点）', 'success');
}

// ==== 复合节点库（保存到「节点模块 → 我的复合」） ====
// 深拷贝复合节点参数，并为子图节点重建新 ID（含 inMap/outMap 的节点引用），
// 保证同一模板生成的多个复合节点并行执行时子图互不串扰（nodeById 按视图栈顶优先查找）
function cloneCompositeParams(params) {
    const sg = params.subgraph || {};
    const idMap = {};
    const nodes = (sg.nodes || []).map(n => {
        const nid = newId(); idMap[n.id] = nid;
        return { id: nid, type: n.type, x: n.x, y: n.y, params: JSON.parse(JSON.stringify(n.params || {})) };
    });
    const links = (sg.links || []).map(l => ({
        id: 'l' + seg(),
        from: { node: idMap[l.from.node] || l.from.node, port: l.from.port },
        to: { node: idMap[l.to.node] || l.to.node, port: l.to.port }
    }));
    return {
        label: params.label || '复合节点', hint: params.hint || '封装的子流程',
        subgraph: { nodes, links },
        inMap: (params.inMap || []).map(m => Object.assign({}, m, { node: idMap[m.node] || m.node })),
        outMap: params.outMap ? Object.assign({}, params.outMap, { node: idMap[params.outMap.node] || params.outMap.node }) : null
    };
}
// 把画布上的复合节点收藏到节点模块列表（重名覆盖）
function saveCompositeToLibrary(node, name) {
    const params = JSON.parse(JSON.stringify(node.params || {}));
    const idx = state.savedComposites.findIndex(c => c.name === name);
    if (idx >= 0) state.savedComposites[idx] = { name, params };
    else state.savedComposites.push({ name, params });
    saveComposites().then(ok => { if (ok) { toast('已保存复合节点「' + name + '」到节点模块', 'success'); renderNodeModal(); } });
}
// 从节点模块列表移除
function removeSavedComposite(name) {
    if (!confirm('确定从节点模块删除「' + name + '」？')) return;
    state.savedComposites = state.savedComposites.filter(c => c.name !== name);
    saveComposites().then(ok => { if (ok) { toast('已删除「' + name + '」', 'success'); renderNodeModal(); } });
}
// 按收藏的复合节点生成一个可编辑的复合节点加入画布（子图 ID 重映射，多个实例互不干扰）
function instantiateSavedComposite(saved, x, y) {
    const params = cloneCompositeParams(saved.params || {});
    const n = { id: newId(), type: 'composite', x, y, params, running: false, _s: 'pending', outValue: null, enabled: true };
    state.nodes.push(n);
    renderGraph();
    return n;
}

// ==== 拖动 ====
function startDragNode(e, node) {
    if (e.target.closest('.port') || e.target.closest('.node-x')) return;
    e.preventDefault();
    const startX = e.clientX, startY = e.clientY, ox = node.x, oy = node.y;
    const move = ev => { node.x = Math.max(10, ox + (ev.clientX - startX)); node.y = Math.max(10, oy + (ev.clientY - startY)); renderNodes(); renderLinks(); };
    const up = () => {
        window.removeEventListener('mousemove', move); window.removeEventListener('mouseup', up);
        // 松开鼠标时把节点位置吸附到 10 的倍数（网格对齐）
        node.x = Math.max(10, Math.round(node.x / 10) * 10);
        node.y = Math.max(10, Math.round(node.y / 10) * 10);
        renderNodes(); renderLinks();
    };
    window.addEventListener('mousemove', move); window.addEventListener('mouseup', up);
}
function startLinkDrag(e, nodeId, port) {
    e.preventDefault(); e.stopPropagation();
    const aEl = e.currentTarget, a = portPosOf(aEl);
    state.linkDrag = { from: { node: nodeId, port }, x: a.x, y: a.y };
    renderLinks();
    const move = ev => { const r = canvas.getBoundingClientRect(); state.linkDrag.x = ev.clientX - r.left + canvas.scrollLeft; state.linkDrag.y = ev.clientY - r.top + canvas.scrollTop; renderLinks(); };
    const up = ev => {
        window.removeEventListener('mousemove', move); window.removeEventListener('mouseup', up);
        const inEl = document.elementFromPoint(ev.clientX, ev.clientY);
        const p = inEl && (inEl.closest ? inEl.closest('.port-in') : null);
        if (p) { const to = { node: p.dataset.node, port: p.dataset.port }; ensureLink(state.linkDrag.from, to); }
        state.linkDrag = { from: null, x: 0, y: 0 }; renderLinks();
    };
    window.addEventListener('mousemove', move); window.addEventListener('mouseup', up);
}
function tryDropLink(e, nodeId, port) { /* 输入端口也响应鼠标释放做兜底命中 */ e.preventDefault(); }