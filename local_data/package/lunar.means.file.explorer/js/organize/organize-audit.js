/**
 * 智能整理：文件夹分布审核模块
 * 汇总整理后拟形成的文件夹分布，调用 AI 审核并应用修正建议
 */

/**
 * 汇总整理后拟形成的文件夹分布（各文件夹将放入的文件数量与类型）
 * @param {Array<Object>} operations - 操作列表
 * @param {Array<string>} knownFolders - 已知文件夹列表
 * @returns {string} 分布描述文本
 */
function summarizeFolderDistribution(operations, knownFolders) {
    const stats = {};
    for (const folder of knownFolders) {
        stats[folder] = { count: 0, types: new Set() };
    }
    let rootCount = 0;
    const rootTypes = new Set();

    for (const op of operations) {
        if (!op || op.type !== 'move' || !op.target) continue;
        const target = String(op.target).replace(/\\/g, '/');
        const idx = target.lastIndexOf('/');
        const folder = idx >= 0 ? target.slice(0, idx) : '';
        const fileExt = idx >= 0 ? target.slice(idx + 1) : target;
        const ext = fileExt.slice(fileExt.lastIndexOf('.') + 1).toLowerCase() || '无';
        if (folder && stats[folder]) {
            stats[folder].count++;
            stats[folder].types.add(ext);
        } else {
            rootCount++;
            rootTypes.add(ext);
        }
    }

    const lines = ['本次整理后拟形成的文件夹分布：'];
    for (const folder of knownFolders) {
        if (stats[folder].count > 0) {
            lines.push(`- ${folder}：${stats[folder].count} 个文件（${Array.from(stats[folder].types).join('、')}）`);
        }
    }
    if (rootCount > 0) {
        lines.push(`- （当前目录，保持不动）：${rootCount} 个文件（${Array.from(rootTypes).join('、')}）`);
    }
    if (lines.length === 1) {
        lines.push('- （无文件被移动到文件夹）');
    }
    return lines.join('\n');
}

/**
 * 调用 AI 审核文件夹分布，返回需修正的合并/调整建议
 * @param {string} distributionText - 分布描述文本
 * @returns {Promise<Array<{from: string, to: string, reason: string}>>}
 */
async function auditFolderDistribution(distributionText) {
    const system = [
        '你是文件整理审核助手。以下是本次整理后拟形成的文件夹分布，请检查是否存在分布不合理：',
        '1. 语义重复的文件夹（同义、近义或可合并）→ 应合并；',
        '2. 功能不合适的文件夹（命名混乱、粒度不当）→ 应调整；',
        '3. 层级不合理的文件夹 → 应调整。',
        '只返回 JSON 数组，不要输出其他内容，格式：[{"from":"被合并/被调整的文件夹名","to":"保留的目标文件夹名","reason":"原因"}]',
        '没有需要修正的问题时返回 []'
    ].join('\n');

    const messages = [
        { role: 'system', content: system },
        { role: 'user', content: distributionText }
    ];
    const response = await callOrganizeAI(messages);
    return parseFolderCorrections(response);
}

/**
 * 解析 AI 返回的修正建议 JSON 数组（兼容 ```json 代码块包裹）
 * @param {string} response - AI 响应文本
 * @returns {Array<{from: string, to: string, reason: string}>}
 */
function parseFolderCorrections(response) {
    const cleaned = response.replace(/```json\s*|```\s*/g, '').trim();
    const match = cleaned.match(/\[[\s\S]*\]/);
    const arr = match ? JSON.parse(match[0]) : JSON.parse(cleaned);
    return (Array.isArray(arr) ? arr : []).filter(c => {
        return c && c.from && c.to && String(c.from).trim() !== String(c.to).trim();
    }).map(c => ({
        from: String(c.from).trim(),
        to: String(c.to).trim(),
        reason: c.reason ? String(c.reason) : ''
    }));
}

/**
 * 应用修正建议：将操作中 target 以 from 开头的路径替换为 to
 * @param {Array<Object>} operations - 操作列表
 * @param {Array<{from: string, to: string}>} corrections - 修正建议
 * @returns {{operations: Array<Object>, adjustCount: number}}
 */
function applyFolderCorrections(operations, corrections) {
    let adjustCount = 0;
    const adjustedOps = operations.map(op => {
        if (!op || !op.target) return op;
        const rawTarget = String(op.target).replace(/\\/g, '/');
        let newTarget = rawTarget;
        let changed = false;
        for (const c of corrections) {
            const prefix = c.from + '/';
            if (newTarget === c.from || newTarget.startsWith(prefix)) {
                newTarget = c.to + newTarget.slice(c.from.length);
                changed = true;
            }
        }
        if (changed) {
            adjustCount++;
            return { ...op, target: newTarget };
        }
        return op;
    });
    return { operations: adjustedOps, adjustCount: adjustCount };
}
