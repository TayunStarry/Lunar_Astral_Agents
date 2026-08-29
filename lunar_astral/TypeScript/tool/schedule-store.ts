import type { ScheduleItem } from './schedule-defs';
import { SCHEDULE_FILE_PATH } from './schedule-defs';

/** 从磁盘加载计划表原始数据 */
export function loadSchedulesFromDisk(): ScheduleItem[] {
	const [fileData, , , readErr] = readFile(SCHEDULE_FILE_PATH);
	if (readErr) {
		return [];
	}
	try {
		const jsonStr = atob(fileData);
		const data = JSON.parse(jsonStr);
		return Array.isArray(data) ? data : [];
	} catch (e) {
		console.error('[计划表] 计划表数据解析失败:', e);
		return [];
	}
}

/** 将计划表保存到磁盘 */
export function saveSchedulesToDisk(schedules: ScheduleItem[]): boolean {
	try {
		const jsonStr = JSON.stringify(schedules, null, 2);
		const [, , saveErr] = saveFile(SCHEDULE_FILE_PATH, true, jsonStr);
		if (saveErr) {
			console.error('[计划表] 保存计划表失败:', saveErr);
			return false;
		}
		return true;
	} catch (e) {
		console.error('[计划表] 序列化计划表失败:', e);
		return false;
	}
}
