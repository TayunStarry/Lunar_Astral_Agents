/**
 * 智能整理主流程模块
 * 负责哈希命名与智能整理的多阶段编排（预处理 → 逐文件 AI 决策 → 分布审核 → 执行）
 */

/**
 * 哈希命名：基于文件内容 MD5 的前 16 位重命名当前层级全部文件
 * 重名（内容相同或与已有条目冲突）时在文件名后追加 '+'
 * @param {Object} fileManager - 文件管理器实例
 */
async function startHashRename(fileManager) {
    const targetFiles = fileManager.files.filter(f => !f.isDir);
    if (targetFiles.length === 0) {
        showToast('当前层级没有可命名的文件', 'info');
        return;
    }

    const confirmed = await showConfirmModal(
        '确认哈希命名',
        `将基于文件内容 MD5 的前 16 位重命名当前层级的全部 ${targetFiles.length} 个文件？\n重复内容将在文件名后追加 '+' 区分。`,
        'warning'
    );
    if (!confirmed) return;

    try {
        const response = await fetch('/file/hash-rename', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ path: fileManager.currentPath || '' })
        });
        const result = await response.json();
        if (!response.ok || !result.success) {
            throw new Error(result.error || '哈希命名失败');
        }

        const renamedCount = result.renamed || 0;
        const unchangedCount = (result.results || []).filter(r => r.unchanged).length;
        const message = `哈希命名完成：${renamedCount} 个文件已重命名`
            + (unchangedCount ? `，${unchangedCount} 个已是哈希名` : '');
        showToast(message, 'success');
        await fileManager.loadFiles();
        fileManager.updateStats();
    } catch (error) {
        showToast(error.message || '哈希命名失败', 'error');
        console.error('哈希命名失败:', error);
    }
}

/**
 * 启动智能整理流程
 * @param {FileManager} fileManager - 文件管理器实例
 */
async function startSmartOrganize(fileManager) {
    if (isOrganizing) return;

    const targetFiles = fileManager.files.filter(f => !f.isDir);
    if (targetFiles.length === 0) {
        showToast('当前层级没有可整理的文件', 'info');
        return;
    }

    isOrganizing = true;
    const modal = document.getElementById('organize-modal');
    const actionsEl = document.getElementById('organize-actions');
    document.getElementById('organize-log').innerHTML = '';
    actionsEl.style.display = 'none';
    modal.classList.add('show');

    try {
        // 阶段 1: 预处理文件
        const folders = fileManager.files.filter(f => f.isDir).map(f => f.name);
        addOrganizeLog(`开始整理当前层级 ${targetFiles.length} 个文件...`);
        addOrganizeLog(`当前文件夹列表: ${folders.length ? folders.join('、') : '（无）'}`);

        const fileDataList = [];
        for (let i = 0; i < targetFiles.length; i++) {
            const file = targetFiles[i];
            setOrganizeProgress(Math.round((i / targetFiles.length) * 45), `预处理文件 (${i + 1}/${targetFiles.length}): ${file.name}`);
            const data = await preprocessOrganizeFile(file);
            if (data.type === 'meta' && data.note) {
                addOrganizeLog(`降级处理: ${file.name} — ${data.note}`, 'warning');
            }
            fileDataList.push(data);
        }

        // 阶段 2: 逐文件 AI 决策（每次携带当前已知文件夹列表，含上一文件拟新增的文件夹）
        const knownFolders = [...folders]; // 已知文件夹集合（整理过程中持续累积）
        const operations = [];
        for (let i = 0; i < fileDataList.length; i++) {
            const file = fileDataList[i];
            setOrganizeProgress(5 + Math.round((i / fileDataList.length) * 65), `AI 处理文件 (${i + 1}/${fileDataList.length}): ${file.name}`);
            addOrganizeLog(`AI 处理文件 (${i + 1}/${fileDataList.length}): ${file.name}`);
            try {
                const decision = await askFileDecision(file, knownFolders);
                const op = buildFileOperation(file.name, decision, knownFolders);
                if (op) {
                    operations.push(op);
                    addOrganizeLog(`决定: ${file.name} → ${op.type === 'move' ? `移动到「${op.target}」` : `重命名为「${op.target}」`}`, 'success');
                } else {
                    addOrganizeLog(`决定: ${file.name} 保持不动`, 'info');
                }
            } catch (err) {
                addOrganizeLog(`${file.name} AI 处理失败: ${err.message}，跳过`, 'error');
            }
        }

        if (operations.length === 0) {
            addOrganizeLog('AI 未生成任何整理操作，文件可能已足够整洁', 'warning');
            setOrganizeProgress(100, '整理完成（无操作）');
            showToast('智能整理完成，未产生操作', 'info');
            return;
        }

        // 阶段 3: AI 全局审核文件夹分布（判定重复/不合理的文件夹并改写移动操作）
        setOrganizeProgress(72, 'AI 审核文件夹分布是否合理...');
        addOrganizeLog('AI 审核文件夹分布（检查重复/不合理文件夹）...');
        try {
            const distributionText = summarizeFolderDistribution(operations, knownFolders);
            const corrections = await auditFolderDistribution(distributionText);
            if (corrections.length > 0) {
                addOrganizeLog(`AI 判定 ${corrections.length} 处文件夹分布需修正`, 'warning');
                corrections.forEach(c => {
                    addOrganizeLog(`修正建议: 「${c.from}」→「${c.to}」（${c.reason || '分布不合理'}）`, 'warning');
                });
                const adjusted = applyFolderCorrections(operations, corrections);
                if (adjusted.adjustCount > 0) {
                    operations.length = 0;
                    operations.push(...adjusted.operations);
                    addOrganizeLog(`已改写 ${adjusted.adjustCount} 处操作目标`, 'success');
                }
            } else {
                addOrganizeLog('AI 审核通过：文件夹分布合理', 'success');
            }
        } catch (err) {
            addOrganizeLog(`分布审核失败: ${err.message}，按当前方案执行`, 'warning');
        }

        // 阶段 4: 执行整理操作
        addOrganizeLog(`共 ${operations.length} 个操作，开始执行...`);
        setOrganizeProgress(85, '正在执行整理操作...');
        const result = await executeOrganizeOperations(fileManager.currentPath, operations);
        setOrganizeProgress(100, '整理完成');

        const successCount = result.success_count || 0;
        const failCount = result.fail_count || 0;
        if (successCount > 0) {
            addOrganizeLog(`执行成功 ${successCount} 个操作`, 'success');
        }
        if (failCount > 0) {
            addOrganizeLog(`执行失败 ${failCount} 个操作，详见下方日志`, 'error');
            (result.results || []).filter(r => !r.success).forEach(r => {
                addOrganizeLog(`失败: ${r.source} → ${r.target || ''}（${r.error || '未知错误'}）`, 'error');
            });
        }
        showToast(`智能整理完成：${successCount} 成功，${failCount} 失败`, failCount ? 'warning' : 'success');
        await fileManager.loadFiles();
    } catch (err) {
        addOrganizeLog(`整理流程失败: ${err.message}`, 'error');
        setOrganizeProgress(100, '整理失败');
        showToast('智能整理失败', 'error');
        console.error('智能整理失败:', err);
    } finally {
        actionsEl.style.display = 'flex';
        isOrganizing = false;
    }
}
