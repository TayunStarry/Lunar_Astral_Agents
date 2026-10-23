import type { ScheduleItem } from './defs';
import { SCHEDULE_FILE_PATH } from './defs';

/** 从磁盘加载计划表原始数据（readTextFile 直出 UTF-8 文本；计划内容含中文，不能经 atob 的 latin1 解码） */
export function loadSchedulesFromDisk(): ScheduleItem[] {
	const [fileData, readErr] = readTextFile(SCHEDULE_FILE_PATH);
	if (readErr || !fileData) {
		return [];
	}
	try {
		const data = JSON.parse(fileData);
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
