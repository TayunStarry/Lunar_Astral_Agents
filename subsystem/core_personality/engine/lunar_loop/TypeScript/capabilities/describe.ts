import { formatHandle, parseHandle } from './handle';
import { getResourceEntry } from './resource';

// ===== 句柄描述能力 =====
//
// 图片 / 动图 / 视频统一交给 Go 侧描述者（agents/Descriptor）：
//   有描述 → 直接返回；
//   没有描述 → 现场生成并写回资源目录；
//   定向问题 → 只回本次调用，不覆盖共用描述。
//
// 感知者（PerceiverRole）与 JS 视觉描述角色均已移除；
// 文本 / 表格由 inspectTextResource 交给描述者的文件探查链路。

/**
 * 取回资源的描述文本
 *
 * 顺序：目录摘要命中 → 图片/动图/视频交给描述者 → 音频返回已有转写。
 *
 * @param handle 资源句柄（@I.007 / @V.001）
 * @param question 可选定向问题
 * @returns 描述文本（失败返回空串）
 */
export function describeResource(handle: string, question = ''): string {
	const parsed = parseHandle(handle);
	if (!parsed) return '';
	const normalized = formatHandle(parsed);
	const cached = getResourceEntry(normalized)?.summary || '';

	// 无定向问题且已有描述：直接命中，零模型调用
	if (cached && !question) return cached;

	// 图片 / 动图 / 视频：有描述由 Go 侧直接返回，没有则现场观看并入库
	if (parsed.kind === 'I' || parsed.kind === 'V') {
		return describeMedia(normalized, question, cached);
	}

	// 音频：只有转写摘要，没有可现场观看的本体
	if (parsed.kind === 'A') return cached;

	return cached;
}

/**
 * 文本句柄探查：把自然语言需求交给 Go 侧描述者
 *
 * @param handle 文本资源句柄（@D.xxx / @T.xxx）
 * @param request 自然语言需求
 * @returns 描述者答复；失败返回空串（调用方给出统一失败提示）
 */
export function inspectTextResource(handle: string, request: string): string {
	try {
		const [text, error] = descriptorInspect(handle, request);
		if (error) {
			console.error('[描述者] 文件探查失败:', error?.message ?? String(error));
			return '';
		}
		return String(text || '').trim();
	}
	catch (error) {
		console.error('[描述者] 文件探查抛异常:', error);
		return '';
	}
}

/** 影像描述：Go 侧描述者已有描述直接返回，否则现场观看并写回 */
function describeMedia(handle: string, question: string, cached: string): string {
	try {
		const [text, error] = descriptorDescribeMedia(handle, question);
		if (error) {
			console.error('[描述者] 影像描述失败:', error?.message ?? String(error));
			return cached;
		}
		const desc = String(text || '').trim();
		return desc || cached;
	}
	catch (error) {
		console.error('[描述者] 影像描述抛异常:', error);
		return cached;
	}
}
