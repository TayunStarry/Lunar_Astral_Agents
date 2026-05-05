/**
 * 文件管理器主类
 * 整合所有功能模块
 */

import { bindEvents } from './event-handler.js';
import { loadFiles as loadFilesFromApi, traverseAllFiles, saveIndexToFile, loadIndexFromFile } from './file-operations.js';
import {
    updateStats,
    updateBreadcrumb,
    updateFileGrid,
    updateBatchActions,
    updateFileCardSelection,
    renderPagination
} from './ui-renderer.js';
import { isImageFile, isVideoFile, isAudioFile, isTextFile, showToast } from './utils.js';
import { showTextModal, handleKeyboardEvent } from './modal-handler.js';
import {
    renameFile as renameFileApi,
    deleteFile as deleteFileApi,
    downloadFile as downloadFileApi
} from './file-operations.js';

/**
 * 文件管理器类
 */
export class FileManager {
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
            this.files = await loadFilesFromApi(this.currentPath);
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
                saveIndexToFile(this.allFiles)
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
        await renameFileApi(file, this.currentPath, async () => {
            await this.loadFiles();
        });
    }

    /**
     * 删除文件
     * @param {Object} file - 文件对象
     */
    async deleteFile(file) {
        await deleteFileApi(file, async () => {
            await this.loadFiles();
        });
    }

    /**
     * 下载文件
     * @param {Object} file - 文件对象
     */
    async downloadFile(file) {
        await downloadFileApi(file);
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
