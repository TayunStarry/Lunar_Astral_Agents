/**
 * 文件操作模块
 * 负责文件管理器的所有文件操作功能
 */

import { encodeFileName, isValidFileName, getFileType, showToast } from './utils.js';
import { updateUploadProgress } from './ui-renderer.js';

/**
 * 加载文件列表
 * @param {string} currentPath - 当前路径
 * @returns {Promise<Array>} - 文件列表
 */
export async function loadFiles(currentPath) {
    try {
        const response = await fetch(`/file_list/${currentPath}`);
        if (!response.ok) throw new Error('加载文件失败');
        return await response.json();
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
export async function uploadFile(file, currentPath, onProgress, overwrite = true) {
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
export async function handleFileUpload(files, currentPath, onComplete) {
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
export async function createNewFolder(currentPath, onComplete) {
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

        await uploadFile(file, currentPath, () => {}, true);
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
export async function deleteFile(file, onComplete) {
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
export async function renameFile(file, currentPath, onComplete) {
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
    await uploadFile(newFile, currentPath, () => {}, true);
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
    await uploadFile(file, currentPath, () => {}, true);
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
    await uploadFile(newFile, targetDir || currentPath, () => {}, false);
}

/**
 * 批量删除
 * @param {Set} selectedFiles - 选中的文件集合
 * @param {Function} onComplete - 完成回调
 */
export async function batchDelete(selectedFiles, onComplete) {
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
export async function batchCompress(files, selectedFiles) {
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
export async function handleZipUpload(file, currentPath, onComplete) {
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
            await uploadFile(uploadFileObj, currentPath, () => {}, false);
        }

        showToast(`成功解压 ${result.total_files} 个文件`, 'success');
        onComplete();
    } catch (error) {
        showToast('解压失败', 'error');
        console.error('解压失败:', error);
    }
}

/**
 * 下载文件
 * @param {Object} file - 要下载的文件对象
 */
export async function downloadFile(file) {
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
export async function traverseAllFiles(startPath = '') {
    const allFiles = [];
    const queue = [startPath];

    while (queue.length > 0) {
        const currentPath = queue.shift();
        try {
            const response = await fetch(`/file_list/${currentPath}`);
            if (!response.ok) continue;
            const files = await response.json();
            allFiles.push(...files);
            const subDirs = files.filter(file => file.isDir);
            for (const dir of subDirs) {
                queue.push(dir.path);
            }
        } catch (error) {
            console.error('遍历文件失败:', error);
        }
    }

    return allFiles;
}
