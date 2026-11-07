/**
 * 星月智能 · 桌面端 / 移动端共享工具
 *
 * 只包含无端差异的纯函数：ID、时间、转义、路径编码、文件大小、剪贴板。
 */

/** UUID v4 风格消息 ID（msg- 前缀 + 标准 v4 UUID） */
export function generateId() {
	const bytes = new Uint8Array(16);
	crypto.getRandomValues(bytes);
	bytes[6] = (bytes[6] & 0x0f) | 0x40; // 版本位 → v4
	bytes[8] = (bytes[8] & 0x3f) | 0x80; // 变体位 → RFC 4122
	const hex = Array.from(bytes, b => b.toString(16).padStart(2, '0')).join('');
	const uuid = `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`;
	return `msg-${uuid}`;
}

/** 本地时间字符串；withSeconds=false 时只到分钟（移动端历史列表） */
export function formatTime(timestamp, withSeconds = true) {
	if (!timestamp) {
		const now = new Date();
		return now.toLocaleTimeString('zh-CN', withSeconds
			? { hour: '2-digit', minute: '2-digit', second: '2-digit' }
			: { hour: '2-digit', minute: '2-digit' });
	}
	const date = new Date(timestamp);
	if (Number.isNaN(date.getTime())) return '';
	return date.toLocaleTimeString('zh-CN', withSeconds
		? { hour: '2-digit', minute: '2-digit', second: '2-digit' }
		: { hour: '2-digit', minute: '2-digit' });
}

/** HTML 转义（用于 innerHTML 拼接） */
export function escapeHtml(text) {
	const div = document.createElement('div');
	div.textContent = text == null ? '' : String(text);
	return div.innerHTML;
}

/** 编码文件路径（X-File-Name 头要求：目录与文件名一起 base64） */
export function encodeFilePath(filepath) {
	const encodedParams = encodeURIComponent(filepath);
	const decodedParams = encodedParams.replace(/%([0-9A-F]{2})/g, (_, p1) => String.fromCharCode(parseInt(p1, 16)));
	return btoa(decodedParams);
}

/** 将字节数格式化为可读文本（B/KB/MB/GB）；precision/compact 供两端保留原显示习惯 */
export function formatFileSize(bytes, { precision = 1, compact = true } = {}) {
	if (!Number.isFinite(bytes) || bytes < 0) return '未知';
	if (bytes === 0) return '0 B';
	const units = ['B', 'KB', 'MB', 'GB'];
	let value = bytes;
	let unit = 0;
	while (value >= 1024 && unit < units.length - 1) {
		value /= 1024;
		unit++;
	}
	if (compact) {
		const text = value >= 100 || unit === 0 ? String(Math.round(value)) : value.toFixed(precision);
		return `${text} ${units[unit]}`;
	}
	return `${parseFloat(value.toFixed(precision))} ${units[unit]}`;
}

/** 读取文件为 data URL */
export function readFileAsDataUrl(file) {
	return new Promise((resolve, reject) => {
		const reader = new FileReader();
		reader.onload = () => resolve(reader.result);
		reader.onerror = () => reject(reader.error || new Error('FileReader error'));
		reader.readAsDataURL(file);
	});
}

/** 读取文件为纯 base64（不带 data URL 前缀） */
export function fileToRawBase64(file) {
	return new Promise((resolve, reject) => {
		const reader = new FileReader();
		reader.onload = () => {
			const result = reader.result;
			if (typeof result !== 'string' || !result.startsWith('data:')) {
				reject(new Error('读取文件失败'));
				return;
			}
			const commaIdx = result.indexOf(',');
			if (commaIdx < 0) {
				reject(new Error('Invalid data URL format'));
				return;
			}
			resolve(result.slice(commaIdx + 1));
		};
		reader.onerror = () => reject(reader.error || new Error('FileReader error'));
		reader.readAsDataURL(file);
	});
}

/** 复制文本到剪贴板 */
export async function copyToClipboard(text) {
	try {
		await navigator.clipboard.writeText(text || '');
		return true;
	} catch {
		return false;
	}
}
