import { PostMessage } from '../../index';

/** 搜索者是否已初始化 */
let searchInitialized = false;

/** 初始化搜索者智能体 */
function ensureSearcherInitialized(): boolean {
	if (searchInitialized) return true;
	if (!searchIsReady()) {
		const [success, err] = searchInit();
		if (err) {
			console.error('[搜索者] 初始化失败:', err);
			return false;
		}
	}
	searchInitialized = true;
	console.log('[搜索者] 初始化完成');
	return true;
}

/**
 * 搜索者角色（工具调用模式）
 *
 * 通过对话者的 dispatch_searcher 工具调用触发，
 * 执行网络搜索或记忆库查询，返回结构化报告。
 */
export class SearcherRole {
	/** 消息历史（供调试导出） */
	public messages: PostMessage[] = [];

		/**
		 * 执行搜索研究任务
		 *
		 * @param taskDescription 对话者通过工具调用传来的研究需求描述
		 * @returns 搜索报告文本（markdown 格式）
		 */
		public async createCreativeWork(taskDescription: string): Promise<string> {
			if (!taskDescription || taskDescription.trim().length === 0) {
				return '研究任务调度失败：任务描述不能为空，请提供具体的搜索研究需求';
			}

			// 确保搜索者已初始化
			if (!ensureSearcherInitialized()) {
				return '研究任务调度失败：搜索者子智能体未就绪，请稍后重试';
			}

			console.log('[搜索者] 开始执行研究:', taskDescription);
			const [report, error] = searchExecute(taskDescription.trim());

			if (error) {
				console.error('[搜索者] 执行失败:', error);
				return `研究任务执行失败：${error}`;
			}

			if (report && report.trim().length > 0) {
				// 记录到自身历史（供调试导出）
				this.messages.push({ role: 'assistant', content: report });
				console.log('[搜索者] 研究完成，报告已生成');
				return report;
			}

			return '研究任务完成，但未找到相关信息。';
		}

		/**
		 * 导出搜索者运行时上下文到文件（覆写模式）
		 *
		 * @param dialogueMessages 对话历史消息
		 * @param unreadContext 当前未读上下文
		 * @param outputPath 输出文件路径（默认 agent_debug_搜索者.json）
		 * @returns 导出文件路径，或空字符串表示失败
		 */
		public dumpContext(dialogueMessages: PostMessage[], unreadContext: PostMessage[], outputPath?: string): string {
			const path = outputPath || 'agent_debug_搜索者.json';

			const timestamp = new Date().toLocaleString('zh-CN', {
				year: 'numeric', month: '2-digit', day: '2-digit',
				hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false
			});

			const snapshot = {
				timestamp,
				role: '搜索者',
			ownMessagesCount: this.messages.length,
			ownMessages: this.messages.map((msg, idx) => {
				const content = typeof msg.content === 'string'
					? msg.content
					: JSON.stringify(msg.content);
				return {
					index: idx,
					role: msg.role,
					contentPreview: content.length > 500 ? content.slice(0, 500) + '...' : content,
					contentLength: content.length,
				};
			}),
			dialogueMessagesCount: dialogueMessages.length,
			dialogueMessages: dialogueMessages.slice(-15).map((msg, idx) => {
				const content = typeof msg.content === 'string'
					? msg.content
					: JSON.stringify(msg.content);
				return {
					index: idx,
					role: msg.role,
					contentPreview: content.length > 300 ? content.slice(0, 300) + '...' : content,
				};
			}),
			unreadContextCount: unreadContext.length,
			unreadContext: unreadContext.slice(-10).map((msg, idx) => {
				const content = typeof msg.content === 'string'
					? msg.content
					: JSON.stringify(msg.content);
				return {
					index: idx,
					role: msg.role,
					contentPreview: content.length > 300 ? content.slice(0, 300) + '...' : content,
				};
			}),
			searchInitialized,
		};

		const [, error] = saveDebugFile(path, JSON.stringify(snapshot, null, 2));
		if (error) {
			console.error('[搜索者] 导出 TS 层上下文失败:', error);
			return '';
		}

		// Go 层快照
		const goPath = path.replace('.json', '_go.json');
		const [, goError] = searchDumpContext('', goPath);
		if (goError) {
			console.error('[搜索者] 导出 Go 层上下文失败:', goError);
		}

		console.log('[搜索者] 上下文快照已导出:', path);
		return path;
	}
}
