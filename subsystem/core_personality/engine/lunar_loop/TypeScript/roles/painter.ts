import { ToolCall } from '../config/tool';
import {
	CreateCharacterParams,
	DiffusionGenerationParams,
	DimensionPlan,
	GenerateImageParams,
	GroupPhotoParams,
	ImageTagParams,
	ImageToImageParams,
	PortraitParams,
	SchemeSwitchParams
} from '../config/image';
import { PostMessage, ToolCallItem } from '../config/model';
import { CustomCharacterProfile, loadCustomCharacters } from '../capabilities/character';
import { CreativeRoleBase } from '../agent/creative';
import { painterTools } from './painter/tools/index';
import { PainterPromptLibrary, loadCharacterPrompts } from './painter/prompts';
import { CharacterProfile, PaintingDetail } from './painter/types';
import { characterCatalogText, mergeProfiles } from './painter/characters';
import { buildPaintingSummary, collectPaintingDetail } from './painter/summary';
import {
	bindReference,
	buildReferenceTaskContent,
	clearReference,
	createReferenceState,
	describeReference,
	ReferenceState
} from './painter/reference';
import { resolveDimensionPlan } from './painter/frame';
import { handleDiffusionGeneration, handleImageToImage } from './painter/generation-handler';
import { handlePortrait } from './painter/portrait-handler';
import { handleGroupPhoto } from './painter/group-handler';
import {
	handleCreateCharacter,
	handleListImages,
	handleSwitchDiffusionScheme,
	handleUpscaleImage
} from './painter/manage-handler';
import type { PainterHost } from './painter/host';
import { pushAssistantImage } from '../io/conversation';
import { localPathToReadURL } from '../io/conversation/codec';

/**
 * 组装绘制者系统提示词：把角色提示词中的基础外观填入占位符
 *
 * 外观数据与绘制工具共用 prompts/painterCharacters.md 的「基础外观」节，
 * 改外观只需维护该文件，系统提示与出图模板同步生效。
 */
function composePainterPrompt(): string {
	const characters = loadCharacterPrompts();
	return fileView('prompts/painterRole.md')[0].trim()
		.split('{lunar-appearance}').join(characters.text('月华/基础外观'))
		.split('{crystal-appearance}').join(characters.text('琉璃/基础外观'));
}

/** 绘制者角色 */
export class PainterRole extends CreativeRoleBase<PaintingDetail> implements PainterHost {
	/** 智能体角色标签（TUI 实况屏显示） */
	protected roleTag: string = "绘制者";
	/** 提示词库（公用提示词 + 角色提示词） */
	readonly prompts = new PainterPromptLibrary();
	/**
	 * 自定义角色库（SQLite knowledge.db → painter_characters 表）
	 *
	 * 构造时先加载一次，供工具描述生成角色目录；每次创作任务开始时再刷新，
	 * 避免启动时数据库未就绪或其它会话新增的角色在当前实例中不可见。
	 */
	customCharacters: Map<string, CustomCharacterProfile> = loadCustomCharacters();
	/** 本次任务的参考图状态（任务级，开工时重置） */
	private readonly reference: ReferenceState = createReferenceState();
	/** 最近一次生成作品的地址（服务 URL 或本地路径，供超分工具对最新作品重建） */
	private resultPath = '';

	/** 构造函数 */
	public constructor() {
		super(composePainterPrompt());
	}

	/** 本次任务参考图（归档 blob 相对路径；供图生图处理器与冒烟测试读取） */
	public get referenceImage(): string { return this.reference.image; }
	public set referenceImage(value: string) { this.reference.image = value; }

	/** 参考图句柄 */
	public get referenceHandle(): string { return this.reference.handle; }
	public set referenceHandle(value: string) { this.reference.handle = value; }

	/** 参考图视觉副本（唯一可以放进 image_url 的参考图形态） */
	public get referenceVision(): string { return this.reference.vision; }
	public set referenceVision(value: string) { this.reference.vision = value; }

	/** 参考图比例基准 */
	public get referenceSize(): { width: number; height: number } | null { return this.reference.size; }
	public set referenceSize(value: { width: number; height: number } | null) { this.reference.size = value; }

	/** 最近一次生成作品的地址 */
	public get lastResultPath(): string { return this.resultPath; }

	/** 记录最近一次生成作品的路径 */
	public rememberResult(path: string): void {
		this.resultPath = path;
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
		bindReference(this.reference, reference);
		return super.createCreativeWork(taskDescription);
	}

	/**
	 * 把任务要求与参考图**视觉副本**合成一条多模态任务消息送进自己的上下文（覆写基类钩子）
	 *
	 * @param taskDescription 对话者传来的任务描述
	 * @returns 含参考图的内容项数组；无参考图时退化为纯文本
	 */
	protected buildTaskContent(taskDescription: string): PostMessage['content'] {
		return buildReferenceTaskContent(this.reference, taskDescription);
	}

	/** 当前任务的输出尺寸计划：有参考图时按参考图比例，否则用绘制者指定的比例预设 */
	public planFor(ratio?: string): DimensionPlan {
		const reference = this.reference.image ? (this.reference.size || undefined) : undefined;
		return resolveDimensionPlan(ratio, reference);
	}

	/** 参考图当前状态描述（供工具结果回显，避免模型在无参考图时反复尝试图生图） */
	public referenceStatus(): string {
		return describeReference(this.reference, this.planFor());
	}

	/** 清空参考图状态（错误重置路径调用；任务级清理由 createCreativeWork 负责） */
	public resetReference(): void {
		clearReference(this.reference);
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
		const catalog = this.characterCatalog();
		const definitions = JSON.stringify(painterTools).split('{characters}').join(catalog);
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
			case 'diffusion_generation': return handleDiffusionGeneration(this, args as DiffusionGenerationParams);
			case 'image_to_image': return handleImageToImage(this, args as ImageToImageParams);
			case 'portrait': return handlePortrait(this, args as PortraitParams);
			case 'create_character': return handleCreateCharacter(this, args as CreateCharacterParams);
			case 'group_photo': return handleGroupPhoto(this, args as GroupPhotoParams);
			case 'switch_diffusion_scheme': return handleSwitchDiffusionScheme(args as SchemeSwitchParams);
			case 'list_images': return handleListImages(this);
			case 'upscale_image': return handleUpscaleImage(this, args as ImageTagParams);
			default: return `未知工具: ${funcName}，可用工具为 diffusion_generation、image_to_image、portrait、create_character、group_photo、switch_diffusion_scheme、list_images、upscale_image`;
		}
	}

	/** 从工具调用中提取绘画作品详情 */
	protected collectDetail(toolCall: ToolCallItem, paintings: PaintingDetail[]): void {
		collectPaintingDetail(toolCall, paintings);
	}

	/** 构建绘画作品摘要，使用月华话术格式 */
	protected buildSummary(paintings: PaintingDetail[]): string {
		return buildPaintingSummary(paintings);
	}

	/** 执行图像生成并推送至前端，返回本次生成的结果描述 */
	public paint(imageParams: GenerateImageParams, label: string): string {
		const [result, error] = generateImage(imageParams);
		if (error) {
			console.error(`[绘制者] ${label}失败:`, error);
			return `${label}失败: ${error}`;
		}
		if (!result || !result.base64) return `${label}失败：引擎返回空结果`;
		// 记录作品路径，供超分工具对最近作品重建
		this.rememberResult(result.path);
		/**
		 * 作品图**不回写历史**（原 recordPainting 机制已移除）：
		 *   - 改图基座走 reference_tag → referenceImage 管线（原图 blob 直给扩散引擎，
		 *     视觉副本随任务消息进入上下文），历史回执图在该链路中零参与；
		 *   - 绘制者的汇报只描述结果，从不基于回执图内容作答；
		 *   - 旧机制把 user 图消息插在 assistant(tool_call) 与 tool(result) 之间，
		 *     违反协议顺序，实测诱发模型以相同参数重复调用生成工具（多烧一次 40 步）。
		 */
		/** 实际生效的步数（0 表示交引擎按 sd_steps 处理） */
		const stepsNote = imageParams.steps ? `，${imageParams.steps} 步` : '';
		console.log(`[绘制者] ${label}成功，尺寸: ${result.width}x${result.height}${stepsNote}${imageParams.superResolution ? '（含超分重建）' : ''}`);
		// 图片以 URL 传递：作品已由绘图引擎落盘到 multimedia/generated/，
		// 推送时由句柄系统做哈希唯一性归一，返回的 URL 才是最终保留路径
		const url = localPathToReadURL(result.path);
		const urls = url ? pushAssistantImage([url], false) : [];
		if (urls.length === 0) console.warn(`[绘制者] 推送${label}结果到前端失败`);
		else this.rememberResult(urls[0]);
		return `${label}成功。图片尺寸: ${result.width}x${result.height}，seed: ${result.seed}${stepsNote}${imageParams.superResolution ? '，已附带超分辨率重建' : ''}`;
	}

	/** 角色目录文本（替换工具描述里的 {characters} 占位符） */
	private characterCatalog(): string {
		return characterCatalogText(this.characterProfiles());
	}

	/** 全部可用角色（内置 + 自定义），每次调用都重新合并 */
	private characterProfiles(): Map<string, CharacterProfile> {
		return mergeProfiles(this.prompts.builtinCharacters, this.customCharacters);
	}
}
