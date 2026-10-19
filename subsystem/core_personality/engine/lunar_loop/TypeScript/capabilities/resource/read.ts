import {
	ResourceEntry,
	ResourceReadResult,
	ResourceSearchHit,
	ResourceSection,
	ResourceStats
} from '../../config/resource';
import { formatHandle, parseHandle } from '../handle';
import { deleteResource } from './manage';

/** 源文件丢失/损坏时的固定提示（句柄条目同时被移除，不滞留失效句柄） */
export const LOST_FILE_NOTICE = '文件损毁或丢失,请更换句柄';
/** 句柄不存在 / 无法解析时的固定提示 */
export const HANDLE_INVALID_NOTICE = '句柄无效,文件不存在';

/**
 * 按句柄取回资源内容（带锚点时按锚点整段展开）
 *
 * blob 已丢失/损坏（result.lost）时顺带移除该句柄条目：失效句柄不再滞留目录，
 * 调用方据 lost 标记返回 LOST_FILE_NOTICE。
 *
 * @param handle 资源句柄
 * @param maxBytes 单次取回字节上限
 * @returns 取回结果；句柄非法或资源不存在返回 null
 */
export function readResource(handle: string, maxBytes?: number): ResourceReadResult | null {
	const parsed = parseHandle(handle);
	if (!parsed) {
		console.error('[资源] 非法句柄:', handle);
		return null;
	}
	try {
		const [result, error] = resourceRead(formatHandle(parsed), maxBytes);
		if (error || !result) {
			console.error('[资源] 取回失败:', error?.message || '返回为空', handle);
			return null;
		}
		// 本体丢失/损坏：立即移除句柄条目（幂等），失效句柄不再滞留目录
		if (result.lost) {
			console.warn('[资源] 本体丢失/损坏，移除句柄:', formatHandle(parsed));
			deleteResource(formatHandle(parsed));
		}
		return result;
	}
	catch (error) {
		console.error('[资源] 取回抛异常:', error);
		return null;
	}
}

/**
 * 读取资源大纲（段落表）；段落表为空且 blob 在位时由 Go 侧现场补建
 *
 * @param handle 资源句柄
 * @returns 段落数组（失败返回空数组）
 */
export function outlineResource(handle: string): ResourceSection[] {
	const parsed = parseHandle(handle);
	if (!parsed) return [];
	try {
		const [sections, , error] = resourceOutline(formatHandle(parsed));
		if (error || !sections) {
			console.error('[资源] 读大纲失败:', error?.message || '返回为空', handle);
			return [];
		}
		return sections;
	}
	catch (error) {
		console.error('[资源] 读大纲抛异常:', error);
		return [];
	}
}

/**
 * 列出资源目录
 *
 * @param kind 类型码过滤（空表示全部）
 * @param limit 条数上限
 * @returns 资源条目数组（失败返回空数组）
 */
export function listResources(kind = '', limit = 50): ResourceEntry[] {
	try {
		const [entries, error] = resourceList(kind, limit);
		if (error || !entries) {
			console.error('[资源] 列目录失败:', error?.message || '返回为空');
			return [];
		}
		return entries;
	}
	catch (error) {
		console.error('[资源] 列目录抛异常:', error);
		return [];
	}
}

/**
 * 跨资源搜索（定位到「哪份资源的哪一节」，取正文请随后 readResource）
 *
 * @param query 关键词
 * @param kind 类型码过滤
 * @param limit 条数上限
 * @returns 命中数组（失败返回空数组）
 */
export function searchResources(query: string, kind = '', limit = 10): ResourceSearchHit[] {
	if (!query || !query.trim()) return [];
	try {
		const [hits, error] = resourceSearch(query.trim(), kind, limit);
		if (error || !hits) {
			console.error('[资源] 搜索失败:', error?.message || '返回为空');
			return [];
		}
		return hits;
	}
	catch (error) {
		console.error('[资源] 搜索抛异常:', error);
		return [];
	}
}

/**
 * 资源目录统计
 *
 * @returns 统计对象；失败返回 null
 */
export function resourceStatistics(): ResourceStats | null {
	try {
		const [stats, error] = resourceStats();
		return error ? null : stats;
	}
	catch (error) {
		console.error('[资源] 统计抛异常:', error);
		return null;
	}
}

/**
 * 在资源目录中按文件名查找（兼容历史按文件名寻址的调用方）
 *
 * @param fileName 文件名（可带 # 前缀，历史形态）
 * @param kind 类型码过滤
 * @returns 命中的资源条目；未找到返回 null
 */
export function findResourceByName(fileName: string, kind = ''): ResourceEntry | null {
	const target = String(fileName || '').replace(/^#/, '').trim();
	if (!target) return null;
	// 目录按更新时间倒序，取首个同名条目；同时容忍哈希名与原始名两种形态
	for (const entry of listResources(kind, 500)) {
		if (entry.name === target) return entry;
	}
	return null;
}

/**
 * 按句柄取资源目录条目（句柄解析失败或资源不存在返回 null）
 *
 * @param handle 资源句柄（可带锚点）
 * @returns 资源条目
 */
export function getResourceEntry(handle: string): ResourceEntry | null {
	const parsed = parseHandle(handle);
	if (!parsed) return null;
	try {
		const [entries, error] = resourceList(parsed.kind, 1000);
		if (error || !entries) return null;
		return entries.find(entry => entry.seq === parsed.seq) ?? null;
	}
	catch (error) {
		console.error('[资源] 查条目抛异常:', error);
		return null;
	}
}

/**
 * 取资源本体的路径来源（相对 LocalDir 的路径，**不加任何协议前缀**）
 *
 * 为什么是裸相对路径而不是 `file://`：
 *   - `file://` 是给 llama-server 媒体管线用的 URI 方案（它的 media-path 指向 multimedia 根），
 *     不是本进程内的路径格式。本进程内直接用路径即可，Go 侧按 LocalDir 解析。
 *   - 系统既有的文件读取端点就是 `GET /file/read/<相对 LocalDir 路径>`
 *     （engine/persistence 的 ReadHandler → ReadFile），同样以「相对 LocalDir 路径」为约定。
 *
 * **只给 Go 侧管线用**（扩散引擎的 initImg、RealESRGAN 超分、动图判定等），
 * 它返回的是**原图**路径。不要拿它拼 `image_url`：图片进思维链只能走
 * `/file/miniature/` 或 `visionRead`（见 docs/adr/0008）。
 *
 * @param handle 资源句柄
 * @returns 相对 LocalDir 的路径；blob 丢失或不存在返回空串
 */
export function resourceSource(handle: string): string {
	const entry = getResourceEntry(handle);
	if (!entry || entry.state !== 'active' || !entry.blob_path) return '';
	return entry.blob_path;
}
