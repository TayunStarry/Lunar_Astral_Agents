/**
 * 状态与导航渲染模块
 * 负责统计信息、面包屑、批量操作按钮、卡片选中状态与上传进度的渲染
 */

/**
 * 更新统计信息
 * @param {Array} files - 文件列表
 * @param {boolean} isSearching - 是否在搜索
 * @param {Array} searchResults - 搜索结果
 */
function updateStats(files, isSearching = false, searchResults = []) {
    const totalFilesElement = document.getElementById('total-files');
    const totalFoldersElement = document.getElementById('total-folders');
    const totalSizeElement = document.getElementById('total-size');

    const targetFiles = isSearching ? searchResults : files;
    const folders = targetFiles.filter(f => f.isDir).length;
    const fileCount = targetFiles.filter(f => !f.isDir).length;
    const totalSize = targetFiles.reduce((sum, file) => sum + file.size, 0);

    totalFilesElement.textContent = fileCount;
    totalFoldersElement.textContent = folders;
    totalSizeElement.textContent = formatFileSize(totalSize);

    // 智能整理 / 哈希命名按钮：非搜索状态且当前层级文件数达到阈值时显示
    const organizeBtn = document.getElementById('smart-organize-btn');
    const showOrganize = (!isSearching && fileCount >= ORGANIZE_THRESHOLD);
    if (organizeBtn) {
        organizeBtn.style.display = showOrganize ? 'inline-flex' : 'none';
    }
    const hashRenameBtn = document.getElementById('hash-rename-btn');
    if (hashRenameBtn) {
        hashRenameBtn.style.display = showOrganize ? 'inline-flex' : 'none';
    }

    // 图片转码按钮：非搜索状态且当前层级存在可重编码的图片文件时显示
    const convertBtn = document.getElementById('image-convert-btn');
    if (convertBtn) {
        const hasConvertible = !isSearching && targetFiles.some(f => !f.isDir && isConvertibleImage(f.name));
        convertBtn.style.display = hasConvertible ? 'inline-flex' : 'none';
    }
}

/**
 * 更新面包屑导航
 * @param {string} currentPath - 当前路径
 * @param {boolean} isSearching - 是否在搜索
 * @param {Function} onNavigate - 导航回调函数
 */
function updateBreadcrumb(currentPath, isSearching = false, onNavigate) {
    const backButton = document.getElementById('back-button');
    backButton.style.display = currentPath ? 'inline-block' : 'none';

    const breadcrumb = document.querySelector('.breadcrumb');
    breadcrumb.innerHTML = '';
    breadcrumb.appendChild(backButton);

    // 根目录
    const rootItem = document.createElement('a');
    rootItem.className = 'breadcrumb-item';
    rootItem.href = '#';
    rootItem.dataset.path = '';
    rootItem.innerHTML = '<i class="fas fa-home"></i> 根目录';
    rootItem.addEventListener('click', (e) => {
        e.preventDefault();
        onNavigate('', true);
    });
    breadcrumb.appendChild(rootItem);

    if (isSearching) {
        const searchItem = document.createElement('span');
        searchItem.className = 'breadcrumb-item';
        searchItem.innerHTML = '<i class="fas fa-search"></i> 搜索结果';
        searchItem.style.color = '#3498db';
        searchItem.style.fontWeight = '600';
        breadcrumb.appendChild(searchItem);
    } else if (currentPath) {
        const pathParts = currentPath.split('/');
        let currentPathBuilder = '';
        pathParts.forEach(part => {
            if (!part) return;
            currentPathBuilder += (currentPathBuilder ? '/' : '') + part;
            const breadcrumbItem = document.createElement('a');
            breadcrumbItem.className = 'breadcrumb-item';
            breadcrumbItem.href = '#';
            breadcrumbItem.dataset.path = currentPathBuilder;
            breadcrumbItem.textContent = part;
            breadcrumbItem.addEventListener('click', (e) => {
                e.preventDefault();
                onNavigate(e.currentTarget.dataset.path, true);
            });
            breadcrumb.appendChild(breadcrumbItem);
        });
    }
}

/**
 * 更新批量操作按钮显示
 * @param {Set} selectedFiles - 选中的文件集合
 */
function updateBatchActions(selectedFiles) {
    const batchActions = document.querySelector('.batch-actions');
    // 有可操作项目（当前视图非空）或已有选中项时显示批量操作栏
    const hasVisibleItems = document.querySelectorAll('.file-card').length > 0;
    if (hasVisibleItems || selectedFiles.size > 0) {
        batchActions.classList.add('show');
    } else {
        batchActions.classList.remove('show');
    }
}

/**
 * 更新文件卡片选中状态
 * @param {Object} file - 文件对象
 * @param {boolean} isSelected - 是否选中
 */
function updateFileCardSelection(file, isSelected) {
    const cards = document.querySelectorAll('.file-card');
    for (const card of cards) {
        if (card.dataset.path === file.path) {
            if (isSelected) {
                card.classList.add('selected');
            } else {
                card.classList.remove('selected');
            }
            const checkbox = card.querySelector('.file-checkbox');
            if (checkbox) checkbox.checked = isSelected;
            break;
        }
    }
}

/**
 * 更新上传进度
 * @param {number} progress - 进度值 (0-100)
 * @param {boolean} show - 是否显示进度条
 */
function updateUploadProgress(progress, show) {
    const uploadProgress = document.getElementById('upload-progress');
    const progressFill = document.getElementById('progress-fill');
    const progressText = document.getElementById('progress-text');

    if (show) {
        uploadProgress.style.display = 'flex';
    } else {
        uploadProgress.style.display = 'none';
    }

    progressFill.style.width = `${progress}%`;
    progressText.textContent = `${Math.round(progress)}%`;
}
