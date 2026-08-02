import { ToolCall, OnlyData } from '../index';

// ==== 工具定义 ====

/** 智能体控制工具定义 */
export const agentControlTools: ToolCall[] = [
	{
		type: "function",
		function: {
			name: "play_action",
			description: "让智能体执行预设动作。可用动作：荡秋千（需要鼠标追踪）、翻花绳（需要鼠标追踪）。执行动作时会自动切换鼠标追踪状态。",
			parameters: {
				type: "object",
				properties: {
					action_name: {
						type: "string",
						description: "动作名称，可选值：荡秋千、翻花绳",
						enum: ["荡秋千", "翻花绳"]
					}
				},
				required: ["action_name"]
			}
		}
	},
	{
		type: "function",
		function: {
			name: "agent_movement",
			description: "控制智能体移动到指定位置。移动期间会自动关闭鼠标追踪，移动结束后可选恢复。移动有10秒超时限制。",
			parameters: {
				type: "object",
				properties: {
					x: {
						type: "number",
						description: "目标X坐标"
					},
					y: {
						type: "number",
						description: "目标Y坐标（地面为0）"
					},
					z: {
						type: "number",
						description: "目标Z坐标"
					},
					resume_tracking: {
						type: "boolean",
						description: "移动结束后是否恢复鼠标追踪，默认为 true"
					}
				},
				required: ["x", "y", "z"]
			}
		}
	}
];

// ==== 工具处理函数 ====

/** 允许执行的动作列表 */
const ALLOWED_ACTIONS = ['荡秋千', '翻花绳'];

/** 解析工具调用参数 */
function parseArgs(args?: Record<string, any> | string): Record<string, any> {
	return typeof args === 'string' ? JSON.parse(args) : (args || {});
}

/** 处理执行预设动作工具 */
async function handlePlayAction(args?: Record<string, any> | string): Promise<string[]> {
	const { action_name } = parseArgs(args);

	if (!action_name || typeof action_name !== 'string' || action_name.trim().length === 0) {
		return ['执行动作失败：动作名称不能为空，请提供有效的动作名称', ''];
	}

	if (!ALLOWED_ACTIONS.includes(action_name)) {
		return [`执行动作失败：不支持的动作 "${action_name}"，可用动作为：${ALLOWED_ACTIONS.join('、')}`, ''];
	}

	pushContext('action', JSON.stringify({ type: 'action', action: action_name }), '');

	console.log(`[智能体控制] 执行动作: ${action_name}`);
	return [`已执行动作：${action_name}`, ''];
}

/** 处理智能体移动工具 */
async function handleAgentMovement(args?: Record<string, any> | string): Promise<string[]> {
	const { x, y, z, resume_tracking } = parseArgs(args);

	if (typeof x !== 'number' || typeof y !== 'number' || typeof z !== 'number') {
		return ['移动失败：坐标参数 x、y、z 必须为数字', ''];
	}

	if (isNaN(x) || isNaN(y) || isNaN(z)) {
		return ['移动失败：坐标参数 x、y、z 不能为 NaN', ''];
	}

	const resumeTracking = resume_tracking !== false;

	pushContext('movement', JSON.stringify({ type: 'movement', position: { x, y, z }, resumeTracking }), '');

	console.log(`[智能体控制] 移动到 (${x}, ${y}, ${z})，恢复鼠标追踪: ${resumeTracking}`);
	return [`正在移动到 (${x}, ${y}, ${z})`, ''];
}

// ==== 模块级注册 ====

// 注册智能体控制工具到 LTPfunction 映射表
OnlyData.LTPfunction.set('play_action', handlePlayAction);
OnlyData.LTPfunction.set('agent_movement', handleAgentMovement);
// 注册智能体控制工具到 LTPdefinition 列表
OnlyData.LTPdefinition.push(...agentControlTools);
