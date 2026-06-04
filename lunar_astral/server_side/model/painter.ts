import { ToolCall, RandomFloor, ModelBuilder, PostMessage, modelResponse, InferencePayload, ModelProtocol, AuthHeaders, OnlyData, GenerateImageParams } from '../index';

export class PainterRole extends ModelBuilder {
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
		'一手插在外套口袋里,另一只手轻抬至脸颊旁比\u201cV\u201d字手势,身体略侧,双脚前后交叉站立',
		'双手自然垂放于身前,手指轻轻交握,双肩微微内收,双脚并拢,站姿端正',
		'手抬起至嘴前,十指轻轻触碰\uff08做捂嘴状\uff09,身体微微后仰,一条腿向后小半步,重心落在后脚',
		'双手举过头顶比心或张开五指,一条腿向后踢起,身体微微前倾,脚尖离地呈跳跃瞬间',
		'双手食指在胸前互点,头部微微低下,双膝内扣,两脚脚尖向内呈内八站姿',
		'双手叉腰,挺胸收腹,一条腿向侧方伸出,脚尖点地,身体笔直有力',
	]
	/** 自我外观提示 */
	protected selfAppearancePrompt = fileView('prompts/selfAppearance.md')[0]
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
						"use_reference": {
							type: "boolean",
							description: "是否使用上一次生成的图像作为参考,默认值为 false"
						},
						"strength": {
							type: "number",
							description: "参考图像的影响强度,取值范围为 0 到 1,默认值为 0.65"
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
						"environment": {
							type: "string",
							description: "环境提示词,描述背景环境或场景"
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
	/** 构造函数 */
	public constructor() {
		super(fileView('prompts/painterRole.md')[0]);
	}
	/** 获得写入了动作与表情的自我外观提示词 */
	protected writeAppearancePrompt(expression?: string, posture?: string, environment?: string): string {
		/** 当前表情提示词, 默认使用随机表情提示 */
		const currentExpression = expression || this.defaultExpressionPrompt[RandomFloor(0, this.defaultExpressionPrompt.length - 1)];
		/** 当前姿势提示词, 默认使用随机姿势提示 */
		const currentPosture = posture || this.defaultPosturePrompt[RandomFloor(0, this.defaultPosturePrompt.length - 1)];
		// 替换表情提示词与姿势提示词
		return this.selfAppearancePrompt.replace('{expression}', currentExpression).replace('{posture}', currentPosture).replace('{environment}', environment || '');
	}

	/**
	 * 检查是否需要生成图片
	 *
	 * 通过关键词匹配与自然语言理解技术，分析对话消息中是否包含明确的图片生成需求。
	 * 首先进行轻量级关键词预筛选，若匹配则进一步通过 LLM 进行精确意图判断。
	 *
	 * @param {PostMessage[]} dialogueMessages - 对话角色的消息列表
	 *
	 * @returns {Promise<boolean>} - 是否需要生成图片
	 */
	public async checkImageGenerationNeed(dialogueMessages: PostMessage[]): Promise<boolean> {
		try {
			// 提取最近消息中的文本内容
			const recentTexts = this.extractRecentTexts(dialogueMessages, 5);
			if (recentTexts.length === 0) {
				console.log('[画家] 没有可用的对话文本，跳过图片生成检查');
				return false;
			}

			// 关键词预筛选：检查是否存在图片生成相关的关键词
			const hasImageKeyword = this.matchImageGenerationKeywords(recentTexts);
			if (!hasImageKeyword) {
				console.log('[画家] 关键词预筛选未命中，跳过图片生成');
				return false;
			}

			console.log('[画家] 关键词预筛选命中，进行 LLM 精确意图判断...');

			// 使用 LLM 进行精确意图判断
			const intentConfirmed = await this.confirmImageGenerationIntent(recentTexts);
			if (intentConfirmed) {
				console.log('[画家] LLM 确认存在图片生成需求');
			} else {
				console.log('[画家] LLM 判定无需生成图片');
			}

			return intentConfirmed;
		}
		catch (error) {
			console.error('[画家] 图片生成需求检测失败:', error);
			return false;
		}
	}

	/**
	 * 从消息列表中提取最近的文本内容
	 *
	 * @param {PostMessage[]} messages - 消息列表
	 * @param {number} count - 提取最近的消息数量
	 *
	 * @returns {string[]} - 提取的文本内容数组
	 */
	private extractRecentTexts(messages: PostMessage[], count: number): string[] {
		const recentMessages = messages.slice(-count);
		const texts: string[] = [];

		for (const message of recentMessages) {
			if (typeof message.content === 'string') {
				texts.push(message.content);
			} else if (Array.isArray(message.content)) {
				for (const item of message.content) {
					if (item.type === 'text') {
						texts.push(item.text);
					}
				}
			}
		}

		return texts;
	}

	/**
	 * 关键词匹配：检测文本中是否包含图片生成相关关键词
	 *
	 * @param {string[]} texts - 文本内容数组
	 *
	 * @returns {boolean} - 是否匹配到关键词
	 */
	private matchImageGenerationKeywords(texts: string[]): boolean {
		/** 图片生成关键词模式 */
		const imageKeywords = [
			/画(?:一(?:张|幅|个))?/,
			/生成(?:一(?:张|幅|个))?.*(?:图|画|图片|图像)/,
			/图片/,
			/绘画/,
			/画图/,
			/自画像/,
			/画像/,
			/绘制/,
			/创作.*(?:图|画)/,
			/帮我.*画/,
			/给我.*画/,
			/来(?:一(?:张|幅|个))?.*(?:图|画)/,
			/draw/,
			/paint/,
			/image|picture|portrait/,
			/generate.*image/,
			/create.*(?:image|picture)/,
			/插图/,
			/插画/,
			/(?:做|弄|整)(?:一(?:张|幅|个))?.*(?:图|画)/,
		];

		for (const text of texts) {
			for (const keyword of imageKeywords) {
				if (keyword.test(text)) {
					console.log(`[画家] 关键词匹配命中: "${keyword}" 在文本 "${text.slice(0, 50)}..."`);
					return true;
				}
			}
		}

		return false;
	}

	/**
	 * 通过 LLM 确认图片生成意图
	 *
	 * 使用画家的 LLM 对对话文本进行精确的意图分析，判断用户是否明确请求图片生成。
	 *
	 * @param {string[]} texts - 待分析的文本内容
	 *
	 * @returns {Promise<boolean>} - LLM 是否确认存在图片生成意图
	 */
	private async confirmImageGenerationIntent(texts: string[]): Promise<boolean> {
		try {
			/** 意图分析提示词 */
			const intentPrompt = `请分析以下对话内容，判断用户是否明确表达了生成图片、绘制图像、创作画作的意图。
仅当用户明确请求生成图片时才返回 true，否则返回 false。
请仅返回 true 或 false，不要包含其他内容。

对话内容：
${texts.map((t, i) => `[${i + 1}] ${t}`).join('\n')}`;

			// 临时设置上下文进行意图分析
			this.coverContext({ role: 'user', content: intentPrompt });
			this.runtimeMessages = [];

			/** 调用 LLM 进行意图分析 */
			const response = this.intentAnalysisRun();
			/** 获取 LLM 返回的分析结果 */
			const result = response.body?.choices?.[0]?.message?.content?.trim().toLowerCase() || '';

			console.log(`[画家] LLM 意图分析原始结果: "${result}"`);

			return result.includes('true');
		}
		catch (error) {
			console.error('[画家] LLM 意图分析失败，回退到关键词匹配结果:', error);
			// 如果 LLM 调用失败，回退到仅依赖关键词匹配的结果
			return true;
		}
	}

	/**
	 * 意图分析专用的 LLM 推理
	 * 禁用工具调用，仅进行文本分类
	 *
	 * @returns {modelResponse} - LLM 响应
	 */
	private intentAnalysisRun(): modelResponse {
		const requestBody: InferencePayload = {
			model: OnlyData.MultimodalName,
			messages: [
				{ role: 'system', content: '你是一个意图分析助手，仅返回 true 或 false。' },
				...this.messages,
				...this.runtimeMessages
			],
			stream: false,
			tool_choice: 'none',
		};

		const headers: AuthHeaders = {
			Authorization: `Bearer ${encodeURIComponent(OnlyData.SystemKey)}`,
			'Content-Type': 'application/json',
		};

		const modelRequest: ModelProtocol = {
			method: 'POST',
			crossDomain: true,
			headers,
			body: JSON.stringify(requestBody)
		};

		const endpoint = '/chat/completions';
		const [result, error] = syncFetch({ url: OnlyData.systemUrl + endpoint, execute: modelRequest });

		if (error) throw error;
		return result;
	}

	/**
	 * 执行绘画任务
	 *
	 * 这是绘画智能体的主入口方法，在思考链的最后阶段被调用。
	 * 基于对话历史和用户需求进行推理，自动提取绘画参数，并选择合适的绘画模式。
	 *
	 * @param {PostMessage[]} dialogueMessages - 对话角色的消息列表
	 * @param {string} finalResponse - 智能体的最终响应文本
	 *
	 * @returns {Promise<void>} - 绘画任务执行完成后的 Promise
	 */
	public async executePaintingTask(dialogueMessages: PostMessage[], finalResponse: string): Promise<void> {
		console.log('[画家] 开始执行绘画任务...');

		try {
			// 构建绘画任务的上下文
			this.buildPaintingContext(dialogueMessages, finalResponse);

			// 设置运行时消息（当前时间）
			this.runtimeMessages = [
				{ role: 'user', content: `当前时间: ${new Date().toLocaleString()}` }
			];

			// 执行绘画推理循环
			this.executePaintingLoop();

			console.log('[画家] 绘画任务执行完成');
		}
		catch (error) {
			console.error('[画家] 绘画任务执行失败:', error);
			throw error;
		}
	}

	/**
	 * 构建绘画任务的上下文
	 *
	 * 将对话消息和最终响应组合成适合画家 LLM 分析的上下文格式。
	 *
	 * @param {PostMessage[]} dialogueMessages - 对话角色的消息列表
	 * @param {string} finalResponse - 智能体的最终响应文本
	 */
	private buildPaintingContext(dialogueMessages: PostMessage[], finalResponse: string): void {
		/** 提取对话中的文本内容 */
		const dialogueTexts = this.extractRecentTexts(dialogueMessages, 10);

		/** 构建绘画上下文提示 */
		const paintingContext = `请根据以下对话内容，判断是否需要生成图片，并选择合适的绘画模式：

【对话历史】
${dialogueTexts.map((t, i) => `[${i + 1}] ${t}`).join('\n')}

【智能体最终回复】
${finalResponse}

请分析对话内容：
1. 判断用户是否明确请求了图片生成
2. 如果是，提取必要的绘画参数（主题、风格、色彩、构图等）
3. 选择正确的绘画模式：
   - 如果用户请求生成月华自己的形象（自画像、自己的照片、自己的样子等），使用 self_portrait 工具
   - 如果用户请求生成其他内容的图像，使用 diffusion_generation 工具

如果不需要生成图片，请直接回复无需生成图片。`;

		// 覆写画家上下文
		this.coverContext({ role: 'user', content: paintingContext });
	}

	/**
	 * 绘画模型推理
	 *
	 * 使用画家专属的绘画工具进行 LLM 推理，由 LLM 自主决定调用哪个绘画工具。
	 *
	 * @returns {modelResponse} - LLM 响应
	 */
	private painterModelRun(): modelResponse {
		const requestBody: InferencePayload = {
			model: OnlyData.MultimodalName,
			messages: [
				{ role: 'system', content: this.systemPrompt },
				...this.messages,
				...this.runtimeMessages
			],
			stream: false,
			tools: this.roleTool,
			tool_choice: 'auto',
		};

		const headers: AuthHeaders = {
			Authorization: `Bearer ${encodeURIComponent(OnlyData.SystemKey)}`,
			'Content-Type': 'application/json',
		};

		const modelRequest: ModelProtocol = {
			method: 'POST',
			crossDomain: true,
			headers,
			body: JSON.stringify(requestBody)
		};

		const endpoint = '/chat/completions';
		const [result, error] = syncFetch({ url: OnlyData.systemUrl + endpoint, execute: modelRequest });

		if (error) throw error;
		return result;
	}

	/**
	 * 执行绘画推理循环
	 *
	 * 支持多轮工具调用，直到 LLM 不再返回工具调用为止。
	 * 最大迭代次数为 3 轮，防止无限循环。
	 */
	private executePaintingLoop(): void {
		/** 最大迭代次数 */
		const MAX_ITERATIONS = 3;

		for (let i = 0; i < MAX_ITERATIONS; i++) {
			console.log(`[画家] 第 ${i + 1} 轮绘画推理`);

			/** LLM 响应 */
			let response: modelResponse;
			try {
				response = this.painterModelRun();
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
				console.log(`[画家] 模型完成绘画分析: ${replyContent.slice(0, 200)}`);
				if (replyContent) {
					this.writeContext(choice.message);
				}
				break;
			}

			console.log(`[画家] 第 ${i + 1} 轮检测到 ${toolCalls.length} 个工具调用`);

			// 将助手消息写入上下文（包含工具调用信息）
			this.writeContext(choice.message);

			// 遍历执行所有工具调用
			for (const toolCall of toolCalls) {
				console.log(`[画家] 执行工具: ${toolCall.function.name}`);

				/** 工具执行结果 */
				const result = this.executePaintingTool(toolCall);

				// 将工具执行结果写入上下文
				this.writeContext({
					role: 'tool',
					content: result,
					tool_call_id: toolCall.id
				});
			}
		}
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
	private executePaintingTool(
		toolCall: NonNullable<modelResponse['body']['choices'][0]['message']['tool_calls']>[0]
	): string {
		/** 工具函数名称 */
		const funcName = toolCall.function.name;

		/** 工具函数参数 */
		let args: Record<string, any> = {};
		try {
			args = typeof toolCall.function.arguments === 'string'
				? JSON.parse(toolCall.function.arguments)
				: toolCall.function.arguments;
		}
		catch (parseError) {
			console.error(`[画家] 工具调用参数解析失败:`, toolCall.function.arguments);
			return `工具调用参数解析失败，请确保传入合法的 JSON 字符串。错误: ${parseError}`;
		}

		console.log(`[画家] 工具参数:`, JSON.stringify(args, null, 2));

		switch (funcName) {
			case 'diffusion_generation':
				return this.handleDiffusionGeneration(args);
			case 'self_portrait':
				return this.handleSelfPortrait(args);
			default:
				console.warn(`[画家] 未知工具: ${funcName}`);
				return `未知工具: ${funcName}，可用工具为 diffusion_generation 和 self_portrait`;
		}
	}

	/**
	 * 处理扩散图像生成
	 *
	 * 基于正反提示词调用图像生成引擎，生成用户所需的图像。
	 *
	 * @param {Record<string, any>} args - 扩散生成的参数
	 * @param {string} args.prompt - 正向提示词
	 * @param {string} [args.negative_prompt] - 负向提示词
	 * @param {boolean} [args.use_reference] - 是否使用参考图像
	 * @param {number} [args.strength] - 参考图像影响强度
	 * @param {number} [args.cfg_scale] - 提示词权重
	 *
	 * @returns {string} - 生成结果描述
	 */
	private handleDiffusionGeneration(args: Record<string, any>): string {
		try {
			/** 正向提示词 */
			const prompt = args.prompt || '';
			if (!prompt.trim()) {
				return '扩散生成失败：正向提示词不能为空';
			}

			console.log(`[画家] 扩散生成 - 正向提示词: ${prompt.slice(0, 100)}...`);

			/** 构建图像生成参数 */
			const imageParams: GenerateImageParams = {
				prompt: prompt,
				negativePrompt: args.negative_prompt || '',
				strength: args.strength ?? 0.65,
				cfgScale: args.cfg_scale ?? 1.0,
			};

			// 如果使用参考图像，需要设置初始图像路径
			if (args.use_reference) {
				console.log('[画家] 使用参考图像模式');
				// TODO: 从持久化存储中获取上一次生成的图像路径
			}

			console.log('[画家] 调用图像生成引擎...');

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
	 * @param {Record<string, any>} args - 自画像生成的参数
	 * @param {string} args.expression - 表情描述
	 * @param {string} args.posture - 姿势描述
	 * @param {string} args.environment - 环境描述
	 *
	 * @returns {string} - 生成结果描述
	 */
	private handleSelfPortrait(args: Record<string, any>): string {
		try {
			/** 表情描述 */
			const expression = args.expression || '';
			/** 姿势描述 */
			const posture = args.posture || '';
			/** 环境描述 */
			const environment = args.environment || '';

			console.log(`[画家] 自画像生成 - 表情: "${expression}", 姿势: "${posture}", 环境: "${environment}"`);

			// 构建完整的自画像提示词
			const fullPrompt = this.writeAppearancePrompt(expression, posture, environment);

			console.log(`[画家] 自画像完整提示词长度: ${fullPrompt.length} 字符`);

			/** 构建图像生成参数 */
			const imageParams: GenerateImageParams = {
				prompt: fullPrompt,
				negativePrompt: '低质量,模糊,畸形,多只手,多只脚,坏手,坏脚,NSFW',
				cfgScale: 1.0,
			};

			console.log('[画家] 调用图像生成引擎生成自画像...');

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