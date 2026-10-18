import { GlobalConfig } from '../../config/global';
import { pushAssistantContext } from '../../io/conversation';
import { RandomFloor } from '../../tools/math';
import { parseToolArgs } from './errors';

// ==== 快速应答台词：按「起句 + 动作 + 收尾」拼接，贴合月华俏皮自信、口语化的人设 ====

/** 起句（俏皮、自信，不用「安排好了」这类状态播报） */
const ACK_LEADS = ['收到', '好嘞', '交给我吧', '没问题', '这个我来'];

/** 收尾（自然口语，带一点活泼的语气） */
const ACK_ENDS = ['～', '，稍等我一下～', '，等我好消息～', '，一会儿就好～'];

/** 绘图动作 */
const DRAW_ACTIONS = ['这就动笔', '这就开画', '马上开工', '先起个稿'];

/** 联网搜索动作 */
const SEARCH_ACTIONS = ['这就去翻资料', '这就去查', '马上去找', '先去翻一翻'];

/** 从数组里随机取一项 */
function pickAckPart(parts: string[]): string {
	return parts[RandomFloor(0, parts.length - 1)];
}

/** 拼接一条快速应答台词 */
function composeAck(actions: string[]): string {
	return `${pickAckPart(ACK_LEADS)}！${pickAckPart(actions)}${pickAckPart(ACK_ENDS)}`;
}

/**
 * 高耗时工具的快速应答文案
 *
 * 只覆盖「用户会明显等待」的两类任务：绘制者与 WebLTP 联网搜索。
 * 返回空串表示不是高耗时工具，不需要快速应答。
 */
function slowTaskAckText(functionName: string, rawArgs: unknown): string {
	if (functionName === 'dispatch_painter') return composeAck(DRAW_ACTIONS);
	if (functionName === 'use_the_program') {
		const target = String(parseToolArgs(rawArgs).tool || '').trim().toLowerCase();
		if (target === 'web_search') return composeAck(SEARCH_ACTIONS);
	}
	return '';
}

/**
 * 发送一次快速应答（不走模型思考，直接文本 + TTS）
 *
 * 一轮思考链内只触发一次（GlobalConfig.fastAckSent）。QQ 来源的消息把
 * auto 置为 true：QQ 桥接会正常转发，但不占用本轮应答份额，真正的最终回应随后生成。
 */
export function maybeSendFastAck(functionName: string, rawArgs: unknown): void {
	if (GlobalConfig.fastAckSent) return;
	const text = slowTaskAckText(functionName, rawArgs);
	if (!text) return;
	GlobalConfig.fastAckSent = true;
	/** 语音数据（TTS 失败时留空，文本仍然照常推送） */
	let audio = '';
	try {
		const [voice, error] = tts(text);
		if (!error && voice) audio = voice;
	}
	catch (error) {
		console.error('[对话者] 快速应答 TTS 失败，仅推送文本:', error);
	}
	try {
		pushAssistantContext('text', text, audio, GlobalConfig.turnFromQQ);
		console.log(`[对话者] 已发送高耗时任务快速应答: ${text}`);
	}
	catch (error) {
		// 快速应答失败不影响主任务；标记保留，避免同一轮反复重试刷屏
		console.error('[对话者] 快速应答推送失败:', error);
	}
}
