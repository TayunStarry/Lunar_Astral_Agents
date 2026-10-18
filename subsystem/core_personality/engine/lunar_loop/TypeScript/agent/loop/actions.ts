import { queryBestAction, syncActionsToMemory } from '../capabilities/memory';
import { RandomFloat } from '../../tool/math';

/** 默认后备动作列表（引擎未就绪时使用） */
const FALLBACK_ACTIONS: string[] = ['荡秋千', '翻花绳'];

/** 从前端渲染引擎获取当前可用动作名称列表 */
function getAvailableActionNames(): string[] {
	try {
		const raw = getAvailableActions();
		if (!raw || raw === '{}') return FALLBACK_ACTIONS;
		const parsed: { actions?: Array<{ name: string }> } = JSON.parse(raw);
		if (parsed.actions && Array.isArray(parsed.actions) && parsed.actions.length > 0) {
			return parsed.actions.map(item => item.name);
		}
	}
	catch {
		// 解析失败，使用后备列表
	}
	return FALLBACK_ACTIONS;
}

/** 执行匹配到的动作 */
export async function performMatchedAction(query: string): Promise<void> {
	/** 获取当前可用动作名称列表 */
	const actionNames = getAvailableActionNames();
	// 判定动作组是否存入记忆库
	if (!syncActionsToMemory(actionNames)) return;
	/** 文本与动作库执行一次匹配，取最高匹配度结果 */
	const bestAction = queryBestAction(query);
	// 如果没有匹配到动作，直接返回
	if (!bestAction) return;
	/** 获取当前代理位置 */
	const pos = getAgentPosition();
	// 随机移动代理位置，避免重复执行相同动作
	sendToEngine('movement', JSON.stringify({ position: { x: pos.x + RandomFloat(-0.5, 0.5), y: pos.y, z: pos.z + RandomFloat(-0.5, 0.5), }, resumeTracking: true, }));
	// 执行匹配到的动作
	sendToEngine('action', JSON.stringify({ action: bestAction }));
	// 行动摘要推送到前端「行动」标签
	pushContext('action', `月华${bestAction}了`, '');
}
