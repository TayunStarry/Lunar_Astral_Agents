// ============================================================
//  计划表 — 入口聚合（初始化 / 工具处理 / 到期检查），保持公共 API 不变
// ============================================================

import { GlobalConfig } from '../index';
import type { ScheduleItem } from './schedule-defs';
import { scheduleTools, PRESET_DAILY_TASKS } from './schedule-defs';
import { normalizeTime, dailyTaskTime, formatDate } from './schedule-time';
import { loadSchedulesFromDisk, saveSchedulesToDisk } from './schedule-store';

// 重导出定义模块（保持原公共 API：SCHEDULE_TRIGGER_PREFIX / scheduleTools）
export { SCHEDULE_TRIGGER_PREFIX, scheduleTools } from './schedule-defs';

/** 计划表内存缓存，避免循环中反复读取磁盘 */
let scheduleCache: ScheduleItem[] = [];

/**
 * 初始化计划表：从磁盘加载到内存缓存，自动归一化历史数据中的时间格式。
 * 文件不存在时自动创建空文件。
 */
export function initSchedules(): void {
	const raw = loadSchedulesFromDisk();

	// 幂等合并预设每日任务：固定ID缺失时补充，保证每天定时问候存在
	const existingIds = new Set(raw.map(item => item.id));
	let added = 0;
	for (const preset of PRESET_DAILY_TASKS) {
		if (!existingIds.has(preset.id)) {
			raw.push({ ...preset });
			added++;
		}
	}

	if (raw.length === 0) {
		// 确保空文件存在
		saveSchedulesToDisk([]);
		scheduleCache = [];
		console.log('[计划表] 初始化完成，计划表为空');
		return;
	}

	/** 是否需要回写（时间格式被修正或补充了预设任务） */
	let needsRewrite = added > 0;

	for (const item of raw) {
		// 每日任务 time 为 "HH:MM"（每日执行时间），跳过完整时间归一化
		if (item.type === 'daily') continue;
		const normalized = normalizeTime(item.time);
		if (normalized && normalized !== item.time) {
			item.time = normalized;
			needsRewrite = true;
		}
	}

	scheduleCache = raw;

	if (needsRewrite) {
		saveSchedulesToDisk(scheduleCache);
		if (added > 0) console.log(`[计划表] 已补充 ${added} 个预设每日任务`);
		console.log('[计划表] 已修正历史数据中的非标准时间格式');
	}
	console.log(`[计划表] 初始化完成，共加载 ${scheduleCache.length} 个计划项`);
}

// ==== 工具函数 ====

/** 生成唯一ID */
function generateId(): string {
	return `schedule_${Date.now()}_${Math.random().toString(36).slice(2, 9)}`;
}

/** 解析工具调用参数 */
function parseArgs(args?: Record<string, any> | string): Record<string, any> {
	return typeof args === 'string' ? JSON.parse(args) : (args || {});
}

// ==== 工具处理函数 ====

/** 处理创建计划项工具 */
async function handleCreateSchedule(args?: Record<string, any> | string): Promise<string[]> {
	const { time, content } = parseArgs(args);
	if (!time || time.trim().length === 0) {
		return ['创建计划项失败：执行时间不能为空，请提供有效的时间点', ''];
	}
	if (!content || content.trim().length === 0) {
		return ['创建计划项失败：工作内容不能为空，请提供具体的计划内容', ''];
	}

	/** 归一化时间 */
	const normalizedTime = normalizeTime(time);
	if (!normalizedTime) {
		return [`创建计划项失败：无法解析时间格式 "${time}"，请使用 ISO 8601 格式 (如 "2026-06-14T15:30:00") 或中文格式 (如 "2026年6月14日 15:30")`, ''];
	}

	const newItem: ScheduleItem = {
		id: generateId(),
		time: normalizedTime,
		content: content.trim()
	};
	scheduleCache.push(newItem);

	if (!saveSchedulesToDisk(scheduleCache)) {
		// 回滚缓存
		scheduleCache.pop();
		return ['创建计划项失败：保存到磁盘时出错，请稍后重试', ''];
	}

	console.log(`[计划表] 创建成功: [${newItem.id}] ${newItem.time} - ${newItem.content}`);
	return [`计划项创建成功：ID为 ${newItem.id}，执行时间: ${newItem.time}，内容: ${newItem.content}`, ''];
}

/** 处理编辑计划项工具 */
async function handleEditSchedule(args?: Record<string, any> | string): Promise<string[]> {
	const { id, time, content } = parseArgs(args);
	if (!id || id.trim().length === 0) {
		return ['编辑计划项失败：计划项ID不能为空，请从 query_schedule 获取有效ID', ''];
	}
	if ((!time || time.trim().length === 0) && (!content || content.trim().length === 0)) {
		return ['编辑计划项失败：至少需要提供 time 或 content 中的一个进行修改', ''];
	}

	const index = scheduleCache.findIndex(item => item.id === id.trim());
	if (index === -1) {
		return [`编辑计划项失败：未找到ID为 ${id} 的计划项，请使用 query_schedule 确认正确的ID`, ''];
	}

	/** 修改前的快照（用于回滚） */
	const snapshot = { ...scheduleCache[index] };

	if (time && time.trim().length > 0) {
		const normalizedTime = normalizeTime(time);
		if (!normalizedTime) {
			return [`编辑计划项失败：无法解析时间格式 "${time}"，请使用 ISO 8601 或中文日期时间格式`, ''];
		}
		scheduleCache[index].time = normalizedTime;
	}
	if (content && content.trim().length > 0) {
		scheduleCache[index].content = content.trim();
	}

	if (!saveSchedulesToDisk(scheduleCache)) {
		// 回滚缓存
		scheduleCache[index] = snapshot;
		return ['编辑计划项失败：保存到磁盘时出错，请稍后重试', ''];
	}

	console.log(`[计划表] 编辑成功: [${id}] -> ${scheduleCache[index].time} - ${scheduleCache[index].content}`);
	return [`计划项编辑成功：ID为 ${id}，已更新为 执行时间: ${scheduleCache[index].time}，内容: ${scheduleCache[index].content}`, ''];
}

/** 处理删除计划项工具 */
async function handleDeleteSchedule(args?: Record<string, any> | string): Promise<string[]> {
	const { id } = parseArgs(args);
	if (!id || id.trim().length === 0) {
		return ['删除计划项失败：计划项ID不能为空', ''];
	}

	const index = scheduleCache.findIndex(item => item.id === id.trim());
	if (index === -1) {
		return [`删除计划项失败：未找到ID为 ${id} 的计划项`, ''];
	}

	/** 被删除的项（用于回滚） */
	const deletedItem = scheduleCache[index];
	scheduleCache.splice(index, 1);

	if (!saveSchedulesToDisk(scheduleCache)) {
		// 回滚缓存
		scheduleCache.splice(index, 0, deletedItem);
		return ['删除计划项失败：保存到磁盘时出错，请稍后重试', ''];
	}

	console.log(`[计划表] 删除成功: [${id}] ${deletedItem.time} - ${deletedItem.content}`);
	return [`计划项删除成功：已移除 [${deletedItem.id}] ${deletedItem.time} - ${deletedItem.content}`, ''];
}

/** 处理查询计划项工具 */
async function handleQuerySchedule(args?: Record<string, any> | string): Promise<string[]> {
	const { keyword } = parseArgs(args);

	if (scheduleCache.length === 0) {
		return ['当前计划表为空，没有任何计划项，可以放心创建新计划。', ''];
	}

	/** 按时间升序排序 */
	const sorted = [...scheduleCache].sort((a, b) => new Date(a.time).getTime() - new Date(b.time).getTime());

	/** 筛选后的计划项 */
	const filtered = keyword && keyword.trim().length > 0
		? sorted.filter(item => item.content.includes(keyword.trim()) || item.time.includes(keyword.trim()))
		: sorted;

	if (filtered.length === 0) {
		return [`未找到包含关键词 "${keyword}" 的计划项，当前共有 ${scheduleCache.length} 个计划项。`, ''];
	}

	return [`当前共有 ${scheduleCache.length} 个计划项` + (keyword ? `，匹配 "${keyword}" 的有 ${filtered.length} 条` : '') + ':\n' +
		filtered.map((item, i) => `[计划项${i + 1}] ID:${item.id} | 时间:${item.time} | 内容:${item.content}`).join('\n'), ''];
}

// ==== 定时检查 ====

/**
 * 检查到期计划项（仅使用内存缓存，不读取磁盘）。
 * 一次性任务到期后从缓存和磁盘移除；每日任务完成后不移除，仅标记"今日已执行"。
 * 同一时间段多个每日任务激活时，仅执行距离当前时间最近的一个，其余标记"今日已执行"（不执行）。
 */
export function checkDueItems(): ScheduleItem[] {
	if (scheduleCache.length === 0) return [];

	const now = new Date();
	/** 今天日期（YYYY-MM-DD），用于每日任务"今日已执行"判定 */
	const todayStr = formatDate(now);
	/** 已到期的普通计划项（完成后移除） */
	const dueOnce: ScheduleItem[] = [];
	/** 今日到期且尚未执行的每日任务 */
	const dueDaily: ScheduleItem[] = [];
	/** 未到期或保留的计划项 */
	const remaining: ScheduleItem[] = [];

	for (const item of scheduleCache) {
		// 每日任务：今天该时点已到且今天未执行过 → 到期候选；任务本身永远保留
		if (item.type === 'daily') {
			const todayTime = dailyTaskTime(item, now);
			if (todayTime && now >= todayTime && item.completedDate !== todayStr) {
				dueDaily.push(item);
			}
			remaining.push(item);
			continue;
		}
		const itemTime = new Date(item.time);
		if (isNaN(itemTime.getTime())) {
			console.warn(`[计划表] 无效的时间格式，跳过: [${item.id}] ${item.time}`);
			remaining.push(item);
			continue;
		}
		if (now >= itemTime) {
			dueOnce.push(item);
			console.log(`[计划表] 触发到期计划项: [${item.id}] ${item.time} - ${item.content}`);
		} else {
			remaining.push(item);
		}
	}

	/** 本周期实际执行的计划项 */
	const executed: ScheduleItem[] = [...dueOnce];

	// 每日任务冲突处理：同一时间段多个任务激活时，仅执行距离当前时间最近的一个；
	// 所有到期的每日任务（含被执行的）均标记"今日已执行"，通过完成时间字段判定
	if (dueDaily.length > 0) {
		// 按与当前时间的距离升序，距离最近的一个执行
		dueDaily.sort((a, b) => {
			const ta = dailyTaskTime(a, now)!.getTime();
			const tb = dailyTaskTime(b, now)!.getTime();
			return Math.abs(now.getTime() - ta) - Math.abs(now.getTime() - tb);
		});
		const chosen = dueDaily.shift()!;
		console.log(`[计划表] 触发每日任务: [${chosen.id}] ${chosen.time} - ${chosen.content}`);
		executed.push(chosen);
		// 其余到期的每日任务标记"今日已执行"（不执行）
		for (const d of dueDaily) {
			console.log(`[计划表] 每日任务冲突，标记今日已执行(不执行): [${d.id}] ${d.time} - ${d.content}`);
		}
		// 所有到期的每日任务（含被执行的）均标记今日完成
		for (const d of [chosen, ...dueDaily]) {
			d.completedDate = todayStr;
		}
	}

	// 有变更时更新缓存和磁盘
	if (executed.length > 0) {
		scheduleCache = remaining;
		saveSchedulesToDisk(scheduleCache);
	}

	return executed;
}

// ==== 模块级注册 ====

// 初始化计划表缓存
initSchedules();

// 注册计划表工具到 LTPfunction 列表
GlobalConfig.LTPfunction.set('create_schedule', handleCreateSchedule);
GlobalConfig.LTPfunction.set('edit_schedule', handleEditSchedule);
GlobalConfig.LTPfunction.set('delete_schedule', handleDeleteSchedule);
GlobalConfig.LTPfunction.set('query_schedule', handleQuerySchedule);
// 注册计划表工具到 LTPdefinition 列表
GlobalConfig.LTPdefinition.push(...scheduleTools);
