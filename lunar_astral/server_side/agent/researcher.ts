import { ToolCall, ToolCallItem, modelResponse, OnlyData, SearchMode, BaseConfig, PostMessage, ModelBuilder } from '../index';

/** 网络搜索参数接口 */
interface WebSearchParams {
	/** 搜索查询关键词或问题 */
	query: string;
	/** 搜索模式：simple（轻量摘要）、webpage（网页搜索，默认）、depth（深度研究）、assembly（大会辩论式深度研究） */
	mode?: SearchMode | 'assembly';
}

/** 记忆库查询参数接口 */
interface MemoryQueryParams {
	/** 查询文本 */
	query: string;
	/** 返回结果数量，默认10 */
	top_k?: number;
}

/** 链接处理参数接口 */
interface ProcessLinksParams {
	/** 包含链接的文本内容 */
	text: string;
}

/** 研究详情记录（单次工具调用的发现） */
interface ResearchDetail {
	/** 工具名称 */
	toolName: string;
	/** 查询关键词 */
	query: string;
	/** 搜索模式（web_search 专用） */
	mode?: string;
	/** 查询返回的关键信息摘要 */
	keyFindings: string;
}

/** 研究者角色 */
export class ResearcherRole extends ModelBuilder {
	/** 独立历史（跨周期持久化，供对话者消费后清空） */
	private _history: PostMessage[] = [];
	/** 对话历史读取条数 */
	private readonly DIALOGUE_HISTORY_LIMIT = 15;
	/** 自身历史读取条数 */
	private readonly OWN_HISTORY_LIMIT = 5;
	/** 最大推理迭代次数 */
	private readonly MAX_ITERATIONS = 5;
	/** 未读消息检查条数 */
	private readonly UNREAD_CHECK_COUNT = 10;
	/** 网络检索子系统是否已初始化 */
	private webSearchInitialized = false;
	/** 大会辩论模式记忆库是否已注入 */
	private assemblyMemoryInjected = false;

	/** 研究者专属工具定义 */
	private readonly researchTools: ToolCall[] = [
		{
			type: "function",
			function: {
				name: "web_search",
				description: "执行网络搜索，获取实时信息。当需要联网获取实时数据、最新资讯、事实查询等信息时使用。支持四种模式：simple（轻量摘要）、webpage（网页搜索，默认）、depth（深度研究，子问题拆解并行搜索）、assembly（大会辩论式深度研究，维新派vs守旧派多轮辩论，综合网络与记忆库信息生成报告，适合复杂争议性问题）。",
				parameters: {
					type: "object",
					properties: {
						query: {
							type: "string",
							description: "搜索查询关键词或问题"
						},
						mode: {
							type: "string",
							description: "搜索模式：simple（轻量摘要）、webpage（网页搜索，默认）、depth（深度研究）或 assembly（大会辩论式深度研究，综合网络搜索与记忆库）",
							enum: ["simple", "webpage", "depth", "assembly"]
						}
					},
					required: ["query"]
				}
			}
		},
		{
			type: "function",
			function: {
				name: "memory_query",
				description: "查询内部记忆库中的历史对话和事件记录。用于回忆过去的对话内容、查找用户偏好、追溯历史事件等需要从内部记忆中检索信息的场景。",
				parameters: {
					type: "object",
					properties: {
						query: {
							type: "string",
							description: "查询文本，用于在记忆库中搜索相关记录"
						},
						top_k: {
							type: "number",
							description: "返回的最相关结果数量，默认10"
						}
					},
					required: ["query"]
				}
			}
		},
		{
			type: "function",
			function: {
				name: "process_links",
				description: "处理消息中的链接。自动识别链接类型：网页链接抓取内容并总结、图片链接使用视觉模型识别、下载链接自动下载文件。处理后将原始链接替换为摘要标签。",
				parameters: {
					type: "object",
					properties: {
						text: {
							type: "string",
							description: "包含链接的文本内容，工具会自动提取并处理其中的所有链接"
						}
					},
					required: ["text"]
				}
			}
		}
	];

	/** 研究意图关键词模式 — 匹配需要深度智慧的意图场景 */
	private readonly researchKeywords = [
		/查(?:一查|一下|询|找|找找|看看)/,
		/搜索/,
		/搜(?:一搜|一下)/,
		/搜索(?:一搜|一下)/,
		/(?:帮我|给我|为我|替我)(?:查|搜索|找|调查|研究|检索|查询)/,
		/研究(?:一(?:下|研究))/,
		/调查(?:一(?:下|调查))?/,
		/思考(?:一(?:下|思考))?/,
		/回忆(?:一(?:下|回忆))?/,
		/想想?(?:看|起|到)/,
		/记不记得/,
		/还记得/,
		/以前(?:说过|聊过|讨论过|提过|提到)/,
		/上次(?:说|聊|讨论|提|提到)/,
		/了解(?:一(?:下|了解))?/,
		/(?:是|到底(?:是)|究竟(?:是))什么/,
		/(?:怎么|为什么|怎么回事)/,
		/(?:最新|最近|当前|目前|今天|现在).*(?:消息|新闻|情况|状态|动态|信息|数据)/,
		/(?:有没有|是否).*(?:相关|关于)/,
		/深入(?:了解|分析|研究)/,
		/详细(?:了解|分析|说明|解释)/,
		/分析(?:一(?:下|分析))?/,
		/核实/,
		/验证/,
		/(?:真|假|正确|错误|靠谱|可靠)/,
		/(?:资料|文献|论文|报告|数据|统计)/,
	];

	/** 构造函数 */
	public constructor() {
		super(fileView('prompts/researcherRole.md')[0]);
	}

	/** 获取历史摘要（对话者调用后清空） */
	public consumeHistory(): PostMessage[] {
		const result = [...this._history];
		this._history = [];
		return result;
	}

	/**
	 * 执行研究流程
	 *
	 * @param dialogueMessages 对话历史消息（来自 dialogueRole.messages）
	 * @param unreadContext 当前未读上下文快照
	 * @param count 检查的消息数量
	 *
	 * @returns true 表示未执行研究，false 表示已执行研究
	 */
	public executeResearch(dialogueMessages: PostMessage[], unreadContext: PostMessage[], count: number = this.UNREAD_CHECK_COUNT): boolean {
		// 构建上下文：对话历史 + 自身历史 + 当前未读
		const dialogueHistory = dialogueMessages.slice(-this.DIALOGUE_HISTORY_LIMIT);
		const ownHistory = this._history.slice(-this.OWN_HISTORY_LIMIT);
		this.coverContext([...dialogueHistory, ...ownHistory, ...unreadContext]);

		// 提取未读消息文本
		const unreadTexts = this.extractUnreadTexts(unreadContext, count);

		// 检查是否匹配研究关键词
		if (!this.matchKeywords(unreadTexts)) return true;

		// 研究详情收集
		const details: ResearchDetail[] = [];

		// 推理循环：多轮搜索与查询
		for (let i = 0; i < this.MAX_ITERATIONS; i++) {
			console.log(`[研究者] 第 ${i + 1} 轮推理`);
			let response: modelResponse;
			try {
				response = this.run([], this.researchTools);
			}
			catch (error) {
				console.error(`[研究者] 第 ${i + 1} 轮推理失败:`, error);
				break;
			}
			const choice = response.body?.choices?.[0];
			if (!choice) {
				console.log(`[研究者] 模型返回空结果，结束循环`);
				break;
			}
			const toolCalls = choice.message?.tool_calls;
			// 如果模型没有调用任何工具，结束循环
			if (!toolCalls || toolCalls.length === 0) break;
			// 将助手消息写入上下文（包含工具调用信息）
			this.writeContext(choice.message);
			// 遍历执行所有工具调用
			for (const toolCall of toolCalls) {
				console.log(`[研究者] 执行工具: ${toolCall.function.name}`);
				const result = this.executeTool(toolCall);
				// 将工具执行结果写入上下文
				this.writeContext({ role: 'tool', content: result, tool_call_id: toolCall.id });
				// 收集研究详情
				this.collectDetail(toolCall, details);
			}
		}

		// 综合分析：LLM 基于所有工具调用结果生成结构化报告
		if (details.length > 0) {
			const report = this.synthesizeReport();
			this._history.push({ role: 'user', content: report });
			console.log(`[研究者] 已将研究报告写入历史（${details.length} 条工具调用记录）`);
		}

		return false;
	}

	/** 检查未读消息是否匹配研究关键词 */
	private matchKeywords(texts: string[]): boolean {
		return texts.some(text => this.researchKeywords.some(keyword => keyword.test(text)));
	}

	/** 执行工具调用（同步，直接调用 Go 绑定函数） */
	private executeTool(toolCall: ToolCallItem): string {
		const funcName = toolCall.function.name;
		let args: Record<string, any> = {};
		try {
			args = typeof toolCall.function.arguments === 'string'
				? JSON.parse(toolCall.function.arguments)
				: toolCall.function.arguments;
		}
		catch (parseError) {
			console.error(`[研究者] 工具调用参数解析失败:`, toolCall.function.arguments);
			return `工具调用参数解析失败，请确保传入合法的 JSON 字符串。错误: ${parseError}`;
		}
		switch (funcName) {
			case 'web_search': return this.handleWebSearch(args as WebSearchParams);
			case 'memory_query': return this.handleMemoryQuery(args as MemoryQueryParams);
			case 'process_links': return this.handleProcessLinks(args as ProcessLinksParams);
			default: return `未知工具: ${funcName}，可用工具为 web_search、memory_query 和 process_links`;
		}
	}

	/** 处理网络搜索工具调用（同步） */
	private handleWebSearch(args: WebSearchParams): string {
		try {
			const query = args.query || '';
			if (!query.trim()) return '搜索失败：查询关键词不能为空';
			const mode = args.mode || 'webpage';

			// 确保子系统已初始化
			if (!this.webSearchInitialized) {
				const initResult = this.initWebSearch();
				if (!initResult) return '搜索失败：网络检索子系统初始化失败';
			}

			console.log(`[研究者] 网络搜索: query="${query}", mode="${mode}"`);

			let result: string;
			let error: Error | null = null;

			switch (mode) {
				case 'assembly':
					// 大会辩论模式：需先注入记忆库提供者
					if (!this.assemblyMemoryInjected) {
						this.injectMemoryProvider();
					}
					[result, error] = webSearchAssembly(query.trim());
					break;
				case 'depth':
					[result, error] = webSearchDepth(query.trim());
					break;
				case 'webpage':
					[result, error] = webSearchWebpage(query.trim());
					break;
				case 'simple':
				default:
					[result, error] = webSearchSimple(query.trim());
					break;
			}

			if (error) {
				console.error(`[研究者] 网络搜索失败: ${error.message || String(error)}`);
				return `搜索失败：${error.message || String(error)}`;
			}

			const textResult = result || '未找到相关搜索结果';
			console.log(`[研究者] 搜索结果长度: ${textResult.length} 字符`);
			return textResult;
		}
		catch (error) {
			console.error('[研究者] 网络搜索处理异常:', error);
			return `网络搜索异常: ${error}`;
		}
	}

	/** 处理记忆库查询工具调用（同步） */
	private handleMemoryQuery(args: MemoryQueryParams): string {
		try {
			const query = args.query || '';
			if (!query.trim()) return '查询失败：查询文本不能为空';
			const topK = args.top_k || 10;

			// 确保记忆库已初始化
			if (!BaseConfig.memoryReady) {
				BaseConfig.initMemory();
				if (!BaseConfig.memoryReady) return '查询失败：记忆库未就绪';
			}

			console.log(`[研究者] 记忆库查询: query="${query}", topK=${topK}`);

			const [results, error] = memoryQuery('lunar_messages', query.trim(), topK);
			if (error) {
				console.error(`[研究者] 记忆库查询失败: ${error}`);
				return `记忆库查询失败：${error}`;
			}

			if (!results || results.length === 0) return '记忆库中未找到相关记录';

			const formattedResults = results.map((r, i) =>
				`[记录${i + 1}] 相似度:${(r.similarity * 100).toFixed(1)}% | 内容:${r.content}`
			).join('\n');

			console.log(`[研究者] 查询到 ${results.length} 条相关记录`);
			return formattedResults;
		}
		catch (error) {
			console.error('[研究者] 记忆库查询处理异常:', error);
			return `记忆库查询异常: ${error}`;
		}
	}

	/** 处理链接处理工具调用（同步） */
	private handleProcessLinks(args: ProcessLinksParams): string {
		try {
			const text = args.text || '';
			if (!text.trim()) return '处理失败：文本内容不能为空';

			// 确保子系统已初始化
			if (!this.webSearchInitialized) {
				const initResult = this.initWebSearch();
				if (!initResult) return '处理失败：网络检索子系统初始化失败';
			}

			console.log(`[研究者] 处理链接: 文本长度=${text.length}`);

			const [replacedText, descriptions, error] = webSearchProcessLinks(text);
			if (error) {
				console.error(`[研究者] 链接处理失败: ${error}`);
				return `链接处理失败：${error}`;
			}

			if (!descriptions || descriptions.length === 0) return '未检测到链接';

			const result = `替换后文本:\n${replacedText}\n\n链接详情:\n${descriptions.join('\n')}`;
			console.log(`[研究者] 处理了 ${descriptions.length} 个链接`);
			return result;
		}
		catch (error) {
			console.error('[研究者] 链接处理异常:', error);
			return `链接处理异常: ${error}`;
		}
	}

	/** 初始化网络检索子系统 */
	private initWebSearch(): boolean {
		if (this.webSearchInitialized) return true;
		try {
			const [success, err] = webSearchInit(
				OnlyData.systemUrl,
				OnlyData.SystemKey,
				OnlyData.MultimodalName,
				4096,
				0.7
			);
			if (err) {
				console.error('[研究者] 网络检索初始化失败:', err);
				return false;
			}

			// 设置下载回调：下载到 local_data/downloads/{会话ID}/
			try {
				const downloadDir = 'local_data/downloads';
				const groupID = new Date().toISOString().replace(/[:.]/g, '-').slice(0, 19);
				webSearchSetDownloadFunc(downloadDir, groupID);
				console.log(`[研究者] 下载目录已配置: ${downloadDir}/${groupID}`);
			}
			catch (dlErr) {
				console.warn('[研究者] 设置下载回调失败，链接处理中的下载链接将降级:', dlErr);
			}

			this.webSearchInitialized = true;
			console.log('[研究者] 网络检索子系统初始化成功');
			return true;
		}
		catch (e) {
			console.error('[研究者] 网络检索初始化异常:', e);
			return false;
		}
	}

	/** 为大会辩论模式注入记忆库提供者 */
	private injectMemoryProvider(): void {
		try {
			const [success, err] = webSearchSetMemoryProvider();
			if (err) {
				console.warn('[研究者] 注入记忆库提供者失败:', err);
				return;
			}
			this.assemblyMemoryInjected = true;
			console.log('[研究者] 已为大会辩论模式注入记忆库提供者');
		}
		catch (e) {
			console.warn('[研究者] 注入记忆库提供者异常:', e);
		}
	}

	/** 从工具调用中提取研究详情 */
	private collectDetail(toolCall: ToolCallItem, details: ResearchDetail[]): void {
		try {
			const args = typeof toolCall.function.arguments === 'string'
				? JSON.parse(toolCall.function.arguments)
				: toolCall.function.arguments;
			const query = args.query || args.text || '';
			const keyFindings = query ? `查询: ${query}` : '';
			if (toolCall.function.name === 'web_search') {
				details.push({
					toolName: 'web_search',
					query,
					mode: args.mode || 'webpage',
					keyFindings,
				});
			} else if (toolCall.function.name === 'memory_query') {
				details.push({
					toolName: 'memory_query',
					query,
					keyFindings,
				});
			} else if (toolCall.function.name === 'process_links') {
				details.push({
					toolName: 'process_links',
					query,
					keyFindings,
				});
			}
		} catch {
			// 解析失败时跳过，不阻断流程
		}
	}

	/**
	 * 综合分析：LLM 基于所有工具调用结果生成结构化报告
	 *
	 * 推理循环结束后，上下文中已包含所有工具调用及其结果。
	 * 再跑一次不带工具的推理，让 LLM 综合分析并输出 [研究报告] 格式。
	 */
	private synthesizeReport(): string {
		try {
			// 不带工具调用，让模型综合已有信息生成报告
			const response = this.run([], []);
			const content = response.body?.choices?.[0]?.message?.content || '';
			if (content.trim().length === 0) {
				console.warn('[研究者] 综合分析返回空结果');
				return '[研究报告] 研究过程中未能生成有效报告，请稍后重试。';
			}
			return content;
		}
		catch (error) {
			console.error('[研究者] 综合分析失败:', error);
			return '[研究报告] 研究报告生成失败，请稍后重试。';
		}
	}

	/** 提取未读消息文本 */
	private extractUnreadTexts(unreadContext: PostMessage[], count: number): string[] {
		const texts: string[] = [];
		for (const message of unreadContext.slice(-count)) {
			if (typeof message.content === 'string') texts.push(message.content);
			else message.content.forEach(item => { if (item.type === 'text') texts.push(item.text); });
		}
		return texts;
	}
}
