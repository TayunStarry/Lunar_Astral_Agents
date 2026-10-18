/**
 * 绘制工具共用参数定义
 *
 * 同一参数在多个工具中保持一致，避免各工具各写一份文案后逐渐漂移。
 * 工具特有的缺省值/生效条件由调用处展开后追加，不在本模块硬编码。
 */

/** 共用参数的结构类型（保持 type 为字面量，便于直接嵌入工具 schema） */
type ParamSchema = {
	type: 'string' | 'number' | 'boolean';
	enum?: string[];
	description?: string;
};

/** 画面比例预设（枚举与通用选择原则） */
export const RATIO_PARAM: ParamSchema = {
	type: "string",
	enum: ["1:1", "4:3", "3:4", "16:9", "9:16", "3:2", "2:3"],
	description: "画面比例预设。人物/物品用 2:3 或 3:4，风景/横构图用 3:2 或 16:9，方形构图用 1:1"
};

/** 迭代步数 */
export const STEPS_PARAM: ParamSchema = {
	type: "number",
	description: "迭代步数，取值 8~60：quality 方案建议 30~60（常规 40，精细 50~60）；turbo 方案建议 8~12。省略时使用当前模型方案的推荐步数。越高细节越好但耗时线性增加，不影响构图"
};

/** 负面提示词 */
export const NEGATIVE_PROMPT_PARAM: ParamSchema = {
	type: "string",
	description: "负面提示文本，用于排除不希望出现的元素"
};

/** 提示词引导系数（引导蒸馏模型固定 1.0） */
export const CFG_SCALE_PARAM: ParamSchema = {
	type: "number",
	description: "提示词引导系数（0~2），默认 1.0。当前为引导蒸馏模型，保持 1.0；调高会发灰、过曝、出现塑料感"
};

/** 生成后追加超分重建 */
export const SUPER_RESOLUTION_PARAM: ParamSchema = {
	type: "boolean",
	description: "生成完成后追加 RealESRGAN 超分辨率重建（4x），提升清晰度与细节。用户要求高清、精细画质时开启，耗时增加"
};
