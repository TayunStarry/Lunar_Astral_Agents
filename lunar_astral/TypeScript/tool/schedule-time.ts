import type { ScheduleItem } from './schedule-defs';

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
