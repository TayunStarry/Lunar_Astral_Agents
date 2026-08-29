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
 * 可重编码的图片格式扩展名列表（png / jpg / jpeg / webp）
 */
const CONVERTIBLE_IMAGE_EXTS = ['.png', '.jpg', '.jpeg', '.webp'];

/**
 * 检查文件是否为可重编码的图片文件
 * @param {string} filename - 文件名
 * @returns {boolean} - 是否可重编码
 */
function isConvertibleImage(filename) {
    const ext = filename.toLowerCase().slice(filename.lastIndexOf('.'));
    return CONVERTIBLE_IMAGE_EXTS.includes(ext);
}

/**
 * 检查文件是否为 GGUF 模型文件
 * @param {string} filename - 文件名
 * @returns {boolean} - 是否为 GGUF 文件
 */
function isGGUFFile(filename) {
    const ext = filename.toLowerCase().slice(filename.lastIndexOf('.'));
    return ext === '.gguf';
}

/**
 * 检查文件是否为 ZIP 压缩包
 * @param {string} filename - 文件名
 * @returns {boolean} - 是否为 ZIP 文件
 */
function isZipFile(filename) {
    const ext = filename.toLowerCase().slice(filename.lastIndexOf('.'));
    return ext === '.zip';
}

/**
 * HTML 转义（用于安全渲染后端返回的元数据文本）
 * @param {string} str - 原始文本
 * @returns {string} - 转义后的文本
 */
function mediaEscapeHTML(str) {
    const div = document.createElement('div');
    div.textContent = str;
    return div.innerHTML;
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
    card.draggable = true;

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
        img.src = `/file/read/${file.path}`;
        img.alt = file.name;
        card.appendChild(img);
    }
    else if (isVideoFile(file.name)) {
        const img = document.createElement('img');
        img.className = 'file-thumbnail';
        img.alt = file.name;
        // 异步获取视频第一帧
        getVideoThumbnailFromUrl(`/file/read/${file.path}`)
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
    pageFiles.forEach((file) => {
        const fileCard = createFileCard(
            file,
            selectedFiles,
            callbacks.onToggleSelection,
            callbacks.onFileClick,
            callbacks.onRename,
            callbacks.onDownload,
            callbacks.onDelete
        );
        fileGrid.appendChild(fileCard);
    });

    renderPagination(totalPages, currentPage, callbacks.onPageChange);
    updateBatchActions(selectedFiles);
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
        const response = await fetch(`/file/list/${currentPath}`);
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

        xhr.open('POST', '/file/write');
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
    const folderName = await showPromptModal(
        '新建文件夹',
        '请输入文件夹名称',
        '',
        '支持中英文、数字、下划线、连字符',
        (value) => {
            if (!value) return '文件夹名称不能为空';
            if (!isValidFileName(value)) return '文件夹名称包含非法字符（< > : / " \\ | ? *）';
            return null;
        }
    );
    if (!folderName) return;

    try {
        const tempFileName = `${folderName}/.temp`;
        const fullPath = currentPath ? `${currentPath}/${tempFileName}` : tempFileName;
        const blob = new Blob([''], { type: 'text/plain' });
        const file = new File([blob], tempFileName, { type: 'text/plain' });

        await uploadFile(file, currentPath, () => { }, true);
        await fetch(`/file/delete/${fullPath}`, { method: 'DELETE' });

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
    const confirmed = await showConfirmModal(
        '确认删除',
        `确定要删除 ${file.isDir ? '目录' : '文件'} 「${file.name}」吗？\n此操作不可撤销，请谨慎操作。`,
        'danger'
    );
    if (!confirmed) return;

    try {
        const response = await fetch(`/file/delete/${file.path}`, { method: 'DELETE' });
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
    const newName = await showPromptModal(
        `重命名${file.isDir ? '目录' : '文件'}`,
        `请输入新的${file.isDir ? '目录' : '文件'}名称`,
        file.name,
        '支持中英文、数字、下划线、连字符',
        (value) => {
            if (!value) return '名称不能为空';
            if (!isValidFileName(value)) return '名称包含非法字符（< > : / " \\ | ? *）';
            return null;
        }
    );
    if (!newName || newName === file.name) return;

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
    const response = await fetch(`/file/read/${file.path}`);
    if (!response.ok) throw new Error('读取文件失败');
    const content = await response.blob();

    await fetch(`/file/delete/${file.path}`, { method: 'DELETE' });
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
    await fetch(`/file/delete/${directory.path}`, { method: 'DELETE' });
}

/**
 * 获取目录内容
 * @param {string} dirPath - 目录路径
 * @returns {Promise<Array>} - 目录内容的 Promise 对象
 */
async function getDirectoryContent(dirPath) {
    const response = await fetch(`/file/list/${dirPath}`);
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
    await fetch(`/file/delete/${fullPath}`, { method: 'DELETE' });
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
    const fileResponse = await fetch(`/file/read/${file.path}`);
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

/**
 * 下载文件
 * @param {Object} file - 要下载的文件对象
 */
async function downloadFile(file) {
    try {
        const response = await fetch(`/file/download/${file.path}`);
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
            const response = await fetch(`/file/list/${currentPath}`);
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

/**
 * 智能整理模块
 * 当当前层级文件数 ≥ ORGANIZE_THRESHOLD 时提供「智能整理」按钮，
 * 通过多模态 AI 识别文件并生成 移动/重命名 方案后自动执行。
 */


/** 智能整理触发阈值（当前层级文件数达到该值显示按钮） */
const ORGANIZE_THRESHOLD = 50;
/** 整理 AI 模型名（与全项目约定一致，硬编码） */
const ORGANIZE_MODEL = 'system-multimodal';
/** 文本内容取样长度（开头 / 结尾各 2000 字） */
const TEXT_SAMPLE_LEN = 2000;

/** 整理文件类别映射 */
const ORGANIZE_CATEGORIES = {
    text: ['.txt', '.md', '.log', '.csv', '.json', '.xml', '.yaml', '.yml', '.toml', '.ini', '.cfg', '.conf', '.html', '.css', '.js', '.ts', '.jsx', '.tsx', '.vue', '.go', '.py', '.java', '.c', '.cpp', '.h', '.hpp', '.rs', '.rb', '.sh', '.bat', '.ps1', '.sql', '.pem'],
    image: ['.png', '.jpg', '.jpeg', '.webp', '.gif', '.svg', '.bmp', '.ico', '.tiff', '.tif', '.avif']
};

/** 智能整理是否进行中 */
let isOrganizing = false;

/**
 * 获取文件的整理类别
 * @param {string} name - 文件名
 * @returns {string} text / image / other
 */
function getOrganizeCategory(name) {
    const ext = name.slice(name.lastIndexOf('.')).toLowerCase();
    for (const [cat, exts] of Object.entries(ORGANIZE_CATEGORIES)) {
        if (exts.includes(ext)) return cat;
    }
    return 'other';
}

/**
 * 读取文本文件内容样本（开头 2000 字 + 结尾 2000 字，不足 4000 字取全部）
 * @param {string} relativePath - 相对路径
 * @returns {Promise<string>}
 */
async function readTextSample(relativePath) {
    const response = await fetch(`/file/read/${relativePath}`);
    if (!response.ok) throw new Error('读取文本失败');
    const text = await response.text();
    if (text.length <= TEXT_SAMPLE_LEN * 2) return text;
    return text.substring(0, TEXT_SAMPLE_LEN) +
        '\n\n......[中间内容省略]......\n\n' +
        text.substring(text.length - TEXT_SAMPLE_LEN);
}

/**
 * 通过 /resize 处理图片，返回去除 data URI 前缀的 base64
 * @param {string} relativePath - 图片相对路径
 * @returns {Promise<string>}
 */
async function resizeImageData(relativePath) {
    // 先从服务器获取图片 Blob（/resize 需要服务端来源的数据）
    const fetchResponse = await fetch(`/file/read/${relativePath}`);
    if (!fetchResponse.ok) throw new Error('获取图片失败');
    const blob = await fetchResponse.blob();

    const formData = new FormData();
    formData.append('image', blob);
    const resizeResponse = await fetch('/resize', { method: 'POST', body: formData });
    if (!resizeResponse.ok) throw new Error('图片缩放失败');

    // /resize 返回帧数组，每帧含带 data URI 前缀的 base64，取第一帧
    const resizeData = await resizeResponse.json();
    const frames = Array.isArray(resizeData) ? resizeData : (resizeData.frames || []);
    const frame = frames[0] || {};
    let base64 = frame.base64 || resizeData.base64 || '';
    base64 = base64.replace(/^data:image\/\w+;base64,/, '');
    return base64;
}

/**
 * 预处理单个文件，构造提交给 AI 的文件描述
 * @param {Object} file - 文件对象
 * @returns {Promise<Object>}
 */
async function preprocessOrganizeFile(file) {
    const category = getOrganizeCategory(file.name);
    const meta = {
        name: file.name,
        size: formatFileSize(file.size),
        ext: file.name.slice(file.name.lastIndexOf('.')).toLowerCase(),
        category: category
    };

    switch (category) {
        case 'image': {
            try {
                const base64 = await resizeImageData(file.path);
                return { ...meta, type: 'image', base64 };
            } catch (err) {
                return { ...meta, type: 'meta', note: `图片预处理失败: ${err.message}` };
            }
        }
        case 'text': {
            try {
                const content = await readTextSample(file.path);
                return { ...meta, type: 'text', content };
            } catch (err) {
                return { ...meta, type: 'meta', note: `文本读取失败: ${err.message}` };
            }
        }
        default:
            return { ...meta, type: 'meta' };
    }
}

/**
 * 调用多模态 AI 接口
 * @param {Array<Object>} messages - 消息数组
 * @returns {Promise<string>} AI 返回的文本内容
 */
async function callOrganizeAI(messages) {
    const response = await fetch('/v1/chat/completions', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
            model: ORGANIZE_MODEL,
            messages: messages,
            stream: false
        })
    });
    if (!response.ok) throw new Error(`AI 调用失败: ${response.statusText}`);
    const data = await response.json();
    // OpenAI 兼容响应
    if (data.choices && data.choices[0]) {
        return data.choices[0].message.content;
    }
    // 代理包装响应
    if (data.success && data.data && data.data.choices) {
        return data.data.choices[0].message.content;
    }
    throw new Error('AI 响应格式异常');
}

/**
 * 执行整理操作（/file/organize）
 * @param {string} basePath - 工作目录基础路径（相对 LocalDir）
 * @param {Array<Object>} operations - 操作列表
 * @returns {Promise<Object>}
 */
async function executeOrganizeOperations(basePath, operations) {
    const response = await fetch('/file/organize', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ base_path: basePath, operations: operations })
    });
    if (!response.ok) throw new Error('执行整理操作失败');
    return await response.json();
}

/**
 * 更新整理进度条与状态文字
 * @param {number} percent - 进度百分比
 * @param {string} status - 状态文字
 */
function setOrganizeProgress(percent, status) {
    document.getElementById('organize-progress-fill').style.width = `${percent}%`;
    document.getElementById('organize-status').textContent = status;
}

/**
 * 追加整理日志
 * @param {string} message - 日志内容
 * @param {string} type - 日志类型: info / success / error / warning
 */
function addOrganizeLog(message, type = 'info') {
    const log = document.getElementById('organize-log');
    const item = document.createElement('div');
    item.className = `organize-log-item ${type}`;
    const icons = {
        success: 'fa-check-circle',
        error: 'fa-exclamation-circle',
        info: 'fa-circle-info',
        warning: 'fa-exclamation-triangle'
    };
    item.innerHTML = `<i class="fas ${icons[type] || icons.info}"></i><span>${message}</span>`;
    log.appendChild(item);
    log.scrollTop = log.scrollHeight;
}

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

/**
 * 逐文件 AI 决策与分布审核辅助函数
 * 每个文件独立询问 AI（携带当前已知文件夹列表，含之前文件拟新增的文件夹），
 * 全部处理完成后再次调用 AI 审核文件夹分布并改写不合理的移动操作
 */

/**
 * 询问 AI 对单个文件的处理决定
 * @param {Object} file - 预处理后的文件描述
 * @param {Array<string>} knownFolders - 当前已知文件夹列表（含拟新增的）
 * @returns {Promise<{rename_to: string|null, target_folder: string|null}>}
 */
async function askFileDecision(file, knownFolders) {
    const messages = buildFileDecisionMessages(file, knownFolders);
    const response = await callOrganizeAI(messages);
    return parseOrganizeDecision(response);
}

/**
 * 组装单文件决策的 AI 消息（多模态内容数组，图片以 image_url 提交）
 * @param {Object} file - 预处理后的文件描述
 * @param {Array<string>} knownFolders - 当前已知文件夹列表
 * @returns {Array<Object>} 消息数组
 */
function buildFileDecisionMessages(file, knownFolders) {
    const system = [
        '你是文件整理助手。当前正在进行逐个文件的智能整理，你会依次看到每个待整理的文件。',
        '对当前这个文件，你需要决定：',
        '1. rename_to：是否重命名（保持扩展名不变；无需重命名时为 null）；',
        '2. target_folder：放到哪个文件夹（从「当前已知文件夹」中选择，或新建一个语义清晰的新文件夹名；留在当前目录时为 null）。',
        '规则：',
        '- 优先复用「当前已知文件夹」，语义匹配时不要重复新建同义文件夹；',
        '- 新建的文件夹名会加入「当前已知文件夹」，供后续文件复用；',
        '- 只返回 JSON 对象，不要输出其他内容，格式：{"rename_to":"新文件名或null","target_folder":"文件夹名或null"}'
    ].join('\n');

    const userParts = [
        { type: 'text', text: `当前已知文件夹列表：${knownFolders.length ? knownFolders.join('、') : '（无，可新建）'}\n请决定下列文件的整理方式：` }
    ];
    if (file.type === 'image' && file.base64) {
        userParts.push({ type: 'text', text: `文件：${file.name}（${file.size}，${file.ext}）` });
        userParts.push({ type: 'image_url', image_url: { url: `data:image/jpeg;base64,${file.base64}` } });
    } else if (file.type === 'text' && file.content) {
        userParts.push({ type: 'text', text: `文件：${file.name}（${file.size}，${file.ext}）\n内容样本：\n${file.content}` });
    } else {
        const note = file.note ? ` — ${file.note}` : '';
        userParts.push({ type: 'text', text: `文件：${file.name}（${file.size}，${file.ext}）${note}` });
    }

    return [
        { role: 'system', content: system },
        { role: 'user', content: userParts }
    ];
}

/**
 * 解析 AI 返回的单文件决策 JSON（兼容 ```json 代码块包裹）
 * @param {string} response - AI 响应文本
 * @returns {{rename_to: string|null, target_folder: string|null}}
 */
function parseOrganizeDecision(response) {
    const cleaned = response.replace(/```json\s*|```\s*/g, '').trim();
    const match = cleaned.match(/\{[\s\S]*\}/);
    const obj = match ? JSON.parse(match[0]) : JSON.parse(cleaned);
    return {
        rename_to: obj.rename_to != null ? String(obj.rename_to).trim() : null,
        target_folder: obj.target_folder != null ? String(obj.target_folder).trim() : null
    };
}

/**
 * 根据 AI 决策生成整理操作（move / rename），并将拟新增文件夹加入 knownFolders
 * @param {string} fileName - 原文件名
 * @param {{rename_to: string|null, target_folder: string|null}} decision - AI 决策
 * @param {Array<string>} knownFolders - 当前已知文件夹列表（会被追加新增文件夹）
 * @returns {Object|null} 操作对象或 null（保持不动）
 */
function buildFileOperation(fileName, decision, knownFolders) {
    const dotIdx = fileName.lastIndexOf('.');
    const ext = dotIdx >= 0 ? fileName.slice(dotIdx) : '';

    // 重命名（保持扩展名不变）
    let finalName = fileName;
    let renamed = false;
    if (decision.rename_to && decision.rename_to !== fileName) {
        let newName = decision.rename_to;
        if (ext && !newName.toLowerCase().endsWith(ext.toLowerCase())) {
            newName += ext;
        }
        if (newName && newName !== fileName) {
            finalName = newName;
            renamed = true;
        }
    }

    // 目标文件夹（去除首尾斜杠）
    const folder = decision.target_folder
        ? decision.target_folder.replace(/^\/+|\/+$/g, '').replace(/\\/g, '/')
        : '';

    if (folder && folder !== '.') {
        // 拟新增的文件夹加入已知集合，供后续文件复用
        if (!knownFolders.includes(folder)) {
            knownFolders.push(folder);
        }
        const target = `${folder}/${finalName}`;
        if (target !== fileName) {
            return { type: 'move', source: fileName, target: target };
        }
        return null;
    }

    if (renamed) {
        return { type: 'rename', source: fileName, target: finalName };
    }
    return null;
}

/**
 * 汇总整理后拟形成的文件夹分布（各文件夹将放入的文件数量与类型）
 * @param {Array<Object>} operations - 操作列表
 * @param {Array<string>} knownFolders - 已知文件夹列表
 * @returns {string} 分布描述文本
 */
function summarizeFolderDistribution(operations, knownFolders) {
    const stats = {};
    for (const folder of knownFolders) {
        stats[folder] = { count: 0, types: new Set() };
    }
    let rootCount = 0;
    const rootTypes = new Set();

    for (const op of operations) {
        if (!op || op.type !== 'move' || !op.target) continue;
        const target = String(op.target).replace(/\\/g, '/');
        const idx = target.lastIndexOf('/');
        const folder = idx >= 0 ? target.slice(0, idx) : '';
        const fileExt = idx >= 0 ? target.slice(idx + 1) : target;
        const ext = fileExt.slice(fileExt.lastIndexOf('.') + 1).toLowerCase() || '无';
        if (folder && stats[folder]) {
            stats[folder].count++;
            stats[folder].types.add(ext);
        } else {
            rootCount++;
            rootTypes.add(ext);
        }
    }

    const lines = ['本次整理后拟形成的文件夹分布：'];
    for (const folder of knownFolders) {
        if (stats[folder].count > 0) {
            lines.push(`- ${folder}：${stats[folder].count} 个文件（${Array.from(stats[folder].types).join('、')}）`);
        }
    }
    if (rootCount > 0) {
        lines.push(`- （当前目录，保持不动）：${rootCount} 个文件（${Array.from(rootTypes).join('、')}）`);
    }
    if (lines.length === 1) {
        lines.push('- （无文件被移动到文件夹）');
    }
    return lines.join('\n');
}

/**
 * 调用 AI 审核文件夹分布，返回需修正的合并/调整建议
 * @param {string} distributionText - 分布描述文本
 * @returns {Promise<Array<{from: string, to: string, reason: string}>>}
 */
async function auditFolderDistribution(distributionText) {
    const system = [
        '你是文件整理审核助手。以下是本次整理后拟形成的文件夹分布，请检查是否存在分布不合理：',
        '1. 语义重复的文件夹（同义、近义或可合并）→ 应合并；',
        '2. 功能不合适的文件夹（命名混乱、粒度不当）→ 应调整；',
        '3. 层级不合理的文件夹 → 应调整。',
        '只返回 JSON 数组，不要输出其他内容，格式：[{"from":"被合并/被调整的文件夹名","to":"保留的目标文件夹名","reason":"原因"}]',
        '没有需要修正的问题时返回 []'
    ].join('\n');

    const messages = [
        { role: 'system', content: system },
        { role: 'user', content: distributionText }
    ];
    const response = await callOrganizeAI(messages);
    return parseFolderCorrections(response);
}

/**
 * 解析 AI 返回的修正建议 JSON 数组（兼容 ```json 代码块包裹）
 * @param {string} response - AI 响应文本
 * @returns {Array<{from: string, to: string, reason: string}>}
 */
function parseFolderCorrections(response) {
    const cleaned = response.replace(/```json\s*|```\s*/g, '').trim();
    const match = cleaned.match(/\[[\s\S]*\]/);
    const arr = match ? JSON.parse(match[0]) : JSON.parse(cleaned);
    return (Array.isArray(arr) ? arr : []).filter(c => {
        return c && c.from && c.to && String(c.from).trim() !== String(c.to).trim();
    }).map(c => ({
        from: String(c.from).trim(),
        to: String(c.to).trim(),
        reason: c.reason ? String(c.reason) : ''
    }));
}

/**
 * 应用修正建议：将操作中 target 以 from 开头的路径替换为 to
 * @param {Array<Object>} operations - 操作列表
 * @param {Array<{from: string, to: string}>} corrections - 修正建议
 * @returns {{operations: Array<Object>, adjustCount: number}}
 */
function applyFolderCorrections(operations, corrections) {
    let adjustCount = 0;
    const adjustedOps = operations.map(op => {
        if (!op || !op.target) return op;
        const rawTarget = String(op.target).replace(/\\/g, '/');
        let newTarget = rawTarget;
        let changed = false;
        for (const c of corrections) {
            const prefix = c.from + '/';
            if (newTarget === c.from || newTarget.startsWith(prefix)) {
                newTarget = c.to + newTarget.slice(c.from.length);
                changed = true;
            }
        }
        if (changed) {
            adjustCount++;
            return { ...op, target: newTarget };
        }
        return op;
    });
    return { operations: adjustedOps, adjustCount: adjustCount };
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
 * 操作模态框配置类型枚举
 * @enum {string}
 */
const ActionModalType = {
    CONFIRM: 'confirm',
    PROMPT: 'prompt'
};

/**
 * 通用操作模态框 — 封装 show / hide / 事件绑定
 * 返回 Promise，confirm 模式下 resolve(true/false)，prompt 模式下 resolve(string|null)
 *
 * @param {Object} options - 配置项
 * @param {string} options.type   - 'confirm' | 'prompt'
 * @param {string} options.title  - 标题
 * @param {string} options.message - 描述文字（confirm 模式必填）
 * @param {string} [options.label] - 输入框标签（prompt 模式）
 * @param {string} [options.defaultValue] - 输入框默认值（prompt 模式）
 * @param {string} [options.hint] - 输入框提示文字（prompt 模式）
 * @param {string} [options.icon] - Font Awesome 图标类名（'fa-question-circle'）
 * @param {string} [options.iconType] - 图标风格: 'info' | 'danger' | 'warning'
 * @param {string} [options.confirmText] - 确认按钮文字
 * @param {string} [options.confirmClass] - 确认按钮额外 CSS 类 ('btn-primary', 'btn-danger')
 * @param {string} [options.cancelText] - 取消按钮文字
 * @param {Function} [options.onValidate] - (value: string) => string|null  校验函数，返回错误信息
 * @returns {Promise<boolean|string|null>}
 */
function showActionModal(options) {
    const {
        type,
        title,
        message = '',
        label = '',
        defaultValue = '',
        hint = '',
        icon = 'fa-question-circle',
        iconType = 'info',
        confirmText = '确认',
        confirmClass = 'btn-primary',
        cancelText = '取消',
        onValidate = null
    } = options;

    return new Promise((resolve) => {
        const modal = document.getElementById('action-modal');
        const modalContent = modal.querySelector('.action-modal-content');
        const closeBtn = document.getElementById('action-modal-close');
        const iconEl = document.getElementById('action-modal-icon');
        const iconInner = iconEl.querySelector('i');
        const titleEl = document.getElementById('action-modal-title');
        const messageEl = document.getElementById('action-modal-message');
        const inputGroup = document.getElementById('action-modal-input-group');
        const labelEl = document.getElementById('action-modal-label');
        const inputEl = document.getElementById('action-modal-input');
        const hintEl = document.getElementById('action-modal-hint');
        const cancelBtn = document.getElementById('action-modal-cancel');
        const confirmBtn = document.getElementById('action-modal-confirm');

        // ---- 清除上一次的状态 ----
        let resolved = false;
        inputEl.value = '';
        inputEl.classList.remove('error');
        hintEl.textContent = '';
        hintEl.classList.remove('error');
        // 重置动画（移除后重排触发）
        modalContent.style.animation = 'none';
        void modalContent.offsetWidth;
        modalContent.style.animation = '';

        /**
         * 安全 resolve，防止重复关闭
         */
        function finalize(value) {
            if (resolved) return;
            resolved = true;
            modal.classList.remove('show');
            resolve(value);
        }

        // ---- 填充 UI ----
        // 图标
        iconEl.className = 'action-modal-icon';
        if (iconType === 'danger') iconEl.classList.add('danger');
        else if (iconType === 'warning') iconEl.classList.add('warning');
        else iconEl.classList.add('info');
        iconInner.className = `fas ${icon}`;

        titleEl.textContent = title;
        messageEl.textContent = message;

        // 输入区域
        if (type === ActionModalType.PROMPT) {
            inputGroup.style.display = 'block';
            labelEl.textContent = label;
            inputEl.value = defaultValue;
            hintEl.textContent = hint;
            inputEl.classList.remove('error');
            hintEl.classList.remove('error');
        } else {
            inputGroup.style.display = 'none';
        }

        // 按钮
        cancelBtn.innerHTML = `<i class="fas fa-times"></i> ${cancelText}`;
        confirmBtn.className = `btn ${confirmClass}`;
        confirmBtn.innerHTML = `<i class="fas fa-check"></i> ${confirmText}`;

        // ---- 事件绑定 ----
        /**
         * 处理确认
         */
        function handleConfirm() {
            if (type === ActionModalType.PROMPT) {
                const value = inputEl.value.trim();
                if (onValidate) {
                    const error = onValidate(value);
                    if (error) {
                        inputEl.classList.add('error');
                        hintEl.textContent = error;
                        hintEl.classList.add('error');
                        inputEl.focus();
                        return;
                    }
                }
                finalize(value || null);
            } else {
                finalize(true);
            }
        }

        /**
         * 处理取消
         */
        function handleCancel() {
            finalize(type === ActionModalType.PROMPT ? null : false);
        }

        // 绑定事件
        confirmBtn.onclick = handleConfirm;
        cancelBtn.onclick = handleCancel;
        closeBtn.onclick = handleCancel;

        // 点击遮罩关闭
        modal.onclick = (e) => {
            if (e.target === modal) handleCancel();
        };

        // 键盘支持
        modal.onkeydown = null; // 清除旧监听器
        const keydownHandler = (e) => {
            if (e.key === 'Enter') {
                e.preventDefault();
                handleConfirm();
            } else if (e.key === 'Escape') {
                e.preventDefault();
                handleCancel();
            }
        };
        modal.addEventListener('keydown', keydownHandler, { once: false });

        // 清理键盘监听器（在关闭时）
        const cleanupKeydown = () => {
            modal.removeEventListener('keydown', keydownHandler);
        };
        const origFinalize = finalize;
        finalize = (value) => {
            cleanupKeydown();
            origFinalize(value);
        };

        // ---- 显示 ----
        modal.classList.add('show');

        // 聚焦输入框或确认按钮
        if (type === ActionModalType.PROMPT) {
            requestAnimationFrame(() => {
                inputEl.focus();
                // 选中默认值文本方便替换
                if (defaultValue) inputEl.select();
            });
        } else {
            requestAnimationFrame(() => confirmBtn.focus());
        }
    });
}

/**
 * 快捷确认模态框
 * @param {string} title - 标题
 * @param {string} message - 描述文字
 * @param {'danger'|'warning'|'info'} [type='danger'] - 风格
 * @returns {Promise<boolean>}
 */
function showConfirmModal(title, message, type = 'danger') {
    const iconMap = {
        danger: { icon: 'fa-exclamation-triangle', iconType: 'danger', confirmClass: 'btn-danger', confirmText: '确认删除' },
        warning: { icon: 'fa-exclamation-circle', iconType: 'warning', confirmClass: 'btn-accent', confirmText: '确认' },
        info: { icon: 'fa-info-circle', iconType: 'info', confirmClass: 'btn-primary', confirmText: '确认' }
    };
    const cfg = iconMap[type] || iconMap.info;

    return showActionModal({
        type: ActionModalType.CONFIRM,
        title,
        message,
        icon: cfg.icon,
        iconType: cfg.iconType,
        confirmText: cfg.confirmText,
        confirmClass: cfg.confirmClass
    });
}

/**
 * 快捷输入模态框
 * @param {string} title - 标题
 * @param {string} label - 输入框标签
 * @param {string} [defaultValue=''] - 默认值
 * @param {string} [hint=''] - 输入提示
 * @param {Function} [onValidate] - 校验函数 (value) => errorString|null
 * @returns {Promise<string|null>} 用户输入值，取消时返回 null
 */
function showPromptModal(title, label, defaultValue = '', hint = '', onValidate = null) {
    return showActionModal({
        type: ActionModalType.PROMPT,
        title,
        label,
        defaultValue,
        hint,
        icon: 'fa-pen-to-square',
        iconType: 'info',
        confirmText: '确认',
        confirmClass: 'btn-primary',
        onValidate
    });
}

/**
 * 显示文本模态框
 * @param {Object} file - 要预览的文件对象
 * @param {string} currentPath - 当前路径
 * @param {Function} onSave - 保存完成回调
 */
async function showTextModal(file, currentPath, onSave) {
    try {
        const response = await fetch(`/file/read/${file.path}`);
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
            link.href = `/file/download/${file.path}`;
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
        await uploadFile(uploadFileObj, currentPath, () => { }, true);
    } catch (error) {
        throw new Error('保存文件失败');
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

        // 移动目标选择 / 冲突处理模态框
        const moveModal = document.getElementById('move-modal');
        if (moveModal.classList.contains('show')) closeMoveModal();
        const conflictModal = document.getElementById('conflict-modal');
        if (conflictModal.classList.contains('show')) closeConflictModal();

        // 智能整理模态框（进行中不允许关闭）
        const organizeModal = document.getElementById('organize-modal');
        if (organizeModal.classList.contains('show') && !isOrganizing) {
            organizeModal.classList.remove('show');
        }

        // 图片转码 / GGUF 预览 / ZIP 预览模态框
        const convertModal = document.getElementById('convert-modal');
        if (convertModal.classList.contains('show')) closeConvertModal();
        const ggufModal = document.getElementById('gguf-modal');
        if (ggufModal.classList.contains('show')) closeGGUFModal();
        const zipModal = document.getElementById('zip-modal');
        if (zipModal.classList.contains('show')) closeZipModal();

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
                    imagePreview.src = `/file/read/${file.path}`;
                } else {
                    imageDragContainer.style.display = 'none';
                    imagePreview.style.display = 'none';
                    videoPreview.style.display = 'block';
                    videoPreview.alt = file.name;
                    videoPreview.src = `/file/read/${file.path}`;
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
 * 图片转码与 GGUF 预览模块
 * 参考 lunar.means.image.converter 与 lunar.means.gguf.viewer 扩展包实现
 */


// ==== GGUF 模型元数据预览 ====

/** GGUF 摘要字段中文映射 */
const GGUF_LABEL_MAP = {
    'Model Name': '模型名称',
    'Architecture': '架构',
    'Quantization': '量化方式',
    'Quant Version': '量化版本',
    'Context Length': '上下文长度',
    'Embedding Dim': '嵌入维度',
    'Block Count': '层数',
    'Attention Heads': '注意力头数',
    'KV Heads': 'KV 头数',
    'FFN Dim': 'FFN 维度',
    'Vocab Size': '词表大小'
};

/** GGUF 摘要字段展示顺序 */
const GGUF_KEY_ORDER = [
    'Model Name', 'Architecture', 'Quantization', 'Quant Version',
    'Context Length', 'Embedding Dim', 'Block Count',
    'Attention Heads', 'KV Heads', 'FFN Dim', 'Vocab Size'
];

/**
 * 展示 GGUF 模型预览模态框
 * @param {Object} file - 文件对象（path 为相对 LocalDir 的路径）
 */
async function showGGUFModal(file) {
    const modal = document.getElementById('gguf-modal');
    const pathEl = document.getElementById('gguf-file-path');
    const cardsEl = document.getElementById('gguf-summary-cards');
    const bodyEl = document.getElementById('gguf-metadata-body');

    document.getElementById('gguf-search-input').value = '';
    document.getElementById('gguf-metadata-count').textContent = '';
    pathEl.textContent = file.path;
    cardsEl.innerHTML = '<div class="gguf-loading"><i class="fas fa-spinner fa-spin"></i> 正在解析模型元数据...</div>';
    bodyEl.innerHTML = '';
    modal.classList.add('show');

    try {
        const response = await fetch('/gguf/metadata', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ filePath: file.path })
        });
        const data = await response.json();

        if (!data.success) {
            cardsEl.innerHTML = `<div class="gguf-error"><i class="fas fa-exclamation-triangle"></i> ${mediaEscapeHTML(data.error || '解析失败，请检查文件格式')}</div>`;
            return;
        }

        pathEl.textContent = data.filePath || file.path;
        renderGGUFSummary(cardsEl, data.summary);
        document.getElementById('gguf-metadata-count').textContent = `（${data.count} 项）`;
        renderGGUFMetadataTable(bodyEl, data.metadata);
    } catch (err) {
        cardsEl.innerHTML = '<div class="gguf-error"><i class="fas fa-exclamation-triangle"></i> 网络请求失败，请检查服务是否正常运行</div>';
    }
}

/**
 * 关闭 GGUF 预览模态框
 */
function closeGGUFModal() {
    document.getElementById('gguf-modal').classList.remove('show');
}

/**
 * 渲染 GGUF 摘要卡片
 * @param {HTMLElement} container - 卡片容器
 * @param {Object} summary - 摘要字段映射
 */
function renderGGUFSummary(container, summary) {
    container.innerHTML = '';
    if (!summary) {
        container.innerHTML = '<div class="gguf-error"><i class="fas fa-inbox"></i> 无摘要信息</div>';
        return;
    }

    const orderedKeys = GGUF_KEY_ORDER.filter(k => summary[k]);
    for (const key of Object.keys(summary)) {
        if (!orderedKeys.includes(key)) orderedKeys.push(key);
    }

    for (const key of orderedKeys) {
        const value = summary[key];
        if (value === undefined || value === '') continue;
        const label = GGUF_LABEL_MAP[key] || key;
        const card = document.createElement('div');
        card.className = 'gguf-summary-card';
        card.innerHTML = `
            <div class="gguf-summary-label">${mediaEscapeHTML(label)}</div>
            <div class="gguf-summary-value">${mediaEscapeHTML(String(value))}</div>
        `;
        container.appendChild(card);
    }
}

/**
 * 渲染 GGUF 元数据表格
 * @param {HTMLElement} tbody - 表格主体
 * @param {Object} metadata - 元数据键值映射
 */
function renderGGUFMetadataTable(tbody, metadata) {
    tbody.innerHTML = '';
    const keys = Object.keys(metadata || {}).sort();
    if (keys.length === 0) {
        tbody.innerHTML = '<tr><td colspan="2" class="gguf-empty"><i class="fas fa-inbox"></i> 暂无元数据</td></tr>';
        return;
    }
    for (const key of keys) {
        const value = metadata[key];
        const tr = document.createElement('tr');
        tr.setAttribute('data-key', key.toLowerCase());
        tr.setAttribute('data-value', String(value).toLowerCase());
        const tdKey = document.createElement('td');
        tdKey.textContent = key;
        const tdValue = document.createElement('td');
        tdValue.textContent = value;
        const numValue = Number(value);
        if (!isNaN(numValue) && String(value).trim() !== '') {
            tdValue.className = 'gguf-value-number';
        } else if (value === 'true' || value === 'false') {
            tdValue.className = 'gguf-value-bool';
        } else {
            tdValue.className = 'gguf-value-string';
        }
        tr.appendChild(tdKey);
        tr.appendChild(tdValue);
        tbody.appendChild(tr);
    }
}

// ==== 图片转码 ====

/** 图片转码状态 */
const convertState = {
    mode: 'batch',       // batch 批量 / selected 选中
    sourceFormat: 'all', // all / png / jpeg / webp
    targetFormat: 'jpeg',
    deleteSource: true,  // 默认转换后删除源文件
    quality: 90
};

/**
 * 打开图片转码模态框
 * @param {FileManager} fileManager - 文件管理器实例
 */
function openConvertModal(fileManager) {
    // 统计当前层级可转码图片与选中图片
    const folderImages = fileManager.files.filter(f => !f.isDir && isConvertibleImage(f.name));
    const selectedImages = Array.from(fileManager.selectedFiles)
        .map(path => fileManager.files.find(f => f.path === path))
        .filter(f => f && !f.isDir && isConvertibleImage(f.name));

    // 有选中的图片时默认「转换选中图片」，否则默认「批量转换」
    convertState.mode = selectedImages.length > 0 ? 'selected' : 'batch';
    convertState.sourceFormat = 'all';
    convertState.targetFormat = 'jpeg';

    updateConvertModeTabs();
    updateConvertModeHint(folderImages, selectedImages);
    setConvertTabActive('convert-source-tabs', 'all');
    setConvertTabActive('convert-target-tabs', 'jpeg');

    const resultEl = document.getElementById('convert-result');
    resultEl.style.display = 'none';
    resultEl.innerHTML = '';

    document.getElementById('convert-modal').classList.add('show');
}

/**
 * 关闭图片转码模态框
 */
function closeConvertModal() {
    document.getElementById('convert-modal').classList.remove('show');
}

/**
 * 更新转换模式选项卡高亮
 */
function updateConvertModeTabs() {
    document.querySelectorAll('#convert-mode-tabs .convert-mode-tab').forEach(tab => {
        tab.classList.toggle('active', tab.dataset.mode === convertState.mode);
    });
}

/**
 * 更新模式提示文案
 * @param {Array<Object>} folderImages - 当前层级可转码图片列表
 * @param {Array<Object>} selectedImages - 选中的可转码图片列表
 */
function updateConvertModeHint(folderImages, selectedImages) {
    const hintEl = document.getElementById('convert-mode-hint');
    const sourceSection = document.getElementById('convert-source-section');
    if (convertState.mode === 'selected') {
        if (selectedImages.length > 0) {
            hintEl.innerHTML = `<i class="fas fa-check"></i> 将转换选中的 ${selectedImages.length} 张图片`;
        } else {
            hintEl.innerHTML = '<i class="fas fa-info-circle"></i> 未勾选图片文件，可先勾选文件卡片右上角的复选框，即可只转换指定的图片';
        }
        sourceSection.style.display = 'none';
    } else {
        const total = folderImages.length;
        hintEl.innerHTML = `<i class="fas fa-info-circle"></i> 将批量转换当前层级全部 ${total} 张可转码图片（也可以先勾选部分图片，只转换所选）`;
        sourceSection.style.display = '';
    }
}

/**
 * 切换格式选项卡高亮
 * @param {string} containerId - 选项卡容器 ID
 * @param {string} format - 选中格式
 */
function setConvertTabActive(containerId, format) {
    document.querySelectorAll(`#${containerId} .convert-tab`).forEach(tab => {
        tab.classList.toggle('active', tab.dataset.format === format);
    });
}

/**
 * 开始执行图片转码
 * @param {FileManager} fileManager - 文件管理器实例
 */
async function startImageConvert(fileManager) {
    const targetFormat = convertState.targetFormat;
    const deleteSource = document.getElementById('convert-delete-source').checked;
    const quality = parseInt(document.getElementById('convert-quality').value, 10) || 90;
    const resultEl = document.getElementById('convert-result');

    const folderImages = fileManager.files.filter(f => !f.isDir && isConvertibleImage(f.name));
    const selectedImages = Array.from(fileManager.selectedFiles)
        .map(path => fileManager.files.find(f => f.path === path))
        .filter(f => f && !f.isDir && isConvertibleImage(f.name));

    // 批量模式需要源格式；选中模式逐张转换无需源格式过滤
    const sourceFormat = convertState.mode === 'batch' ? convertState.sourceFormat : 'all';

    resultEl.style.display = 'block';
    resultEl.innerHTML = '<div class="convert-progress"><i class="fas fa-spinner fa-spin"></i> 正在转换，请稍候...</div>';

    try {
        if (convertState.mode === 'batch') {
            await batchImageConvert(fileManager, folderImages, sourceFormat, targetFormat, deleteSource, quality, resultEl);
        } else {
            if (selectedImages.length === 0) {
                resultEl.innerHTML = '<div class="convert-error"><i class="fas fa-info-circle"></i> 未勾选任何图片文件，请先勾选要转换的图片</div>';
                return;
            }
            await selectedImageConvert(selectedImages, targetFormat, deleteSource, quality, resultEl);
        }

        await fileManager.loadFiles();
        fileManager.updateStats();
    } catch (err) {
        resultEl.innerHTML = `<div class="convert-error"><i class="fas fa-exclamation-triangle"></i> 转换流程失败: ${mediaEscapeHTML(err.message || '未知错误')}</div>`;
    }
}

/**
 * 批量转换：POST /convert/batch
 * @param {FileManager} fileManager - 文件管理器实例
 */
async function batchImageConvert(fileManager, folderImages, sourceFormat, targetFormat, deleteSource, quality, resultEl) {
    const response = await fetch('/convert/batch', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
            folder: fileManager.currentPath || '.',
            source_format: sourceFormat,
            target_format: targetFormat,
            delete_source: deleteSource,
            quality: quality
        })
    });
    if (!response.ok) throw new Error('批量转换请求失败');
    const data = await response.json();
    if (!data.success) throw new Error(data.error || '批量转换失败');

    let html = `<div class="convert-summary"><i class="fas fa-check-circle"></i> 批量转换完成：成功 ${data.success_count}，失败 ${data.fail_count}，共 ${data.total} 项</div>`;
    const failed = (data.results || []).filter(r => !r.success);
    if (failed.length > 0) {
        html += '<div class="convert-fail-list">';
        failed.forEach(r => {
            html += `<div class="convert-fail-item"><i class="fas fa-times-circle"></i> ${mediaEscapeHTML(r.path.split(/[\\/]/).pop())} — ${mediaEscapeHTML(r.error || '未知错误')}</div>`;
        });
        html += '</div>';
    }
    resultEl.innerHTML = html;
}

/**
 * 逐个转换选中的图片：POST /convert/image
 * @param {Array<Object>} selectedImages - 选中的可转码图片
 */
async function selectedImageConvert(selectedImages, targetFormat, deleteSource, quality, resultEl) {
    const results = [];
    const targetExt = '.' + (targetFormat === 'jpeg' ? 'jpg' : targetFormat);

    for (let i = 0; i < selectedImages.length; i++) {
        const file = selectedImages[i];
        const ext = file.name.toLowerCase().slice(file.name.lastIndexOf('.'));
        resultEl.innerHTML = `<div class="convert-progress"><i class="fas fa-spinner fa-spin"></i> 正在转换 (${i + 1}/${selectedImages.length}): ${mediaEscapeHTML(file.name)}</div>`;

        // 源格式与目标格式相同则跳过
        if (ext === targetExt) {
            results.push({ name: file.name, success: false, error: '源格式与目标格式相同，无需转换' });
            continue;
        }

        try {
            const response = await fetch('/convert/image', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    path: file.path,
                    target_format: targetFormat,
                    delete_source: deleteSource,
                    quality: quality
                })
            });
            const data = await response.json();
            if (!data.success) {
                results.push({ name: file.name, success: false, error: data.error || '转换失败' });
            } else {
                results.push({ name: file.name, success: true });
            }
        } catch (err) {
            results.push({ name: file.name, success: false, error: err.message || '网络请求失败' });
        }
    }

    const successCount = results.filter(r => r.success).length;
    let html = `<div class="convert-summary"><i class="fas fa-check-circle"></i> 选中图片转换完成：成功 ${successCount}，失败 ${results.length - successCount}</div>`;
    const failed = results.filter(r => !r.success);
    if (failed.length > 0) {
        html += '<div class="convert-fail-list">';
        failed.forEach(r => {
            html += `<div class="convert-fail-item"><i class="fas fa-times-circle"></i> ${mediaEscapeHTML(r.name)} — ${mediaEscapeHTML(r.error || '未知错误')}</div>`;
        });
        html += '</div>';
    }
    resultEl.innerHTML = html;
}

/**
 * 格式化字节大小
 * @param {number} bytes - 字节数
 * @returns {string} 人类可读大小
 */
function zipFormatSize(bytes) {
    if (!bytes && bytes !== 0) return '-';
    if (bytes < 1024) return bytes + ' B';
    if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB';
    if (bytes < 1024 * 1024 * 1024) return (bytes / (1024 * 1024)).toFixed(2) + ' MB';
    return (bytes / (1024 * 1024 * 1024)).toFixed(2) + ' GB';
}

/**
 * 展示 ZIP 压缩包预览模态框
 * @param {Object} file - ZIP 文件对象
 * @param {Object} fileManager - 文件管理器实例
 */
async function showZipModal(file, fileManager) {
    const modal = document.getElementById('zip-modal');
    const filePathEl = document.getElementById('zip-file-path');
    const summaryEl = document.getElementById('zip-summary-cards');
    const entryBody = document.getElementById('zip-entry-body');
    const entryCount = document.getElementById('zip-entry-count');
    const resultEl = document.getElementById('zip-extract-result');
    const pathInput = document.getElementById('zip-extract-path');
    const hintEl = document.getElementById('zip-extract-hint');

    filePathEl.textContent = file.path || file.name;
    summaryEl.innerHTML = '<div class="zip-loading"><i class="fas fa-spinner fa-spin"></i> 正在读取压缩包信息...</div>';
    entryBody.innerHTML = '';
    entryCount.textContent = '';
    resultEl.innerHTML = '';
    hintEl.innerHTML = '';

    // 默认解压路径：当前路径/压缩包名/
    const baseName = (file.name || '').replace(/\.zip$/i, '');
    const dirParts = [];
    if (fileManager.currentPath) dirParts.push(fileManager.currentPath);
    dirParts.push(baseName);
    pathInput.value = dirParts.join('/') + '/';

    modal.classList.add('show');

    try {
        const response = await fetch('/file/archive/metadata', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ path: file.path })
        });
        if (!response.ok) throw new Error('读取失败');
        const data = await response.json();
        if (!data.success) throw new Error(data.error || '读取失败');

        // 摘要卡片
        const dirCount = data.entries.filter(e => e.isDir).length;
        const fileCount = data.file_count - dirCount;
        const cards = [
            { label: '压缩包大小', value: zipFormatSize(data.zip_size) },
            { label: '解压后大小', value: zipFormatSize(data.total_size) },
            { label: '文件数', value: fileCount },
            { label: '文件夹数', value: dirCount }
        ];
        summaryEl.innerHTML = cards.map(c =>
            `<div class="zip-summary-card"><div class="zip-summary-label">${c.label}</div><div class="zip-summary-value">${c.value}</div></div>`
        ).join('');

        entryCount.textContent = `（${data.file_count} 个条目）`;

        // 条目列表
        if (data.entries.length === 0) {
            entryBody.innerHTML = '<tr><td colspan="4" class="zip-empty">压缩包为空</td></tr>';
        } else {
            entryBody.innerHTML = data.entries.map(entry => {
                const icon = entry.isDir
                    ? '<i class="fas fa-folder" style="color: var(--accent);"></i>'
                    : '<i class="fas fa-file" style="color: var(--text-muted);"></i>';
                const type = entry.isDir ? '文件夹' : '文件';
                return `<tr>
                    <td class="zip-entry-name">${icon} <span>${mediaEscapeHTML(entry.name)}</span></td>
                    <td>${type}</td>
                    <td>${zipFormatSize(entry.size)}</td>
                    <td>${zipFormatSize(entry.compressed)}</td>
                </tr>`;
            }).join('');
        }

        hintEl.innerHTML = `<i class="fas fa-info-circle"></i> 将解压到：${mediaEscapeHTML(pathInput.value)}`;
    } catch (error) {
        summaryEl.innerHTML = `<div class="zip-error"><i class="fas fa-exclamation-triangle"></i> ${mediaEscapeHTML(error.message || '读取压缩包信息失败')}</div>`;
        console.error('读取 ZIP 元数据失败:', error);
    }
}

/**
 * 关闭 ZIP 预览模态框
 */
function closeZipModal() {
    document.getElementById('zip-modal').classList.remove('show');
    document.getElementById('zip-extract-result').innerHTML = '';
}

/**
 * 执行 ZIP 解压
 * @param {Object} fileManager - 文件管理器实例
 */
async function startZipExtract(fileManager) {
    const resultEl = document.getElementById('zip-extract-result');
    const filePath = document.getElementById('zip-file-path').textContent;
    const targetDir = document.getElementById('zip-extract-path').value.trim();

    if (!targetDir) {
        resultEl.innerHTML = '<div class="zip-error"><i class="fas fa-exclamation-triangle"></i> 请输入解压路径</div>';
        return;
    }

    resultEl.innerHTML = '<div class="zip-progress"><i class="fas fa-spinner fa-spin"></i> 正在解压...</div>';

    try {
        const response = await fetch('/file/archive/extract', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ path: filePath, target_dir: targetDir })
        });
        if (!response.ok) throw new Error('解压失败');
        const data = await response.json();
        if (!data.success) throw new Error(data.error || '解压失败');

        resultEl.innerHTML = `<div class="zip-success"><i class="fas fa-check-circle"></i> 已解压 ${data.file_count} 个文件到 ${mediaEscapeHTML(data.target_dir)}</div>`;
        showToast(`解压完成：${data.file_count} 个文件`, 'success');

        // 解压完成后刷新文件列表并关闭模态框
        setTimeout(async () => {
            await fileManager.loadFiles();
            fileManager.updateStats();
            closeZipModal();
        }, 800);
    } catch (error) {
        resultEl.innerHTML = `<div class="zip-error"><i class="fas fa-exclamation-triangle"></i> ${mediaEscapeHTML(error.message || '解压失败')}</div>`;
        console.error('解压失败:', error);
    }
}

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
        this.pageSize = 24;
    }

    /**
     * 初始化文件管理器
     */
    init() {
        bindEvents(this);
        this.loadFiles();
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
                this.allFiles = await traverseAllFiles();
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
            if (isGGUFFile(file.name)) {
                // GGUF 模型文件：解析元数据作为预览
                showGGUFModal(file);
            } else if (isZipFile(file.name)) {
                // ZIP 压缩包：预览元数据并支持解压
                showZipModal(file, this);
            } else if (isImageFile(file.name) || isVideoFile(file.name) || isAudioFile(file.name)) {
                const mediaIndex = this.currentMediaList.findIndex(media => media.path === file.path);
                this.currentMediaIndex = mediaIndex;
                previewImage(`/file/read/${file.path}`, file.name);
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
     * 全选当前层级（或搜索结果）中的所有项目
     */
    selectAllVisible() {
        const displayFiles = this.isSearching ? this.searchResults : this.files;
        for (const file of displayFiles) {
            this.selectedFiles.add(file.path);
            updateFileCardSelection(file, true);
        }
        this.updateBatchActions();
    }

    /**
     * 取消全选
     */
    clearSelection() {
        for (const file of this.files) {
            if (this.selectedFiles.has(file.path)) {
                updateFileCardSelection(file, false);
            }
        }
        this.selectedFiles.clear();
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


// ===== LTPX AtoA 智能体（月华经琉璃调用本包的文件操作能力） =====
// 琉璃主窗口通过 postMessage 投递 ltpx_run，本包解析自然语言指令执行文件操作，
// 驱动页面 UI（跳转目录/选中文件/打开模态窗），完成后经 window.parent 回传 ltpx_result。

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

// ===== LTPX AtoA 指令主处理（下半部分续下） =====
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

// 等待 DOM 加载完成
document.addEventListener('DOMContentLoaded', () => {
    // 初始化文件管理器
    const fileManager = new FileManager();
    fileManager.init();

    // LTPX AtoA：监听琉璃主窗口投递的工具调用
    window.addEventListener('message', async (event) => {
        const data = event.data;
        if (!data || typeof data !== 'object' || data.type !== 'ltpx_run') return;
        try {
            const result = await handleLTPXInstruction(fileManager, (data.arguments || {}).instruction || '');
            postLTPXResult(data.request_id, result.success, result.text, result.error);
        } catch (e) {
            console.error('LTPX AtoA 执行失败:', e);
            postLTPXResult(data.request_id, false, '', e.message || '执行失败');
        }
    });
});
