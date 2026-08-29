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