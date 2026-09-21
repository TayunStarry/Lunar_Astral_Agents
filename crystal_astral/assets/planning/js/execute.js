// ==== 执行 ====
// 扁平执行池架构：所有节点严格同一层级，连线仅定义先后顺序，无任何拓扑层级。
// 节点在池中并行独立执行：任一节点完成即出池并使其后续节点立即入池，失败仅自身出池，
// 单个节点的完成/失败/等待均不影响池中其他节点。
function markRunning(node, on) {
    node.running = on;
    outgoingLinks(node.id).forEach(l => l.running = on);
    renderNodes(); renderLinks();
}
async function runSingle(id) {
    const n = nodeById(id); if (!n) return;
    if (!state.connected) { toast('尚未连接引擎', 'error'); return; }
    n._s = 'pending'; n.outValue = null; n.outValues = null; n._pop = null;
    state.callCount = 0; state.maxCalls = 300; state.limitedNotified = false;
    state.singleRun = true; // 单节点执行：仅运行该节点本身，不向主图后续传播
    try { await runNode(n); } finally { state.singleRun = false; }
    replayDisplayPops(); // 显示类节点激活有消息 → 弹出气泡
}
// ==== 启动运行 ====
// 触发型启动节点（手动/事件/时间）触发时，与其直接相连的所有节点立即加入执行池并行执行
async function runAll() {
    if (state.running) return;
    if (!state.connected) { toast('尚未连接引擎', 'error'); return; }
    if (!state.nodes.length) { toast('画布为空', 'error'); return; }
    state.running = true; state.abort = false; setRunBtn(true);
    state.active = 0; state.callCount = 0; state.maxCalls = 300; state.limitedNotified = false;
    state.pool.clear(); state.compRuns = []; // 清空执行池与残留的复合运行时实例
    sanitizeGraph(); // 运行前剔除无效连线（undefined 端点会让激活链抛异常）
    state.nodes.forEach(n => { n._s = 'pending'; n.outValue = null; n.outValues = null; n.running = false; n._pop = null; n._queue = null; });
    state.links.forEach(l => l.running = false);
    renderGraph();
    // 所有启动节点同时触发；各自把直接相连节点加入执行池，各分支并行独立，互不等待
    const TRIGGER = { start: 1, event_start: 1, clock_start: 1 };
    let seeds = state.nodes.filter(n => TRIGGER[n.type] && n.enabled !== false);
    if (!seeds.length) seeds = state.nodes.filter(n => incomingLinks(n.id).length === 0 && n.enabled !== false); // 无启动节点时以无入线节点为入口
    seeds.forEach(n => triggerSeed(n));
    finishCheck(); // 无触发/瞬时结束时收尾
}
// 触发启动节点：不占执行池（等待事件/周期投递不算节点执行），命中后与手动触发同流程——把直接相连节点加入执行池
// 触发器先同步占位 active 计数：防止 runAll 末尾的 finishCheck 在微任务尚未启动时误判「池空无源」提前收尾
function triggerSeed(node) {
    state.active++;
    node._queue = (node._queue || Promise.resolve()).then(async () => {
        try { await runNode(node); } catch (e) { node._s = 'err'; node.outValue = { error: String(e) }; }
        if (node._s === 'ok') dispatchSuccessors(node); // 先把后续节点入池再释放占位，保证计数不归零误判
        if (state.active > 0) state.active--;
        renderNodes(); finishCheck();
    });
}
// ==== 执行池 ====
// 连线即先后顺序：节点成功出池后，其所有连线指向的后续节点立即加入执行池
// （池内多节点时，任一节点完成即刻处理其后续，绝不等待其他节点）
function dispatchSuccessors(node) {
    const seen = new Set();
    outgoingLinks(node.id).forEach(l => {
        if (seen.has(l.to.node)) return; // 同一后续节点只入池一次（多条连线仅传递不同端口的数据）
        seen.add(l.to.node);
        const t = nodeById(l.to.node);
        if (t && t.enabled !== false) enterPool(t);
    });
}
// 节点加入执行池：同节点多次激活经 promise 队列串行，池内不同节点完全并行、状态互不干扰
function enterPool(node) {
    if ((!state.running && !state.singleRun) || state.abort || !node || node.enabled === false) return;
    if (!acquireCall()) return; // 单次运行最大节点执行数量耗尽 → 不再入池
    state.pool.set(node.id, (state.pool.get(node.id) || 0) + 1);
    state.active++;
    if (node._comp) node._comp.active++; // 复合节点实例的派生激活计数（供复合节点完成判定）
    node._queue = (node._queue || Promise.resolve()).then(() => runPooled(node));
}
function acquireCall() {
    if (state.callCount >= state.maxCalls) {
        if (!state.limitedNotified) {
            state.limitedNotified = true;
            toast('已达最大节点执行数量 ' + state.maxCalls + '，执行终止（可用「调用上限」节点调整）', 'error');
            finishCheck();
        }
        return false;
    }
    state.callCount++;
    return true;
}
async function runPooled(node) {
    try {
        if ((!state.running && !state.singleRun) || state.abort) return;
        node._s = 'pending'; // 多次激活时复位状态
        await runNode(node);
        if (node._s === 'ok') dispatchSuccessors(node); // 成功 → 后续立即入池；失败 → 仅自身出池，不激活后续，不影响其他节点
    } catch (e) {
        node._s = 'err'; node.outValue = { error: '节点执行异常: ' + String(e) }; // 单节点异常不阻断执行池
    } finally {
        const c = (state.pool.get(node.id) || 1) - 1;
        if (c <= 0) state.pool.delete(node.id); else state.pool.set(node.id, c);
        if (state.active > 0) state.active--;
        if (node._comp) { // 复合实例派生激活出池：归零时唤醒等待中的复合节点
            const inst = node._comp;
            if (inst.active > 0) inst.active--;
            if (inst.active === 0 && inst.done) { const d = inst.done; inst.done = null; d(); }
        }
        renderNodes();
        finishCheck();
    }
}
// 终止条件（三选一即终）：a) 执行池清空且无时钟/事件触发源可再加入新节点；b) 最大节点执行数量耗尽；c) 用户手动停止
function finishCheck() {
    if (!state.running || state.abort) return;
    if (state.active > 0) return;
    if (state.callCount >= state.maxCalls) { clearClocks(); resolveEvWaiters(); finishRun(); return; } // b) 上限耗尽
    if (state.clocks.length) return;    // 时钟启动持续投递新节点，运行保持直至手动停止
    if (state.evWaiters.length) return; // 事件启动仍在监听，运行保持
    finishRun(); // a) 池空且无新节点来源
}
function finishRun() {
    if (!state.running) return;
    state.nodes.forEach(n => { n._s = 'idle'; n.outValue = null; n.outValues = null; n.running = false; });
    state.links.forEach(l => l.running = false);
    renderGraph();
    replayDisplayPops(); // 显示类节点激活有消息 → 运行收尾弹出气泡（渲染重建后补挂）
    setRunBtn(false); state.running = false;
}
function resolveEvWaiters() {
    state.evWaiters.forEach(h => { clearTimeout(h._t); h.res({ __abort: true }); });
    state.evWaiters = [];
}
function setRunBtn(on) {
    const b = $('runBtn');
    b.innerHTML = on ? '<i class="fas fa-stop"></i> 停止运行' : '<i class="fas fa-play"></i> 运行全部';
}
// 用户手动结束运行状态：清空执行池与所有触发源
function stopRun() {
    state.abort = true;
    clearClocks();
    resolveEvWaiters();
    state.compRuns.forEach(i => { if (i.done) { const d = i.done; i.done = null; d(); } }); // 唤醒等待派生排空的复合节点
    state.compRuns = [];
    state.pool.clear();
    state.active = 0;
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
        else if (node.type === 'wait') { const ms = Number(node.params.ms) || 500; await sleep(ms); node.outValue = { slept_ms: ms }; node._s = 'ok'; } // 仅延时自身后续节点，不阻塞执行池其他节点
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
        else if (node.type === 'event_start') { // 事件触发型启动节点：监测到目标事件 → 与手动触发同流程（激活后续节点）
            const topic = String(node.params.topic || '').trim() || null;
            const r = await awaitEvent(topic, Number(node.params.timeout) || 8000);
            if (r && r.__abort) { node._s = 'skip'; }
            else if (r && r.__timeout) { node.outValue = { error: '等待事件超时' }; node._s = 'err'; }
            else { node.outValue = (r && r.payload !== undefined) ? r.payload : (r || {}); node._s = 'ok'; }
        }
        else if (node.type === 'clock_start') { // 时间触发型启动节点：到达周期 → 与手动触发同流程（激活后续节点），直至手动停止
            const ms = Math.max(50, Number(node.params.interval) || 2000);
            const fire = () => { if (state.abort) { clearClocks(); return; } dispatchSuccessors(node); };
            state.clocks.push(setInterval(fire, ms));
            node.outValue = { tick: true }; node._s = 'ok';
        }
        else if (node.type === 'limit') { // 调用上限：动态调整本次运行的最大节点执行数量（全局生效）
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
        else if (node.type === 'composite') { // 复合节点：子图节点扁平并入全局执行池，与主图节点同层并行
            await runCompositeNode(node);
        }
        else {
            // 注入上游数据：对每条输入连线，按连线源端口取上游输出写入本节点参数（多输出节点各路独立）
            incomingLinks(node.id).forEach(l => {
                if (l.to.port === 'gate' || l.from.port === 'gate') return; // 顺序端口连线仅传放行，不注入数据
                const sv = srcOut(nodeById(l.from.node), l.from.port);
                if (sv !== undefined && sv !== null) node.params[l.to.port] = sv;
            });
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
    } catch (e) { node._s = 'err'; node.outValue = { error: String(e) }; }
    markRunning(node, false);
}

// ==== 复合节点扁平执行 ====
// 复合节点仅是编辑层的封装：执行时子图节点经 ID 重映射后直接加入全局执行池，
// 与主图节点严格同层并行（无视图栈、无嵌套执行），可多实例/循环激活且互不串扰。
// 复合节点自身留在池中等待其派生激活全部出池，再以出口节点输出作为自身输出并激活后续节点。
function runCompositeNode(node) {
    const sg = node.params.subgraph || {};
    const inMap = node.params.inMap || [], outMap = node.params.outMap || null;
    if (!sg.nodes || !sg.nodes.length) { node.outValue = { error: '复合节点为空' }; node._s = 'err'; return Promise.resolve(); }
    const extIn = {}; // 外部输入（按连线源端口取上游输出）
    incomingLinks(node.id).forEach(l => {
        if (l.to.port === 'gate' || l.from.port === 'gate') return; // 顺序端口连线仅传放行，不作为数据输入
        const sv = srcOut(nodeById(l.from.node), l.from.port);
        if (sv !== undefined && sv !== null) extIn[l.to.port] = sv;
    });
    const inst = { index: new Map(), links: [], active: 0, done: null }; // 扁平运行时实例（非层级视图，仅登记派生节点）
    const idMap = {};
    sg.nodes.forEach(n => {
        const clone = Object.assign(JSON.parse(JSON.stringify(n)), { running: false, _s: 'pending', outValue: null, outValues: null, _pop: null, _queue: null, _comp: inst });
        clone.id = n.id + '~c' + (++state.compSeq); // ID 重映射，避免与主图/其他实例冲突
        idMap[n.id] = clone.id;
        inst.index.set(clone.id, clone);
    });
    // 子图快照连线清洗：端点端口不在节点当前端口集合中的历史连线（如冗余顺序线）直接剔除
    (sg.links || []).forEach(l => {
        const f = inst.index.get(idMap[l.from.node]), t = inst.index.get(idMap[l.to.node]);
        if (!f || !t) return;
        if (!portExists(f.type, f, 'out', l.from.port) || !portExists(t.type, t, 'in', l.to.port)) return;
        inst.links.push({ id: 'l' + seg(), from: { node: idMap[l.from.node], port: l.from.port }, to: { node: idMap[l.to.node], port: l.to.port } });
    });
    state.compRuns.push(inst);
    // 外部输入注入到映射的内部节点参数
    inMap.forEach(m => { const t = inst.index.get(idMap[m.node]); if (t && extIn[m.k] !== undefined) t.params[m.port] = extIn[m.k]; });
    const drained = new Promise(res => { inst.done = res; }); // 完成条件：本实例派生激活全部出池
    // 入口：无内部入线的节点立即加入执行池（与主图节点同层并行，受全局执行数量上限约束）
    sg.nodes.filter(n => !inst.links.some(l => l.to.node === idMap[n.id]))
        .forEach(n => enterPool(inst.index.get(idMap[n.id])));
    if (inst.active === 0) { const d = inst.done; inst.done = null; if (d) d(); } // 无可入池节点（如内部成环）时直接排空
    return drained.then(() => {
        state.compRuns = state.compRuns.filter(x => x !== inst);
        // 内部显示类节点的弹出消息转挂到复合节点（统一回放）
        const innerPops = [...inst.index.values()].map(n => n._pop).filter(Boolean);
        if (innerPops.length) node._pop = { kind: 'multi', pops: innerPops };
        // 出口取值：声明的出口节点输出即复合输出；无出口时内部无错误即成功
        if (outMap) {
            const exit = inst.index.get(idMap[outMap.node]);
            node._s = exit ? exit._s : 'ok';
            node.outValue = exit ? exit.outValue : null;
        } else {
            node._s = [...inst.index.values()].some(n => n._s === 'err') ? 'err' : 'ok';
            node.outValue = null;
        }
    });
}
