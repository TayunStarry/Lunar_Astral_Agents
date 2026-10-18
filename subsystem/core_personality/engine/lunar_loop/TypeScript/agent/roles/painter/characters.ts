import type { PortraitParams } from '../../../config/image';
import type { CustomCharacterProfile } from '../../capabilities/character';
import { DEFAULT_RATIO } from './frame';
import type { PainterPromptLibrary } from './prompts';
import type { CharacterProfile, PortraitPreset } from './types';

/** 合并内置角色与自定义角色（自定义角色每次调用都重新合并，创建后立即生效） */
export function mergeProfiles(
	builtin: Map<string, CharacterProfile>,
	custom: Map<string, CustomCharacterProfile>
): Map<string, CharacterProfile> {
	const profiles = new Map(builtin);
	for (const [name, profile] of custom) {
		profiles.set(name, { ...profile, builtin: false });
	}
	return profiles;
}

/** 角色名 → 人话（作品摘要与日志用；自定义角色直接显示其名字） */
export function characterLabel(character: string | undefined): string {
	const name = String(character ?? '').trim();
	switch (name) {
		case '月华': return '月华';
		case '琉璃': return '琉璃';
		case '通用-男': return '男性角色';
		case '通用-女': return '女性角色';
		default: return name || '角色';
	}
}

/** 角色目录文本（替换工具描述里的 {characters} 占位符） */
export function characterCatalogText(profiles: Map<string, CharacterProfile>): string {
	const names = [...profiles.keys()];
	return names.length > 0
		? `当前可用角色：${names.join('、')}；也可用 通用-女 / 通用-男 临时描述外观`
		: '当前可用角色：通用-女 / 通用-男（临时描述外观）';
}

/** 角色默认服装：Q版优先取 Q 版服装，缺省回退全尺寸，再缺省用日常服装 */
export function characterOutfit(profile: CharacterProfile, chibi: boolean): string {
	const outfit = chibi
		? (profile.outfitChibi || profile.outfitFull)
		: profile.outfitFull;
	return outfit || '穿着简洁得体的日常服装';
}

/**
 * 立绘预设：把「角色 + 是否Q版」解析成模板、底板、默认服装与缺省画幅
 *
 * 角色来源有三类：
 *   - 内置角色（月华 / 琉璃）：各自一套外观底板 + 全尺寸/Q版模板 + 默认服装；
 *   - 自定义角色（SQLite 角色库）：复用通用男女模板，底板换成角色库里的基础外观；
 *   - 通用-女 / 通用-男 或临时角色：男女底板 + 绘制者给出的 appearance。
 * 性别只影响模板里的「1女孩 / 1男孩」，自定义角色在创建时已经确定。
 */
export function portraitPreset(
	args: PortraitParams,
	prompts: PainterPromptLibrary,
	customCharacters: Map<string, CustomCharacterProfile>
): PortraitPreset {
	const templates = prompts.templates;
	const raw = String(args.character ?? '').trim();
	const chibi = args.chibi === true;
	const defaultRatio = chibi ? DEFAULT_RATIO : '2:3';
	if (raw === '月华') {
		return {
			kind: '月华', chibi, label: `月华${chibi ? 'Q版' : '全尺寸'}自画像`,
			template: chibi ? templates.chibiSelf : templates.fullSelf,
			base: prompts.builtinCharacters.get('月华')!.appearance,
			outfit: chibi ? templates.defaultOutfitChibi : templates.defaultOutfitFull,
			gender: '女孩', defaultRatio,
		};
	}
	if (raw === '琉璃') {
		return {
			kind: '琉璃', chibi, label: `琉璃${chibi ? 'Q版' : '全尺寸'}立绘`,
			template: chibi ? templates.crystalChibi : templates.crystalFull,
			base: prompts.builtinCharacters.get('琉璃')!.appearance,
			outfit: chibi ? templates.crystalOutfitChibi : templates.crystalOutfitFull,
			gender: '女孩', defaultRatio,
		};
	}
	/** 自定义角色：角色库里的基础外观 + 默认服装，复用通用男女模板 */
	const custom = customCharacters.get(raw);
	if (custom) {
		const profile: CharacterProfile = { ...custom, builtin: false };
		return {
			kind: raw, chibi, label: `${raw}${chibi ? 'Q版' : '全尺寸'}立绘`,
			template: chibi ? templates.genericChibi : templates.genericFull,
			base: profile.appearance,
			outfit: characterOutfit(profile, chibi),
			gender: profile.gender === '男' ? '男孩' : '女孩',
			defaultRatio,
		};
	}
	/** 通用角色：通用男女底板 + 绘制者给出的外观描述 */
	const male = raw === '通用-男';
	/** 未登记的角色名（既不是内置也不是自定义角色），也当作外观描述的一部分保留下来 */
	const named = raw && raw !== '通用-女' && raw !== '通用-男' && raw !== '其他' ? `名为「${raw}」的角色` : '';
	const desc = [named, String(args.appearance ?? '').trim()].filter(t => t).join('，');
	const base = (male ? templates.maleBase : templates.femaleBase) + (desc ? `，${desc}` : '');
	if (!desc) console.log('[绘制者] 通用角色未提供 appearance：仅按通用底板绘制，人物特征会很泛');
	return {
		kind: male ? '通用-男' : '通用-女', chibi, label: `${chibi ? 'Q版' : '全尺寸'}${male ? '男性' : '女性'}角色立绘`,
		template: chibi ? templates.genericChibi : templates.genericFull,
		base,
		outfit: desc ? '' : '穿着简洁得体的日常服装',
		gender: male ? '男孩' : '女孩', defaultRatio,
	};
}

/** 规整合照参与者：接受数组或单个字符串，去空白、去重、保序 */
export function normalizeGroupRoles(value: unknown): string[] {
	const list = Array.isArray(value) ? value : [value];
	const roles: string[] = [];
	for (const item of list) {
		const role = String(item ?? '').trim();
		if (!role || roles.includes(role)) continue;
		roles.push(role);
	}
	return roles;
}

/**
 * 单个参与者 → 外观提示词
 *
 * 已知角色复用「基础外观 + 默认服装（按 Q 版/全尺寸选择）」，不维护单独的合照外观；
 * 临时角色按传入字符串原样使用。withOutfit=false 表示 planning 已统一给出服装，
 * 此时成员块只保留身份特征，避免与规划里的服装描述打架。
 */
export function groupMemberAppearance(
	profiles: Map<string, CharacterProfile>,
	role: string,
	chibi: boolean,
	withOutfit: boolean
): string {
	const profile = profiles.get(role);
	if (!profile) return role;
	const outfit = withOutfit ? `，${characterOutfit(profile, chibi)}` : '';
	return `${profile.name}，${profile.appearance}${outfit}`;
}

/** 未提供 planning 时的默认规划：已知角色用随机表情/姿势 + 当前比例下的默认服装，临时角色不额外追加 */
export function defaultGroupPlanning(
	roles: string[],
	chibi: boolean,
	profiles: Map<string, CharacterProfile>,
	prompts: PainterPromptLibrary
): string {
	const entries: string[] = [];
	for (const role of roles) {
		const profile = profiles.get(role);
		if (!profile) continue;
		entries.push(`${profile.name}：${prompts.randomExpression()}，${prompts.randomPosture()}，${characterOutfit(profile, chibi)}`);
	}
	return entries.join('；');
}
