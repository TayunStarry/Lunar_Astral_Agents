// ==== 画布 / 复合节点库 持久化 ====
// 画布以 JSON 文件保存到 local_data/database/engine_graph/<名称>.json（多画布：可新建/命名/重命名/删除/切换）
// 复合节点库保存到 database/lunar.engine_manager_ltp9.composites.json（供「节点模块 → 我的复合」复用）
function canvasPath(name) { return GRAPH_DIR + '/' + name + '.json'; }
// URL 路径编码：保留斜杠，其余字符转义（中文画布名可用）
function encPath(p) { return encodeURIComponent(p).replace(/%2F/gi, '/'); }
// 画布名清洗：去除路径分隔符等非法字符
function sanitizeName(name) { return String(name || '').trim().replace(/[\\/:*?"<>|]/g, ''); }

function graphJson() {
    const ids = new Set(state.nodes.map(n => n.id)); // 保存时过滤无效连线，防止坏线入库
    return JSON.stringify({
        nodes: state.nodes.map(n => ({ id: n.id, type: n.type, x: n.x, y: n.y, params: n.params, enabled: n.enabled })),
        links: state.links.filter(l => l && l.from && ids.has(l.from.node) && l.to && ids.has(l.to.node)).map(l => ({ id: l.id, from: l.from, to: l.to }))
    }, null, 2);
}

// ==== 画布文件底层操作 ====
async function writeCanvas(name, content) {
    try {
        const resp = await fetch('/file/write', {
            method: 'POST',
            headers: {
                'X-File-Name': btoa(unescape(encodeURIComponent(canvasPath(name)))),
                'X-Overwrite': 'true'
            },
            body: new Blob([content], { type: 'application/json' })
        });
        if (!resp.ok) { toast('保存失败: ' + resp.status, 'error'); return false; }
        return true;
    } catch (e) { toast('保存异常: ' + e.message, 'error'); return false; }
}
async function readCanvasRaw(name) {
    try {
        const resp = await fetch('/file/read/' + encPath(canvasPath(name)));
        if (!resp.ok) return null;
        return await resp.text();
    } catch (e) { return null; }
}
async function readCanvasJson(name) {
    const t = await readCanvasRaw(name);
    if (t === null) return null;
    try { return JSON.parse(t); } catch (e) { toast('画布文件解析失败: ' + name, 'error'); return null; }
}
async function deleteCanvasFile(name) {
    try {
        const resp = await fetch('/file/delete/' + encPath(canvasPath(name)), { method: 'DELETE' });
        if (!resp.ok) { toast('删除失败: ' + resp.status, 'error'); return false; }
        return true;
    } catch (e) { toast('删除异常: ' + e.message, 'error'); return false; }
}
// 列出 engine_graph 目录下所有画布（按名称排序，忽略子目录与非 json）
// 注意：/file/list 返回字段为小写 json 标签（name/isDir/size/lastModified/path）
async function listCanvases() {
    try {
        const resp = await fetch('/file/list/' + GRAPH_DIR, { method: 'POST' });
        if (!resp.ok) { state.canvases = []; return; }
        const list = await resp.json();
        state.canvases = (list || [])
            .filter(f => !f.isDir && String(f.name).toLowerCase().endsWith('.json'))
            .map(f => ({ name: f.name.replace(/\.json$/i, ''), size: f.size, modified: f.lastModified, path: f.path }))
            .sort((a, b) => a.name.localeCompare(b.name, 'zh-CN'));
    } catch (e) { state.canvases = []; }
}
function renderCanvasLabel() { const el = $('canvasNameLabel'); if (el) el.textContent = state.canvasName || '未命名'; }

// ==== 画布管理动作 ====
// 保存当前画布到当前名称（未命名时先询问）
async function save() {
    if (!state.canvasName) {
        const name = sanitizeName(prompt('当前画布未命名，请输入画布名称', '默认画布'));
        if (!name) return;
        state.canvasName = name;
    }
    if (!(await writeCanvas(state.canvasName, graphJson()))) return;
    if (!state.canvases.some(c => c.name === state.canvasName)) state.canvases.push({ name: state.canvasName });
    localStorage.setItem('ltp9.lastCanvas', state.canvasName);
    renderCanvasLabel();
    toast('画布已保存 → ' + canvasPath(state.canvasName), 'success');
}
async function loadCanvas(name) {
    const g = await readCanvasJson(name);
    if (!g) { toast('加载画布失败（文件缺失或损坏）', 'error'); return; }
    state.canvasName = name;
    localStorage.setItem('ltp9.lastCanvas', name);
    applyGraph(g);
    renderCanvasLabel();
    toast('已加载画布「' + name + '」', 'success');
}
// 新建空画布并切换到它
async function createCanvas(name) {
    if (!(await writeCanvas(name, JSON.stringify({ nodes: [], links: [] })))) return;
    state.canvasName = name;
    localStorage.setItem('ltp9.lastCanvas', name);
    state.nodes = []; state.links = [];
    renderGraph();
    renderCanvasLabel();
    toast('已新建画布「' + name + '」', 'success');
}
async function renameCanvas(oldName, newName) {
    if (newName === oldName) return;
    if (state.canvases.some(c => c.name === newName)) { toast('画布「' + newName + '」已存在', 'error'); return; }
    const raw = await readCanvasRaw(oldName);
    if (raw === null) { toast('读取原画布失败', 'error'); return; }
    if (!(await writeCanvas(newName, raw))) return;
    if (!(await deleteCanvasFile(oldName))) { await deleteCanvasFile(newName); return; } // 删除失败则回滚新建的文件
    state.canvases = state.canvases.map(c => c.name === oldName ? Object.assign({}, c, { name: newName }) : c);
    if (state.canvasName === oldName) { state.canvasName = newName; localStorage.setItem('ltp9.lastCanvas', newName); renderCanvasLabel(); }
    toast('已重命名为「' + newName + '」', 'success');
}
async function deleteCanvas(name) {
    if (!(await deleteCanvasFile(name))) return;
    state.canvases = state.canvases.filter(c => c.name !== name);
    if (state.canvasName === name) {
        state.canvasName = null;
        if (state.canvases.length) { await loadCanvas(state.canvases[0].name); }
        else { state.nodes = []; state.links = []; renderGraph(); renderCanvasLabel(); toast('画布已删除，当前为空画布', 'success'); }
    } else { toast('已删除画布「' + name + '」', 'success'); }
}
// 首次启动：有画布则载入上次使用（或第一个）画布；无画布则用示例画布输出为「默认画布」并落盘
async function initCanvas() {
    await listCanvases();
    if (state.canvases.length) {
        const last = localStorage.getItem('ltp9.lastCanvas');
        await loadCanvas(state.canvases.some(c => c.name === last) ? last : state.canvases[0].name);
        return;
    }
    state.canvasName = '默认画布';
    renderCanvasLabel();
    demoGraph();
    await writeCanvas(state.canvasName, graphJson()); // 示例画布落盘为默认画布，之后可在画布管理里看到/重命名
    await listCanvases();
}

// ==== 画布管理浮窗（复用参数浮窗组件） ====
function canvasActionBtn(icon, title, fn) {
    const b = document.createElement('button'); b.className = 'btn-ghost btn-mini'; b.innerHTML = '<i class="fas ' + icon + '"></i>'; b.title = title;
    b.addEventListener('click', e => { e.stopPropagation(); fn(); });
    return b;
}
async function openCanvasManager() {
    await listCanvases();
    clearNodePanel();
    modalTop.innerHTML = '';
    const t = document.createElement('span'); t.className = 'modal-title';
    t.innerHTML = '<i class="fas fa-map"></i> 画布管理' + (state.canvasName ? ' · 当前：' + escHtml(state.canvasName) : '');
    t.title = '画布文件存放于 ' + GRAPH_DIR;
    modalTop.appendChild(t); modalTop.appendChild(closeBtn());
    modalBody.innerHTML = '';
    const wrap = document.createElement('div'); wrap.className = 'form';
    // 新建画布
    const newField = document.createElement('div'); newField.className = 'field';
    const lab = document.createElement('label'); lab.className = 'field-label'; lab.textContent = '新建画布（空白画布，命名后自动切换）';
    const row = document.createElement('div'); row.className = 'row-inline';
    const inp = document.createElement('input'); inp.className = 'inp'; inp.placeholder = '输入画布名称';
    const newBtn = document.createElement('button'); newBtn.className = 'btn-primary btn-mini'; newBtn.innerHTML = '<i class="fas fa-plus"></i> 新建';
    inp.addEventListener('keydown', e => { if (e.key === 'Enter') newBtn.click(); });
    newBtn.addEventListener('click', async () => {
        const name = sanitizeName(inp.value);
        if (!name) { toast('请输入画布名称', 'error'); return; }
        if (state.canvases.some(c => c.name === name)) { toast('画布「' + name + '」已存在', 'error'); return; }
        await createCanvas(name);
        openCanvasManager();
    });
    row.appendChild(inp); row.appendChild(newBtn);
    newField.appendChild(lab); newField.appendChild(row);
    wrap.appendChild(newField);
    // 画布列表
    if (!state.canvases.length) {
        const tip = document.createElement('div'); tip.className = 'empty-tip'; tip.textContent = '暂无画布，可新建，或在顶栏点保存生成当前画布'; wrap.appendChild(tip);
    }
    state.canvases.forEach(c => {
        const r2 = document.createElement('div'); r2.className = 'canvas-row' + (c.name === state.canvasName ? ' active' : '');
        const nm = document.createElement('span'); nm.className = 'canvas-name'; nm.textContent = c.name; nm.title = canvasPath(c.name);
        r2.appendChild(nm);
        const acts = document.createElement('span'); acts.className = 'canvas-actions';
        acts.appendChild(canvasActionBtn('fa-folder-open', '加载此画布（替换当前画布内容）', async () => { await loadCanvas(c.name); closeModal(); }));
        acts.appendChild(canvasActionBtn('fa-pen', '重命名', async () => {
            const nn = sanitizeName(prompt('重命名画布', c.name));
            if (!nn || nn === c.name) return;
            await renameCanvas(c.name, nn);
            openCanvasManager();
        }));
        acts.appendChild(canvasActionBtn('fa-trash-alt', '删除此画布（不可恢复）', async () => {
            if (!confirm('确定删除画布「' + c.name + '」？此操作不可恢复')) return;
            await deleteCanvas(c.name);
            openCanvasManager();
        }));
        r2.appendChild(acts);
        wrap.appendChild(r2);
    });
    modalBody.appendChild(wrap);
    modal.classList.add('open');
}

// ==== 复合节点库（保存到「节点模块 → 我的复合」） ====
async function loadComposites() {
    try {
        const resp = await fetch('/file/read/' + COMPOSITES_FILE);
        if (!resp.ok) { state.savedComposites = []; return; }
        const data = await resp.json();
        state.savedComposites = (data && Array.isArray(data.composites)) ? data.composites : [];
    } catch (e) { state.savedComposites = []; }
}
async function saveComposites() {
    try {
        const resp = await fetch('/file/write', {
            method: 'POST',
            headers: {
                'X-File-Name': btoa(unescape(encodeURIComponent(COMPOSITES_FILE))),
                'X-Overwrite': 'true'
            },
            body: new Blob([JSON.stringify({ composites: state.savedComposites }, null, 2)], { type: 'application/json' })
        });
        if (!resp.ok) { toast('保存复合节点库失败: ' + resp.status, 'error'); return false; }
        return true;
    } catch (e) { toast('保存复合节点库异常: ' + e.message, 'error'); return false; }
}

// ==== 载入 / 清空 / 示例 ====
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
// 「选择画布」与「加载画布」为同一功能：均打开画布管理浮窗（由顶栏画布按钮触发，见 openCanvasManager）
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
