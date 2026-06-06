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
			const response = this.run(this.ragMessages, []);
			// 处理响应文本内容
			this.analyzeMessageResponse(response.body, cache, source);
			// 如果有工具调用,处理它们并重新发送请求
			if (cache.toolCalls.length > 0) {
				/** 处理工具调用 */
				const hasProcessedToolCalls = await this.batchExecutionToolCall(cache, source);
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
	/** 格式化历史消息
	 *  处理流程：
	 *  1. 遍历原始消息数组，将嵌套的多模态消息结构扁平化为独立消息
	 *  2. 扁平化过程中严格保持消息的原始时间顺序（数组下标顺序即为时间顺序）
	 *  3. 若视觉消息总数超过10条，对连续的视觉消息组分批摘要，摘要结果插入原位
	 *  4. 不执行任何去重操作，确保所有消息完整保留
	 *
	 *  时间顺序保证机制：
	 *  - PostMessage 类型不含时间戳字段，消息数组的下标顺序即为唯一的时间依据
	 *  - 扁平化时按原始数组顺序逐条处理，拆分后的子消息紧跟原消息位置
	 *  - 视觉消息摘要替换原消息组的位置，不改变前后文本消息的相对顺序
	 */
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
					if (content.type === 'text') {
						flattenedMessages.push({ role: message.role, content: content.text });
					}
					// 视觉内容项：保留为单条视觉消息（内容为单元素数组）
					else {
						flattenedMessages.push({ role: message.role, content: [content] });
					}
				}
			}
		}
		// 统计视觉消息总数，用于判断是否需要分批摘要
		const visionCount = flattenedMessages.filter(m => Array.isArray(m.content)).length;
		// 视觉消息数量<=10，无需摘要，直接使用扁平化结果
		if (visionCount <= 10) {
			this.messages = flattenedMessages;
			return;
		}
		// 视觉消息数量>10，需要对连续的视觉消息组分批摘要以减少上下文长度
		// 处理策略：遍历扁平化数组，遇到连续视觉消息时累积到缓冲区，
		// 遇到非视觉消息时先处理缓冲区，确保摘要结果插入原位，不改变前后消息的相对顺序
		/** 处理后的最终消息数组 */
		const processedMessages: PostMessage[] = [];
		/** 当前连续视觉消息的缓冲区 */
		let visionBuffer: PostMessage[] = [];
		for (const message of flattenedMessages) {
			// 判断当前消息是否为视觉消息（内容为数组类型）
			const isVisionMessage = Array.isArray(message.content);
			if (isVisionMessage) {
				// 累积连续的视觉消息到缓冲区
				visionBuffer.push(message);
			} else {
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
	/** 处理连续视觉消息缓冲区
	 *  当连续视觉消息数量<=10时，直接保留原消息
	 *  当连续视觉消息数量>10时，分批调用描述角色进行摘要，摘要结果替换原消息组
	 *  摘要结果插入到输出数组的当前位置，保证时间顺序正确
	 */
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
	protected analyzeMessageResponse(message: ModelResponseBody, cache: ChatCache, source: AgentDefine): void {
		try {
			// 处理推理内容数据
			if (message.choices?.[0]?.message?.reasoning_content) {
				cache.thinkingContent = message.choices[0].message.reasoning_content;
			}
			// 检查是否有词元生成速度数据
			if (message.timings?.predicted_per_second) {
				source.responseSpeed = message.timings.predicted_per_second;
				console.log(`词元生成速度: ${message.timings.predicted_per_second}`);
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
	protected async batchExecutionToolCall(state: ChatCache, source: AgentDefine): Promise<boolean> {
		/** 工具调用标志 */
		let hasToolCalls = false;
		// 遍历所有工具调用
		for (const toolCall of state.toolCalls) {
			/** 工具函数名称 */
			const functionName = toolCall.function.name;
			/** 工具函数参数 */
			const functionArgs = toolCall.function.arguments;
			/** 查询对应的月华工具包 */
			const lunarToolPackage = OnlyData.lunarToolPackageMap.get(functionName);
			// 检查是否有对应的工具包
			if (!lunarToolPackage) {
				source.unreadContext.push({ role: "tool", content: `未找到工具包: ${functionName}`, tool_call_id: toolCall.id });
				continue;
			}
			try {
				/** 工具函数执行结果 */
				const toolResult = await lunarToolPackage(functionArgs);
				// 将工具响应添加到消息历史中
				source.unreadContext.push({ role: "tool", content: toolResult, tool_call_id: toolCall.id });
				// 标记有工具调用
				hasToolCalls = true;
			}
			catch (error) {
				// 将工具调用失败信息添加到消息历史中
				source.unreadContext.push({ role: "tool", content: `调用${functionName}失败: ${error}`, tool_call_id: toolCall.id });
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