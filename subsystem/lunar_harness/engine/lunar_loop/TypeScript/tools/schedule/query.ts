// ============================================================
//  计划表 — 查询引擎（关键词数组 × 时间范围，按相关度排序）
// ============================================================

import type { ScheduleItem } from './defs';
import { dailyTaskTime, parseDateRange } from './time';

/** 单个关键词的命中权重：内容命中 > 时间命中 */
const CONTENT_HIT_SCORE = 2;
const TIME_HIT_SCORE = 1;

/** 计划项展示行 */
function formatLine(item: ScheduleItem, index: number): string {
	const time = item.type === 'daily' ? `每日 ${item.time}` : item.time;
	return `[计划项${index + 1}] ID:${item.id} | 时间:${time} | 内容:${item.content}`;
}

/** 条目时间排序键（daily 任务按今天执行时刻参与排序，无法解析时排最后） */
function sortKey(item: ScheduleItem, now: Date): number {
	if (item.type === 'daily') return dailyTaskTime(item, now)?.getTime() ?? Number.MAX_SAFE_INTEGER;
	const t = new Date(item.time).getTime();
	return isNaN(t) ? Number.MAX_SAFE_INTEGER : t;
}

/**
 * 计算计划项对关键词数组的命中得分
 *
 * 每个关键词独立计分：内容命中 +2，内容未中但时间命中 +1（如用 "15:30" 查时间）；
 * 全部命中（多关键词同时命中）的条目自然排在最前。
 */
function scoreItem(item: ScheduleItem, keywords: string[]): number {
	const content = item.content.toLowerCase();
	let score = 0;
	for (const keyword of keywords) {
		const kw = keyword.toLowerCase();
		if (!kw) continue;
		if (content.includes(kw)) score += CONTENT_HIT_SCORE;
		else if (item.time.includes(keyword)) score += TIME_HIT_SCORE;
	}
	return score;
}

/** 判断计划项是否落在查询范围内（daily 任务每天执行，任何范围都包含） */
function inRange(item: ScheduleItem, start: Date, end: Date, now: Date): boolean {
	if (item.type === 'daily') return true;
	const t = new Date(item.time).getTime();
	if (isNaN(t)) return false;
	return t >= start.getTime() && t <= end.getTime();
}

/**
 * 执行计划表查询
 *
 * @param cache 计划表内存缓存
 * @param keywords 关键词数组（可为空；元素自动 trim，忽略空串）
 * @param date 日期原文（相对词 / ISO / 中文；缺省按 scope 取默认）
 * @param scope 查询粒度（day / month / year / all；缺省按 date 形态推断，查某天不填日期默认今天）
 * @returns 工具结果文本（单元素数组）
 */
export function querySchedules(
	cache: ScheduleItem[],
	keywords: unknown,
	date: unknown,
	scope: unknown
): string[] {
	// 关键词数组规整：兼容单个字符串误传
	const rawList = Array.isArray(keywords) ? keywords : (keywords ? [keywords] : []);
	const keywordList = rawList.map(k => String(k ?? '').trim()).filter(k => k.length > 0);

	// 日期范围解析（scope=all 时不限时间；解析失败直接报错提示）
	const { range, error } = parseDateRange(
		date === undefined || date === null ? '' : String(date),
		scope === undefined || scope === null ? undefined : String(scope)
	);
	if (error) return [`查询计划失败：${error}`, ''];

	const now = new Date();
	// 预选：范围内条目
	const candidates = range
		? cache.filter(item => inRange(item, range.start, range.end, now))
		: [...cache];

	// 关键词过滤 + 相关度排序；无关键词按时间升序
	let matched: Array<{ item: ScheduleItem; score: number }>;
	if (keywordList.length > 0) {
		matched = candidates
			.map(item => ({ item, score: scoreItem(item, keywordList) }))
			.filter(entry => entry.score > 0)
			.sort((a, b) =>
				b.score - a.score ||
				sortKey(a.item, now) - sortKey(b.item, now)
			);
	}
	else {
		matched = candidates
			.map(item => ({ item, score: 0 }))
			.sort((a, b) => sortKey(a.item, now) - sortKey(b.item, now));
	}

	// 空态文案：区分"范围内没有计划"与"有计划但关键词没命中"
	if (matched.length === 0) {
		if (candidates.length === 0) {
			const scopeText = range ? `${range.label}没有安排任何计划` : '当前计划表为空';
			return [`${scopeText}，没有任何计划项。`, ''];
		}
		const scopeText = range ? `${range.label}` : '计划表';
		return [`${scopeText}共有 ${candidates.length} 个计划项，但没有命中关键词「${keywordList.join('、')}」的计划。`, ''];
	}

	// 汇总文案：范围 + 总数 + 命中数 + 逐条明细
	const parts: string[] = [];
	parts.push(range
		? `${range.label}共有 ${candidates.length} 个计划项` + (keywordList.length ? `，命中「${keywordList.join('、')}」的有 ${matched.length} 条（按相关度排序）` : '（按时间排序）')
		: `当前计划表共有 ${candidates.length} 个计划项` + (keywordList.length ? `，命中「${keywordList.join('、')}」的有 ${matched.length} 条（按相关度排序）` : ''));
	matched.forEach((entry, i) => parts.push(formatLine(entry.item, i)));
	return [parts.join('\n'), ''];
}
