/**
 * 事件绑定模块
 * 负责文件管理器的所有事件绑定
 */

import {
    handleFileUpload,
    handleZipUpload,
    createNewFolder,
    batchDelete,
    batchCompress,
    renameFile,
    downloadFile,
    deleteFile
} from './file-operations.js';
import { closeTextModal, closeQRCodeModal, showQRCode, showTextModal } from './modal-handler.js';
import { showToast } from './utils.js';

/**
 * 绑定事件
 * @param {Object} fileManager - 文件管理器实例
 */
export function bindEvents(fileManager) {
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
