import { OnlyData, PostMessage } from '../index';

/** 学习者触发关键词 — 统一匹配所有研究/回忆意图 */
const learnerKeywords = [
	// 回忆偏向
	/回忆(?:一(?:下|回忆))?/, /想想?(?:看|起|到)/, /记不记得/,
	/还记得/, /以前(?:说过|聊过|讨论过|提过|提到)/,
	/上次(?:说|聊|讨论|提|提到)/,
	// 搜索偏向
	/搜索/, /搜(?:一搜|一下)/, /深入(?:了解|分析|研究)/,
	/详细(?:了解|分析|说明|解释)/, /分析(?:一(?:下|分析))?/,
	/(?:资料|文献|论文|报告|数据|统计)/, /调查(?:一(?:下|调查))?/,
	/核实/, /验证/,
	// 模糊偏向
	/查(?:一查|一下|询|找|找找|看看)/,
	/(?:帮我|给我|为我|替我)(?:查|搜索|找|调查|研究|检索|查询)/,
	/(?:真|假|正确|错误|靠谱|可靠)/,
];

/** 回忆偏向关键词 — 仅需查记忆库，无需网络搜索 */
const recallKeywords = [
	/回忆(?:一(?:下|回忆))?/, /想想?(?:看|起|到)/, /记不记得/,
	/还记得/, /以前(?:说过|聊过|讨论过|提过|提到)/,
	/上次(?:说|聊|讨论|提|提到)/,
];

/** 判定是否为回忆偏向意图 */
function isRecallIntent(texts: string[]): boolean {
	return texts.some(text => recallKeywords.some(kw => kw.test(text)));
}

/** 学习者是否已初始化 */
let learnerInitialized = false;

/** 初始化学习者智能体 */
function ensureLearnerInitialized(): boolean {
	if (learnerInitialized) return true;
	if (!learnerIsReady()) {
		const [success, err] = learnerInit(
			OnlyData.systemUrl,
			OnlyData.SystemKey,
			OnlyData.MultimodalName,
			4096,
			0.7,
			OnlyData.systemUrl,
			OnlyData.SystemKey,
			OnlyData.EmbeddingName
		);
		if (err) {
			console.error('[学习者] 初始化失败:', err);
			return false;
		}
	}
	learnerInitialized = true;
	console.log('[学习者] 初始化完成');
	return true;
}

/** 学习者角色（轻量级：正则匹配 + Goja 调用） */
export class LearnerRole {
	/** 消息历史（供主智能体消费） */
	public messages: PostMessage[] = [];

	/** 消费历史（主智能体调用后清空） */
	public consumeHistory(): PostMessage[] {
		const result = [...this.messages];
		this.messages = [];
		return result;
	}

	/**
	 * 执行学习研究
	 *
	 * @param unreadContext 当前未读上下文
	 * @returns true 表示未执行研究，false 表示已执行研究
	 */
	public executeLearner(unreadContext: PostMessage[]): boolean {
		// 提取未读消息文本
		const unreadTexts = this.extractTexts(unreadContext);

		// 检查是否匹配任意学习者关键词
		if (!unreadTexts.some(text => learnerKeywords.some(keyword => keyword.test(text)))) {
			return true; // 未匹配，不执行
		}

		// 确保学习者已初始化
		if (!ensureLearnerInitialized()) return true;

		// 判定运行模式：回忆模式仅查记忆库，搜索模式走完整流程
		const mode: 'recall' | 'full' = isRecallIntent(unreadTexts) ? 'recall' : 'full';

		// 调用 Go 层学习者（传入未读消息文本数组，不再传入对话历史）
		console.log('[学习者] 开始执行研究, 模式:', mode);
		const [report, error] = learnerExecute(unreadTexts, mode);

		if (error) {
			console.error('[学习者] 执行失败:', error);
			return true;
		}

		// 将研究报告写入历史
		if (report && report.trim().length > 0) {
			this.messages.push({ role: 'user', content: report });
			console.log('[学习者] 已将研究报告写入历史');
		}

		return false;
	}

	/** 提取消息文本 */
	private extractTexts(messages: PostMessage[]): string[] {
		const texts: string[] = [];
		for (const msg of messages) {
			if (typeof msg.content === 'string') {
				texts.push(msg.content);
			} else if (Array.isArray(msg.content)) {
				msg.content.forEach((item: any) => {
					if (item.type === 'text') texts.push(item.text);
				});
			}
		}
		return texts;
	}

	/**
	 * 导出学习者运行时上下文到文件（覆写模式）
	 *
	 * 同时导出 TS 层（消息历史、运行模式）和 Go 层（搜索结果、策略评估、记忆匹配）。
	 *
	 * @param dialogueMessages 对话历史消息
	 * @param unreadContext 当前未读上下文
	 * @param outputPath 输出文件路径（默认 agent_debug_学习者.json）
	 * @returns 导出文件路径，或空字符串表示失败
	 */
	public dumpContext(dialogueMessages: PostMessage[], unreadContext: PostMessage[], outputPath?: string): string {
		const path = outputPath || 'agent_debug_学习者.json';

		// TS 层快照
		const timestamp = new Date().toLocaleString('zh-CN', {
			year: 'numeric', month: '2-digit', day: '2-digit',
			hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false
		});

		const unreadTexts = this.extractTexts(unreadContext);
		const mode = isRecallIntent(unreadTexts) ? 'recall' : 'full';

		const snapshot = {
			timestamp,
			role: '学习者',
			mode,
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
			learnerInitialized,
		};

		const [, error] = saveDebugFile(path, JSON.stringify(snapshot, null, 2));
		if (error) {
			console.error('[学习者] 导出 TS 层上下文失败:', error);
			return '';
		}

		// Go 层快照（搜索结果、策略评估、记忆匹配等）
		if (learnerInitialized) {
			const goPath = path.replace('.json', '_go.json');
			const [, goError] = learnerDumpContext(unreadTexts, mode, goPath);
			if (goError) {
				console.error('[学习者] 导出 Go 层上下文失败:', goError);
			}
		}

		console.log('[学习者] 上下文快照已导出:', path);
		return path;
	}
}