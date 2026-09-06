/**
 * 文件管理器主类（核心模块）
 * 负责实例状态、初始化、文件加载、网格更新、分页与搜索。
 * 动作类方法（打开/导航/重命名/删除/下载）见 file-manager-actions.js，
 * 选中与键盘类方法见 file-manager-selection.js。
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
}
