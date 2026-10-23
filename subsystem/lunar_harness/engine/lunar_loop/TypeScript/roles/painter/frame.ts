import { GlobalConfig } from '../../config/global';
import { DimensionPlan } from '../../config/image';

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

/** 默认画面比例（文生图/自画像未指定比例时使用） */
export const DEFAULT_RATIO = '1:1';

/** 图片句柄清单条数上限（list_images 用；防御模型索要过多条目） */
export const LIST_LIMIT_MAX = 100;

/** 图生图缺省强度（绘制者未指定时；仅"中性"兜底，具体档位见 painterRole.md 指引） */
const DEFAULT_STRENGTH = 0.5;
/** 迭代步数区间：下限 8 供 turbo 方案使用；绘制者未指定时回落到当前方案的推荐步数 */
const STEPS_MIN = 8;
const STEPS_MAX = 60;

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
export function normalizeStrength(value: unknown, label: string): number {
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
export function normalizeSteps(value: unknown, label: string): number {
	const num = Number(value);
	if (!Number.isFinite(num) || num <= 0) {
		console.log(`[绘制者] ${label}未指定步数，按当前方案推荐步数处理`);
		return 0;
	}
	const steps = Math.round(Math.min(Math.max(num, STEPS_MIN), STEPS_MAX));
	if (steps !== Math.round(num)) console.log(`[绘制者] ${label}步数 ${num} 越界，已收进 ${steps}`);
	return steps;
}
