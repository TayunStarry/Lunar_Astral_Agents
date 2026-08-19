import { GlobalConfig, ChatCache, ModelResponseBody, AgentDefine, ModelBuilder, PostMessage } from '../index';

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
			// 从向量数据库查询相关历史消息作为 RAG 上下文
			this.queryRagMessages();
			/** 向处理器模型发送请求并等待响应 */
			const response = this.run(this.ragMessages, GlobalConfig.LTPdefinition);
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
	/** 格式化历史消息：图片总数≥20时先摘要再扁平化为纯文本 */
	public formatHistoricalMessages(source: AgentDefine) {
		// 如果消息数组为空,则不处理
		if (this.messages.length === 0) return;
		/** 整个消息队列中的图片帧总数 */
		const totalImages = this.countTotalImages(this.messages);
		// 图片帧数≥20：对每个多媒体消息摘要图片，最终全部扁平化为纯文本
		if (totalImages >= 20) {
			/** 处理后的纯文本消息数组 */
			const processedMessages: PostMessage[] = [];
			// 遍历所有消息对象
			for (const message of this.messages) {
				// 已是纯文本：直接保留
				if (typeof message.content === 'string') {
					processedMessages.push(message);
					continue;
				}
				/** 对多媒体消息执行图片摘要，合并为纯文本 */
				const textResult = this.summarizeMessageImages(message, source);
				// 如果摘要结果为空或仅包含空格,则跳过
				if (!textResult || textResult.trim() === '') continue;
				// 保留摘要结果
				processedMessages.push({ role: message.role, content: textResult });
			}
			this.messages = processedMessages;
		}
		// 如果处理后消息数组为空（摘要全部失败等极端情况），跳过续写
		if (this.messages.length === 0) return;
		/** 最新消息的角色 */
		const latestRole = this.messages.slice(-1)[0].role;
		// 如果最新消息是用户或工具,则不处理
		if (latestRole === 'user' || latestRole === 'tool') return;
		/** 随机拼接提示词：从多个维度随机组合，生成自然的话题引导 */
		const prompt = this.buildContinuationPrompt();
		// 添加随机拼接的提示词到处理器模型的上下文
		this.writeContext({ role: 'user', content: prompt });
	}
	/** 对单个多媒体消息中的图片执行摘要，返回合并后的纯文本；无内容时返回 null */
	private summarizeMessageImages(message: PostMessage, source: AgentDefine): string | null {
		if (typeof message.content === 'string') return message.content;
		/** 消息中的图片内容项 */
		const imageItems = message.content.filter((c: any) => c.type === 'image_url');
		/** 消息中的文本内容项 */
		const textItems = message.content.filter((c: any) => c.type === 'text');
		/** 原始文本部分 */
		const textPart = textItems.map((c: any) => c.text).join('\n');
		// 无图片时直接返回文本
		if (imageItems.length === 0) return textPart || null;
		try {
			// 将图片包装为独立消息，喂给描述角色进行摘要
			source.descriptionRole.coverContext({ role: 'user', content: imageItems });
			/** 运行描述角色模型，获取图片摘要 */
			const summaryRequest = source.descriptionRole.run([], []);
			/** 图片摘要结果 */
			const summary = summaryRequest.body?.choices?.[0]?.message?.content;
			// 检查摘要结果是否有效
			if (summary && summary.trim().length > 0) {
				return textPart ? `${textPart}\n[图片描述：${summary}]` : `[图片描述：${summary}]`;
			}
			// 摘要为空时仅保留文本
			return textPart || null;
		}
		catch (error) {
			console.error('[对话者] 图片摘要异常:', error);
			return textPart || null;
		}
	}
	/** 统计单条消息中的 image_url 项数量 */
	private countImagesInMessage(message: PostMessage): number {
		if (typeof message.content === 'string') return 0;
		return message.content.filter((c: any) => c.type === 'image_url').length;
	}
	/** 统计消息数组中所有图片帧的总数 */
	private countTotalImages(messages: PostMessage[]): number {
		return messages.reduce((sum, m) => sum + this.countImagesInMessage(m), 0);
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
			const lunarToolPackage = GlobalConfig.LTPfunction.get(functionName);
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