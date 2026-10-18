import { ToolCall } from '../../config/tool';
import { GlobalConfig } from '../../config/global';
import { RandomFloor } from '../../tool/math';
import { GenerateImageParams, DiffusionGenerationParams, ImageToImageParams, PortraitParams, GroupPhotoParams, ImageTagParams, SchemeSwitchParams, CreateCharacterParams, DimensionPlan } from '../../config/image';
import { ToolCallItem } from '../../config/model';
import { CreativeRoleBase } from '../base/creative';
import { interactEvent } from '../capabilities/ltp-event';
import { attachImageTags, resolveImageRef, sanitizeImageDesc, visionReference } from '../capabilities/image-tag';
import { loadPromptDocument } from '../capabilities/prompt';
import { CustomCharacterProfile, loadCustomCharacters, saveCustomCharacter } from '../capabilities/character';
import { listResources } from '../capabilities/resource';
import type { PostMessage } from '../../config/model';

// ==== 画面比例与尺寸策略 ====
// 两套判据，避免"短边基准"与"长边上限"互相打架：
//   预设比例（文生图/自画像/无参考图的合照）：短边基准 1024，长边按比例对齐 32
//   参考图比例（图生图/已选定参考图的合照）：总像素预算 + 长边上限内尽量保持比例忠实，
//     短边可在 REF_SIDE_MIN~SIDE_BASE 之间浮动（超宽画幅靠缩短短边换取比例不失真）
const SIDE_BASE = 1024;
/** 扩散引擎的边长上限（与 multimodal_analysis normalizeImageSide 的 maxSide 保持一致） */
const SIDE_MAX = 2816;
/** 边长对齐粒度（32 的倍数） */
const SIDE_ALIGN = 32;
/** 参考图比例路径下的短边下限（极端长宽比时用于换取比例忠实） */
const REF_SIDE_MIN = 512;
/** 默认总像素预算（百万像素）：≈1024×1536，可在 lunar_config.json 的 diffusion.pixel_budget_mp 覆盖 */
const DEFAULT_PIXEL_BUDGET_MP = 1.6;
/**
 * 预算软上限系数：预算是「上限」而非「目标」，但短边基准 1024 是清晰度下限，
 * 二者冲突时（例如 2.75:1 在 1024 短边下为 2.88MP > 1.6MP）允许小幅超出预算，
 * 避免预算反而把常规画幅降级。
 */
const PIXEL_BUDGET_SLACK = 1.15;

/** 当前生效的像素预算（像素单位） */
function pixelBudget(): number {
	const configured = Number(GlobalConfig.customConfig?.diffusion?.pixel_budget_mp);
	const budgetMp = Number.isFinite(configured) && configured > 0 ? configured : DEFAULT_PIXEL_BUDGET_MP;
	return budgetMp * 1_000_000;
}

/** 把边长对齐到 32 的倍数并收进引擎允许区间 */
function alignSide(value: number): number {
	const clamped = Math.min(Math.max(Math.round(value), 256), SIDE_MAX);
	return Math.min(Math.round(clamped / SIDE_ALIGN) * SIDE_ALIGN, SIDE_MAX);
}

/** 预设画面比例的比值表（短边 1024，长边由比值推导并对齐 32） */
const RATIO_BASE: Record<string, { ratio: number; horizontal: boolean }> = {
	'1:1': { ratio: 1, horizontal: true },
	'4:3': { ratio: 4 / 3, horizontal: true },
	'3:4': { ratio: 4 / 3, horizontal: false },
	'16:9': { ratio: 16 / 9, horizontal: true },
	'9:16': { ratio: 16 / 9, horizontal: false },
	'3:2': { ratio: 3 / 2, horizontal: true },
	'2:3': { ratio: 3 / 2, horizontal: false },
};

/** 预设画面比例：短边 1024，长边按比值放大并强制对齐到 32 的倍数（与引擎归一化一致） */
export const RATIO_PRESETS: Record<string, { w: number; h: number }> = Object.fromEntries(
	Object.entries(RATIO_BASE).map(([key, item]) => {
		const long = alignSide(SIDE_BASE * item.ratio);
		return [key, item.horizontal ? { w: long, h: SIDE_BASE } : { w: SIDE_BASE, h: long }];
	})
);

/** 默认画面比例（文生图/自画像未指定比例时使用） */
export const DEFAULT_RATIO = '1:1';

/** 图生图强度缺省值（绘制者未指定时的中性值，不再随机） */
/**
 * 图片句柄清单条数上限（list_images 用；防御模型索要过多条目）
 */
const LIST_LIMIT_MAX = 100;

/** 图生图缺省强度（绘制者未指定时；仅"中性"兜底，具体档位见 painterRole.md 指引） */
const DEFAULT_STRENGTH = 0.5;
/** 迭代步数区间：下限 8 供 turbo 方案使用；绘制者未指定时回落到当前方案的推荐步数 */
const STEPS_MIN = 8;
const STEPS_MAX = 60;

/**
 * 由参照尺寸推导输出尺寸：在「总像素预算（上限）+ 长边上限」内尽量贴住参照比例，短边可在 512~1024 之间浮动。
 *
 * 判据顺序（避免"短边基准"与"长边上限"互相打架）：
 *   1. 期望短边 = SIDE_BASE，被两个上限压制——总像素预算开方、长边上限除以比例；
 *   2. 短边再受下限保护（REF_SIDE_MIN）：极端比例时用更短的短边换取比例忠实；
 *   3. 在候选短边（32 对齐）中搜索「比例最接近参照 + 尺寸最大」的一对边长，
 *      两边都取 32 的倍数（引擎硬要求），故比例只能贴近、无法完全相等。
 * 示例：1664×2496（2:3）→ 1024×1536；4000×1000（4:1）→ 640×2528（3.95:1，而非旧实现的 2.75:1）。
 */
export function planDimensions(reference?: { width: number; height: number }): DimensionPlan {
	if (!reference || reference.width <= 0 || reference.height <= 0) {
		const preset = RATIO_PRESETS[DEFAULT_RATIO];
		return { width: preset.w, height: preset.h, origin: `${DEFAULT_RATIO} 预设` };
	}
	const longRef = Math.max(reference.width, reference.height);
	const shortRef = Math.min(reference.width, reference.height);
	/** 参照比例（长边/短边，≥1） */
	const ratio = longRef / shortRef;
	/** 期望短边：基准 SIDE_BASE 是上限（不主动超过它），并受像素预算与长边上限压制 */
	const desired = Math.min(SIDE_BASE, Math.sqrt((pixelBudget() * PIXEL_BUDGET_SLACK) / ratio), SIDE_MAX / ratio);
	/** 短边下限：极端比例时用更短的短边换取比例忠实（不低于 REF_SIDE_MIN） */
	const lowerBound = Math.min(REF_SIDE_MIN, desired);
	/** 短边搜索上界：与期望一致（候选循环内还会以 SIDE_BASE 兜底） */
	const upperBound = Math.max(lowerBound, desired);
	/** 预算容差：两边对齐到 32 后允许 2% 溢出，换取比例更贴近 */
	const budgetLimit = pixelBudget() * PIXEL_BUDGET_SLACK * 1.02;

	let best: { short: number; long: number; err: number; overBudget: boolean } | null = null;
	for (let s = alignSide(lowerBound); s <= upperBound + SIDE_ALIGN; s += SIDE_ALIGN) {
		if (s > SIDE_BASE) break; // 短边不超过基准 1024：清晰度由基准决定，预算是上限而非目标
		const l = alignSide(s * ratio);
		if (l > SIDE_MAX) continue;
		const overBudget = s * l > budgetLimit;
		const err = Math.abs(l / s - ratio) / ratio;
		// 优先级：不超预算 > 画面更大（比例差异 <1% 视为同精度，多用像素更清晰）> 比例更准
		let better = false;
		if (!best) better = true;
		else if (best.overBudget !== overBudget) better = !overBudget;
		else if (!overBudget && Math.abs(err - best.err) <= Math.max(best.err, err) * 0.01 && s * l > best.short * best.long) better = true;
		else if (err < best.err - 1e-9) better = true;
		if (better) best = { short: s, long: l, err, overBudget };
	}
	// 兜底（比例极端到无可行候选）：按长边上限给一对合法尺寸
	if (!best) {
		const l = alignSide(Math.min(SIDE_MAX, Math.max(REF_SIDE_MIN, SIDE_BASE) * ratio));
		best = { short: alignSide(Math.max(REF_SIDE_MIN, l / ratio)), long: l, err: 1, overBudget: false };
	}
	/** 方向沿用参照图本身（竖版不反转） */
	const vertical = reference.height >= reference.width;
	const capped = best.err > 0.08;
	return {
		width: vertical ? best.short : best.long,
		height: vertical ? best.long : best.short,
		origin: `参照图 ${reference.width}x${reference.height} 比例${capped ? '（受长边/像素上限限制，比例已尽力贴近）' : ''}`,
	};
}

/** 解析画面比例预设，未指定或无法识别时返回参照比例（再退化为 1:1） */
export function resolveDimensionPlan(ratio?: string, reference?: { width: number; height: number }): DimensionPlan {
	const key = String(ratio || '').trim().replace(/[×xX]/g, ':').replace(/\s/g, '');
	const preset = RATIO_PRESETS[key];
	if (preset) return { width: preset.w, height: preset.h, origin: `${key} 预设` };
	return planDimensions(reference);
}

/** 规整绘制者传入的图生图强度：收进 0.05~0.95，缺省取 0.5 并记录（不再随机） */
function normalizeStrength(value: unknown, label: string): number {
	const num = Number(value);
	if (Number.isFinite(num)) {
		const clamped = Math.min(Math.max(num, 0.05), 0.95);
		if (Math.abs(clamped - num) > 1e-6) console.log(`[绘制者] ${label}强度 ${num} 越界，已收进 ${clamped}`);
		return clamped;
	}
	console.log(`[绘制者] ${label}未指定强度，按中性值 ${DEFAULT_STRENGTH} 处理`);
	return DEFAULT_STRENGTH;
}

/** 规整绘制者传入的迭代步数：夹在 8~60；未指定返回 0（交引擎按当前方案推荐步数/配置 sd_steps 归一化） */
function normalizeSteps(value: unknown, label: string): number {
	const num = Number(value);
	if (!Number.isFinite(num) || num <= 0) {
		console.log(`[绘制者] ${label}未指定步数，按当前方案推荐步数处理`);
		return 0;
	}
	const steps = Math.round(Math.min(Math.max(num, STEPS_MIN), STEPS_MAX));
	if (steps !== Math.round(num)) console.log(`[绘制者] ${label}步数 ${num} 越界，已收进 ${steps}`);
	return steps;
}

/** 绘画作品详情记录（用于向对话者传递作品信息） */
interface PaintingDetail {
	/** 工具名称 */
	toolName: string;
	/** 正向提示词摘要 */
	promptSummary: string;
	/** 立绘角色（portrait 专用：月华 / 琉璃 / 其他） */
	character?: string;
	/** 是否 Q 版（portrait 专用） */
	chibi?: boolean;
	/** 表情（人物像专用） */
	expression?: string;
	/** 姿势（人物像专用） */
	posture?: string;
	/** 环境（人物像专用） */
	environment?: string;
	/** 合照参与者（合照专用） */
	roles?: string[];
	/** 合照角色的表情/动作/服饰规划（合照专用） */
	planning?: string;
}

/** 角色档案（内置角色与自定义角色在绘制侧的统一视图） */
interface CharacterProfile extends CustomCharacterProfile {
	/** 是否为内置角色（内置角色使用专属立绘模板，自定义角色走通用模板） */
	builtin: boolean;
}

/** 外观模板占位符填充参数 */
interface AppearanceParts {
	/** 表情提示词 */
	expression?: string;
	/** 姿势提示词 */
	posture?: string;
	/** 服装提示词 */
	outfit?: string;
	/** 环境提示词 */
	environment?: string;
}

/** 绘制者角色 */
export class PainterRole extends CreativeRoleBase<PaintingDetail> {
	/** 智能体角色标签（TUI 实况屏显示） */
	protected roleTag: string = "绘制者";
	/** 公用提示词（默认表情/姿势/负面/合照模板与背景，见 prompts/painterCommon.md） */
	private readonly commonPrompts = loadPromptDocument('prompts/painterCommon.md')
	/** 角色提示词（外观/默认服装/立绘模板，见 prompts/painterCharacters.md） */
	private readonly characterPrompts = loadPromptDocument('prompts/painterCharacters.md')
	/** 默认表情提示 */
	private readonly defaultExpressionPrompt = this.commonPrompts.list('默认表情')
	/** 默认姿势提示 */
	private readonly defaultPosturePrompt = this.commonPrompts.list('默认姿势')
	/** 月华基础外观（发型/瞳色/发饰等比例无关特征，三套人设模板共用） */
	private readonly appearanceBasePrompt = this.characterPrompts.text('月华/基础外观')
	/** Q版（二头身）自我外观模板 */
	private readonly chibiAppearancePrompt = this.characterPrompts.text('月华/立绘-Q版')
	/** 全尺寸（全身立绘）自我外观模板 */
	private readonly fullAppearancePrompt = this.characterPrompts.text('月华/立绘-全尺寸')
	/** 琉璃基础外观（发型/瞳色/发饰等比例无关特征，全尺寸与Q版立绘共用） */
	private readonly crystalBasePrompt = this.characterPrompts.text('琉璃/基础外观')
	/** 琉璃全尺寸立绘外观模板 */
	private readonly crystalFullAppearancePrompt = this.characterPrompts.text('琉璃/立绘-全尺寸')
	/** 琉璃Q版立绘外观模板 */
	private readonly crystalChibiAppearancePrompt = this.characterPrompts.text('琉璃/立绘-Q版')
	/** 通用角色底板（女 / 男）：绘制月华与琉璃之外的人物，画风与两位自家人保持一致 */
	private readonly portraitBaseFemalePrompt = this.characterPrompts.text('通用角色/底板-女')
	private readonly portraitBaseMalePrompt = this.characterPrompts.text('通用角色/底板-男')
	/** 通用立绘模板（男 / 女共用，{gender} 由底板决定），全尺寸与Q版各一套 */
	private readonly genericFullAppearancePrompt = this.characterPrompts.text('通用角色/立绘-全尺寸')
	private readonly genericChibiAppearancePrompt = this.characterPrompts.text('通用角色/立绘-Q版')
	/** Q版默认服装提示词（黑色蕾丝洛丽塔连衣裙，黑色蕾丝袖口，胸前黑色蕾丝蝴蝶结配红宝石，无白色装饰） */
	private readonly defaultOutfitChibiPrompt = this.characterPrompts.text('月华/默认服装-Q版')
	/** 全尺寸默认服装提示词（黑色哥特萝莉风长裙，黑色蕾丝立领与袖口，胸前红宝石蝴蝶结饰扣，无白色装饰） */
	private readonly defaultOutfitFullPrompt = this.characterPrompts.text('月华/默认服装-全尺寸')
	/** 琉璃全尺寸默认服装提示词（白+冰蓝星星主题：星纹吊带裙、外披宽松白外套、白袜白乐福鞋） */
	private readonly crystalDefaultOutfitFullPrompt = this.characterPrompts.text('琉璃/默认服装-全尺寸')
	/** 琉璃Q版默认服装提示词（同一主题的Q版简化：保留星纹吊带裙与缎带蝴蝶结，外套更短） */
	private readonly crystalDefaultOutfitChibiPrompt = this.characterPrompts.text('琉璃/默认服装-Q版')
	/** 合照模板（{count} / {members} / {planning} / {environment}） */
	private readonly groupPhotoTemplate = this.commonPrompts.text('合照模板')
	/** 合照预设背景（未指定环境时随机挑选） */
	private readonly groupEnvironmentPresets = this.commonPrompts.list('合照背景')
	/** 默认负面提示词（立绘与图生图共用） */
	private readonly defaultNegativePrompt = this.commonPrompts.text('默认负面提示词')
	/** 内置角色档案（月华 / 琉璃）：立绘与合照共用同一份外观与默认服装 */
	private readonly builtinCharacters: Map<string, CharacterProfile> = new Map([
		['月华', {
			name: '月华', gender: '女', appearance: this.appearanceBasePrompt,
			outfitFull: this.defaultOutfitFullPrompt, outfitChibi: this.defaultOutfitChibiPrompt, builtin: true,
		}],
		['琉璃', {
			name: '琉璃', gender: '女', appearance: this.crystalBasePrompt,
			outfitFull: this.crystalDefaultOutfitFullPrompt, outfitChibi: this.crystalDefaultOutfitChibiPrompt, builtin: true,
		}],
	])
	/**
	 * 自定义角色库（SQLite knowledge.db → painter_characters 表）
	 *
	 * 构造时先加载一次，供工具描述生成角色目录；每次创作任务开始时再刷新，
	 * 避免启动时数据库未就绪或其它会话新增的角色在当前实例中不可见。
	 */
	private customCharacters: Map<string, CustomCharacterProfile> = loadCustomCharacters()
	/**
	 * 本次任务的参考图（对话者经 dispatch_painter 的 reference_tag 指定，宿主按标签取自资源目录数据库）
	 *
	 * 任务级状态：每次 createCreativeWork 开工时清空，任务内有效。
	 * referenceImage 为参考图在内容仓库中的相对 LocalDir 路径（即归档 blob，直接用作图生图基座），
	 * 但**它只交给扩散引擎**：模型自己看到的参考图是 referenceVision（视觉读取副本，
	 * ≤ vision_max_side），原图字节永不进入思维链（见 docs/adr/0008）。
	 */
	private referenceImage: string = '';
	/** 参考图的句柄（@I.00x，与资源目录数据库中的记录对应；未附带参考图时为空） */
	private referenceHandle: string = '';
	/** 参考图视觉副本（端点 URL 或 data URI，由 agent.image_input 决定）：唯一可以放进 image_url 的参考图形态 */
	private referenceVision: string = '';
	/** 参考图比例基准（取自视觉副本，与原图等比；图生图按此推导输出尺寸） */
	private referenceSize: { width: number; height: number } | null = null;
	/** 最近一次生成作品的本地路径（绝对路径，供超分工具对最新作品重建） */
	private lastResultPath: string = '';
	/** 绘画角色工具 */
	private readonly roleTool: ToolCall[] = [
		{
			type: "function",
			function: {
				name: "diffusion_generation",
				description: "根据文本描述进行文生图。仅适用于风景、物品、场景、氛围图等与人物形象无关的创作；严禁用于绘制任何人物形象(月华、琉璃或其他人物都必须调用 portrait)。",
				parameters: {
					type: "object",
					properties: {
						"prompt": {
							type: "string",
							description: "图像生成的正向描述文本"
						},
						"ratio": {
							type: "string",
							enum: ["1:1", "4:3", "3:4", "16:9", "9:16", "3:2", "2:3"],
							description: "画面比例预设,默认 1:1。按题材选择:人物/物品用 2:3 或 3:4,风景/横构图用 3:2 或 16:9,方形构图用 1:1。"
						},
						"steps": {
							type: "number",
							description: "迭代步数,取值 8~60:quality 方案建议 30~60(常规 40,精细 50~60);turbo 方案建议 8~12。省略时使用当前模型方案的推荐步数。越高细节越好但耗时线性增加,不影响构图"
						},
						"negative_prompt": {
							type: "string",
							description: "负面提示文本,用于排除图像中不希望出现的元素"
						},
						"cfg_scale": {
							type: "number",
							description: "提示词权重调节参数(0~2),默认值为 1.0"
						},
						"super_resolution": {
							type: "boolean",
							description: "生成完成后追加 RealESRGAN 超分辨率重建(4x),提升清晰度与细节。用户要求高清、精细画质时开启,耗时增加"
						}
					},
					required: [
						"prompt"
					]
				}
			}
		},
		{
			type: "function",
			function: {
				name: "image_to_image",
				description: "基于参考图进行图生图创作:以参考图为底图,按文本描述改变风格、场景、服饰或细节,同时保留参考图的主体特征。无参考图时不可调用:人物形象改用 portrait,非人物改用 diffusion_generation,或说明需要用户提供参考图片。",
				parameters: {
					type: "object",
					properties: {
						"prompt": {
							type: "string",
							description: "图像生成的正向描述文本,描述希望参考图变成什么样子"
						},
						"strength": {
							type: "number",
							description: "图生图强度,取值范围 0 到 1,越大越偏离参考图。必须由你根据创作意图指定:只改画风/光照/质感用 0.2~0.4;换服饰/换配色/微调细节用 0.4~0.6;换背景/换地点/大幅改场景 用 0.8~0.9 推荐 0.85;替换画面里的一个对象(如把北极熊换成熊猫)同样用 0.8~0.9——实测 0.65/0.75 换不掉大物体。注意:强度≥0.8 时提示词主导画面,没写进提示词的东西会消失,因此提示词必须描述整个画面(含所有要保留的主体)。缺省按中性值 0.5"
						},
						"steps": {
							type: "number",
							description: "迭代步数,取值 8~60:quality 方案建议 30~60(常规 40,精细 50~60);turbo 方案建议 8~12。省略时使用当前模型方案的推荐步数。越高细节越好但耗时线性增加,不影响构图"
						},
						"negative_prompt": {
							type: "string",
							description: "负面提示文本,用于排除图像中不希望出现的元素(换场景时点名去掉旧场景元素,如 beach, ocean, water, sea)"
						},
						"cfg_scale": {
							type: "number",
							description: "提示词引导系数(0~2),默认 1.0。当前模型为引导蒸馏模型,图生图同样保持 1.0;调高会发灰、过曝、塑料感。需要改得更多时调 strength,不要调 cfg"
						},
						"super_resolution": {
							type: "boolean",
							description: "生成完成后追加 RealESRGAN 超分辨率重建(4x),提升清晰度与细节。用户要求高清、精细画质时开启,耗时增加"
						}
					},
					required: [
						"prompt"
					]
				}
			}
		},
		{
			type: "function",
			function: {
				name: "portrait",
				description: "生成指定角色的立绘。凡是要求绘制月华、琉璃或其他人物的形象时,都必须且只能调用本函数,禁止用 diffusion_generation 或 image_to_image 代替",
				parameters: {
					type: "object",
					properties: {
						"character": {
							type: "string",
							description: "目标角色:内置角色(月华/琉璃)或已创建的自定义角色名,已内置外观,只需给出表情/姿势/环境;通用-女 / 通用-男 或临时角色必须用 appearance 描述其外观。{characters}"
						},
						"chibi": {
							type: "boolean",
							description: "是否Q版:true=二头身萌系比例(贴纸/表情包/头像),false 或不填=标准头身比的全身立绘"
						},
						"appearance": {
							type: "string",
							description: "character=通用-女 / 通用-男 或临时角色时填写该人物的外观:性别、年龄感、发型发色、瞳色、服装、配饰、身份。内置/自定义角色不需要填写。"
						},
						"expression": {
							type: "string",
							description: "表情提示词,描述想要展现的表情"
						},
						"posture": {
							type: "string",
							description: "动作提示词,描述想要展现的姿势或动作"
						},
						"outfit": {
							type: "string",
							description: "服装提示词。不提供则用该角色的默认服装（通用角色仅在 appearance 未写服装时兜底）"
						},
						"environment": {
							type: "string",
							description: "环境提示词,描述背景或场景。月华偏暗色与红,琉璃偏淡蓝天空与星光"
						},
						"ratio": {
							type: "string",
							enum: ["1:1", "4:3", "3:4", "16:9", "9:16", "3:2", "2:3"],
							description: "画面比例预设。缺省:全尺寸 2:3(竖版全身立绘),Q版 1:1。横版场景改 3:2 或 16:9。"
						},
						"steps": {
							type: "number",
							description: "迭代步数,取值 8~60:quality 方案建议 30~60(常规 40,精细 50~60);turbo 方案建议 8~12。省略时使用当前模型方案的推荐步数。越高细节越好但耗时线性增加,不影响构图"
						},
						"negative_prompt": {
							type: "string",
							description: "负面提示文本,用于排除图像中不希望出现的元素"
						},
						"cfg_scale": {
							type: "number",
							description: "提示词引导系数(0~2),本模型为引导蒸馏,默认 1.0,不要调高"
						},
						"super_resolution": {
							type: "boolean",
							description: "生成完成后追加 RealESRGAN 超分辨率重建(4x),提升清晰度与细节。用户要求高清、精细画质时开启,耗时增加"
						}
					},
					required: [
						"character",
						"expression",
						"posture",
						"environment"
					]
				}
			}
		},
		{
			type: "function",
			function: {
				name: "create_character",
				description: "创建或更新一个可复用的自定义角色,写入角色库(SQL数据库)。适合会反复出场的原创角色:创建后 portrait 的 character 与 group_photo 的 roles 都能直接用角色名引用,无需每次重写外观。一次性路人不建议创建。内置角色(月华/琉璃)不能重名",
				parameters: {
					type: "object",
					properties: {
						"name": {
							type: "string",
							description: "角色名(唯一键,后续绘制时用这个名字引用)"
						},
						"gender": {
							type: "string",
							enum: ["女", "男"],
							description: "性别,决定立绘模板中的性别表述"
						},
						"appearance": {
							type: "string",
							description: "基础外观:年龄感、发型发色、瞳色、面部特征、配饰、体型。这是角色的身份定义,不写表情/动作/场景"
						},
						"outfit_full": {
							type: "string",
							description: "默认全尺寸服装。省略则使用简洁得体的日常服装"
						},
						"outfit_chibi": {
							type: "string",
							description: "默认Q版服装。省略则复用全尺寸服装"
						}
					},
					required: [
						"name",
						"gender",
						"appearance"
					]
				}
			}
		},
		{
			type: "function",
			function: {
				name: "group_photo",
				description: "生成月华与一个或多个角色的合照。roles 按顺序列出参与者:内置角色与已创建的自定义角色直接写名字,自动继承外观与默认服装;临时角色写完整外观描述(性别、年龄感、发型发色、瞳色、服装、配饰、身份)。chibi=true 时所有参与者使用Q版二头身比例。planning 统一规划每人的表情/动作/服饰,例如\"月华:俏皮眨眼,比V手势,黑色哥特长裙;琉璃:安静微笑,双手交叠,白色星纹裙\";省略则每人使用随机默认表情/动作与默认服装。environment 省略时从预设背景中随机挑选。若本次任务附带参考图,将以该参考图为底图进行图生图,尽量保留参与者的容貌;未附带则按文本描述绘制",
				parameters: {
					type: "object",
					properties: {
						"roles": {
							type: "array",
							items: {
								type: "string"
							},
							description: "合照参与者列表,至少一位。内置角色与已创建的自定义角色直接写名字(自动继承外观与默认服装);临时角色写完整外观描述。{characters}"
						},
						"chibi": {
							type: "boolean",
							description: "是否Q版:true=所有参与者使用Q版二头身比例,false 或不填=全尺寸合照"
						},
						"planning": {
							type: "string",
							description: "合照的统一人物规划:逐人写清表情、动作与服饰,用分号分隔。省略则全部使用默认表情/动作与默认服装"
						},
						"environment": {
							type: "string",
							description: "环境提示词,描述背景环境或场景。省略时随机使用预设背景"
						},
						"ratio": {
							type: "string",
							enum: ["1:1", "4:3", "3:4", "16:9", "9:16", "3:2", "2:3"],
							description: "画面比例预设,默认 1:1。仅在未选定参考图时生效;已选定参考图时输出尺寸自动随参考图比例"
						},
						"strength": {
							type: "number",
							description: "已选定参考图时的图生图强度,取值 0 到 1,越大越偏离参考图。保留参与者容貌建议 0.3~0.5;缺省按中性值 0.5。未选定参考图时忽略"
						},
						"steps": {
							type: "number",
							description: "迭代步数,取值 8~60:quality 方案建议 30~60(常规 40,精细 50~60);turbo 方案建议 8~12。省略时使用当前模型方案的推荐步数。越高细节越好但耗时线性增加,不影响构图"
						},
						"negative_prompt": {
							type: "string",
							description: "负面提示文本,用于排除图像中不希望出现的元素"
						},
						"cfg_scale": {
							type: "number",
							description: "提示词权重调节参数,取值范围为 0 到 2,默认值为 1.0"
						},
						"super_resolution": {
							type: "boolean",
							description: "生成完成后追加 RealESRGAN 超分辨率重建（4x）,提升清晰度与细节。用户要求高清、精细画质时开启,耗时增加"
						}
					},
					required: [
						"roles"
					]
				}
			}
		},
		{
			type: "function",
			function: {
				name: "switch_diffusion_scheme",
				description: "切换后续绘图使用的模型方案。quality=质量优先(默认,细节更好、耗时更长);turbo=速度优先(出图更快,适合草图、快速确认与批量尝试,推荐 8~12 步)。切换后省略 steps 即使用该方案的推荐步数。切换状态由宿主记录,对所有后续绘图工具生效;任务本身不需要画得快时不要切换,保持当前方案即可",
				parameters: {
					type: "object",
					properties: {
						"scheme": {
							type: "string",
							enum: ["turbo", "quality"],
							description: "目标方案:turbo=速度优先,quality=质量优先"
						}
					},
					required: [
						"scheme"
					]
				}
			}
		},
		{
			type: "function",
			function: {
				name: "list_images",
				description: "查看资源目录中最近的图片句柄与内容描述（按登记时间倒序，越靠前越新）。清单只含文字信息，可用于了解历史图片、确认句柄或为超分定位目标。需要图生图基座时由对话者经 dispatch_painter 的 reference_tag 指定，任务消息会直接附上图片本体",
				parameters: {
					type: "object",
					properties: {}
				}
			}
		},
		{
			type: "function",
			function: {
				name: "upscale_image",
				description: "对已有图片执行 RealESRGAN 超分辨率放大（4x）,提升清晰度与细节。用户要求把某张图片变清晰、放大或提升画质时调用;不指定句柄时对最近一次生成的作品超分。与生成工具的 super_resolution 参数不同,本工具不重新生成图像,只做纯超分",
				parameters: {
					type: "object",
					properties: {
						"image_tag": {
							type: "string",
							description: "图片句柄（如 @I.003），对目录中指定图片超分。不填时对最近一次生成的作品超分"
						}
					}
				}
			}
		}
	]
	/** 构造函数 */
	public constructor() {
		super(fileView('prompts/painterRole.md')[0].trim());
	}
	/**
	 * 创作任务入口（覆写基类，接收对话者指定的参考图）
	 *
	 * 任务级参考图生命周期：每次开工时清空上一任务的参考图状态，再按
	 * 对话者经 dispatch_painter 的 reference_tag 指定的图片（宿主已按标签从
	 * 资源目录数据库解析出归档 blob 路径）绑定本次任务的基座图。
	 * 参考图本体随后由 buildTaskContent 以多模态形式随任务消息进入自己的历史。
	 *
	 * @param taskDescription 对话者传来的创作要求文本
	 * @param reference 参考图（句柄 + 归档 blob 相对路径）；纯文生图时省略
	 */
	public async createCreativeWork(taskDescription: string, reference?: { handle: string; source: string; desc?: string }): Promise<string> {
		// 每次任务开工刷新角色库：同一运行期内也能看到其它会话/进程新增的角色
		this.customCharacters = loadCustomCharacters();
		this.referenceImage = '';
		this.referenceHandle = '';
		this.referenceVision = '';
		this.referenceSize = null;
		if (reference?.source) {
			this.referenceHandle = reference.handle || '';
			this.referenceImage = reference.source;
			// 模型侧的参考图只能是视觉读取管线产物：原图（3072×4096 那种）会撑爆多模态模型的
			// 非因果注意力批、让整个 llama-server 实例断言退出，而且违反「思维链不流转原图」。
			// 缩放由 Go 侧视觉管线完成，这里（以及任何 JS 代码）没有缩放入口。
			// visionRead 只用于探测副本可生成并取得比例基准；真正进入 image_url 的是视觉读取管线产物
			// （url 形态为端点 URL，base64 形态为同一管线的 data URI）。
			const [, visionError, visionWidth, visionHeight] = visionRead(reference.source, this.referenceHandle);
			const visionURL = visionReference(this.referenceHandle, reference.source);
			if (visionError || !visionURL) {
				console.error('[绘制者] 参考图视觉副本生成失败，本次任务模型将看不到参考图:', visionError?.message ?? String(visionError ?? '无法构造视觉副本'));
			}
			else {
				this.referenceVision = visionURL;
				if (visionWidth > 0 && visionHeight > 0) this.referenceSize = { width: visionWidth, height: visionHeight };
			}
			console.log(`[绘制者] 本次任务参考图: ${this.referenceHandle || '(无句柄)'} → ${reference.source}${this.referenceSize ? `（视觉副本 ${this.referenceSize.width}x${this.referenceSize.height}）` : '（视觉副本生成失败）'}`);
		}
		return super.createCreativeWork(taskDescription);
	}

	/**
	 * 把任务要求与参考图**视觉副本**合成一条多模态任务消息送进自己的上下文（覆写基类钩子）
	 *
	 * 对话者的要求文本 + 视觉读取副本 —— 绘画者拿到的就是这两样东西，
	 * 模型在写提示词前能亲眼看到参考图（绘图是分钟级任务，看图只是秒级开销）。
	 *
	 * 副本由 createCreativeWork 阶段一次性解析成视觉读取管线产物（端点 URL / data URI），
	 * 这里只做拼装：不读原图、不缩放、不降级 —— 没有副本就退回纯文本。
	 *
	 * @param taskDescription 对话者传来的任务描述
	 * @returns 含参考图的内容项数组；无参考图时退化为纯文本
	 */
	protected buildTaskContent(taskDescription: string): PostMessage['content'] {
		if (!this.referenceVision) {
			if (this.referenceImage) console.log('[绘制者] 参考图无可用视觉副本，退回纯文本任务描述');
			return taskDescription;
		}
		console.log(`[绘制者] 参考图视觉副本已附入任务上下文（${Math.round(this.referenceVision.length / 1024)}KB）`);
		return [
			{ type: 'text', text: taskDescription },
			{ type: 'image_url', image_url: { url: this.referenceVision } }
		];
	}

	/** 当前任务的输出尺寸计划：有参考图时按参考图比例，否则用绘制者指定的比例预设 */
	private planFor(ratio?: string): DimensionPlan {
		const reference = this.referenceImage ? (this.referenceSize || undefined) : undefined;
		return resolveDimensionPlan(ratio, reference);
	}
	/** 参考图当前状态描述（供工具结果回显，避免模型在无参考图时反复尝试图生图） */
	private referenceStatus(): string {
		if (this.referenceImage) {
			const size = this.referenceSize ? `（${this.referenceSize.width}x${this.referenceSize.height}）` : '';
			const plan = this.planFor();
			return `本次任务参考图：${this.referenceHandle || this.referenceImage}${size}，输出尺寸将按参考图比例取 ${plan.width}x${plan.height}`;
		}
		return '本次任务尚未附带参考图';
	}
	/** 清空参考图状态（错误重置路径调用；任务级清理由 createCreativeWork 负责） */
	public resetReference(): void {
		this.referenceImage = '';
		this.referenceHandle = '';
		this.referenceVision = '';
		this.referenceSize = null;
	}
	/** 角色名称 */
	protected get roleName(): string { return '绘制者' }
	/**
	 * 获取工具定义
	 *
	 * 角色库是动态的（可能刚创建/更新过自定义角色），因此每轮把 `{characters}`
	 * 占位符替换成最新角色目录，让模型始终知道所有可用角色。
	 * 工具定义是纯数据（不含函数），JSON 往返可以安全地覆盖嵌套参数描述。
	 */
	protected getToolDefinitions(): ToolCall[] {
		const catalog = this.characterCatalogText();
		const definitions = JSON.stringify(this.roleTool).split('{characters}').join(catalog);
		return JSON.parse(definitions) as ToolCall[];
	}
	/** 执行绘画工具调用 */
	protected executeTool(toolCall: ToolCallItem): string {
		const funcName = toolCall.function.name;
		let args: Record<string, any> = {};
		try {
			args = typeof toolCall.function.arguments === 'string' ? JSON.parse(toolCall.function.arguments) : toolCall.function.arguments;
		}
		catch (parseError) {
			console.error(`[绘制者] 工具调用参数解析失败:`, toolCall.function.arguments);
			return `工具调用参数解析失败，请确保传入合法的 JSON 字符串。错误: ${parseError}`;
		}
		switch (funcName) {
			case 'diffusion_generation': return this.handleDiffusionGeneration(args as DiffusionGenerationParams);
			case 'image_to_image': return this.handleImageToImage(args as ImageToImageParams);
			case 'portrait': return this.handlePortrait(args as PortraitParams);
			case 'create_character': return this.handleCreateCharacter(args as CreateCharacterParams);
			case 'group_photo': return this.handleGroupPhoto(args as GroupPhotoParams);
			case 'switch_diffusion_scheme': return this.handleSwitchDiffusionScheme(args as SchemeSwitchParams);
			case 'list_images': return this.handleListImages();
			case 'upscale_image': return this.handleUpscaleImage(args as ImageTagParams);
			default: return `未知工具: ${funcName}，可用工具为 diffusion_generation、image_to_image、portrait、create_character、group_photo、switch_diffusion_scheme、list_images、upscale_image`;
		}
	}
	/** 从工具调用中提取绘画作品详情 */
	protected collectDetail(toolCall: ToolCallItem, paintings: PaintingDetail[]): void {
		try {
			const args = typeof toolCall.function.arguments === 'string'
				? JSON.parse(toolCall.function.arguments)
				: toolCall.function.arguments;
			const name = toolCall.function.name;
			if (name === 'portrait') {
				const character = String(args.character || '通用-女');
				paintings.push({
					toolName: name,
					promptSummary: `${this.characterLabel(character)}${args.chibi === true ? 'Q版' : '全尺寸'}立绘`,
					character: character,
					chibi: args.chibi === true,
					expression: args.expression || '',
					posture: args.posture || '',
					environment: args.environment || '',
				});
			}
			else if (name === 'group_photo') {
				const roles = this.normalizeGroupRoles(args.roles);
				paintings.push({
					toolName: name,
					promptSummary: `${args.chibi === true ? 'Q版' : ''}与${roles.join('、') || '他人'}的合照`,
					roles,
					chibi: args.chibi === true,
					planning: args.planning || '',
					environment: args.environment || '',
				});
			}
			else if (name === 'diffusion_generation' || name === 'image_to_image') {
				const prompt = args.prompt || '';
				/** 强度标注（图生图由绘制者指定，记入作品详情便于核对） */
				const strengthNote = name === 'image_to_image' && args.strength !== undefined
					? `（强度 ${normalizeStrength(args.strength, '图生图')}）`
					: '';
				paintings.push({
					toolName: name,
					promptSummary: (prompt.length > 100 ? prompt.slice(0, 97) + '...' : prompt) + strengthNote,
				});
			}
			else if (name === 'upscale_image') {
				const resolved = args.image_tag ? resolveImageRef(String(args.image_tag)) : null;
				paintings.push({
					toolName: name,
					promptSummary: resolved ? `${resolved.handle} 超分重建` : '最近作品超分重建',
				});
			}
		} catch {
			// 解析失败时跳过，不阻断流程
		}
	}
	/** 构建绘画作品摘要，使用月华话术格式 */
	protected buildSummary(paintings: PaintingDetail[]): string {
		// 检查是否有作品
		if (paintings.length === 0) return '月华没有绘制任何作品';
		// 事件 -> 绘制画作前：推送作品详情，插件可改写后再汇总
		const feedback: PaintingDetail[] = interactEvent('draw_painting_before', { paintings }).return;
		// 插件可改写作品详情，如添加乐器、速度、结构等
		if (feedback && Array.isArray(feedback) && feedback.every(p => p.toolName && p.promptSummary)) {
			paintings = feedback;
		}
		const parts: string[] = [];
		for (let i = 0; i < paintings.length; i++) {
			const p = paintings[i];
			if (p.toolName === 'portrait') {
				/** 角色 → 话术：月华说「自画像」，琉璃与他人说「立绘」 */
				let desc: string;
				if (p.character === '月华') desc = p.chibi ? '月华绘制了一幅Q版自画像' : '月华绘制了一幅全尺寸自画像';
				else if (p.character === '琉璃') desc = `月华为琉璃绘制了一幅${p.chibi ? 'Q版' : '全尺寸'}立绘`;
				else desc = `月华绘制了一幅${p.chibi ? 'Q版' : '全尺寸'}${this.characterLabel(p.character)}立绘`;
				if (p.expression) desc += `，展现了${p.expression}`;
				if (p.environment) desc += `，背景是${p.environment}`;
				parts.push(desc + '。');
			}
			else if (p.toolName === 'group_photo') {
				let desc = `月华绘制了一幅${p.chibi ? 'Q版' : ''}与${p.roles && p.roles.length > 0 ? p.roles.join('、') : '他人'}的合照`;
				if (p.planning) desc += `，${p.planning}`;
				if (p.environment) desc += `，背景是${p.environment}`;
				parts.push(desc + '。');
			}
			else if (p.toolName === 'image_to_image') parts.push(`月华参照参考图绘制了一幅图像：${p.promptSummary}。`);
			else if (p.toolName === 'upscale_image') parts.push(`月华为${p.promptSummary}提升了清晰度，图像已变得更精细。`);
			else parts.push(`月华绘制了一幅图像：${p.promptSummary}。`);
		}
		parts.push('图像已通过前端推送给用户。');
		return parts.join('\n');
	}
	/**
	 * 将基础外观、表情、服装、姿势、环境写入外观模板，得到完整提示词
	 *
	 * @param template 场景外观模板（Q版/全尺寸/合照/琉璃立绘）
	 * @param parts 占位符填充参数，缺省时使用随机表情、随机姿势与默认服装
	 * @param defaultOutfit 该场景的默认服装提示词
	 * @param base 该角色的基础外观（月华 / 琉璃各自一套），缺省为月华
	 */
	private renderAppearance(template: string, parts: AppearanceParts, defaultOutfit: string, base?: string, gender?: string): string {
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
	private renderTemplate(template: string, vars: Record<string, string>): string {
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
	/** 执行图像生成并推送至前端，返回本次生成的结果描述 */
	private paint(imageParams: GenerateImageParams, label: string): string {
		const [result, error] = generateImage(imageParams);
		if (error) {
			console.error(`[绘制者] ${label}失败:`, error);
			return `${label}失败: ${error}`;
		}
		if (!result || !result.base64) return `${label}失败：引擎返回空结果`;
		// 记录作品路径，供超分工具对最近作品重建
		this.lastResultPath = result.path;
		// 画作写入自己的历史（随 40 条滚动窗口保留，超限自然抛弃）
		this.recordPainting(label, result.base64);
		/** 实际生效的步数（0 表示交引擎按 sd_steps 处理） */
		const stepsNote = imageParams.steps ? `，${imageParams.steps} 步` : '';
		console.log(`[绘制者] ${label}成功，尺寸: ${result.width}x${result.height}${stepsNote}${imageParams.superResolution ? '（含超分重建）' : ''}`);
		if (!pushImage([result.base64])) console.warn(`[绘制者] 推送${label}结果到前端失败`);
		return `${label}成功。图片尺寸: ${result.width}x${result.height}，seed: ${result.seed}${stepsNote}${imageParams.superResolution ? '，已附带超分辨率重建' : ''}`;
	}
	/**
	 * 把刚完成的作品写入自己的历史（多模态消息）
	 *
	 * 绘画者能在自己的 40 条历史中看到自己画过的每一幅画（超出窗口的内容直接抛弃）。
	 *
	 * 作品同样必须经视觉管线入库再回取副本：扩散引擎吐出的原图（1024×1536 的 PNG，
	 * 动辄数 MB）直接进 image_url 就是又一条"原图流转"，会撑爆多模态实例。
	 * attachImageTags 负责入库 + 改写为视觉副本；入库失败时该项退化为文本占位。
	 *
	 * @param label 生成类目（如「图生图生成」「合照生成」）
	 * @param base64 作品图片数据（data URI 或裸 base64）
	 */
	private recordPainting(label: string, base64: string): void {
		if (!base64) return;
		try {
			const uri = base64.startsWith('data:') ? base64 : `data:image/png;base64,${base64}`;
			const message: PostMessage = {
				role: 'user',
				content: [
					{ type: 'text', text: `【${label}】这是你刚完成的作品` },
					{ type: 'image_url', image_url: { url: uri } }
				]
			};
			attachImageTags(message);
			this.writeContext(message);
		}
		catch (error) {
			console.error('[绘制者] 画作写入历史失败:', error);
		}
	}
	/** 处理通用扩散图像生成（文生图）：画面比例与步数由绘制者选定，比例缺省 1:1 */
	private handleDiffusionGeneration(args: DiffusionGenerationParams): string {
		try {
			const prompt = args.prompt || '';
			if (!prompt.trim()) return '扩散生成失败：正向提示词不能为空';
			/** 输出尺寸计划（文生图无参考图，取比例预设） */
			const plan = resolveDimensionPlan(args.ratio);
			/** 迭代步数（0 表示交引擎按 sd_steps 处理） */
			const steps = normalizeSteps(args.steps, '扩散生成');
			console.log(`[绘制者] 扩散生成 - 正向提示词: ${prompt.slice(0, 100)}...，画面比例: ${plan.origin} → ${plan.width}x${plan.height}，步数: ${steps || '默认'}`);
			return this.paint({
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
	private resolveImg2ImgCfg(value: unknown): number {
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
	private handleImageToImage(args: ImageToImageParams): string {
		try {
			const prompt = args.prompt || '';
			if (!prompt.trim()) return '图生图失败：正向提示词不能为空';
			if (!this.referenceImage) {
				return `图生图失败：${this.referenceStatus()}。图生图基座由对话者经 dispatch_painter 的 reference_tag 指定；当前任务无参考图时请改用 diffusion_generation 进行文生图`;
			}
			const strength = normalizeStrength(args.strength, '图生图');
			const steps = normalizeSteps(args.steps, '图生图');
			const cfgScale = this.resolveImg2ImgCfg(args.cfg_scale);
			// 比例随参考图：预算内保持比例忠实，短边不低于 512
			const plan = this.planFor();
			console.log(`[绘制者] 图生图 - 正向提示词: ${prompt.slice(0, 100)}...，参考图: ${this.referenceImage}，强度: ${strength}，cfg: ${cfgScale}，尺寸: ${plan.origin} → ${plan.width}x${plan.height}，步数: ${steps || '默认'}`);
			return this.paint({
				prompt: prompt,
				negativePrompt: args.negative_prompt || '',
				width: plan.width,
				height: plan.height,
				steps: steps,
				cfgScale: cfgScale,
				initImg: this.referenceImage,
				strength: strength,
				superResolution: args.super_resolution === true,
			}, '图生图生成');
		}
		catch (error) {
			console.error('[绘制者] 图生图处理异常:', error);
			return `图生图生成异常: ${error}`;
		}
	}
	/** 角色名 → 人话（作品摘要与日志用；自定义角色直接显示其名字） */
	private characterLabel(character: string | undefined): string {
		const name = String(character ?? '').trim();
		switch (name) {
			case '月华': return '月华';
			case '琉璃': return '琉璃';
			case '通用-男': return '男性角色';
			case '通用-女': return '女性角色';
			default: return name || '角色';
		}
	}
	/**
	 * 全部可用角色（内置 + 自定义）
	 *
	 * 每次调用都重新合并：创建自定义角色后，下一轮工具描述立即能看到新角色。
	 */
	private characterProfiles(): Map<string, CharacterProfile> {
		const profiles = new Map(this.builtinCharacters);
		for (const [name, profile] of this.customCharacters) {
			profiles.set(name, { ...profile, builtin: false });
		}
		return profiles;
	}
	/** 角色目录文本（替换工具描述里的 {characters} 占位符） */
	private characterCatalogText(): string {
		const names = [...this.characterProfiles().keys()];
		return names.length > 0
			? `当前可用角色：${names.join('、')}；也可用 通用-女 / 通用-男 临时描述外观`
			: '当前可用角色：通用-女 / 通用-男（临时描述外观）';
	}
	/** 角色默认服装：Q版优先取 Q 版服装，缺省回退全尺寸，再缺省用日常服装 */
	private characterOutfit(profile: CharacterProfile, chibi: boolean): string {
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
	private portraitPreset(args: PortraitParams): {
		kind: string; chibi: boolean; label: string; template: string;
		base: string; outfit: string; gender: string; defaultRatio: string;
	} {
		const raw = String(args.character ?? '').trim();
		const chibi = args.chibi === true;
		const defaultRatio = chibi ? DEFAULT_RATIO : '2:3';
		if (raw === '月华') {
			return {
				kind: '月华', chibi, label: `月华${chibi ? 'Q版' : '全尺寸'}自画像`,
				template: chibi ? this.chibiAppearancePrompt : this.fullAppearancePrompt,
				base: this.appearanceBasePrompt,
				outfit: chibi ? this.defaultOutfitChibiPrompt : this.defaultOutfitFullPrompt,
				gender: '女孩', defaultRatio,
			};
		}
		if (raw === '琉璃') {
			return {
				kind: '琉璃', chibi, label: `琉璃${chibi ? 'Q版' : '全尺寸'}立绘`,
				template: chibi ? this.crystalChibiAppearancePrompt : this.crystalFullAppearancePrompt,
				base: this.crystalBasePrompt,
				outfit: chibi ? this.crystalDefaultOutfitChibiPrompt : this.crystalDefaultOutfitFullPrompt,
				gender: '女孩', defaultRatio,
			};
		}
		/** 自定义角色：角色库里的基础外观 + 默认服装，复用通用男女模板 */
		const custom = this.customCharacters.get(raw);
		if (custom) {
			const profile: CharacterProfile = { ...custom, builtin: false };
			return {
				kind: raw, chibi, label: `${raw}${chibi ? 'Q版' : '全尺寸'}立绘`,
				template: chibi ? this.genericChibiAppearancePrompt : this.genericFullAppearancePrompt,
				base: profile.appearance,
				outfit: this.characterOutfit(profile, chibi),
				gender: profile.gender === '男' ? '男孩' : '女孩',
				defaultRatio,
			};
		}
		/** 通用角色：通用男女底板 + 绘制者给出的外观描述 */
		const male = raw === '通用-男';
		/** 未登记的角色名（既不是内置也不是自定义角色），也当作外观描述的一部分保留下来 */
		const named = raw && raw !== '通用-女' && raw !== '通用-男' && raw !== '其他' ? `名为「${raw}」的角色` : '';
		const desc = [named, String(args.appearance ?? '').trim()].filter(t => t).join('，');
		const base = (male ? this.portraitBaseMalePrompt : this.portraitBaseFemalePrompt) + (desc ? `，${desc}` : '');
		if (!desc) console.log('[绘制者] 通用角色未提供 appearance：仅按通用底板绘制，人物特征会很泛');
		return {
			kind: male ? '通用-男' : '通用-女', chibi, label: `${chibi ? 'Q版' : '全尺寸'}${male ? '男性' : '女性'}角色立绘`,
			template: chibi ? this.genericChibiAppearancePrompt : this.genericFullAppearancePrompt,
			base,
			outfit: desc ? '' : '穿着简洁得体的日常服装',
			gender: male ? '男孩' : '女孩', defaultRatio,
		};
	}
	/** 处理立绘生成（月华 / 琉璃 / 通用角色，全尺寸与Q版共用同一入口） */
	private handlePortrait(args: PortraitParams): string {
		try {
			const preset = this.portraitPreset(args);
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
			const fullPrompt = this.renderAppearance(preset.template, args, preset.outfit, preset.base, preset.gender);
			return this.paint({
				prompt: fullPrompt,
				negativePrompt: args.negative_prompt || this.defaultNegativePrompt,
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
	/** 创建或更新自定义角色（写入 SQLite 角色库，供立绘与合照共用） */
	private handleCreateCharacter(args: CreateCharacterParams): string {
		const name = String(args.name || '').trim();
		const appearance = String(args.appearance || '').trim();
		if (!name) return '创建角色失败：角色名不能为空';
		if (!appearance) return '创建角色失败：appearance 不能为空（写清年龄感、发型发色、瞳色、面部特征、配饰、体型）';
		if (name === '月华' || name === '琉璃' || name === '通用-女' || name === '通用-男') {
			return `创建角色失败：「${name}」是内置角色名，请换一个名字`;
		}
		const existed = this.customCharacters.has(name);
		const profile: CustomCharacterProfile = {
			name,
			gender: args.gender === '男' ? '男' : '女',
			appearance,
			outfitFull: String(args.outfit_full || '').trim() || undefined,
			outfitChibi: String(args.outfit_chibi || '').trim() || undefined,
		};
		if (!saveCustomCharacter(profile)) return '创建角色失败：角色库写入失败，请稍后重试';
		this.customCharacters.set(name, profile);
		console.log(`[绘制者] 自定义角色${existed ? '更新' : '创建'}成功: ${name}`);
		return `${existed ? '已更新' : '已创建'}自定义角色「${name}」（${profile.gender}）。之后绘制立绘或合照时可直接用该名字引用。`;
	}
	/**
	 * 处理合照生成
	 *
	 * 参与者由 roles 定义：内置角色与自定义角色继承外观与默认服装，临时角色按传入
	 * 字符串原样使用。chibi=true 时所有参与者使用 Q 版比例。表情/动作/服饰由
	 * planning 统一规划；未提供时对已知角色使用随机默认表情/姿势与默认服装。
	 * environment 未提供时从预设背景中随机挑选。
	 * 与旧版一致：本次任务附带参考图时以参考图为底图做图生图，比例随参考图。
	 */
	private handleGroupPhoto(args: GroupPhotoParams): string {
		try {
			const roles = this.normalizeGroupRoles(args.roles);
			if (roles.length === 0) return '合照生成失败：roles 至少需要一位参与者';
			const chibi = args.chibi === true;
			const planningProvided = !!String(args.planning || '').trim();
			const planning = String(args.planning || '').trim() || this.defaultGroupPlanning(roles, chibi);
			const environment = String(args.environment || '').trim() || this.randomGroupEnvironment();
			const members = roles
				.map(role => this.groupMemberAppearance(role, chibi, !planningProvided))
				.join('；');
			const fullPrompt = this.renderTemplate(this.groupPhotoTemplate, {
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
			const useReference = !!this.referenceImage;
			// 有参考图时比例随参考图；无参考图时按绘制者指定的比例预设（缺省 1:1）
			const plan = useReference ? this.planFor() : resolveDimensionPlan(args.ratio);
			/** 迭代步数（0 表示交引擎按 sd_steps 处理） */
			const steps = normalizeSteps(args.steps, '合照');
			const imageParams: GenerateImageParams = {
				prompt: fullPrompt,
				negativePrompt: args.negative_prompt || this.defaultNegativePrompt,
				width: plan.width,
				height: plan.height,
				steps: steps,
				cfgScale: args.cfg_scale ?? 1.0,
				superResolution: args.super_resolution === true,
			};
			// 参考图（本次任务附带）：以参考图为底图，尽量保留参与者容貌
			if (useReference) {
				imageParams.initImg = this.referenceImage;
				imageParams.strength = normalizeStrength(args.strength, '合照');
				console.log(`[绘制者] 合照图生图模式，参考图: ${this.referenceImage}，强度: ${imageParams.strength}，尺寸: ${plan.origin} → ${plan.width}x${plan.height}，步数: ${steps || '默认'}`);
			}
			return this.paint(imageParams, '合照生成');
		}
		catch (error) {
			console.error('[绘制者] 合照生成处理异常:', error);
			return `合照生成异常: ${error}`;
		}
	}
	/** 规整合照参与者：接受数组或单个字符串，去空白、去重、保序 */
	private normalizeGroupRoles(value: unknown): string[] {
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
	private groupMemberAppearance(role: string, chibi: boolean, withOutfit: boolean): string {
		const profile = this.characterProfiles().get(role);
		if (!profile) return role;
		const outfit = withOutfit ? `，${this.characterOutfit(profile, chibi)}` : '';
		return `${profile.name}，${profile.appearance}${outfit}`;
	}
	/** 未提供 planning 时的默认规划：已知角色用随机表情/姿势 + 当前比例下的默认服装，临时角色不额外追加 */
	private defaultGroupPlanning(roles: string[], chibi: boolean): string {
		const entries: string[] = [];
		const profiles = this.characterProfiles();
		for (const role of roles) {
			const profile = profiles.get(role);
			if (!profile) continue;
			entries.push(`${profile.name}：${this.randomExpression()}，${this.randomPosture()}，${this.characterOutfit(profile, chibi)}`);
		}
		return entries.join('；');
	}
	/** 随机取一条默认表情 */
	private randomExpression(): string {
		return this.defaultExpressionPrompt[RandomFloor(0, this.defaultExpressionPrompt.length - 1)];
	}
	/** 随机取一条默认姿势 */
	private randomPosture(): string {
		return this.defaultPosturePrompt[RandomFloor(0, this.defaultPosturePrompt.length - 1)];
	}
	/** 随机取一条预设合照背景 */
	private randomGroupEnvironment(): string {
		return this.groupEnvironmentPresets[RandomFloor(0, this.groupEnvironmentPresets.length - 1)];
	}
	/** 切换绘图模型方案（状态由 Go 层记录，与图像工坊前端共享） */
	private handleSwitchDiffusionScheme(args: SchemeSwitchParams): string {
		const name = String(args.scheme || '').trim();
		if (!name) return '切换模型方案失败：请指定 turbo 或 quality';
		const [current, error] = setDiffusionScheme(name);
		if (error) return `切换模型方案失败：${error.message ?? String(error)}`;
		return `已切换到 ${current}（${this.schemeLabel(current)}）模型方案，后续绘图将使用该方案。`;
	}
	/** 方案键名 → 人话 */
	private schemeLabel(name: string): string {
		switch (name) {
			case 'turbo': return '速度优先';
			case 'quality': return '质量优先';
			default: return name || '默认';
		}
	}
	/**
	 * 处理图片句柄清单查询（了解历史图片、确认句柄或为超分定位目标）
	 *
	 * 清单来源＝**资源目录数据库**（唯一权威）。需要图生图基座时由对话者经
	 * dispatch_painter 的 reference_tag 指定，任务消息会直接附上图片本体。
	 */
	private handleListImages(): string {
		const entries = listResources('I', LIST_LIMIT_MAX);
		if (entries.length === 0) return '资源目录中还没有任何图片：当前没有可引用的图片句柄';
		const lines = entries.map(entry => {
			const desc = entry.summary ? sanitizeImageDesc(entry.summary, 40) : '（暂无描述）';
			const lost = entry.state === 'active' ? '' : ' [本体已回收]';
			return `${entry.handle} ${desc}${lost}`;
		});
		return `资源目录中最近的 ${entries.length} 张图片（句柄越新越靠前）：\n${lines.join('\n')}\n可调用 upscale_image 对指定图片超分；图生图基座由对话者经 reference_tag 指定。${this.referenceStatus()}。`;
	}
	/** 处理独立超分：对最近生成作品或按句柄取图片执行 RealESRGAN 重建 */
	private handleUpscaleImage(args: ImageTagParams): string {
		try {
			const resolved = args.image_tag ? resolveImageRef(String(args.image_tag)) : null;
			let source = '';
			let handleLabel = '';
			if (args.image_tag) {
				if (!resolved) return '超分失败：句柄不存在或已失效，请先调用 list_images 或 list_resources 查看可用图片';
				if (!resolved.source) return `超分失败：${resolved.handle} 的本体已从内容仓库回收，无法超分`;
				source = resolved.source;
				handleLabel = resolved.handle;
			}
			else {
				if (!this.lastResultPath) return '超分失败：还没有生成过作品，请通过 image_tag 指定图片句柄，或先完成一次图像生成';
				source = this.lastResultPath;
				handleLabel = '最近作品';
			}
			console.log(`[绘制者] 超分重建 - 来源: ${handleLabel} ${source.substring(0, 60)}...`);
			const [result, error] = upscaleImage(source);
			if (error) {
				console.error('[绘制者] 超分失败:', error);
				return `超分失败: ${error}`;
			}
			if (!result || !result.base64) return '超分失败：引擎返回空结果';
			// 超分产物同样写入自己的历史（属于自己产出的一幅画作）
			this.recordPainting('超分重建', result.base64);
			if (!pushImage([result.base64])) console.warn('[绘制者] 推送超分结果到前端失败');
			return `超分成功。图片尺寸: ${result.width}x${result.height}，已推送给用户`;
		}
		catch (error) {
			console.error('[绘制者] 超分处理异常:', error);
			return `超分处理异常: ${error}`;
		}
	}
}