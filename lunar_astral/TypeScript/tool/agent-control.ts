import { ToolCall, OnlyData, AgentDefine } from '../index';

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
		},
		{
			type: "function",
			function: {
				name: "query_agent_position",
				description: "查询智能体当前在3D场景中的位置坐标。返回{x, y, z}坐标，可用于确定移动目标。",
				parameters: {
					type: "object",
					properties: {},
					required: []
				}
			}
		},
		{
			type: "function",
			function: {
				name: "dispatch_painter",
				description: "向绘画师子智能体发布绘画创作任务。绘画师会完善需求并调用专业工具生成图像，完成后将作品直接推送至前端展示。",
				parameters: {
					type: "object",
					properties: {
						description: {
							type: "string",
							description: "绘画需求描述，如'画一只在樱花树下的白猫'、'画一幅星空下的少女'。描述越详细，绘画效果越好。"
						}
					},
					required: ["description"]
				}
			}
		},
		{
			type: "function",
			function: {
				name: "dispatch_musician",
				description: "向演奏家子智能体发布音乐创作任务。演奏家会完善需求并调用专业工具创作音乐，完成后将乐谱和音频直接推送至前端展示。",
				parameters: {
					type: "object",
					properties: {
						description: {
							type: "string",
							description: "音乐需求描述，如'创作一首轻快的钢琴曲'、'写一首抒情的钢琴与大提琴二重奏'。描述越详细，音乐创作效果越好。"
						}
					},
					required: ["description"]
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

/** 处理查询智能体位置工具 */
async function handleQueryAgentPosition(args?: Record<string, any> | string): Promise<string[]> {
	const pos = getAgentPosition();
	const posStr = `当前智能体位置: x=${pos.x.toFixed(2)}, y=${pos.y.toFixed(2)}, z=${pos.z.toFixed(2)}`;
	console.log(`[智能体控制] ${posStr}`);
	return [posStr, ''];
}

/** 处理绘画师调度工具 */
async function handleDispatchPainter(args?: Record<string, any> | string): Promise<string[]> {
	const { description } = parseArgs(args);

	if (!description || typeof description !== 'string' || description.trim().length === 0) {
		return ['绘画任务调度失败：创作描述不能为空，请提供具体的绘画需求', ''];
	}

	const instance = AgentDefine.instance;
	if (!instance || !instance.painterRole) {
		return ['绘画任务调度失败：绘画师子智能体未就绪，请稍后重试', ''];
	}

	console.log(`[智能体控制] 调度绘画师: ${description}`);
	const result = await instance.painterRole.createCreativeWork(description.trim());
	console.log(`[智能体控制] 绘画师完成: ${result}`);
	return [result, ''];
}

/** 处理演奏家调度工具 */
async function handleDispatchMusician(args?: Record<string, any> | string): Promise<string[]> {
	const { description } = parseArgs(args);

	if (!description || typeof description !== 'string' || description.trim().length === 0) {
		return ['音乐任务调度失败：创作描述不能为空，请提供具体的音乐需求', ''];
	}

	const instance = AgentDefine.instance;
	if (!instance || !instance.musicianRole) {
		return ['音乐任务调度失败：演奏家子智能体未就绪，请稍后重试', ''];
	}

	console.log(`[智能体控制] 调度演奏家: ${description}`);
	const result = await instance.musicianRole.createCreativeWork(description.trim());
	console.log(`[智能体控制] 演奏家完成: ${result}`);
	return [result, ''];
}

// ==== 模块级注册 ====

// 注册智能体控制工具到 LTPfunction 映射表
OnlyData.LTPfunction.set('play_action', handlePlayAction);
OnlyData.LTPfunction.set('agent_movement', handleAgentMovement);
OnlyData.LTPfunction.set('query_agent_position', handleQueryAgentPosition);
OnlyData.LTPfunction.set('dispatch_painter', handleDispatchPainter);
OnlyData.LTPfunction.set('dispatch_musician', handleDispatchMusician);
// 注册智能体控制工具到 LTPdefinition 列表
OnlyData.LTPdefinition.push(...agentControlTools);