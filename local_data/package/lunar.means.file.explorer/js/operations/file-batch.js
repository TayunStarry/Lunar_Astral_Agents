/**
 * 批量操作模块
 * 负责批量删除与批量压缩（ZIP 保存到服务器）
 */

/**
 * 批量删除
 * @param {Set} selectedFiles - 选中的文件集合
 * @param {Function} onComplete - 完成回调
 */
async function batchDelete(selectedFiles, onComplete) {
    if (selectedFiles.size === 0) return;

    const confirmed = await showConfirmModal(
        '批量删除确认',
        `确定要删除选中的 ${selectedFiles.size} 个项目吗？\n此操作不可撤销，请谨慎操作。`,
        'danger'
    );
    if (!confirmed) return;

    try {
        let deletedCount = 0;
        for (const filePath of selectedFiles) {
            const response = await fetch(`/file/delete/${filePath}`, { method: 'DELETE' });
            if (response.ok) deletedCount++;
        }

        showToast(`成功删除 ${deletedCount} 个项目`, 'success');
        onComplete();
    } catch (error) {
        showToast('批量删除失败', 'error');
        console.error('批量删除失败:', error);
    }
}

/**
 * 批量压缩（ZIP 保存到服务器当前层级，支持文件夹）
 * @param {Object} fileManager - 文件管理器实例
 */
async function batchCompress(fileManager) {
    const selectedFiles = fileManager.selectedFiles;
    if (selectedFiles.size === 0) {
        showToast('请先选择要压缩的文件', 'info');
        return;
    }

    const files = fileManager.files;
    const currentPath = fileManager.currentPath || '';

    try {
        // 收集选中项的相对路径（文件与文件夹均支持，文件夹递归打包）
        const selectedPaths = files
            .filter(file => selectedFiles.has(file.path))
            .map(file => file.path);

        const zipName = `压缩文件_${new Date().getTime()}.zip`;

        const response = await fetch('/file/archive/create', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                paths: selectedPaths,
                zip_name: zipName,
                save_path: currentPath
            })
        });

        if (!response.ok) throw new Error('压缩失败');

        const result = await response.json();
        if (!result.success) throw new Error(result.error || '压缩失败');

        showToast(`成功压缩 ${selectedFiles.size} 个项目到 ${result.path}`, 'success');
        await fileManager.loadFiles();
        fileManager.updateStats();
    } catch (error) {
        showToast('压缩失败', 'error');
        console.error('压缩失败:', error);
    }
}
