/**
 * 工具函数模块
 * 提供文件管理器所需的各种辅助函数
 */

/**
 * 格式化文件大小
 * @param {number} bytes - 文件大小（字节）
 * @returns {string} - 格式化后的文件大小字符串
 */
function formatFileSize(bytes) {
    if (bytes === 0) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
}

/**
 * 格式化日期字符串
 * @param {string} dateString - 日期字符串（ISO 8601 格式）
 * @returns {string} - 格式化后的日期字符串
 */
function formatDate(dateString) {
    const date = new Date(dateString);
    return date.toLocaleString('zh-CN', {
        year: 'numeric',
        month: '2-digit',
        day: '2-digit',
        hour: '2-digit',
        minute: '2-digit'
    });
}

/**
 * 对文件名进行编码
 * @param {string} filename - 文件名
 * @returns {string} - 编码后的文件名
 */
function encodeFileName(filename) {
    const encodedParams = encodeURIComponent(filename);
    const decodedParams = encodedParams.replace(/%([0-9A-F]{2})/g, (_, p1) => String.fromCharCode(parseInt(p1, 16)));
    return btoa(decodedParams);
}

/**
 * 检查文件名是否有效
 * @param {string} filename - 文件名
 * @returns {boolean} - 是否有效
 */
function isValidFileName(filename) {
    const invalidChars = /[<>:/"\\|?*]/;
    return !invalidChars.test(filename) && filename.trim() !== '';
}

/**
 * 检查文件是否为图片文件
 * @param {string} filename - 文件名
 * @returns {boolean} - 是否为图片文件
 */
function isImageFile(filename) {
    const imageExtensions = ['.jpg', '.jpeg', '.png', '.gif', '.bmp', '.svg', '.webp'];
    const extension = filename.toLowerCase().slice(filename.lastIndexOf('.'));
    return imageExtensions.includes(extension);
}

/**
 * 检查文件是否为视频文件
 * @param {string} filename - 文件名
 * @returns {boolean} - 是否为视频文件
 */
function isVideoFile(filename) {
    const videoExtensions = ['.mp4', '.avi', '.mov', '.wmv', '.flv', '.webm', '.mkv'];
    const ext = filename.toLowerCase().slice(filename.lastIndexOf('.'));
    return videoExtensions.includes(ext);
}

/**
 * 检查文件是否为音频文件
 * @param {string} filename - 文件名
 * @returns {boolean} - 是否为音频文件
 */
function isAudioFile(filename) {
    const audioExtensions = ['.mp3', '.wav', '.ogg', '.flac', '.aac', '.m4a', '.wma'];
    const ext = filename.toLowerCase().slice(filename.lastIndexOf('.'));
    return audioExtensions.includes(ext);
}

/**
 * 检查文件是否为文本文件
 * @param {string} filename - 文件名
 * @returns {boolean} - 是否为文本文件
 */
function isTextFile(filename) {
    const plainText = ['.txt', '.md', '.log'];
    const web = ['.html', '.css', '.js', '.ts', '.jsx', '.tsx', '.vue'];
    const backend = ['.py', '.java', '.php', '.rb', '.go', '.rs', '.kt', '.scala', '.cs', '.swift'];
    const system = ['.c', '.cpp', '.cxx', '.h', '.hpp'];
    const data = ['.json', '.xml', '.csv', '.sql', '.yml', '.yaml'];
    const script = ['.sh', '.bat', '.ps1'];
    const config = ['.pem'];
    const textExtensions = [...plainText, ...web, ...backend, ...system, ...data, ...script, ...config];
    const ext = filename.toLowerCase().slice(filename.lastIndexOf('.'));
    return textExtensions.includes(ext);
}

/**
 * 获取文件图标
 * @param {string} filename - 文件名
 * @returns {string} - 文件图标 HTML 字符串
 */
function getFileIcon(filename) {
    const ext = filename.toLowerCase().slice(filename.lastIndexOf('.'));
    const iconMap = {
        '.txt': '<i class="fas fa-file-alt"></i>',
        '.md': '<i class="fab fa-markdown"></i>',
        '.json': '<i class="fas fa-file-code"></i>',
        '.html': '<i class="fab fa-html5"></i>',
        '.css': '<i class="fab fa-css3-alt"></i>',
        '.js': '<i class="fab fa-js"></i>',
        '.ts': '<i class="fab fa-js"></i>',
        '.pdf': '<i class="fas fa-file-pdf"></i>',
        '.doc': '<i class="fas fa-file-word"></i>',
        '.docx': '<i class="fas fa-file-word"></i>',
        '.xls': '<i class="fas fa-file-excel"></i>',
        '.xlsx': '<i class="fas fa-file-excel"></i>',
        '.ppt': '<i class="fas fa-file-powerpoint"></i>',
        '.pptx': '<i class="fas fa-file-powerpoint"></i>',
        '.zip': '<i class="fas fa-file-archive"></i>',
        '.rar': '<i class="fas fa-file-archive"></i>',
        '.7z': '<i class="fas fa-file-archive"></i>',
        '.mp3': '<i class="fas fa-file-audio"></i>',
        '.mp4': '<i class="fas fa-file-video"></i>',
    };
    return iconMap[ext] || '<i class="fas fa-file"></i>';
}

/**
 * 获取文件类型的 MIME 字符串
 * @param {string} extension - 文件扩展名
 * @returns {string} - 文件类型的 MIME 字符串
 */
function getFileType(extension) {
    const mimeTypes = {
        '.txt': 'text/plain',
        '.md': 'text/markdown',
        '.json': 'application/json',
        '.html': 'text/html',
        '.css': 'text/css',
        '.js': 'application/javascript',
        '.jpg': 'image/jpeg',
        '.jpeg': 'image/jpeg',
        '.png': 'image/png',
        '.gif': 'image/gif',
        '.svg': 'image/svg+xml',
        '.webp': 'image/webp',
        '.mp4': 'video/mp4',
        '.avi': 'video/x-msvideo',
        '.mov': 'video/quicktime',
        '.wmv': 'video/x-ms-wmv',
        '.flv': 'video/x-flv',
        '.webm': 'video/webm',
        '.pdf': 'application/pdf',
        '.zip': 'application/zip'
    };
    return mimeTypes[extension.toLowerCase()] || 'application/octet-stream';
}

/**
 * 显示 Toast 通知
 * @param {string} message - 消息内容
 * @param {string} type - 消息类型 ('success', 'error', 'info')
 */
function showToast(message, type = 'info') {
    const toast = document.getElementById('toast');
    toast.textContent = message;
    toast.className = `toast ${type} show`;
    setTimeout(() => {
        toast.classList.remove('show');
    }, 3000);
}

/**
 * 从视频 URL 获取视频缩略图 URL
 * @param {string} videoUrl - 视频文件访问地址
 * @returns {Promise<string>} 缩略图的 data URL
 */
async function getVideoThumbnailFromUrl(videoUrl) {
    return new Promise((resolve, reject) => {
        const video = document.createElement('video');
        video.preload = 'metadata';
        video.muted = true;
        video.crossOrigin = 'anonymous';
        video.onloadeddata = () => {
            video.currentTime = 1;
        };
        video.onseeked = () => {
            const canvas = document.createElement('canvas');
            canvas.width = video.videoWidth || 640;
            canvas.height = video.videoHeight || 360;
            const ctx = canvas.getContext('2d');
            if (ctx) {
                ctx.drawImage(video, 0, 0, canvas.width, canvas.height);
                resolve(canvas.toDataURL('image/jpeg'));
            } else {
                reject(new Error('Failed to get video context'));
            }
            URL.revokeObjectURL(video.src);
        };
        video.onerror = () => {
            URL.revokeObjectURL(video.src);
            reject(new Error('Failed to load video'));
        };
        video.src = videoUrl;
    });
}

/**
 * UI 渲染模块
 * 负责文件管理器的所有 UI 渲染功能
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
function createFileCard(file, selectedFiles, onToggleSelection, onFileClick, onRename, onDownload, onDelete) {
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
function updateFileGrid(files, selectedFiles, isSearching, searchResults, currentPage, pageSize, callbacks) {
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
function renderPagination(totalPages, currentPage, onPageChange) {
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
function updateBatchActions(selectedFiles) {
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

/**
 * 文件操作模块
 * 负责文件管理器的所有文件操作功能
 */


/**
 * 加载文件列表
 * @param {string} currentPath - 当前路径
 * @returns {Promise<Array>} - 文件列表
 */
async function loadFiles(currentPath) {
    try {
        const response = await fetch(`/file_list/${currentPath}`);
        if (!response.ok) throw new Error('加载文件失败');
        const files = await response.json();
        // 确保返回的是数组
        if (Array.isArray(files)) {
            return files;
        }
        return [];
    } catch (error) {
        showToast('加载文件失败', 'error');
        console.error('加载文件失败:', error);
        throw error;
    }
}

/**
 * 上传单个文件
 * @param {File} file - 要上传的文件对象
 * @param {string} currentPath - 当前路径
 * @param {Function} onProgress - 上传进度回调函数
 * @param {boolean} overwrite - 是否覆盖已存在文件
 * @returns {Promise} - 上传完成后的 Promise 对象
 */
async function uploadFile(file, currentPath, onProgress, overwrite = true) {
    return new Promise((resolve, reject) => {
        const formData = new FormData();
        formData.append('file', file);
        const fullPath = currentPath ? `${currentPath}/${file.name}` : file.name;

        const xhr = new XMLHttpRequest();

        xhr.upload.addEventListener('progress', e => {
            if (e.lengthComputable) {
                const progress = e.loaded / e.total;
                onProgress(progress);
            }
        });

        xhr.addEventListener('load', () => {
            if (xhr.status === 200) {
                resolve(JSON.parse(xhr.responseText));
            } else {
                reject(new Error(xhr.responseText || '上传失败'));
            }
        });

        xhr.addEventListener('error', () => reject(new Error('上传失败')));
        xhr.addEventListener('timeout', () => reject(new Error('上传超时')));

        xhr.open('POST', '/save');
        xhr.setRequestHeader('X-File-Name', encodeFileName(fullPath));
        xhr.setRequestHeader('X-Overwrite', overwrite.toString());
        xhr.send(file);
    });
}

/**
 * 处理文件上传
 * @param {FileList} files - 要上传的文件列表
 * @param {string} currentPath - 当前路径
 * @param {Function} onComplete - 上传完成回调
 */
async function handleFileUpload(files, currentPath, onComplete) {
    if (files.length === 0) return;

    updateUploadProgress(0, true);
    let uploadedFiles = 0;

    for (const file of files) {
        try {
            await uploadFile(file, currentPath, progress => {
                const totalProgress = ((uploadedFiles + progress) / files.length) * 100;
                updateUploadProgress(totalProgress, true);
            });
            uploadedFiles++;
        } catch (error) {
            showToast(`上传失败: ${file.name}`, 'error');
            console.error('上传失败:', error);
        }
    }

    updateUploadProgress(0, false);
    showToast(`成功上传 ${uploadedFiles} 个文件`, 'success');
    onComplete();
}

/**
 * 创建新文件夹
 * @param {string} currentPath - 当前路径
 * @param {Function} onComplete - 完成回调
 */
async function createNewFolder(currentPath, onComplete) {
    const folderName = prompt('请输入文件夹名称:');
    if (!folderName) return;

    if (!isValidFileName(folderName)) {
        showToast('文件夹名称不合法', 'error');
        return;
    }

    try {
        const tempFileName = `${folderName}/.temp`;
        const fullPath = currentPath ? `${currentPath}/${tempFileName}` : tempFileName;
        const blob = new Blob([''], { type: 'text/plain' });
        const file = new File([blob], tempFileName, { type: 'text/plain' });

        await uploadFile(file, currentPath, () => { }, true);
        await fetch(`/delete/${fullPath}`, { method: 'DELETE' });

        showToast(`文件夹 "${folderName}" 创建成功`, 'success');
        onComplete();
    } catch (error) {
        showToast('创建文件夹失败', 'error');
        console.error('创建文件夹失败:', error);
    }
}

/**
 * 删除文件或目录
 * @param {Object} file - 要删除的文件或目录对象
 * @param {Function} onComplete - 完成回调
 */
async function deleteFile(file, onComplete) {
    if (!confirm(`确定要删除 ${file.isDir ? '目录' : '文件'} "${file.name}" 吗？`)) {
        return;
    }

    try {
        const response = await fetch(`/delete/${file.path}`, { method: 'DELETE' });
        if (!response.ok) throw new Error('删除失败');

        showToast('删除成功', 'success');
        onComplete();
    } catch (error) {
        showToast('删除失败', 'error');
        console.error('删除失败:', error);
    }
}

/**
 * 重命名文件或目录
 * @param {Object} file - 要重命名的文件或目录对象
 * @param {string} currentPath - 当前路径
 * @param {Function} onComplete - 完成回调
 */
async function renameFile(file, currentPath, onComplete) {
    const newName = prompt(`请输入新的${file.isDir ? '目录' : '文件'}名称:`, file.name);
    if (!newName || newName === file.name) return;

    if (!isValidFileName(newName)) {
        showToast('名称不合法', 'error');
        return;
    }

    try {
        if (file.isDir) {
            await renameDirectory(file, newName, currentPath);
        } else {
            await renameSingleFile(file, newName, currentPath);
        }

        showToast('重命名成功', 'success');
        onComplete();
    } catch (error) {
        showToast(`重命名失败: ${error.message}`, 'error');
        console.error('重命名失败:', error);
    }
}

/**
 * 重命名单个文件
 * @param {Object} file - 要重命名的文件对象
 * @param {string} newName - 新的文件名
 * @param {string} currentPath - 当前路径
 * @returns {Promise} - 重命名操作后的 Promise 对象
 */
async function renameSingleFile(file, newName, currentPath) {
    const response = await fetch(`/read/${file.path}`);
    if (!response.ok) throw new Error('读取文件失败');
    const content = await response.blob();

    await fetch(`/delete/${file.path}`, { method: 'DELETE' });
    const newFile = new File([content], newName, { type: content.type });
    await uploadFile(newFile, currentPath, () => { }, true);
}

/**
 * 重命名目录
 * @param {Object} directory - 要重命名的目录对象
 * @param {string} newName - 新的目录名
 * @param {string} currentPath - 当前路径
 * @returns {Promise} - 重命名操作后的 Promise 对象
 */
async function renameDirectory(directory, newName, currentPath) {
    await createDirectory(newName, currentPath);
    await copyDirectoryContent(directory.path, newName, currentPath);
    await fetch(`/delete/${directory.path}`, { method: 'DELETE' });
}

/**
 * 获取目录内容
 * @param {string} dirPath - 目录路径
 * @returns {Promise<Array>} - 目录内容的 Promise 对象
 */
async function getDirectoryContent(dirPath) {
    const response = await fetch(`/file_list/${dirPath}`);
    if (!response.ok) throw new Error('读取目录失败');
    return await response.json();
}

/**
 * 创建目录
 * @param {string} dirName - 目录名
 * @param {string} currentPath - 当前路径
 * @returns {Promise} - 创建目录后的 Promise 对象
 */
async function createDirectory(dirName, currentPath) {
    const tempFileName = `${dirName}/.temp`;
    const blob = new Blob([''], { type: 'text/plain' });
    const file = new File([blob], tempFileName, { type: 'text/plain' });
    await uploadFile(file, currentPath, () => { }, true);
    const fullPath = currentPath ? `${currentPath}/${tempFileName}` : tempFileName;
    await fetch(`/delete/${fullPath}`, { method: 'DELETE' });
}

/**
 * 复制目录内容
 * @param {string} sourceDirPath - 源目录路径
 * @param {string} targetDirName - 目标目录名
 * @param {string} currentPath - 当前路径
 * @returns {Promise} - 复制目录内容后的 Promise 对象
 */
async function copyDirectoryContent(sourceDirPath, targetDirName, currentPath) {
    const filesInDir = await getDirectoryContent(sourceDirPath);
    for (const file of filesInDir) {
        const fileName = file.path.split('\\').pop();
        const targetPath = `${targetDirName}/${fileName}`;
        if (file.isDir) {
            await createDirectory(targetPath, currentPath);
            await copyDirectoryContent(file.path, targetPath, currentPath);
        } else {
            await copySingleFile(file, targetPath, currentPath);
        }
    }
}

/**
 * 复制单个文件
 * @param {Object} file - 要复制的文件对象
 * @param {string} targetPath - 目标文件路径
 * @param {string} currentPath - 当前路径
 * @returns {Promise} - 复制文件后的 Promise 对象
 */
async function copySingleFile(file, targetPath, currentPath) {
    const fileResponse = await fetch(`/read/${file.path}`);
    if (!fileResponse.ok) return;
    const fileBlob = await fileResponse.blob();
    const fileName = targetPath.split('/').pop();
    const newFile = new File([fileBlob], fileName, { type: fileBlob.type });
    const targetDir = targetPath.substring(0, targetPath.lastIndexOf('/'));
    await uploadFile(newFile, targetDir || currentPath, () => { }, false);
}

/**
 * 批量删除
 * @param {Set} selectedFiles - 选中的文件集合
 * @param {Function} onComplete - 完成回调
 */
async function batchDelete(selectedFiles, onComplete) {
    if (selectedFiles.size === 0) return;

    if (!confirm(`确定要删除选中的 ${selectedFiles.size} 个项目吗？`)) {
        return;
    }

    try {
        let deletedCount = 0;
        for (const filePath of selectedFiles) {
            const response = await fetch(`/delete/${filePath}`, { method: 'DELETE' });
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
 * 批量压缩
 * @param {Array} files - 所有文件列表
 * @param {Set} selectedFiles - 选中的文件集合
 */
async function batchCompress(files, selectedFiles) {
    if (selectedFiles.size === 0) {
        showToast('请先选择要压缩的文件', 'info');
        return;
    }

    try {
        const selectedFileObjects = files.filter(file => selectedFiles.has(file.path));
        const formData = new FormData();

        for (const fileObj of selectedFileObjects) {
            if (fileObj.isDir) continue;
            const response = await fetch(`/read/${fileObj.path}`);
            const blob = await response.blob();
            const file = new File([blob], fileObj.name, { type: blob.type });
            formData.append('files', file);
        }

        const zipName = `压缩文件_${new Date().getTime()}.zip`;
        formData.append('zip_name', zipName);

        const response = await fetch('/archive', {
            method: 'POST',
            body: formData
        });

        if (!response.ok) throw new Error('压缩失败');

        const blob = await response.blob();
        const url = URL.createObjectURL(blob);
        const a = document.createElement('a');
        a.href = url;
        a.download = zipName;
        document.body.appendChild(a);
        a.click();
        document.body.removeChild(a);
        URL.revokeObjectURL(url);

        showToast(`成功压缩 ${selectedFiles.size} 个项目`, 'success');
    } catch (error) {
        showToast('压缩失败', 'error');
        console.error('压缩失败:', error);
    }
}

/**
 * 处理 ZIP 文件上传解压
 * @param {File} file - 要上传的 ZIP 文件
 * @param {string} currentPath - 当前路径
 * @param {Function} onComplete - 完成回调
 */
async function handleZipUpload(file, currentPath, onComplete) {
    if (!file) return;

    try {
        const formData = new FormData();
        formData.append('zip_file', file);

        const response = await fetch('/archive', {
            method: 'PUT',
            body: formData
        });

        if (!response.ok) throw new Error('解压失败');

        const result = await response.json();

        for (const extractedFile of result.extracted_files) {
            if (extractedFile.is_dir) continue;
            const contentBytes = Uint8Array.from(atob(extractedFile.content), c => c.charCodeAt(0));
            const blob = new Blob([contentBytes]);
            const uploadFileObj = new File([blob], extractedFile.name, { type: getFileType(extractedFile.extension) });
            await uploadFile(uploadFileObj, currentPath, () => { }, false);
        }

        showToast(`成功解压 ${result.total_files} 个文件`, 'success');
        onComplete();
    }
     catch (error) {
        showToast('解压失败', 'error');
        console.error('解压失败:', error);
    }
}

/**
 * 下载文件
 * @param {Object} file - 要下载的文件对象
 */
async function downloadFile(file) {
    try {
        const response = await fetch(`/download/${file.path}`);
        if (!response.ok) throw new Error('下载失败');

        const blob = await response.blob();
        const url = URL.createObjectURL(blob);
        const a = document.createElement('a');
        a.href = url;
        a.download = file.name;
        document.body.appendChild(a);
        a.click();
        document.body.removeChild(a);
        URL.revokeObjectURL(url);
    } catch (error) {
        showToast('下载失败', 'error');
        console.error('下载失败:', error);
    }
}

/**
 * 遍历所有文件（用于搜索）
 * @param {string} startPath - 起始路径
 * @returns {Promise<Array>} - 所有文件列表
 */
async function traverseAllFiles(startPath = '') {
    const allFiles = [];
    const queue = [startPath];

    while (queue.length > 0) {
        const currentPath = queue.shift();
        try {
            const response = await fetch(`/file_list/${currentPath}`);
            if (!response.ok) continue;
            const files = await response.json();
            // 确保 files 是一个数组
            if (Array.isArray(files)) {
                allFiles.push(...files);
                const subDirs = files.filter(file => file.isDir);
                for (const dir of subDirs) {
                    queue.push(dir.path);
                }
            }
        } catch (error) {
            console.error('遍历文件失败:', error);
        }
    }

    return allFiles;
}

/**
 * 将搜索索引保存到根目录下的 file_query.index
 * @param {Array} indexData - 文件/目录列表
 * @returns {Promise<void>}
 */
async function saveIndexToFile(indexData) {
    try {
        const jsonStr = JSON.stringify(indexData);
        const blob = new Blob([jsonStr], { type: 'application/json' });
        const file = new File([blob], 'file_query.index', { type: 'application/json' });
        // 上传到根目录（currentPath 为空）
        await uploadFile(file, '', () => { }, true);
    } catch (error) {
        console.error('保存索引文件失败:', error);
    }
}

/**
 * 从根目录加载 file_query.index
 * @returns {Promise<Array|null>} 返回文件列表数组，若文件不存在或解析失败则返回 null
 */
async function loadIndexFromFile() {
    try {
        const response = await fetch('/read/file_query.index');
        if (!response.ok) return null;
        const data = await response.json();
        return Array.isArray(data) ? data : null;
    }
    catch (error) {
        console.warn('读取索引文件失败，将重新构建索引:', error);
        return null;
    }
}

/**
 * 模态框处理模块
 * 负责文件管理器的所有模态框相关功能
 */


/**
 * 当前编辑的文件
 * @type {Object|null}
 */
let currentEditFile = null;

/**
 * 显示文本模态框
 * @param {Object} file - 要预览的文件对象
 * @param {string} currentPath - 当前路径
 * @param {Function} onSave - 保存完成回调
 */
async function showTextModal(file, currentPath, onSave) {
    try {
        const response = await fetch(`/read/${file.path}`);
        if (!response.ok) throw new Error('读取文件失败');
        const content = await response.text();

        const modal = document.getElementById('text-modal');
        const modalTitle = document.getElementById('text-modal-title');
        const modalFileInfo = document.getElementById('text-modal-file-info');
        const modalContent = document.getElementById('text-modal-content');
        const modalEditor = document.getElementById('text-modal-editor');
        const modalSave = document.getElementById('text-modal-save');
        const modalEdit = document.getElementById('text-modal-edit');
        const modalCopy = document.getElementById('text-modal-copy');
        const modalDownload = document.getElementById('text-modal-download');

        modalTitle.textContent = file.name;
        const fileSize = formatFileSize(file.size);
        const lastModified = formatDate(file.lastModified);
        modalFileInfo.textContent = `${fileSize} · ${lastModified}`;
        modalEditor.value = content;

        // 显示预览，隐藏编辑器和保存按钮
        modalContent.style.display = 'block';
        modalEditor.style.display = 'none';
        modalSave.style.display = 'none';

        // 尝试对代码文件应用语法高亮
        try {
            if (isTextFile(file.name)) {
                const ext = file.name.toLowerCase().slice(file.name.lastIndexOf('.') + 1);
                const highlighted = hljs.highlight(content, { language: ext }).value;
                modalContent.innerHTML = highlighted;
                modalContent.className = 'hljs';
            } else {
                modalContent.textContent = content;
                modalContent.className = '';
            }
        } catch (error) {
            modalContent.textContent = content;
            modalContent.className = '';
        }

        // 显示模态框
        modal.classList.add('show');

        // 保存当前文件引用
        currentEditFile = file;

        // 编辑按钮点击事件
        modalEdit.onclick = () => {
            modalContent.style.display = 'none';
            modalEditor.style.display = 'block';
            modalSave.style.display = 'inline-block';
        };

        // 复制按钮点击事件
        modalCopy.onclick = () => {
            navigator.clipboard.writeText(content)
                .then(() => showToast('已复制到剪贴板', 'success'))
                .catch(() => showToast('复制失败', 'error'));
        };

        // 下载按钮点击事件
        modalDownload.onclick = () => {
            const link = document.createElement('a');
            link.href = `/download/${file.path}`;
            link.download = file.name;
            document.body.appendChild(link);
            link.click();
            document.body.removeChild(link);
        };

        // 保存按钮点击事件
        modalSave.onclick = async () => {
            try {
                const newContent = modalEditor.value;
                await saveTextFile(file, newContent, currentPath);

                // 保存成功后，更新预览内容
                modalContent.textContent = newContent;
                modalContent.style.display = 'block';
                modalEditor.style.display = 'none';
                modalSave.style.display = 'none';

                // 重新应用语法高亮
                try {
                    if (isTextFile(file.name)) {
                        const ext = file.name.toLowerCase().slice(file.name.lastIndexOf('.') + 1);
                        const highlighted = hljs.highlight(newContent, { language: ext }).value;
                        modalContent.innerHTML = highlighted;
                        modalContent.className = 'hljs';
                    }
                } catch (error) {
                    modalContent.textContent = newContent;
                    modalContent.className = '';
                }

                showToast('保存成功', 'success');
                onSave();
            } catch (error) {
                showToast('保存失败', 'error');
                console.error('保存失败:', error);
            }
        };
    } catch (error) {
        showToast('预览文件失败', 'error');
        console.error('预览文件失败:', error);
    }
}

/**
 * 关闭文本模态框
 * @param {Event} event - 点击事件对象
 */
function closeTextModal(event) {
    const modal = document.getElementById('text-modal');
    if (
        event.target === modal ||
        event.target.closest('.modal-close-btn') ||
        event.target.classList.contains('close')
    ) {
        modal.classList.remove('show');
        currentEditFile = null;
    }
}

/**
 * 保存文本文件
 * @param {Object} file - 要保存的文件对象
 * @param {string} content - 新的文件内容
 * @param {string} currentPath - 当前路径
 * @returns {Promise} - 保存完成后的 Promise 对象
 */
async function saveTextFile(file, content, currentPath) {
    try {
        const blob = new Blob([content], { type: 'text/plain' });
        const uploadFileObj = new File([blob], file.name, { type: 'text/plain' });
        await uploadFile(uploadFileObj, currentPath, () => {}, true);
    } catch (error) {
        throw new Error('保存文件失败');
    }
}

/**
 * 显示二维码模态框
 */
async function showQRCode() {
    try {
        const currentUrl = window.location.origin;
        const qrcodeContainer = document.getElementById('qrcode-container');
        const qrcodeUrl = document.getElementById('qrcode-url');
        const qrcodeModal = document.getElementById('qrcode-modal');

        // 清空二维码容器
        qrcodeContainer.innerHTML = '';

        // 生成二维码
        new QRCode(qrcodeContainer, {
            text: currentUrl,
            width: 256,
            height: 256,
            colorDark: '#000000',
            colorLight: '#ffffff',
            correctLevel: QRCode.CorrectLevel.H
        });

        // 显示URL
        qrcodeUrl.textContent = currentUrl;

        // 显示模态框
        qrcodeModal.classList.add('show');
    } catch (error) {
        showToast('生成二维码失败', 'error');
        console.error('生成二维码失败:', error);
    }
}

/**
 * 关闭二维码模态框
 * @param {Event} event - 点击事件对象
 */
function closeQRCodeModal(event) {
    const modal = document.getElementById('qrcode-modal');
    if (
        event.target === modal ||
        event.target.closest('.modal-close-btn') ||
        event.target.classList.contains('close')
    ) {
        modal.classList.remove('show');
    }
}

/**
 * 检查是否在媒体预览中
 * @returns {boolean} - 是否在媒体预览中
 */
function isInMediaPreview() {
    return !!document.querySelector('.image-preview-container');
}

/**
 * 处理键盘事件
 * @param {Event} event - 键盘事件对象
 * @param {Array} currentMediaList - 当前媒体列表
 * @param {number} currentMediaIndex - 当前媒体索引
 * @param {Function} onMediaChange - 媒体变化回调
 * @returns {number} - 更新后的媒体索引
 */
function handleKeyboardEvent(event, currentMediaList, currentMediaIndex, onMediaChange) {
    // 处理 ESC 键关闭模态框
    if (event.key === 'Escape') {
        const textModal = document.getElementById('text-modal');
        if (textModal.classList.contains('show')) {
            textModal.classList.remove('show');
            currentEditFile = null;
        }

        const qrcodeModal = document.getElementById('qrcode-modal');
        if (qrcodeModal.classList.contains('show')) {
            qrcodeModal.classList.remove('show');
        }

        return currentMediaIndex;
    }

    // 处理媒体预览的键盘导航
    if (isInMediaPreview() && currentMediaList.length > 0) {
        document.querySelector('.image-preview-container');
        const imageInfo = document.querySelector('.image-info');
        const imagePreview = document.querySelector('.image-preview');
        const videoPreview = document.querySelector('.video-preview');
        const imageDragContainer = document.querySelector('.image-drag-container');

        let newIndex = currentMediaIndex;
        let handled = false;

        switch (event.key) {
            case 'ArrowLeft':
                newIndex = (currentMediaIndex - 1 + currentMediaList.length) % currentMediaList.length;
                handled = true;
                break;
            case 'ArrowRight':
                newIndex = (currentMediaIndex + 1) % currentMediaList.length;
                handled = true;
                break;
        }

        if (handled && newIndex !== currentMediaIndex) {
            const file = currentMediaList[newIndex];
            const quicklyLoadMediaPreview = () => {
                if (file.name.toLowerCase().match(/\.(jpg|jpeg|png|gif|bmp|svg|webp)$/i)) {
                    imageDragContainer.style.display = 'block';
                    imagePreview.style.display = 'block';
                    videoPreview.style.display = 'none';
                    imagePreview.alt = file.name;
                    imagePreview.src = `/read/${file.path}`;
                } else {
                    imageDragContainer.style.display = 'none';
                    imagePreview.style.display = 'none';
                    videoPreview.style.display = 'block';
                    videoPreview.alt = file.name;
                    videoPreview.src = `/read/${file.path}`;
                }
                imageInfo.style.display = 'none';
            };

            quicklyLoadMediaPreview();
            onMediaChange(newIndex);
            return newIndex;
        }
    }

    return currentMediaIndex;
}

/**
 * 事件绑定模块
 * 负责文件管理器的所有事件绑定
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

    // ZIP 上传解压
    document.getElementById('zip-upload').addEventListener('change', async (e) => {
        await handleZipUpload(e.target.files[0], fileManager.currentPath, async () => {
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
        await batchCompress(fileManager.files, fileManager.selectedFiles);
    });

    // 返回按钮
    document.getElementById('back-button').addEventListener('click', () => {
        fileManager.goBack();
    });

    // 二维码按钮
    document.getElementById('qrcode-button').addEventListener('click', async () => {
        await showQRCode();
    });

    // 文本模态框点击关闭
    document.getElementById('text-modal').addEventListener('click', (e) => {
        closeTextModal(e);
    });

    // 二维码模态框点击关闭
    document.getElementById('qrcode-modal').addEventListener('click', (e) => {
        closeQRCodeModal(e);
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

/**
 * 文件管理器主类
 * 整合所有功能模块
 */


/**
 * 文件管理器类
 */
class FileManager {
    /**
     * 构造函数
     */
    constructor() {
        /** @type {string} 当前路径 */
        this.currentPath = '';
        /** @type {Set<string>} 选中的文件集合 */
        this.selectedFiles = new Set();
        /** @type {Array<Object>} 当前目录的文件列表 */
        this.files = [];
        /** @type {boolean} 是否在搜索中 */
        this.isSearching = false;
        /** @type {Array<Object>} 搜索结果 */
        this.searchResults = [];
        /** @type {Array<Object>} 所有文件（用于搜索） */
        this.allFiles = [];
        /** @type {number} 当前媒体索引 */
        this.currentMediaIndex = 0;
        /** @type {Array<Object>} 当前媒体列表 */
        this.currentMediaList = [];
        /** @type {number} 当前页码 */
        this.currentPage = 1;
        /** @type {number} 每页显示数量 */
        this.pageSize = 10;
    }

    /**
     * 初始化文件管理器
     */
    init() {
        bindEvents(this);
        this.loadFiles();
        this.loadIndexIfExists();
    }
    /** 尝试从本地索引文件加载 allFiles */
    async loadIndexIfExists() {
        try {
            const cached = await loadIndexFromFile();
            if (cached && Array.isArray(cached) && cached.length > 0) {
                this.allFiles = cached;
                showToast('✅ 已从本地索引文件加载' + cached.length + '条记录', 'info');
            }
        }
        // 忽略错误，搜索时再重建
        catch (error) { }
    }
    /**
     * 加载文件列表
     */
    async loadFiles() {
        try {
            this.files = await loadFiles(this.currentPath);
            if (!this.isSearching) {
                this.currentPage = 1;
                this.updateFileGrid();
                this.updateStats();
                this.updateBreadcrumb();
            }
        } catch (error) {
            showToast('加载文件失败', 'error');
        }
    }

    /**
     * 更新文件网格
     */
    updateFileGrid() {
        const displayFiles = this.isSearching ? this.searchResults : this.files;
        const sortedFiles = [...displayFiles].sort((a, b) => {
            if (a.isDir && !b.isDir) return -1;
            if (!a.isDir && b.isDir) return 1;
            return a.name.localeCompare(b.name);
        });

        // 媒体预览列表基于全部结果
        this.currentMediaList = displayFiles.filter(
            file => !file.isDir && (isImageFile(file.name) || isVideoFile(file.name) || isAudioFile(file.name))
        );

        // 分页计算
        const totalPages = Math.ceil(sortedFiles.length / this.pageSize);
        if (this.currentPage > totalPages && totalPages > 0) {
            this.currentPage = totalPages;
        }

        updateFileGrid(
            this.files,
            this.selectedFiles,
            this.isSearching,
            this.searchResults,
            this.currentPage,
            this.pageSize,
            {
                onToggleSelection: (file, isSelected) => this.toggleFileSelection(file, isSelected),
                onFileClick: (file) => this.handleFileClick(file),
                onRename: (file) => this.renameFile(file),
                onDownload: (file) => this.downloadFile(file),
                onDelete: (file) => this.deleteFile(file),
                onPageChange: (page) => this.handlePageChange(page)
            }
        );
    }

    /**
     * 处理页面变化
     * @param {number|string} page - 页码或操作
     */
    handlePageChange(page) {
        const displayFiles = this.isSearching ? this.searchResults : this.files;
        const totalPages = Math.ceil(displayFiles.length / this.pageSize);

        if (page === 'prev' && this.currentPage > 1) {
            this.currentPage--;
        } else if (page === 'next' && this.currentPage < totalPages) {
            this.currentPage++;
        } else if (typeof page === 'number') {
            this.currentPage = page;
        }

        this.updateFileGrid();
    }

    /**
     * 处理搜索
     * @param {Event} e - 事件对象
     */
    async handleSearch(e) {
        const query = e.target.value.trim();
        const searchClear = document.getElementById('search-clear');

        if (!query) {
            this.clearSearch();
            return;
        }

        searchClear.style.display = 'block';
        await this.searchFiles(query);
    }

    /**
     * 清除搜索
     */
    clearSearch() {
        const searchInput = document.getElementById('search-input');
        const searchClear = document.getElementById('search-clear');
        searchInput.value = '';
        searchClear.style.display = 'none';
        this.isSearching = false;
        this.searchResults = [];
        this.currentPage = 1;
        this.updateFileGrid();
        this.updateBreadcrumb();
    }

    /**
     * 搜索文件
     * @param {string} query - 搜索关键字
     */
    async searchFiles(query) {
        if (!query) {
            this.clearSearch();
            return;
        }

        this.isSearching = true;
        showToast('正在搜索...', 'info');

        try {
            if (this.allFiles.length === 0) {
                setTimeout(() => showToast('正在构建搜索索引，请稍后...', 'info'), 2000);
                this.allFiles = await traverseAllFiles();
                // 将索引保存到文件
                saveIndexToFile(this.allFiles);
            }

            this.searchResults = this.allFiles.filter(
                file => file.name.toLowerCase().includes(query.toLowerCase())
            );

            this.currentPage = 1;
            this.updateFileGrid();
            this.updateBreadcrumb(true);
            showToast(`找到 ${this.searchResults.length} 个结果`, 'success');
        } catch (error) {
            showToast('搜索失败', 'error');
            console.error('搜索失败:', error);
        }
    }

    /**
     * 处理文件点击
     * @param {Object} file - 文件对象
     */
    handleFileClick(file) {
        if (file.isDir) {
            this.navigateToDirectory(file);
        } else {
            if (isImageFile(file.name) || isVideoFile(file.name) || isAudioFile(file.name)) {
                const mediaIndex = this.currentMediaList.findIndex(media => media.path === file.path);
                this.currentMediaIndex = mediaIndex;
                previewImage(`/read/${file.path}`, file.name);
            } else if (isTextFile(file.name)) {
                showTextModal(file, this.currentPath, async () => {
                    await this.loadFiles();
                });
            }
        }
    }

    /**
     * 导航到目录
     * @param {Object} directory - 目录对象
     */
    navigateToDirectory(directory) {
        const normalizedPath = directory.path.replace(/\\/g, '/');
        this.currentPath = normalizedPath;
        this.selectedFiles.clear();
        this.updateBatchActions();
        this.currentPage = 1;
        this.loadFiles();
    }

    /**
     * 返回上一级
     */
    goBack() {
        if (!this.currentPath) return;

        const pathParts = this.currentPath.split('/');
        pathParts.pop();
        this.currentPath = pathParts.join('/');
        this.selectedFiles.clear();
        this.updateBatchActions();
        this.currentPage = 1;
        this.loadFiles();
    }

    /**
     * 更新面包屑导航
     * @param {boolean} isSearching - 是否在搜索中
     */
    updateBreadcrumb(isSearching = false) {
        updateBreadcrumb(this.currentPath, isSearching, (path, shouldClearSearch) => {
            if (shouldClearSearch) {
                this.clearSearch();
            }
            if (path !== undefined) {
                this.currentPath = path;
                this.selectedFiles.clear();
                this.updateBatchActions();
                this.currentPage = 1;
                this.loadFiles();
            }
        });
    }

    /**
     * 重命名文件
     * @param {Object} file - 文件对象
     */
    async renameFile(file) {
        await renameFile(file, this.currentPath, async () => {
            await this.loadFiles();
        });
    }

    /**
     * 删除文件
     * @param {Object} file - 文件对象
     */
    async deleteFile(file) {
        await deleteFile(file, async () => {
            await this.loadFiles();
        });
    }

    /**
     * 下载文件
     * @param {Object} file - 文件对象
     */
    async downloadFile(file) {
        await downloadFile(file);
    }

    /**
     * 切换文件选中状态
     * @param {Object} file - 文件对象
     * @param {boolean} isSelected - 是否选中
     */
    toggleFileSelection(file, isSelected) {
        if (isSelected) {
            this.selectedFiles.add(file.path);
        } else {
            this.selectedFiles.delete(file.path);
        }

        updateFileCardSelection(file, isSelected);
        this.updateBatchActions();
    }

    /**
     * 更新批量操作按钮
     */
    updateBatchActions() {
        updateBatchActions(this.selectedFiles);
    }

    /**
     * 更新统计信息
     */
    updateStats() {
        updateStats(this.files, this.isSearching, this.searchResults);
    }

    /**
     * 处理键盘事件
     * @param {Event} event - 键盘事件对象
     */
    handleKeyboard(event) {
        this.currentMediaIndex = handleKeyboardEvent(
            event,
            this.currentMediaList,
            this.currentMediaIndex,
            (newIndex) => {
                this.currentMediaIndex = newIndex;
            }
        );
    }
}

/**
 * 文件管理器入口脚本
 */


// 等待 DOM 加载完成
document.addEventListener('DOMContentLoaded', () => {
    // 初始化文件管理器
    const fileManager = new FileManager();
    fileManager.init();
});
