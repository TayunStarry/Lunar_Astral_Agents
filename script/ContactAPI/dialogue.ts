import * as EntryAPI from '../EntryAPI/code';
/** 消息元素关联对象 */
interface MessageElement {
    /** 历史消息对象 */
    messageObject: EntryAPI.HistoryMessage;
    /** 消息元素的 DOM 节点，无则为 null */
    messageElement: HTMLElement | null;
    /** 内容元素的 DOM 节点，无则为 null */
    contentElement: HTMLElement | null;
};

/** 最近保留的消息数量（不包括最后一条用户消息） */
const keepRecentCount = 8;

/** 最大上下文消息数 */
const maxContextMessages = 24;

/**
 * 构建上下文消息数组，包含排序后的早期消息、最近消息和最后一条用户消息
 *
 * @param {HTMLElement|undefined} contentElement - 消息元素，可选参数
 *
 * @returns {Promise<EntryAPI.PostMessage[]>} - 包含排序后的上下文消息数组的 Promise
 */
async function buildContextMessages(contentElement?: HTMLElement): Promise<EntryAPI.PostMessage[]> {
	// 使用 structuredClone 进行深拷贝，避免后续操作污染原始数组
	const availableHistory = structuredClone(EntryAPI.OnlyData.historyMessage);
	/** 无需进行排序的最近消息（保持原序） */
	const recentMemories = availableHistory.slice(-keepRecentCount);
	/** 从对话历史中倒序查找最近一条用户发出的消息 */
	const lastUserMessage = availableHistory.slice().reverse().find(msg => msg.role === "user");
	// 如果没有找到用户消息，返回默认历史
	if (!lastUserMessage) return await getDefaultHistory(maxContextMessages);
	/** 当前用户消息的嵌入向量 */
	const currentEmbedResponse = lastUserMessage.embedVector;
	// 如果当前用户消息没有嵌入向量，返回默认历史
	if (!currentEmbedResponse || currentEmbedResponse.length === 0) return await getDefaultHistory(maxContextMessages, contentElement);
	/** 解析知识库查询结果 */
	const knowledgeResponseResult = await EntryAPI.captureKnowledgeRanking("knowledge/lunar_notes.json", currentEmbedResponse);
	/** 排序后的远期记忆 */
	const remoteMemory = EntryAPI.knowledgeRanking(availableHistory.slice(0, -keepRecentCount), currentEmbedResponse, keepRecentCount);
	/** 最终的消息数组（知识库消息 + 远期记忆（相关性排序）+ 最近消息（保持原序）） */
	const finalMessages = [...knowledgeResponseResult, ...remoteMemory, ...recentMemories];
	/** 去重后的最终消息数组 */
	const finalMessage = EntryAPI.uniqueFinalMessages(finalMessages);
	/** 将去重后的消息数组转换为 PostMessage 格式数组 */
	const messages = (await convertToPostMessageFormat(finalMessage, contentElement)).filter(msg => msg.content);
	// 如果是调试模式，添加调试信息
	if (EntryAPI.OnlyData.isDebugMode) {
		await renderDebugInfo(messages, remoteMemory.length, recentMemories.length);
	}
	// 返回转换后的消息数组
	return messages;
}

/**
 * 将内部消息格式转换为 PostMessage 格式
 *
 * @param {EntryAPI.MixedMessage[]} messages - 内部消息格式数组
 *
 * @returns {Promise<EntryAPI.PostMessage[]>} - 转换后的消息数组
 */
export async function convertToPostMessageFormat(messages: EntryAPI.MixedMessage[], contentElement?: HTMLElement): Promise<EntryAPI.PostMessage[]> {
	/**
	 * 将图片 URL 转换为 Base64 编码
	 *
	 * @param {string} url - 图片 URL
	 * @returns {Promise<string>} - Base64 编码的图片数据
	 */
	async function convertUrlToBase64(url: string): Promise<string> {
		/** 从URL获取图片文件 */
		const response = await fetch(url);
		/** 从响应中获取图片 Blob 对象 */
		const blob = await response.blob();
		// 将图片 Blob 对象转换为 Base64 编码字符串
		return EntryAPI.FileToBase64(blob);
	}
	/**
	 * 转换图片URL为完整格式
	 *
	 * @param {string} imageUrl - 图片URL
	 * @returns {string} - 转换后的图片URL
	 */
	function transformImageUrl(imageUrl: string): string {
		if (imageUrl.startsWith("data:image")) return imageUrl;
		if (imageUrl.startsWith("http")) return imageUrl;
		return EntryAPI.OnlyData.fileServiceUrl + imageUrl;
	};
	/** 处理后的消息数组 */
	const processedMessages: EntryAPI.PostMessage[] = [];
	// 遍历原始消息数组
	for (const { role, content: text, imageUrl } of messages) {
		// 无图消息直接添加
		if (!imageUrl) {
			processedMessages.push({ role, content: text });
			continue;
		}
		// 检查是否为支持的视频文件格式
		if (EntryAPI.OnlyData.videoFormatsExtensions.some(format => imageUrl.toLowerCase().endsWith(format))) {
			// 显示视频解读提示
			contentElement.innerHTML = '<em><strong>正在认真观看视频中, 请耐心等待月华看完哦......</strong></em>'
			// 处理视频文件
			await EntryAPI.processVideoFile(transformImageUrl(imageUrl), text, role, processedMessages);
		}
		// 处理普通图片
		else {
			/** 转换图片URL为完整格式 */
			let url = transformImageUrl(imageUrl);
			/** 检查当前URL是否为localhost格式 */
			const isLocalhost = EntryAPI.OnlyData.MultimodalUrl.startsWith("/v1");
			/** 非localhost环境下，确保图片URL是base64格式 */
			if (!isLocalhost && !url.startsWith("data:image")) url = await convertUrlToBase64(url);
			/** 构造多模态内容数组 */
			const content: [EntryAPI.ImageContent, EntryAPI.TextContent] = [
				{ type: "image_url", image_url: { url } },
				{ type: "text", text: text || EntryAPI.OnlyData.imagePrompt }
			];
			// 合并多模态内容和文本消息
			processedMessages.push({ role, content });
		}
	}
	return processedMessages;
};

/**
 * 渲染调试信息
 *
 * @param {EntryAPI.PostMessage[]} messages - 最终的消息数组
 *
 * @param {number} sortedCount - 排序的消息数量
 *
 * @param {number} recentCount - 保持原序的最近消息数量
 */
async function renderDebugInfo(messages: EntryAPI.PostMessage[], sortedCount: number, recentCount: number): Promise<void> {
	/** 序列化消息数组 */
	const messagesJson = JSON.stringify(messages, null, 2);
	/** 调试信息 */
	const debugInfo = [
		`排序策略: 最近${recentCount}条保持原序, ${sortedCount}条按相似度排序`,
		`总消息数: ${messages.length}`].join('\n');
	/** 消息格式的修饰符 */
	const modify = ['<think>\n```json\n', '\n```\n</think>'];
	/** 渲染处理后的消息数组 */
	const messageElement = await EntryAPI.tracelessRenderMessage(
		modify[0] + messagesJson + modify[1] + debugInfo,
		EntryAPI.chatHistoryPanel
	);
	// 为think区块添加折叠功能
	(messageElement?.querySelectorAll(".toggle_think_button") as NodeListOf<HTMLButtonElement>).forEach(EntryAPI.bindFoldingButton);
};

/**
 * 获取默认的历史消息
 *
 * @param {number} maxMessages 最大消息数量
 *
 * @param {HTMLElement|undefined} messageElement - 消息元素，可选参数
 *
 * @returns {Promise<EntryAPI.PostMessage[]>} 默认的历史消息数组
 */
async function getDefaultHistory(maxMessages: number, messageElement?: HTMLElement): Promise<EntryAPI.PostMessage[]> {
	/** 最后指定数量的历史消息数组 */
	const lastMessages = EntryAPI.OnlyData.historyMessage.slice(-maxMessages);
	// 转换为 PostMessage 格式
	return await convertToPostMessageFormat(lastMessages, messageElement);
}

/**
 * 创建与月华交互的消息数组
 *
 * @param {string|undefined} promptMessage - 自定义提示消息，可选参数
 *
 * @param {HTMLElement|undefined} messageElement - 消息元素，可选参数
 *
 * @returns {Promise<Array>} 包含role和content属性的消息对象数组
 */
export async function createMessages(promptMessage?: string, contentElement?: HTMLElement): Promise<EntryAPI.PostMessage[]> {
	/** 加载对话历史消息 */
	const messages: EntryAPI.PostMessage[] = await buildContextMessages(contentElement);
	/** 查询当前地址 */
	async function queryCurrentAddress(): Promise<string[]> {
        // 如果当前地址已缓存，直接返回
        if (EntryAPI.OnlyData.currentAddress.length > 0) return EntryAPI.OnlyData.currentAddress;
		/** 从IP地址查询位置信息 */
		const addressRegion = await fetch('https://ipapi.co/json/')
		// 检查响应状态
		if (!addressRegion.ok) {
			EntryAPI.showSystemMessage('获取位置失败：' + addressRegion.statusText, 'error');
			return ['江苏省', '南京市'];
		}
		/** 解析JSON响应 */
		const data = await addressRegion.json();
		/** 提取省份信息 */
		const province = data.region;
		/** 提取城市信息 */
		const city = data.city;
		// 确保省份和城市信息存在
		if (!province || !city) {
			EntryAPI.showSystemMessage('获取位置失败：' + '省份或城市信息缺失', 'error');
			return ['江苏省', '南京市'];
		}
		// 缓存当前地址
		EntryAPI.OnlyData.currentAddress = [province, city];
		// 返回省份和城市
		return [province, city];
	};
	// 添加系统提示消息
	if (EntryAPI.OnlyData.systemPrompt) {
		/** 替换系统提示中的占位符 */
		const systemPrompt = EntryAPI.OnlyData.systemPrompt
			// 转换用户名称
			.replace(/{name}/g, EntryAPI.OnlyData.customConfig.userName || "你")
			// 转换当前时间
			.replace(/{current-time}/g, new Date().toLocaleString())
			// 转换当前地址
			.replace(/{current-address}/g, await queryCurrentAddress().then(address => address.join(' ')));
		// 确保系统提示消息在数组最前面
		messages.unshift({ role: "system", content: systemPrompt })
	};
	// 添加自定义提示消息
	if (promptMessage) messages.push({ role: "user", content: promptMessage });
	// 输出消息数组
	return messages;
};

/**
 * 创建助手消息元素并渲染到页面，为后续接收API响应做准备
 *
 * @param {HTMLElement} container - 消息容器元素，用于渲染助手消息
 *
 * @param {relay.HistoryMessage} message - 助手消息内容
 *
 * @returns {Promise<HTMLElement | null>}  返回渲染到页面上的助手消息元素
 */
export async function createMessageElement(container: HTMLElement, message: EntryAPI.HistoryMessage): Promise<HTMLElement | null> {
	/**
	 * 创建消息元素并将其渲染到页面上
	 *
	 * 这样用户可以看到助手已经开始准备回复
	 */
	let messageElement = EntryAPI.renderMessage(message, container);
	// 为消息元素创建停止按钮，允许用户中止当前的API请求
	if (messageElement) EntryAPI.createStopButton(messageElement);
	// 禁用输入按钮，防止用户在请求处理期间重复发送消息，避免请求冲突
	EntryAPI.disabledReleaseButton(true);
	// 创建中止控制器，用于后续在需要时取消正在进行的API请求
	EntryAPI.OnlyData.abortController = new AbortController();
	// 设置月华为思考状态（无超时），提示用户月华正在处理请求
	EntryAPI.setEmotionState(EntryAPI.EmotionalState.THINKING);
	// 返回渲染到页面上的消息元素，供后续操作使用
	return messageElement;
};

/**
 * 获取用户输入的消息并清空输入框
 *
 * 从截图输入框、Live2D输入框和聊天输入框中获取内容，拼接后返回，同时清空这些输入框
 *
 * @returns {string[]} 拼接后的用户输入消息数组, 若消息长度超过最大长度, 则按最大长度拆分
 */
export function getUserMessage(): string[] {
	/**
	 * 将各输入框内容存储到数组中
	 */
	const userInput = [EntryAPI.screenshotWriteArea, EntryAPI.live2dWriteArea, EntryAPI.chatWriteArea, EntryAPI.renderWriteArea, EntryAPI.noteWriteArea] as HTMLInputElement[];
	/**
	 * 过滤掉空字符串
	 */
	const message = userInput.map(item => item.value.trim()).filter(item => item).join('\n');
	// 清空所有输入框
	userInput.forEach(item => item.value = '');
	// 移除所有文本域的 style 属性
	document.querySelectorAll('.auto-resize-textarea').forEach(textarea => textarea.removeAttribute('style'));
	// 返回拼接后的消息
	return EntryAPI.splitTextToStrings(message);
};

/**
 * 创建助手消息元素
 *
 * @param {HTMLElement} container - 消息容器元素
 *
 * @returns {Promise<MessageElement>}
 */
async function createAssistantMessageElement(container: HTMLElement): Promise<MessageElement> {
	/** 创建用于占位的空 AI消息对象 */
	const messageObject = await EntryAPI.createMessageObject("assistant", "");
	/** 将助手消息渲染到页面 */
	const messageElement = await createMessageElement(container, messageObject);
	/** 获取AI消息内容元素 */
	const contentElement = messageElement?.querySelector(".markdown-content") as HTMLElement;
	// 返回消息元素关联对象
	return { messageObject, messageElement, contentElement };
};

/**
 * 执行对话并解析响应，将助手消息渲染到页面
 *
 * @param {HTMLElement} container - 消息容器元素
 *
 * @param {string|undefined} promptMessage - 自定义提示消息
 */
export async function executeDialogueAndParse(container: HTMLElement, promptMessage?: string) {
	// 生成助手消息元素关联对象
	const { messageObject, messageElement, contentElement } = await createAssistantMessageElement(container);
	// 检查消息元素是否存在
	if (!contentElement) {
		// 若内容元素不存在，清理资源并返回
		await EntryAPI.cleanupResources(contentElement, messageObject, messageElement);
		return;
	}
	try {
		/** 构建消息数组 */
		const messages = await createMessages(promptMessage, contentElement);
		/** 处理状态 */
		const state: EntryAPI.StreamProcessingState = {
			/** 累积所有工具调用的数组 */
			toolCalls: [] as any[],
			/** 当前正在累积的工具调用对象 */
			currentToolCall: null as any,
			/** 当前工具调用在流中的索引，用于识别是否属于同一次调用 */
			currentToolCallIndex: -1,
			/** 累积当前工具调用的参数字符串 */
			currentFunctionArgs: "",
			/** 累积当前工具调用的函数名 */
			currentFunctionName: "",
			/** 独立推理内容的字符串累积 */
			reasoningContent: "",
			/** 提取思考内容的字符串累积 */
			thinkingContent: "",
			/** 提取描述内容的字符串累积 */
			descriptionContent: ""
		};
		/** 发送请求并处理工具调用 */
		const result = await EntryAPI.sendRequestWithTools(messages, container, messageObject, contentElement, state);
		// 如果启用了自动播放功能，播放语音
		if (EntryAPI.OnlyData.autoPlaySpeech) EntryAPI.playSpeechModel();
		// 执行聊天结束事件
		await handleChatEndEvent(result.textContent, contentElement);
	}
	catch (error) {
		// 忽略中止错误
		if (!(error instanceof Error) || error.name === "AbortError") return;
		// 捕获异常并显示错误信息
		EntryAPI.showSystemMessage(`${error.name} | ${error.message} | ${error.stack}`, "error");
		// 渲染错误消息到聊天记录
		EntryAPI.tracelessRenderMessage(`抱歉，请求处理时出错: ${error.message}`, container);
	}
	finally {
		await EntryAPI.cleanupResources(contentElement, messageObject, messageElement);
	}
};

/**
 * 发送聊天消息到后端模型
 */
export async function sendChatMessageToBackendModel() {
	/**
	 * 获取用户输入并去除前后空格
	 */
	const userMessage = getUserMessage();
	// 检查消息是否为空或按钮是否禁用
	if (!userMessage.join('\n').trim() || EntryAPI.getReleaseButtonsDisabledState()) return;
	/**
	 * 发送消息到后端模型
	 */
	async function SendMessage(message: string) {
		/**
		 * 添加用户消息到聊天记录
		 */
		const userMsgObj = EntryAPI.createMessageObject("user", message);
		// 将用户消息渲染到页面上
		EntryAPI.renderMessage(await userMsgObj, EntryAPI.chatHistoryPanel);
		// 等待 1 秒，确保前端渲染完成后再继续
		await new Promise(resolve => setTimeout(resolve, 500));
	}
	// 遍历用户消息数组，依次发送每个消息
	for (let i = 0; i < userMessage.length; i++) {
		await SendMessage(userMessage[i]);
	}
	// 调用中止控制器的abort方法，中止当前正在进行的API请求
	EntryAPI.OnlyData.abortController?.abort();
	// 停止语音播放
	EntryAPI.stopSpeechModel();
	// 调用后端 API 继续对话流程
	await executeDialogueAndParse(EntryAPI.chatHistoryPanel, undefined);
};

/**
 * 处理聊天结束事件，解析消息中的事件标签并执行相应操作，同时根据消息内容处理特殊展示。
 *
 * @param {string} assistantMessage - 助手发送的消息内容
 *
 * @param {HTMLElement} messageElement - 用于展示内容的 DOM 元素
 */
export async function handleChatEndEvent(assistantMessage: string, messageElement: HTMLElement) {
	/** 从助手消息中提取结论内容 */
	const extractedContent = EntryAPI.extractConclusion(assistantMessage);
	// 若消息包含 markdown 代码块，则重新加载消息并处理 markdown
	if (/```markdown/i.test(assistantMessage)) {
		EntryAPI.reloadMessageAndMarkdown(assistantMessage, messageElement);
	}
	// 否则生成集合渲染
	else EntryAPI.generateCollectionRendering(messageElement);
	// 绑定代码执行按钮
	EntryAPI.bindCodeExecuteButtons(messageElement);
	// 解析提取的内容中的事件标签并执行对应的处理函数
	await parseEventTag(extractedContent, 500);
	// 若连续记忆模式已启用，则永久化对话历史中的所有消息
	if (EntryAPI.OnlyData.isContinuousMemory) {
		await EntryAPI.controlContinuousMemory.run();
	}
	// 若主动消息模式已启用，则在 1 分钟后触发主动延续对话的消息
	if (EntryAPI.OnlyData.isActiveMessageMode) {
		/**
		 * 计算延迟执行的时间
		 */
		const delay = EntryAPI.DelayExecutionManager.calculateDelayTime(1);
		// 调用延迟执行管理器，在指定延迟时间后运行约束执行器
		EntryAPI.DelayExecutionManager.call("主动延续话题", async () => EntryAPI.controlActiveMessage.run(), delay);
	};
	//	若对话历史长度大于等于 16，则启用连续记忆模式
	if (EntryAPI.OnlyData.historyMessage.length >= 16 && !EntryAPI.OnlyData.isContinuousMemory) {
		// 更新连续记忆模式按钮图标为无限循环图标
		EntryAPI.longTermMemoryButton.innerHTML = '<i class="fas fa-infinity"></i>';
		// 显示连续记忆模式已启用的系统消息
		EntryAPI.showSystemMessage("启用< 连续记忆模式 >", "success");
		// 启用连续记忆模式
		EntryAPI.OnlyData.isContinuousMemory = true;
		// 切换连续记忆模式按钮样式
		EntryAPI.longTermMemoryButton.classList.add("clicking");
	}
	// 若对话历史长度大于等于 8，则启用主动消息模式
	if (EntryAPI.OnlyData.historyMessage.length >= 8 && !EntryAPI.OnlyData.isActiveMessageMode) {
		// 更新主动消息模式按钮图标为内存图标
		EntryAPI.activeMessageButton.innerHTML = '<i class="fas fa-comment-dots"></i>';
		// 显示主动消息模式已启用的系统消息
		EntryAPI.showSystemMessage("启用< 主动消息模式 >", "success");
		// 启用主动消息模式
		EntryAPI.OnlyData.isActiveMessageMode = true;
		// 切换主动消息模式按钮样式
		EntryAPI.activeMessageButton.classList.add("clicking");
	}
};

/**
 * 解析输入内容中的事件标签，并根据标签调用对应的处理函数。
 *
 * 每个标签处理完成后，根据指定的延迟时间进行等待。
 *
 * @param {string} input - 待解析的输入内容，可能包含事件标签
 *
 * @param {number} [delayMs=10] - 处理标签后的延迟时间（毫秒），默认为 10 毫秒；若为 0 则不延迟
 */
async function parseEventTag(input: string, delayMs: number = 10) {
	/** 用于存储所有标签中的内容 */
	let mergedIntel = '';
	/** 构建正则表达式，用于匹配所有 <!--(.*?)--> 标签格式的内容（不考虑空格） */
	const pattern = new RegExp(`<!--\s*(.*?)\s*-->`, 'g');
	/** 用于存储正则匹配结果 */
	let match: any[] | null;
	// 循环匹配输入内容中的所有符合条件的标签
	while ((match = pattern.exec(input)) !== null) {
		// 提取标签中的信息并合并
		mergedIntel += match[1];
	}
	// 等待指定的延迟时间，确保视觉效果符合预期
	await new Promise(resolve => setTimeout(resolve, delayMs));
	// 如果匹配到标签内容，则调用情绪模式匹配
	if (mergedIntel.trim().length > 0) EntryAPI.matchEmotionalPatterns(mergedIntel);
	// 如果没有标签，则进入说话模式
	else EntryAPI.setStateWithTimeout(EntryAPI.EmotionalState.SPEAKING);
};
