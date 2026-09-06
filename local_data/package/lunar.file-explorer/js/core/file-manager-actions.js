/**
 * 文件管理器主类（动作方法模块）
 * 负责文件点击分发、目录导航、面包屑、重命名 / 删除 / 下载。
 * 通过 Object.assign 挂载到 FileManager.prototype。
 */

/**
 * 处理文件点击
 * @param {Object} file - 文件对象
 */
FileManager.prototype.handleFileClick = async function (file) {
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
};

/**
 * 导航到目录
 * @param {Object} directory - 目录对象
 */
FileManager.prototype.navigateToDirectory = function (directory) {
    const normalizedPath = directory.path.replace(/\\/g, '/');
    this.currentPath = normalizedPath;
    this.selectedFiles.clear();
    this.updateBatchActions();
    this.currentPage = 1;
    this.loadFiles();
};

/**
 * 返回上一级
 */
FileManager.prototype.goBack = function () {
    if (!this.currentPath) return;

    const pathParts = this.currentPath.split('/');
    pathParts.pop();
    this.currentPath = pathParts.join('/');
    this.selectedFiles.clear();
    this.updateBatchActions();
    this.currentPage = 1;
    this.loadFiles();
};

/**
 * 更新面包屑导航
 * @param {boolean} isSearching - 是否在搜索中
 */
FileManager.prototype.updateBreadcrumb = function (isSearching = false) {
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
};

/**
 * 重命名文件
 * @param {Object} file - 文件对象
 */
FileManager.prototype.renameFile = async function (file) {
    await renameFile(file, this.currentPath, async () => {
        await this.loadFiles();
    });
};

/**
 * 删除文件
 * @param {Object} file - 文件对象
 */
FileManager.prototype.deleteFile = async function (file) {
    await deleteFile(file, async () => {
        await this.loadFiles();
    });
};

/**
 * 下载文件
 * @param {Object} file - 文件对象
 */
FileManager.prototype.downloadFile = async function (file) {
    await downloadFile(file);
};
