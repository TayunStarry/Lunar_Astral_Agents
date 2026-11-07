/**
 * 星月智能 · 桌面端 / 移动端共享后端接口
 */
import { ENDPOINTS } from './constants.js';

/**
 * 发送一组消息（OpenAI 兼容格式）
 * @param {Array<object>} messages
 * @returns {Promise<object>}
 */
export async function sendMessages(messages) {
	const resp = await fetch(ENDPOINTS.sendMessage, {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ messages })
	});
	if (!resp.ok) {
		const errorData = await resp.json().catch(() => ({}));
		throw new Error(errorData.message || `HTTP error! status: ${resp.status}`);
	}
	return resp.json();
}

/** 读取通用配置 */
export async function readGeneralConfig() {
	const resp = await fetch(ENDPOINTS.generalConfig, { cache: 'no-store' });
	if (!resp.ok) throw new Error(`读取配置失败 (${resp.status})`);
	return resp.json();
}
