import type { GenerateImageParams, GroupPhotoParams } from '../../config/image';
import {
	defaultGroupPlanning,
	groupMemberAppearance,
	mergeProfiles,
	normalizeGroupRoles
} from './characters';
import { normalizeSteps, normalizeStrength, resolveDimensionPlan } from './frame';
import type { PainterHost } from './host';

/**
 * 处理合照生成
 *
 * 参与者由 roles 定义：内置角色与自定义角色继承外观与默认服装，临时角色按传入
 * 字符串原样使用。chibi=true 时所有参与者使用 Q 版比例。表情/动作/服饰由
 * planning 统一规划；未提供时对已知角色使用随机默认表情/姿势与默认服装。
 * environment 未提供时从预设背景中随机挑选。
 * 与旧版一致：本次任务附带参考图时以参考图为底图做图生图，比例随参考图。
 */
export function handleGroupPhoto(host: PainterHost, args: GroupPhotoParams): string {
	try {
		const roles = normalizeGroupRoles(args.roles);
		if (roles.length === 0) return '合照生成失败：roles 至少需要一位参与者';
		const chibi = args.chibi === true;
		const profiles = mergeProfiles(host.prompts.builtinCharacters, host.customCharacters);
		const planningProvided = !!String(args.planning || '').trim();
		const planning = String(args.planning || '').trim() || defaultGroupPlanning(roles, chibi, profiles, host.prompts);
		const environment = String(args.environment || '').trim() || host.prompts.randomGroupEnvironment();
		const members = roles
			.map(role => groupMemberAppearance(profiles, role, chibi, !planningProvided))
			.join('；');
		const fullPrompt = host.prompts.renderTemplate(host.prompts.templates.groupPhoto, {
			proportion: chibi ? 'Q版二头身，头身比约1:2，' : '',
			count: String(roles.length),
			members,
			planning: planning ? `人物安排：${planning}。` : '',
			environment,
		});
		console.log(`[绘制者] -> ${chibi ? 'Q版' : '全尺寸'}合照生成（${roles.length}人）`);
		console.log(`参与者: "${roles.join('、')}"`);
		console.log(`人物规划: "${planning || '默认'}"`);
		console.log(`环境: "${environment}"`);
		// 本次是否以参考图为底图（任务级状态，未附带时按文生图绘制）
		const useReference = !!host.referenceImage;
		// 有参考图时比例随参考图；无参考图时按绘制者指定的比例预设（缺省 1:1）
		const plan = useReference ? host.planFor() : resolveDimensionPlan(args.ratio);
		/** 迭代步数（0 表示交引擎按当前方案推荐步数处理，方案未配置时回退 sd_steps） */
		const steps = normalizeSteps(args.steps, '合照');
		const imageParams: GenerateImageParams = {
			prompt: fullPrompt,
			negativePrompt: args.negative_prompt || host.prompts.templates.defaultNegative,
			width: plan.width,
			height: plan.height,
			steps: steps,
			cfgScale: args.cfg_scale ?? 1.0,
			superResolution: args.super_resolution === true,
		};
		// 参考图（本次任务附带）：以参考图为底图，尽量保留参与者容貌
		if (useReference) {
			imageParams.initImg = host.referenceImage;
			imageParams.strength = normalizeStrength(args.strength, '合照');
			console.log(`[绘制者] 合照图生图模式，参考图: ${host.referenceImage}，强度: ${imageParams.strength}，尺寸: ${plan.origin} → ${plan.width}x${plan.height}，步数: ${steps || '默认'}`);
		}
		return host.paint(imageParams, '合照生成');
	}
	catch (error) {
		console.error('[绘制者] 合照生成处理异常:', error);
		return `合照生成异常: ${error}`;
	}
}
