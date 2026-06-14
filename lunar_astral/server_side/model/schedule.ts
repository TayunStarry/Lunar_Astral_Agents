import { ToolCall, OnlyData } from '../index';

/** 计划表项 */
interface ScheduleItem {
	/** 唯一标识 */
	id: string;
	/** 预约执行时间 (ISO 8601 格式) */
	time: string;
	/** 计划工作内容 */
	content: string;
}

/** 计划表存储路径 */
const SCHEDULE_FILE_PATH = 'database/schedule.json';

/** 计划表工具定义 */
export const scheduleTools: ToolCall[] = [
	{
		type: "function",
		function: {
			name: "create_schedule",
			description: "在计划表中创建新的计划项，指定执行时间点和对应的工作内容。时间格式为 ISO 8601 (如 '2026-06-14T15:30:00') 或中文日期时间格式 (如 '2026年6月14日 15:30')。",
			parameters: {
				type: "object",
				properties: {
					time: {
						type: "string",
						description: "计划执行的时间点，支持 ISO 8601 格式或中文日期时间格式，例如 '2026-06-14T15:30:00' 或 '2026年6月14日 15:30'"
					},
					content: {
						type: "string",
						description: "计划执行的工作内容描述，应清晰说明需要完成的事项"
					}
				},
				required: ["time", "content"]
			}
		}
	},
	{
		type: "function",
		function: {
			name: "edit_schedule",
			description: "编辑计划表中已存在的计划项，可修改其执行时间和/或工作内容。需要提供计划项的ID，至少提供 time 或 content 中的一个进行修改。",
			parameters: {
				type: "object",
				properties: {
					id: {
						type: "string",
						description: "要编辑的计划项ID，从 query_schedule 返回结果中获得"
					},
					time: {
						type: "string",
						description: "修改后的执行时间点，不修改则留空"
					},
					content: {
						type: "string",
						description: "修改后的工作内容描述，不修改则留空"
					}
				},
				required: ["id"]
			}
		}
	},
	{
		type: "function",
		function: {
			name: "delete_schedule",
			description: "从计划表中删除指定的计划项。需要提供计划项的ID。",
			parameters: {
				type: "object",
				properties: {
					id: {
						type: "string",
						description: "要删除的计划项ID，从 query_schedule 返回结果中获得"
					}
				},
				required: ["id"]
			}
		}
	},
	{
		type: "function",
		function: {
			name: "query_schedule",
			description: "查询计划表中已有的计划项列表，可按关键词筛选。返回所有匹配的计划项及其ID、时间和内容。",
			parameters: {
				type: "object",
				properties: {
					keyword: {
						type: "string",
						description: "用于筛选计划项的关键词，留空则返回全部计划项"
					}
				},
				required: []
			}
		}
	}
];

// ==== 内存缓存 ====

/** 计划表内存缓存，避免循环中反复读取磁盘 */
let scheduleCache: ScheduleItem[] = [];

// ==== 磁盘 I/O ====

/** 从磁盘加载计划表原始数据 */
function loadSchedulesFromDisk(): ScheduleItem[] {
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
function saveSchedulesToDisk(schedules: ScheduleItem[]): boolean {
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

// ==== 时间归一化 ====

/** 中文日期时间正则: "2026年6月14日 07:44:00" 或 "2026年6月14日 15:30" */
const CN_DATE_REGEX = /^(\d{4})年(\d{1,2})月(\d{1,2})日\s+(\d{1,2}):(\d{2})(?::(\d{2}))?$/;

/**
 * 将各种时间格式归一化为 ISO 8601 (YYYY-MM-DDTHH:mm:ss)
 * 无法解析时返回 null
 */
function normalizeTime(raw: string): string | null {
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

/** 格式化为 ISO 8601 字符串 */
function formatISO(date: Date): string {
	const y = date.getFullYear();
	const mo = String(date.getMonth() + 1).padStart(2, '0');
	const d = String(date.getDate()).padStart(2, '0');
	const h = String(date.getHours()).padStart(2, '0');
	const mi = String(date.getMinutes()).padStart(2, '0');
	const s = String(date.getSeconds()).padStart(2, '0');
	return `${y}-${mo}-${d}T${h}:${mi}:${s}`;
}

// ==== 初始化 ====

/**
 * 初始化计划表：从磁盘加载到内存缓存，自动归一化历史数据中的时间格式。
 * 文件不存在时自动创建空文件。
 */
export function initSchedules(): void {
	const raw = loadSchedulesFromDisk();

	if (raw.length === 0) {
		// 确保空文件存在
		saveSchedulesToDisk([]);
		scheduleCache = [];
		console.log('[计划表] 初始化完成，计划表为空');
		return;
	}

	/** 是否需要回写（时间格式被修正） */
	let needsRewrite = false;

	for (const item of raw) {
		const normalized = normalizeTime(item.time);
		if (normalized && normalized !== item.time) {
			item.time = normalized;
			needsRewrite = true;
		}
	}

	scheduleCache = raw;

	if (needsRewrite) {
		saveSchedulesToDisk(scheduleCache);
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
function handleCreateSchedule(args?: Record<string, any> | string): string {
	const { time, content } = parseArgs(args);
	if (!time || time.trim().length === 0) {
		return '创建计划项失败：执行时间不能为空，请提供有效的时间点';
	}
	if (!content || content.trim().length === 0) {
		return '创建计划项失败：工作内容不能为空，请提供具体的计划内容';
	}

	/** 归一化时间 */
	const normalizedTime = normalizeTime(time);
	if (!normalizedTime) {
		return `创建计划项失败：无法解析时间格式 "${time}"，请使用 ISO 8601 格式 (如 "2026-06-14T15:30:00") 或中文格式 (如 "2026年6月14日 15:30")`;
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
		return '创建计划项失败：保存到磁盘时出错，请稍后重试';
	}

	console.log(`[计划表] 创建成功: [${newItem.id}] ${newItem.time} - ${newItem.content}`);
	return `计划项创建成功：ID为 ${newItem.id}，执行时间: ${newItem.time}，内容: ${newItem.content}`;
}

/** 处理编辑计划项工具 */
function handleEditSchedule(args?: Record<string, any> | string): string {
	const { id, time, content } = parseArgs(args);
	if (!id || id.trim().length === 0) {
		return '编辑计划项失败：计划项ID不能为空，请从 query_schedule 获取有效ID';
	}
	if ((!time || time.trim().length === 0) && (!content || content.trim().length === 0)) {
		return '编辑计划项失败：至少需要提供 time 或 content 中的一个进行修改';
	}

	const index = scheduleCache.findIndex(item => item.id === id.trim());
	if (index === -1) {
		return `编辑计划项失败：未找到ID为 ${id} 的计划项，请使用 query_schedule 确认正确的ID`;
	}

	/** 修改前的快照（用于回滚） */
	const snapshot = { ...scheduleCache[index] };

	if (time && time.trim().length > 0) {
		const normalizedTime = normalizeTime(time);
		if (!normalizedTime) {
			return `编辑计划项失败：无法解析时间格式 "${time}"，请使用 ISO 8601 或中文日期时间格式`;
		}
		scheduleCache[index].time = normalizedTime;
	}
	if (content && content.trim().length > 0) {
		scheduleCache[index].content = content.trim();
	}

	if (!saveSchedulesToDisk(scheduleCache)) {
		// 回滚缓存
		scheduleCache[index] = snapshot;
		return '编辑计划项失败：保存到磁盘时出错，请稍后重试';
	}

	console.log(`[计划表] 编辑成功: [${id}] -> ${scheduleCache[index].time} - ${scheduleCache[index].content}`);
	return `计划项编辑成功：ID为 ${id}，已更新为 执行时间: ${scheduleCache[index].time}，内容: ${scheduleCache[index].content}`;
}

/** 处理删除计划项工具 */
function handleDeleteSchedule(args?: Record<string, any> | string): string {
	const { id } = parseArgs(args);
	if (!id || id.trim().length === 0) {
		return '删除计划项失败：计划项ID不能为空';
	}

	const index = scheduleCache.findIndex(item => item.id === id.trim());
	if (index === -1) {
		return `删除计划项失败：未找到ID为 ${id} 的计划项`;
	}

	/** 被删除的项（用于回滚） */
	const deletedItem = scheduleCache[index];
	scheduleCache.splice(index, 1);

	if (!saveSchedulesToDisk(scheduleCache)) {
		// 回滚缓存
		scheduleCache.splice(index, 0, deletedItem);
		return '删除计划项失败：保存到磁盘时出错，请稍后重试';
	}

	console.log(`[计划表] 删除成功: [${id}] ${deletedItem.time} - ${deletedItem.content}`);
	return `计划项删除成功：已移除 [${deletedItem.id}] ${deletedItem.time} - ${deletedItem.content}`;
}

/** 处理查询计划项工具 */
function handleQuerySchedule(args?: Record<string, any> | string): string {
	const { keyword } = parseArgs(args);

	if (scheduleCache.length === 0) {
		return '当前计划表为空，没有任何计划项，可以放心创建新计划。';
	}

	/** 按时间升序排序 */
	const sorted = [...scheduleCache].sort((a, b) => new Date(a.time).getTime() - new Date(b.time).getTime());

	/** 筛选后的计划项 */
	const filtered = keyword && keyword.trim().length > 0
		? sorted.filter(item => item.content.includes(keyword.trim()) || item.time.includes(keyword.trim()))
		: sorted;

	if (filtered.length === 0) {
		return `未找到包含关键词 "${keyword}" 的计划项，当前共有 ${scheduleCache.length} 个计划项。`;
	}

	return `当前共有 ${scheduleCache.length} 个计划项` + (keyword ? `，匹配 "${keyword}" 的有 ${filtered.length} 条` : '') + ':\n' +
		filtered.map((item, i) => `[计划项${i + 1}] ID:${item.id} | 时间:${item.time} | 内容:${item.content}`).join('\n');
}

// ==== 定时检查 ====

/**
 * 检查到期计划项（仅使用内存缓存，不读取磁盘）。
 * 返回已到期的计划项列表，并从缓存和磁盘中移除。
 */
export function checkDueItems(): ScheduleItem[] {
	if (scheduleCache.length === 0) return [];

	const now = new Date();
	/** 已到期的计划项 */
	const dueItems: ScheduleItem[] = [];
	/** 未到期的计划项 */
	const remaining: ScheduleItem[] = [];

	for (const item of scheduleCache) {
		const itemTime = new Date(item.time);
		if (isNaN(itemTime.getTime())) {
			console.warn(`[计划表] 无效的时间格式，跳过: [${item.id}] ${item.time}`);
			remaining.push(item);
			continue;
		}
		if (now >= itemTime) {
			dueItems.push(item);
			console.log(`[计划表] 触发到期计划项: [${item.id}] ${item.time} - ${item.content}`);
		} else {
			remaining.push(item);
		}
	}

	// 如果有到期项，更新缓存和磁盘
	if (dueItems.length > 0) {
		scheduleCache = remaining;
		saveSchedulesToDisk(scheduleCache);
	}

	return dueItems;
}

// ==== 模块级注册 ====

// 初始化计划表缓存
initSchedules();

// 将工具处理函数注册到月华工具协议映射表
OnlyData.lunarToolPackageMap.set('create_schedule', handleCreateSchedule);
OnlyData.lunarToolPackageMap.set('edit_schedule', handleEditSchedule);
OnlyData.lunarToolPackageMap.set('delete_schedule', handleDeleteSchedule);
OnlyData.lunarToolPackageMap.set('query_schedule', handleQuerySchedule);