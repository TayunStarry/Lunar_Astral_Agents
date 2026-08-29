/**
 * LTPX AtoA 辅助函数模块
 * 提供结果回传、路径归一化、指令参数提取、文件匹配与列表渲染等辅助能力
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

/** 从指令中提取路径/文件参数（引号/书名号 > 盘符路径 > 含斜杠路径 > 带扩展名文件名） */
function extractLTPXPath(instruction) {
    const quoted = instruction.match(/[「『"'']([^「」『』"'']+)[」』"']/);
    if (quoted) return quoted[1].trim();
    const drive = instruction.match(/[A-Za-z]:[\\/][^\s，。；,;、]*/);
    if (drive) return drive[0].trim();
    const slash = instruction.match(/(?:^|[，,、\s])([^，,、\s]*[\\/][^，,、\s]*)(?=[，,。;；、\s]|$)/);
    if (slash) return slash[1].trim();
    const fileName = instruction.match(/([^\s，,。；;()（）]+\.[A-Za-z0-9]{1,6})\b/i);
    if (fileName) return fileName[1].trim();
    return null;
}

/** 从「路径/目录/文件」混合串中提取最终文件名（忽略目录方位词与连接词）
 *  例1:「images/DeepSeek/ 目录下的 c927ee92537959f1.jpg」→「c927ee92537959f1.jpg」
 *  例2:「解压 压缩包.zip 到 images/out/」→「压缩包.zip」 */
function extractLTPXFileName(raw) {
    if (!raw) return null;
    let s = String(raw).trim();
    s = s.replace(/目录(?:下|中|里|内)?的?|文件夹(?:下|中|里|内)?的?/g, ' ');
    s = s.replace(/[\\/]+/g, ' ');
    const tokens = s.split(/[\s，,。；;、（）()：:]+/).filter(Boolean);
    if (!tokens.length) return null;
    // 优先取带扩展名且最靠后的段（「目录下的 a.jpg 重命名为 b.png」应取 a.jpg），否则取最后一段
    let tail = tokens[tokens.length - 1];
    for (let i = tokens.length - 1; i >= 0; i--) {
        if (/\.[A-Za-z0-9]{1,6}$/i.test(tokens[i])) { tail = tokens[i]; break; }
    }
    return tail.replace(/^(?:的|里|中|内|下|在|位于)+/, '') || null;
}

/** 从指令中解析目标目录（如「images/xxx 目录下的」「xxx文件夹中的」），存在则切换到该目录并刷新 UI */
async function resolveLTPXDirectory(fm, text) {
    if (!text) return false;
    const m = text.match(/([^，,。;；、\s()（）]+(?:[\\/][^，,。;；、\s()（）]*)*?)\s*(?:目录|文件夹)(?:下|中|里|内)?/);
    if (!m) return false;
    // 「移动到 X 目录下」中的 X 是目标目录，不应切换当前工作目录
    if (/(?:到|移动到|移到|移入|迁入|复制到|剪切到|拷到)\s*$/.test(text.slice(0, m.index))) return false;
    const dir = normalizeLTPXPath(m[1].replace(/[\\/]+$/, ''));
    if (!dir) return false;
    try {
        const files = await loadFiles(dir);
        fm.currentPath = dir;
        fm.files = files;
        fm.currentPage = 1;
        fm.isSearching = false;
        fm.updateFileGrid();
        fm.updateStats();
        fm.updateBreadcrumb();
        return true;
    } catch (e) {
        return false; // 目录不可达：保持当前上下文，后续分支兜底
    }
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

/** 按名称列表在当前目录收集文件 */
function collectLTPXFiles(files, raw) {
    if (!raw) return [];
    const names = String(raw).split(/\s*(?:和|与|、|,|，|及)\s*|\s+/).filter(Boolean);
    const matched = [];
    names.forEach(n => {
        const f = matchLTPXFile(files, extractLTPXFileName(n));
        if (f) matched.push(f);
    });
    return matched;
}
