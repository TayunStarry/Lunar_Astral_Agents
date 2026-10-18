import { GlobalConfig } from '../../config/global';

/** 已完成应答计数（用于周期性显存守卫） */
let completedResponseCount = 0;

/** ASR 卸载标记：本轮消息含语音时重置，无语音的轮次发起卸载后置位，避免重复请求路由服务器 */
let asrUnloaded = false;

/** ASR 卸载守卫：检查本轮是否有语音消息（包含音频文件或 URL），无语音的轮次发起卸载后置位，避免重复请求路由服务器 */
export function guardASRUnload(): void {
	// 检查本轮是否有语音消息（包含音频文件或 URL）
	asrUnloaded = GlobalConfig.unreadContext.some(
		message => {
			const items = Array.isArray(message.content) ? message.content : [];
			return items.some(item => item.type === 'input_audio' || item.type === 'audio_url');
		}
	);
	// 如果有语音消息，或当前轮次无消息，不卸载模型
	if (asrUnloaded || GlobalConfig.unreadContext.length === 0) return;
	/** 发起卸载请求，由 Go 层处理模型卸载（如模型未加载则静默跳过） */
	const [_, error] = syncFetch(
		{
			url: url()[0] + '/vram/unload',
			execute: {
				method: 'POST',
				crossDomain: true,
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ model: GlobalConfig.AsrName }),
			},
		}
	);
	// 卸载失败静默跳过，不影响思考链流程
	// 注：syncFetch 的 error 是 Go 侧 error 值（无 message 属性），故按 message → String 逐级取文本
	if (error) console.error('ASR 卸载执行失败:', error.message ?? String(error));
}

/** 周期性显存守卫：每完成 N 次应答检查一次可用显存，不足时卸载本地模型，释放随上下文增长的 KV 缓存占用（下次应答会自动重载模型） */
export function guardChatVRAM(): void {
	/** 聊天显存守卫配置（system） */
	const system = GlobalConfig.customConfig?.system;
	/** 守卫开关（默认开启） */
	const enabled = system?.chat_vram_guard ?? true;
	if (!enabled) return;
	/** 检查间隔：每完成 N 次应答触发一次（默认 16） */
	const interval = system?.chat_vram_guard_interval ?? 16;
	// 非法间隔视为关闭守卫
	if (!Number.isFinite(interval) || interval <= 0) return;
	/** 可用显存阈值（MiB，默认 1024） */
	const thresholdMiB = system?.chat_vram_guard_mib ?? 1024;
	// 应答计数，未达到间隔时跳过
	completedResponseCount++;
	if (completedResponseCount % interval !== 0) return;
	// 调用显存守卫端点，由 Go 层检测显存并按需卸载
	const [result, error] = syncFetch({
		url: url()[0] + '/vram/guard',
		execute: {
			method: 'POST',
			crossDomain: true,
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({ threshold_mib: thresholdMiB }),
		},
	});
	// 守卫失败静默跳过，不影响应答流程
	if (error) {
		console.error('显存守卫执行失败:', error.message);
		return;
	}
	if (result?.body?.triggered) {
		console.log(`显存守卫: 可用显存 ${result.body.free_mib} MiB 低于阈值 ${result.body.threshold_mib} MiB, 已卸载模型: ${(result.body.unloaded || []).join(', ')}`);
	}
	else if (GlobalConfig.debugMode) {
		console.log(`显存守卫: 可用显存 ${result?.body?.free_mib} MiB, 无需卸载`);
	}
}
