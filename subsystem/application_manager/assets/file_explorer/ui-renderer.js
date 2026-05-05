/**
 * UI 渲染模块
 * 负责文件管理器的所有 UI 渲染功能
 */

import { formatFileSize, formatDate, getFileIcon, isImageFile, isVideoFile, getVideoThumbnailFromUrl } from './utils.js';

/**
 * 更新统计信息
 * @param {Array} files - 文件列表
 * @param {boolean} isSearching - 是否在搜索
 * @param {Array} searchResults - 搜索结果
 */
export function updateStats(files, isSearching = false, searchResults = []) {
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
}

/**
 * 更新面包屑导航
 * @param {string} currentPath - 当前路径
 * @param {boolean} isSearching - 是否在搜索
 * @param {Function} onNavigate - 导航回调函数
 */
export function updateBreadcrumb(currentPath, isSearching = false, onNavigate) {
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
 * 创建文件卡片
 * @param {Object} file - 文件对象
 * @param {Set} selectedFiles - 选中的文件集合
 * @param {Function} onToggleSelection - 切换选中回调
 * @param {Function} onFileClick - 文件点击回调
 * @param {Function} onRename - 重命名回调
 * @param {Function} onDownload - 下载回调
 * @param {Function} onDelete - 删除回调
 * @returns {HTMLElement} - 文件卡片元素
 */
export function createFileCard(file, selectedFiles, onToggleSelection, onFileClick, onRename, onDownload, onDelete) {
    const card = document.createElement('div');
    card.className = 'file-card';
    card.dataset.path = file.path;

    // 复选框
    const checkbox = document.createElement('input');
    checkbox.type = 'checkbox';
    checkbox.className = 'file-checkbox';
    checkbox.checked = selectedFiles.has(file.path);
    checkbox.addEventListener('change', (e) => {
        e.stopPropagation();
        onToggleSelection(file, checkbox.checked);
    });
    card.appendChild(checkbox);

    // 文件图标/缩略图
    if (file.isDir) {
        const icon = document.createElement('div');
        icon.className = 'file-icon';
        icon.innerHTML = '<i class="fas fa-folder"></i>';
        card.appendChild(icon);
    }
    else if (isImageFile(file.name)) {
        const img = document.createElement('img');
        img.className = 'file-thumbnail';
        img.src = `/read/${file.path}`;
        img.alt = file.name;
        card.appendChild(img);
    }
    else if (isVideoFile(file.name)) {
        const img = document.createElement('img');
        img.className = 'file-thumbnail';
        img.alt = file.name;
        // 异步获取视频第一帧
        getVideoThumbnailFromUrl(`/read/${file.path}`)
            .then(thumbnailUrl => {
                img.src = thumbnailUrl;
            })
            .catch(() => {
                // 获取失败时回退为默认图标
                card.removeChild(img);
                const icon = document.createElement('div');
                icon.className = 'file-icon';
                icon.innerHTML = '<i class="fas fa-file-video"></i>';
                card.insertBefore(icon, card.querySelector('.file-name'));
            });
        card.appendChild(img);
    }
    else {
        const icon = document.createElement('div');
        icon.className = 'file-icon';
        icon.innerHTML = getFileIcon(file.name);
        card.appendChild(icon);
    }

    // 文件名
    const name = document.createElement('div');
    name.className = 'file-name';
    name.textContent = file.name;
    card.appendChild(name);

    // 文件元信息
    const meta = document.createElement('div');
    meta.className = 'file-meta';
    meta.innerHTML = `
        <div>大小: ${formatFileSize(file.size)}</div>
        <div>修改: ${formatDate(file.lastModified)}</div>
    `;
    card.appendChild(meta);

    // 操作按钮
    const actions = document.createElement('div');
    actions.className = 'file-actions';

    const renameBtn = document.createElement('button');
    renameBtn.className = 'btn btn-small btn-info';
    renameBtn.innerHTML = '<i class="fas fa-edit"></i>';
    renameBtn.title = '重命名';
    renameBtn.addEventListener('click', (e) => {
        e.stopPropagation();
        onRename(file);
    });
    actions.appendChild(renameBtn);

    if (!file.isDir) {
        const downloadBtn = document.createElement('button');
        downloadBtn.className = 'btn btn-small btn-secondary';
        downloadBtn.innerHTML = '<i class="fas fa-download"></i>';
        downloadBtn.title = '下载';
        downloadBtn.addEventListener('click', (e) => {
            e.stopPropagation();
            onDownload(file);
        });
        actions.appendChild(downloadBtn);
    }

    const deleteBtn = document.createElement('button');
    deleteBtn.className = 'btn btn-small btn-danger';
    deleteBtn.innerHTML = '<i class="fas fa-trash"></i>';
    deleteBtn.title = '删除';
    deleteBtn.addEventListener('click', (e) => {
        e.stopPropagation();
        onDelete(file);
    });
    actions.appendChild(deleteBtn);

    card.appendChild(actions);

    // 点击事件
    card.addEventListener('click', (e) => {
        if (!e.target.closest('.file-checkbox')) onFileClick(file);
    });

    return card;
}

/**
 * 更新文件网格
 * @param {Array} files - 文件列表
 * @param {Set} selectedFiles - 选中的文件集合
 * @param {boolean} isSearching - 是否在搜索
 * @param {Array} searchResults - 搜索结果
 * @param {number} currentPage - 当前页码
 * @param {number} pageSize - 每页数量
 * @param {Object} callbacks - 回调函数对象
 */
export function updateFileGrid(files, selectedFiles, isSearching, searchResults, currentPage, pageSize, callbacks) {
    const fileGrid = document.getElementById('file-grid');
    fileGrid.innerHTML = '';

    const displayFiles = isSearching ? searchResults : files;
    const sortedFiles = [...displayFiles].sort((a, b) => {
        if (a.isDir && !b.isDir) return -1;
        if (!a.isDir && b.isDir) return 1;
        return a.name.localeCompare(b.name);
    });

    // 媒体预览列表基于全部结果
    // （注意：这里我们不在这里设置 currentMediaList，因为这应该在 FileManager 中管理

    // 分页计算
    const totalPages = Math.ceil(sortedFiles.length / pageSize);
    if (currentPage > totalPages && totalPages > 0) currentPage = totalPages;
    const start = (currentPage - 1) * pageSize;
    const pageFiles = sortedFiles.slice(start, start + pageSize);

    // 渲染当前页
    pageFiles.forEach((file, index) => {
        const fileCard = createFileCard(
            file,
            selectedFiles,
            callbacks.onToggleSelection,
            callbacks.onFileClick,
            callbacks.onRename,
            callbacks.onDownload,
            callbacks.onDelete
        );
        fileCard.style.animationDelay = `${index * 0.04}s`;
        fileGrid.appendChild(fileCard);
    });

    renderPagination(totalPages, currentPage, callbacks.onPageChange);
}

/**
 * 渲染分页导航
 * @param {number} totalPages - 总页数
 * @param {number} currentPage - 当前页码
 * @param {Function} onPageChange - 页码变化回调
 */
export function renderPagination(totalPages, currentPage, onPageChange) {
    const pagination = document.getElementById('pagination');
    pagination.innerHTML = '';

    if (totalPages <= 1) {
        return;
    }

    // 上一页按钮
    const prevBtn = document.createElement('button');
    prevBtn.className = 'btn btn-small btn-pagination';
    prevBtn.dataset.page = 'prev';
    prevBtn.disabled = currentPage === 1;
    prevBtn.innerHTML = '‹ 上一页';
    prevBtn.addEventListener('click', () => onPageChange('prev'));
    pagination.appendChild(prevBtn);

    // 页码按钮
    const maxPagesToShow = 7;
    let startPage, endPage;
    if (totalPages <= maxPagesToShow) {
        startPage = 1;
        endPage = totalPages;
    } else {
        if (currentPage <= 4) {
            startPage = 1;
            endPage = maxPagesToShow;
        } else if (currentPage + 3 >= totalPages) {
            startPage = totalPages - maxPagesToShow + 1;
            endPage = totalPages;
        } else {
            startPage = currentPage - 3;
            endPage = currentPage + 3;
        }
    }

    for (let i = startPage; i <= endPage; i++) {
        const pageBtn = document.createElement('button');
        pageBtn.className = `btn btn-small btn-pagination ${i === currentPage ? 'btn-primary' : ''}`;
        pageBtn.dataset.page = i;
        pageBtn.textContent = i;
        pageBtn.addEventListener('click', () => onPageChange(i));
        pagination.appendChild(pageBtn);
    }

    // 下一页按钮
    const nextBtn = document.createElement('button');
    nextBtn.className = 'btn btn-small btn-pagination';
    nextBtn.dataset.page = 'next';
    nextBtn.disabled = currentPage === totalPages;
    nextBtn.innerHTML = '下一页 ›';
    nextBtn.addEventListener('click', () => onPageChange('next'));
    pagination.appendChild(nextBtn);
}

/**
 * 更新批量操作按钮显示
 * @param {Set} selectedFiles - 选中的文件集合
 */
export function updateBatchActions(selectedFiles) {
    const batchActions = document.querySelector('.batch-actions');
    if (selectedFiles.size > 0) {
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
export function updateFileCardSelection(file, isSelected) {
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
export function updateUploadProgress(progress, show) {
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
