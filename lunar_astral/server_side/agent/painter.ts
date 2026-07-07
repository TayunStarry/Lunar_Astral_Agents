import { ToolCall, PostMessage,RandomFloor, ModelBuilder, modelResponse, GenerateImageParams, DiffusionGenerationParams, SelfPortraitParams, ToolCallItem, AgentDefine } from '../index';

/** 绘画角色提示词 */
class Prompt extends ModelBuilder {
	/** 默认表情提示 */
	protected defaultExpressionPrompt = [
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
	protected defaultPosturePrompt = [
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
	protected selfAppearancePrompt = fileView('prompts/selfAppearance.md')[0]
	/** 默认服装提示词 */
	protected defaultOutfitPrompt = '穿着宽松的奶油白色针织连帽拉链外套，敞开拉链，里面是纯白色圆领T恤，高腰深蓝和白色格纹百褶迷你裙，侧腰位置悬挂着白色和深蓝的大缎带蝴蝶结，饰有圆润的白色珍珠装饰和金色高光，白色短袜，黑色系带低帮帆布鞋'
	/** 获得写入了动作、表情与服装的自我外观提示词 */
	protected writeAppearancePrompt(expression?: string, posture?: string, outfit?: string, environment?: string): string {
		/** 当前表情提示词, 默认使用随机表情提示 */
		const currentExpression = expression || this.defaultExpressionPrompt[RandomFloor(0, this.defaultExpressionPrompt.length - 1)];
		/** 当前姿势提示词, 默认使用随机姿势提示 */
		const currentPosture = posture || this.defaultPosturePrompt[RandomFloor(0, this.defaultPosturePrompt.length - 1)];
		/** 当前服装提示词, 默认使用默认服装提示 */
		const currentOutfit = outfit || this.defaultOutfitPrompt;
		// 替换表情提示词、姿势提示词、服装提示词与环境提示词
		return this.selfAppearancePrompt.replace('{expression}', currentExpression).replace('{posture}', currentPosture).replace('{outfit}', currentOutfit).replace('{environment}', environment || '');
	}
}

/** 绘画角色工具链 */
class Toolchain extends Prompt {
	/** 绘画角色工具 */
	protected roleTool: ToolCall[] = [
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
	/**
	 * 处理扩散图像生成
	 *
	 * 基于正反提示词调用图像生成引擎，生成用户所需的图像。
	 *
	 * @param {DiffusionGenerationParams} args - 扩散生成的参数
	 *
	 * @returns {string} - 生成结果描述
	 */
	protected handleDiffusionGeneration(args: DiffusionGenerationParams): string {
		try {
			/** 正向提示词 */
			const prompt = args.prompt || '';
			if (!prompt.trim()) return '扩散生成失败：正向提示词不能为空';
			console.log(`[画家] 扩散生成 - 正向提示词: ${prompt.slice(0, 100)}...`);
			/** 构建图像生成参数 */
			const imageParams: GenerateImageParams = {
				prompt: prompt,
				negativePrompt: args.negative_prompt || '',
				cfgScale: args.cfg_scale ?? 1.0,
			};

			/** 调用图像生成引擎 */
			const [result, error] = generateImage(imageParams);

			if (error) {
				console.error('[画家] 图像生成失败:', error);
				return `扩散图像生成失败: ${error}`;
			}

			if (!result || !result.base64) {
				return '扩散图像生成失败：引擎返回空结果';
			}

			console.log(`[画家] 扩散图像生成成功，尺寸: ${result.width}x${result.height}`);

			// 推送生成的图片到前端
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
	/**
	 * 处理自画像生成
	 *
	 * 基于表情、姿势和环境描述，生成月华的自画像。
	 *
	 * @param {SelfPortraitParams} args - 自画像生成的参数
	 *
	 * @returns {string} - 生成结果描述
	 */
	protected handleSelfPortrait(args: SelfPortraitParams): string {
		try {
			console.log(`[画家] -> 自画像生成`);
			console.log(`表情: "${args.expression}"`)
			console.log(`姿势: "${args.posture}"`)
			console.log(`服装: "${args.outfit}"`)
			console.log(`环境: "${args.environment}"`)
			console.log(`负面提示词: "${args.negative_prompt}"`)
			console.log(`提示词引导系数: "${args.cfg_scale}"`)
			// 构建完整的自画像提示词
			const fullPrompt = this.writeAppearancePrompt(args.expression, args.posture, args.outfit, args.environment);
			const defaultNegativePrompt = '低分辨率, 糙噪点, 超现实主义, 丑陋的面部特征, 失真表情, 模糊轮廓, 颜色失衡, 不均匀光影, 强烈对比度, 过曝或欠曝, 杂乱背景, 像素化, 彩虹效果, 畸形肢体, 错位比例, 低质感纹理';
			const imageParams: GenerateImageParams = {
				prompt: fullPrompt,
				negativePrompt: args.negative_prompt || defaultNegativePrompt,
				cfgScale: args.cfg_scale ?? 1.0,
			};

			/** 调用图像生成引擎 */
			const [result, error] = generateImage(imageParams);

			if (error) {
				console.error('[画家] 自画像生成失败:', error);
				return `自画像生成失败: ${error}`;
			}

			if (!result || !result.base64) {
				return '自画像生成失败：引擎返回空结果';
			}

			console.log(`[画家] 自画像生成成功，尺寸: ${result.width}x${result.height}`);

			// 推送生成的图片到前端
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
export class PainterRole extends Toolchain {
	/** 画家独立历史（跨周期持久化，供对话者消费后清空） */
	private _history: PostMessage[] = [];
	/** 构造函数 */
	public constructor() {
		super(fileView('prompts/painterRole.md')[0]);
	}
	/** 获取画家历史摘要（对话者调用后清空） */
	public consumeHistory(): PostMessage[] {
		const result = [...this._history];
		this._history = [];
		return result;
	}
	/** 创建图像渲染上下文 */
	public createImageRendering(source: AgentDefine, unreadContext: PostMessage[], count: number = 10): boolean {
		// 构建画家上下文：对话历史（最近15条）+ 画家自身历史（最近5条）+ 当前未读
		const dialogueHistory = source.dialogueRole.messages.slice(-15);
		const painterHistory = this._history.slice(-5);
		this.coverContext([...dialogueHistory, ...painterHistory, ...unreadContext]);
		/** 未读消息文本内容 */
		const unreadTexts: string[] = [];
		// 遍历未读消息，提取文本内容
		for (const message of unreadContext.slice(-count)) {
			// 如果消息内容是字符串，直接添加
			if (typeof message.content === 'string') unreadTexts.push(message.content);
			// 如果消息内容是数组，遍历添加文本内容
			else message.content.forEach(item => { if (item.type === 'text') unreadTexts.push(item.text); });
		}
		/** 检查是否允许生成图片 */
		let allowGeneration = false;
		/** 图片生成关键词模式 — 仅匹配用户明确要求生成图片的意图 */
		const imageKeywords = [
			/画(?:一(?:张|幅|个))?(?:图|画|图片|图像|插画|插图)/,
			/生成(?:一(?:张|幅|个))?(?:图|画|图片|图像|插画|插图)/,
			/绘制(?:一(?:张|幅|个))?(?:图|画|图片|图像|插画|插图)/,
			/创作(?:一(?:张|幅|个))?(?:图|画|图片|图像|插画|插图)/,
			/(?:帮我|给我|为我)(?:画|绘制|生成|创作|做|弄|整)(?:一(?:张|幅|个))?(?:图|画|图片|图像|插画|插图)?/,
			/(?:做|弄|整)(?:一(?:张|幅|个))?(?:图|画|图片|图像|插画|插图)/,
			/来(?:一(?:张|幅|个))?(?:图|画|图片|图像|插画|插图)/,
			/自画像/,
			/画(?:一(?:张|幅|个))?自画像/,
		];
		// 遍历消息文本，检查是否包含图片生成关键词
		unreadTexts.forEach(text => imageKeywords.forEach(keyword => { if (keyword.test(text)) allowGeneration = true; }));
		// 如果没有包含图片生成关键词，直接返回
		if (!allowGeneration) return true;
		/** 最大迭代次数 */
		const MAX_ITERATIONS = 3;

		/** 绘画记录 — 收集所有生成的图像详情，用于向对话者传递 */
		const paintings: PaintingDetail[] = [];

		// 执行绘画推理循环
		for (let i = 0; i < MAX_ITERATIONS; i++) {
			console.log(`[画家] 第 ${i + 1} 轮绘画推理`);
			/** LLM 响应 */
			let response: modelResponse;
			try {
				response = this.run([], this.roleTool);
			}
			catch (error) {
				console.error(`[画家] 第 ${i + 1} 轮推理失败:`, error);
				break;
			}
			/** 模型返回的选项 */
			const choice = response.body?.choices?.[0];
			if (!choice) {
				console.log('[画家] 模型返回空结果，结束绘画循环');
				break;
			}
			/** 工具调用列表 */
			const toolCalls = choice.message?.tool_calls;
			if (!toolCalls || toolCalls.length === 0) {
				/** 模型回复内容 */
				const replyContent = choice.message?.content || '';
				// 如果模型回复内容不为空，将模型回复内容写入到聊天模型的未读上下文
				if (replyContent) source.unreadContext.push({ role: 'tool', content: `[画面内容] ${replyContent}` });
				break;
			}
			// 将助手消息写入上下文（包含工具调用信息）
			this.writeContext(choice.message);
			// 遍历执行所有工具调用
			for (const toolCall of toolCalls) {
				console.log(`[画家] 执行工具: ${toolCall.function.name}`);
				/** 工具执行结果 */
				const result = this.executePaintingTool(toolCall);
				// 将工具执行结果写入上下文
				this.writeContext({ role: 'tool', content: result, tool_call_id: toolCall.id });

				// 收集绘画工具调用的详情，用于向对话者传递作品信息
				this.collectPaintingDetail(toolCall, paintings);
			}
		}

		// 将绘画作品详情写入画家历史，供对话者消费
		if (paintings.length > 0) {
			const summary = this.buildPaintingSummary(paintings);
			this._history.push({ role: 'tool', content: summary });
			console.log(`[画家] 已将 ${paintings.length} 幅作品详情写入历史`);
		}

		return false;
	}

	/**
	 * 从工具调用中提取绘画作品详情
	 */
	private collectPaintingDetail(toolCall: ToolCallItem, paintings: PaintingDetail[]): void {
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

	/**
	 * 构建绘画作品摘要，供对话者使用
	 *
	 * 对话者将根据此摘要向用户介绍生成的图像作品，
	 * 因此需要包含完整的作品信息以确保对话准确性。
	 */
	private buildPaintingSummary(paintings: PaintingDetail[]): string {
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
	/**
	 * 执行绘画工具调用
	 *
	 * 解析工具调用参数，路由到对应的工具处理函数。
	 *
	 * @param toolCall - 工具调用对象
	 *
	 * @returns {string} - 工具执行结果描述
	 */
	private executePaintingTool(toolCall: ToolCallItem): string {
		/** 工具函数名称 */
		const funcName = toolCall.function.name;
		/** 工具函数参数 */
		let args: Record<string, any> = {};
		try {
			args = typeof toolCall.function.arguments === 'string' ? JSON.parse(toolCall.function.arguments) : toolCall.function.arguments;
		}
		catch (parseError) {
			console.error(`[画家] 工具调用参数解析失败:`, toolCall.function.arguments);
			return `工具调用参数解析失败，请确保传入合法的 JSON 字符串。错误: ${parseError}`;
		}
		// 根据工具函数名称路由到对应的处理函数
		switch (funcName) {
			// 处理扩散生成工具调用
			case 'diffusion_generation': return this.handleDiffusionGeneration(args as DiffusionGenerationParams);
			// 处理自画像工具调用
			case 'self_portrait': return this.handleSelfPortrait(args as SelfPortraitParams);
			// 处理未知工具调用
			default: return `未知工具: ${funcName}，可用工具为 diffusion_generation 和 self_portrait`;
		}
	}
}