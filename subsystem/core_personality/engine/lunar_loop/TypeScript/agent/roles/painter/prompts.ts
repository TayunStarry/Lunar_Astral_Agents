import { RandomFloor } from '../../../tool/math';
import { loadPromptDocument } from '../../capabilities/prompt';
import { AppearanceParts, CharacterProfile } from './types';

/**
 * 绘制者提示词库
 *
 * 公用提示词与角色提示词按标题路径读取（见 prompts/painterCommon.md 与
 * prompts/painterCharacters.md），负责外观模板填充与默认表情/姿势/背景的随机取用。
 */
export class PainterPromptLibrary {
	/** 公用提示词（默认表情/姿势/负面/合照模板与背景，见 prompts/painterCommon.md） */
	private readonly commonPrompts = loadPromptDocument('prompts/painterCommon.md');
	/** 角色提示词（外观/默认服装/立绘模板，见 prompts/painterCharacters.md） */
	private readonly characterPrompts = loadPromptDocument('prompts/painterCharacters.md');
	/** 默认表情提示 */
	private readonly defaultExpressionPrompt = this.commonPrompts.list('默认表情');
	/** 默认姿势提示 */
	private readonly defaultPosturePrompt = this.commonPrompts.list('默认姿势');
	/** 月华基础外观（发型/瞳色/发饰等比例无关特征，三套人设模板共用） */
	private readonly appearanceBasePrompt = this.characterPrompts.text('月华/基础外观');
	/** Q版（二头身）自我外观模板 */
	private readonly chibiAppearancePrompt = this.characterPrompts.text('月华/立绘-Q版');
	/** 全尺寸（全身立绘）自我外观模板 */
	private readonly fullAppearancePrompt = this.characterPrompts.text('月华/立绘-全尺寸');
	/** 琉璃基础外观（发型/瞳色/发饰等比例无关特征，全尺寸与Q版立绘共用） */
	private readonly crystalBasePrompt = this.characterPrompts.text('琉璃/基础外观');
	/** 琉璃全尺寸立绘外观模板 */
	private readonly crystalFullAppearancePrompt = this.characterPrompts.text('琉璃/立绘-全尺寸');
	/** 琉璃Q版立绘外观模板 */
	private readonly crystalChibiAppearancePrompt = this.characterPrompts.text('琉璃/立绘-Q版');
	/** 通用角色底板（女 / 男）：绘制月华与琉璃之外的人物，画风与两位自家人保持一致 */
	private readonly portraitBaseFemalePrompt = this.characterPrompts.text('通用角色/底板-女');
	private readonly portraitBaseMalePrompt = this.characterPrompts.text('通用角色/底板-男');
	/** 通用立绘模板（男 / 女共用，{gender} 由底板决定），全尺寸与Q版各一套 */
	private readonly genericFullAppearancePrompt = this.characterPrompts.text('通用角色/立绘-全尺寸');
	private readonly genericChibiAppearancePrompt = this.characterPrompts.text('通用角色/立绘-Q版');
	/** Q版默认服装提示词（黑色蕾丝洛丽塔连衣裙，黑色蕾丝袖口，胸前黑色蕾丝蝴蝶结配红宝石，无白色装饰） */
	private readonly defaultOutfitChibiPrompt = this.characterPrompts.text('月华/默认服装-Q版');
	/** 全尺寸默认服装提示词（黑色哥特萝莉风长裙，黑色蕾丝立领与袖口，胸前红宝石蝴蝶结饰扣，无白色装饰） */
	private readonly defaultOutfitFullPrompt = this.characterPrompts.text('月华/默认服装-全尺寸');
	/** 琉璃全尺寸默认服装提示词（白+冰蓝星星主题：星纹吊带裙、外披宽松白外套、白袜白乐福鞋） */
	private readonly crystalDefaultOutfitFullPrompt = this.characterPrompts.text('琉璃/默认服装-全尺寸');
	/** 琉璃Q版默认服装提示词（同一主题的Q版简化：保留星纹吊带裙与缎带蝴蝶结，外套更短） */
	private readonly crystalDefaultOutfitChibiPrompt = this.characterPrompts.text('琉璃/默认服装-Q版');
	/** 合照模板（{count} / {members} / {planning} / {environment}） */
	private readonly groupPhotoTemplate = this.commonPrompts.text('合照模板');
	/** 合照预设背景（未指定环境时随机挑选） */
	private readonly groupEnvironmentPresets = this.commonPrompts.list('合照背景');
	/** 默认负面提示词（立绘与图生图共用） */
	private readonly defaultNegativePrompt = this.commonPrompts.text('默认负面提示词');
	/** 内置角色档案（月华 / 琉璃）：立绘与合照共用同一份外观与默认服装 */
	readonly builtinCharacters: Map<string, CharacterProfile> = new Map([
		['月华', {
			name: '月华', gender: '女', appearance: this.appearanceBasePrompt,
			outfitFull: this.defaultOutfitFullPrompt, outfitChibi: this.defaultOutfitChibiPrompt, builtin: true,
		}],
		['琉璃', {
			name: '琉璃', gender: '女', appearance: this.crystalBasePrompt,
			outfitFull: this.crystalDefaultOutfitFullPrompt, outfitChibi: this.crystalDefaultOutfitChibiPrompt, builtin: true,
		}],
	]);

	/** 月华 / 琉璃 / 通用角色 / 自定义角色共用的模板与默认服装（只读访问） */
	get templates() {
		return {
			chibiSelf: this.chibiAppearancePrompt,
			fullSelf: this.fullAppearancePrompt,
			crystalFull: this.crystalFullAppearancePrompt,
			crystalChibi: this.crystalChibiAppearancePrompt,
			genericFull: this.genericFullAppearancePrompt,
			genericChibi: this.genericChibiAppearancePrompt,
			femaleBase: this.portraitBaseFemalePrompt,
			maleBase: this.portraitBaseMalePrompt,
			defaultOutfitChibi: this.defaultOutfitChibiPrompt,
			defaultOutfitFull: this.defaultOutfitFullPrompt,
			crystalOutfitFull: this.crystalDefaultOutfitFullPrompt,
			crystalOutfitChibi: this.crystalDefaultOutfitChibiPrompt,
			defaultNegative: this.defaultNegativePrompt,
			groupPhoto: this.groupPhotoTemplate,
		};
	}

	/**
	 * 将基础外观、表情、服装、姿势、环境写入外观模板，得到完整提示词
	 *
	 * @param template 场景外观模板（Q版/全尺寸/合照/琉璃立绘）
	 * @param parts 占位符填充参数，缺省时使用随机表情、随机姿势与默认服装
	 * @param defaultOutfit 该场景的默认服装提示词
	 * @param base 该角色的基础外观（月华 / 琉璃各自一套），缺省为月华
	 */
	renderAppearance(template: string, parts: AppearanceParts, defaultOutfit: string, base?: string, gender?: string): string {
		const expression = parts.expression || this.randomExpression();
		const posture = parts.posture || this.randomPosture();
		const outfit = parts.outfit || defaultOutfit;
		return this.renderTemplate(template, {
			base: base || this.appearanceBasePrompt,
			gender: gender || '女孩',
			expression,
			outfit,
			posture,
			environment: parts.environment || '',
		});
	}

	/** 按 {key} 占位符填充模板（replaceAll 语义；避免 String.replace 只替换首个与 $ 转义问题） */
	renderTemplate(template: string, vars: Record<string, string>): string {
		let filled = template;
		for (const [key, value] of Object.entries(vars)) {
			filled = filled.split(`{${key}}`).join(value);
		}
		return this.tidyPrompt(filled);
	}

	/** 合并空槽位留下的连续标点与空句（通用角色把服装写进 appearance 时，{outfit} 槽位会是空串） */
	private tidyPrompt(text: string): string {
		return text
			.replace(/[，,]\s*[，,]+/g, '，')
			.replace(/[，,]\s*。/g, '。')
			.replace(/。\s*。/g, '。')
			.replace(/\s{2,}/g, ' ')
			.trim();
	}

	/** 随机取一条默认表情 */
	randomExpression(): string {
		return this.defaultExpressionPrompt[RandomFloor(0, this.defaultExpressionPrompt.length - 1)];
	}

	/** 随机取一条默认姿势 */
	randomPosture(): string {
		return this.defaultPosturePrompt[RandomFloor(0, this.defaultPosturePrompt.length - 1)];
	}

	/** 随机取一条预设合照背景 */
	randomGroupEnvironment(): string {
		return this.groupEnvironmentPresets[RandomFloor(0, this.groupEnvironmentPresets.length - 1)];
	}
}
