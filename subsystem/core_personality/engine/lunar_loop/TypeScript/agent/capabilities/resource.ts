import {
	ResourceAdmitInput,
	ResourceAdmitResult,
	ResourceEntry,
	ResourceReadResult,
	ResourceSection,
	ResourceSearchHit,
	ResourceStats
} from '../../config/resource';
import { formatHandle, parseHandle } from './handle';

// ===== 统一资源归档层的能力封装（TypeScript 侧唯一入口） =====
//
// 权威存储与句柄分配在 Go 侧（FileManager/module + engine/archive.go 适配器）。
// 本模块只做三件事：包错误、做缓存、把「丢失」翻译成可用的降级文本。
//
// 禁止在其它模块直接调用 resourceAdmit / resourceRead 等全局函数。

/** 资源句柄语法错误（调用方据此区分「参数不合法」与「运行时失败」） */
export class HandleError extends Error {
	constructor(message: string) {
		super(message);
		this.name = 'HandleError';
	}
}

/** base64 编码（本地实现，避免依赖运行时 atob/btoa 的可用性） */
const BASE64_ALPHABET = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/';

/**
 * 把字节数组编码为 base64 字符串
 *
 * @param bytes 原始字节
 * @returns base64 文本
 */
export function bytesToBase64(bytes: Uint8Array): string {
	let out = '';
	for (let i = 0; i < bytes.length; i += 3) {
		const b0 = bytes[i];
		const b1 = i + 1 < bytes.length ? bytes[i + 1] : 0;
		const b2 = i + 2 < bytes.length ? bytes[i + 2] : 0;
		out += BASE64_ALPHABET[b0 >> 2];
		out += BASE64_ALPHABET[((b0 & 0x03) << 4) | (b1 >> 4)];
		out += i + 1 < bytes.length ? BASE64_ALPHABET[((b1 & 0x0f) << 2) | (b2 >> 6)] : '=';
		out += i + 2 < bytes.length ? BASE64_ALPHABET[b2 & 0x3f] : '=';
	}
	return out;
}

/**
 * 统一入库：归一来源 → 落盘 → 登记资源目录 → 返回句柄
 *
 * @param input 入库入参（sourceKind 决定归一分支）
 * @returns 入库结果；失败返回 null（已打印错误日志）
 */
export function admitResource(input: ResourceAdmitInput): ResourceAdmitResult | null {
	try {
		// 字节入参统一走 base64 文本通道（goja 侧按字符串解码，避免二进制跨运行时失真）
		const payload = input.data instanceof Uint8Array
			? { ...input, data: bytesToBase64(input.data) }
			: input;
		const [result, error] = resourceAdmit(payload);
		if (error || !result) {
			console.error('[资源] 入库失败:', error?.message || '返回为空', input.sourceKind, input.name || input.source || '');
			return null;
		}
		return result;
	}
	catch (error) {
		console.error('[资源] 入库抛异常:', error);
		return null;
	}
}

/**
 * 入库前端已落盘的本地文件（零拷贝路径：只登记不复制）
 *
 * @param source 来源（相对 LocalDir 路径 / 本机服务地址 / file:// 前缀）
 * @param name 原始文件名
 * @param origin 来源语义
 * @param retain 保留等级建议
 * @returns 入库结果；失败返回 null
 */
export function admitLocalFile(
	source: string,
	name: string,
	origin: ResourceAdmitInput['origin'] = 'client-upload',
	retain?: ResourceAdmitInput['retain']
): ResourceAdmitResult | null {
	return admitResource({ sourceKind: 'path', source, name, origin, retain });
}

/**
 * 入库远程地址（QQ 等渠道仅给 URL 的图片 / 视频 / 音频）
 *
 * @param url 远程地址
 * @param name 建议文件名（可空：按 URL 路径推断）
 * @param origin 来源语义（默认 channel）
 * @param retain 保留等级建议
 * @returns 入库结果；失败返回 null
 */
export function admitRemoteURL(
	url: string,
	name = '',
	origin: ResourceAdmitInput['origin'] = 'channel',
	retain?: ResourceAdmitInput['retain']
): ResourceAdmitResult | null {
	return admitResource({ sourceKind: 'url', source: url, name, origin, retain });
}

/**
 * 入库字节 / data URI（工具产出、转发内联）
 *
 * @param data 原始字节或 base64 字符串
 * @param name 建议文件名（决定扩展名与类型码）
 * @param origin 来源语义（默认 tool-output）
 * @param retain 保留等级建议
 * @returns 入库结果；失败返回 null
 */
export function admitBytes(
	data: string | Uint8Array,
	name: string,
	origin: ResourceAdmitInput['origin'] = 'tool-output',
	retain?: ResourceAdmitInput['retain']
): ResourceAdmitResult | null {
	return admitResource({ sourceKind: 'bytes', data, name, origin, retain });
}

/**
 * 入库 data URI（内联图片直传场景）
 *
 * @param dataURI data:image/...;base64,... 形态
 * @param name 建议文件名
 * @param origin 来源语义
 * @param retain 保留等级建议
 * @returns 入库结果；失败返回 null
 */
export function admitDataURI(
	dataURI: string,
	name: string,
	origin: ResourceAdmitInput['origin'] = 'tool-output',
	retain?: ResourceAdmitInput['retain']
): ResourceAdmitResult | null {
	return admitResource({ sourceKind: 'data', source: dataURI, name, origin, retain });
}

/**
 * 按句柄取回资源内容（带锚点时按锚点整段展开）
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
 * 写入资源摘要 / 描述（图片视觉描述、文档概要）
 *
 * @param id 资源目录主键
 * @param summary 摘要文本
 * @param tags 标签（省略表示保留原标签）
 */
export function setResourceSummary(id: number, summary: string, tags?: string[]): void {
	try {
		const [ok, error] = resourceSetSummary(id, summary, tags ?? []);
		if (error || !ok) console.error('[资源] 写摘要失败:', error?.message || '返回失败');
	}
	catch (error) {
		console.error('[资源] 写摘要抛异常:', error);
	}
}

/**
 * 全量替换资源段落表
 *
 * @param id 资源目录主键
 * @param sections 段落草稿
 * @returns 是否成功
 */
export function setResourceSections(id: number, sections: ResourceSectionDraftInput[]): boolean {
	try {
		const [ok, error] = resourceSetSections(id, sections);
		if (error || !ok) {
			console.error('[资源] 写段落表失败:', error?.message || '返回失败');
			return false;
		}
		return true;
	}
	catch (error) {
		console.error('[资源] 写段落表抛异常:', error);
		return false;
	}
}

/** 段落草稿入参（字段名与 Go 侧一致） */
export type ResourceSectionDraftInput = {
	ordinal: number;
	level: number;
	title: string;
	path: string;
	lineStart: number;
	lineEnd: number;
	chars: number;
};

/**
 * 触发内容仓库回收（只删 blob、保留目录行；句柄仍可解析并降级为描述）
 *
 * @param force 忽略配额直接按等级回收
 * @returns 回收统计；失败返回 null
 */
export function collectResources(force = false): {
	scanned: number;
	collected: number;
	freed_bytes: number;
	remaining_bytes: number;
	over_quota: boolean;
} | null {
	try {
		const [result, error] = resourceCollect({ force });
		if (error || !result) {
			console.error('[资源] 回收失败:', error?.message || '返回为空');
			return null;
		}
		return result;
	}
	catch (error) {
		console.error('[资源] 回收抛异常:', error);
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
 *     （file_manager 的 ReadHandler → module.ReadFile），同样以「相对 LocalDir 路径」为约定。
 *
 * **只给 Go 侧管线用**（扩散引擎的 initImg、RealESRGAN 超分、动图判定等），
 * 它返回的是**原图**路径。不要拿它拼 `image_url`：图片进思维链只能走
 * `/file/read-multimodal/` 或 `visionRead`（见 docs/adr/0008）。
 *
 * @param handle 资源句柄
 * @returns 相对 LocalDir 的路径；blob 丢失或不存在返回空串
 */
export function resourceSource(handle: string): string {
	const entry = getResourceEntry(handle);
	if (!entry || entry.state !== 'active' || !entry.blob_path) return '';
	return entry.blob_path;
}

/**
 * 显式删除资源（目录行 + 独占 blob；仍被其它目录行引用时保留实体）
 *
 * @param handle 资源句柄
 * @returns 是否删除成功
 */
export function deleteResource(handle: string): boolean {
	const parsed = parseHandle(handle);
	if (!parsed) return false;
	try {
		const [ok, error] = resourceDelete(formatHandle(parsed));
		if (error || !ok) {
			console.error('[资源] 删除失败:', error?.message || '返回失败', handle);
			return false;
		}
		return true;
	}
	catch (error) {
		console.error('[资源] 删除抛异常:', error);
		return false;
	}
}

/**
 * 入库图片并按**内容哈希**去重（同一张图反复发送时复用既有句柄）
 *
 * 为什么需要去重：入库层默认「每次入库一行、序号永不复用」，同一张图每发一次就会多一个
 * 目录行 —— 库里出现过同一张图占 `@I.002`/`@I.003`/`@I.004` 三条的情况。图片没有文件名
 * 这类业务身份，内容哈希就是它唯一的身份。
 *
 * **去重在 Go 侧完成**（`dedup: true`）：哈希与二进制都在入库层处理，JS 侧不参与。
 * 这是关键教训 —— 曾有一版在 JS 里用 `atob` 解码 base64 再取 `charCodeAt` 还原字节，
 * 而 `atob` 适配器返回的是 Go 字符串，非法 UTF-8 字节被逐字节替换为 U+FFFD（低字节 0xFD），
 * 导致所有经该路径入库的图片变成无法解码的垃圾数据。
 *
 * @param imageData 图片数据 URI（前端压缩后的图片）
 * @param name 建议文件名（决定扩展名与类型码）
 * @param origin 来源语义
 * @returns 入库结果（命中时复用既有句柄与实体）；失败返回 null
 */
export function admitImageDataURI(
	imageData: string,
	name: string,
	origin: ResourceAdmitInput['origin'] = 'client-upload'
): ResourceAdmitResult | null {
	if (!imageData) return null;
	// data URI 直接交给入库层解码：Go 侧解码是二进制安全的，且 hash 在那里一并算出
	if (imageData.startsWith('data:')) {
		return admitResource({ sourceKind: 'data', source: imageData, name, origin, kindHint: 'I', dedup: true });
	}
	// 非 data URI（已是本地路径或远程地址）：按来源归一，同样开启内容去重
	const sourceKind = /^https?:\/\//i.test(imageData) ? 'url' : 'path';
	return admitResource({ sourceKind, source: imageData, name, origin, kindHint: 'I', dedup: true });
}

/**
 * 按文件名把文件收进资源归档层（工作区文件与 LocalDir 文件都支持）
 *
 * 解析顺序（与用户心智一致：先系统资料，再项目文件）：
 *   1. 工作区相对路径（用户项目目录内的源码 / 文档）
 *   2. LocalDir 相对路径（系统数据目录内的文件）
 *
 * 资源名以**引用时书写的相对路径**为准，而不是裸文件名：项目里常有多个同名文件
 * （如各子系统的 build.ps1），只用裸名会让目录里出现多条无法区分的记录。
 *
 * 为什么需要它：对话里写 `@AGENTS.md` 这类引用时，文件可能从未被上传过，
 * 归档里自然没有它。此函数负责「按需收录」——把用户点名的文件读取进来建立存档副本，
 * 之后即可持久引用。工作区来源在 Go 侧只复制/硬链接，绝不移动或删除原文件。
 *
 * @param fileName 相对路径或文件名（可带目录前缀）
 * @returns 入库结果；两处都找不到或类型不受支持时返回 null
 */
export function admitDocumentByName(fileName: string): ResourceAdmitResult | null {
	const target = String(fileName || '').trim().replace(/^[#@]/, '');
	if (!target) return null;
	// 资源名保留用户书写的相对路径，避免同名文件在目录里无法区分
	const name = target.replace(/^\.\//, '');
	// 1. 工作区（项目目录）
	const fromWorkspace = admitResource({ sourceKind: 'workspace', source: target, name, origin: 'client-upload', retain: 'normal' });
	if (fromWorkspace) return fromWorkspace;
	// 2. LocalDir（系统数据目录）
	return admitResource({ sourceKind: 'path', source: target, name, origin: 'client-upload', retain: 'normal' });
}

/**
 * 把「资源不存在 / blob 丢失」翻译为模型可见的降级说明
 *
 * @param handle 资源句柄
 * @param reason 具体原因（如「图片本体已被回收」）
 * @returns 可直接拼进上下文的说明文本
 */
export function degradeNotice(handle: string, reason: string): string {
	const parsed = parseHandle(handle);
	const label = parsed ? formatHandle(parsed) : handle;
	return `${label}（${reason}）`;
}
