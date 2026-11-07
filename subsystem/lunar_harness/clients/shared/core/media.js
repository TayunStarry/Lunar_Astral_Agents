/**
 * 星月智能 · 多模态信封构造（桌面端 / 移动端共用）
 *
 * 与 Go 侧 engine/mediaobject.Envelope 保持一致：六类信封各自一个专用字段，
 * 字段名与 type 同名，其余专用字段必须为空。写入侧只按来源方声明
 * （浏览器给出的 file.type / 文件名扩展名）填写 source / url / mime / name，
 * 接收侧只做校验与拒绝，不根据 URL 或内容反推类型。
 */
import { getFileExtension } from './files.js';
import { ENDPOINTS } from './constants.js';

/** 前端上传来源标记（与 Go 侧 SourceClientUpload 对应） */
export const CLIENT_UPLOAD_SOURCE = 'client_upload';

/** 视频平台网页声明（与 Go 侧 WebPlatformVideo 对应；普通网页不声明） */
export const WEB_PLATFORM_VIDEO = 'web_video';

/** 扩展名 → MIME（File.type 缺失或为通用类型时用于补全声明） */
const MIME_BY_EXTENSION = {
	jpg: 'image/jpeg', jpeg: 'image/jpeg', png: 'image/png', gif: 'image/gif',
	webp: 'image/webp', bmp: 'image/bmp', svg: 'image/svg+xml', avif: 'image/avif',
	heic: 'image/heic', heif: 'image/heif',
	mp4: 'video/mp4', mov: 'video/quicktime', avi: 'video/x-msvideo', mkv: 'video/x-matroska',
	flv: 'video/x-flv', wmv: 'video/x-ms-wmv', m4v: 'video/x-m4v', mpeg: 'video/mpeg',
	mpg: 'video/mpeg', '3gp': 'video/3gpp',
	mp3: 'audio/mpeg', wav: 'audio/wav', ogg: 'audio/ogg', oga: 'audio/ogg', m4a: 'audio/mp4',
	aac: 'audio/aac', flac: 'audio/flac', opus: 'audio/opus', amr: 'audio/amr',
	wma: 'audio/x-ms-wma', aiff: 'audio/aiff', webm: 'audio/webm', silk: 'audio/silk',
	txt: 'text/plain', log: 'text/plain', md: 'text/markdown', markdown: 'text/markdown',
	csv: 'text/csv', html: 'text/html', htm: 'text/html', css: 'text/css',
	js: 'text/javascript', mjs: 'text/javascript', ts: 'text/typescript', json: 'application/json',
	xml: 'application/xml', yaml: 'application/yaml', yml: 'application/yaml',
	pdf: 'application/pdf',
	doc: 'application/msword',
	docx: 'application/vnd.openxmlformats-officedocument.wordprocessingml.document',
	xls: 'application/vnd.ms-excel',
	xlsx: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
	ppt: 'application/vnd.ms-powerpoint',
	pptx: 'application/vnd.openxmlformats-officedocument.presentationml.presentation',
	zip: 'application/zip', '7z': 'application/x-7z-compressed', rar: 'application/vnd.rar',
	gz: 'application/gzip', tar: 'application/x-tar', apk: 'application/vnd.android.package-archive'
};

/** 具备动画能力的图片类型（判不出来时按静态图发送，不伪造类型） */
const ANIMATED_IMAGE_MIMES = new Set(['image/gif', 'image/apng']);
const ANIMATED_IMAGE_EXTENSIONS = new Set(['gif', 'apng']);

/** 输入框文本中的 http(s) 链接（允许中文路径，但遇到中文标点即截断） */
const WEB_URL_PATTERN = /https?:\/\/[^\s<>"'`，。；：！？、（）【】《》〈〉「」『』]+/gu;

/** 链接末尾常被一并抓进来的标点 */
const TRAILING_PUNCTUATION = /[.,;:!?，。；：！？、）】》」』"'`]+$/;

/** 图片信封：静态图与动图按浏览器声明的 File.type 与文件名分开 */
export function imageEnvelope(file, url) {
	const type = isAnimatedImage(file) ? 'image_animated' : 'image_static';
	return { type, [type]: buildRef(file, { url }) };
}

/** 视频信封 */
export function videoEnvelope(file, url) {
	return { type: 'video', video: buildRef(file, { url }) };
}

/** 文件信封：去重交给后端内容哈希，name / bytes 保留原始文件名与大小 */
export function fileEnvelope(file, url) {
	const payload = buildRef(file, { url });
	if (Number.isFinite(file?.size)) payload.bytes = file.size;
	return { type: 'file', file: payload };
}

/** 音频信封：内联数据以 data URI 放在 url 字段，不使用已废弃的 input_audio */
export function audioEnvelope(file, data, format) {
	const mime = declaredMIME(file) || MIME_BY_EXTENSION[String(format || '').toLowerCase()] || '';
	const mediaType = mime || 'application/octet-stream';
	return {
		type: 'audio',
		audio: buildRef(file, { url: `data:${mediaType};base64,${data}`, mime })
	};
}

/**
 * 从用户输入中抽取 URL，产出 webpage 信封，正文保持纯文本。
 * 域名表取自后端 engine/mediaobject.VideoPlatformHosts，前端不另维护清单；
 * 清单不可用时保守回退为纯文本，避免把视频平台误声明成普通网页。
 * @param {string} text 用户输入的原始文本
 * @returns {Promise<{text: string, envelopes: object[]}>}
 */
export async function extractWebpageEnvelopes(text) {
	const raw = String(text || '');
	if (!raw || !raw.includes('://')) return { text: raw, envelopes: [] };
	const hosts = await ensureVideoPlatformHosts();
	if (!hosts) return { text: raw, envelopes: [] };
	const urls = [];
	const remaining = raw.replace(WEB_URL_PATTERN, match => {
		const url = trimTrailingPunctuation(match);
		if (!isWebURL(url)) return match;
		urls.push(url);
		return '';
	});
	if (!urls.length) return { text: raw, envelopes: [] };
	const envelopes = [...new Set(urls)].map(url => {
		const webpage = { source: CLIENT_UPLOAD_SOURCE, url };
		if (isVideoPlatformURL(url, hosts)) webpage.platform = WEB_PLATFORM_VIDEO;
		return { type: 'webpage', webpage };
	});
	const cleaned = remaining
		.replace(/[ \t]{2,}/g, ' ')
		.replace(/[ \t]+([，。；：！？、）】》」』])/g, '$1')
		.replace(/\n{3,}/g, '\n\n')
		.trim();
	return { text: cleaned, envelopes };
}

/** 共享视频平台域名表（与后端同源），失败时返回 null 且允许下次重试 */
let videoPlatformHosts = null;
let videoPlatformHostsRequest = null;

/** 拉取共享视频平台域名表 */
export function ensureVideoPlatformHosts() {
	if (videoPlatformHosts) return Promise.resolve(videoPlatformHosts);
	if (!videoPlatformHostsRequest) {
		videoPlatformHostsRequest = fetch(ENDPOINTS.videoPlatformHosts, { cache: 'no-store' })
			.then(response => (response.ok ? response.json() : null))
			.then(data => {
				const hosts = Array.isArray(data?.video_platform_hosts) ? data.video_platform_hosts : null;
				if (!hosts) return null;
				videoPlatformHosts = hosts
					.map(host => String(host || '').trim().toLowerCase())
					.filter(Boolean);
				return videoPlatformHosts;
			})
			.catch(() => null)
			.finally(() => {
				videoPlatformHostsRequest = null;
			});
	}
	return videoPlatformHostsRequest;
}

/** 主机是否命中共享视频平台域名表 */
export function isVideoPlatformURL(url, hosts = videoPlatformHosts) {
	let host = '';
	try {
		host = new URL(url).hostname.toLowerCase();
	} catch {
		return false;
	}
	return (hosts || []).some(domain => host === domain || host.endsWith(`.${domain}`));
}

/** 公共引用字段：source + url 二选一 + 声明的 mime / name */
function buildRef(file, { url, mime } = {}) {
	const ref = { source: CLIENT_UPLOAD_SOURCE };
	if (url) ref.url = url;
	const declared = mime === undefined ? declaredMIME(file) : mime;
	if (declared) ref.mime = declared;
	const name = String(file?.name || '');
	if (name) ref.name = name;
	return ref;
}

/** 浏览器声明的 MIME；缺失或为通用类型时按扩展名映射补全 */
function declaredMIME(file) {
	const mime = baseMIME(file);
	if (mime && mime !== 'application/octet-stream') return mime;
	return MIME_BY_EXTENSION[getFileExtension(file?.name)] || '';
}

function baseMIME(file) {
	return String(file?.type || '').toLowerCase().split(';')[0].trim();
}

/** 动图判定：浏览器声明优先，其次看扩展名；两者都判不出时按静态图 */
function isAnimatedImage(file) {
	const mime = baseMIME(file);
	if (mime.startsWith('image/')) return ANIMATED_IMAGE_MIMES.has(mime);
	return ANIMATED_IMAGE_EXTENSIONS.has(getFileExtension(file?.name));
}

function isWebURL(raw) {
	try {
		const parsed = new URL(raw);
		return (parsed.protocol === 'http:' || parsed.protocol === 'https:') && !!parsed.hostname;
	} catch {
		return false;
	}
}

function trimTrailingPunctuation(raw) {
	let url = raw;
	for (;;) {
		const trimmed = url.replace(TRAILING_PUNCTUATION, '');
		if (trimmed !== url) {
			url = trimmed;
			continue;
		}
		if (url.endsWith(')') && count(url, '(') < count(url, ')')) {
			url = url.slice(0, -1);
			continue;
		}
		if (url.endsWith(']') && count(url, '[') < count(url, ']')) {
			url = url.slice(0, -1);
			continue;
		}
		return url;
	}
}

function count(text, char) {
	return text.split(char).length - 1;
}
