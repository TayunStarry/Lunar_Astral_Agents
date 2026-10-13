// ==== 判定 / 取值 工具 ====
// （事件总线 awaitEvent/emitEventLocal、时钟 clearClocks、路径提取 pathKeys/extractField/
//   parseMaybeJson、上游取值 srcOut 已随调度循环迁移至引擎 blueprint.go，前端不再需要）
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