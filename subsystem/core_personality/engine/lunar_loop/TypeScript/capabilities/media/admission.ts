import { admitResource, getResourceEntry, setResourceSummary } from '../resource';

/** 资源化后的媒体：句柄 + 目录主键 */
export type AdmittedMedia = { handle: string; id: number };

/**
 * 把媒体收进资源目录
 *
 * @param source 媒体来源（路径 / 服务地址 / 远程 URL / data URI）
 * @param name 建议文件名（决定类型码与扩展名）
 * @param origin 来源语义
 * @param kindHint 类型码建议值
 * @returns 句柄与目录主键；失败返回 null
 */
export function admitMedia(
	source: string,
	name: string,
	origin: 'client-upload' | 'channel' | 'tool-output' | 'engine-temp',
	kindHint?: 'D' | 'I' | 'V' | 'A'
): AdmittedMedia | null {
	if (!source) return null;
	const lower = source.toLowerCase();
	const sourceKind = lower.startsWith('data:') ? 'data' : (/^https?:\/\//i.test(source) ? 'url' : 'path');
	const result = admitResource({ sourceKind, source, name, origin, kindHint });
	return result ? { handle: result.handle, id: result.id } : null;
}

/** 把媒体的理解结论写回资源目录（供 use_resource 复用） */
export function storeMediaSummary(media: AdmittedMedia, summary: string): void {
	if (!media || !summary || !summary.trim()) return;
	setResourceSummary(media.id, summary.trim().slice(0, 1000));
}

/**
 * 判断资源是否已有理解结论（避免重复生成描述）
 *
 * @param media 入库后的媒体
 * @returns 已有的结论文本；无则空串
 */
export function existingSummary(media: AdmittedMedia): string {
	const entry = getResourceEntry(media.handle);
	return entry?.summary || '';
}

/** 按来源推断文件名（媒体资源命名用；无法推断时按类型码兜底） */
export function mediaName(source: string, fallbackExt: string): string {
	const lower = (source || '').toLowerCase();
	const dataMatch = /^data:(image|video|audio)\/(\w+)[;,]/.exec(lower);
	if (dataMatch) return `media.${dataMatch[2] === 'jpeg' ? 'jpg' : dataMatch[2]}`;
	const path = lower.split('?')[0].split('#')[0];
	const base = path.slice(path.lastIndexOf('/') + 1);
	if (base && /\.[a-z0-9]{2,5}$/.test(base)) return base;
	return `media${fallbackExt}`;
}
