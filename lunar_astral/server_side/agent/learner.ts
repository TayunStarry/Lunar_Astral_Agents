import { OnlyData, PostMessage } from '../index';

/** 学习者正则关键词 — 仅在用户明确意图为回忆或搜索时触发 */
const learnerKeywords = [
	/查(?:一查|一下|询|找|找找|看看)/,
	/搜索/,
	/搜(?:一搜|一下)/,
	/(?:帮我|给我|为我|替我)(?:查|搜索|找|调查|研究|检索|查询)/,
	/研究(?:一(?:下|研究))/,
	/调查(?:一(?:下|调查))?/,
	/回忆(?:一(?:下|回忆))?/,
	/想想?(?:看|起|到)/,
	/记不记得/,
	/还记得/,
	/以前(?:说过|聊过|讨论过|提过|提到)/,
	/上次(?:说|聊|讨论|提|提到)/,
	/深入(?:了解|分析|研究)/,
	/详细(?:了解|分析|说明|解释)/,
	/分析(?:一(?:下|分析))?/,
	/核实/,
	/验证/,
	/(?:真|假|正确|错误|靠谱|可靠)/,
	/(?:资料|文献|论文|报告|数据|统计)/
];

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
	 * @param dialogueMessages 对话历史消息
	 * @param unreadContext 当前未读上下文
	 * @returns true 表示未执行研究，false 表示已执行研究
	 */
	public executeLearner(dialogueMessages: PostMessage[], unreadContext: PostMessage[]): boolean {
		// 提取未读消息文本
		const unreadTexts = this.extractTexts(unreadContext);

		// 检查是否匹配学习者关键词
		if (!this.matchKeywords(unreadTexts)) return true;

		// 确保学习者已初始化
		if (!ensureLearnerInitialized()) return true;

		// 序列化消息为 JSON
		const dialogueJSON = JSON.stringify(dialogueMessages.slice(-15));
		const unreadJSON = JSON.stringify(unreadContext.slice(-10));

		// 调用 Go 层学习者
		console.log('[学习者] 开始执行研究...');
		const [report, error] = learnerExecute(dialogueJSON, unreadJSON);

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

	/** 检查是否匹配关键词 */
	private matchKeywords(texts: string[]): boolean {
		return texts.some(text => learnerKeywords.some(keyword => keyword.test(text)));
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
}
