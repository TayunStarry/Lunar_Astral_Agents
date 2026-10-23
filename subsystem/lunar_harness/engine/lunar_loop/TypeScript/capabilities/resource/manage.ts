import { formatHandle, parseHandle } from '../handle';

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
