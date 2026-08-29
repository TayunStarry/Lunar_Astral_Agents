/**
 * 重命名与目录复制模块
 * 负责文件/目录重命名、目录内容读取、创建与递归复制
 */

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
