import { ToolCall } from '../config/tool';
import { GlobalConfig } from '../config/global';
import { getCacheEntry, normalizeImageTag } from '../agent/capabilities/image-cache';
import type { PainterRole } from '../agent/roles/painter';
import type { MusicianRole } from '../agent/roles/musician';

// ==== 智能体控制工具 ====
// 工具定义导出为常量（对话者聚合），处理函数经 LTPfunction 分发；
// 子智能体实例由 roles.ts 经 registerAgentTargets 注入（避免 tool → roles 静态循环引用）

/** 已注入的子智能体实例（roles.ts 模块初始化时注入） */
let agentTargets: { painterRole: PainterRole; musicianRole: MusicianRole } | null = null;

/** 注入子智能体实例（roles.ts 模块初始化时调用一次） */
export function registerAgentTargets(targets: { painterRole: PainterRole; musicianRole: MusicianRole }): void {
    agentTargets = targets;
}

// ==== 工具定义 ====

/** 智能体控制工具定义 */
export const agentControlTools: ToolCall[] = [
	{
		type: "function",
		function: {
			name: "dispatch_painter",
			description: "向绘图者子智能体发布绘画创作任务。绘图者会完善需求并调用专业工具生成图像，完成后将作品直接推送至前端展示。若用户要求参照图片创作（改画风、换场景、改服饰、保留角色容貌等），通过 reference_tag 指定 [图片.00x] 标签，绘图者将以该图为图生图基座。",
			parameters: {
				type: "object",
				properties: {
					description: {
						type: "string",
						description: "绘画需求描述，如'画一只在樱花树下的白猫'、'把 [图片.003] 画成雪景'。描述越详细，绘画效果越好。"
					},
					reference_tag: {
						type: "string",
						description: "图生图基座图的图片标签（如 图片.003）。用户要求参照某张图片创作时，填该图片在消息中出现的 [图片.00x] 标签；纯文生图时省略。"
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
			description: "向演奏者子智能体发布音乐创作任务。演奏者会完善需求并调用专业工具创作音乐，完成后将乐谱和音频直接推送至前端展示。",
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

/** 处理绘制者调度工具 */
async function handleDispatchPainter(args?: Record<string, any> | string): Promise<string[]> {
	const { description, reference_tag } = parseArgs(args);

	if (!description || typeof description !== 'string' || description.trim().length === 0) {
		return ['绘画任务调度失败：创作描述不能为空，请提供具体的绘画需求', ''];
	}

	if (!agentTargets?.painterRole) {
		return ['绘画任务调度失败：绘制者子智能体未就绪，请稍后重试', ''];
	}

	// 图生图意图传递：对话者经 reference_tag 指定基座图（标签来自共享图片缓存，
	// 新图、降级图与合并转发图同命名空间），宿主在此预先解析并落盘，绘图者可直接创作
	let taskDescription = description.trim();
	if (reference_tag) {
		const tag = normalizeImageTag(String(reference_tag));
		const entry = tag ? getCacheEntry(tag) : null;
		if (entry) {
			const localPath = agentTargets.painterRole.prepareReference(entry.source);
			taskDescription += localPath
				? `（用户指定 [${tag}] 作为图生图基座，参考图已准备就绪：${localPath}，请直接调用 image_to_image 创作）`
				: `（用户指定 [${tag}] 作为基座，但该图源已无法读取（远端地址可能失效），请调用 list_images 后用 select_reference_image 换一个标签重试）`;
		}
		else {
			taskDescription += `（用户提到 [${reference_tag}]，但该标签已过期或不存在，请调用 list_images 查看可用图片后按标签选定基座，取不到再回退文生图）`;
		}
	}

	console.log(`[智能体控制] 调度绘制者${reference_tag ? '（图生图）' : ''}: ${taskDescription.split('\n')[0]}`);
	const result = await agentTargets.painterRole.createCreativeWork(taskDescription);
	console.log(`[智能体控制] 绘制者完成: ${result}`);
	return [result, ''];
}

/** 处理演奏者调度工具 */
async function handleDispatchMusician(args?: Record<string, any> | string): Promise<string[]> {
	const { description } = parseArgs(args);

	if (!description || typeof description !== 'string' || description.trim().length === 0) {
		return ['音乐任务调度失败：创作描述不能为空，请提供具体的音乐需求', ''];
	}

	if (!agentTargets?.musicianRole) {
		return ['音乐任务调度失败：演奏者子智能体未就绪，请稍后重试', ''];
	}

	console.log(`[智能体控制] 调度演奏者: ${description}`);
	const result = await agentTargets.musicianRole.createCreativeWork(description.trim());
	console.log(`[智能体控制] 演奏者完成: ${result}`);
	return [result, ''];
}

// ==== 模块级注册 ====

// 注册智能体控制工具处理函数到 LTPfunction 映射表（工具定义由对话者聚合，不再入公共数组）
GlobalConfig.LTPfunction.set('dispatch_painter', handleDispatchPainter);
GlobalConfig.LTPfunction.set('dispatch_musician', handleDispatchMusician);