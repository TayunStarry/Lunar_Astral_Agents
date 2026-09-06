/**
 * 文件管理器主类（选中与键盘模块）
 * 负责选中切换、全选 / 取消全选与键盘事件转发。
 * 通过 Object.assign 挂载到 FileManager.prototype。
 */

/**
 * 切换文件选中状态
 * @param {Object} file - 文件对象
 * @param {boolean} isSelected - 是否选中
 */
FileManager.prototype.toggleFileSelection = function (file, isSelected) {
    if (isSelected) {
        this.selectedFiles.add(file.path);
    } else {
        this.selectedFiles.delete(file.path);
    }

    updateFileCardSelection(file, isSelected);
    this.updateBatchActions();
};

/**
 * 全选当前层级（或搜索结果）中的所有项目
 */
FileManager.prototype.selectAllVisible = function () {
    const displayFiles = this.isSearching ? this.searchResults : this.files;
    for (const file of displayFiles) {
        this.selectedFiles.add(file.path);
        updateFileCardSelection(file, true);
    }
    this.updateBatchActions();
};

/**
 * 取消全选
 */
FileManager.prototype.clearSelection = function () {
    for (const file of this.files) {
        if (this.selectedFiles.has(file.path)) {
            updateFileCardSelection(file, false);
        }
    }
    this.selectedFiles.clear();
    this.updateBatchActions();
};

/**
 * 处理键盘事件
 * @param {Event} event - 键盘事件对象
 */
FileManager.prototype.handleKeyboard = function (event) {
    this.currentMediaIndex = handleKeyboardEvent(
        event,
        this.currentMediaList,
        this.currentMediaIndex,
        (newIndex) => {
            this.currentMediaIndex = newIndex;
        }
    );
};
