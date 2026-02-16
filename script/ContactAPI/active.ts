import * as EntryAPI from '../EntryAPI/code';

/**
 * 主动消息约束执行器，用于限制主动消息的频率。
 *
 * 每个30分钟内最多允许3次主动消息，超过次数则执行禁止回调。
 */
export const controlActiveMessage = new EntryAPI.ConstraintExecution(30, 3, allowActiveMessage, disableActiveMessage);

/**
 * 连续记忆约束执行器，用于限制连续记忆的频率。
 *
 * 每个5分钟内最多允许1次连续记忆，超过次数则执行禁止回调。
 */
export const controlContinuousMemory = new EntryAPI.ConstraintExecution(5, 1, allowContinuousMemory);

/**
 * 允许连续记忆
 *
 * 当连续记忆约束执行器允许执行时调用，负责将当前聊天记录缓存到knowledge/continuous_memory.json文件中。
 */
async function allowContinuousMemory() {
	await EntryAPI.batchProcessingKnowledgeWrite('knowledge/continuous_memory.json', EntryAPI.OnlyData.historyMessage);
	EntryAPI.showSystemMessage("聊天记录已缓存", 'success');
}
/**
 * 主动消息允许执行回调函数
 *
 * 当主动消息约束执行器允许执行时调用，负责获取主动消息的Markdown内容并渲染到聊天记录中。
 */
async function allowActiveMessage() {
	/**
	 * 获取主动消息的Markdown内容
	 */
	const markdown = await EntryAPI.fetchMarkdown('/read/resources/prompts/activeMessage.md');
	// 若调试模式开启，则渲染< 动态提示词 >
	if (EntryAPI.OnlyData.isDebugMode) {
		/**
		 * 渲染< 动态提示词 >
		 */
		const messageElement = await EntryAPI.tracelessRenderMessage('<think>\n' + markdown + '\n</think>', EntryAPI.chatHistoryPanel);
		// 为think区块添加折叠功能
		(messageElement?.querySelectorAll(".toggle_think_button") as NodeListOf<HTMLButtonElement>).forEach(EntryAPI.bindFoldingButton);
	};
	// 从API加载对话内容
	await EntryAPI.executeDialogueAndParse(EntryAPI.chatHistoryPanel, markdown);
	// 设置超时状态为用户输入状态
	EntryAPI.setStateWithTimeout(EntryAPI.EmotionalState.AWAIT);
}

/**
 * 主动消息禁止执行回调函数
 *
 * 当主动消息约束执行器禁止执行时调用，负责显示系统提示消息。
 */
async function disableActiveMessage() {
	// 当执行受限时，显示系统提示消息
	EntryAPI.showSystemMessage("你是不是没空搭理月华呀? 那我就在旁边乖乖等你啦", "error");
}