/**
 * 速率限制器类，用于限制指定周期内函数的调用次数。
 *
 * 当调用次数未超过最大限制时执行允许回调，超过则执行禁止回调。
 */
export class RateLimitManager  {
	/** 约束周期（毫秒） */
	private period: number;
	/** 周期内允许的最大调用次数 */
	private maxCount: number;
	/** 调用次数未超限时执行的回调 */
	private allowedCallback: (...args: any[]) => Promise<any>;
	/** 调用次数超限时执行的回调 */
	private forbiddenCallback: ((...args: any[]) => Promise<any>) | undefined;
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
		this.forbiddenCallback = forbiddenCallback || undefined;
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