import { OnlyData, ChatCache, ModelResponseBody, AgentDefine, ModelBuilder, PostMessage } from '../index';

/** 聊天对话角色 */
export class DialogueRole extends ModelBuilder {
	/** 发送请求并获取响应 */
	public async callMultimediaAndToolParsing(cache: ChatCache, source: AgentDefine): Promise<void> {
		try {
			// 对消息中的图片文件进行压缩与解析处理
			await source.LiteImageFile();
			// 合并 学习者角色 历史摘要
			source.learnerRole.consumeHistory().forEach(msg => source.unreadContext.push(msg));
			// 合并 画家角色 历史摘要
			source.painterRole.consumeHistory().forEach(msg => source.unreadContext.push(msg));
			// 合并 音乐家角色 历史摘要
			source.musicianRole.consumeHistory().forEach(msg => source.unreadContext.push(msg));
			// 将未读上下文数组中的消息添加到处理器模型的上下文
			source.unreadContext.forEach(context => this.writeContext(context));
			// 清空未读上下文数组
			source.unreadContext = [];
			// 格式化历史消息
			this.formatHistoricalMessages(source);
			// 添加当前时间到运行时消息列表
			this.runtimeMessages = [{ role: 'user', content: `当前时间: ${new Date().toLocaleString()}` }];
			// 从向量数据库查询相关历史消息作为 RAG 上下文
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
		/** 判断消息是否为音频消息（input_audio 类型） */
		const isAudioMessage = (c: any): boolean => c.type === 'input_audio';
		// 遍历原始消息数组，将嵌套结构转换为扁平结构
		for (const message of this.messages) {
			// 消息内容为数组时，根据是否含音频采取不同策略
			if (typeof message.content !== 'string') {
				// 检查消息是否包含音频内容
				if (message.content.some(isAudioMessage)) {
					flattenedMessages.push(message);
					continue;
				}
				// 非音频消息（图片等）：按原逻辑拆分每个内容项为独立消息
				// 但若消息同时包含文本和图片，保留合并结构（避免多模态代理因
				// 文本缺少媒体标记而报错："number of media markers does not match"）
				const hasText = message.content.some((c: any) => c.type === 'text');
				const hasImage = message.content.some((c: any) => c.type === 'image_url');
				if (hasText && hasImage) {
					flattenedMessages.push(message);
					continue;
				}
				for (const content of message.content) {
					// 文本内容项：提取为纯文本消息
					if (content.type === 'text') flattenedMessages.push({ role: message.role, content: content.text });
					// 视觉内容项：保留为单条消息（内容为单元素数组）
					else flattenedMessages.push({ role: message.role, content: [content] });
				}
			}
			else flattenedMessages.push(message);
		}
		/** 视觉消息总数 */
		const visionCount = flattenedMessages.filter(m => { if (!Array.isArray(m.content) || m.content.some(isAudioMessage)) return false; }).length;
		// 视觉消息数量<=10，无需摘要，直接使用扁平化结果
		if (visionCount <= 10) this.messages = flattenedMessages;
		// 视觉消息数量>10，需要对连续的视觉消息组分批摘要以减少上下文长度
		else {
			/** 处理后的最终消息数组 */
			const processedMessages: PostMessage[] = [];
			/** 当前连续视觉消息的缓冲区 */
			let visionBuffer: PostMessage[] = [];
			// 遍历扁平化后的消息数组
			for (const message of flattenedMessages) {
				// 判断当前消息是否为视觉消息（内容为数组类型，且非音频消息）
				const isVisionMessage = Array.isArray(message.content) && !message.content.some(isAudioMessage);
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
		/** 随机拼接提示词：从多个维度随机组合，生成自然的话题引导 */
		const prompt = this.buildContinuationPrompt();
		// 添加随机拼接的提示词到处理器模型的上下文
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
				// 将工具响应添加到消息历史中，图片与文本合并为单条消息
				if (base64Image && typeof base64Image === 'string' && base64Image.length > 0) {
					/** 合并文本与图片消息 */
					const message: PostMessage = {
						role: "user",
						content: [
							{ type: "text", text: textContent },
							{ type: "image_url", image_url: { url: base64Image } }
						]
					}
					this.messages.push(message);
					console.log(`[工具调用] ${functionName} 返回图片数据，长度=${base64Image.length} 字节`);
				}
				else this.messages.push({ role: "tool", content: textContent, tool_call_id: toolCall.id });
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
			source.finalResponse = state.descriptionContent;
			// 打印思考标签内容
			console.log(newThinkTag);
		}
		// 直接使用描述内容
		else source.finalResponse = state.descriptionContent;
		// 返回修正后的消息内容
		return source.finalResponse;
	}
	/** 随机拼接续写提示词：从多个维度抽取条目组合，生成自然的话题引导 */
	private buildContinuationPrompt(): string {
		/** 随机取数组元素 */
		const pick = <T>(arr: T[]): T => arr[Math.floor(Math.random() * arr.length)];
		/** 兴趣话题 */
		const interests = [
			'旅行', '游戏', '音乐', '电影', '书籍', '动漫', '美食', '运动',
			'摄影', '绘画', '手工', '编程', '天文', '历史', '哲学', '科技',
			'宠物', '园艺', '穿搭', '舞蹈', '乐器', '写作', '钓鱼', '骑行',
		];
		/** 正在做的事情 */
		const doing = [
			'正在喝一杯热茶', '正在窗边发呆', '刚刚整理完房间', '正在浏览网页',
			'正在听一首新歌', '刚刚看完一段视频', '正在翻看旧照片', '正在写日记',
			'正在做手工', '正在画一幅画', '正在弹琴', '正在做一道菜',
			'正在散步', '正在看窗外的风景', '正在刷手机', '正在整理书架',
		];
		/** 想做的事情 */
		const wantTo = [
			'想去海边看日落', '想学一门新乐器', '想去看一场演唱会', '想去爬山',
			'想养一只猫', '想尝试做一道新菜', '想去看极光', '想去逛博物馆',
			'想学画画', '想去露营', '想写一首诗', '想去看一场电影',
			'想去游乐园', '想学跳舞', '想去看樱花', '想开一家小店',
		];
		/** 所在位置 */
		const location = [
			'坐在窗边的书桌前', '窝在沙发里', '躺在草地上', '站在阳台上',
			'靠在床头', '坐在咖啡馆的角落', '在公园的长椅上', '在图书馆里',
			'在厨房里', '在工作室里', '在花园里', '在天台上',
		];
		/** 当前动作 */
		const action = [
			'伸了个懒腰', '托着下巴', '揉了揉眼睛', '转着手里的笔',
			'轻轻哼着歌', '翘着二郎腿', '抱着抱枕', '拨弄着头发',
			'用手指敲着桌面', '晃着双脚', '靠在椅背上', '侧着头',
		];
		/** 当前心情 */
		const mood = [
			'心情很放松', '觉得有点无聊', '心情特别好', '有点小期待',
			'感觉懒洋洋的', '很平静', '有点好奇', '心情不错',
			'稍微有点困', '精神很好', '有点怀旧', '感觉很温暖',
		];

		/** 所有维度池 */
		const pools: { label: string; items: string[] }[] = [
			{ label: '兴趣', items: interests },
			{ label: '正在做', items: doing },
			{ label: '想做', items: wantTo },
			{ label: '位置', items: location },
			{ label: '动作', items: action },
			{ label: '心情', items: mood },
		];

		// 随机选取 2~3 个维度
		const count = 2 + Math.floor(Math.random() * 2); // 2 或 3
		const shuffled = [...pools].sort(() => Math.random() - 0.5);
		const selected = shuffled.slice(0, count);

		// 从每个选中维度随机取一条
		const parts = selected.map(p => pick(p.items));

		// 拼接为自然语句
		const prefix = pick(['你', '现在你', '此刻你', '这会儿你',]);
		const suffix = pick(['，聊点什么吧~', '，来聊聊吧~', '，说说看吧~', '，展开聊聊？', '，有什么想说的吗？', '，分享一下呗~',]);
		return `${prefix}${parts.join('，')}${suffix}`;
	}
	/** 构造函数 */
	public constructor() {
		super(fileView('prompts/dialogueRole.md')[0]);
	}
}