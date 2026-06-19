import { OnlyData, ChatCache, ModelResponseBody, AgentDefine, ModelBuilder, PostMessage } from '../index';

/** 聊天对话角色 */
export class DialogueRole extends ModelBuilder {
	/** 发送请求并获取响应 */
	public async callMultimediaAndToolParsing(cache: ChatCache, source: AgentDefine): Promise<void> {
		try {
			// 对消息中的图片文件进行压缩与解析处理
			await source.LiteImageFile();
			// 将未读上下文数组中的消息添加到处理器模型的上下文
			source.unreadContext.forEach(context => this.writeContext(context));
			// 清空未读上下文数组
			source.unreadContext = [];
			// 格式化历史消息
			this.formatHistoricalMessages(source);
			// 添加当前时间到运行时消息列表
			this.runtimeMessages = [{ role: 'user', content: `当前时间: ${new Date().toLocaleString()}` }];
			// 从 chromem-go 查询相关历史消息作为 RAG 上下文
			this.queryRagMessages();
			/** 向处理器模型发送请求并等待响应 */
			const response = this.run(this.ragMessages, [...OnlyData.LTPdefinition]);
			// 处理响应文本内容
			this.analyzeMessageResponse(response.body, cache);
			// 如果有工具调用,处理它们并重新发送请求
			if (cache.toolCalls.length > 0) {
				// 在递归前写入 assistant 的 tool_call 消息，确保上下文完整
				this.writeContext(response.body.choices?.[0]?.message);
				/** 处理工具调用 */
				const hasProcessedToolCalls = await this.batchExecutionToolCall(cache);
				// 如果有处理过的工具调用,重新发送请求（包含工具调用结果）
				if (hasProcessedToolCalls) return await this.callMultimediaAndToolParsing(cache, source);
			}
			// 在历史上下文中添加模型响应
			this.writeContext(response.body.choices?.[0]?.message);
		}
		catch (error) {
			console.error('请求处理错误:', error);
		}
		// 更新消息内容
		this.updateMessageContent(cache, source);
	}
	/** 格式化历史消息 */
	public formatHistoricalMessages(source: AgentDefine) {
		// 如果消息数组为空,则不处理
		if (this.messages.length === 0) return;
		/** 扁平化后的消息数组，严格保持原始时间顺序 */
		const flattenedMessages: PostMessage[] = [];
		// 遍历原始消息数组，将嵌套结构转换为扁平结构
		// 关键：按数组下标顺序处理，确保时间先后关系不变
		for (const message of this.messages) {
			// 消息内容为字符串时，已是扁平结构，直接保留
			if (typeof message.content === 'string') {
				flattenedMessages.push(message);
			}
			// 消息内容为数组时，将每个内容项拆分为独立消息
			// 拆分后的子消息按原数组内顺序依次追加，保持时间先后
			else {
				for (const content of message.content) {
					// 文本内容项：提取为纯文本消息
					if (content.type === 'text') flattenedMessages.push({ role: message.role, content: content.text });
					// 视觉内容项：保留为单条视觉消息（内容为单元素数组）
					else flattenedMessages.push({ role: message.role, content: [content] });
				}
			}
		}
		// 统计视觉消息总数，用于判断是否需要分批摘要
		const visionCount = flattenedMessages.filter(m => Array.isArray(m.content)).length;
		// 视觉消息数量<=10，无需摘要，直接使用扁平化结果
		if (visionCount <= 10) {
			this.messages = flattenedMessages;
		}
		// 视觉消息数量>10，需要对连续的视觉消息组分批摘要以减少上下文长度
		// 处理策略：遍历扁平化数组，遇到连续视觉消息时累积到缓冲区，
		// 遇到非视觉消息时先处理缓冲区，确保摘要结果插入原位，不改变前后消息的相对顺序
		else {
			/** 处理后的最终消息数组 */
			const processedMessages: PostMessage[] = [];
			/** 当前连续视觉消息的缓冲区 */
			let visionBuffer: PostMessage[] = [];
			// 遍历扁平化后的消息数组
			for (const message of flattenedMessages) {
				// 判断当前消息是否为视觉消息（内容为数组类型）
				const isVisionMessage = Array.isArray(message.content);
				// 累积连续的视觉消息到缓冲区
				if (isVisionMessage) visionBuffer.push(message);
				else {
					// 遇到非视觉消息，先处理缓冲区中累积的视觉消息
					// 这确保视觉消息的摘要结果出现在正确的位置（在当前文本消息之前）
					if (visionBuffer.length > 0) {
						this.processVisionBuffer(visionBuffer, processedMessages, source);
						visionBuffer = [];
					}
					processedMessages.push(message);
				}
			}
			// 处理末尾可能残留的视觉消息缓冲区
			if (visionBuffer.length > 0) {
				this.processVisionBuffer(visionBuffer, processedMessages, source);
			}
			// 覆写处理器模型的上下文为处理后的消息数组
			this.messages = processedMessages;
		}
		// 续写提示词逻辑：所有场景共享
		/** 最新消息的角色 */
		const latestRole = this.messages.slice(-1)[0].role;
		// 如果最新消息是用户或工具,则不处理
		if (latestRole === 'user' || latestRole === 'tool') return;
		/** 继续话题的提示词列表 */
		const continuationPrompts = [
			'请延续当前话题，继续展开讨论。',
			'请完善当前话题，对已有内容进行补充和优化。',
			'请将话题转向旅行，聊聊旅行相关的见闻或计划。',
			'请将话题转向游戏，聊聊最近有趣的游戏体验。',
			'请将话题转向音乐，聊聊最近在听的音乐或音乐推荐。',
			'请将话题转向电影，聊聊最近看过或想看的电影。',
			'请将话题转向书籍，聊聊最近在读或推荐的书籍。',
			'请将话题转向动漫，聊聊最近在追或推荐的动漫。',
		];
		/** 随机选择一个提示词 */
		const prompt = continuationPrompts[Math.floor(Math.random() * continuationPrompts.length)];
		// 添加随机选择的提示词到处理器模型的上下文
		this.writeContext({ role: 'user', content: prompt });
	}
	/** 处理连续视觉消息缓冲区 */
	private processVisionBuffer(buffer: PostMessage[], output: PostMessage[], source: AgentDefine): void {
		// 连续视觉消息<=10条，直接追加到输出数组，保持原始顺序
		if (buffer.length <= 10) {
			output.push(...buffer);
			return;
		}
		// 连续视觉消息>10条，分批摘要处理，每批最多10条
		for (let i = 0; i < buffer.length; i += 10) {
			/** 截取当前批次的视觉消息（每批次最多10条） */
			const batchFrames = buffer.slice(i, i + 10);
			// 覆盖描述角色的上下文，传入当前批次的视觉消息
			source.descriptionRole.coverContext(batchFrames);
			/** 执行描述角色的模型运行，获取总结请求响应 */
			const summaryRequest = source.descriptionRole.run([], []);
			/** 模型总结结果 */
			const summary = summaryRequest.body?.choices?.[0]?.message?.content;
			// 过滤空字符串和仅包含空格的字符串，将摘要作为用户消息插入到当前位置
			if (summary && summary.trim().length > 0) {
				output.push({ role: 'user', content: summary });
			}
		}
	}
	/** 处理聊天消息响应 */
	protected analyzeMessageResponse(message: ModelResponseBody, cache: ChatCache): void {
		try {
			// 处理推理内容数据
			if (message.choices?.[0]?.message?.reasoning_content) {
				cache.thinkingContent = message.choices[0].message.reasoning_content;
			}
			// 检查是否有词元生成速度数据
			if (message.timings?.predicted_per_second) {
				console.log(`词元生成速度: ${message.timings.predicted_per_second}`);
			}
			// 检查缓存命中数量
			if (message.timings?.cache_n !== undefined) {
				console.log(`缓存命中数量: ${message.timings.cache_n}`);
			}
			// 处理工具调用
			if (message.choices?.[0]?.message?.tool_calls) {
				// 遍历所有工具调用
				for (const toolCall of message.choices[0].message.tool_calls) {
					try {
						// 解析arguments字段
						toolCall.function.arguments = JSON.parse(toolCall.function.arguments);
						// 记录工具调用
						cache.toolCalls.push(toolCall);
					}
					catch (parseError) {
						console.error('工具调用参数解析错误:', parseError);
					}
				}
			}
			// 处理内容数据
			if (message.choices?.[0]?.message?.content) {
				cache.descriptionContent = message.choices[0].message.content;
			}
		}
		catch (error) {
			console.error('聊天消息响应处理错误:', error);
		}
	}
	/** 批量执行工具调用 */
	protected async batchExecutionToolCall(state: ChatCache): Promise<boolean> {
		/** 工具调用标志 */
		let hasToolCalls = false;
		// 遍历所有工具调用
		for (const toolCall of state.toolCalls) {
			/** 工具函数名称 */
			const functionName = toolCall.function.name;
			/** 工具函数参数 */
			const functionArgs = toolCall.function.arguments;
			/** 查询对应的月华工具包 */
			const lunarToolPackage = OnlyData.LTPfunction.get(functionName);
			// 检查是否有对应的工具包
			if (!lunarToolPackage) {
				this.messages.push({ role: "tool", content: `未找到工具包: ${functionName}`, tool_call_id: toolCall.id });
				continue;
			}
			try {
				/** 工具函数执行结果：string[]格式，下标0=文本内容，下标1=图片base64数据（无则为空字符串） */
				const toolResult = await lunarToolPackage(functionArgs);
				/** 提取文本内容 */
				const textContent = Array.isArray(toolResult) ? toolResult[0] : String(toolResult);
				/** 提取图片base64数据 */
				const base64Image = Array.isArray(toolResult) ? toolResult[1] : '';
				// 将文本工具响应添加到消息历史中
				this.messages.push({ role: "tool", content: textContent, tool_call_id: toolCall.id });
				// 如果有图片数据，作为视觉消息追加到消息历史中
				if (base64Image && typeof base64Image === 'string' && base64Image.length > 0) {
					this.messages.push({ role: "tool", content: [{ type: "image_url", image_url: { url: base64Image } }], tool_call_id: toolCall.id });
					console.log(`[工具调用] ${functionName} 返回图片数据，长度=${base64Image.length} 字节`);
				}
				// 标记有工具调用
				hasToolCalls = true;
			}
			catch (error) {
				// 将工具调用失败信息添加到消息历史中
				this.messages.push({ role: "tool", content: `调用${functionName}失败: ${error}`, tool_call_id: toolCall.id });
			}
		}
		// 处理完所有工具调用后,清空状态
		state.currentToolCallIndex = -1;
		state.currentFunctionArgs = "";
		state.currentFunctionName = "";
		state.currentToolCall = null;
		state.toolCalls = [];
		// 标记有工具调用
		return hasToolCalls;
	};
	/** 更新消息内容 */
	protected updateMessageContent(state: ChatCache, source: AgentDefine): string {
		// 检查推理内容是否为空
		if (state.thinkingContent.trim() !== "") {
			/** 新的思考标签内容 */
			const newThinkTag = '<think>\n' + state.thinkingContent + '\n</think>\n';
			// 合并为带有思考标签的描述内容
			// source.finalResponse = newThinkTag + state.descriptionContent;
			source.finalResponse = state.descriptionContent;
			// 打印思考标签内容
			console.log(newThinkTag);
		}
		// 直接使用描述内容
		else source.finalResponse = state.descriptionContent;
		// 返回修正后的消息内容
		return source.finalResponse;
	}
	/** 构造函数 */
	public constructor() {
		super(fileView('prompts/dialogueRole.md')[0]);
	}
}