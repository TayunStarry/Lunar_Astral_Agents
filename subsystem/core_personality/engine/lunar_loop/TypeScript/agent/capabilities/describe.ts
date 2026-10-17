import { ModelBuilder } from '../base/builder';
import { sanitizeImageDesc } from './image-tag';
import { formatHandle, parseHandle } from './handle';
import { getResourceEntry, readResource, setResourceSummary } from './resource';

// ===== 资源描述能力（图片视觉描述 + 视频/音频理解结论） =====
//
// 句柄取用链路的降级层：当资源本体不可直读（图片出窗、blob 被回收）时，
// 由视觉模型现场生成或从资源目录摘要字段取回描述。
//
// 角色注入方式与 image-recall 的 registerImageDescribeRole 一致：
// 由 roles.ts 在模块初始化时注入描述角色单例，避免 capabilities → roles 静态循环引用。

/** 描述角色（视觉模型） */
let describeRole: ModelBuilder | null = null;
/** 理解角色（视频/音频感知者） */
let perceiveRole: ModelBuilder | null = null;

/** 注入描述角色（roles.ts 调用） */
export function registerDescribeRole(role: ModelBuilder): void {
	describeRole = role;
}

/** 注入理解角色（roles.ts 调用；缺省时视频/音频仅返回目录摘要） */
export function registerPerceiveRole(role: ModelBuilder): void {
	perceiveRole = role;
}

/** 资源目录条目的摘要（无条目返回空串） */
function storedSummary(handle: string): string {
	const entry = getResourceEntry(handle);
	return entry?.summary || '';
}

/**
 * 生成或取回资源的描述文本
 *
 * 顺序：目录摘要命中 → 视觉/理解模型现场生成并回写 → 空串（调用方自行降级）。
 * 定向问题只回本次调用，不写共用摘要，避免窄问题覆盖通用描述。
 *
 * @param handle 资源句柄（形如 @I.007 / @V.001）
 * @param question 可选定向问题
 * @returns 描述文本（失败返回空串）
 */
export function describeResource(handle: string, question = ''): string {
	const parsed = parseHandle(handle);
	if (!parsed) return '';
	const normalized = formatHandle(parsed);
	const cached = storedSummary(normalized);
	// 无定向问题且已有摘要：直接命中，零模型调用
	if (cached && !question) return sanitizeImageDesc(cached, 200);

	// 图片走视觉描述角色
	if (parsed.kind === 'I') {
		const source = resourceSource(normalized);
		if (!describeRole || !source) return sanitizeImageDesc(cached, 200);
		try {
			const instruction = question
				? `用户想知道：${question}。请观察这张图片，用不超过100字的中文描述其中与该问题相关的关键信息，直接输出描述本身。`
				: '用一句不超过20字的中文描述这张图片的主体与场景，只输出描述本身。';
			describeRole.coverContext({
				role: 'user',
				content: [
					{ type: 'text', text: instruction },
					{ type: 'image_url', image_url: { url: source } }
				]
			});
			const response = describeRole.run([], []);
			const desc = sanitizeImageDesc(response.body?.choices?.[0]?.message?.content || '', 200);
			if (desc && !question) {
				const entry = getResourceEntry(normalized);
				if (entry) setResourceSummary(entry.id, desc);
			}
			return desc || sanitizeImageDesc(cached, 200);
		}
		catch (error) {
			console.error(`[资源描述] ${normalized} 生成失败:`, error);
			return sanitizeImageDesc(cached, 200);
		}
	}

	// 视频 / 音频：目录摘要优先（由感知者写入），无摘要时返回空串由调用方兜底
	return sanitizeImageDesc(cached, 200);
}

/**
 * 取资源本体的**给模型看的**来源：图片一律转成 data URI
 *
 * 为什么不能直接给相对路径：这个来源是要塞进 `image_url.url` 交给推理服务的，
 * 而服务端只认它自己能解析的形态（data URI / http URL / 相对 `--media-path` 的 file://），
 * 不认「相对 LocalDir 的裸路径」。此前这里直接返回 `blob_path`，导致 `describeResource`
 * 对归档图片一直取不到内容（库里所有图片摘要都是空的）。
 *
 * 图片本体经 `readResource` 取回 base64（二进制资源走 base64 通道），此处拼成 data URI。
 *
 * @param handle 资源句柄
 * @returns data URI；取不到返回空串
 */
function resourceSource(handle: string): string {
	const result = readResource(handle, 8 * 1024 * 1024);
	if (!result || result.lost || !result.base64) return '';
	// 截断过的二进制不能当图片用（半个文件解码必然失败），宁可返回空让调用方降级
	if (result.truncated) return '';
	const mime = result.mime || 'image/png';
	return `data:${mime};base64,${result.base64}`;
}
