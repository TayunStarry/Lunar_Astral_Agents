import * as EntryAPI from '../EntryAPI/code';

/**
 * 处理流式响应数据
 *
 * @param {string} chunk - 解码后的流式数据块
 *
 * @param {EntryAPI.HistoryMessage} messageObject - 消息对象
 *
 * @param {HTMLElement} contentElement - 内容元素
 *
 * @param {EntryAPI.StreamProcessingState} state - 处理状态对象
 */
function processStreamingData(chunk: string, messageObject: EntryAPI.HistoryMessage, contentElement: HTMLElement, state: EntryAPI.StreamProcessingState) {
	// 遍历每一行数据
	for (const line of chunk.split("\n").filter((line) => line.trim() !== "")) {
		// 检查数据块是否包含有效数据
		if (!line.startsWith("data: ")) continue;
		/** 移除数据块前缀 */
		const data = line.replace("data: ", "");
		// 如果收到结束信号，退出循环
		if (data === "[DONE]") break;
		/** 解析JSON数据 */
		const jsonData = JSON.parse(data);
		// 检查是否有工具调用
		if (jsonData.choices?.[0]?.delta?.tool_calls) {
			processToolCallData(jsonData.choices[0].delta.tool_calls, state);
			return;
		}
		// 检查是否有结束的工具调用
		if (jsonData.choices?.[0]?.finish_reason === "tool_calls") {
			finalizeCurrentToolCall(state);
			return;
		}
		// 检查是否有预测令牌数
		if (jsonData.timings?.predicted_per_second && EntryAPI.OnlyData.isDebugMode) {
			updateTokenSpeed(jsonData.timings.predicted_per_second);
		}
		// 处理内容数据
		processContentData(jsonData, messageObject, contentElement, state);
	}
	// 添加代码高亮
	contentElement.querySelectorAll('pre code').forEach(block => (window as any).hljs.highlightElement(block));
}

/**
 * 处理工具调用数据
 *
 * @param {any[]} toolCallChunks - 工具调用数据块
 *
 * @param {EntryAPI.StreamProcessingState} state - 处理状态对象
 */
function processToolCallData(toolCallChunks: any[], state: EntryAPI.StreamProcessingState) {
	// 遍历工具调用数据块
	for (const chunk of toolCallChunks) {
		// 如果是一个新的工具调用
		if (chunk.index !== state.currentToolCallIndex) {
			// 如果之前有正在收集的工具调用，先保存它
			if (state.currentToolCall !== null) finalizeCurrentToolCall(state);
			// 开始新的工具调用
			state.currentToolCallIndex = chunk.index;
			state.currentToolCall = { type: "function", function: { name: "", arguments: "" } };
			state.currentFunctionArgs = "";
			state.currentFunctionName = "";
		}
		// 累积工具调用的名称和参数
		if (chunk.function?.name) {
			state.currentFunctionName += chunk.function.name;
		}
		if (chunk.function?.arguments) {
			state.currentFunctionArgs += chunk.function.arguments;
		}
		// 更新当前工具调用
		if (state.currentToolCall) {
			state.currentToolCall.function.name = state.currentFunctionName;
			state.currentToolCall.function.arguments = state.currentFunctionArgs;
		}
	}
}

/**
 * 完成当前工具调用
 *
 * @param {EntryAPI.StreamProcessingState} state - 处理状态对象
 */
function finalizeCurrentToolCall(state: EntryAPI.StreamProcessingState) {
	// 如果当前有正在收集的工具调用
	if (state.currentToolCall !== null) {
		try {
			state.currentToolCall.function.arguments = JSON.parse(state.currentFunctionArgs);
		}
		catch {
			// 如果解析失败，传入原始字符串
			state.currentToolCall.function.arguments = state.currentFunctionArgs;
		}
		state.toolCalls.push(state.currentToolCall);
		// 重置变量
		state.currentToolCall = null;
		state.currentToolCallIndex = -1;
		state.currentFunctionArgs = "";
		state.currentFunctionName = "";
	}
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
 * 处理内容数据
 *
 * @param {any} jsonData - JSON数据
 *
 * @param {EntryAPI.HistoryMessage} messageObject - 消息对象
 *
 * @param {HTMLElement} contentElement - 内容元素
 *
 * @param {EntryAPI.StreamProcessingState} state - 处理状态对象
 */
function processContentData(jsonData: any, messageObject: EntryAPI.HistoryMessage, contentElement: HTMLElement, state: EntryAPI.StreamProcessingState) {
	/** 提取内容数据 */
	const contentData = jsonData.choices?.[0]?.delta?.content;
	/** 提取推理内容数据 */
	const reasoningContentData = jsonData.choices?.[0]?.delta?.reasoning_content;
	// 累积描述内容
	if (contentData) state.descriptionContent += contentData;
	// 遍历所有模式，尝试匹配思考标签
	for (const pattern of EntryAPI.ThinkType) {
		/** 匹配思考标签 */
		const match = state.descriptionContent.match(pattern) as RegExpMatchArray | null;
		// 如果匹配成功，提取思考内容
		if (!match) continue;
		// 更新预备的思考内容
		state.thinkingContent += match[1].trim();
		// 更新预备的描述内容
		state.descriptionContent = match[2].trim();
		// 匹配结束, 跳出循环
		break;
	}
	// 提取独立的推理内容
	if (reasoningContentData) state.reasoningContent += reasoningContentData;
	// 更新消息内容
	updateMessageContent(messageObject, contentElement, state);
}

/**
 * 更新消息内容
 *
 * @param {EntryAPI.HistoryMessage} messageObject - 消息对象
 *
 * @param {HTMLElement} contentElement - 内容元素
 *
 */
function updateMessageContent(messageObject: EntryAPI.HistoryMessage, contentElement: HTMLElement, state: EntryAPI.StreamProcessingState) {
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
 * @returns {Promise<{ textContent: string }>}
 */
export async function sendRequestWithTools(messages: EntryAPI.PostMessage[], container: HTMLElement, messageObject: EntryAPI.HistoryMessage, contentElement: HTMLElement, state: EntryAPI.StreamProcessingState): Promise<{ textContent: string; }> {
	/** 向处理器模型发送请求并等待响应 */
	const response = await new EntryAPI.MultimodalRequest(messages, true, true).response;
	// 如果未能获得期望中的响应，则抛出错误
	if (!response.ok || !response.body) throw new Error(`API返回错误: ${response.status} ${response.statusText}`);
	/** 获取响应流的读取器 */
	const reader = response.body.getReader();
	/** 创建文本解码器 */
	const decoder = new TextDecoder();
	// 循环处理流式返回的文本块
	while (true) {
		// 自动处理滚动行为
		EntryAPI.autoScrollToBottom(container, {});
		// 读取数据块
		const { value, done } = await reader.read();
		// 如果传输完成，跳出循环
		if (done) break;
		/** 解码数据块 */
		const chunk = decoder.decode(value);
		// 处理流式数据
		processStreamingData(chunk, messageObject, contentElement, state);
	}
	// 确保最后一个工具调用被保存
	finalizeCurrentToolCall(state);
	// 如果有工具调用，处理它们并重新发送请求
	if (state.toolCalls.length > 0) {
		/** 处理工具调用 */
		const hasProcessedToolCalls = await EntryAPI.handleToolCalls(state, messages, contentElement, messageObject);
		// 如果有处理过的工具调用，重新发送请求（包含工具调用结果）
		if (hasProcessedToolCalls) return await sendRequestWithTools(messages, container, messageObject, contentElement, state);
	}
	// 如果没有工具调用或工具调用处理完成，继续正常流程
	return { textContent: messageObject.content };
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