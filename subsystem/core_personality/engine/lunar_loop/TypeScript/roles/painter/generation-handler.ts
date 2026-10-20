import { GlobalConfig } from '../../config/global';
import type { DiffusionGenerationParams, ImageToImageParams } from '../../config/image';
import { normalizeSteps, normalizeStrength, resolveDimensionPlan } from './frame';
import type { PainterHost } from './host';

/** 处理通用扩散图像生成（文生图）：画面比例与步数由绘制者选定，比例缺省 1:1 */
export function handleDiffusionGeneration(host: PainterHost, args: DiffusionGenerationParams): string {
	try {
		const prompt = args.prompt || '';
		if (!prompt.trim()) return '扩散生成失败：正向提示词不能为空';
		/** 输出尺寸计划（文生图无参考图，取比例预设） */
		const plan = resolveDimensionPlan(args.ratio);
		/** 迭代步数（0 表示交引擎按当前方案推荐步数处理，方案未配置时回退 sd_steps） */
		const steps = normalizeSteps(args.steps, '扩散生成');
		console.log(`[绘制者] 扩散生成 - 正向提示词: ${prompt.slice(0, 100)}...，画面比例: ${plan.origin} → ${plan.width}x${plan.height}，步数: ${steps || '默认'}`);
		return host.paint({
			prompt: prompt,
			negativePrompt: args.negative_prompt || '',
			width: plan.width,
			height: plan.height,
			steps: steps,
			cfgScale: args.cfg_scale ?? 1.0,
			superResolution: args.super_resolution === true,
		}, '扩散图像生成');
	}
	catch (error) {
		console.error('[绘制者] 扩散生成处理异常:', error);
		return `扩散图像生成异常: ${error}`;
	}
}

/**
 * 图生图的提示词引导系数：缺省 1.0，可用 `lunar_config.json` 的 `diffusion.img2img_cfg` 覆盖
 *
 * 为什么缺省是 1.0（此前是"1.5 下限 / 2.0 缺省"，这是错的）：
 * 当前基线模型 Qwen-Image 2.1 是**引导蒸馏**模型，官方与调参手册都要求 CFG=1.0
 * （`docs/code-wiki/12-图像生成参数策略.md`：「发灰、塑料感 → 把 CFG 压到 1.0」）。
 * 给 2.0~2.5 等于让它走训练时没见过的双条件残差：画面发灰、过曝、塑料感，
 * 这正是用户反馈「图生图质量远不如文生图」的主因之一（文生图一直是 1.0）。
 *
 * 图生图"改不动"的正确杠杆是 `strength` 而不是 cfg：strength 决定重绘多少步，
 * 0.8~0.9 时提示词本身就主导画面（见 painterRole.md 的实测表）。
 * 若换用非蒸馏模型（SDXL 等）才需要把 cfg 抬到 2~3，此时改配置而不是改代码。
 */
function resolveImg2ImgCfg(value: unknown): number {
	/** cfg 的合法区间（低于 0.5 会失去提示词引导，高于 4 会烧掉画面） */
	const CFG_MIN = 0.5;
	const CFG_MAX = 4;
	const clampCfg = (v: number) => Math.min(Math.max(v, CFG_MIN), CFG_MAX);
	/** 部署级覆盖：模型是否蒸馏属于部署属性，不该由绘制者每次猜 */
	const configured = Number(GlobalConfig.customConfig?.diffusion?.img2img_cfg);
	if (Number.isFinite(configured) && configured > 0) {
		const cfg = clampCfg(configured);
		if (Math.abs(cfg - configured) > 1e-6) console.log(`[绘制者] 配置 img2img_cfg=${configured} 越界，已收进 ${cfg}`);
		return cfg;
	}
	if (typeof value === 'number' && Number.isFinite(value) && value > 0) {
		const cfg = clampCfg(value);
		if (cfg > 1.05) console.log(`[绘制者] 图生图 cfg=${value}：本模型为引导蒸馏，建议 1.0（过高会发灰/过曝/塑料感）`);
		return cfg;
	}
	return 1.0;
}

/** 处理图生图生成（基于本次任务附带的参考图）：强度与步数由绘制者指定，比例随参考图 */
export function handleImageToImage(host: PainterHost, args: ImageToImageParams): string {
	try {
		const prompt = args.prompt || '';
		if (!prompt.trim()) return '图生图失败：正向提示词不能为空';
		if (!host.referenceImage) {
			return `图生图失败：${host.referenceStatus()}。图生图基座由对话者经 dispatch_painter 的 reference_tag 指定；当前任务无参考图时请改用 diffusion_generation 进行文生图`;
		}
		const strength = normalizeStrength(args.strength, '图生图');
		const steps = normalizeSteps(args.steps, '图生图');
		const cfgScale = resolveImg2ImgCfg(args.cfg_scale);
		// 比例随参考图：预算内保持比例忠实，短边不低于 512
		const plan = host.planFor();
		console.log(`[绘制者] 图生图 - 正向提示词: ${prompt.slice(0, 100)}...，参考图: ${host.referenceImage}，强度: ${strength}，cfg: ${cfgScale}，尺寸: ${plan.origin} → ${plan.width}x${plan.height}，步数: ${steps || '默认'}`);
		return host.paint({
			prompt: prompt,
			negativePrompt: args.negative_prompt || '',
			width: plan.width,
			height: plan.height,
			steps: steps,
			cfgScale: cfgScale,
			initImg: host.referenceImage,
			strength: strength,
			superResolution: args.super_resolution === true,
		}, '图生图生成');
	}
	catch (error) {
		console.error('[绘制者] 图生图处理异常:', error);
		return `图生图生成异常: ${error}`;
	}
}
