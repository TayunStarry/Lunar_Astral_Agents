import * as EntryAPI from '../EntryAPI/code';

/**
 * 创建消息对象，包含时间戳、是否为提示、是否不渲染、角色和内容
 *
 * @param {string} role - 消息的角色，例如 'user', 'assistant' 等
 *
 * @param {string} content - 消息的内容
 *
 * @param {boolean} [recorded=true] - 是否将消息记录到对话历史中，默认为 true
 *
 * @param {boolean} [isPrompt=false] - 消息是否为提示，默认为 false
 *
 * @param {boolean} [noRender=false] - 消息是否不进行渲染，默认为 false
 *
 * @param {string} [imageUrl=null] - 图片消息的 URL，默认为 null (用于在消息中渲染图片)
 *
 * @param {string} [deletable=null] - 消息是否可删除，默认为 null (用于在删除消息时删除文件)
 *
 * @returns {HistoryMessage} 包含消息信息的对象
 */
export async function createMessageObject(role: EntryAPI.PostMessageRole, content: string, recorded: boolean = true, isPrompt: boolean = false, noRender: boolean = false, imageUrl: string | null = null, deletable: boolean | null = null): Promise<EntryAPI.HistoryMessage> {
	/** 消息对象 */
	const message: EntryAPI.HistoryMessage = {
		role,
		content,
		isPrompt,
		noRender,
		imageUrl,
		deletable,
		uuid: createUniqueLabel(),
		embedVector: content.length >= 1 ? await new EntryAPI.EmbeddingRequest(content, false, false).output() : null,
	};
	// 如果消息被记录, 则添加到对话历史中
	if (recorded) EntryAPI.OnlyData.historyMessage.push(message);
	// 返回创建的消息对象
	return message;
};

/**
 * 创建图片消息对象，包含时间戳、角色、内容、图片URL、是否可删除和唯一标识符
 *
 * @param {string} role - 消息的角色，例如 'user', 'assistant' 等
 *
 * @param {string} content - 消息的内容
 *
 * @param {string} imageUrl - 图片消息的 URL
 *
 * @returns {EntryAPI.HistoryMessage} 包含图片消息信息的对象
 */
export function createImageMessage(role: EntryAPI.PostMessageRole, content: string, imageUrl: string, uuid?: string): EntryAPI.HistoryMessage {
	/** 消息对象 */
	const message: EntryAPI.HistoryMessage = {
		role,
		content,
		isPrompt: false,
		noRender: false,
		imageUrl,
		deletable: true,
		uuid: uuid || createUniqueLabel(),
		embedVector: null,
	};
	// 返回创建的消息对象
	return message;
};

/**
 * 创建轻量渲染内容
 *
 * 从输入框获取内容，若内容不为空则添加消息到历史记录并进行渲染，最后清空输入框
 */
export async function createSimpleRendering() {
	/**
	 * 获取用户输入的消息
	 */
	const userMessage = EntryAPI.getUserMessage();
	// 检查消息是否为空或按钮是否禁用
	if (!userMessage.join('\n').trim() || EntryAPI.getReleaseButtonsDisabledState()) return;
	/**
	 * 发送消息到后端模型
	 */
	async function SendMessage(message: string) {
		/**
		 * 调用 createMessageObject 函数添加消息到历史记录，获取消息对象
		 */
		const userMsgObj = await createMessageObject("user", message, false, true);
		// 清空已经渲染过的内容
		EntryAPI.simpleRenderingPanel.innerHTML = '';
		// 调用 renderMessage 函数渲染消息到指定面板
		EntryAPI.renderMessage(userMsgObj, EntryAPI.simpleRenderingPanel);
	}
	// 遍历用户消息数组，依次发送每个消息
	userMessage.forEach(SendMessage);
};

/**
 * * 生成一个符合UUID格式的唯一字符串标识符
 *
 * * UUID (Universally Unique Identifier) 是一种在分布式系统中用来唯一标识信息的标准
 *
 * * 此函数用于创建一个随机的UUID, 它遵循RFC 4122标准的版本4（随机UUID）
 *
 * @returns {string} 返回一个UUID字符串, 格式为 xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx
 *
 * * 其中 x 表示一个随机的十六进制数字, y 表示一个随机生成但经过特定处理的十六进制数字
 */
export function createUniqueLabel(): string {
	// 定义UUID的模式, 包含固定的'-'位置和需要被替换的'x'和'y'字符
	return 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g,
		function (character: string) {
			/**
			 * * 获取一个随机数, 范围在0到15之间, 并转换为整数
			 */
			const randomValue = (Math.random() * 16) | 0;
			/**
			 * * 根据字符类型（x 或 y）返回一个随机数, 范围在0到15之间, 并转换为整数
			 */
			const maskedRandomValue = character === 'x' ? randomValue : (randomValue & 0x3 | 0x8);
			// 将处理后的随机数转换为十六进制字符串
			return maskedRandomValue.toString(16);
		}
	);
};

/**
 * 处理聊天容器的自动滚动行为
 *
 * 当用户接近底部时（距离底部小于容器高度的15%），在有新消息添加后自动滚动到底部
 * 当用户正在查看历史消息时，保持当前滚动位置
 *
 * @param {HTMLElement} container - 消息容器元素
 *
 * @param {Object} options - 配置选项
 *
 * @param {number} [options.threshold=0.1] - 触发滚动的阈值比例（相对于容器高度）
 *
 * @param {boolean} [options.smooth=true] - 是否使用平滑滚动效果
 *
 * @returns {boolean} - 是否执行了滚动操作
 */
export function autoScrollToBottom(container: HTMLElement, options: { threshold?: number, smooth?: boolean }): boolean {
	// 参数验证
	if (!(container instanceof HTMLElement)) {
		return false;
	}
	// 设置默认选项
	const { threshold = 0.15, smooth = true } = options;
	/**
	 * 获取消息容器的滚动高度减去滚动位置和容器高度的差值
	 */
	const distanceToBottom = container.scrollHeight - container.scrollTop - container.clientHeight;
	/**
	 * 滚动阈值，用于判断用户是否接近底部
	 */
	const scrollThreshold = container.clientHeight * threshold;
	// 只有当用户接近底部时, 执行滚动
	if (distanceToBottom <= scrollThreshold) {
		container.scrollTo({ top: container.scrollHeight, behavior: smooth ? 'smooth' : 'auto' });
		return true;
	}
	return false;
};

/**
 * 处理工具调用的辅助函数
 *
 * @param {EntryAPI.ToolCall[]} toolCalls 工具调用数组
 *
 * @param {EntryAPI.PostMessage[]} messages 消息历史数组
 *
 * @param {HTMLElement} messageElement 消息元素
 *
 * @param {EntryAPI.HistoryMessage} messageObject 消息对象
 *
 * @returns {boolean} 是否有工具调用
 */
export async function handleToolCalls(state: EntryAPI.ChatCache, messages: EntryAPI.PostMessage[], messageElement: HTMLElement, messageObject: EntryAPI.HistoryMessage): Promise<boolean> {
	/** 工具调用标志 */
	let hasToolCalls = false;
	// 遍历所有工具调用
	for (const toolCall of state.toolCalls) {
		// 仅处理函数类型的工具调用
		if (toolCall.type !== "function") continue;
		/** 工具函数名称 */
		const functionName = toolCall.function.name;
		/** 工具函数参数 */
		const functionArgs = toolCall.function.arguments;
		/** 查询对应的月华工具包 */
		const lunarToolPackage = EntryAPI.OnlyData.lunarToolPackageMap.get(functionName);
		// 检查是否有对应的工具包
		if (!lunarToolPackage) {
			messages.push({ role: "tool", content: `未找到工具包: ${functionName}`, tool_call_id: toolCall.id });
			continue;
		}
		try {
			/** 工具函数执行结果 */
			const toolResult = await lunarToolPackage(functionArgs, messageElement, messageObject);
			// 将工具响应添加到消息历史中
			messages.push({ role: "tool", content: toolResult, tool_call_id: toolCall.id });
			// 标记有工具调用
			hasToolCalls = true;
		}
		catch (error) {
			// 忽略非Error类型的异常
			if (!(error instanceof Error)) return false;
			// 将工具调用失败信息添加到消息历史中
			messages.push({ role: "tool", content: `调用${functionName}失败: ${error}`, tool_call_id: toolCall.id });
		}
	}
	// 处理完所有工具调用后，清空状态
	state.currentToolCallIndex = -1;
	state.currentFunctionArgs = "";
	state.currentFunctionName = "";
	state.currentToolCall = null;
	state.toolCalls = [];
	// 标记有工具调用
	return hasToolCalls;
};

/**
 * 订阅工具调用事件
 *
 * @param {string} name 工具函数名称
 *
 * @param {(args: Object, messageElement: HTMLElement) => Promise<Object>} callback 事件回调函数
 */
export function subscriptionToolCall(name: string, callback: (args: EntryAPI.ToolCallParameters, messageElement: HTMLElement, messageObject: EntryAPI.HistoryMessage) => Promise<string>) {
	/**
	 * 实际注册到映射表的异步包装函数
	 * 统一打印调用日志并转发结果
	 */
	async function event(args: EntryAPI.ToolCallParameters, messageElement: HTMLElement, messageObject: EntryAPI.HistoryMessage): Promise<string> {
		// 判断是否是调试模式, 决定是否显示工具参数
		if (EntryAPI.OnlyData.isDebugMode) EntryAPI.showSystemMessage(`月华将使用: ${name} ${JSON.stringify(args)}`, 'success');
		// 非调试模式下, 仅显示工具名称
		else EntryAPI.showSystemMessage(`月华将使用: ${name}`, 'success');
		// 调用实际的回调函数
		return await callback(args, messageElement, messageObject);
	}
	// 注册到全局工具函数映射表
	EntryAPI.OnlyData.lunarToolPackageMap.set(name, event);
};