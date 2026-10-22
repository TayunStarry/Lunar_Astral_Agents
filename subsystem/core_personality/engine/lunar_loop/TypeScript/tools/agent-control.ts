import { ToolCall } from '../config/tool';
import { GlobalConfig } from '../config/global';
import { resolveImageRef } from '../capabilities/image-tag';
import type { PainterRole } from '../roles/painter';
import type { MusicianRole } from '../roles/musician';

// ==== 智能体控制工具 ====
// 工具定义导出为常量（对话者聚合），处理函数经 LTPfunction 分发；
// 子智能体实例由 roles/index.ts 经 registerAgentTargets 注入（避免 tool → roles 静态循环引用）

/** 已注入的子智能体实例（roles/index.ts 模块初始化时注入） */
let agentTargets: { painterRole: PainterRole; musicianRole: MusicianRole } | null = null;

/** 注入子智能体实例（roles/index.ts 模块初始化时调用一次） */
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
			description: "向绘图者子智能体发布绘画创作任务。绘图者会完善需求并调用专业工具生成图像，完成后将作品直接推送至前端展示。若用户要求参照图片创作（改画风、换场景、改服饰、保留角色容貌等），通过 reference_tag 指定图片句柄（形如 @I.003），绘图者将以该图为图生图基座。",
			parameters: {
				type: 'object',
				properties: {
					description: {
						type: 'string',
						description: "绘画需求描述，如'画一只在樱花树下的白猫'、'把 @I.003 画成雪景'。描述越详细，绘画效果越好。以第一人称书写时，「我」「我的」指月华本人（自画像），「我的妹妹」指琉璃。"
					},
					reference_tag: {
						type: 'string',
						description: "图生图基座图的图片句柄（如 @I.003）。用户要求参照某张图片创作时，填该图片在消息中出现的 [@I.00x] 句柄；纯文生图时省略。"
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

    // 图生图意图传递：对话者经 reference_tag 指定基座图（用标签指定图片）。
    // 宿主按标签从资源目录数据库解析出图片，直接作为参考图交给绘制者 ——
    // 绘制者拿到的就是标签对应的图片多模态消息和对话者的要求文本，无需任何措辞约定。
    let reference: { handle: string; source: string; desc?: string } | undefined;
    if (reference_tag) {
        const resolved = resolveImageRef(String(reference_tag));
        if (!resolved) {
            return [`绘画任务调度失败：图片句柄 ${reference_tag} 不存在，请用 browse_resources（kind="I"）查看可引用的图片句柄后重试`, ''];
        }
        if (!resolved.source) {
            return [`绘画任务调度失败：${resolved.handle} 的图片本体已从内容仓库回收（仅存描述${resolved.desc ? `：${resolved.desc}` : ''}），请让用户重新发送该图片，或换一张图片的句柄`, ''];
        }
        reference = resolved;
    }

    console.log(`[智能体控制] 调度绘制者${reference ? `（图生图，参考图 ${reference.handle}）` : ''}: ${description.trim().split('\n')[0]}`);
    const result = await agentTargets.painterRole.createCreativeWork(description.trim(), reference);
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