// ============================================================
//  计划表 — 入口聚合（初始化 / 综合工具分发 / 到期检查），保持公共 API 不变
// ============================================================

import { GlobalConfig } from '../../config/global';
import type { ScheduleItem } from './defs';
import { scheduleTools, PRESET_DAILY_TASKS, SCHEDULE_OVERDUE_ABANDON_MS, SCHEDULE_TRIGGER_PREFIX } from './defs';
import { normalizeTime, dailyTaskTime, formatDate } from './time';
import { loadSchedulesFromDisk, saveSchedulesToDisk } from './store';
import { querySchedules } from './query';

// 重导出定义模块（保持原公共 API：SCHEDULE_TRIGGER_PREFIX / scheduleTools）
export { SCHEDULE_TRIGGER_PREFIX, scheduleTools } from './defs';

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
		if (existingIds.has(preset.id)) continue;
		raw.push({ ...preset });
		added++;
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

/** 生成唯一ID（仿 UUID 格式，前缀 schedule 标识来源） */
function generateId(): string {
	/** 执行字符替换 */
	function execute(character: string) {
		/** 随机值 */
		const randomValue = (Math.random() * 16) | 0;
		/** 掩码后的值 */
		const maskedRandomValue = character === 'x' ? randomValue : (randomValue & 0x3 | 0x8);
		// 返回16进制格式的掩码后的值
		return maskedRandomValue.toString(16);
	}
	// 返回唯一的计划标签(仿 UUID 格式)
	return 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, execute);
};

/** 解析工具调用参数 */
function parseArgs(args?: Record<string, any> | string): Record<string, any> {
	return typeof args === 'string' ? JSON.parse(args) : (args || {});
}

// ==== 工具处理函数 ====

/** 处理创建动作 */
async function handleCreate(args?: Record<string, any> | string): Promise<string[]> {
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

/** 处理编辑动作 */
async function handleEdit(args?: Record<string, any> | string): Promise<string[]> {
	const { id, time, content } = parseArgs(args);
	if (!id || id.trim().length === 0) {
		return ['编辑计划项失败：计划项ID不能为空，请先用 action=query 查询获取有效ID', ''];
	}
	if ((!time || time.trim().length === 0) && (!content || content.trim().length === 0)) {
		return ['编辑计划项失败：至少需要提供 time 或 content 中的一个进行修改', ''];
	}

	const index = scheduleCache.findIndex(item => item.id === id.trim());
	if (index === -1) {
		return [`编辑计划项失败：未找到ID为 ${id} 的计划项，请先用 action=query 查询确认正确的ID`, ''];
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

/** 处理删除动作 */
async function handleDelete(args?: Record<string, any> | string): Promise<string[]> {
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

/**
 * 计划表综合工具入口：按 action 分发到创建 / 编辑 / 删除 / 查询
 *
 * 查询走独立查询引擎（keywords 数组 × date/scope 时间范围，按相关度排序，见 query.ts）。
 */
async function handleSchedule(args?: Record<string, any> | string): Promise<string[]> {
	const { action } = parseArgs(args);
	switch (String(action || '').trim()) {
		case 'create': return handleCreate(args);
		case 'edit': return handleEdit(args);
		case 'delete': return handleDelete(args);
		case 'query': {
			const { keywords, date, scope } = parseArgs(args);
			return querySchedules(scheduleCache, keywords, date, scope);
		}
		default:
			return [`计划表工具调用失败：未知的动作 "${action}"，action 必须是 create / edit / delete / query 之一`, ''];
	}
}

// ==== 定时检查 ====

/**
 * 检查到期计划项（仅使用内存缓存，不读取磁盘）。
 * 统一规则（对一次性/每日任务同样生效）：
 * 1. 超时放弃：到期后超过 SCHEDULE_OVERDUE_ABANDON_MS（30分钟）仍未执行的计划直接放弃，视为已完成
 *    （一次性任务从计划表删除；每日任务标记 completedDate 为今日，不再执行）。
 * 2. 每轮最多执行一个：所有到期且未超时的计划中，仅执行距离当前时间最近的一个，
 *    其余保持待触发状态（不标记、不删除），等待后续轮次。
 * 3. 一次性任务执行后从计划表移除；每日任务执行后标记"今日已执行"（保留）。
 */
export function checkDueItems(): ScheduleItem[] {
	if (scheduleCache.length === 0) return [];

	const now = new Date();
	/** 今天日期（YYYY-MM-DD），用于每日任务"今日已执行"判定 */
	const todayStr = formatDate(now);
	/** 本轮结束后仍保留在计划表中的计划项 */
	const kept: ScheduleItem[] = [];
	/** 本轮从计划表移除的计划项（一次性任务：被执行或超时放弃） */
	const removed: ScheduleItem[] = [];
	/** 到期且未超时的候选计划项（含每日任务），附带各自的理论执行时间 */
	const candidates: Array<{ item: ScheduleItem; dueTime: Date }> = [];

	for (const item of scheduleCache) {
		// 每日任务：任务本身永远保留，仅按今日执行状态判定是否成为候选
		if (item.type === 'daily') {
			const todayTime = dailyTaskTime(item, now);
			if (todayTime && now >= todayTime && item.completedDate !== todayStr) {
				if (now.getTime() - todayTime.getTime() > SCHEDULE_OVERDUE_ABANDON_MS) {
					// 超时放弃：标记"今日已执行"，不再执行
					item.completedDate = todayStr;
					console.log(`[计划表] 每日任务超时放弃: [${item.id}] ${item.time} - ${item.content}`);
				}
				else candidates.push({ item, dueTime: todayTime });
			}
			kept.push(item);
			continue;
		}
		const itemTime = new Date(item.time);
		if (isNaN(itemTime.getTime())) {
			console.warn(`[计划表] 无效的时间格式，跳过: [${item.id}] ${item.time}`);
			kept.push(item);
			continue;
		}
		if (now < itemTime) {
			kept.push(item);
			continue;
		}
		if (now.getTime() - itemTime.getTime() > SCHEDULE_OVERDUE_ABANDON_MS) {
			// 超时放弃：一次性任务直接从计划表删除
			removed.push(item);
			console.log(`[计划表] 一次性任务超时放弃: [${item.id}] ${item.time} - ${item.content}`);
			continue;
		}
		candidates.push({ item, dueTime: itemTime });
	}

	/** 本轮实际执行的计划项 */
	const executed: ScheduleItem[] = [];

	// 每轮最多执行一个：所有到期候选（含每日任务）中，仅执行距离当前时间最近的一个
	if (candidates.length > 0) {
		candidates.sort((a, b) =>
			Math.abs(now.getTime() - a.dueTime.getTime()) - Math.abs(now.getTime() - b.dueTime.getTime())
		);
		const chosen = candidates[0].item;
		if (chosen.type === 'daily') {
			chosen.completedDate = todayStr;
			console.log(`[计划表] 触发每日任务: [${chosen.id}] ${chosen.time} - ${chosen.content}`);
		}
		else {
			removed.push(chosen);
			console.log(`[计划表] 触发到期计划项: [${chosen.id}] ${chosen.time} - ${chosen.content}`);
		}
		executed.push(chosen);
	}

	// 有变更时更新缓存和磁盘
	if (executed.length > 0 || removed.length > 0) {
		scheduleCache = kept;
		saveSchedulesToDisk(scheduleCache);
	}

	return executed;
}

// ==== 模块级注册 ====

/**
 * 消费到期计划项并把它们推进未读上下文
 *
 * 为什么单独抽出来：`checkDueItems()` 会**改盘**（一次性任务删除、每日任务标记今日已完成），
 * 所以每个 tick 只能调用一次。它只在对话链的空消息分支里调用；生活链不再整体让位，
 * 因此计划表到期项始终由对话链 tick 正常消费，不会被生活链压住。
 *
 * @returns 本次被消费的计划项
 */
export function flushDueScheduleItems(): ScheduleItem[] {
	const due = checkDueItems();
	for (const item of due) {
		GlobalConfig.unreadContext.push({ role: 'user', content: `${SCHEDULE_TRIGGER_PREFIX} 预约时间已到，请执行以下计划：${item.content}` });
	}
	return due;
}

// 初始化计划表缓存
initSchedules();

// 注册计划表综合工具到 LTPfunction 列表（工具定义由对话者聚合，不再入公共数组）
GlobalConfig.LTPfunction.set('schedule', handleSchedule);
