import { ToolCall } from '../index';

/** 计划表项 */
export interface ScheduleItem {
	/** 唯一标识 */
	id: string;
	/** 执行时间：一次性任务为 ISO 8601 完整时间；每日任务为 "HH:MM" 每日执行时间 */
	time: string;
	/** 计划工作内容 */
	content: string;
	/** 任务类型：once 一次性（完成后移除）/ daily 每日任务（完成后不移除） */
	type?: 'once' | 'daily';
	/** 最近完成日期（YYYY-MM-DD），每日任务"每天只执行一次"的判定依据 */
	completedDate?: string;
}

/**
 * 计划表触发消息统一前缀：计划到期触发时，提醒内容统一以该前缀开头。
 * 供 memorizeUnreadRecords() 运行时识别并过滤计划表自动消息，避免被当作普通用户发言写入长期记忆。
 */
export const SCHEDULE_TRIGGER_PREFIX = '[计划提醒]';

/** 预设每日任务：每天按「早/中/晚」节奏安排的定时关怀 */
export const PRESET_DAILY_TASKS: ScheduleItem[] = [
	{ id: 'daily_greeting_0630', type: 'daily', time: '06:30', content: '向用户发送清晨早安问候，关心其今日安排' },
	{ id: 'daily_greeting_0800', type: 'daily', time: '08:00', content: '向用户发送早间问候，精神饱满开启一天' },
	{ id: 'daily_greeting_1000', type: 'daily', time: '10:00', content: '向用户发送"上午好，该喝水啦"的补水提醒' },
	{ id: 'daily_greeting_1200', type: 'daily', time: '12:00', content: '向用户发送午安问候，关心其午餐情况' },
	{ id: 'daily_greeting_1500', type: 'daily', time: '15:00', content: '向用户发送"下午好，该喝水啦"的补水提醒' },
	{ id: 'daily_greeting_1730', type: 'daily', time: '17:30', content: '向用户发送傍晚问候，关心一天收尾与晚间安排' },
	{ id: 'daily_greeting_2230', type: 'daily', time: '22:30', content: '向用户发送晚安问候，祝其好眠' },
];

/** 计划表存储路径 */
export const SCHEDULE_FILE_PATH = 'database/schedule.json';

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
