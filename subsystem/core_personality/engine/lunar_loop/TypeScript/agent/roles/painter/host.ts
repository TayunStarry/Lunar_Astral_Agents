import type { CustomCharacterProfile } from '../../capabilities/character';
import type { DimensionPlan, GenerateImageParams } from '../../../config/image';
import type { PainterPromptLibrary } from './prompts';

/**
 * 绘制者运行时上下文
 *
 * 处理器只依赖这组能力（提示词库、角色库、参考图状态与绘图入口），
 * 不直接持有角色实例，便于按职责拆分到独立模块。
 */
export interface PainterHost {
	/** 提示词库 */
	readonly prompts: PainterPromptLibrary;
	/** 自定义角色库 */
	readonly customCharacters: Map<string, CustomCharacterProfile>;
	/** 本次任务参考图（归档 blob 相对路径；无参考图为空串） */
	readonly referenceImage: string;
	/** 最近一次生成作品的本地路径 */
	readonly lastResultPath: string;
	/** 当前任务的输出尺寸计划 */
	planFor(ratio?: string): DimensionPlan;
	/** 参考图状态描述 */
	referenceStatus(): string;
	/** 执行图像生成并推送至前端 */
	paint(imageParams: GenerateImageParams, label: string): string;
	/** 记录最近一次生成作品的路径 */
	rememberResult(path: string): void;
}
