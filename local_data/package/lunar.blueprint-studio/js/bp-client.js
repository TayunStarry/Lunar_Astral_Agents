// ==== 蓝图后端执行客户端 ====
// 调度循环已迁移至引擎（engine/plugin blueprint.go）：前端提交图数据（ltp9/bp_run），
// 后端逐节点执行并推送状态（ltp9/bp_node），浏览器专属节点经 ltp9/bp_exec 协作执行，
// 前端回报 ltp9/bp_exec_result，运行收尾推送 ltp9/bp_done。
// 前端职责：图显示 / 状态渲染 / 参数编辑 / 持久化 / 浏览器节点执行。

// ==== 运行控制 ====
function bpGraphPayload() {
    return {
        nodes: state.nodes.map(n => ({ id: n.id, type: n.type, params: n.params, enabled: n.enabled })),
        links: state.links.filter(l => l && l.from && l.to).map(l => ({ id: l.id, from: l.from, to: l.to }))
    };
}
// 本地复位：全部节点回到待运行态（后端开始执行前的前端镜像）
function bpResetAll() {
    state.nodes.forEach(n => { n._s = 'pending'; n.outValue = null; n.outValues = null; n.running = false; n._pop = null; });
    state.links.forEach(l => l.running = false);
    renderGraph();
}
// 运行全部：提交整图给后端调度（单运行实例：运行中后端直接拒绝）
async function runAll() {
    if (state.running) return;
    if (!state.connected) { toast('尚未连接引擎', 'error'); return; }
    if (!state.nodes.length) { toast('画布为空', 'error'); return; }
    sanitizeGraph(); // 运行前剔除无效连线（undefined 端点会让激活链异常）
    bpResetAll();
    const ack = await sendWait({ type: 'ltp9/bp_run', mode: 'all', graph: bpGraphPayload() }, '运行全部');
    if (!ack || ack.ok === false) { toast((ack && ack.error) || '运行请求失败', 'error'); return; }
    state.bpRunId = ack.run_id; state.running = true; setRunBtn(true);
}
// 仅运行单节点：后端只执行该节点本身，不向主图后续传播
async function runSingle(id) {
    if (!state.connected) { toast('尚未连接引擎', 'error'); return; }
    if (!nodeById(id)) return;
    const ack = await sendWait({ type: 'ltp9/bp_run', mode: 'single', node_id: id, graph: bpGraphPayload() }, '运行节点');
    if (!ack || ack.ok === false) { toast((ack && ack.error) || '运行请求失败', 'error'); return; }
    state.bpRunId = ack.run_id; state.running = true; setRunBtn(true);
}
// 停止运行：后端立即收尾（正在执行的阻塞节点不强杀，仅停止派发后续）
function stopBpRun() { send({ type: 'ltp9/bp_stop', run_id: state.bpRunId }, '停止运行'); }
function setRunBtn(on) {
    const b = $('runBtn');
    b.innerHTML = on ? '<i class="fas fa-stop"></i> 停止运行' : '<i class="fas fa-play"></i> 运行全部';
}

// ==== 后端推送处理 ====
// websocket.js handleMessage 委托入口（bp_ack 已由 ACK_TYPES + pending 机制先行消费）
function handleBpMessage(m) {
    if (m.type === 'ltp9/bp_ack') return;
    // 集线器为无差别广播：仅处理本标签页发起的运行（空闲时 bpRunId 为 null，忽略一切推送）
    if (!state.bpRunId || m.run_id !== state.bpRunId) return;
    switch (m.type) {
        case 'ltp9/bp_node': applyBpNodeStatus(m); break;
        case 'ltp9/bp_exec': execBrowserNode(m); break;
        case 'ltp9/bp_done': bpFinishRun(m); break;
    }
}
// 逐节点状态同步：running 点亮连线，完成时更新状态/输出/参数回写/气泡
function applyBpNodeStatus(m) {
    const n = nodeById(m.node_id);
    const statusTextOf = s => ({ running: '运行中…', ok: '成功', err: '错误', skip: '跳过' }[s] || s);
    if (!n) {
        // 复合子图重映射节点（~cN 后缀）前端无对应卡片，仅记日志
        addLog({ dir: 'recv', type: 'ltp9/bp_node', label: '', req: '', env: m, isError: m.status === 'err', summary: '子图节点 ' + m.node_id + ' ' + statusTextOf(m.status) + (m.error ? '：' + m.error : '') });
        return;
    }
    if (m.status === 'running') {
        n.running = true;
        outgoingLinks(n.id).forEach(l => l.running = true);
        addLog({ dir: 'send', type: 'node:' + n.type, label: metaOf(n.type).label, req: '', env: { node: n.id }, isError: false, summary: '执行节点: ' + metaOf(n.type).label });
        renderNodes(); renderLinks();
        return;
    }
    n.running = false;
    outgoingLinks(n.id).forEach(l => l.running = false);
    n._s = m.status;
    if ('out_value' in m) n.outValue = m.out_value;
    if (m.out_values) n.outValues = m.out_values;
    if (m.params_patch) Object.assign(n.params, m.params_patch);
    if (m.pop) n._pop = m.pop; // 显示类气泡暂存，运行收尾统一回放
    const meta = metaOf(n.type);
    addLog({
        dir: 'recv', type: 'ltp9/bp_node', label: meta.label, req: '', env: m, isError: m.status === 'err',
        summary: meta.label + ' ' + statusTextOf(m.status) + (m.error ? '：' + m.error : '') +
            (m.status === 'ok' && m.out_value !== undefined && m.out_value !== null ? ' → ' + bpOutSummary(m.out_value) : '')
    });
    renderNodes(); renderLinks();
}
// 运行收尾：复位全部节点状态并回放显示类气泡
function bpFinishRun(m) {
    state.bpRunId = null;
    state.nodes.forEach(n => { n._s = 'idle'; n.outValue = null; n.outValues = null; n.running = false; });
    state.links.forEach(l => l.running = false);
    renderGraph();
    replayDisplayPops(); // 显示类节点激活有消息 → 运行收尾弹出气泡（渲染重建后补挂）
    setRunBtn(false); state.running = false;
    const reason = m && m.reason;
    if (reason === 'maxcalls') toast('已达最大节点执行数量，执行终止（可用「调用上限」节点调整）', 'error');
    else if (reason === 'stopped') toast('运行已停止', 'success');
    else if (reason === 'error') toast('运行异常终止', 'error');
}
// 输出摘要（日志用）：字符串截断，对象 JSON 截断
function bpOutSummary(v) {
    try {
        const s = typeof v === 'string' ? v : JSON.stringify(v);
        return s === undefined ? String(v) : (s.length > 120 ? s.slice(0, 120) + '…' : s);
    } catch (e) { return String(v); }
}

// ==== 浏览器协作节点执行 ====
// 后端调度遇到浏览器专属节点时推送 ltp9/bp_exec（params 为已注入上游数据的最终参数），
// 前端执行后回报 ltp9/bp_exec_result（value/params_patch/pop 随 bp_node 同步回图）。
async function execBrowserNode(m) {
    const p = m.params || {};
    let res;
    try {
        switch (m.node_type) {
            case 'microphone': { // 麦克风：请求授权并录制指定时长，输出 base64 音频（webm/opus）
                const sec = Math.max(0.3, Number(p.duration) || 3);
                addLog({ dir: 'send', type: 'node:microphone', label: '麦克风录音', req: '', env: {}, isError: false, summary: `请求麦克风授权，录音 ${sec} 秒…` });
                const b64 = await recordAudio(sec);
                res = b64 ? { ok: true, value: b64, params_patch: { audio: b64 } } : { ok: false, error: '录音失败或麦克风未授权' };
                break;
            }
            case 'speaker': { // 扬声器：播放上游音频（或本节点填写的 base64）
                let uri = audioDataUriOf(p.audio);
                if (!uri && typeof p.audio === 'string' && p.audio.trim()) uri = b64ToDataUri(p.audio.trim());
                if (!uri) res = { ok: false, error: '无音频输入（请连线或在参数中填入 base64 音频）' };
                else {
                    try { await new Audio(uri).play(); res = { ok: true, value: { played: true } }; }
                    catch (e) { res = { ok: false, error: '播放失败: ' + String(e) }; }
                }
                break;
            }
            case 'audio_eq': res = await computeAudioEqualizer(p); break;
            case 'image_confuse': res = await computeImageConfusion(p); break;
            case 'video_keyframe': res = await computeVideoKeyframe(p); break;
            default: res = { ok: false, error: '未知浏览器节点类型: ' + m.node_type };
        }
    } catch (e) { res = { ok: false, error: String(e) }; }
    if (res.summary) {
        addLog({ dir: 'send', type: 'node:' + m.node_type, label: metaOf(m.node_type).label, req: '', env: { node: m.node_id }, isError: !res.ok, summary: res.summary });
    }
    send({
        type: 'ltp9/bp_exec_result', run_id: m.run_id, exec_id: m.exec_id,
        ok: !!res.ok, value: res.value, error: res.error || '', params_patch: res.params_patch, pop: res.pop
    }, '浏览器节点回报');
}
