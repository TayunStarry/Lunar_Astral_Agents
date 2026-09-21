// ==== 判定 / 取值 / 事件总线 工具 ====
// 前端事件总线：事件启动节点 await；事件触发/时钟启动投递
function awaitEvent(topic, timeout) {
    return new Promise(res => {
        const h = { topic: topic || null, res, _t: null };
        state.evWaiters.push(h);
        h._t = setTimeout(() => { state.evWaiters = state.evWaiters.filter(x => x !== h); res({ __timeout: true }); }, timeout || 8000);
    });
}
function emitEventLocal(topic, payload) {
    const waiters = state.evWaiters; state.evWaiters = [];
    waiters.forEach(h => {
        if (h.topic && topic && h.topic !== topic) { state.evWaiters.push(h); return; }
        clearTimeout(h._t); h.res({ topic: topic || (h.topic || ''), payload });
    });
}
function clearClocks() { state.clocks.forEach(t => clearInterval(t)); state.clocks = []; }
// 解析取值路径：支持 a.b[0].c 或 a.b.0.c 混合写法 → ['a','b','0','c']
function pathKeys(path) {
    return String(path).replace(/\[(\d+)\]/g, '.$1').split('.').filter(Boolean);
}
// 按路径从对象取值；path 为空返回对象本身；数组下标用 [n] 或 .n 均可
function extractField(obj, path) {
    if (obj == null) return undefined;
    if (!path) return obj;
    let v = obj;
    for (const k of pathKeys(path)) { if (v == null) return undefined; v = v[k]; }
    return v;
}
// 字符串形如 JSON（{ 或 [ 开头）时尝试解析为对象，失败/非字符串原样返回；
// 引擎回执经 coreOut 提取后 text 常为 JSON 文本形态，按路径取字段前需先解析
function parseMaybeJson(v) {
    if (typeof v !== 'string') return v;
    const t = v.trim();
    if (!t || (t[0] !== '{' && t[0] !== '[')) return v;
    try { return JSON.parse(t); } catch (e) { return v; }
}
// 节点端口集合：有数据输入/输出时仅提供数据端口；仅在某侧缺失端口时自动补全「顺序」端口
// （顺序端口是数据空白节点的通用连接点，保证任何节点都能被接入/引出执行链）
function inPortsOf(type, node) {
    if (type === 'composite' && node) {
        const map = (node.params && node.params.inMap) || [];
        if (map.length) return map.map(m => ({ k: m.k, label: m.label || m.k }));
        return [{ k: 'gate', label: '顺序' }];
    }
    const d = metaOf(type);
    if ((d.ins || []).length) return d.ins;
    return [{ k: 'gate', label: '顺序' }];
}
function outPortsOf(type, node) {
    if (type === 'composite' && node) {
        const has = !!(node.params && node.params.outMap);
        return has ? [{ k: 'out', label: '输出' }] : [{ k: 'gate', label: '顺序' }];
    }
    const d = metaOf(type); const outs = d.outs || (d.out ? [d.out] : []); // outs 支持多输出端口（提取拆分）
    if (outs.length) return outs;
    return [{ k: 'gate', label: '顺序' }];
}
// 端口有效性判定：连线端点必须落在节点当前实际提供的端口上（历史画布的冗余顺序线在清洗时自动剔除）
function portExists(type, node, side, port) {
    return (side === 'in' ? inPortsOf(type, node) : outPortsOf(type, node)).some(p => p.k === port);
}
// 取上游连线输出值：多输出节点（outValues 含该源端口）按连线源端口取，其余节点统一取 outValue。
// 注意必须传连线的 from.port（上游输出端口名），而非下游自己的输入端口名
function srcOut(src, fromPort) {
    if (!src) return undefined;
    if (src.outValues && Object.prototype.hasOwnProperty.call(src.outValues, fromPort)) return src.outValues[fromPort];
    return src.outValue;
}