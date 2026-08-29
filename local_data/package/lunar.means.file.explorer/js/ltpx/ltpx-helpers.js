/**
 * LTPX AtoA 辅助函数模块
 * 提供结果回传、路径归一化、文件匹配与列表渲染等辅助能力
 */

/** 回传执行结果给琉璃主窗口（琉璃再转交月华） */
function postLTPXResult(requestId, success, text, error) {
    try {
        window.parent.postMessage({
            type: 'ltpx_result',
            request_id: requestId,
            success: !!success,
            text: text || '',
            error: error || '',
            keep_open: true // 文件管理器执行后保持页面展示（展示执行后的路径/选中状态），由用户手动关闭
        }, '*');
    } catch (e) {
        console.error('LTPX 回传结果失败:', e);
    }
}

/** 归一化路径：兼容绝对路径(d:/xxx)、含 local_data 前缀、相对路径 */
function normalizeLTPXPath(p) {
    if (p === undefined || p === null) return '';
    let path = String(p).replace(/\\/g, '/').trim();
    path = path.replace(/^[A-Za-z]:/, '');
    const idx = path.indexOf('/local_data');
    if (idx !== -1) path = path.slice(idx + '/local_data'.length);
    return path.replace(/^\/+/, '').replace(/\/+$/, '');
}

/** 在当前目录文件列表中按名称匹配文件（支持去扩展名/忽略大小写） */
function matchLTPXFile(files, name) {
    if (!name) return null;
    const target = String(name).trim().toLowerCase().replace(/[\\/]+$/, '');
    if (!target) return null;
    return files.find(f => {
        const n = f.name.toLowerCase();
        if (n === target) return true;
        if (!f.isDir && n.replace(/\.[^.]+$/, '') === target) return true;
        return false;
    }) || null;
}

/** 将文件列表渲染为文本 */
function formatLTPXList(files, currentPath) {
    const lines = [`当前路径：${currentPath || '（根目录）'}`, `共 ${files.length} 项：`];
    files.forEach(f => {
        if (f.isDir) lines.push(`- [目录] ${f.name}/`);
        else lines.push(`- [文件] ${f.name}（${formatFileSize(f.size || 0)}）`);
    });
    return lines.join('\n');
}
