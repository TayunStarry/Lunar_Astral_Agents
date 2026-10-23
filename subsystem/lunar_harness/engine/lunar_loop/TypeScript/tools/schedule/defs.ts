import { ToolCall } from '../../config/tool';

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

/** 计划到期后的最大容忍时长（毫秒）：超时超过该时长的计划直接放弃，视为已完成（once 删除 / daily 标记今日已完成） */
export const SCHEDULE_OVERDUE_ABANDON_MS = 30 * 60 * 1000;

/** 预设每日任务：每天按「早/中/晚」节奏安排的定时关怀 */
export const PRESET_DAILY_TASKS: ScheduleItem[] = [
	{ id: 'daily_greeting_0630', type: 'daily', time: '07:00', content: '向用户发送清晨早安问候，关心其今日安排' },
	{ id: 'daily_greeting_1000', type: 'daily', time: '10:00', content: '向用户发送"上午好，该喝水啦"的补水提醒' },
	{ id: 'daily_greeting_1200', type: 'daily', time: '12:00', content: '向用户发送午安问候，关心其午餐情况' },
	{ id: 'daily_greeting_1500', type: 'daily', time: '15:00', content: '向用户发送"下午好，该喝水啦"的补水提醒' },
	{ id: 'daily_greeting_1730', type: 'daily', time: '18:00', content: '向用户发送傍晚问候，关心一天收尾与晚间安排' },
	{ id: 'daily_greeting_2230', type: 'daily', time: '22:00', content: '向用户发送晚安问候，祝其好眠' },
];

/** 计划表存储路径 */
export const SCHEDULE_FILE_PATH = 'database/schedule.json';

/** 计划表综合工具定义（创建 / 编辑 / 删除 / 查询一体，按 action 分发） */
export const scheduleTools: ToolCall[] = [
	{
		type: "function",
		function: {
			name: "schedule",
			description: "计划表综合工具：创建、编辑、删除与查询计划项。查询支持关键词数组（内容/时间混合检索，多条结果按相关度排序）与时间范围（某天/某月/某年，查某天不填日期默认今天）；可按内容查时间、按时间查内容、按关键词查完整计划。",
			parameters: {
				type: "object",
				properties: {
					action: {
						type: "string",
						enum: ["create", "edit", "delete", "query"],
						description: "要执行的动作：create 创建计划 / edit 编辑计划 / delete 删除计划 / query 查询计划"
					},
					id: {
						type: "string",
						description: "计划项ID（edit/delete 必填），从 action=query 的返回结果中获得"
					},
					time: {
						type: "string",
						description: "计划执行时间点（create 必填；edit 选填）。支持 ISO 8601（'2026-06-14T15:30:00'）或中文格式（'2026年6月14日 15:30'）"
					},
					content: {
						type: "string",
						description: "计划工作内容（create 必填；edit 选填，提供时覆盖原内容）"
					},
					keywords: {
						type: "array",
						items: { type: "string" },
						description: "查询关键词数组（query 选填）：同时检索计划内容与时间，命中越多排序越靠前；可传单个关键词；不传则返回范围内全部计划"
					},
					date: {
						type: "string",
						description: "查询日期范围（query 选填）：'2026-06-14' 查某天 / '2026-06' 查某月 / '2026' 查某年；支持中文（'6月14日'）与相对词（今天/明天/昨天）。查某天计划不填时默认今天"
					},
					scope: {
						type: "string",
						enum: ["day", "month", "year", "all"],
						description: "查询粒度（query 选填）：day 某天 / month 某月 / year 某年 / all 全部计划；缺省时按 date 格式推断，date 与 keywords 都未提供时也可用 all 列出全部计划"
					}
				},
				required: ["action"]
			}
		}
	}
];
