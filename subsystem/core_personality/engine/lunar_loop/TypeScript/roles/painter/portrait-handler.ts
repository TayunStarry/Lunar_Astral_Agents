import type { PortraitParams } from '../../config/image';
import { portraitPreset } from './characters';
import { normalizeSteps, resolveDimensionPlan } from './frame';
import type { PainterHost } from './host';

/** 处理立绘生成（月华 / 琉璃 / 通用角色，全尺寸与Q版共用同一入口） */
export function handlePortrait(host: PainterHost, args: PortraitParams): string {
	try {
		const preset = portraitPreset(args, host.prompts, host.customCharacters);
		console.log(`[绘制者] -> ${preset.label}生成`);
		console.log(`角色: "${args.character}" chibi=${preset.chibi}`)
		console.log(`表情: "${args.expression}"`)
		console.log(`姿势: "${args.posture}"`)
		console.log(`服装: "${args.outfit}"`)
		console.log(`环境: "${args.environment}"`)
		console.log(`负面提示词: "${args.negative_prompt}"`)
		console.log(`提示词引导系数: "${args.cfg_scale}"`)
		/** 输出尺寸计划：立绘缺省竖版 2:3（Q版缺省 1:1），显式给 ratio 则以它为准 */
		const plan = resolveDimensionPlan(args.ratio || preset.defaultRatio);
		/** 迭代步数（0 表示交引擎按 sd_steps 处理） */
		const steps = normalizeSteps(args.steps, preset.label);
		const fullPrompt = host.prompts.renderAppearance(preset.template, args, preset.outfit, preset.base, preset.gender);
		return host.paint({
			prompt: fullPrompt,
			negativePrompt: args.negative_prompt || host.prompts.templates.defaultNegative,
			width: plan.width,
			height: plan.height,
			steps: steps,
			cfgScale: args.cfg_scale ?? 1.0,
			superResolution: args.super_resolution === true,
		}, `${preset.label}生成`);
	}
	catch (error) {
		console.error('[绘制者] 立绘处理异常:', error);
		return `立绘生成异常: ${error}`;
	}
}
