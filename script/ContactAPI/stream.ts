import * as EntryAPI from '../EntryAPI/code';

/**
 * 更新消息内容
 *
 * @param {EntryAPI.HistoryMessage} messageObject - 消息对象
 *
 * @param {HTMLElement} contentElement - 内容元素
 *
 */
export function updateMessageContent(messageObject: EntryAPI.HistoryMessage, contentElement: HTMLElement, state: EntryAPI.ChatCache): string {
	// 检查推理内容是否为空
	if (state.reasoningContent.trim() !== "" || state.thinkingContent.trim() !== "") {
		/** 新的思考标签内容 */
		const newThinkTag = '<think>\n' + state.reasoningContent + state.thinkingContent + '\n</think>';
		// 修正复合描述内容
		messageObject.content = newThinkTag + state.descriptionContent;
	}
	// 修正简单描述内容
	else messageObject.content = state.descriptionContent;
	// 检查消息内容是否为空
	if (messageObject.content.trim() === "") return;
	// 在单独的波浪线字符（即前后没有波浪线或空白字符）两侧添加空格，以确保格式一致性
	messageObject.content = messageObject.content.replace(/(?<![~\s])~(?![~\s])/g, ' ~ ');
	// 处理内容更新，对内容中的思考标签进行处理
	contentElement.innerHTML = EntryAPI.processThinkTags(messageObject.content);
	return messageObject.content;
}

/**
 * 更新令牌速度显示
 *
 * @param {number} predictedPerSecond - 每秒预测令牌数
 */
function updateTokenSpeed(predictedPerSecond: number) {
	// 显示令牌速度显示
	EntryAPI.tokenCounterPanel.style.display = "block";
	// 更新令牌速度显示
	EntryAPI.tokenCounterPanel.innerHTML = `${predictedPerSecond?.toFixed(2) || "N/A"} token/s`;
}

/**
 * 发送请求并处理工具调用
 *
 * @param {EntryAPI.PostMessage[]} messages - 消息数组
 *
 * @param {HTMLElement} container - 消息容器
 *
 * @param {EntryAPI.HistoryMessage} messageObject - 消息对象
 *
 * @param {HTMLElement} contentElement - 内容元素
 *
 * @param {EntryAPI.ChatCache} cache - 流处理状态缓存
 */
export async function sendRequestWithTools(messages: EntryAPI.PostMessage[], container: HTMLElement, messageObject: EntryAPI.HistoryMessage, contentElement: HTMLElement, cache: EntryAPI.ChatCache) {
	/** 向处理器模型发送请求并等待响应（禁用流式响应） */
	const response = await new EntryAPI.MultimodalRequest(messages, true, false).response;
	// 如果未能获得期望中的响应，则抛出错误
	if (!response.ok) throw new Error(`API返回错误: ${response.status} ${response.statusText}`);
	/** 解析响应为JSON */
	const jsonData = await response.json();
	// 处理推理内容数据
	if (jsonData.choices?.[0]?.message?.reasoning_content && EntryAPI.OnlyData.isDebugMode) {
		cache.reasoningContent = jsonData.choices[0].message.reasoning_content;
	}
	// 检查是否有预测令牌数
	if (jsonData.timings?.predicted_per_second && EntryAPI.OnlyData.isDebugMode) {
		updateTokenSpeed(jsonData.timings.predicted_per_second);
	}
	// 处理工具调用
	if (jsonData.choices?.[0]?.message?.tool_calls) {
		for (const toolCall of jsonData.choices[0].message.tool_calls) {
			// 解析arguments字段
			toolCall.function.arguments = JSON.parse(toolCall.function.arguments);
			// 记录工具调用
			cache.toolCalls.push(toolCall);
		}
	}
	// 处理内容数据
	if (jsonData.choices?.[0]?.message?.content) {
		cache.descriptionContent = jsonData.choices[0].message.content;
	}
	// 如果有工具调用，处理它们并重新发送请求
	if (cache.toolCalls.length > 0) {
		/** 处理工具调用 */
		const hasProcessedToolCalls = await EntryAPI.handleToolCalls(cache, messages, contentElement, messageObject);
		// 如果有处理过的工具调用，重新发送请求（包含工具调用结果）
		if (hasProcessedToolCalls) return await sendRequestWithTools(messages, container, messageObject, contentElement, cache);
	}
}

/**
 * 清理资源
 *
 * @param {HTMLElement | null} contentElement - 内容元素
 *
 * @param {EntryAPI.HistoryMessage | undefined} messageObject - 消息对象
 *
 * @param {HTMLElement | null} messageElement - 消息元素
 */
export async function cleanupResources(contentElement: HTMLElement | null, messageObject?: EntryAPI.HistoryMessage, messageElement?: HTMLElement | null) {
	if (contentElement) {
		// 为think区块添加折叠功能
		(contentElement.querySelectorAll(".toggle_think_button") as NodeListOf<HTMLButtonElement>).forEach(EntryAPI.bindFoldingButton);
	}
	if (messageObject) {
		// 生成嵌入向量
		messageObject.embedVector = await new EntryAPI.EmbeddingRequest(messageObject.content, false, false).output();
	}
	// 移除停止生成按钮
	messageElement?.querySelector('.stop_generation_button')?.remove();
	// 重新启用输入按钮，允许用户继续发送消息
	EntryAPI.disabledReleaseButton(false);
	// 清理中止控制器
	EntryAPI.OnlyData.abortController = null;
}

/**
 * 聊天缓存信息
 *
 * 用于缓存聊天过程中的状态，包括当前工具调用、当前工具调用索引、当前函数参数、当前函数名称、思考内容、描述内容、推理内容和工具调用数组。
 */
export class CacheRocessing implements EntryAPI.ChatCache {
	public currentToolCall: EntryAPI.ToolCall;
	public currentToolCallIndex: number = -1;
	public currentFunctionArgs: string;
	public currentFunctionName: string;
	public thinkingContent: string = "";
	public descriptionContent: string = "";
	public reasoningContent: string = "";
	public toolCalls: EntryAPI.ToolCall[] = [];
	constructor() { }
}