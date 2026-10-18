/** JSON 容错解析（工具参数在个别路径上可能是字符串） */
export function parseToolArgs(raw: unknown): Record<string, any> {
	if (raw && typeof raw === 'object') return raw as Record<string, any>;
	try {
		const parsed = JSON.parse(String(raw ?? '{}'));
		return parsed && typeof parsed === 'object' ? parsed : {};
	}
	catch {
		return {};
	}
}

/**
 * 把任意错误值转成可读文本
 *
 * 为什么要专门处理：`syncFetch` 失败时抛出的是 **Go 侧 error**（goja 导出为只有内部字段的
 * 对象），`console.error('...', error)` 打出来就是 `{}`，日志里只留下一句
 * 「请求处理错误: {}」——真实原因（模型服务崩了 / 连接被拒）无从查起。
 */
export function describeError(error: unknown): string {
	if (!error) return '未知错误';
	if (typeof error === 'string') return error;
	const anyError = error as { message?: string; stack?: string };
	if (typeof anyError.message === 'string' && anyError.message) return anyError.message;
	const text = String(error);
	return text && text !== '[object Object]' ? text : '模型调用失败（宿主未提供错误详情）';
}

/** 通知宿主「本轮模型调用成功」——用于清零宿主的连续失败计数（宿主未注册时静默跳过） */
export function notifyAgentSuccess(): void {
	try {
		if (typeof notifyAgentIdle === 'function') notifyAgentIdle();
	}
	catch {
		// 上报失败不影响主流程
	}
}

/** 通知宿主「本轮应答失败」：连续失败达到阈值时宿主会自动重载引擎 */
export function reportAgentFailure(reason: string): void {
	try {
		if (typeof notifyAgentFailure === 'function') notifyAgentFailure(reason);
	}
	catch {
		// 上报失败不影响主流程
	}
}
