import { ToolCall, OnlyData, AgentDefine } from '../index';

// ==== 工具定义 ====

/** 智能体控制工具定义 */
export const agentControlTools: ToolCall[] = [
	{
		type: "function",
		function: {
			name: "dispatch_actor",
			description: "向行动者子智能体发布行动任务。行动者负责控制月华在3D场景中的动画、位移和空间感知。只需用一句话描述你想让月华做什么，行动者会自行规划并执行具体操作。",
			parameters: {
				type: "object",
				properties: {
					description: {
						type: "string",
						description: "行动需求描述，如'让月华去荡秋千'、'移动到秋千旁边'、'翻花绳'。描述越清晰，行动者执行越准确。"
					}
				},
				required: ["description"]
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

/** 解析工具调用参数 */
function parseArgs(args?: Record<string, any> | string): Record<string, any> {
	return typeof args === 'string' ? JSON.parse(args) : (args || {});
}

/** 处理行动者调度工具 */
async function handleDispatchActor(args?: Record<string, any> | string): Promise<string[]> {
	const { description } = parseArgs(args);

	if (!description || typeof description !== 'string' || description.trim().length === 0) {
		return ['行动任务调度失败：任务描述不能为空，请提供具体的行动需求', ''];
	}

	const instance = AgentDefine.instance;
	if (!instance || !instance.actorRole) {
		return ['行动任务调度失败：行动者子智能体未就绪，请稍后重试', ''];
	}

	console.log(`[智能体控制] 调度行动者: ${description}`);
	const result = await instance.actorRole.createCreativeWork(description.trim());
	console.log(`[智能体控制] 行动者完成: ${result}`);
	return [result, ''];
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
OnlyData.LTPfunction.set('dispatch_actor', handleDispatchActor);
OnlyData.LTPfunction.set('dispatch_painter', handleDispatchPainter);
OnlyData.LTPfunction.set('dispatch_musician', handleDispatchMusician);
// 注册智能体控制工具到 LTPdefinition 列表
OnlyData.LTPdefinition.push(...agentControlTools);