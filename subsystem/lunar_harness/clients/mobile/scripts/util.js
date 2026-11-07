/**
 * 移动端工具：通用能力来自 /shared/core/utils.js，
 * 这里只保留移动端独有的配色常量与 DOM 小工具。
 */
export { generateId, formatTime, escapeHtml, encodeFilePath, formatFileSize, fileToRawBase64 } from '/shared/core/utils.js';

/** 边框颜色常量 */
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
	'var(--status-ff6b9c)'
];

/** 获取随机边框颜色 */
export function randomBorderColor() {
	return BORDER_COLORS[Math.floor(Math.random() * BORDER_COLORS.length)];
}

/** 清空容器内容 */
export function clearContainer(container) {
	container.innerHTML = '';
}
