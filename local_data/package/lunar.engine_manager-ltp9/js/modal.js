// ==== 参数编辑（浮窗面板） ====
function clearNodePanel() { nodesLayer.querySelectorAll('.node.panel-active').forEach(el => el.classList.remove('panel-active')); }
function openNodeModal(node) {
    const meta = metaOf(node.type);
    clearNodePanel();
    modalTop.innerHTML = '';
    const t = document.createElement('span'); t.className = 'modal-title';
    t.innerHTML = '<i class="fas ' + meta.icon + '" style="color:' + meta.color + '"></i> ' + meta.label + ' · 参数'; t.title = meta.desc;
    modalTop.appendChild(t);
    const dup = document.createElement('button'); dup.className = 'btn-ghost btn-mini'; dup.innerHTML = '<i class="fas fa-copy"></i> 复制';
    dup.addEventListener('click', () => { const c = duplicateNode(node.id); closeModal(); if (c) openNodeModal(c); });
    modalTop.appendChild(dup);
    const run = document.createElement('button'); run.className = 'btn-primary btn-mini'; run.innerHTML = '<i class="fas fa-play"></i> 仅运行';
    run.addEventListener('click', () => { closeModal(); runSingle(node.id); });
    modalTop.appendChild(run); modalTop.appendChild(closeBtn());
    modalBody.innerHTML = '';
    const wrap = document.createElement('div'); wrap.className = 'form';
    if (meta.desc) { const d = document.createElement('div'); d.className = 'field-hint'; d.textContent = meta.desc; wrap.appendChild(d); }
    const hint = document.createElement('div'); hint.className = 'field-hint';
    hint.innerHTML = '端口：上方=输入、下方=输出；<时钟线>决定执行链，其余为数据信号线。拖动标题栏可移动本面板。';
    wrap.appendChild(hint);
    (meta.fields || []).forEach(f => wrap.appendChild(buildField(f, node.params)));
    if (node.type === 'composite') { // 复合节点：展示内部概要 + 展开入口
        const sg = node.params.subgraph || {};
        const info = document.createElement('div'); info.className = 'field-hint';
        info.textContent = '内部封装：' + (sg.nodes || []).length + ' 个节点 / ' + (sg.links || []).length + ' 条连线；输入 '
            + ((node.params.inMap || []).length) + ' 路、输出 ' + (node.params.outMap ? 1 : 0) + ' 路。点「展开」还原为原始节点，修改后可重新封装。';
        wrap.appendChild(info);
        const ex = document.createElement('button'); ex.className = 'btn-ghost'; ex.innerHTML = '<i class="fas fa-box-open"></i> 展开复合节点';
        ex.addEventListener('click', () => { closeModal(); unpackComposite(node); });
        wrap.appendChild(ex);
        const lib = document.createElement('button'); lib.className = 'btn-ghost'; lib.innerHTML = '<i class="fas fa-bookmark"></i> 保存到节点模块';
        lib.title = '把当前复合节点收藏到左侧「节点模块 → 我的复合」，可随时加入任意画布（重名会覆盖）';
        lib.addEventListener('click', () => {
            const name = prompt('保存为节点模块名称（重名会覆盖）', node.params.label || '复合节点');
            if (!name || !name.trim()) return;
            saveCompositeToLibrary(node, name.trim());
        });
        wrap.appendChild(lib);
    }
    modalBody.appendChild(wrap);
    modal.classList.add('open');
    state.modalNodeId = node.id;
    // 切换「目标包」后重渲染表单：让「函数名」下拉跟随所选包的真实导出变化
    wrap.querySelectorAll('input[data-dynamic="plugins"]').forEach(inp => {
        inp.addEventListener('change', () => {
            const n = nodeById(state.modalNodeId); if (n) openNodeModal(n);
        });
    });
    const card = nodesLayer.querySelector('.node[data-id="' + node.id + '"]');
    if (card) card.classList.add('panel-active'); // 被设置节点闪烁
}
// 字段的可选值：dynamic 选项从引擎真实注册信息（state.catalog）取，静态 options 兜底
function fieldOptions(f, p) {
    if (f.dynamic === 'plugins') return state.catalog.plugins;
    if (f.dynamic === 'agentPlugins') return state.catalog.agentPlugins;
    if (f.dynamic === 'events') return state.catalog.events;
    if (f.dynamic === 'exports') return (state.catalog.exports[(p && p.plugin) || ''] || []);
    return f.options || [];
}
function buildField(f, p) {
    const box = document.createElement('div'); box.className = 'field';
    const lab = document.createElement('label'); lab.className = 'field-label'; lab.textContent = f.label; box.appendChild(lab);
    const setter = v => { p[f.key] = v; };
    let ctl;
    switch (f.type) {
        case 'select': {
            const sel = document.createElement('select'); sel.className = 'sel';
            fieldOptions(f, p).forEach(o => { const op = document.createElement('option'); op.value = o; op.textContent = o; if (String(p[f.key]) === o) op.selected = true; sel.appendChild(op); });
            sel.addEventListener('change', () => setter(sel.value)); ctl = sel; break;
        }
        case 'number': {
            ctl = document.createElement('input'); ctl.type = 'number'; ctl.className = 'inp'; ctl.value = p[f.key] != null ? p[f.key] : (f.def != null ? f.def : '');
            ctl.addEventListener('input', () => setter(Number(ctl.value))); break;
        }
        case 'json': case 'textarea': {
            const ta = document.createElement('textarea'); ta.className = 'ta' + (f.type === 'json' ? ' json' : ''); ta.rows = f.rows || 2; ta.spellcheck = false;
            ta.value = p[f.key] != null && typeof p[f.key] === 'string' ? p[f.key] : (p[f.key] != null ? JSON.stringify(p[f.key]) : (f.def != null ? f.def : ''));
            // 按换行符数量决定高度：每多一个换行就多加一行高度（上限 320px，超出滚动）
            const lineH = 18, pad = 16;
            const autosize = () => {
                const lines = Math.max(1, String(ta.value).split('\n').length);
                ta.style.height = Math.min(lines * lineH + pad, 320) + 'px';
            };
            ta.addEventListener('input', () => { setter(ta.value); autosize(); });
            autosize();
            ctl = ta; break;
        }
        default: {
            const inp = document.createElement('input'); inp.type = 'text'; inp.className = 'inp';
            inp.value = p[f.key] != null && typeof p[f.key] === 'string' ? p[f.key] : (p[f.key] != null ? JSON.stringify(p[f.key]) : (f.def != null ? f.def : ''));
            inp.placeholder = f.ph || '';
            const opts = fieldOptions(f, p);
            if (opts.length) { const dlBase = document.createElement('datalist'); dlBase.id = 'dl_' + (++state.seq); opts.forEach(o => { const op = document.createElement('option'); op.value = o; dlBase.appendChild(op); }); inp.setAttribute('list', dlBase.id); document.body.appendChild(dlBase); }
            if (f.dynamic) inp.dataset.dynamic = f.dynamic; // 供「切换包后重渲染」定位
            inp.addEventListener('input', () => setter(inp.value)); ctl = inp; break;
        }
    }
    box.appendChild(ctl); return box;
}
function closeBtn() { const b = document.createElement('button'); b.className = 'modal-close'; b.innerHTML = '&times;'; b.title = '关闭'; b.addEventListener('click', closeModal); return b; }
function closeModal() { modal.classList.remove('open'); clearNodePanel(); modalTop.innerHTML = ''; modalBody.innerHTML = ''; state.modalNodeId = null; }
function initDragParamPop() {
    const el = modal, head = modalTop; if (!head) return;
    head.addEventListener('mousedown', ev => {
        if (ev.target.closest('button')) return;
        const r = el.getBoundingClientRect(), dx = ev.clientX - r.left, dy = ev.clientY - r.top;
        const move = e2 => { el.style.left = (e2.clientX - dx) + 'px'; el.style.top = (e2.clientY - dy) + 'px'; el.style.right = 'auto'; };
        const up = () => { window.removeEventListener('mousemove', move); window.removeEventListener('mouseup', up); };
        window.addEventListener('mousemove', move); window.addEventListener('mouseup', up);
    });
}
function openDetail(title, payload) {
    clearNodePanel();
    modalTop.innerHTML = '';
    const t = document.createElement('span'); t.className = 'modal-title'; t.textContent = title;
    modalTop.appendChild(t); modalTop.appendChild(closeBtn());
    modalBody.innerHTML = '';
    const pre = document.createElement('pre'); pre.className = 'modal-json'; pre.textContent = JSON.stringify(payload, null, 2);
    modalBody.appendChild(pre); modal.classList.add('open');
}