// ==== 节点渲染（含输入/输出端口） ====
// 节点卡片动作按钮辅助：消息气泡 / 图片显示 / 音频播放 / 音频输入。
// 依 magic bytes 识别二进制 base64 的图片与音频，既可据输出实时检测，也支持按节点 out.kind 预置按钮。
function sniffMime(bin) {
    const b = i => bin.charCodeAt(i);
    if (bin.length > 12 && b(0) === 0x52 && bin.slice(8, 12) === 'WAVE') return 'audio/wav';
    if (bin.slice(0, 4) === 'fLaC') return 'audio/flac';
    if (bin.slice(0, 4) === 'OggS') return 'audio/ogg';
    if (bin.slice(0, 4) === 'ID3' || (b(0) === 0xff && (b(1) & 0xe0) === 0xe0)) return 'audio/mpeg';
    if (bin.slice(0, 4) === '\x1aE\xdf\xa3') return 'audio/webm';
    if (b(0) === 0x89 && bin.slice(1, 4) === 'PNG') return 'image/png';
    if (b(0) === 0xff && b(1) === 0xd8 && b(2) === 0xff) return 'image/jpeg';
    if (bin.slice(0, 6) === 'GIF87a' || bin.slice(0, 6) === 'GIF89a') return 'image/gif';
    if (bin.slice(0, 4) === 'RIFF' && bin.slice(8, 12) === 'WEBP') return 'image/webp';
    return null;
}
// 递归扫描输出值，提取 { image, audio }（data URI 形式）
function findMedia(v, found) {
    found = found || {};
    if (!v) return found;
    if (typeof v === 'string') {
        const t = v.trim();
        if (/^data:image\//i.test(t)) { if (!found.image) found.image = t; return found; }
        if (/^data:audio\//i.test(t)) { if (!found.audio) found.audio = t; return found; }
        if (t.length > 60 && t.length % 4 === 0 && /^[A-Za-z0-9+/=\s]+$/.test(t)) {
            try {
                const bin = atob(t);
                const mime = sniffMime(bin);
                if (mime && !found[mime.startsWith('image') ? 'image' : 'audio']) found[mime.startsWith('image') ? 'image' : 'audio'] = 'data:' + mime + ';base64,' + t;
            } catch (e) { /* 非合法 base64，忽略 */ }
        }
        return found;
    }
    if (Array.isArray(v) || (typeof v === 'object')) Object.values(v).forEach(i => findMedia(i, found));
    return found;
}
// 节点应提供的媒体按钮：按 out.kind 预置（即便未运行也显示），并兜底实时检测输出里的媒体
function mediaButtonsFor(node) {
    const meta = metaOf(node.type);
    const md = node.outValue ? findMedia(node.outValue) : null;
    const wantImg = meta.out && meta.out.kind === 'image';
    const wantAud = meta.out && meta.out.kind === 'audio';
    const b = [];
    if (wantImg || (md && md.image)) b.push({ kind: 'image', dataUri: (md && md.image) || null });
    if (wantAud || (md && md.audio)) b.push({ kind: 'audio', dataUri: (md && md.audio) || null });
    return b;
}
function nodeEl(node) { return nodesLayer.querySelector('.node[data-id="' + node.id + '"]'); }
function closeNodePop(node) { const el = nodeEl(node); if (el) el.querySelectorAll('.node-pop').forEach(p => p.remove()); }
// 消息气泡/图片展示：点击图标在节点上方弹出，再点或点别处关闭
function toggleNodePop(node, html) {
    const el = nodeEl(node); if (!el) return;
    closeNodePop(node);
    const pop = document.createElement('div'); pop.className = 'node-pop';
    if (typeof html === 'string' && /^</.test(html)) pop.innerHTML = html; else pop.textContent = String(html);
    el.appendChild(pop);
    const off = e => { if (e.target.closest && e.target.closest('.node-pop')) return; document.removeEventListener('click', off, true); closeNodePop(node); };
    document.addEventListener('click', off, true);
}
// 「文本显示」节点：气泡展示当前文本/输出内容（override 用于激活回放时传入运行值）
function showNodeBubble(node, override) {
    const v = override !== undefined ? override : (node.outValue !== undefined && node.outValue !== null ? node.outValue : node.params.text);
    const txt = typeof v === 'string' ? v : (v == null ? '' : JSON.stringify(v, null, 2));
    toggleNodePop(node, `<pre class="pop-text">${escHtml(txt) || '（空）'}</pre>`);
}
// 「图片显示」按钮：气泡展示节点输出里的图片
function showNodeImage(node, dataUri) {
    let d = dataUri;
    if (!d) { const md = node.outValue ? findMedia(node.outValue) : null; if (md && md.image) d = md.image; }
    if (!d) { toast('无图片输出，请先运行此节点', 'error'); return; }
    toggleNodePop(node, `<img class="pop-img" src="${d}">`);
}
// 「播放音频」按钮：播放节点输出里的音频（base64 → data URI）
function playNodeAudio(node, dataUri) {
    let d = dataUri;
    if (!d) { const md = node.outValue ? findMedia(node.outValue) : null; if (md && md.audio) d = md.audio; }
    if (!d) { toast('无音频输出，请先运行此节点', 'error'); return; }
    const a = new Audio(d);
    a.play().catch(() => toast('音频播放失败', 'error'));
}
// 回放显示类节点激活时记录的弹出消息：运行收尾统一补挂（渲染重建 DOM 会清掉早先弹出的气泡）
function replayDisplayPops() {
    state.nodes.forEach(n => {
        if (!n._pop) return;
        const pops = n._pop.kind === 'multi' ? n._pop.pops : [n._pop];
        n._pop = null;
        pops.forEach(p => { if (!p) return; if (p.kind === 'image') showNodeImage(n, p.v); else showNodeBubble(n, p.v); });
    });
}
// 「音频输入」按钮（qwen_asr）：选本地音频文件 → base64 写入本节点音频参数
function pickAudioInput(node) {
    const inp = document.createElement('input');
    inp.type = 'file'; inp.accept = 'audio/*,.wav,.mp3,.webm,.ogg,.m4a'; inp.style.display = 'none';
    inp.addEventListener('change', () => {
        const f = inp.files && inp.files[0]; if (!f) return;
        const fr = new FileReader();
        fr.onload = () => { node.params.audio = String(fr.result).split(',')[1]; toast(`已载入音频（base64 ${(node.params.audio.length / 1024).toFixed(0)}KB）`, 'success'); renderNodes(); };
        fr.onerror = () => toast('读取音频失败', 'error');
        fr.readAsDataURL(f);
    });
    document.body.appendChild(inp); inp.click(); inp.remove();
}
// 从任意输出值里取音频 data URI（递归 detect 图片/音频）
function audioDataUriOf(v) { const md = findMedia(v); return md ? md.audio : null; }
// 裸 base64 → data URI（按 magic bytes 推断 MIME；非法返回 null）
function b64ToDataUri(b64) {
    if (typeof b64 !== 'string' || !b64.trim()) return null;
    const t = b64.trim();
    try { const m = sniffMime(atob(t)); return m ? 'data:' + m + ';base64,' + t : null; } catch (e) { return null; }
}
// 麦克风录音：MediaRecorder 录制 duration 秒，返回 base64 音频（webm/opus）；失败/未授权返回 null
function recordAudio(durationSec) {
    return new Promise(resolve => {
        if (!navigator.mediaDevices || !navigator.mediaDevices.getUserMedia || typeof MediaRecorder === 'undefined') { resolve(null); return; }
        navigator.mediaDevices.getUserMedia({ audio: true }).then(stream => {
            let rec = null;
            try {
                if (MediaRecorder.isTypeSupported && MediaRecorder.isTypeSupported('audio/webm;codecs=opus')) rec = new MediaRecorder(stream, { mimeType: 'audio/webm;codecs=opus' });
                else rec = new MediaRecorder(stream);
            } catch (e) { try { rec = new MediaRecorder(stream); } catch (e2) { stream.getTracks().forEach(t => t.stop()); resolve(null); return; } }
            const chunks = [];
            rec.ondataavailable = e => { if (e.data && e.data.size) chunks.push(e.data); };
            rec.onstop = () => {
                stream.getTracks().forEach(t => t.stop());
                const blob = new Blob(chunks, { type: rec.mimeType || 'audio/webm' });
                const fr = new FileReader();
                fr.onload = () => resolve(String(fr.result).split(',')[1]);
                fr.onerror = () => resolve(null);
                fr.readAsDataURL(blob);
            };
            rec.onerror = () => { stream.getTracks().forEach(t => t.stop()); resolve(null); };
            rec.start();
            setTimeout(() => { if (rec.state !== 'inactive') rec.stop(); }, durationSec * 1000);
        }).catch(() => resolve(null));
    });
}
// 生成节点头部动作按钮（stopPropagation，避免误触发编辑/拖动）
function makeActionBtn(icon, title, fn) {
    const s = document.createElement('span'); s.className = 'node-act'; s.innerHTML = `<i class="fas ${icon}"></i>`; s.title = title;
    s.addEventListener('mousedown', e => e.stopPropagation());
    s.addEventListener('click', e => { e.stopPropagation(); fn(); });
    return s;
}

function renderGraph() { renderNodes(); renderLinks(); $('canvasHint').style.display = state.nodes.length ? 'none' : 'block'; }
function renderNodes() {
    nodesLayer.innerHTML = '';
    state.nodes.forEach(node => {
        const meta = metaOf(node.type);
        const ins = inPortsOf(node.type, node), outs = outPortsOf(node.type, node);
        const el = document.createElement('div');
        el.className = 'node' + statusClass(node, 'el') + (state.selected.has(node.id) ? ' selected' : '');
        el.style.left = node.x + 'px'; el.style.top = node.y + 'px'; el.style.width = NODE_W + 'px'; el.style.height = '100px';
        el.style.setProperty('--nc', meta.color);
        el.dataset.id = node.id;
        el.dataset.run = node._s || 'pending';
        const head = document.createElement('div'); head.className = 'node-head';
        // 复合节点用封装时的自定义名称作标题
        const headLabel = (node.type === 'composite' && node.params.label) ? node.params.label : meta.label;
        head.innerHTML = `<i class="fas ${meta.icon}"></i><span class="node-label">${escHtml(headLabel)}</span>`;
        // 节点动作按钮：展开复合 · 消息气泡（文本显示）· 音频输入（识别音频）· 图片显示 · 音频播放
        const acts = document.createElement('div'); acts.className = 'node-actions';
        if (node.type === 'composite') acts.appendChild(makeActionBtn('fa-box-open', '展开复合节点（还原内部节点）', () => unpackComposite(node)));
        if (node.type === 'display') acts.appendChild(makeActionBtn('fa-comment-dots', '查看文本内容（消息气泡）', () => showNodeBubble(node)));
        if (node.type === 'qwen_asr') acts.appendChild(makeActionBtn('fa-file-audio', '载入音频 → base64 注入本节点音频参数', () => pickAudioInput(node)));
        mediaButtonsFor(node).forEach(b => {
            if (b.kind === 'audio') acts.appendChild(makeActionBtn('fa-play', '播放本节点输出的音频', () => playNodeAudio(node, b.dataUri)));
            else acts.appendChild(makeActionBtn('fa-image', '显示本节点输出的图片', () => showNodeImage(node, b.dataUri)));
        });
        head.appendChild(acts);
        const copy = document.createElement('span'); copy.className = 'node-copy'; copy.innerHTML = '<i class="fas fa-copy"></i>'; copy.title = '复制节点（含参数与连线）';
        copy.addEventListener('click', e => { e.stopPropagation(); duplicateNode(node.id); });
        head.appendChild(copy);
        const close = document.createElement('span'); close.className = 'node-x'; close.innerHTML = '&times;'; close.title = '删除';
        close.addEventListener('click', e => { e.stopPropagation(); removeNode(node.id); });
        head.appendChild(close); head.addEventListener('mousedown', e => startDragNode(e, node));
        el.appendChild(head);
        const body = document.createElement('div'); body.className = 'node-body';
        body.textContent = statusText(node); el.appendChild(body);
        // 文本/图像显示节点：卡片仅显示描述标签（不受链路控制，用于说明本节点展示什么），内容经图标气泡查看
        if (node.type === 'display' || node.type === 'image_display') {
            const cap = document.createElement('div'); cap.className = 'pv pv-display';
            const fallback = node.type === 'display' ? '用于展示文本内容' : '用于展示图像内容';
            cap.textContent = String(node.params.hint != null && node.params.hint !== '' ? node.params.hint : fallback);
            body.appendChild(cap);
        } else if (node.type === 'composite') {
            const sg = node.params.subgraph || {};
            const cap = document.createElement('div'); cap.className = 'pv pv-display';
            const hint = node.params.hint != null && node.params.hint !== '' ? node.params.hint + ' · ' : '';
            cap.textContent = hint + '内部 ' + (sg.nodes || []).length + ' 节点 / ' + (sg.links || []).length + ' 线';
            body.appendChild(cap);
        }
        // 输入端口（顶部横排），输出端口（底部横排）
        ins.forEach((p, i) => el.appendChild(makePort(el, node.id, 'in', p, i, ins.length)));
        outs.forEach((p, i) => el.appendChild(makePort(el, node.id, 'out', p, i, outs.length)));
        el.addEventListener('click', e => {
            if (e.ctrlKey || e.metaKey) { toggleSelectNode(node.id); return; } // Ctrl+点击多选（封装复合节点）
            if (!el.getAttribute('data-dragging')) openNodeModal(node);
        });
        nodesLayer.appendChild(el);
    });
}
function statusClass(node, kind) {
    const s = node._s || 'pending';
    if (node.running) return ' running';
    if (s === 'ok') return ' done';
    if (s === 'err') return ' done error';
    if (s === 'skip') return ' skip';
    if (s === 'idle') return ' idle';
    return '';
}
function statusText(node) {
    const s = node._s || 'pending';
    if (node.running) return '运行中…';
    if (s === 'ok') return '成功';
    if (s === 'err') return '错误';
    if (s === 'skip') return '跳过';
    if (s === 'idle') return '就绪';
    return '待运行';
}
function makePort(el, nodeId, side, p, i, n) {
    const pt = document.createElement('div');
    pt.className = 'port port-' + (side === 'in' ? 'in' : 'out');
    pt.dataset.node = nodeId; pt.dataset.side = side; pt.dataset.port = p.k;
    pt.title = (side === 'in' ? '输入' : '输出') + '：' + p.label;
    // 上下端口：横向均分
    pt.style.left = ((i + 1) * NODE_W / (n + 1) - 6) + 'px';
    if (side === 'out') pt.addEventListener('mousedown', e => startLinkDrag(e, nodeId, p.k));
    else pt.addEventListener('mouseup', e => tryDropLink(e, nodeId, p.k));
    return pt;
}
function portPosOf(portEl) {
    const r = portEl.getBoundingClientRect(), b = nodesLayer.getBoundingClientRect();
    return { x: r.left - b.left + r.width / 2, y: r.top - b.top + r.height / 2 };
}
function renderLinks() {
    linksSvg.innerHTML = '';
    let maxX = 600, maxY = 300;
    state.links.forEach(link => {
        const aEl = nodesLayer.querySelector('.port-out[data-node="' + link.from.node + '"][data-port="' + link.from.port + '"]');
        const bEl = nodesLayer.querySelector('.port-in[data-node="' + link.to.node + '"][data-port="' + link.to.port + '"]');
        if (!aEl || !bEl) return;
        const a = portPosOf(aEl), b = portPosOf(bEl);
        const path = document.createElementNS('http://www.w3.org/2000/svg', 'path');
        path.setAttribute('d', linkPath(a, b));
        path.setAttribute('class', 'link ' + (isClockLink(link) ? 'clock' : 'signal') + (link.running ? ' running' : ''));
        path.dataset.id = link.id;
        path.addEventListener('dblclick', e => { e.stopPropagation(); removeLink(link.id); });
        linksSvg.appendChild(path);
        maxX = Math.max(maxX, a.x + 40, b.x + 40); maxY = Math.max(maxY, a.y + 40, b.y + 40);
    });
    if (state.linkDrag.from) {
        const src = state.linkDrag.from;
        const aEl = nodesLayer.querySelector('.port-out[data-node="' + src.node + '"][data-port="' + src.port + '"]');
        let a = { x: state.linkDrag.x, y: state.linkDrag.y };
        if (aEl) a = portPosOf(aEl);
        const tmp = document.createElementNS('http://www.w3.org/2000/svg', 'path');
        tmp.setAttribute('d', linkPath(a, { x: state.linkDrag.x, y: state.linkDrag.y }));
        tmp.setAttribute('class', 'link temp'); linksSvg.appendChild(tmp);
        maxX = Math.max(maxX, state.linkDrag.x + 40); maxY = Math.max(maxY, state.linkDrag.y + 40);
    }
    linksSvg.setAttribute('viewBox', '0 0 ' + maxX + ' ' + maxY);
    linksSvg.style.width = maxX + 'px'; linksSvg.style.height = maxY + 'px';
}
// 方向感知的贝塞尔连线：以主要位移方向决定控制点。
// 左右布局（水平位移为主）→ 横向贝塞尔；上下布局（垂直位移为主）→ 纵向贝塞尔，
// 避免上下排列的节点间因固定横向控制点而画出 S 形。
function linkPath(a, b) {
    const dx = b.x - a.x, dy = b.y - a.y;
    if (Math.abs(dy) > Math.abs(dx)) {
        const c = Math.max(24, Math.abs(dy) * 0.5), sy = Math.sign(dy) || 1;
        return `M ${a.x} ${a.y} C ${a.x} ${a.y + sy * c}, ${b.x} ${b.y - sy * c}, ${b.x} ${b.y}`;
    }
    const c = Math.max(24, Math.abs(dx) * 0.5), sx = Math.sign(dx) || 1;
    return `M ${a.x} ${a.y} C ${a.x + sx * c} ${a.y}, ${b.x - sx * c} ${b.y}, ${b.x} ${b.y}`;
}