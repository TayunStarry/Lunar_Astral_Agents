/** 计划接口定义，描述一个计划的基本结构 */
interface Plan {
	/** 计划的唯一标识符 */
	readonly id: string;
	/** 执行计数器，用于控制执行频率 */
	executionCount: number;
	/** 计划执行后的回调函数，接收执行后数据 */
	onExecutionComplete(data: PostPlanExecutionData): Promise<void>;
	/** 计划的类型名称，用于标识计划类型 */
	readonly typeName: string;
	/** 执行间隔，控制计划执行频率 */
	executionInterval: number;
	/** 执行超时时间（毫秒），默认无超时 */
	executionTimeout?: number;
	/** 元数据，存储额外的自定义信息 */
	metadata: Record<string, any>;
};

/** 计划执行后的数据接口 */
interface PostPlanExecutionData {
	/** 当前执行的计划对象 */
	plan: Plan,
	/** 移除计划的回调函数 */
	remove: () => void,
};

/** 计划执行前的数据接口 */
interface PrePlanExecutionData {
	/** 当前待执行的计划对象 */
	plan: Plan,
	/** 移除计划的回调函数 */
	remove: () => void,
	/** 终止执行的回调函数，阻止计划继续执行 */
	cancel: () => void
};

/** 计划执行前的钩子函数类型 */
type PreExecutionHook = (data: PrePlanExecutionData) => Promise<boolean>;

/** 计划执行后的钩子函数类型 */
type PostExecutionHook = (data: PostPlanExecutionData) => Promise<void>;

/** 计划执行错误的钩子函数类型 */
type ErrorExecutionHook = (data: PlanErrorData) => Promise<void>;

/** 计划执行错误的数据接口 */
interface PlanErrorData {
	/** 当前执行的计划对象 */
	plan: Plan,
	/** 移除计划的回调函数 */
	remove: () => void,
	/** 错误信息 */
	error: Error
};

/** 生成一个唯一的计划标签( 仿 UUID 格式 ) */
function uniquePlanLabel(): string {
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

/** 计划管理器 */
export class PlanManager {
	/** 计划队列 */
	private plans: Plan[] = [];
	/** 定时器ID */
	private timerId?: number;
	/** 执行前钩子函数数组 */
	private preExecutionHooks: PreExecutionHook[] = [];
	/** 执行后钩子函数数组 */
	private postExecutionHooks: PostExecutionHook[] = [];
	/** 执行错误钩子函数数组 */
	private errorExecutionHooks: ErrorExecutionHook[] = [];
	/** 计划失败次数映射 */
	private failureCounts: Map<string, number> = new Map();
	/** 构造函数 */
	constructor(interval: number = 1000) {
		// 启动定时器，每秒执行一次计划队列
		this.timerId = setInterval(() => this.processPlans(), interval);
	}
	/** 处理单个计划 */
	private async processPlan(plan: Plan, plansToRemove: Set<string>): Promise<void> {
		// 增加执行计数器
		plan.executionCount += 1;
		// 验证是否到了执行时间
		if (plan.executionCount % plan.executionInterval !== 0) return;
		/** 是否允许执行 */
		let executionAllowed: boolean = true;
		/** 移除计划的回调函数 */
		const remove = () => plansToRemove.add(plan.id);
		/** 终止执行的回调函数，阻止计划继续执行 */
		const cancel = () => executionAllowed = false;
		// 尝试执行计划的回调函数
		try {
			// 创建超时 Promise
			const createTimeoutPromise = (timeout: number): Promise<never> => {
				return new Promise((_, reject) => setTimeout(() => reject(new Error(`计划执行超时 ${timeout}ms`)), timeout));
			};
			// 使用 Promise.race 实现超时控制
			if (plan.executionTimeout && plan.executionTimeout > 0) {
				await Promise.race([this.executePlan(plan, remove, cancel, executionAllowed), createTimeoutPromise(plan.executionTimeout)]);
			}
			// 没有超时时间，直接执行计划
			else await this.executePlan(plan, remove, cancel, executionAllowed);
		}
		catch (error) {
			/** 处理执行过程中的错误 */
			const errorInfo = error instanceof Error ? error : new Error(String(error));
			/** 错误栈 */
			const errorStack: string = errorInfo.stack ?? './';
			/** 错误消息 */
			const errorMessage: string = 'Lunar_Astral_Agents:' + errorInfo.message + errorStack;
			// 打印错误消息
			console.error(errorMessage);
			// 执行错误钩子
			for (const hook of this.errorExecutionHooks) {
				await hook({ plan, remove, error: errorInfo });
			}
			/** 计算失败次数 */
			const currentFailures = (this.failureCounts.get(plan.id) || 0) + 1;
			// 更新失败次数映射
			this.failureCounts.set(plan.id, currentFailures);
			// 如果失败次数达到5次，移除计划
			if (currentFailures >= 5) remove();
		}
	};
	/**
	 * 执行计划
	 * 
	 * @param {Plan} plan - 要执行的计划对象
	 * 
	 * @param {()=>void} remove - 移除计划的回调函数
	 * 
	 * @param {()=>void} cancel - 终止执行的回调函数，阻止计划继续执行
	 * 
	 * @param {boolean} executionAllowed - 是否允许执行
	 */
	private async executePlan(plan: Plan, remove: () => void, cancel: () => void, executionAllowed: boolean) {
		// 执行所有前置钩子
		for (const hook of this.preExecutionHooks) {
			/** 执行前置钩子 */
			const shouldContinue = await hook({ plan, remove, cancel });
			// 验证是否允许执行
			if (!shouldContinue || !executionAllowed) break;
		}
		// 调用计划执行后的回调函数
		if (executionAllowed) {
			// 调用并等待计划执行后的回调函数
			await plan.onExecutionComplete({ plan, remove });
			// 执行所有后置钩子
			for (const hook of this.postExecutionHooks) {
				await hook({ plan, remove });
			}
		}
	};
	/** 处理计划队列 */
	public async processPlans() {
		// 验证计划表队列是否为空
		if (this.plans.length === 0) return
		/** 等待移除的计划ID */
		const plansToRemove: Set<string> = new Set();
		// 遍历执行计划队列
		await Promise.all(this.plans.map(plan => this.processPlan(plan, plansToRemove)));
		// 移除被标记出来的计划
		if (plansToRemove.size > 0) {
			this.plans = this.plans.filter(plan => {
				const shouldRemove = plansToRemove.has(plan.id);
				if (shouldRemove) {
					// 从失败次数映射中删除
					this.failureCounts.delete(plan.id);
				}
				return !shouldRemove;
			});
		}
	};
	/**
	 * 添加计划到队列
	 * 
	 * @param {Plan} plan - 要添加的计划对象
	 */
	public addPlan(plan: Plan): void {
		// 验证 计划对象 是否有效
		if (!plan || typeof plan.id !== 'string') throw new Error('计划对象无效');
		/** 检查是否已存在相同ID的计划 */
		const existingPlanIndex = this.plans.findIndex(p => p.id === plan.id);
		// 验证 是否已存在相同ID的计划
		if (existingPlanIndex !== -1) throw new Error(`计划 ${plan.id} 已存在`);
		// 添加计划到队列
		this.plans.push(plan);
		// 初始化失败次数为0
		this.failureCounts.set(plan.id, 0);
	}
	/**
	 * 从队列中移除计划
	 * 
	 * @param {string} planId - 要移除的计划ID
	 * 
	 * @returns {boolean} 是否成功移除
	 */
	public removePlan(planId: string): boolean {
		/** 初始队列长度 */
		const initialLength = this.plans.length;
		// 从队列中移除计划
		this.plans = this.plans.filter(plan => plan.id !== planId);
		// 从失败次数映射中删除
		this.failureCounts.delete(planId);
		// 返回 是否成功移除
		return this.plans.length < initialLength;
	}
	/**
	 * 获取计划队列（只读副本）
	 * 
	 * @returns {Plan[]} 计划队列的只读副本
	 */
	public getPlans(): Plan[] {
		return [...this.plans];
	}
	/**
	 * 停止计划管理器，清除定时器
	 */
	public stop(): void {
		if (!this.timerId) return;
		clearInterval(this.timerId);
		this.timerId = undefined;
	}
	/**
	 * 重启计划管理器，指定执行间隔
	 */
	public start(interval: number = 1000): void {
		if (this.timerId) return;
		this.timerId = setInterval(() => this.processPlans(), interval);
	}
	/**
	 * 添加执行前钩子
	 * 
	 * @param {PreExecutionHook} hook - 执行前钩子函数
	 */
	public addPreExecutionHook(hook: PreExecutionHook): void {
		this.preExecutionHooks.push(hook);
	}
	/**
	 * 添加执行后钩子
	 * 
	 * @param {PostExecutionHook} hook - 执行后钩子函数
	 */
	public addPostExecutionHook(hook: PostExecutionHook): void {
		this.postExecutionHooks.push(hook);
	}
	/**
	 * 移除执行前钩子
	 * 
	 * @param {PreExecutionHook} hook - 要移除的执行前钩子函数
	 * 
	 * @returns {boolean} 是否成功移除
	 */
	public removePreExecutionHook(hook: PreExecutionHook): boolean {
		/** 检查钩子是否存在 */
		const index = this.preExecutionHooks.indexOf(hook);
		// 如果钩子存在
		if (index !== -1) {
			// 移除钩子
			this.preExecutionHooks.splice(index, 1);
			// 返回 成功移除
			return true;
		}
		// 返回 移除失败
		return false;
	}
	/**
	 * 移除执行后钩子
	 * 
	 * @param {PostExecutionHook} hook - 要移除的执行后钩子函数
	 * 
	 * @returns {boolean} 是否成功移除
	 */
	public removePostExecutionHook(hook: PostExecutionHook): boolean {
		/** 检查钩子是否存在 */
		const index = this.postExecutionHooks.indexOf(hook);
		// 如果钩子存在
		if (index !== -1) {
			// 移除钩子
			this.postExecutionHooks.splice(index, 1);
			// 返回 成功移除
			return true;
		}
		// 返回 移除失败
		return false;
	}
	/**
	 * 清除所有执行前钩子
	 */
	public clearPreExecutionHooks(): void {
		this.preExecutionHooks = [];
	}
	/**
	 * 清除所有执行后钩子
	 */
	public clearPostExecutionHooks(): void {
		this.postExecutionHooks = [];
	}
	/**
	 * 添加执行错误钩子
	 * 
	 * @param {ErrorExecutionHook} hook - 执行错误钩子函数
	 */
	public addErrorExecutionHook(hook: ErrorExecutionHook): void {
		this.errorExecutionHooks.push(hook);
	}
	/**
	 * 移除执行错误钩子
	 * 
	 * @param {ErrorExecutionHook} hook - 要移除的执行错误钩子函数
	 * 
	 * @returns {boolean} 是否成功移除
	 */
	public removeErrorExecutionHook(hook: ErrorExecutionHook): boolean {
		/** 检查钩子是否存在 */
		const index = this.errorExecutionHooks.indexOf(hook);
		// 如果钩子存在
		if (index !== -1) {
			// 移除钩子
			this.errorExecutionHooks.splice(index, 1);
			// 返回 成功移除
			return true;
		}
		// 返回 移除失败
		return false;
	}
	/**
	 * 清除所有执行错误钩子
	 */
	public clearErrorExecutionHooks(): void {
		this.errorExecutionHooks = [];
	}
};

/** 默认计划管理器实例 */
export const defaultPlanManager = new PlanManager();

/** 基础计划 */
export class BasePlan implements Plan {
	/** 计划的唯一标识符 */
	public readonly id: string;
	/** 执行计数器，用于控制执行频率 */
	public executionCount = 1;
	/** 执行超时时间（毫秒），默认无超时 */
	public executionTimeout?: number;
	/**
	 * 计划执行后的回调函数，接收执行后数据
	 * 
	 * @param {PostPlanExecutionData} data - 执行后数据
	 */
	public async onExecutionComplete(data: PostPlanExecutionData): Promise<void> { };
	/** 计划的类型名称，用于标识计划类型 */
	protected constructor(public readonly typeName: string, public executionInterval: number, public metadata: Record<string, any>, executionTimeout?: number) {
		this.executionInterval = Math.floor(executionInterval);
		this.id = uniquePlanLabel();
		this.executionTimeout = executionTimeout;
	};
	/**
	 * 创建基础计划
	 * 
	 * @param {string} typeName - 计划的类型名称
	 * 
	 * @param {number} executionInterval - 执行间隔(秒)，控制计划执行频率
	 * 
	 * @param {Record<string, any>} metadata - 元数据，存储额外的自定义信息
	 * 
	 * @param {number} executionTimeout - 执行超时时间（毫秒），默认无超时
	 * 
	 * @param {PlanManager} manager - 计划管理器实例，默认为默认计划管理器
	 * 
	 * 
	 * @returns {Plan} 创建的计划对象
	 */
	static create(typeName: string, executionInterval: number, metadata: Record<string, any>, executionTimeout?: number, manager: PlanManager = defaultPlanManager): Plan {
		// 验证 类型名称 是否为空
		if (typeName == "") throw new Error("类型名称不能为空");
		// 验证 执行间隔 是否小于等于0
		if (executionInterval <= 0) throw new Error("执行间隔不能小于等于0");
		/** 创建计划对象 */
		const plan = new this(typeName, executionInterval, metadata, executionTimeout);
		// 添加计划到计划管理器
		manager.addPlan(plan);
		// 返回计划对象
		return plan;
	};
	/**
	 * 清理计划资源
	 * 
	 * @param {PlanManager} manager - 计划管理器实例，默认为默认计划管理器
	 */
	public dispose(manager: PlanManager = defaultPlanManager): void {
		// 从计划管理器中移除该计划
		manager.removePlan(this.id);
		// 清理其他可能的资源
	}
};