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

 * @param {EntryAPI.ChatCache} cache - 流处理状态缓存

 * @param {boolean} [streaming=false] - 是否使用流式响应

 * @returns {Promise<void>} - 无返回值
 */
export async function sendRequestWithTools(messages: EntryAPI.PostMessage[], container: HTMLElement, messageObject: EntryAPI.HistoryMessage, contentElement: HTMLElement, cache: EntryAPI.ChatCache, streaming: boolean = false): Promise<void> {
	try {
		/** 向处理器模型发送请求并等待响应 */
		const response = await new EntryAPI.MultimodalRequest(messages, true, streaming).response;
		// 如果未能获得期望中的响应，则抛出错误
		if (!response.ok) throw new Error(`API返回错误: ${response.status} ${response.statusText}`);
		// 默认按照流式响应进行处理
		await processStreamingResponse(response, messageObject, contentElement, cache);
		// 如果有工具调用，处理它们并重新发送请求
		if (cache.toolCalls.length > 0) {
			/** 处理工具调用 */
			const hasProcessedToolCalls = await EntryAPI.handleToolCalls(cache, messages, contentElement, messageObject);
			// 如果有处理过的工具调用，重新发送请求（包含工具调用结果）
			if (hasProcessedToolCalls) return await sendRequestWithTools(messages, container, messageObject, contentElement, cache, streaming);
		}
	}
	catch (error) {
		console.error('请求处理错误:', error);
		// 显示错误信息给用户
		messageObject.content = `错误: ${error instanceof Error ? error.message : String(error)}`;
		updateMessageContent(messageObject, contentElement, cache);
		// 清理资源
		await cleanupResources(contentElement, messageObject);
	}
}

/**
 * 处理工具调用
 */
function processToolCalls(toolCalls: any[], cache: EntryAPI.ChatCache): void {
	for (const toolCall of toolCalls) {
		try {
			toolCall.function.arguments = JSON.parse(toolCall.function.arguments);
			cache.toolCalls.push(toolCall);
		}
		catch (parseError) {
			console.error('工具调用参数解析错误:', parseError);
		}
	}
}

/**
 * 处理单条流数据行
 */
async function processStreamLine(line: string, cache: EntryAPI.ChatCache, messageObject: EntryAPI.HistoryMessage, contentElement: HTMLElement): Promise<boolean> {
	if (line.trim() === '' || line.trim() === 'data: [DONE]') return false;

	if (!line.startsWith('data: ')) {
		await processNonStreamingResponse(line, cache);
		return true;
	}

	try {
		const analysisData = JSON.parse(line.substring(6));
		const choice = analysisData.choices?.[0];
		if (!choice) return false;

		if (analysisData.timings?.predicted_per_second && EntryAPI.OnlyData.isDebugMode) {
			updateTokenSpeed(analysisData.timings.predicted_per_second);
		}

		if (choice.message?.reasoning_content && EntryAPI.OnlyData.isDebugMode) {
			cache.reasoningContent = choice.message.reasoning_content;
		}

		if (choice.message?.tool_calls) {
			processToolCalls(choice.message.tool_calls, cache);
		}

		if (choice.delta?.content) {
			cache.descriptionContent += choice.delta.content;
			updateMessageContent(messageObject, contentElement, cache);
		}

		return !!choice.finish_reason;
	} catch (parseError) {
		console.error('JSON解析错误:', parseError);
		return false;
	}
}

/**
 * 读取流数据
 */
async function readStreamData(reader: ReadableStreamDefaultReader<Uint8Array>, decoder: TextDecoder, cache: EntryAPI.ChatCache, messageObject: EntryAPI.HistoryMessage, contentElement: HTMLElement): Promise<void> {
	while (true) {
		if (EntryAPI.OnlyData.abortController?.signal.aborted) return;

		const { done, value } = await reader.read();
		if (done) break;

		const chunk = decoder.decode(value, { stream: true });
		const lines = chunk.split('\n');

		for (const line of lines) {
			const shouldBreak = await processStreamLine(line, cache, messageObject, contentElement);
			if (shouldBreak) break;
		}
	}
}

/**
 * 处理流错误
 */
async function handleStreamError(error: any, messageObject: EntryAPI.HistoryMessage, contentElement: HTMLElement, cache: EntryAPI.ChatCache): Promise<void> {
	console.error('流式响应处理错误:', error);
	messageObject.content = `错误: ${error instanceof Error ? error.message : String(error)}`;
	updateMessageContent(messageObject, contentElement, cache);
	await cleanupResources(contentElement, messageObject);
}

/**
 * 释放读取器锁
 */
function releaseReaderLock(reader: ReadableStreamDefaultReader<Uint8Array>): void {
	try {
		reader.releaseLock();
	} catch (releaseError) {
		console.error('释放读取器锁失败:', releaseError);
	}
}

/**
 * 处理流式响应
 *
 * @param {Response} response - API响应对象
 *
 * @param {EntryAPI.HistoryMessage} messageObject - 消息对象
 *
 * @param {HTMLElement} contentElement - 内容元素
 *
 * @param {EntryAPI.ChatCache} cache - 流处理状态缓存
 *
 * @returns {Promise<void>} - 无返回值
 */
export async function processStreamingResponse(response: Response, messageObject: EntryAPI.HistoryMessage, contentElement: HTMLElement, cache: EntryAPI.ChatCache): Promise<void> {
	/** 获取响应体读取器 */
	const reader = response.body.getReader();
	/** 创建文本解码器 */
	const decoder = new TextDecoder();
	// 读取流数据
	try {
		await readStreamData(reader, decoder, cache, messageObject, contentElement);
	}
	// 处理流错误
	catch (error) {
		await handleStreamError(error, messageObject, contentElement, cache);
		//throw error;
	}
	// 释放读取器锁
	finally {
		releaseReaderLock(reader);
	}
}

/**
 * 处理非流式响应
 *
 * @param {string} message - 非流式响应消息
 *
 * @param {EntryAPI.ChatCache} cache - 流处理状态缓存
 *
 * @returns {Promise<void>} - 无返回值
 */
export async function processNonStreamingResponse(message: string, cache: EntryAPI.ChatCache): Promise<void> {
	try {
		/** 解析响应为JSON */
		const jsonData = JSON.parse(message);
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
				try {
					// 解析arguments字段
					toolCall.function.arguments = JSON.parse(toolCall.function.arguments);
					// 记录工具调用
					cache.toolCalls.push(toolCall);
				} catch (parseError) {
					console.error('工具调用参数解析错误:', parseError);
				}
			}
		}
		// 处理内容数据
		if (jsonData.choices?.[0]?.message?.content) {
			cache.descriptionContent = jsonData.choices[0].message.content;
		}
	}
	catch (error) {
		console.error('非流式响应处理错误:', error);
		//throw error;
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
 *
 * @returns {Promise<void>} - 无返回值
 */
export async function cleanupResources(contentElement: HTMLElement | null, messageObject?: EntryAPI.HistoryMessage, messageElement?: HTMLElement | null): Promise<void> {
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
export class CacheProcessing implements EntryAPI.ChatCache {
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