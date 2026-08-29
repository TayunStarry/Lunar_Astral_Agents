/**
 * 基础文件操作模块
 * 负责文件列表加载、文件上传、新建文件夹、删除、下载与全量遍历
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
