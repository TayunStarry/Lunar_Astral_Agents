import { ToolCall,  RandomFloor,  GenerateImageParams, DiffusionGenerationParams, SelfPortraitParams, ToolCallItem, CreativeRoleBase } from '../index';

/** 绘画作品详情记录（用于向对话者传递作品信息） */
interface PaintingDetail {
	/** 工具名称 */
	toolName: string;
	/** 正向提示词摘要 */
	promptSummary: string;
	/** 表情（自画像专用） */
	expression?: string;
	/** 姿势（自画像专用） */
	posture?: string;
	/** 环境（自画像专用） */
	environment?: string;
}

/** 画家角色 */
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
	/** 自我外观提示 */
	private readonly selfAppearancePrompt = fileView('prompts/selfAppearance.md')[0]
	/** 默认服装提示词 */
	private readonly defaultOutfitPrompt = '穿着宽松的奶油白色针织连帽拉链外套，敞开拉链，里面是纯白色圆领T恤，高腰深蓝和白色格纹百褶迷你裙，侧腰位置悬挂着白色和深蓝的大缎带蝴蝶结，饰有圆润的白色珍珠装饰和金色高光，白色短袜，黑色系带低帮帆布鞋'
	/** 绘画角色工具 */
	private readonly roleTool: ToolCall[] = [
		{
			type: "function",
			function: {
				name: "diffusion_generation",
				description: "根据文本描述生成图像。如需进行图像创作,请调用此函数",
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
				name: "self_portrait",
				description: "生成自画像。调用此函数来创建自己的形象",
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
							description: "服装提示词,描述想要穿着的服装样式。如果不提供则使用默认服装"
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
		}
	]
	/** 图片生成关键词模式 — 仅匹配用户明确要求生成图片的意图 */
	private readonly imageKeywords = [
		/画(?:一(?:张|幅|个))?(?:图|画|图片|图像|插画|插图)/,
		/生成(?:一(?:张|幅|个))?(?:图|画|图片|图像|插画|插图)/,
		/绘制(?:一(?:张|幅|个))?(?:图|画|图片|图像|插画|插图)/,
		/创作(?:一(?:张|幅|个))?(?:图|画|图片|图像|插画|插图)/,
		/(?:帮我|给我|为我)(?:画|绘制|生成|创作|做|弄|整)(?:一(?:张|幅|个))?(?:图|画|图片|图像|插画|插图)?/,
		/(?:做|弄|整)(?:一(?:张|幅|个))?(?:图|画|图片|图像|插画|插图)/,
		/来(?:一(?:张|幅|个))?(?:图|画|图片|图像|插画|插图)/,
		/自画像/,
		/画(?:一(?:张|幅|个))?自画像/,
	]
	/** 构造函数 */
	public constructor() {
		super(fileView('prompts/painterRole.md')[0]);
	}
	/** 角色名称 */
	protected get roleName(): string { return '画家' }
	/** 检查未读消息是否匹配图片生成关键词 */
	protected matchKeywords(texts: string[]): boolean {
		return texts.some(text => this.imageKeywords.some(keyword => keyword.test(text)));
	}
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
			console.error(`[画家] 工具调用参数解析失败:`, toolCall.function.arguments);
			return `工具调用参数解析失败，请确保传入合法的 JSON 字符串。错误: ${parseError}`;
		}
		switch (funcName) {
			case 'diffusion_generation': return this.handleDiffusionGeneration(args as DiffusionGenerationParams);
			case 'self_portrait': return this.handleSelfPortrait(args as SelfPortraitParams);
			default: return `未知工具: ${funcName}，可用工具为 diffusion_generation 和 self_portrait`;
		}
	}
	/** 从工具调用中提取绘画作品详情 */
	protected collectDetail(toolCall: ToolCallItem, paintings: PaintingDetail[]): void {
		try {
			const args = typeof toolCall.function.arguments === 'string'
				? JSON.parse(toolCall.function.arguments)
				: toolCall.function.arguments;
			if (toolCall.function.name === 'self_portrait') {
				paintings.push({
					toolName: 'self_portrait',
					promptSummary: '自画像',
					expression: args.expression || '',
					posture: args.posture || '',
					environment: args.environment || '',
				});
			} else if (toolCall.function.name === 'diffusion_generation') {
				const prompt = args.prompt || '';
				paintings.push({
					toolName: 'diffusion_generation',
					promptSummary: prompt.length > 100 ? prompt.slice(0, 97) + '...' : prompt,
				});
			}
		} catch {
			// 解析失败时跳过，不阻断流程
		}
	}
	/** 构建绘画作品摘要，供对话者使用 */
	protected buildSummary(paintings: PaintingDetail[]): string {
		const parts: string[] = [];
		parts.push('[绘画创作记录] 你（月华）刚刚完成了以下图像作品创作：');
		for (let i = 0; i < paintings.length; i++) {
			const p = paintings[i];
			const detailLines: string[] = [];
			if (p.toolName === 'self_portrait') {
				detailLines.push(`作品${i + 1}：自画像`);
				if (p.expression) detailLines.push(`  - 表情：${p.expression}`);
				if (p.posture) detailLines.push(`  - 姿势：${p.posture}`);
				if (p.environment) detailLines.push(`  - 环境：${p.environment}`);
			} else {
				detailLines.push(`作品${i + 1}：扩散生成图像`);
				detailLines.push(`  - 画面内容：${p.promptSummary}`);
			}
			parts.push(detailLines.join('\n'));
		}
		parts.push('\n注意：请基于以上真实创作信息向用户介绍图像作品，切勿编造画面内容。图像已通过前端推送给用户。');
		return parts.join('\n');
	}
	/** 获得写入了动作、表情与服装的自我外观提示词 */
	private writeAppearancePrompt(expression?: string, posture?: string, outfit?: string, environment?: string): string {
		const currentExpression = expression || this.defaultExpressionPrompt[RandomFloor(0, this.defaultExpressionPrompt.length - 1)];
		const currentPosture = posture || this.defaultPosturePrompt[RandomFloor(0, this.defaultPosturePrompt.length - 1)];
		const currentOutfit = outfit || this.defaultOutfitPrompt;
		return this.selfAppearancePrompt.replace('{expression}', currentExpression).replace('{posture}', currentPosture).replace('{outfit}', currentOutfit).replace('{environment}', environment || '');
	}
	/** 处理扩散图像生成 */
	private handleDiffusionGeneration(args: DiffusionGenerationParams): string {
		try {
			const prompt = args.prompt || '';
			if (!prompt.trim()) return '扩散生成失败：正向提示词不能为空';
			console.log(`[画家] 扩散生成 - 正向提示词: ${prompt.slice(0, 100)}...`);
			const imageParams: GenerateImageParams = {
				prompt: prompt,
				negativePrompt: args.negative_prompt || '',
				cfgScale: args.cfg_scale ?? 1.0,
			};
			const [result, error] = generateImage(imageParams);
			if (error) {
				console.error('[画家] 图像生成失败:', error);
				return `扩散图像生成失败: ${error}`;
			}
			if (!result || !result.base64) {
				return '扩散图像生成失败：引擎返回空结果';
			}
			console.log(`[画家] 扩散图像生成成功，尺寸: ${result.width}x${result.height}`);
			const pushSuccess = pushImage([result.base64]);
			if (!pushSuccess) {
				console.warn('[画家] 推送图片到前端失败');
			}
			return `扩散图像生成成功。图片尺寸: ${result.width}x${result.height}，seed: ${result.seed}`;
		}
		catch (error) {
			console.error('[画家] 扩散生成处理异常:', error);
			return `扩散图像生成异常: ${error}`;
		}
	}
	/** 处理自画像生成 */
	private handleSelfPortrait(args: SelfPortraitParams): string {
		try {
			console.log(`[画家] -> 自画像生成`);
			console.log(`表情: "${args.expression}"`)
			console.log(`姿势: "${args.posture}"`)
			console.log(`服装: "${args.outfit}"`)
			console.log(`环境: "${args.environment}"`)
			console.log(`负面提示词: "${args.negative_prompt}"`)
			console.log(`提示词引导系数: "${args.cfg_scale}"`)
			const fullPrompt = this.writeAppearancePrompt(args.expression, args.posture, args.outfit, args.environment);
			const defaultNegativePrompt = '低分辨率, 糙噪点, 超现实主义, 丑陋的面部特征, 失真表情, 模糊轮廓, 颜色失衡, 不均匀光影, 强烈对比度, 过曝或欠曝, 杂乱背景, 像素化, 彩虹效果, 畸形肢体, 错位比例, 低质感纹理';
			const imageParams: GenerateImageParams = {
				prompt: fullPrompt,
				negativePrompt: args.negative_prompt || defaultNegativePrompt,
				cfgScale: args.cfg_scale ?? 1.0,
			};
			const [result, error] = generateImage(imageParams);
			if (error) {
				console.error('[画家] 自画像生成失败:', error);
				return `自画像生成失败: ${error}`;
			}
			if (!result || !result.base64) {
				return '自画像生成失败：引擎返回空结果';
			}
			console.log(`[画家] 自画像生成成功，尺寸: ${result.width}x${result.height}`);
			const pushSuccess = pushImage([result.base64]);
			if (!pushSuccess) {
				console.warn('[画家] 推送自画像到前端失败');
			}
			return `自画像生成成功。图片尺寸: ${result.width}x${result.height}，seed: ${result.seed}`;
		}
		catch (error) {
			console.error('[画家] 自画像生成处理异常:', error);
			return `自画像生成异常: ${error}`;
		}
	}
}
