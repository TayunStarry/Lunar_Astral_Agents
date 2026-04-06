import * as EntryAPI from '../EntryAPI/code';

/**
 * 系统消息接口
 * 用于描述一条待展示的系统消息
 */
interface SystemMessage {
	/** 系统消息文本内容 */
	message: string;
	/** 系统消息类型，用于指定样式类名 */
	type: string;
};

/**
 * 约束执行器类，用于限制指定周期内函数的调用次数。
 *
 * 当调用次数未超过最大限制时执行允许回调，超过则执行禁止回调。
 */
export class ConstraintExecution {
	/** 约束周期（毫秒） */
	private period: number;
	/** 周期内允许的最大调用次数 */
	private maxCount: number;
	/** 调用次数未超限时执行的回调 */
	private allowedCallback: (...args: any[]) => Promise<any>;
	/** 调用次数超限时执行的回调 */
	private forbiddenCallback: (...args: any[]) => Promise<any>;
	/** 调用时间戳记录 */
	private callRecords: number[];
	/**
	 * 构造函数，初始化约束执行器。
	 *
	 * @param {number} periodMinutes - 约束周期，单位为分钟。
	 *
	 * @param {number} maxCount - 周期内允许的最大调用次数。
	 *
	 * @param {function} allowedCallback - 调用次数未超过限制时执行的回调函数。
	 *
	 * @param {function} forbiddenCallback - 调用次数超过限制时执行的回调函数。
	 */
	constructor(periodMinutes: number, maxCount: number, allowedCallback: (...args: any[]) => Promise<any>, forbiddenCallback?: (...args: any[]) => Promise<any>) {
		this.period = periodMinutes * 60 * 1000;
		this.maxCount = maxCount;
		this.allowedCallback = allowedCallback;
		this.forbiddenCallback = forbiddenCallback;
		this.callRecords = [];
	}
	/**
	 * 执行调用，并根据当前调用次数决定执行哪个回调函数。
	 *
	 * @param args - 传递给回调函数的参数。
	 *
	 * @returns 调用结果，根据调用次数是否超过最大限制而不同。
	 */
	public async run(...args: any[]): Promise<any> {
		/** 当前时间戳 */
		const now = Date.now();
		// 过滤掉超出约束周期的调用记录
		this.callRecords = this.callRecords.filter((timestamp) => now - timestamp < this.period);
		// 检查当前调用次数是否未超过最大限制
		if (this.callRecords.length < this.maxCount) {
			// 调用次数未超过最大限制，记录当前时间戳
			this.callRecords.push(now);
			// 执行允许回调函数，并返回其结果
			return await this.allowedCallback(...args);
		}
		// 调用次数超过最大限制，执行禁止回调函数，并返回其结果
		else return await this.forbiddenCallback?.(...args) || null;
	}
};

/** 延迟执行标识符 */
export const delayExecutionMap = new Map();

/**
 * 延迟执行管理器类，用于管理延迟执行的任务，提供任务的调用、取消等功能。
 */
export class DelayExecutionManager {
	/**
	 * 计算延迟时间，将分钟转换为毫秒。
	 *
	 * @param {number} minutes - 延迟的分钟数。
	 * @returns {number} 转换后的毫秒数。
	 */
	static calculateDelayTime(minutes: number): number {
		return minutes * 60 * 1000;
	};
	/**
	 * 调用延迟执行函数，支持取消之前相同标识符的任务。
	 *
	 * @param {string} identifier - 用于标识延迟执行任务的唯一字符串。
	 * @param {Promise<void>} callback - 延迟时间到达后执行的异步回调函数。
	 * @param {number} delay - 延迟时间（毫秒）。
	 */
	static call(identifier: string, callback: () => Promise<void>, delay: number) {
		// 若存在相同标识符的任务，则清除之前的定时器
		if (delayExecutionMap.has(identifier)) clearTimeout(delayExecutionMap.get(identifier));
		/** 设置新的定时器，在指定延迟时间后执行回调函数 */
		const timeoutId = setTimeout(
			async () => {
				// 执行传入的异步回调函数
				await callback();
				// 回调函数执行完毕，从映射中移除当前任务的标识符
				delayExecutionMap.delete(identifier);
			},
			delay
		);
		// 将新的定时器 ID 存入映射中
		delayExecutionMap.set(identifier, timeoutId);
	};
	/**
	 * 根据标识符取消对应的延迟执行任务。
	 *
	 * @param {string} identifier - 用于标识延迟执行任务的唯一字符串。
	 */
	static cancel(identifier: string) {
		// 若映射为空或不存在该标识符，则直接返回
		if (delayExecutionMap.size === 0 || !delayExecutionMap.has(identifier)) return;
		// 清除对应的定时器
		clearTimeout(delayExecutionMap.get(identifier));
		// 从映射中删除该标识符
		delayExecutionMap.delete(identifier);
	};
	/**
	 * 取消所有延迟执行的任务。
	 */
	static cancelAll() {
		// 若映射为空，则直接返回
		if (delayExecutionMap.size === 0) return;
		// 遍历所有标识符并调用 cancel 方法取消对应的任务
		delayExecutionMap.forEach((_, identifier) => DelayExecutionManager.cancel(identifier));
	};
};

/**
 * 系统消息队列
 * 按顺序存储待展示的系统消息
 */
export let systemMessageQueue: SystemMessage[] = [];

/**
 * 当前正在展示的定时器 ID
 * 用于控制消息自动隐藏，为 null 表示当前无消息在展示
 */
export let systemMessageTimer: NodeJS.Timeout | null = null;

/**
 * 页面中用于展示系统消息的面板元素
 * 通过 ID 获取并强制类型断言为 HTMLElement
 */
export const systemStatusPanel = document.getElementById("systemStatusPanel") as HTMLElement;

/**
 * 显示系统消息，并在 5 秒后自动隐藏
 *
 * @param {string} message - 需要显示的系统消息内容
 *
 * @param {EntryAPI.ShowStatusType} type - 消息的类型，用于指定样式类名
 */
export function showSystemMessage(message: string, type: EntryAPI.ShowStatusType) {
	// 将消息添加到队列
	systemMessageQueue.push({ message, type });
	// 如果当前没有消息正在显示，则立即显示队列中的第一个消息
	if (!systemMessageTimer) displayNextSystemMessage();
};

/**
 * 显示队列中的下一条系统消息
 */
export function displayNextSystemMessage() {
	// 如果队列为空，直接返回
	if (systemMessageQueue.length === 0) {
		systemMessageTimer = null;
		return;
	}
	// 获取队列中的第一个消息
	const { message, type } = systemMessageQueue.shift() || { message: "发生未知错误", type: "error" };
	// 设置系统状态面板的文本内容为传入的消息
	systemStatusPanel.textContent = message;
	// 设置系统状态面板的类名，包含基础类名、消息类型类名和显示类名
	systemStatusPanel.className = `system-message ${type} show`;
	// 如果消息类型为错误，创建错误日志文件
	if (type === "error") createErrorLogFile(message);
	/** 隐藏系统提示 */
	function hideSystemMessage() {
		// 移除显示类名，隐藏系统消息面板
		systemStatusPanel.classList.remove('show');
		// 清空定时器 ID
		systemMessageTimer = null;
		// 显示下一条消息
		displayNextSystemMessage();
	};
	// 设置一个 3 秒的定时器，3 秒后隐藏系统消息面板并清空定时器 ID
	systemMessageTimer = setTimeout(hideSystemMessage, 3000);
};

/**
 * 将错误信息存储到数据库
 *
 * @param {string | Error} error - 错误消息内容或Error对象
 * @param {string} [context] - 错误发生的上下文信息
 */
export async function createErrorLogFile(error: string | Error, context?: string) {
	/** 当前时间 */
	const now = new Date();
	/** 时间戳字符串 */
	const timestamp = now.toISOString();
	/** 本地化时间字符串 */
	const localTime = now.toLocaleString();
	/** 错误类型 */
	let errorType = '月华出现故障';
	/** 错误描述 */
	let errorDescription = '';
	/** 错误堆栈 */
	let errorStack = '';
	/** 错误发生路径 */
	let errorPath = window.location.href;
	// 处理不同类型的错误输入
	if (error instanceof Error) {
		errorType = error.name || errorType;
		errorDescription = error.message;
		errorStack = error.stack || '无堆栈信息';
	}
	else {
		/** 尝试从字符串中提取错误类型、描述和路径 */
		const splitMessage = error.split(" | ");
		// 提取错误类型、描述和路径
		if (splitMessage.length >= 1) errorType = splitMessage[0];
		if (splitMessage.length >= 2) errorDescription = splitMessage[1];
		if (splitMessage.length >= 3) errorPath = splitMessage[2];
		if (splitMessage.length === 1) errorDescription = error;
	}
	/** 构建完整的错误描述 */
	const fullDescription = [
		`时间戳: ${timestamp}`,
		`本地时间: ${localTime}`,
		`错误类型: ${errorType}`,
		`错误描述: ${errorDescription}`,
		`错误堆栈: ${errorStack}`,
		`错误路径: ${errorPath}`,
		`上下文信息: ${context || '无上下文信息'}`,
		`浏览器: ${navigator.userAgent}`,
		`页面: ${document.title}`,
		`URL: ${window.location.href}`,
	].join('\n');
	// 发送 POST 请求将错误日志存储到数据库
	try {
		/** 构建数据库查询请求体 */
		const operations: EntryAPI.DatabaseOperation[] = [
			{
				type: 'insert',
				table: 'ErrorLog',
				data: {
					Type: errorType,
					Description: fullDescription
				}
			}
		];
		/** 定义创建表操作 */
		const createTableOperation: EntryAPI.DatabaseOperation = {
			type: 'create',
			table: 'ErrorLog',
			definition: {
				columns: [
					{ name: "ID", type: "INTEGER", primary_key: true, auto_increment: true },
					{ name: "Type", type: "TEXT" },
					{ name: "Description", type: "TEXT" }
				]
			}
		};
		/** 解析数据库查询响应 */
		const result: EntryAPI.BatchResult = await EntryAPI.queryFromDatabase(operations, createTableOperation);
		// 检查数据库操作是否成功
		if (!result.success) throw new Error(`数据库操作失败: ${result.error}`);
	}
	catch (fetchError) {
		// 处理日志存储过程中的错误
		console.error('存储错误日志时发生错误:', fetchError);
	}
}