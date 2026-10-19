import type { ScheduleItem } from './defs';

/** 中文日期时间正则: "2026年6月14日 07:44:00" 或 "2026年6月14日 15:30" */
const CN_DATE_REGEX = /^(\d{4})年(\d{1,2})月(\d{1,2})日\s+(\d{1,2}):(\d{2})(?::(\d{2}))?$/;

/** 格式化为 ISO 8601 字符串 */
export function formatISO(date: Date): string {
	const y = date.getFullYear();
	const mo = String(date.getMonth() + 1).padStart(2, '0');
	const d = String(date.getDate()).padStart(2, '0');
	const h = String(date.getHours()).padStart(2, '0');
	const mi = String(date.getMinutes()).padStart(2, '0');
	const s = String(date.getSeconds()).padStart(2, '0');
	return `${y}-${mo}-${d}T${h}:${mi}:${s}`;
}

/** 格式化为日期字符串 (YYYY-MM-DD)，用于每日任务"今日已执行"判定 */
export function formatDate(date: Date): string {
	const y = date.getFullYear();
	const mo = String(date.getMonth() + 1).padStart(2, '0');
	const d = String(date.getDate()).padStart(2, '0');
	return `${y}-${mo}-${d}`;
}

/**
 * 将各种时间格式归一化为 ISO 8601 (YYYY-MM-DDTHH:mm:ss)
 * 无法解析时返回 null
 */
export function normalizeTime(raw: string): string | null {
	if (!raw || raw.trim().length === 0) {
		return null;
	}

	const trimmed = raw.trim();

	// 尝试标准 Date 解析（覆盖 ISO 8601 等）
	const stdDate = new Date(trimmed);
	if (!isNaN(stdDate.getTime())) {
		return formatISO(stdDate);
	}

	// 尝试中文日期格式: "2026年6月14日 07:44:00"
	const cnMatch = trimmed.match(CN_DATE_REGEX);
	if (cnMatch) {
		const [, year, month, day, hour, minute, second] = cnMatch;
		return `${year.padStart(4, '0')}-${month.padStart(2, '0')}-${day.padStart(2, '0')}T${hour.padStart(2, '0')}:${minute.padStart(2, '0')}:${(second || '00').padStart(2, '0')}`;
	}

	return null;
}

/** 计算每日任务"今天"的执行时间点；time 非 "HH:MM" 格式时返回 null */
export function dailyTaskTime(item: ScheduleItem, now: Date): Date | null {
	const match = item.time.trim().match(/^(\d{1,2}):(\d{2})$/);
	if (!match) return null;
	return new Date(now.getFullYear(), now.getMonth(), now.getDate(), Number(match[1]), Number(match[2]));
}

// ==== 查询日期范围解析 ====

/** 日期查询范围（start/end 均为本地时间，end 为该范围的最后一毫秒） */
export interface DateRange {
	/** 起始时刻（含） */
	start: Date;
	/** 结束时刻（含） */
	end: Date;
	/** 人话描述（如 "2026-06-14（今天）"、"2026年6月"、"2026年"） */
	label: string;
}

/** 查询范围粒度：day 某天 / month 某月 / year 某年 / all 全部 */
export type DateScope = 'day' | 'month' | 'year' | 'all';

/** 构造某一天的范围 */
function dayRange(base: Date, labelSuffix = ''): DateRange {
	const start = new Date(base.getFullYear(), base.getMonth(), base.getDate());
	const end = new Date(base.getFullYear(), base.getMonth(), base.getDate(), 23, 59, 59, 999);
	return { start, end, label: `${formatDate(base)}${labelSuffix}` };
}

/** 构造某个月的范围 */
function monthRange(year: number, month: number, label = ''): DateRange {
	return {
		start: new Date(year, month - 1, 1),
		end: new Date(year, month, 0, 23, 59, 59, 999),
		label: label || `${year}年${month}月`,
	};
}

/** 构造某一年的范围 */
function yearRange(year: number, label = ''): DateRange {
	return {
		start: new Date(year, 0, 1),
		end: new Date(year, 11, 31, 23, 59, 59, 999),
		label: label || `${year}年`,
	};
}

/** 相对日期词 → 与今天的天数偏移 */
const RELATIVE_DAYS: Record<string, number> = { '今天': 0, '今日': 0, '明天': 1, '明日': 1, '后天': 2, '昨天': -1, '昨日': -1 };

/** 全角数字/破折号统一为半角 */
function toHalfWidth(raw: string): string {
	return raw.replace(/[０-９]/g, ch => String.fromCharCode(ch.charCodeAt(0) - 0xFEE0)).replace(/[－—–]/g, '-');
}

/**
 * 解析查询的日期范围
 *
 * @param raw 日期原文，支持：相对词（今天/明天/昨天…）、"2026-06-14"、"2026-06"、"2026"、
 *            中文（"2026年6月14日" / "6月14日" / "2026年6月" / "2026年"）；缺省时按 scope 给默认
 * @param scope 范围粒度；缺省时从 raw 形态推断；raw 也缺省时 day→今天 / month→当月 / year→当年 / all→全部
 * @returns 范围；all 或无法解析返回 null（无法解析时由调用方报错，scope=all 时 null 表示不限时间）
 */
export function parseDateRange(raw: string | undefined, scope: string | undefined): { range: DateRange | null; error: string | null } {
	const now = new Date();
	const normalized = toHalfWidth(String(raw || '').trim());
	const effectiveScope: DateScope | undefined = (['day', 'month', 'year', 'all'] as const).includes(scope as DateScope) ? scope as DateScope : undefined;

	// 相对词：只可能是"某天"查询
	if (normalized && normalized in RELATIVE_DAYS) {
		const base = new Date(now.getFullYear(), now.getMonth(), now.getDate() + RELATIVE_DAYS[normalized]);
		const suffix = normalized === '今天' ? '（今天）' : `（${normalized}）`;
		return { range: dayRange(base, suffix), error: null };
	}

	// 无日期原文：按 scope 给默认范围
	if (!normalized) {
		if (effectiveScope === 'month') return { range: monthRange(now.getFullYear(), now.getMonth() + 1), error: null };
		if (effectiveScope === 'year') return { range: yearRange(now.getFullYear()), error: null };
		if (effectiveScope === 'all') return { range: null, error: null };
		// 未指明 scope 的查询默认"今天"（查某天计划不填日期 → 今天）
		return { range: dayRange(now, '（今天）'), error: null };
	}

	// 完整日期："2026-06-14" / "2026年6月14日" / "2026/6/14"
	let match = normalized.match(/^(\d{4})[年\-/.](\d{1,2})[月\-/.](\d{1,2})日?$/);
	if (match) {
		const [, y, m, d] = match;
		const date = new Date(Number(y), Number(m) - 1, Number(d));
		if (isNaN(date.getTime())) return { range: null, error: `无法识别日期 "${raw}"` };
		return { range: dayRange(date), error: null };
	}

	// 年月："2026-06" / "2026年6月"
	match = normalized.match(/^(\d{4})[年\-/.](\d{1,2})月?$/);
	if (match) {
		const [, y, m] = match;
		return { range: monthRange(Number(y), Number(m)), error: null };
	}

	// 月日："6月14日"（补当前年）
	match = normalized.match(/^(\d{1,2})月(\d{1,2})日?$/);
	if (match) {
		const [, m, d] = match;
		return { range: dayRange(new Date(now.getFullYear(), Number(m) - 1, Number(d))), error: null };
	}

	// 仅月份："6月"（补当前年）
	match = normalized.match(/^(\d{1,2})月$/);
	if (match) {
		return { range: monthRange(now.getFullYear(), Number(match[1])), error: null };
	}

	// 仅年份："2026" / "2026年"
	match = normalized.match(/^(\d{4})年?$/);
	if (match) {
		return { range: yearRange(Number(match[1])), error: null };
	}

	// 兜底：交给标准 Date 解析（覆盖 ISO 8601 带时间等形态），取其所在"天"
	const std = new Date(normalized);
	if (!isNaN(std.getTime())) return { range: dayRange(std), error: null };

	return { range: null, error: `无法识别日期 "${raw}"，请使用如 "2026-06-14"、"2026-06"、"2026" 或 "今天"` };
}
