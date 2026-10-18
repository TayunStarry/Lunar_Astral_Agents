import type { PostMessage } from '../../../config/model';
import type { DimensionPlan } from '../../../config/image';
import { visionReference } from '../../capabilities/image-tag';

/** 本次任务的参考图状态（任务级，开工时重置） */
export interface ReferenceState {
	/**
	 * 参考图在内容仓库中的相对 LocalDir 路径（归档 blob，直接用作图生图基座）
	 *
	 * 它**只交给扩散引擎**：模型自己看到的参考图是 vision（视觉读取副本，
	 * ≤ vision_max_side），原图字节永不进入思维链（见 docs/adr/0008）。
	 */
	image: string;
	/** 参考图句柄（@I.00x，与资源目录记录对应；未附带时为空） */
	handle: string;
	/** 参考图视觉副本（端点 URL 或 data URI）：唯一可以放进 image_url 的参考图形态 */
	vision: string;
	/** 参考图比例基准（取自视觉副本，与原图等比；图生图按此推导输出尺寸） */
	size: { width: number; height: number } | null;
}

/** 创建空的参考图状态 */
export function createReferenceState(): ReferenceState {
	return { image: '', handle: '', vision: '', size: null };
}

/** 清空参考图状态（错误重置路径与任务开工时调用） */
export function clearReference(state: ReferenceState): void {
	state.image = '';
	state.handle = '';
	state.vision = '';
	state.size = null;
}

/**
 * 绑定本次任务的参考图
 *
 * 模型侧的参考图只能是视觉读取管线产物：原图（3072×4096 那种）会撑爆多模态模型的
 * 非因果注意力批、让整个 llama-server 实例断言退出，而且违反「思维链不流转原图」。
 * 缩放由 Go 侧视觉管线完成，这里（以及任何 JS 代码）没有缩放入口。
 *
 * @param state 参考图状态（原位修改）
 * @param reference 参考图（句柄 + 归档 blob 相对路径）；纯文生图时省略
 */
export function bindReference(
	state: ReferenceState,
	reference?: { handle: string; source: string; desc?: string }
): void {
	clearReference(state);
	if (!reference?.source) return;
	state.handle = reference.handle || '';
	state.image = reference.source;
	// visionRead 只用于探测副本可生成并取得比例基准；真正进入 image_url 的是视觉读取管线产物
	// （url 形态为端点 URL，base64 形态为同一管线的 data URI）。
	const [, visionError, visionWidth, visionHeight] = visionRead(reference.source, state.handle);
	const visionURL = visionReference(state.handle, reference.source);
	if (visionError || !visionURL) {
		console.error('[绘制者] 参考图视觉副本生成失败，本次任务模型将看不到参考图:', visionError?.message ?? String(visionError ?? '无法构造视觉副本'));
	}
	else {
		state.vision = visionURL;
		if (visionWidth > 0 && visionHeight > 0) state.size = { width: visionWidth, height: visionHeight };
	}
	console.log(`[绘制者] 本次任务参考图: ${state.handle || '(无句柄)'} → ${reference.source}${state.size ? `（视觉副本 ${state.size.width}x${state.size.height}）` : '（视觉副本生成失败）'}`);
}

/**
 * 把任务要求与参考图**视觉副本**合成一条多模态任务消息送进自己的上下文
 *
 * 对话者的要求文本 + 视觉读取副本 —— 绘画者拿到的就是这两样东西，
 * 模型在写提示词前能亲眼看到参考图（绘图是分钟级任务，看图只是秒级开销）。
 *
 * @param state 参考图状态
 * @param taskDescription 对话者传来的任务描述
 * @returns 含参考图的内容项数组；无参考图时退化为纯文本
 */
export function buildReferenceTaskContent(state: ReferenceState, taskDescription: string): PostMessage['content'] {
	if (!state.vision) {
		if (state.image) console.log('[绘制者] 参考图无可用视觉副本，退回纯文本任务描述');
		return taskDescription;
	}
	console.log(`[绘制者] 参考图视觉副本已附入任务上下文（${Math.round(state.vision.length / 1024)}KB）`);
	return [
		{ type: 'text', text: taskDescription },
		{ type: 'image_url', image_url: { url: state.vision } }
	];
}

/** 参考图当前状态描述（供工具结果回显，避免模型在无参考图时反复尝试图生图） */
export function describeReference(state: ReferenceState, plan: DimensionPlan): string {
	if (state.image) {
		const size = state.size ? `（${state.size.width}x${state.size.height}）` : '';
		return `本次任务参考图：${state.handle || state.image}${size}，输出尺寸将按参考图比例取 ${plan.width}x${plan.height}`;
	}
	return '本次任务尚未附带参考图';
}
