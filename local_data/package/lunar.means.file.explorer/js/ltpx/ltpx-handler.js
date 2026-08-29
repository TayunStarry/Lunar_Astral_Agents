/**
 * LTPX AtoA 指令主处理模块
 * 解析自然语言指令并执行 解压 / GGUF 预览 / 查看 / 搜索 / 压缩 / 移动 / 重命名 /
 * 删除 / 新建文件夹 / 自动整理 / 哈希命名 / 选中 / 目录跳转 等操作
 */

// ===== LTPX AtoA 指令主处理 =====
async function handleLTPXInstruction(fm, instruction) {
    const text = instruction || '';
    const lower = text.toLowerCase();
    if (!fm.files || fm.files.length === 0) {
        try { fm.files = await loadFiles(fm.currentPath); } catch (e) { /* 忽略，后续分支兜底 */ }
    }
    // 指令指明目标目录（如「images/xxx 目录下的...」）时先切换到该目录，
    // 保证后续按文件名匹配与目录级操作（哈希命名/整理）作用于正确目录
    await resolveLTPXDirectory(fm, text);

    // ---- 解压 ----
    if (/解压|extract|unzip/.test(lower)) {
        const file = matchLTPXFile(fm.files, extractLTPXFileName(text)) || fm.files.find(f => isZipFile(f.name));
        if (!file) return { success: false, text: '', error: '未找到要解压的 ZIP 压缩包，请确认文件存在' };
        await showZipModal(file, fm); // UI：打开压缩包预览模态窗
        const targetRaw = text.match(/(?:解压到|到|extract to|->)[：: ]*(.+)/i);
        const targetDir = targetRaw ? normalizeLTPXPath(targetRaw[1]) : document.getElementById('zip-extract-path').value.trim();
        const resp = await fetch('/file/archive/extract', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ path: file.path, target_dir: targetDir })
        });
        const data = await resp.json();
        if (!resp.ok || !data.success) return { success: false, text: '', error: (data && data.error) || '解压失败' };
        await fm.loadFiles();
        fm.updateStats();
        closeZipModal();
        return { success: true, text: `已解压 ${data.file_count} 个文件到 ${data.target_dir}` };
    }

    // ---- GGUF / 元数据 ----
    if (/gguf|元数据|metadata/.test(lower)) {
        const file = matchLTPXFile(fm.files, extractLTPXFileName(text)) || fm.files.find(f => isGGUFFile(f.name));
        if (!file) return { success: false, text: '', error: '未找到 GGUF 模型文件' };
        await showGGUFModal(file); // UI：打开 GGUF 元数据预览模态窗
        return { success: true, text: `已打开「${file.name}」的 GGUF 元数据预览` };
    }

    // ---- 查看/打开文件 ----
    if (/查看|打开|预览|view|open|read/i.test(lower)) {
        const file = matchLTPXFile(fm.files, extractLTPXFileName(text));
        if (file && !file.isDir) {
            if (isZipFile(file.name)) {
                await showZipModal(file, fm);
                return { success: true, text: `已打开「${file.name}」的压缩包预览` };
            }
            if (isGGUFFile(file.name)) {
                await showGGUFModal(file);
                return { success: true, text: `已打开「${file.name}」的 GGUF 元数据预览` };
            }
            if (isImageFile(file.name) || isVideoFile(file.name) || isAudioFile(file.name)) {
                previewImage(`/file/read/${file.path}`, file.name);
                return { success: true, text: `已打开「${file.name}」的预览` };
            }
            if (isTextFile(file.name)) {
                await showTextModal(file, fm.currentPath, async () => { await fm.loadFiles(); });
                return { success: true, text: `已打开「${file.name}」的文本预览` };
            }
            return { success: true, text: `已定位到「${file.name}」` };
        }
    }

    // ---- 搜索文件（全量遍历 + 名称模糊匹配） ----
    if (/搜索|查找|查询|find|search/i.test(lower)) {
        const query = (text.match(/(?:搜索|查找|查询|find|search)\s*(?:一下|下)?\s*[：: ]*(.+)/i) || [])[1];
        if (!query) return { success: false, text: '', error: '请提供搜索关键字，例如：搜索 报告.pdf' };
        await fm.searchFiles(query.trim());
        const results = fm.isSearching ? fm.searchResults : [];
        if (results.length === 0) {
            fm.clearSearch();
            return { success: true, text: `未找到与「${query.trim()}」相关的文件` };
        }
        const lines = [`搜索「${query.trim()}」共找到 ${results.length} 个结果：`];
        results.slice(0, 50).forEach(f => {
            lines.push(f.isDir ? `- [目录] ${f.path}/` : `- [文件] ${f.path}（${formatFileSize(f.size || 0)}）`);
        });
        if (results.length > 50) lines.push(`... 其余 ${results.length - 50} 个结果请在文件管理器中查看`);
        return { success: true, text: lines.join('\n') };
    }

    // ---- 压缩 ----
    if (/压缩|打包|compress|zip/.test(lower) && !/解压|extract|unzip/.test(lower)) {
        const nameMatch = text.match(/(?:压缩|打包|compress)[：: ]*(.+)/i);
        let targets = nameMatch ? collectLTPXFiles(fm.files, nameMatch[1].replace(/(?:压缩|打包)$/i, '')) : [];
        if (targets.length === 0) {
            targets = fm.files.filter(f => !f.isDir && !isZipFile(f.name)).slice(0, 50);
        }
        if (targets.length === 0) return { success: false, text: '', error: '当前目录没有可压缩的文件' };
        // UI：选中待压缩文件
        targets.forEach(f => {
            fm.selectedFiles.add(f.path);
            updateFileCardSelection(f, true);
        });
        fm.updateBatchActions();
        const zipName = `压缩文件_${new Date().getTime()}.zip`;
        const resp = await fetch('/file/archive/create', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ paths: targets.map(f => f.path), zip_name: zipName, save_path: fm.currentPath || '' })
        });
        const data = await resp.json();
        if (!resp.ok || !data.success) return { success: false, text: '', error: (data && data.error) || '压缩失败' };
        await fm.loadFiles();
        fm.updateStats();
        return { success: true, text: `已压缩 ${targets.length} 个项目到 ${data.path}` };
    }

    // ---- 移动 ----
    if (/移动|move|转移|剪切/.test(lower)) {
        const m = text.match(/(?:把|将|移动|move)?\s*([^到]+?)\s*(?:到|移动到|move to|->)\s*(.+)/i);
        if (m) {
            const allFlag = /所有|全部|整个/.test(m[1]);
            const sources = allFlag ? fm.files.filter(f => !f.isDir) : collectLTPXFiles(fm.files, m[1]);
            if (sources.length === 0) return { success: false, text: '', error: '未找到要移动的文件' };
            const targetDir = normalizeLTPXPath(m[2].replace(/(?:目录|文件夹)(?:下|中|里|内)?\s*$/, ''));
            const result = await callMoveApi(sources.map(f => f.path), targetDir, 'auto_rename');
            if (!result) return { success: false, text: '', error: '移动失败' };
            if (result.conflicts && result.conflicts.length > 0) {
                return { success: true, text: `检测到 ${result.conflicts.length} 处同名冲突，请在文件管理器中确认处理方式` };
            }
            if (result.success) {
                fm.selectedFiles.clear();
                fm.updateBatchActions();
                await fm.loadFiles();
                return { success: true, text: `已将 ${sources.length} 个项目移动到 ${targetDir || '根目录'}` };
            }
            return { success: false, text: '', error: result.error || '移动失败' };
        }
        // 未提供目标：选中文件并打开移动目标选择界面
        const f = matchLTPXFile(fm.files, extractLTPXFileName(text));
        if (f) {
            fm.selectedFiles.add(f.path);
            updateFileCardSelection(f, true);
            fm.updateBatchActions();
        }
        if (fm.selectedFiles.size === 0) return { success: false, text: '', error: '未找到要移动的文件' };
        await showMoveModal(fm);
        return { success: true, text: `已打开移动目标选择界面（选中 ${fm.selectedFiles.size} 项），请选择目标文件夹` };
    }

    // ---- 重命名 ----
    if (/重命名|改名|rename/.test(lower)) {
        const m = text.match(/(?:把|将|重命名|改名)?\s*(.+?)\s*(?:重命名为|为|改成|->|rename to)\s*(.+)/i);
        if (m) {
            // m[1] 可能形如「images/xxx/ 目录下的 file.jpg」，先切换目录（开头已做），此处提取最终文件名匹配
            const file = matchLTPXFile(fm.files, extractLTPXFileName(m[1]));
            if (!file) return { success: false, text: '', error: '未找到要重命名的文件' };
            const newName = m[2].trim();
            try {
                if (file.isDir) await renameDirectory(file, newName, fm.currentPath);
                else await renameSingleFile(file, newName, fm.currentPath);
                await fm.loadFiles();
                return { success: true, text: `已将「${file.name}」重命名为「${newName}」` };
            } catch (e) {
                return { success: false, text: '', error: '重命名失败: ' + (e.message || e) };
            }
        }
        // 未提供新名：选中文件并打开重命名输入界面（用户输入后自动完成）
        const file = matchLTPXFile(fm.files, extractLTPXFileName(text));
        if (!file) return { success: false, text: '', error: '未找到要重命名的文件' };
        fm.selectedFiles.add(file.path);
        updateFileCardSelection(file, true);
        fm.updateBatchActions();
        setTimeout(() => { renameFile(file, fm.currentPath, () => fm.loadFiles()); }, 50);
        return { success: true, text: `已打开「${file.name}」的重命名输入界面，请输入新名称` };
    }

    // ---- 删除 ----
    if (/删除|移除|delete|remove/.test(lower)) {
        const name = (text.match(/(?:删除|移除|delete|remove)[：: ]*(.+)/i) || [])[1];
        const file = matchLTPXFile(fm.files, extractLTPXFileName(name));
        if (!file) return { success: false, text: '', error: '未找到要删除的文件' };
        fm.selectedFiles.add(file.path);
        updateFileCardSelection(file, true);
        fm.updateBatchActions();
        const resp = await fetch(`/file/delete/${file.path}`, { method: 'DELETE' });
        if (!resp.ok) return { success: false, text: '', error: '删除失败' };
        await fm.loadFiles();
        return { success: true, text: `已删除「${file.name}」` };
    }

    // ---- 新建文件夹 ----
    if (/新建|创建|mkdir/.test(lower) && /文件夹|目录|folder|dir/.test(lower)) {
        const name = (text.match(/(?:新建|创建|mkdir)[：: ]*(.+)/i) || [])[1];
        if (name) {
            await createDirectory(name.trim(), fm.currentPath);
            await fm.loadFiles();
            return { success: true, text: `已新建文件夹「${name.trim()}」` };
        }
        setTimeout(() => { createNewFolder(fm.currentPath, () => fm.loadFiles()); }, 50);
        return { success: true, text: '已打开新建文件夹输入界面，请输入文件夹名称' };
    }

    // ---- 自动整理 ----
    if (/整理|organize/.test(lower)) {
        const nonDir = fm.files.filter(f => !f.isDir);
        if (nonDir.length === 0) return { success: false, text: '', error: '当前目录没有可整理的文件' };
        startSmartOrganize(fm); // 后台执行，UI 显示整理模态窗与进度
        return { success: true, text: `已对当前目录（${fm.currentPath || '根目录'}）启动智能整理，可在文件管理器中查看进度` };
    }

    // ---- 哈希命名 ----
    if (/哈希|hash/.test(lower)) {
        const nonDir = fm.files.filter(f => !f.isDir);
        if (nonDir.length === 0) return { success: false, text: '', error: '当前目录没有可命名的文件' };
        const resp = await fetch('/file/hash-rename', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ path: fm.currentPath || '' })
        });
        const data = await resp.json();
        if (!resp.ok || !data.success) return { success: false, text: '', error: (data && data.error) || '哈希命名失败' };
        await fm.loadFiles();
        const unchanged = (data.results || []).filter(r => r.unchanged).length;
        const suffix = (data.renamed || 0) === 0 && unchanged > 0 ? `（${unchanged} 个文件已为哈希名，无需重命名）` : '';
        return { success: true, text: `哈希命名完成：${data.renamed || 0} 个文件已重命名${suffix}` };
    }

    // ---- 选中文件 ----
    if (/选中|选择|select/.test(lower)) {
        const name = (text.match(/(?:选中|选择|select)[：: ]*(.+)/i) || [])[1];
        const file = matchLTPXFile(fm.files, extractLTPXFileName(name));
        if (!file) return { success: false, text: '', error: '未找到要选中的文件' };
        fm.selectedFiles.add(file.path);
        updateFileCardSelection(file, true);
        fm.updateBatchActions();
        return { success: true, text: `已选中「${file.name}」` };
    }

    // ---- 目录跳转 / 文件列表（兜底） ----
    const pathRaw = extractLTPXPath(text);
    const path = pathRaw ? normalizeLTPXPath(pathRaw) : fm.currentPath;
    try {
        const files = await loadFiles(path);
        fm.currentPath = path;
        fm.selectedFiles.clear();
        fm.updateBatchActions();
        fm.currentPage = 1;
        fm.files = files;
        fm.isSearching = false;
        fm.updateFileGrid();
        fm.updateStats();
        fm.updateBreadcrumb();
        return { success: true, text: formatLTPXList(files, path) };
    } catch (e) {
        return { success: false, text: '', error: '无法读取该路径: ' + (e.message || e) };
    }
}
