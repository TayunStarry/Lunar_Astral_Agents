import { pushAssistantContext } from '../../io/conversation';
import { RandomFloat } from '../../tools/math';

/**
 * 执行匹配到的动作。
 *
 * 动作词表（animation_list）与匹配都在 Go 侧完成：matchAction 读取渲染引擎上报的
 * 当前可用动作，用进程内向量索引选最匹配的一个。动作词表不再写进记忆库——
 * 它是渲染器的临时能力清单，不是月华的长期记忆。
 */
export async function performMatchedAction(query: string): Promise<void> {
	/** 动作文本 → 当前可用动作名（无匹配返回空串） */
	const bestAction = matchAction(String(query || '').trim());
	if (!bestAction) return;
	/** 获取当前代理位置 */
	const pos = getAgentPosition();
	// 随机移动代理位置，避免重复执行相同动作
	sendToEngine('movement', JSON.stringify({ position: { x: pos.x + RandomFloat(-0.5, 0.5), y: pos.y, z: pos.z + RandomFloat(-0.5, 0.5), }, resumeTracking: true, }));
	// 执行匹配到的动作
	sendToEngine('action', JSON.stringify({ action: bestAction }));
	// 行动摘要推送到前端「行动」标签
	pushAssistantContext('action', `月华${bestAction}了`, '');
}
