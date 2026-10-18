import type { CustomCharacterProfile } from '../../capabilities/character';

/** 绘画作品详情记录（用于向对话者传递作品信息） */
export interface PaintingDetail {
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
export interface CharacterProfile extends CustomCharacterProfile {
	/** 是否为内置角色（内置角色使用专属立绘模板，自定义角色走通用模板） */
	builtin: boolean;
}

/** 外观模板占位符填充参数 */
export interface AppearanceParts {
	/** 表情提示词 */
	expression?: string;
	/** 姿势提示词 */
	posture?: string;
	/** 服装提示词 */
	outfit?: string;
	/** 环境提示词 */
	environment?: string;
}

/** 立绘预设：模板、底板、默认服装与缺省画幅 */
export interface PortraitPreset {
	/** 角色标识（月华 / 琉璃 / 自定义角色名 / 通用-男 / 通用-女） */
	kind: string;
	/** 是否 Q 版 */
	chibi: boolean;
	/** 日志与汇报用的作品标签 */
	label: string;
	/** 场景外观模板 */
	template: string;
	/** 角色基础外观 */
	base: string;
	/** 默认服装 */
	outfit: string;
	/** 模板性别表述（男孩 / 女孩） */
	gender: string;
	/** 缺省画幅比例 */
	defaultRatio: string;
}
