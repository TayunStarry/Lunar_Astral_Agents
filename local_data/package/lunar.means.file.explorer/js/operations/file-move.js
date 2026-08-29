/**
 * 文件移动模块
 * 负责文件移动的模态框交互、冲突处理与拖放移动
 */

/** 待确认的移动参数（冲突模态框重试时使用） */
let pendingMoveArgs = null;

/**
 * 打开移动目标选择模态框
 * @param {FileManager} fileManager - 文件管理器实例
 */
async function showMoveModal(fileManager) {
    if (fileManager.selectedFiles.size === 0) {
        showToast('请先选择要移动的项目', 'info');
        return;
    }

    const folderList = document.getElementById('move-folder-list');
    folderList.innerHTML = '';

    // 当前层级的子文件夹列表（渲染为可点击的 chip）
    const folders = fileManager.files.filter(f => f.isDir);
    if (folders.length === 0) {
        const empty = document.createElement('div');
        empty.className = 'move-folder-empty';
        empty.innerHTML = '<i class="fas fa-folder-open"></i> 当前层级没有子文件夹，可手动输入目标路径';
        folderList.appendChild(empty);
    } else {
        folders.forEach(folder => {
            const chip = document.createElement('button');
            chip.type = 'button';
            chip.className = 'move-folder-chip';
            chip.dataset.path = folder.path;
            chip.innerHTML = `<i class="fas fa-folder"></i> ${folder.name}`;
            chip.addEventListener('click', () => {
                folderList.querySelectorAll('.move-folder-chip').forEach(c => c.classList.remove('selected'));
                chip.classList.add('selected');
                document.getElementById('move-modal-path').value = folder.path;
            });
            folderList.appendChild(chip);
        });
    }

    document.getElementById('move-modal-title').textContent = `移动 ${fileManager.selectedFiles.size} 个项目`;
    document.getElementById('move-modal-message').textContent = '请选择目标文件夹（子文件夹或手动输入路径）';
    document.getElementById('move-modal-path').value = '';
    document.getElementById('move-modal').classList.add('show');
}

/**
 * 关闭移动目标选择模态框
 */
function closeMoveModal() {
    document.getElementById('move-modal').classList.remove('show');
}

/**
 * 调用后端文件移动接口
 * @param {Array<string>} sources - 源路径列表（相对 LocalDir）
 * @param {string} targetDir - 目标文件夹（相对 LocalDir，空表示根目录）
 * @param {string} strategy - 冲突策略: ask / auto_rename / overwrite
 * @returns {Promise<Object|null>} 后端响应，失败返回 null
 */
async function callMoveApi(sources, targetDir, strategy) {
    try {
        const response = await fetch('/file/move', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                sources: sources,
                target_dir: targetDir,
                conflict_strategy: strategy,
                create_dirs: false
            })
        });
        if (!response.ok) throw new Error('移动请求失败');
        return await response.json();
    } catch (error) {
        showToast(`移动失败: ${error.message}`, 'error');
        console.error('移动失败:', error);
        return null;
    }
}

/**
 * 发起一次移动并处理结果（ask 预检到冲突时转冲突处理模态框）
 * @param {FileManager} fileManager - 文件管理器实例
 * @param {Array<string>} sources - 源路径列表
 * @param {string} targetDir - 目标文件夹
 * @param {string} strategy - 冲突策略
 */
async function performMove(fileManager, sources, targetDir, strategy) {
    const result = await callMoveApi(sources, targetDir, strategy);
    if (!result) return;

    // 预检到同名冲突：弹出冲突处理模态框
    if (result.conflicts && result.conflicts.length > 0) {
        pendingMoveArgs = { fileManager, sources, targetDir };
        showConflictModal(result.conflicts);
        return;
    }

    if (result.success) {
        showToast('移动成功', 'success');
        fileManager.selectedFiles.clear();
        fileManager.updateBatchActions();
        await fileManager.loadFiles();
    } else {
        showToast(`移动失败: ${result.error || '未知错误'}`, 'error');
    }
}

/**
 * 显示移动冲突处理模态框
 * @param {Array<Object>} conflicts - 冲突列表（含 source / target / is_dir）
 */
function showConflictModal(conflicts) {
    const listHtml = conflicts.map(c => {
        const name = c.source.replace(/\\/g, '/').split('/').pop();
        return `<div class="conflict-item">${c.is_dir ? '<i class="fas fa-folder"></i>' : '<i class="fas fa-file"></i>'} ${name} → ${c.target}</div>`;
    }).join('');
    document.getElementById('conflict-message').innerHTML =
        `目标位置存在 <strong>${conflicts.length}</strong> 个同名项目：<br>${listHtml}<br>请选择处理方式：`;
    document.getElementById('conflict-modal').classList.add('show');
}

/**
 * 关闭冲突处理模态框
 */
function closeConflictModal() {
    document.getElementById('conflict-modal').classList.remove('show');
    pendingMoveArgs = null;
}
