// ==== 事件绑定 ====
// 节点模块分类：类型 → 分页
const NODE_CATS = [
    { key: 'start', label: '入口启动', types: ['start', 'event_start', 'clock_start'] },
    { key: 'engine', label: '引擎交互', types: ['event', 'broadcast_all', 'broadcast_target', 'call', 'tool', 'command', 'agent', 'platform', 'async'] },
    { key: 'data', label: '数据安全', types: ['db', 'memory', 'file', 'crypto', 'base64', 'jwt', 'emoji'] },
    { key: 'ai', label: '网络通讯', types: ['llm', 'embed', 'http', 'network'] },
    { key: 'media', label: '语音系统', types: ['kokoro_tts', 'qwen_tts', 'qwen_asr', 'microphone', 'speaker', 'audio_eq'] },
    { key: 'video', label: '视频处理', types: ['video_keyframe'] },
    { key: 'flow', label: '流程处理', types: ['wait', 'transform', 'extract', 'display', 'image_display', 'image_confuse', 'stats', 'limit'] },
    { key: 'gates', label: '逻辑控制', types: ['and', 'or', 'not', 'nand', 'nor'] },
    { key: 'composites', label: '我的复合', types: [] } // 用户保存的复合节点（非节点类型，单独渲染）
];
let nodeCatActive = 'start';
function openNodeModalPanel() { $('nodeSidebar').classList.add('open'); renderNodeModal(); }
function closeNodeModalPanel() { $('nodeSidebar').classList.remove('open'); }
function toggleNodeSidebar() { const s = $('nodeSidebar'); s.classList.toggle('open'); if (s.classList.contains('open')) { closeLogModal(); renderNodeModal(); } } // 两个侧栏互斥，打开节点模块时收起日志
function renderNodeModal() {
    const tabs = $('nodeTabs'), body = $('nodeContent');
    tabs.innerHTML = '';
    NODE_CATS.forEach(c => {
        const b = document.createElement('button'); b.className = 'node-tab' + (c.key === nodeCatActive ? ' active' : '');
        b.textContent = c.label;
        b.addEventListener('click', () => { nodeCatActive = c.key; renderNodeModal(); });
        tabs.appendChild(b);
    });
    body.innerHTML = '';
    const cat = NODE_CATS.find(c => c.key === nodeCatActive) || NODE_CATS[0];
    if (cat.key === 'composites') {
        // 「我的复合」：展示用户保存的复合节点（点击加入画布，可拖拽到指定位置）
        if (!state.savedComposites.length) {
            const tip = document.createElement('div'); tip.className = 'empty-tip';
            tip.textContent = '暂无保存的复合节点\n在复合节点参数面板点「保存到节点模块」即可收藏复用';
            body.appendChild(tip);
            return;
        }
        state.savedComposites.forEach(c => {
            const chip = document.createElement('div'); chip.className = 'node-chip';
            chip.draggable = true;
            chip.title = '点击加入画布，或拖拽到画布指定位置';
            const icon = document.createElement('i'); icon.className = 'fas fa-object-group'; icon.style.color = '#a78bfa';
            chip.appendChild(icon);
            const label = document.createElement('span'); label.textContent = c.name; chip.appendChild(label);
            const del = document.createElement('span'); del.className = 'chip-del'; del.innerHTML = '<i class="fas fa-trash-alt"></i>'; del.title = '从节点模块删除';
            del.addEventListener('click', e => { e.stopPropagation(); removeSavedComposite(c.name); });
            chip.appendChild(del);
            chip.addEventListener('dragstart', e => {
                e.dataTransfer.setData('text/plain', 'composite:' + c.name);
                e.dataTransfer.effectAllowed = 'copy';
            });
            chip.addEventListener('click', () => {
                const n = instantiateSavedComposite(c, 80 + Math.random() * 120, 80 + Math.random() * 80);
                openNodeModal(n);
            });
            body.appendChild(chip);
        });
        return;
    }
    cat.types.forEach(type => {
        const meta = metaOf(type);
        const chip = document.createElement('div'); chip.className = 'node-chip';
        chip.draggable = true; // 支持拖拽到画布释放
        chip.title = (meta.desc || '') + '\n拖拽到画布释放，或点击快速添加';
        const icon = document.createElement('i'); icon.className = 'fas ' + meta.icon; icon.style.color = meta.color || 'var(--accent)';
        chip.appendChild(icon);
        const label = document.createElement('span'); label.textContent = meta.label; chip.appendChild(label);
        chip.addEventListener('dragstart', e => {
            e.dataTransfer.setData('text/plain', type);
            e.dataTransfer.effectAllowed = 'copy';
        });
        chip.addEventListener('click', () => {
            const n = addNode(type, 80 + Math.random() * 120, 80 + Math.random() * 80);
            openNodeModal(n);
        });
        body.appendChild(chip);
    });
}
function bindNodeModal() {
    $('nodeFab').addEventListener('click', toggleNodeSidebar);
}
function bindActions() {
    $('runBtn').addEventListener('click', () => { if (state.running || state.clocks.length) stopRun(); else runAll(); });
    $('demoBtn').addEventListener('click', demoGraph);
    $('saveBtn').addEventListener('click', save);
    $('canvasBtn').addEventListener('click', openCanvasManager); // 选择与加载画布为同一功能（打开画布管理）
    $('clearBtn').addEventListener('click', clearGraph);
    $('compositeBtn').addEventListener('click', encapsulateSelected);
    $('reconnectBtn').addEventListener('click', () => { state.retry = 0; if (state.ws) { try { state.ws.close(); } catch (e) {} } connect(); });
    $('logFilters').addEventListener('click', e => { const chip = e.target.closest('.filter-chip'); if (!chip) return; document.querySelectorAll('.filter-chip').forEach(c => c.classList.remove('active')); chip.classList.add('active'); state.filter = chip.dataset.filter; renderLog(); });
    $('clearLogBtn').addEventListener('click', () => { state.log = []; renderLog(); });
    $('logFab').addEventListener('click', toggleLogSidebar);
    // 节点模块：拖拽节点芯片到画布释放，生成节点（含「我的复合」收藏的复合节点）
    const canvasEl = $('canvas');
    canvasEl.addEventListener('dragover', e => { e.preventDefault(); e.dataTransfer.dropEffect = 'copy'; canvasEl.classList.add('drag-over'); });
    canvasEl.addEventListener('dragleave', e => { if (!canvasEl.contains(e.relatedTarget)) canvasEl.classList.remove('drag-over'); });
    canvasEl.addEventListener('drop', e => {
        e.preventDefault(); canvasEl.classList.remove('drag-over');
        const type = e.dataTransfer.getData('text/plain');
        if (!type) return;
        const r = canvasEl.getBoundingClientRect();
        const x = e.clientX - r.left + canvasEl.scrollLeft - NODE_W / 2;
        const y = e.clientY - r.top + canvasEl.scrollTop - 15;
        if (type.startsWith('composite:')) {
            const name = type.slice('composite:'.length);
            const saved = state.savedComposites.find(c => c.name === name);
            if (!saved) { toast('未找到复合节点「' + name + '」', 'error'); return; }
            instantiateSavedComposite(saved, Math.max(10, x), Math.max(10, y));
            return;
        }
        if (!NODES[type]) return;
        addNode(type, Math.max(10, x), Math.max(10, y));
    });
    initDragParamPop();
    document.addEventListener('keydown', e => { if (e.key === 'Escape') { closeModal(); closeLogModal(); closeNodeModalPanel(); } });
}

// ==== 启动 ====
async function init() {
    bindNodeModal(); bindActions();
    renderGraph(); renderLog(); renderWs(); updateFab(); renderCanvasLabel();
    connect();
    await loadComposites();
    await initCanvas();
}
init();