import { ToolCall } from '../../config/tool';
import { GlobalConfig } from '../../config/global';
import { RandomFloor } from '../../tool/math';
import { GenerateImageParams, DiffusionGenerationParams, ImageToImageParams, SelfPortraitParams, GroupPhotoParams, ImageTagParams, DimensionPlan } from '../../config/image';
import { ToolCallItem } from '../../config/model';
import { CreativeRoleBase } from '../base/creative';
import { interactEvent } from '../capabilities/ltp-event';
import { getCacheEntry, listCacheEntries, normalizeImageTag, sanitizeImageDesc, IMAGE_CACHE_CAPACITY } from '../capabilities/image-cache';

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
const DEFAULT_STRENGTH = 0.5;
/** 迭代步数区间与缺省：夹在 30~60，绘制者未指定时回落到配置 sd_steps */
const STEPS_MIN = 30;
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

/** 规整绘制者传入的迭代步数：夹在 30~60；未指定返回 0（交引擎按 sd_steps 归一化） */
function normalizeSteps(value: unknown, label: string): number {
	const num = Number(value);
	if (!Number.isFinite(num) || num <= 0) {
		console.log(`[绘制者] ${label}未指定步数，按配置 sd_steps 处理`);
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
	/** 表情（人物像专用） */
	expression?: string;
	/** 姿势（人物像专用） */
	posture?: string;
	/** 环境（人物像专用） */
	environment?: string;
	/** 合照对象（合照专用） */
	companion?: string;
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
	/** 合照对象描述（仅合照模板使用） */
	companion?: string;
}

/** 绘制者角色 */
export class PainterRole extends CreativeRoleBase<PaintingDetail> {
	/** 智能体角色标签（TUI 实况屏显示） */
	protected roleTag: string = "绘制者";
	/** 默认表情提示 */
	private readonly defaultExpressionPrompt = [
		'温柔的表情,开心的笑容,脸颊泛红',
		'害羞的表情,抿嘴微笑,眼神躲闪',
		'俏皮的表情,单眼眨眼,嘴角微微上扬,略带调皮的笑容',
		'平静的表情,眼神略微向下看向一侧,嘴唇轻抿,无笑容,若有所思',
		'惊讶的表情,双眼睁大,眉毛抬高,嘴巴微张成O形,脸颊泛红',
		'非常开心的表情,双眼弯成月牙形,张大嘴巴欢笑,脸颊泛红明显',
		'害羞的表情,眼神向下看,眉毛呈八字形,抿嘴微笑,脸颊大面积泛红',
		'自信的表情,眼神直视前方,眉毛微微上扬,嘴角带有一抹浅笑,眼神明亮'
	]
	/** 默认姿势提示 */
	private readonly defaultPosturePrompt = [
		'一条腿轻轻抬起,俏皮姿势',
		'双手背在身后,身体微微前倾,双脚并拢',
		'一手插在外套口袋里,另一只手轻抬至脸颊旁比"V"字手势,身体略侧,双脚前后交叉站立',
		'双手自然垂放于身前,手指轻轻交握,双肩微微内收,双脚并拢,站姿端正',
		'手抬起至嘴前,十指轻轻触碰"做捂嘴状",身体微微后仰,一条腿向后小半步,重心落在后脚',
		'双手举过头顶比心或张开五指,一条腿向后踢起,身体微微前倾,脚尖离地呈跳跃瞬间',
		'双手食指在胸前互点,头部微微低下,双膝内扣,两脚脚尖向内呈内八站姿',
		'双手叉腰,挺胸收腹,一条腿向侧方伸出,脚尖点地,身体笔直有力',
	]
	/** 月华基础外观（发型/瞳色/发饰等比例无关特征，三套人设模板共用） */
	private readonly appearanceBasePrompt = fileView('prompts/appearanceBase.md')[0]
	/** Q版（二头身）自我外观模板 */
	private readonly chibiAppearancePrompt = fileView('prompts/selfAppearanceChibi.md')[0]
	/** 全尺寸（全身立绘）自我外观模板 */
	private readonly fullAppearancePrompt = fileView('prompts/selfAppearanceFull.md')[0]
	/** 与他人合照外观模板 */
	private readonly groupAppearancePrompt = fileView('prompts/selfAppearanceGroup.md')[0]
	/** 琉璃固定形象（合照对象为琉璃时注入的内置外观描述，保证形象一致） */
	private readonly crystalAppearancePrompt = fileView('prompts/crystalAppearance.md')[0]
	/** Q版默认服装提示词（黑色蕾丝洛丽塔连衣裙，黑色蕾丝袖口，胸前黑色蕾丝蝴蝶结配红宝石，无白色装饰） */
	private readonly defaultOutfitChibiPrompt = '穿着纯黑色洛丽塔风连衣裙，长泡泡袖，袖口缀黑色蕾丝荷叶边，胸前系黑色蕾丝蝴蝶结且蝴蝶结中央镶嵌红色宝石，裙摆层叠黑色蕾丝花边，无任何白色蕾丝或白色装饰'
	/** 全尺寸默认服装提示词（黑色哥特萝莉风长裙，黑色蕾丝立领与袖口，胸前红宝石蝴蝶结饰扣，无白色装饰） */
	private readonly defaultOutfitFullPrompt = '穿着黑色哥特萝莉风长裙，黑色蕾丝立领，胸前系黑色蕾丝蝴蝶结且蝴蝶结中央镶嵌红色宝石饰扣，长泡泡袖收黑色蕾丝袖口，多层黑色蕾丝裙摆，无任何白色蕾丝或白色装饰'
	/** 默认负面提示词（人物像与图生图共用） */
	private readonly defaultNegativePrompt = '低分辨率, 糙噪点, 超现实主义, 丑陋的面部特征, 失真表情, 模糊轮廓, 颜色失衡, 不均匀光影, 强烈对比度, 过曝或欠曝, 杂乱背景, 像素化, 彩虹效果, 畸形肢体, 错位比例, 低质感纹理'
	/** 参考图（按图片标签从共享缓存选定）：本地图片路径（相对 LocalDir），非空时图生图与合照切换为图生图流程 */
	public referenceImage: string = '';
	/** 参考图是否由本次任务提供（对话者预置或本任务内 select_reference_image 选定）：false 时不得用于本任务 */
	private referencePrepared: boolean = false;
	/** 参考图的原始像素尺寸（图生图按此比例推导输出尺寸；未选定参考图时为 null） */
	private referenceSize: { width: number; height: number } | null = null;
	/** 最近一次生成作品的本地路径（绝对路径，供超分工具对最新作品重建） */
	private lastResultPath: string = '';
	/** 任务描述中对话者预置参考图的标准措辞（agent-control.ts 按此格式写入） */
	private static readonly REFERENCE_READY_PATTERN = /（用户指定 \[(图片\.\d+)\] 作为图生图基座，参考图已准备就绪：([^，）]+)/;
	/** 绘画角色工具 */
	private readonly roleTool: ToolCall[] = [
		{
			type: "function",
			function: {
				name: "diffusion_generation",
				description: "根据文本描述生成与月华本人无关的图像，纯文生图。仅适用于风景、物品、其他角色、场景等通用创作;严禁用于绘制月华自己的形象（绘制月华必须调用 self_portrait 或 chibi_self_portrait）。若需要参照某张已有图片创作,请先用 list_images 查看图片缓存并用 select_reference_image 按标签选定基座图,再改用 image_to_image",
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
							description: "画面比例预设,默认 1:1。按题材选择:人物/物品用 2:3 或 3:4,风景/横构图用 3:2 或 16:9,方形构图用 1:1。短边固定 1024px"
						},
						"steps": {
							type: "number",
							description: "迭代步数,取值 30~60:快速草图 30,常规 40（省略时用系统默认）,精细打磨 50~60。越高细节越好但耗时线性增加,不影响构图"
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
						"prompt"
					]
				}
			}
		},
		{
			type: "function",
			function: {
				name: "image_to_image",
				description: "基于当前选定的参考图进行图生图创作:以参考图为底图,按文本描述改变风格、场景、服饰或细节,同时保留参考图的主体特征。输出尺寸自动按参考图比例推导(短边不低于 1024px),无需也无法另行指定比例。尚未选定参考图时,请先调用 list_images 查看图片缓存并用 select_reference_image 按标签选定基座图;缓存中没有合适图片时改用 diffusion_generation 进行文生图",
				parameters: {
					type: "object",
					properties: {
						"prompt": {
							type: "string",
							description: "图像生成的正向描述文本,描述希望参考图变成什么样子"
						},
						"strength": {
							type: "number",
							description: "图生图强度,取值范围 0 到 1,越大越偏离参考图。必须由你根据创作意图指定:只改画风/光照/清晰度用 0.2~0.4;换场景/服饰但保留主体用 0.4~0.55;大幅改动姿态/构图用 0.6~0.8。缺省按中性值 0.5"
						},
						"steps": {
							type: "number",
							description: "迭代步数,取值 30~60:快速草图 30,常规 40（省略时用系统默认）,精细打磨 50~60。越高细节越好但耗时线性增加,不影响构图"
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
						"prompt"
					]
				}
			}
		},
		{
			type: "function",
			function: {
				name: "self_portrait",
				description: "生成月华的全尺寸自画像（全身立绘,标准少女头身比）。凡是要求绘制月华、你自己、\"我\"的形象,且未特别要求Q版时,都必须且只能调用此函数,禁止调用 diffusion_generation",
				parameters: {
					type: "object",
					properties: {
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
							description: "服装提示词,描述想要穿着的服装样式。不提供则使用默认服装"
						},
						"environment": {
							type: "string",
							description: "环境提示词,描述背景环境或场景"
						},
						"ratio": {
							type: "string",
							enum: ["1:1", "4:3", "3:4", "16:9", "9:16", "3:2", "2:3"],
							description: "画面比例预设,默认 1:1。全身立绘/坐姿建议 2:3,横构图场景建议 3:2 或 16:9。短边固定 1024px"
						},
						"steps": {
							type: "number",
							description: "迭代步数,取值 30~60:快速草图 30,常规 40（省略时用系统默认）,精细打磨 50~60。越高细节越好但耗时线性增加,不影响构图"
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
				name: "chibi_self_portrait",
				description: "生成月华的Q版自画像（二头身可爱比例）。当要求绘制Q版、二头身、萌系或大头娃娃风格的月华形象时调用此函数,禁止调用 diffusion_generation",
				parameters: {
					type: "object",
					properties: {
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
							description: "服装提示词,描述想要穿着的服装样式。不提供则使用默认服装"
						},
						"environment": {
							type: "string",
							description: "环境提示词,描述背景环境或场景"
						},
						"ratio": {
							type: "string",
							enum: ["1:1", "4:3", "3:4", "16:9", "9:16", "3:2", "2:3"],
							description: "画面比例预设,默认 1:1（Q版二头身适合方形或略宽）。短边固定 1024px"
						},
						"steps": {
							type: "number",
							description: "迭代步数,取值 30~60:快速草图 30,常规 40（省略时用系统默认）,精细打磨 50~60。越高细节越好但耗时线性增加,不影响构图"
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
				name: "group_photo",
				description: "生成月华与其他人的合照。当要求绘制月华和某人（朋友、家人、其他角色等）的合影时调用此函数。合照对象为琉璃时,companion 直接填写\"琉璃\"即可,系统会自动注入琉璃的固定形象。若已选定参考图,将以该参考图为底图进行图生图,尽量保留合照对象的容貌;未选定则按文本描述绘制",
				parameters: {
					type: "object",
					properties: {
						"companion": {
							type: "string",
							description: "合照对象描述,描述对方的性别、外貌、发型、服装、身份等特征。合照对象为琉璃时直接填写\"琉璃\""
						},
						"expression": {
							type: "string",
							description: "月华的表情提示词,描述想要展现的表情"
						},
						"posture": {
							type: "string",
							description: "月华的动作提示词,描述想要展现的姿势或动作"
						},
						"outfit": {
							type: "string",
							description: "月华的服装提示词,描述想要穿着的服装样式。不提供则使用默认服装"
						},
						"environment": {
							type: "string",
							description: "环境提示词,描述背景环境或场景"
						},
						"ratio": {
							type: "string",
							enum: ["1:1", "4:3", "3:4", "16:9", "9:16", "3:2", "2:3"],
							description: "画面比例预设,默认 1:1。仅在未选定参考图时生效;已选定参考图时输出尺寸自动随参考图比例"
						},
						"strength": {
							type: "number",
							description: "已选定参考图时的图生图强度,取值 0 到 1,越大越偏离参考图。保留对方容貌建议 0.3~0.5;缺省按中性值 0.5。未选定参考图时忽略"
						},
						"steps": {
							type: "number",
							description: "迭代步数,取值 30~60:快速草图 30,常规 40（省略时用系统默认）,精细打磨 50~60。越高细节越好但耗时线性增加,不影响构图"
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
						"companion",
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
				name: "list_images",
				description: "查看图片缓存中最近的图片清单（标签、内容描述、是否在多模态窗口内）。清单覆盖对话中出现过的所有静态图片与合并转发内的图片，按注册时间倒序。当用户要求参照某张已有图片创作、对某张图片加工或超分时，先调用此函数获取标签清单，结合需求分析后按标签选定目标。清单只含文字信息，不含图片内容",
				parameters: {
					type: "object",
					properties: {}
				}
			}
		},
		{
			type: "function",
			function: {
				name: "select_reference_image",
				description: "按图片标签从共享图片缓存中选定一张作为图生图基座图（参考图）。选定后 image_to_image 与合照将以该图保留主体特征。任务描述已注明\"参考图已准备就绪\"时无需再调用；标签可从 list_images 清单或任务描述中的 [图片.00x] 获取",
				parameters: {
					type: "object",
					properties: {
						"image_tag": {
							type: "string",
							description: "图片标签，如 图片.003（写作 [图片.003] 也可以）"
						}
					},
					required: [
						"image_tag"
					]
				}
			}
		},
		{
			type: "function",
			function: {
				name: "upscale_image",
				description: "对已有图片执行 RealESRGAN 超分辨率放大（4x）,提升清晰度与细节。用户要求把某张图片变清晰、放大或提升画质时调用;不指定标签时对最近一次生成的作品超分。与生成工具的 super_resolution 参数不同,本工具不重新生成图像,只做纯超分",
				parameters: {
					type: "object",
					properties: {
						"image_tag": {
							type: "string",
							description: "图片标签（如 图片.003），对缓存中指定图片超分。不填时对最近一次生成的作品超分"
						}
					}
				}
			}
		}
	]
	/** 构造函数 */
	public constructor() {
		super(fileView('prompts/painterRole.md')[0]);
	}
	/** 宿主桥接：把共享缓存中的图片源准备为图生图基座（对话者指定 reference_tag 时调用），返回本地路径（失败为空串） */
	public prepareReference(source: string): string {
		const saved = this.saveReferenceImage(source);
		if (saved.path) {
			this.referenceImage = saved.path;
			this.referencePrepared = true;
			this.referenceSize = saved.size;
		}
		return saved.path;
	}
	/**
	 * 任务级参考图生命周期：每个创作任务开工前清空上一任务的参考图，并识别任务描述中
	 * 对话者预置的基座图（`参考图已准备就绪：<path>`）。参考图不再跨任务永久生效——
	 * 否则一次旧图选定会污染之后所有 image_to_image 与合照。
	 */
	public createCreativeWork(taskDescription: string): Promise<string> {
		this.referenceImage = '';
		this.referencePrepared = false;
		this.referenceSize = null;
		const ready = (taskDescription || '').match(PainterRole.REFERENCE_READY_PATTERN);
		if (ready) {
			const path = ready[2].trim();
			const saved = this.loadReferenceSize(path);
			if (saved) {
				this.referenceImage = path;
				this.referencePrepared = true;
				this.referenceSize = saved;
				console.log(`[绘制者] 本次任务参考图已就绪: [${ready[1]}] → ${path}（${saved.width}x${saved.height}）`);
			}
			else {
				console.log(`[绘制者] 任务描述注明参考图 ${path}，但该文件无法读取，请重新选图`);
			}
		}
		return super.createCreativeWork(taskDescription);
	}
	/** 读取参考图尺寸（相对 LocalDir 路径 → 解码取宽高，失败返回 null） */
	private loadReferenceSize(relPath: string): { width: number; height: number } | null {
		try {
			const [resized, err] = resizeImage(relPath);
			if (err || !resized || resized.length === 0) return null;
			const size = { width: resized[0].width, height: resized[0].height };
			return size.width > 0 && size.height > 0 ? size : null;
		}
		catch (error) {
			console.error('[绘制者] 参考图尺寸读取失败:', error);
			return null;
		}
	}
	/** 当前任务的输出尺寸计划：有参考图时按参考图比例，否则用绘制者指定的比例预设 */
	private planFor(ratio?: string): DimensionPlan {
		const reference = this.referencePrepared && this.referenceImage ? (this.referenceSize || undefined) : undefined;
		return resolveDimensionPlan(ratio, reference);
	}
	/** 参考图当前状态描述（供工具结果回显，避免模型在无参考图时反复尝试图生图） */
	private referenceStatus(): string {
		if (this.referencePrepared && this.referenceImage) {
			const size = this.referenceSize ? `（${this.referenceSize.width}x${this.referenceSize.height}）` : '';
			const plan = this.planFor();
			return `本次任务参考图：${this.referenceImage}${size}，输出尺寸将按参考图比例取 ${plan.width}x${plan.height}`;
		}
		return '本次任务尚未选定参考图';
	}
	/** 清空参考图状态（错误重置路径调用；任务级清理由 createCreativeWork 负责） */
	public resetReference(): void {
		this.referenceImage = '';
		this.referencePrepared = false;
		this.referenceSize = null;
	}
	/** 角色名称 */
	protected get roleName(): string { return '绘制者' }
	/** 获取工具定义 */
	protected getToolDefinitions(): ToolCall[] { return this.roleTool }
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
			case 'self_portrait': return this.handleSelfPortrait(args as SelfPortraitParams);
			case 'chibi_self_portrait': return this.handleChibiSelfPortrait(args as SelfPortraitParams);
			case 'group_photo': return this.handleGroupPhoto(args as GroupPhotoParams);
			case 'list_images': return this.handleListImages();
			case 'select_reference_image': return this.handleSelectReferenceImage(args as ImageTagParams);
			case 'upscale_image': return this.handleUpscaleImage(args as ImageTagParams);
			default: return `未知工具: ${funcName}，可用工具为 diffusion_generation、image_to_image、self_portrait、chibi_self_portrait、group_photo、list_images、select_reference_image、upscale_image`;
		}
	}
	/** 从工具调用中提取绘画作品详情 */
	protected collectDetail(toolCall: ToolCallItem, paintings: PaintingDetail[]): void {
		try {
			const args = typeof toolCall.function.arguments === 'string'
				? JSON.parse(toolCall.function.arguments)
				: toolCall.function.arguments;
			const name = toolCall.function.name;
			if (name === 'self_portrait' || name === 'chibi_self_portrait') {
				paintings.push({
					toolName: name,
					promptSummary: name === 'self_portrait' ? '全尺寸自画像' : 'Q版自画像',
					expression: args.expression || '',
					posture: args.posture || '',
					environment: args.environment || '',
				});
			}
			else if (name === 'group_photo') {
				paintings.push({
					toolName: name,
					promptSummary: `与${args.companion || '他人'}的合照`,
					expression: args.expression || '',
					posture: args.posture || '',
					environment: args.environment || '',
					companion: args.companion || '',
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
				const tag = normalizeImageTag(String(args.image_tag || ''));
				paintings.push({
					toolName: name,
					promptSummary: tag ? `[${tag}] 超分重建` : '最近作品超分重建',
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
			if (p.toolName === 'self_portrait' || p.toolName === 'chibi_self_portrait') {
				let desc = p.toolName === 'self_portrait' ? '月华绘制了一幅全尺寸自画像' : '月华绘制了一幅Q版自画像';
				if (p.expression) desc += `，展现了${p.expression}`;
				if (p.environment) desc += `，背景是${p.environment}`;
				parts.push(desc + '。');
			}
			else if (p.toolName === 'group_photo') {
				let desc = `月华绘制了一幅与${p.companion || '他人'}的合照`;
				if (p.expression) desc += `，展现了${p.expression}`;
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
	 * @param template 场景外观模板（Q版/全尺寸/合照）
	 * @param parts 占位符填充参数，缺省时使用随机表情、随机姿势与默认服装
	 * @param defaultOutfit 该场景的默认服装提示词
	 */
	private renderAppearance(template: string, parts: AppearanceParts, defaultOutfit: string): string {
		const expression = parts.expression || this.defaultExpressionPrompt[RandomFloor(0, this.defaultExpressionPrompt.length - 1)];
		const posture = parts.posture || this.defaultPosturePrompt[RandomFloor(0, this.defaultPosturePrompt.length - 1)];
		const outfit = parts.outfit || defaultOutfit;
		return template
			.replace('{base}', this.appearanceBasePrompt)
			.replace('{expression}', expression)
			.replace('{outfit}', outfit)
			.replace('{posture}', posture)
			.replace('{companion}', parts.companion || '')
			.replace('{environment}', parts.environment || '');
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
		/** 实际生效的步数（0 表示交引擎按 sd_steps 处理） */
		const stepsNote = imageParams.steps ? `，${imageParams.steps} 步` : '';
		console.log(`[绘制者] ${label}成功，尺寸: ${result.width}x${result.height}${stepsNote}${imageParams.superResolution ? '（含超分重建）' : ''}`);
		if (!pushImage([result.base64])) console.warn(`[绘制者] 推送${label}结果到前端失败`);
		return `${label}成功。图片尺寸: ${result.width}x${result.height}，seed: ${result.seed}${stepsNote}${imageParams.superResolution ? '，已附带超分辨率重建' : ''}`;
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
	/** 处理图生图生成（基于当前选定的参考图）：强度与步数由绘制者指定，比例随参考图 */
	private handleImageToImage(args: ImageToImageParams): string {
		try {
			const prompt = args.prompt || '';
			if (!prompt.trim()) return '图生图失败：正向提示词不能为空';
			if (!this.referenceImage || !this.referencePrepared) {
				return `图生图失败：${this.referenceStatus()}。请先调用 list_images 查看图片缓存并用 select_reference_image 按标签选定基座图，或改用 diffusion_generation 进行文生图`;
			}
			const strength = normalizeStrength(args.strength, '图生图');
			const steps = normalizeSteps(args.steps, '图生图');
			// 比例随参考图：预算内保持比例忠实，短边不低于 512
			const plan = this.planFor();
			console.log(`[绘制者] 图生图 - 正向提示词: ${prompt.slice(0, 100)}...，参考图: ${this.referenceImage}，强度: ${strength}，尺寸: ${plan.origin} → ${plan.width}x${plan.height}，步数: ${steps || '默认'}`);
			return this.paint({
				prompt: prompt,
				negativePrompt: args.negative_prompt || '',
				width: plan.width,
				height: plan.height,
				steps: steps,
				cfgScale: args.cfg_scale ?? 1.0,
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
	/** 处理全尺寸自画像生成 */
	private handleSelfPortrait(args: SelfPortraitParams): string {
		return this.handlePortrait(args, this.fullAppearancePrompt, this.defaultOutfitFullPrompt, '自画像');
	}
	/** 处理Q版自画像生成 */
	private handleChibiSelfPortrait(args: SelfPortraitParams): string {
		return this.handlePortrait(args, this.chibiAppearancePrompt, this.defaultOutfitChibiPrompt, 'Q版自画像');
	}
	/** 处理人物像生成（全尺寸自画像 / Q版自画像共用）：比例由绘制者选定，缺省 1:1 */
	private handlePortrait(args: SelfPortraitParams, template: string, defaultOutfit: string, label: string): string {
		try {
			console.log(`[绘制者] -> ${label}生成`);
			console.log(`表情: "${args.expression}"`)
			console.log(`姿势: "${args.posture}"`)
			console.log(`服装: "${args.outfit}"`)
			console.log(`环境: "${args.environment}"`)
			console.log(`负面提示词: "${args.negative_prompt}"`)
			console.log(`提示词引导系数: "${args.cfg_scale}"`)
			/** 输出尺寸计划（自画像无参考图，取比例预设） */
			const plan = resolveDimensionPlan(args.ratio);
			/** 迭代步数（0 表示交引擎按 sd_steps 处理） */
			const steps = normalizeSteps(args.steps, label);
			const fullPrompt = this.renderAppearance(template, args, defaultOutfit);
			return this.paint({
				prompt: fullPrompt,
				negativePrompt: args.negative_prompt || this.defaultNegativePrompt,
				width: plan.width,
				height: plan.height,
				steps: steps,
				cfgScale: args.cfg_scale ?? 1.0,
				superResolution: args.super_resolution === true,
			}, `${label}生成`);
		}
		catch (error) {
			console.error(`[绘制者] ${label}处理异常:`, error);
			return `${label}生成异常: ${error}`;
		}
	}
	/** 处理合照生成（已选定参考图时以合照对象照片为底图进行图生图，比例随参考图） */
	private handleGroupPhoto(args: GroupPhotoParams): string {
		try {
			console.log(`[绘制者] -> 合照生成`);
			console.log(`合照对象: "${args.companion}"`)
			console.log(`表情: "${args.expression}"`)
			console.log(`姿势: "${args.posture}"`)
			console.log(`服装: "${args.outfit}"`)
			console.log(`环境: "${args.environment}"`)
			// 合照对象为琉璃时注入内置固定形象，保证形象一致
			let companion = args.companion || '';
			if (companion.includes('琉璃')) companion = this.crystalAppearancePrompt;
			const fullPrompt = this.renderAppearance(this.groupAppearancePrompt, { ...args, companion }, this.defaultOutfitFullPrompt);
			/** 本次是否以参考图为底图 */
			const useReference = !!(this.referenceImage && this.referencePrepared);
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
			// 参考图（本次任务选定）：以合照对象照片为底图，尽量保留其容貌
			if (useReference) {
				imageParams.initImg = this.referenceImage;
				imageParams.strength = normalizeStrength(args.strength, '合照');
				console.log(`[绘制者] 合照图生图模式，参考图: ${this.referenceImage}，强度: ${imageParams.strength}，尺寸: ${plan.origin} → ${plan.width}x${plan.height}，步数: ${steps || '默认'}`);
			}
			else if (this.referenceImage) {
				// 上一任务遗留的参考图不得自动生效，仅提示模型按需重新选定
				console.log('[绘制者] 存在历史参考图但非本次任务选定，按文生图绘制合照');
			}
			return this.paint(imageParams, '合照生成');
		}
		catch (error) {
			console.error('[绘制者] 合照生成处理异常:', error);
			return `合照生成异常: ${error}`;
		}
	}
	/** 处理图片缓存清单查询（绘图者据此分析并按标签选定图生图基座） */
	private handleListImages(): string {
		const entries = listCacheEntries(IMAGE_CACHE_CAPACITY);
		if (entries.length === 0) return '图片缓存为空：当前没有可用的图片标签';
		const lines = entries.map(entry => `[${entry.tag}] ${entry.desc ? sanitizeImageDesc(entry.desc, 40) : '（无描述）'}${entry.pinned ? ' [窗内]' : ''}`);
		return `图片缓存中最近的 ${entries.length} 张图片（标签越新越靠前；[窗内] 表示图片仍在对话者的多模态窗口中）：\n${lines.join('\n')}\n请结合创作需求分析后，调用 select_reference_image 按标签选定图生图基座，或调用 upscale_image 对指定图片超分。${this.referenceStatus()}。`;
	}
	/** 将参考图下载为本地文件（按内容哈希命名，返回相对 LocalDir 的路径与图片尺寸），失败时返回空路径 */
	private saveReferenceImage(imageUrl: string): { path: string; size: { width: number; height: number } | null } {
		try {
			const [resized, err] = resizeImage(imageUrl);
			if (err || !resized || resized.length === 0) return { path: '', size: null };
			const ref = resized[0];
			const ext = ref.format === 'jpeg' ? 'jpg' : 'png';
			// 按内容哈希（SHA-256 前16位）命名，与前端文件哈希命名约定一致
			const [hash, hashErr] = hashBytes(ref.image);
			if (hashErr || !hash) {
				console.error('[绘制者] 参考图哈希计算失败:', hashErr);
				return { path: '', size: null };
			}
			const relPath = `multimedia/reference/${hash}.${ext}`;
			const [, , saveErr] = saveFile(relPath, true, ref.image);
			if (saveErr) {
				console.error('[绘制者] 参考图保存失败:', saveErr);
				return { path: '', size: null };
			}
			console.log(`[绘制者] 参考图已保存: ${relPath}（${ref.width}x${ref.height}）`);
			const size = ref.width > 0 && ref.height > 0 ? { width: ref.width, height: ref.height } : null;
			return { path: relPath, size };
		}
		catch (error) {
			console.error('[绘制者] 参考图处理异常:', error);
			return { path: '', size: null };
		}
	}
	/** 处理参考图选定：按标签从共享缓存取源图并落盘为图生图基座 */
	private handleSelectReferenceImage(args: ImageTagParams): string {
		const tag = normalizeImageTag(String(args.image_tag || ''));
		if (!tag) return '选定参考图失败：请提供图片标签，如 图片.003（可用 list_images 查看当前可用标签）';
		const entry = getCacheEntry(tag);
		if (!entry) return `选定参考图失败：${tag} 已过期或不存在，请先调用 list_images 查看可用图片`;
		const localPath = this.prepareReference(entry.source);
		if (!localPath) return `选定参考图失败：[${tag}] 的源图已无法读取（远端地址可能失效），请改用 list_images 清单中的其它标签`;
		console.log(`[绘制者] 参考图已选定: ${localPath}（${tag}）`);
		return `参考图已选定：[${tag}]。本次任务的 image_to_image 与合照将以该图为基座（任务结束后需重新选定）。`;
	}
	/** 处理独立超分：对最近生成作品或按标签取缓存图片执行 RealESRGAN 重建 */
	private handleUpscaleImage(args: ImageTagParams): string {
		try {
			const tag = normalizeImageTag(String(args.image_tag || ''));
			let source = '';
			if (tag) {
				const entry = getCacheEntry(tag);
				if (!entry) return `超分失败：${tag} 已过期或不存在，请先调用 list_images 查看可用图片`;
				source = entry.source;
			}
			else {
				if (!this.lastResultPath) return '超分失败：还没有生成过作品，请通过 image_tag 指定缓存中的图片，或先完成一次图像生成';
				source = this.lastResultPath;
			}
			console.log(`[绘制者] 超分重建 - 来源: ${source.substring(0, 60)}...`);
			const [result, error] = upscaleImage(source);
			if (error) {
				console.error('[绘制者] 超分失败:', error);
				return `超分失败: ${error}`;
			}
			if (!result || !result.base64) return '超分失败：引擎返回空结果';
			if (!pushImage([result.base64])) console.warn('[绘制者] 推送超分结果到前端失败');
			return `超分成功。图片尺寸: ${result.width}x${result.height}，已推送给用户`;
		}
		catch (error) {
			console.error('[绘制者] 超分处理异常:', error);
			return `超分处理异常: ${error}`;
		}
	}
}