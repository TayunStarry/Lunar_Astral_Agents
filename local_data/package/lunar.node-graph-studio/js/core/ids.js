/**
 * 短 ID 生成：时间戳 + 自增计数 + 随机尾，保证同一会话内绝不重复。
 */

let counter = 0;

export function uid(prefix = "id") {
	counter += 1;
	const stamp = Date.now().toString(36);
	const rand = Math.random().toString(36).slice(2, 6);
	return `${prefix}_${stamp}${counter.toString(36)}${rand}`;
}

/** 供测试重置计数器 */
export function __resetCounter() {
	counter = 0;
}

export function nodeId() {
	return uid("n");
}

export function edgeId() {
	return uid("e");
}
