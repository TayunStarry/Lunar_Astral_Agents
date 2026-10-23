// ===== 对话缓存编解码：图片地址 / base64 媒体落盘 =====

/** 视觉读取端点（思维链唯一允许的静态图消费入口） */
const VISION_READ_PATH = '/file/miniature/';
/** 前端可查看原图的读取端点 */
const FILE_READ_PATH = '/file/read/';

/** 去掉地址中的查询串，取可访问的路径部分 */
function pathPartOf(source: string): string {
	return String(source || '').split('?')[0].split('#')[0];
}

/** 转成前端可查看原图的地址（/file/read/）；无法识别时原样返回 */
export function toDisplaySource(source: string): string {
	const raw = String(source || '');
	if (!raw) return '';
	if (raw.startsWith('data:') || raw.startsWith('blob:')) return raw;
	const path = pathPartOf(raw);
	const localIndex = path.indexOf(FILE_READ_PATH);
	if (localIndex >= 0) return path.slice(localIndex);
	const visionIndex = path.indexOf(VISION_READ_PATH);
	if (visionIndex >= 0) {
		return FILE_READ_PATH + path.slice(visionIndex + VISION_READ_PATH.length);
	}
	if (path.startsWith('multimedia/')) return FILE_READ_PATH + path;
	return raw;
}

/** 转成思维链视觉读取地址；非本地图片返回空串（避免原图进入模型） */
export function toVisionSource(source: string): string {
	const raw = String(source || '');
	if (!raw || raw.startsWith('data:') || raw.startsWith('blob:')) return '';
	const path = pathPartOf(raw);
	const visionIndex = path.indexOf(VISION_READ_PATH);
	if (visionIndex >= 0) return url()[0] + path.slice(visionIndex);
	const readIndex = path.indexOf(FILE_READ_PATH);
	if (readIndex >= 0) {
		const rel = path.slice(readIndex + FILE_READ_PATH.length);
		if (!rel.startsWith('multimedia/')) return '';
		return `${url()[0]}${VISION_READ_PATH}${rel}`;
	}
	if (path.startsWith('multimedia/')) return `${url()[0]}${VISION_READ_PATH}${path}`;
	return '';
}

/** 本地绝对/相对路径 → /file/read/ 地址（仅接受 multimedia/ 下的文件） */
export function localPathToReadURL(source: string): string {
	if (!source) return '';
	const normalized = String(source).replace(/\\/g, '/').split('?')[0];
	const marker = normalized.toLowerCase().lastIndexOf('/multimedia/');
	const rel = marker >= 0
		? normalized.slice(marker + 1)
		: (normalized.toLowerCase().startsWith('multimedia/') ? normalized : '');
	return rel ? FILE_READ_PATH + rel : '';
}

/** 从地址/路径中取文件名 */
export function fileNameOf(source: string): string {
	const path = pathPartOf(String(source || ''));
	const name = path.slice(path.lastIndexOf('/') + 1);
	return name || '文件';
}

/** 用户名前缀拆分结果 */
export interface SplitUserPrefix {
	/** 前缀中的用户名；没有前缀时为空串 */
	sender: string;
	/** 去掉前缀后的正文 */
	text: string;
}

/**
 * 拆出私聊/前端消息开头的 `[用户: 名字]: ` 前缀。
 *
 * 群聊消息形如 `[群聊: …][用户: …]: 正文`，不以 `[用户:` 开头，因此不会被拆开，
 * 群名与人名继续保留在正文里。
 */
export function splitUserPrefix(text: string): SplitUserPrefix {
	const raw = String(text || '');
	const match = /^\[用户:\s*([^\]]*)\]\s*[:：]\s*/u.exec(raw);
	if (!match) return { sender: '', text: raw };
	return { sender: match[1].trim(), text: raw.slice(match[0].length) };
}

/** 按 base64 头部猜图片 MIME（用于无 data URI 前缀的表情包/截图） */
function guessImageMime(base64: string): string {
	const head = String(base64 || '').slice(0, 16);
	if (head.startsWith('/9j/')) return 'image/jpeg';
	if (head.startsWith('iVBOR')) return 'image/png';
	if (head.startsWith('R0lG')) return 'image/gif';
	if (head.startsWith('UklGR')) return 'image/webp';
	if (head.startsWith('Qk')) return 'image/bmp';
	return 'image/png';
}

/**
 * Base64（可带 data URI 前缀）写入 multimedia/ 内容仓库，返回 /file/read/ 地址
 *
 * 不再使用 multimedia/cache 兼容目录：图片与语音与其它媒体共用内容寻址树，
 * 由 Go 侧按内容哈希去重并登记句柄。
 */
export function saveBase64Media(base64: string, mime: string, kind: string): string {
	const raw = String(base64 || '');
	if (!raw) return '';
	const match = /^data:([^;,]+);base64,(.*)$/s.exec(raw);
	const data = match ? match[2] : raw;
	const contentType = match ? match[1] : (kind === 'voice' ? mime : guessImageMime(data));
	try {
		// 必须用 base64Decode 取字节：atob 返回字符串，二进制里的非 UTF-8 字节会被
		// 替换成 U+FFFD，再按 charCodeAt 写盘就变成 0xFD，图片/音频直接损坏。
		const [decoded, decodeErr] = base64Decode(data);
		if (decodeErr || !decoded) {
			console.warn('[对话缓存] base64 解码失败:', decodeErr);
			return '';
		}
		const bytes = new Uint8Array(decoded);
		const extMap: Record<string, string> = {
			'image/jpeg': 'jpg', 'image/png': 'png', 'image/webp': 'webp', 'image/gif': 'gif', 'image/bmp': 'bmp',
			'audio/wav': 'wav', 'audio/x-wav': 'wav', 'audio/mpeg': 'mp3', 'audio/mp3': 'mp3', 'audio/flac': 'flac', 'audio/ogg': 'ogg',
		};
		const ext = extMap[contentType] || 'bin';
		const name = `multimedia/${kind}_${Date.now()}_${Math.random().toString(36).slice(2, 8)}.${ext}`;
		const [, , error] = saveFile(name, true, bytes);
		if (error) {
			console.warn('[对话缓存] 媒体落盘失败:', error);
			return raw.startsWith('data:') ? raw : '';
		}
		return FILE_READ_PATH + name;
	}
	catch (error) {
		console.warn('[对话缓存] 媒体解码失败:', error);
		return '';
	}
}
