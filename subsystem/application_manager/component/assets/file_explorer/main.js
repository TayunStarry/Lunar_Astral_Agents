/**
 * 文件管理器入口脚本
 */

import { FileManager } from './file-manager.js';

// 等待 DOM 加载完成
document.addEventListener('DOMContentLoaded', () => {
    // 初始化文件管理器
    const fileManager = new FileManager();
    fileManager.init();
});
