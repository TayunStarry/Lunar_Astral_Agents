/**
 * 文件管理器入口脚本
 * 等待 DOM 加载完成后初始化文件管理器，并监听琉璃主窗口投递的 LTPX AtoA 工具调用
 */

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
            const result = await runLTPXAgent(fileManager, (data.arguments || {}).instruction || '回到根目录');
            postLTPXResult(data.request_id, result.success, result.text, result.error);
        } catch (e) {
            console.error('LTPX AtoA 执行失败:', e);
            postLTPXResult(data.request_id, false, '', e.message || '执行失败');
        }
    });
});
