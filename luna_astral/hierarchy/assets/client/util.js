// 边框颜色常量
export const BORDER_COLORS = [
    'var(--status-218838)',
    'var(--status-3a5a8a)',
    'var(--status-4a6fa5)',
    'var(--status-6c9bcf)',
    'var(--status-8a2be2)',
    'var(--status-9d6bff)',
    'var(--status-dc3545)',
    'var(--status-fbbf24)',
    'var(--status-ffc107)',
    'var(--status-20c997)',
    'var(--status-ff6b9c)',
];

/**
 * 获取随机边框颜色
 *
 * @returns {string} - CSS颜色值
 */
export function randomBorderColor() {
    return BORDER_COLORS[Math.floor(Math.random() * BORDER_COLORS.length)];
}

/**
 * HTML字符转义
 *
 * @param {string} text - 原始文本
 *
 * @returns {string} - 转义后的HTML安全字符串
 */
export function escapeHtml(text) {
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
}

/**
 * 清空容器内容
 *
 * @param {HTMLElement} container - 目标容器
 */
export function clearContainer(container) {
    container.innerHTML = '';
}

/**
 * 编码文件路径（用于HTTP传输）
 * 后端要求将目录和文件名一起编码后放入 X-File-Name 头
 *
 * @param {string} filepath - 路径，如 "document/readme.txt"
 * @returns {string} - Base64编码后的字符串
 */
export function encodeFilePath(filepath) {
    const encodedParams = encodeURIComponent(filepath);
    const decodedParams = encodedParams.replace(/%([0-9A-F]{2})/g, (_, p1) => String.fromCharCode(parseInt(p1, 16)));
    return btoa(decodedParams);
}