// ==== 执行 ====
function markRunning(node, on) {
    node.running = on;
    outgoingLinks(node.id).forEach(l => l.running = on);
    renderNodes(); renderLinks();
}
async function runSingle(id) {
    const n = nodeById(id); if (!n) return;
    if (!state.connected) { toast('尚未连接引擎', 'error'); return; }
    if (isGate(n.type)) { toast('逻辑门需随主流程运行', 'error'); return; }
    n._s = 'pending'; n.outValue = null; n.outValues = null; n._pop = null;
    await runNode(n);
    replayDisplayPops(); // 显示类节点激活有消息 → 弹出气泡
}
// 拓扑排序已移除：执行改为事件驱动并行传播（沿时钟线激活），无限递归由调用次数上限兜底
async function runAll() {
    if (state.running) return;
    if (!state.connected) { toast('尚未连接引擎', 'error'); return; }
    if (!state.nodes.length) { toast('画布为空', 'error'); return; }
    state.running = true; state.abort = false; setRunBtn(true);
    state.active = 0; state.callCount = 0; state.maxCalls = 300; state.limitedNotified = false;
    state.doneWaiters = []; state.pending.clear();
    sanitizeGraph(); // 运行前剔除无效连线（undefined 端点会让激活链抛异常）
    state.nodes.forEach(n => { n._s = 'pending'; n.outValue = null; n.outValues = null; n.running = false; n._pop = null; n._queue = null; });
    state.links.forEach(l => l.running = false);
    renderGraph();
    // 所有启动节点同时开始；各分支沿时钟线并行独立传播，互不等待
    const TRIGGER = { start: 1, event_start: 1, clock_start: 1 };
    let seeds = state.nodes.filter(n => TRIGGER[n.type] && n.enabled !== false);
    if (!seeds.length) seeds = state.nodes.filter(n => clockLinksOf(n.id).length === 0 && !isGate(n.type) && n.enabled !== false); // 无启动节点时退化为源头节点
    seeds.forEach(n => activateNode(n));
    finishCheck(); // 全部同步结束（无种子/瞬时完成）时收尾
}
// ==== 事件驱动激活 ====
// 节点成功后沿时钟线激活所有下级（数据信号线只传参不触发）；门节点即时评估；同节点多次激活经 promise 队列串行
function activateSuccessors(node) {
    outgoingLinks(node.id).forEach(l => {
        if (!isClockLink(l)) return;
        const t = nodeById(l.to.node);
        if (t && t.enabled !== false) activateNode(t);
    });
}
function activateNode(node) {
    if (!state.running || state.abort || !node || node.enabled === false) return;
    if (isGate(node.type)) { evaluateGate(node); return; } // 门不执行任务，即时评估
    const v = viewOf(node.id); // 挂起任务按所属视图计数：主图任务记 state，子图任务记该复合节点的执行视图
    v.active++; state.active++; // 视图计数供子图完成判定，全局计数供整轮运行收尾
    node._queue = (node._queue || Promise.resolve()).then(() => runActivated(node, v)); // 同节点多次激活排队串行
}
function evaluateGate(node) {
    if (!state.running || state.abort) return;
    const oks = incomingLinks(node.id).map(l => { const s = nodeById(l.from.node); return !!s && s._s === 'ok'; });
    const pass = gateFunc(node.type, oks);
    node._s = pass ? 'ok' : 'skip';
    renderNodes();
    if (pass) activateSuccessors(node); // 放行才继续向下游传播激活
}
function acquireCall() {
    if (state.callCount >= state.maxCalls) {
        if (!state.limitedNotified) { state.limitedNotified = true; toast('已达最大调用次数 ' + state.maxCalls + '，停止激活（可用「调用上限」节点调整）', 'error'); }
        return false;
    }
    state.callCount++;
    return true;
}
async function runActivated(node, owner) {
    const v = owner || viewOf(node.id); // 激活时记录的所属视图（停止/复位可能已改计数，故用入口捕获值回减）
    try {
        if (!state.running || state.abort) return;
        if (!acquireCall()) { node._s = 'err'; renderNodes(); return; }
        node._s = 'pending'; // 多次激活时复位状态
        await runNode(node);
        if (node._s === 'ok') activateSuccessors(node); // 成功才点亮下游时钟链
    } catch (e) {
        node._s = 'err'; node.outValue = { error: '节点执行异常: ' + String(e) }; // 异常不炸传播链
    } finally {
        if (v.active > 0) v.active--;         // 停止运行已强制清零时不再回减，避免出现负计数
        if (state.active > 0) state.active--;
        renderNodes();
        finishCheck();
    }
}
function finishCheck() {
    // 子图完成判定：只看等待者自己那个视图的挂起任务数是否归零，与其它并行分支/复合节点的完成时刻完全无关
    // （旧实现比较全局 state.active 是否回到进入基线，会被并行分支的完成事件撞上数值而提前判定完成）
    for (let i = state.doneWaiters.length - 1; i >= 0; i--) {
        const w = state.doneWaiters[i];
        if (w.view.active === 0) { state.doneWaiters.splice(i, 1)[0].resolve(); return; } // 逆序=最内层子图优先
    }
    if (state.active > 0) return;
    if (state.doneWaiters.length) { state.doneWaiters.pop().resolve(); return; } // 兜底：无活动但仍有等待者
    if (!state.clocks.length) finishRun(); // 无周期时钟时结束运行周期（时钟启动保持运行直至手动停止）
}
function finishRun() {
    state.nodes.forEach(n => { n._s = 'idle'; n.outValue = null; n.outValues = null; n.running = false; });
    state.links.forEach(l => l.running = false);
    renderGraph();
    replayDisplayPops(); // 显示类节点激活有消息 → 运行收尾弹出气泡（渲染重建后补挂）
    setRunBtn(false); state.running = false;
}
// 子图事件驱动执行：激活视图内所有无时钟输入的源头节点，该视图挂起任务数归零时 resolve
// （视图自带计数，故子图完成只取决于自身内部任务，不受主图其它分支/其它复合节点影响）
function runSubgraph(view) {
    return new Promise(resolve => {
        state.doneWaiters.push({ view, resolve }); // 完成条件：view.active === 0
        const ids = new Set(view.nodes.map(n => n.id)); // 子图快照可能携带无效连线，按视图节点清洗
        view.links = view.links.filter(l => l && l.from && ids.has(l.from.node) && l.to && ids.has(l.to.node));
        view.nodes.filter(n => clockLinksOf(n.id).length === 0 && !isGate(n.type) && n.enabled !== false)
            .forEach(n => activateNode(n));
        finishCheck();
    });
}
function setRunBtn(on) {
    const b = $('runBtn');
    b.innerHTML = on ? '<i class="fas fa-stop"></i> 停止运行' : '<i class="fas fa-play"></i> 运行全部';
}
function stopRun() {
    state.abort = true;
    clearClocks();
    state.evWaiters.forEach(h => { clearTimeout(h._t); h.res({ __abort: true }); });
    state.evWaiters = [];
    while (state.doneWaiters.length) state.doneWaiters.pop().resolve(); // 唤醒挂起的子图等待
    state.active = 0;
    state.execViews.forEach(v => { v.active = 0; v.nodes.forEach(n => { n.running = false; n._s = 'idle'; n.outValue = null; n.outValues = null; n._pop = null; }); });
    state.execViews = [];
    state.nodes.forEach(n => { n.running = false; n._s = 'idle'; n.outValue = null; n.outValues = null; n._pop = null; });
    state.links.forEach(l => l.running = false);
    renderGraph(); setRunBtn(false); state.running = false;
}
async function runNode(node) {
    node.running = true; markRunning(node, true);
    const meta = metaOf(node.type);
    addLog({ dir: 'send', type: 'node:' + node.type, label: meta.label, req: '', env: { node: node.id }, isError: false, summary: '执行节点: ' + meta.label });
    try {
        if (node.type === 'start') { node.outValue = { started: true }; node._s = 'ok'; }
        else if (node.type === 'wait') { const ms = Number(node.params.ms) || 500; await sleep(ms); node.outValue = { slept_ms: ms }; node._s = 'ok'; }
        else if (node.type === 'transform') {
            const p = node.params || {};
            const template = String(p.template != null ? p.template : '').trim();
            if (!template) { node.outValue = { error: '输出模板不能为空' }; node._s = 'err'; }
            else {
                const values = {};
                ['in1', 'in2', 'in3', 'in4'].forEach((port, i) => {
                    const l = incomingLinks(node.id).find(x => x.to.port === port);
                    if (!l) return;
                    const src = nodeById(l.from.node);
                    if (!src || src.outValue === undefined) return;
                    const path = String(p['e' + (i + 1)] || '').trim();
                    values[port] = extractField(parseMaybeJson(src.outValue), path); // JSON 文本形态先解析
                });
                let out = template.replace(/\{\{([^}]+)\}\}/g, (m, key) => {
                    const parts = pathKeys(key); const root = parts.shift();
                    if (!(root in values)) return m;
                    let v = values[root];
                    for (const s of parts) { if (v == null) break; v = v[s]; }
                    if (v == null) return '';
                    return typeof v === 'string' ? v : JSON.stringify(v);
                });
                node.outValue = out; node._s = 'ok';
            }
        }
        else if (node.type === 'extract') { // 提取拆分：单输入按多条提取路径拆分为多端口输出（留空=原样透传）
            const l = incomingLinks(node.id).find(x => x.to.port === 'input');
            let v = node.params.input;
            if (l) { const src = nodeById(l.from.node); if (src) v = srcOut(src, l.from.port); }
            node.params.input = v; // 回写参数，参数浮窗可查看
            const pv = parseMaybeJson(v); // JSON 文本形态先解析为对象，再按路径提取
            node.outValues = {};
            ['e1', 'e2', 'e3', 'e4'].forEach((k, i) => {
                const path = String(node.params[k] != null ? node.params[k] : '').trim();
                node.outValues['out' + (i + 1)] = extractField(pv, path);
            });
            node.outValue = node.outValues.out1; // 输出1 兼容作为节点默认输出
            node._s = 'ok';
        }
        else if (node.type === 'display') { // 文本显示：取上游连线文本（无连线则用本节点填写内容），原样输出并回写参数供节点内预览
            const l = incomingLinks(node.id).find(x => x.to.port === 'text');
            let val = node.params.text;
            if (l) { const sv = srcOut(nodeById(l.from.node), l.from.port); if (sv !== undefined && sv !== null) val = sv; }
            if (val !== undefined) node.params.text = val; // 回写，节点内预览与下游保持一致
            node.outValue = val; node._s = 'ok';
            if (val != null && val !== '') node._pop = { kind: 'text', v: val }; // 激活有消息 → 运行收尾弹出气泡
        }
        else if (node.type === 'image_display') { // 图像显示：取上游图像（无连线用本节点填写），透传输出并回写参数
            const l = incomingLinks(node.id).find(x => x.to.port === 'image');
            let val = node.params.image;
            if (l) { const sv = srcOut(nodeById(l.from.node), l.from.port); if (sv !== undefined && sv !== null) val = sv; }
            const md = findMedia(val); // 递归识别 data URI / 裸 base64（magic bytes）
            if (md && md.image) {
                node.params.image = md.image; // 回写 data URI，供气泡展示与下游使用
                node.outValue = md.image; node._s = 'ok';
                node._pop = { kind: 'image', v: md.image }; // 激活有图像 → 运行收尾弹出展示
            } else if (typeof val === 'string' && val.trim()) {
                node.params.image = val; node.outValue = val; node._s = 'ok'; // 非 base64 图像的字符串原样透传
            } else {
                node.outValue = { error: '无图像输入（请连线或在参数中填入 base64）' }; node._s = 'err';
            }
        }
        else if (node.type === 'event_start') { // 等待目标事件触发后启动
            const topic = String(node.params.topic || '').trim() || null;
            const r = await awaitEvent(topic, Number(node.params.timeout) || 8000);
            if (r && r.__abort) { node._s = 'skip'; }
            else if (r && r.__timeout) { node.outValue = { error: '等待事件超时' }; node._s = 'err'; }
            else { node.outValue = (r && r.payload !== undefined) ? r.payload : (r || {}); node._s = 'ok'; }
        }
        else if (node.type === 'clock_start') { // 每隔 interval 沿时钟线激活一次下游节点（并行传播）
            const ms = Math.max(50, Number(node.params.interval) || 2000);
            const fire = () => { if (state.abort) { clearClocks(); return; } activateSuccessors(node); };
            state.clocks.push(setInterval(fire, ms));
            node.outValue = { tick: true }; node._s = 'ok';
        }
        else if (node.type === 'limit') { // 调用上限：动态调整本次运行的最大节点调用次数（全局生效）
            state.maxCalls = Math.max(1, Number(node.params.max) || 300);
            node.outValue = { maxCalls: state.maxCalls }; node._s = 'ok';
        }
        else if (node.type === 'speaker') { // 扬声器：取上游音频（或本节点填写的 base64）通过扬声器播放
            const l = incomingLinks(node.id).find(x => x.to.port === 'audio');
            let uri = null;
            if (l) { const sv = srcOut(nodeById(l.from.node), l.from.port); if (sv) uri = audioDataUriOf(sv); }
            if (!uri && typeof node.params.audio === 'string' && node.params.audio.trim()) uri = b64ToDataUri(node.params.audio);
            if (!uri) { node.outValue = { error: '无音频输入（请连线或在参数中填入 base64 音频）' }; node._s = 'err'; }
            else {
                try { await new Audio(uri).play(); node.outValue = { played: true }; node._s = 'ok'; }
                catch (e) { node.outValue = { error: '播放失败: ' + String(e) }; node._s = 'err'; }
            }
        }
        else if (node.type === 'microphone') { // 麦克风：请求授权并录制指定时长，输出 base64 音频（webm/opus）
            const sec = Math.max(0.3, Number(node.params.duration) || 3);
            addLog({ dir: 'send', type: 'node:microphone', label: '麦克风录音', req: '', env: {}, isError: false, summary: `请求麦克风授权，录音 ${sec} 秒…` });
            const b64 = await recordAudio(sec);
            if (!b64) { node.outValue = { error: '录音失败或麦克风未授权' }; node._s = 'err'; }
            else {
                node.params.audio = b64; // 回写，供连线/查看
                node.outValue = b64; // 裸 base64，连线到 Qwen 识别 / 扬声器皆可直接使用
                node._s = 'ok';
            }
        }
        else if (node.type === 'audio_eq') { await runAudioEqualizer(node); } // 音频均衡器：低频/中频/高频 增益衰减，输出 wav
        else if (node.type === 'image_confuse') { await runImageConfusion(node); } // 图像混淆：混淆 / 解混淆 / 还原
        else if (node.type === 'video_keyframe') { await runVideoKeyframe(node); } // 视频抽帧：URL→/keyframe 关键帧→GIF base64
        else if (node.type === 'composite') { // 复合节点：换入内部子图复用同一执行器，完成后换出
            await runCompositeNode(node);
        }
        else {
            // 注入上游数据：对每条数据输入连线，按连线源端口取上游输出写入本节点参数（多输出节点各路独立）
            incomingLinks(node.id).forEach(l => {
                if (l.to.port === 'gate') return;
                const sv = srcOut(nodeById(l.from.node), l.from.port);
                if (sv !== undefined && sv !== null) node.params[l.to.port] = sv;
            });
            if (isGate(node.type)) { node._s = gateFunc(node.type, incomingLinks(node.id).map(l => nodeById(l.from.node)._s === 'ok')) ? 'ok' : 'skip'; }
            else {
                const env = buildEnvelope(node);
                if (!env) { node.outValue = { error: '参数未完整配置' }; node._s = 'err'; }
                else {
                    if (env.type === 'ltp9/event') emitEventLocal(env.topic, env.payload); // 事件触发时点亮事件启动链
                    const ack = await sendWait(env, meta.label);
                    node.outValue = coreOut(ack, node); // 提取核心数据（剥信封/单值直传），下游直接可用
                    // 失败判定：引擎层错误（result.error）、调用层 ok:false（result.value.ok）、或未收到回执
                    const rv = ack && ack.result;
                    const err = ack && (ack.ok === false || (rv && rv.error) || (rv && rv.value && rv.value.ok === false));
                    node._s = (err || !ack) ? 'err' : 'ok';
                }
            }
        }
    } catch (e) { node._s = 'err'; node.outValue = { error: String(e) }; }
    markRunning(node, false);
}

// ==== 复合节点执行 ====
// 子图以独立执行视图（execViews 压栈）运行，不替换全局 state——复合节点可与主图其他分支真正并行。
// 出口节点的输出即复合节点输出；子图节点不在主图画布渲染，仅状态经复合节点体现。
async function runCompositeNode(node) {
    const sg = node.params.subgraph || {};
    const inMap = node.params.inMap || [], outMap = node.params.outMap || null;
    if (!sg.nodes || !sg.nodes.length) { node.outValue = { error: '复合节点为空' }; node._s = 'err'; return; }
    const extIn = {}; // 外部数据输入（主图视图采集；多输出上游按连线源端口取值）
    incomingLinks(node.id).forEach(l => {
        if (l.to.port === 'gate') return;
        const sv = srcOut(nodeById(l.from.node), l.from.port);
        if (sv !== undefined && sv !== null) extIn[l.to.port] = sv;
    });
    const view = {
        nodes: sg.nodes.map(n => Object.assign(JSON.parse(JSON.stringify(n)), { running: false, _s: 'pending', outValue: null, outValues: null, _pop: null })),
        links: (sg.links || []).map(l => ({ id: l.id, from: { node: l.from.node, port: l.from.port }, to: { node: l.to.node, port: l.to.port } })),
        active: 0 // 子图自己的挂起任务计数（完成判定只看它，不看全局 state.active）
    };
    state.execViews.push(view);
    try {
        // 外部输入注入到映射的内部节点参数（数据信号线语义，不参与内部时钟链判定）
        inMap.forEach(m => { const t = nodeById(m.node); if (t && extIn[m.k] !== undefined) t.params[m.port] = extIn[m.k]; });
        // 内部子图同样走事件驱动并行执行（源头节点同时启动，沿内部时钟线传播；受全局调用上限约束）
        await runSubgraph(view);
        // 内部显示类节点的弹出消息转挂到复合节点（统一回放）
        const innerPops = view.nodes.map(n => n._pop).filter(Boolean);
        if (innerPops.length) node._pop = { kind: 'multi', pops: innerPops };
        // 出口取值：声明的出口节点输出即复合输出；无出口时内部无错误即成功
        if (outMap) {
            const exit = nodeById(outMap.node);
            node._s = exit ? exit._s : 'ok';
            node.outValue = exit ? exit.outValue : null;
        } else {
            node._s = view.nodes.some(n => n._s === 'err') ? 'err' : 'ok';
            node.outValue = null;
        }
    } finally {
        state.execViews.pop();
        renderGraph();
    }
}