import { ToolCall } from '../../config/tool';
import { RandomFloat, RandomFloor } from '../../tool/math';
import { GenerateImageParams, DiffusionGenerationParams, ImageToImageParams, SelfPortraitParams, GroupPhotoParams } from '../../config/image';
import { ToolCallItem } from '../../config/model';
import { CreativeRoleBase } from '../base/creative';
import { interactEvent } from '../capabilities/ltp-event';

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
	/** Q版默认服装提示词（白色洛丽塔连衣裙，胸前酒红蝴蝶结配红宝石胸针） */
	private readonly defaultOutfitChibiPrompt = '穿着纯白色洛丽塔风连衣裙，裙身镶有金色滚边与白色蕾丝花边，短款泡泡袖，胸前系着酒红色蝴蝶结，蝴蝶结中央镶嵌金框红宝石胸针并垂着金色链条吊坠，裙摆点缀两枚金色蝴蝶结，白色蕾丝花边短袜点缀白色蝴蝶结，黑色亮面皮鞋'
	/** 全尺寸默认服装提示词（针织外套搭配格纹百褶裙，腰侧蓝白蝴蝶结配珍珠） */
	private readonly defaultOutfitFullPrompt = '穿着宽松的奶油白色针织连帽拉链外套，敞开拉链，里面是纯白色圆领T恤，高腰深蓝和白色格纹百褶迷你裙，腰侧缀有深蓝与白色相间的缎带蝴蝶结且蝴蝶结中心镶嵌一颗圆珍珠，白色短袜，黑色系带低帮帆布鞋'
	/** 默认负面提示词（人物像与图生图共用） */
	private readonly defaultNegativePrompt = '低分辨率, 糙噪点, 超现实主义, 丑陋的面部特征, 失真表情, 模糊轮廓, 颜色失衡, 不均匀光影, 强烈对比度, 过曝或欠曝, 杂乱背景, 像素化, 彩虹效果, 畸形肢体, 错位比例, 低质感纹理'
	/** 参考图（律令 <参考图> 设置）：本地图片路径（相对 LocalDir），非空时图生图与合照切换为图生图流程 */
	public referenceImage: string = ''
	/** 绘画角色工具 */
	private readonly roleTool: ToolCall[] = [
		{
			type: "function",
			function: {
				name: "diffusion_generation",
				description: "根据文本描述生成与月华本人无关的图像，纯文生图。仅适用于风景、物品、其他角色、场景等通用创作;严禁用于绘制月华自己的形象（绘制月华必须调用 self_portrait 或 chibi_self_portrait）。若需要参照已设置的参考图创作,请改用 image_to_image",
				parameters: {
					type: "object",
					properties: {
						"prompt": {
							type: "string",
							description: "图像生成的正向描述文本"
						},
						"negative_prompt": {
							type: "string",
							description: "负面提示文本,用于排除图像中不希望出现的元素"
						},
						"cfg_scale": {
							type: "number",
							description: "提示词权重调节参数,取值范围为 0 到 2,默认值为 1.0"
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
				description: "基于已设置的参考图（律令 <参考图> 保存）进行图生图创作:以参考图为底图,按文本描述改变风格、场景、服饰或细节,同时保留参考图的主体特征。仅在已设置参考图时可用;未设置参考图时请改用 diffusion_generation 进行文生图",
				parameters: {
					type: "object",
					properties: {
						"prompt": {
							type: "string",
							description: "图像生成的正向描述文本,描述希望参考图变成什么样子"
						},
						"negative_prompt": {
							type: "string",
							description: "负面提示文本,用于排除图像中不希望出现的元素"
						},
						"strength": {
							type: "number",
							description: "图生图强度,取值范围 0 到 1,越大越偏离参考图。保留主体时建议 0.3~0.5,较大改变时 0.6~0.8。默认随机取值"
						},
						"cfg_scale": {
							type: "number",
							description: "提示词权重调节参数,取值范围为 0 到 2,默认值为 1.0"
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
						"negative_prompt": {
							type: "string",
							description: "负面提示文本,用于排除图像中不希望出现的元素"
						},
						"cfg_scale": {
							type: "number",
							description: "提示词权重调节参数,取值范围为 0 到 2,默认值为 1.0"
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
						"negative_prompt": {
							type: "string",
							description: "负面提示文本,用于排除图像中不希望出现的元素"
						},
						"cfg_scale": {
							type: "number",
							description: "提示词权重调节参数,取值范围为 0 到 2,默认值为 1.0"
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
				description: "生成月华与其他人的合照。当要求绘制月华和某人（朋友、家人、其他角色等）的合影时调用此函数。若已设置参考图,将以该参考图为底图进行图生图,尽量保留合照对象的容貌;未设置则按文本描述绘制",
				parameters: {
					type: "object",
					properties: {
						"companion": {
							type: "string",
							description: "合照对象描述,描述对方的性别、外貌、发型、服装、身份等特征"
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
						"negative_prompt": {
							type: "string",
							description: "负面提示文本,用于排除图像中不希望出现的元素"
						},
						"cfg_scale": {
							type: "number",
							description: "提示词权重调节参数,取值范围为 0 到 2,默认值为 1.0"
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
		}
	]
	/** 构造函数 */
	public constructor() {
		super(fileView('prompts/painterRole.md')[0]);
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
			default: return `未知工具: ${funcName}，可用工具为 diffusion_generation、image_to_image、self_portrait、chibi_self_portrait、group_photo`;
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
				paintings.push({
					toolName: name,
					promptSummary: prompt.length > 100 ? prompt.slice(0, 97) + '...' : prompt,
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
		console.log(`[绘制者] ${label}成功，尺寸: ${result.width}x${result.height}`);
		if (!pushImage([result.base64])) console.warn(`[绘制者] 推送${label}结果到前端失败`);
		return `${label}成功。图片尺寸: ${result.width}x${result.height}，seed: ${result.seed}`;
	}
	/** 处理通用扩散图像生成（文生图） */
	private handleDiffusionGeneration(args: DiffusionGenerationParams): string {
		try {
			const prompt = args.prompt || '';
			if (!prompt.trim()) return '扩散生成失败：正向提示词不能为空';
			console.log(`[绘制者] 扩散生成 - 正向提示词: ${prompt.slice(0, 100)}...`);
			return this.paint({
				prompt: prompt,
				negativePrompt: args.negative_prompt || '',
				cfgScale: args.cfg_scale ?? 1.0,
			}, '扩散图像生成');
		}
		catch (error) {
			console.error('[绘制者] 扩散生成处理异常:', error);
			return `扩散图像生成异常: ${error}`;
		}
	}
	/** 处理图生图生成（基于已设置的参考图） */
	private handleImageToImage(args: ImageToImageParams): string {
		try {
			const prompt = args.prompt || '';
			if (!prompt.trim()) return '图生图失败：正向提示词不能为空';
			if (!this.referenceImage) {
				return '图生图失败：尚未设置参考图，请先发送图片并下达 <参考图> 指令，或改用 diffusion_generation 进行文生图';
			}
			const strength = args.strength ?? RandomFloat(0.35, 0.75);
			console.log(`[绘制者] 图生图 - 正向提示词: ${prompt.slice(0, 100)}...，参考图: ${this.referenceImage}，强度: ${strength}`);
			return this.paint({
				prompt: prompt,
				negativePrompt: args.negative_prompt || '',
				cfgScale: args.cfg_scale ?? 1.0,
				initImg: this.referenceImage,
				strength: strength,
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
	/** 处理人物像生成（全尺寸自画像 / Q版自画像共用） */
	private handlePortrait(args: SelfPortraitParams, template: string, defaultOutfit: string, label: string): string {
		try {
			console.log(`[绘制者] -> ${label}生成`);
			console.log(`表情: "${args.expression}"`)
			console.log(`姿势: "${args.posture}"`)
			console.log(`服装: "${args.outfit}"`)
			console.log(`环境: "${args.environment}"`)
			console.log(`负面提示词: "${args.negative_prompt}"`)
			console.log(`提示词引导系数: "${args.cfg_scale}"`)
			const fullPrompt = this.renderAppearance(template, args, defaultOutfit);
			return this.paint({
				prompt: fullPrompt,
				negativePrompt: args.negative_prompt || this.defaultNegativePrompt,
				cfgScale: args.cfg_scale ?? 1.0,
			}, `${label}生成`);
		}
		catch (error) {
			console.error(`[绘制者] ${label}处理异常:`, error);
			return `${label}生成异常: ${error}`;
		}
	}
	/** 处理合照生成（已设置参考图时以合照对象照片为底图进行图生图） */
	private handleGroupPhoto(args: GroupPhotoParams): string {
		try {
			console.log(`[绘制者] -> 合照生成`);
			console.log(`合照对象: "${args.companion}"`)
			console.log(`表情: "${args.expression}"`)
			console.log(`姿势: "${args.posture}"`)
			console.log(`服装: "${args.outfit}"`)
			console.log(`环境: "${args.environment}"`)
			const fullPrompt = this.renderAppearance(this.groupAppearancePrompt, args, this.defaultOutfitFullPrompt);
			const imageParams: GenerateImageParams = {
				prompt: fullPrompt,
				negativePrompt: args.negative_prompt || this.defaultNegativePrompt,
				cfgScale: args.cfg_scale ?? 1.0,
			};
			// 参考图（律令 <参考图> 设置）：以合照对象照片为底图，尽量保留其容貌
			if (this.referenceImage) {
				imageParams.initImg = this.referenceImage;
				imageParams.strength = RandomFloat(0.35, 0.6);
				console.log(`[绘制者] 合照图生图模式，参考图: ${this.referenceImage}，强度: ${imageParams.strength}`);
			}
			return this.paint(imageParams, '合照生成');
		}
		catch (error) {
			console.error('[绘制者] 合照生成处理异常:', error);
			return `合照生成异常: ${error}`;
		}
	}
}