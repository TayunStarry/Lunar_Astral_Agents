/**
 * 事件绑定模块
 * 负责文件管理器的所有事件绑定（上传、新建、批量操作、各模态框、拖放移动、搜索、键盘）
 */

/**
 * 绑定事件
 * @param {Object} fileManager - 文件管理器实例
 */
function bindEvents(fileManager) {
    // 文件上传
    document.getElementById('file-upload').addEventListener('change', async (e) => {
        await handleFileUpload(e.target.files, fileManager.currentPath, async () => {
            await fileManager.loadFiles();
        });
        e.target.value = '';
    });

    // 新建文件夹
    document.getElementById('new-folder').addEventListener('click', async () => {
        await createNewFolder(fileManager.currentPath, async () => {
            await fileManager.loadFiles();
        });
    });

    // 批量删除
    document.getElementById('batch-delete').addEventListener('click', async () => {
        await batchDelete(fileManager.selectedFiles, async () => {
            fileManager.selectedFiles.clear();
            fileManager.updateBatchActions();
            await fileManager.loadFiles();
        });
    });

    // 批量压缩
    document.getElementById('batch-compress').addEventListener('click', async () => {
        await batchCompress(fileManager);
    });

    // 批量移动
    document.getElementById('batch-move').addEventListener('click', () => {
        showMoveModal(fileManager);
    });

    // 全选当前层级所有项目
    document.getElementById('batch-select-all').addEventListener('click', () => {
        fileManager.selectAllVisible();
    });

    // 取消全选
    document.getElementById('batch-clear').addEventListener('click', () => {
        fileManager.clearSelection();
    });

    // 移动模态框：确认 / 取消 / 关闭 / 点击遮罩
    document.getElementById('move-modal-confirm').addEventListener('click', async () => {
        const targetDir = document.getElementById('move-modal-path').value.trim();
        if (!targetDir) {
            showToast('请选择或输入目标文件夹', 'info');
            return;
        }
        const sources = Array.from(fileManager.selectedFiles);
        closeMoveModal();
        await performMove(fileManager, sources, targetDir, 'ask');
    });
    document.getElementById('move-modal-cancel').addEventListener('click', closeMoveModal);
    document.getElementById('move-modal-close').addEventListener('click', closeMoveModal);
    document.getElementById('move-modal').addEventListener('click', (e) => {
        if (e.target === document.getElementById('move-modal')) closeMoveModal();
    });

    // 冲突处理模态框：自动重命名 / 覆盖 / 取消
    document.getElementById('conflict-rename').addEventListener('click', async () => {
        const args = pendingMoveArgs;
        closeConflictModal();
        if (args) await performMove(args.fileManager, args.sources, args.targetDir, 'auto_rename');
    });
    document.getElementById('conflict-overwrite').addEventListener('click', async () => {
        const args = pendingMoveArgs;
        closeConflictModal();
        if (args) await performMove(args.fileManager, args.sources, args.targetDir, 'overwrite');
    });
    document.getElementById('conflict-cancel').addEventListener('click', closeConflictModal);
    document.getElementById('conflict-modal-close').addEventListener('click', closeConflictModal);
    document.getElementById('conflict-modal').addEventListener('click', (e) => {
        if (e.target === document.getElementById('conflict-modal')) closeConflictModal();
    });

    // 智能整理
    document.getElementById('smart-organize-btn').addEventListener('click', () => {
        startSmartOrganize(fileManager);
    });

    // 哈希命名
    document.getElementById('hash-rename-btn').addEventListener('click', () => {
        startHashRename(fileManager);
    });
    document.getElementById('organize-close-btn').addEventListener('click', () => {
        document.getElementById('organize-modal').classList.remove('show');
    });
    document.getElementById('organize-modal-close').addEventListener('click', () => {
        if (!isOrganizing) document.getElementById('organize-modal').classList.remove('show');
    });

    // 图片转码
    document.getElementById('image-convert-btn').addEventListener('click', () => {
        openConvertModal(fileManager);
    });
    document.getElementById('convert-modal-close').addEventListener('click', closeConvertModal);
    document.getElementById('convert-modal-cancel').addEventListener('click', closeConvertModal);
    document.getElementById('convert-modal').addEventListener('click', (e) => {
        if (e.target === document.getElementById('convert-modal')) closeConvertModal();
    });
    document.querySelectorAll('#convert-mode-tabs .convert-mode-tab').forEach(tab => {
        tab.addEventListener('click', () => {
            convertState.mode = tab.dataset.mode;
            updateConvertModeTabs();
            const folderImages = fileManager.files.filter(f => !f.isDir && isConvertibleImage(f.name));
            const selectedImages = Array.from(fileManager.selectedFiles)
                .map(path => fileManager.files.find(f => f.path === path))
                .filter(f => f && !f.isDir && isConvertibleImage(f.name));
            updateConvertModeHint(folderImages, selectedImages);
        });
    });
    document.querySelectorAll('#convert-source-tabs .convert-tab').forEach(tab => {
        tab.addEventListener('click', () => {
            convertState.sourceFormat = tab.dataset.format;
            setConvertTabActive('convert-source-tabs', tab.dataset.format);
        });
    });
    document.querySelectorAll('#convert-target-tabs .convert-tab').forEach(tab => {
        tab.addEventListener('click', () => {
            convertState.targetFormat = tab.dataset.format;
            setConvertTabActive('convert-target-tabs', tab.dataset.format);
        });
    });
    document.getElementById('convert-start').addEventListener('click', () => {
        startImageConvert(fileManager);
    });

    // GGUF 预览
    document.getElementById('gguf-modal-close').addEventListener('click', closeGGUFModal);
    document.getElementById('gguf-modal').addEventListener('click', (e) => {
        if (e.target === document.getElementById('gguf-modal')) closeGGUFModal();
    });
    document.getElementById('gguf-search-input').addEventListener('input', (e) => {
        const query = e.target.value.trim().toLowerCase();
        const rows = document.querySelectorAll('#gguf-metadata-body tr[data-key]');
        rows.forEach(row => {
            const key = row.getAttribute('data-key') || '';
            const value = row.getAttribute('data-value') || '';
            row.style.display = (key.includes(query) || value.includes(query)) ? '' : 'none';
        });
    });

    // ZIP 预览与解压
    document.getElementById('zip-modal-close').addEventListener('click', closeZipModal);
    document.getElementById('zip-modal').addEventListener('click', (e) => {
        if (e.target === document.getElementById('zip-modal')) closeZipModal();
    });
    document.getElementById('zip-extract-btn').addEventListener('click', () => {
        startZipExtract(fileManager);
    });
    // 路径输入时实时更新提示
    document.getElementById('zip-extract-path').addEventListener('input', (e) => {
        const hintEl = document.getElementById('zip-extract-hint');
        if (e.target.value.trim()) {
            hintEl.innerHTML = `<i class="fas fa-info-circle"></i> 将解压到：${mediaEscapeHTML(e.target.value.trim())}`;
        } else {
            hintEl.innerHTML = '';
        }
    });

    // 拖放移动：在文件网格上通过事件委托处理
    const fileGrid = document.getElementById('file-grid');
    fileGrid.addEventListener('dragstart', (e) => {
        const card = e.target.closest('.file-card');
        if (!card || !card.dataset.path) return;
        const path = card.dataset.path;
        // 拖拽选中项时移动全部选中项，否则移动单个
        const movePaths = (fileManager.selectedFiles.has(path) && fileManager.selectedFiles.size > 1)
            ? Array.from(fileManager.selectedFiles)
            : [path];
        e.dataTransfer.setData('text/plain', path);
        e.dataTransfer.setData('application/x-fm-move', JSON.stringify(movePaths));
        e.dataTransfer.effectAllowed = 'move';
        card.classList.add('dragging');
    });
    fileGrid.addEventListener('dragend', (e) => {
        const card = e.target.closest('.file-card');
        if (card) card.classList.remove('dragging');
        fileGrid.querySelectorAll('.file-card.drop-target').forEach(c => c.classList.remove('drop-target'));
    });
    fileGrid.addEventListener('dragover', (e) => {
        const card = e.target.closest('.file-card');
        fileGrid.querySelectorAll('.file-card.drop-target').forEach(c => { if (c !== card) c.classList.remove('drop-target'); });
        // 仅文件夹卡片允许作为投放目标
        if (card && card.dataset.path) {
            const targetFile = fileManager.files.find(f => f.path === card.dataset.path);
            if (targetFile && targetFile.isDir) {
                e.preventDefault();
                e.dataTransfer.dropEffect = 'move';
                card.classList.add('drop-target');
            }
        }
    });
    fileGrid.addEventListener('drop', (e) => {
        e.preventDefault();
        fileGrid.querySelectorAll('.file-card.drop-target').forEach(c => c.classList.remove('drop-target'));
        const targetCard = e.target.closest('.file-card');
        const movePathsRaw = e.dataTransfer.getData('application/x-fm-move');
        if (!targetCard || !movePathsRaw) return;
        const targetFile = fileManager.files.find(f => f.path === targetCard.dataset.path);
        if (!targetFile || !targetFile.isDir) return;

        let sourcePaths;
        try {
            sourcePaths = JSON.parse(movePathsRaw);
        } catch (err) {
            sourcePaths = [e.dataTransfer.getData('text/plain')];
        }
        const validPaths = sourcePaths.filter(p => p && p !== targetFile.path);
        if (validPaths.length === 0) {
            showToast('没有可移动的项目', 'info');
            return;
        }
        performMove(fileManager, validPaths, targetFile.path, 'ask');
    });

    // 返回按钮
    document.getElementById('back-button').addEventListener('click', () => {
        fileManager.goBack();
    });

    // 文本模态框点击关闭
    document.getElementById('text-modal').addEventListener('click', (e) => {
        closeTextModal(e);
    });

    // 搜索输入
    document.getElementById('search-input').addEventListener('input', (e) => {
        fileManager.handleSearch(e);
    });

    // 搜索清除按钮
    document.getElementById('search-clear').addEventListener('click', () => {
        fileManager.clearSearch();
    });

    // 键盘事件
    document.addEventListener('keydown', (e) => {
        fileManager.handleKeyboard(e);
    });
}
