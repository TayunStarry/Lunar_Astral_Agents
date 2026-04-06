import * as EntryAPI from '../EntryAPI/code';

/**
 * 渲染单条消息到指定容器中
 *
 * @param {relay.HistoryMessage} message - 要渲染的消息对象，包含消息内容、角色、时间戳等信息
 *
 * @param {HTMLElement} container - 消息要渲染到的容器元素
 *
 * @returns {HTMLElement|null} - 返回渲染后的消息元素，如果 message.noRender 为 true 则返回 null
 */
export function renderMessage(message: EntryAPI.HistoryMessage, container: HTMLElement): HTMLElement | null {
	// 如果消息标记为不渲染，则直接返回
	if (message.noRender) return null;
	/**
	 * 创建消息的根元素
	 */
	const messageElement = document.createElement("div") as HTMLDivElement;
	/**
	 * 构建语音播放按钮的 HTML 字符串
	 *
	 * 如果消息角色是助手，则返回语音播放按钮的 HTML，否则返回空字符串
	 *
	 * @returns {string} - 语音播放按钮的 HTML 字符串或空字符串
	 */
	function buildSoundButton(): string {
		if (message.role === "assistant") {
			return [
				'<button class="chat-action-button play_speech_button" title="播放语音">',
				`<i class="fas fa-volume-up"></i>`,
				'</button>'
			].join("");
		}
		return "";
	};
	// 为消息根元素添加基础类名
	messageElement.classList.add("message");
	// 设置消息元素的初始 HTML 结构
	messageElement.innerHTML = [
		// 消息头
		'<div class="message-header">',
		`<span>${message.role === "user" ? EntryAPI.OnlyData.customConfig.userName || "你" : "月华"}</span>`,
		'</div>',
		// 消息正文
		`<div class="markdown-content">${message.content}</div>`,
		// 消息操作面板（默认顶部对齐）
		'<div class="message-actions-panel top-align">',
		// 复制消息按钮
		'<button class="chat-action-button copy_message_button" title="复制消息">',
		'<i class="fas fa-copy"></i>',
		'</button>',
		// 删除消息按钮
		'<button class="chat-action-button delete_message_button" title="删除消息">',
		'<i class="fas fa-trash"></i>',
		'</button>',
		// 语音播放按钮
		buildSoundButton(),
		'</div>',
	].join("");
	/**
	 * 获取消息内容元素
	 */
	const contentElement = messageElement.querySelector(".markdown-content") as HTMLElement;
	// 处理消息内容中的思考标签
	contentElement.innerHTML = EntryAPI.processThinkTags(message.content);
	// 生成集合渲染
	generateCollectionRendering(contentElement);
	// 如果消息是提示消息，则移除操作面板和头部，并添加文件消息类名
	if (message?.isPrompt) {
		/** 消息操作面板 */
		const actionsPanel = messageElement.querySelector(".message-actions-panel") as HTMLElement;
		// 移除顶部对齐类名
		actionsPanel.classList.remove("top-align");
		// 添加底部对齐类名
		actionsPanel.classList.add("bottom-align");
		// 如果消息没有文件链接，则移除消息的操作面板
		if (!message?.deletable) actionsPanel.remove();
		// 移除消息的头部信息
		(messageElement.querySelector(".message-header") as HTMLElement).remove();
		// 为消息元素添加文件消息类名，用于样式控制
		messageElement.classList.add("file-message");
		// 随机选择一个边框颜色
		messageElement.style.borderColor = `var(${randomColorStyle()})`;
	}
	// 如果是用户消息，则添加用户消息类名
	else if (message.role === "user") messageElement.classList.add("user-message");
	// 否则为助手消息，添加助手消息类名
	else messageElement.classList.add("assistant-message");
	// 如果消息内容不为空或角色是助手，则执行聊天气泡的创建
	if (message.content.trim() || message.role === 'assistant') {
		// 对消息中的代码块进行语法高亮处理
		messageElement.querySelectorAll('pre code').forEach(block => (window as any).hljs.highlightElement(block));
		// 绑定消息的操作按钮事件
		EntryAPI.bindMessageActionEvents(messageElement, message);
		// 绑定代码执行按钮事件
		EntryAPI.bindCodeExecuteButtons(messageElement);
		// 将消息元素添加到容器中
		container.appendChild(messageElement);
		// 滚动容器到最底部，确保新消息可见
		container.scrollTo({ top: container.scrollHeight, behavior: 'smooth' })
	}
	// 如果消息包含图片 URL，则添加图片渲染
	if (message.imageUrl) {
		/** 构建图片消息对象并清空文本内容 */
		const imageMessage = { ...message, content: '' };
		// 添加图片渲染
		addImageRendering(imageMessage, container)
	};
	// 返回渲染后的消息元素
	return messageElement;
};

/**
 * 无迹渲染消息
 *
 * @param {string} message - 要渲染的消息内容
 *
 * @param {HTMLElement} container - 消息要渲染到的容器元素
 *
 * @returns {Promise<HTMLElement | null> } - 返回渲染后的消息元素，如果 message.noRender 为 true 则返回 null
 */
export async function tracelessRenderMessage(message: string, container: any): Promise<HTMLElement | null> {
	return renderMessage(await EntryAPI.createMessageObject("assistant", message, false, true), container);
};

/**
 * 渲染对话历史中的所有消息到指定容器中
 *
 * @param {HTMLElement} container - 消息要渲染到的容器元素
 *
 * @param {boolean} clearPage - 是否清空容器内的现有内容，默认为 true
 *
 * @param {EntryAPI.HistoryMessage[]} messageArray - 要渲染的消息数组，默认为 EntryAPI.OnlyData.historyMessage
 *
 * @returns {Promise<void>} 该函数不返回任何值
 */
export async function renderAllMessages(container: HTMLElement, clearPage: boolean = true, messageArray: EntryAPI.HistoryMessage[] = EntryAPI.OnlyData.historyMessage): Promise<void> {
	// 清空容器内的现有内容
	if (clearPage) container.innerHTML = '';
	// 滚动到容器顶部
	container.scrollTo({ top: 0, behavior: 'smooth' })
	// 遍历对话历史中的每条消息
	for (const message of messageArray) {
		/** 调用 renderMessage 函数渲染单条消息到容器中 */
		const newMessage = renderMessage(message, container);
		// 若消息渲染成功，则查找消息中的所有思考折叠按钮
		(newMessage?.querySelectorAll(".toggle_think_button") as NodeListOf<HTMLButtonElement>).forEach(EntryAPI.bindFoldingButton);
		// 等待 0.5 秒，确保消息渲染完成
		await new Promise(resolve => setTimeout(resolve, 500));
	}
	/** 统计对话历史中标记为不渲染的消息数量 */
	const hiddenCount = messageArray.filter((msg: { noRender: any; }) => msg.noRender).length;
	// 若存在标记为不渲染的消息，则创建一条提示消息告知用户剩余文件信息数量
	if (hiddenCount >= 1) tracelessRenderMessage(`月华这还有 **${hiddenCount}** 个文件片段哦~~`, container);
};

/**
 * 在指定容器内渲染Mermaid图表
 * @param {HTMLElement} contentElement - 包含Mermaid代码块的DOM容器元素
 */
export async function generateMermaidChart(contentElement: HTMLElement): Promise<void> {
	try {
		/**
		 * 查找容器内所有Mermaid代码块
		 */
		const mermaidBlocks: NodeListOf<HTMLElement> = contentElement.querySelectorAll('code.language-mermaid');
		/**
		 * 渲染单个Mermaid图表
		 * @param {HTMLElement} block - Mermaid代码块元素
		 */
		async function chartRendering(block: HTMLElement): Promise<void> {
			// 若代码块内容长度小于等于20个字符，则直接返回，不渲染图表
			if (block.textContent.length <= 20) return;
			/**
			 * 获取Mermaid图表定义代码
			 */
			const graphDefinition = EntryAPI.removeCodeComments(block.textContent);
			/**
			 * 创建图表容器元素
			 */
			const container = document.createElement('div');
			// 设置容器类名
			container.className = 'mermaid-container';
			/**
			 * 获取代码块的父元素
			 */
			const parent = block.parentElement;
			// 将容器插入到代码块之前
			if (parent) parent.insertBefore(container, block);
			// 渲染Mermaid图表
			try {
				// 解析Mermaid图表定义代码
				try {
					// 解析Mermaid图表定义代码
					await (window as any).mermaid.parse(graphDefinition);
				}
				catch (parseError) {
					// 检查是否是Mermaid语法错误
					if (parseError instanceof Error && parseError.message.includes('Mermaid syntax error')) {
						// 解析失败时抛出包含错误信息的新错误
						throw new Error(`Mermaid语法错误: ${parseError.message}`);
					}
					else {
						// 其他解析错误，直接抛出
						throw parseError;
					}
				}
				/**
				 * 生成唯一的图表ID
				 */
				const id = `mermaid-${Date.now()}-${Math.floor(Math.random() * 10000)}`;
				/**
				 * 渲染Mermaid图表，获取SVG内容
				 */
				const { svg } = await (window as any).mermaid.render(id, graphDefinition);
				/**
				 * 创建DOMParser实例，用于解析SVG字符串
				 */
				const parser = new DOMParser();
				/**
				 * 解析SVG字符串为DOM文档
				 */
				const doc = parser.parseFromString(svg, 'image/svg+xml');
				/**
				 * 获取SVG元素
				 */
				const svgElement = doc.documentElement;
				/**
				 * 获取图表类型
				 */
				const chartType = svgElement.getAttribute('aria-roledescription');
				// 如果是流程图，调整viewBox参数
				if (chartType === 'flowchart') {
					/**
					 * 获取SVG元素的viewBox属性
					 */
					const viewBox = svgElement.getAttribute('viewBox');
					// 检查viewBox属性是否存在
					if (viewBox) {
						/**
						 * 将viewBox值按空格分割并转换为数字数组
						 */
						const values = viewBox.split(/\s+/).map(parseFloat);
						// 检查数组长度和元素是否为有效数字
						if (values.length === 4 && values.every(v => !isNaN(v))) {
							// 调整viewBox的四个值
							values[0] *= 0.45;
							values[1] *= 0.45;
							values[2] *= 1.05;
							values[3] *= 1.05;
							// 更新SVG元素的viewBox属性
							svgElement.setAttribute('viewBox', values.join(' '));
						}
					}
				}
				// 如果是类图，调整viewBox参数
				if (chartType === 'classDiagram') {
					/**
					 * 获取SVG元素的引用的viewBox属性
					 */
					const viewBox = svgElement.getAttribute('viewBox');
					// 检查viewBox属性是否存在
					if (viewBox) {
						/**
						 * 将viewBox值按空格分割并转换为数字数组
						 */
						const values = viewBox.split(/\s+/).map(parseFloat);
						// 检查数组长度和元素是否为有效数字
						if (values.length === 4 && values.every(v => !isNaN(v))) {
							// 调整viewBox的四个值
							values[0] *= 0;
							values[1] *= 0.35;
							values[2] *= 1.05;
							values[3] *= 1.25;
							// 更新SVG元素的viewBox属性
							svgElement.setAttribute('viewBox', values.join(' '));
						}
					}
				}
				/**
				 * 将SVG元素序列化为字符串
				 */
				const modifiedSVG = new XMLSerializer().serializeToString(svgElement);
				// 将处理后的SVG内容插入到容器中，并添加边框样式
				container.innerHTML = `<div style="width: 100%; border: 10px dashed #eee; padding: 0px">${modifiedSVG}</div>`;
				// 移除原始的代码块
				if (parent) parent.removeChild(block);
			}
			catch (mermaidError) {
				// 检查是否是Mermaid渲染错误
				if (mermaidError instanceof Error && mermaidError.message.includes('Mermaid render error')) {
					// 捕获异常并显示错误信息
					EntryAPI.showSystemMessage(`${mermaidError.name} | ${mermaidError.message} | ${mermaidError.stack}`, "error");
					// 设置容器类名，用于样式化错误显示
					container.className = 'mermaid-error';
					// 在容器中显示渲染失败的错误信息
					container.innerHTML = `<p>${mermaidError.message}</p>`;
					// 随机时间后创建图表重绘的主动思考事件
					setTimeout(() => chartRedrawing('Mermaid渲染失败', mermaidError.message), EntryAPI.RandomFloor(450, 550));
				}
			}
		}
		// 如果存在Mermaid代码块且Mermaid库已加载，则遍历渲染每个图表
		if (mermaidBlocks.length > 0 && typeof (window as any).mermaid !== 'undefined') {
			await Promise.all(Array.from(mermaidBlocks).map(chartRendering));
		}
		// 如果存在Mermaid代码块但Mermaid库未加载，输出警告信息
		else if (mermaidBlocks.length > 0) EntryAPI.showSystemMessage("Mermaid库未加载，无法渲染图表", "error");
	}
	catch (parseError) {
		if (parseError instanceof Error) {
			// 捕获异常并显示错误信息
			EntryAPI.showSystemMessage(`${parseError.name} | ${parseError.message} | ${parseError.stack}`, "error");
		}
	}
};

/**
 * 在指定容器内渲染ECharts图表
 *
 * @param {HTMLElement} contentElement - 包含ECharts代码块的DOM容器元素
 */
export function generateEChartsChart(contentElement: HTMLElement) {
	try {
		/**
		 * 定位所有ECharts代码块
		 */
		const echartsBlocks: NodeListOf<HTMLElement> = contentElement.querySelectorAll('code.language-echarts');
		// 无代码块时提前退出
		if (echartsBlocks.length === 0) return;
		// 步骤2: 验证ECharts库加载状态
		if (typeof (window as any).echarts === 'undefined') return EntryAPI.showSystemMessage("ECharts库未加载，无法渲染图表", "error");
		/**
		 * 创建图表容器（替换原始代码块）
		 *
		 * @param {HTMLElement} block - 原始代码块
		 */
		function chartRendering(block: HTMLElement) {
			/**
			 * 创建图表容器（替换原始代码块）
			 */
			const container = document.createElement('div');
			// 设置容器样式
			container.className = 'echarts-container';
			container.style.cssText = 'width:100%; height:400px;';
			// 替换原始代码块
			block.parentElement?.replaceChild(container, block);
			// 创建图表实例
			try {
				// 若代码块内容长度小于等于64个字符，则直接返回，不渲染图表
				if (block.textContent.length <= 64) return;
				/**
				 * 解析JSON内容
				 */
				let config = JSON.parse(EntryAPI.removeCodeComments(block.textContent)) || {};
				// 配置完整性修复流程
				if (!config.series) config.series = [{ type: 'line', data: [5, 20, 36, 10, 10, 20] }];
				else if (!Array.isArray(config.series)) config.series = [config.series];
				// 修复 X 轴配置
				if (!config.xAxis) config.xAxis = { type: 'category', data: [] };
				else if (!config.xAxis.data) config.xAxis.data = [];
				// 修复Y轴配置
				if (!config.yAxis) config.yAxis = { type: 'value' };
				// 补充基础布局配置
				if (!config.grid) config.grid = { left: '3%', right: '4%', bottom: '3%', containLabel: true };
				// 添加默认标题
				if (!config.title) config.title = { text: '月华的绘图册', left: 'center', top: 10 };
				// 多系列时自动生成图例
				if (!config.legend && config.series.length > 1) config.legend = { data: config.series.map((s: { name: any; }) => s.name || '系列'), bottom: 10 };
				// 添加导出工具
				if (!config.toolbox) config.toolbox = { feature: { saveAsImage: {} } };
				/**
				 * 初始化图标容器
				 */
				const chart = (window as any).echarts.init(container);
				// 渲染图表
				chart.setOption(config);
				// 添加：存储图表实例到DOM元素上
				(container as any)._echartsInstance = chart;
				// 绑定响应式调整
				window.addEventListener('resize', () => chart.resize());
				// 添加：强制调整尺寸
				setTimeout(() => chart.resize(), 50);
			}
			catch (error) {
				if (!(error instanceof Error)) return;
				// 捕获异常并显示错误信息
				EntryAPI.showSystemMessage(`${error.name} | ${error.message} | ${error.stack}`, "error");
				// 设置容器类名，用于样式化错误显示
				container.className = 'echarts-error';
				// 在容器中显示渲染失败的错误信息
				container.innerHTML = `<p>${error.message}</p>`;
				// 随机时间后创建图表重绘的主动思考事件
				setTimeout(() => chartRedrawing('ECharts渲染失败', error.message), EntryAPI.RandomFloor(450, 550));
			}
		};
		// 遍历处理每个代码块
		echartsBlocks.forEach(chartRendering);
	}
	catch (error) {
		if (!(error instanceof Error)) return;
		// 捕获异常并显示错误信息
		EntryAPI.showSystemMessage(`${error.name} | ${error.message} | ${error.stack}`, "error");
	}
};

/**
 * 生成集合渲染，用于在指定容器内统一渲染多种类型的内容，包括Mermaid图表、ECharts图表和数学公式。
 *
 * @param {HTMLElement} contentElement - 包含需要渲染内容的DOM容器元素
 */
export function generateCollectionRendering(contentElement: HTMLElement) {
	// 调用函数在指定容器内渲染Mermaid图表
	generateMermaidChart(contentElement);
	// 调用函数在指定容器内渲染ECharts图表
	generateEChartsChart(contentElement);
	// 渲染页面中的公式（使用$...$或\(...\)语法）
	(window as any).renderMathInElement(contentElement,
		{
			// 定义公式的分隔符，用于识别不同格式的数学公式
			delimiters: [
				{ left: '$$', right: '$$', display: true }, // 双美元符号表示块级公式
				{ left: '$', right: '$', display: false }, // 单美元符号表示行内公式
				{ left: '\\(', right: '\\)', display: false }, // \( 和 \) 表示行内公式
				{ left: '\\[', right: '\\]', display: true } // \[ 和 \] 表示块级公式
			],
			// 遇到错误时不抛出异常
			throwOnError: false
		}
	);
};

/**
 * 生成指定描述对应的链接的二维码
 *
 * @param {HTMLElement} container - 用于存放二维码的容器元素
 *
 * @param {function} callback - 回调函数，用于处理链接
 */
export async function generateQRCode(container: HTMLElement, callback?: (data: any) => string) {
	/** 默认回调函数，用于处理链接 */
	const defaultCallback = (url: string) => url.replace(/\/v1$/, '');
	/** 默认链接 */
	const defaultUrl = 'https://gitee.com/TayunStarry/Lunar-Astral-Agents';
	// 若未提供回调函数，则使用默认回调函数
	callback = callback || defaultCallback;
	try {
		/** 初始化链接变量 */
		let url = window.location.origin
		// 如果最终链接为空，则使用默认链接
		if (url.trim() === '') url = defaultUrl;
		// 在指定容器中生成二维码
		new (window as any).QRCode(container,
			{
				text: url,
				width: 256,
				height: 256,
				colorDark: '#000000',
				colorLight: '#ffffff',
				correctLevel: (window as any).QRCode.CorrectLevel.H
			}
		);
	}
	catch (error) {
		if (!(error instanceof Error)) return;
		// 捕获异常并显示错误信息
		EntryAPI.showSystemMessage(`${error.name} | ${error.message} | ${error.stack}`, "error");
	}
};

/**
 * 图表重绘约束执行器，用于限制图表重绘操作的频率。
 *
 * 每个5分钟内最多允许3次图表重绘，超过次数则执行禁止回调。
 */
const chartRedrawConstraint = new EntryAPI.ConstraintExecution(5, 3, allowChartRedrawing, prohibitChartRedrawing);

/**
 * 允许图表重绘的回调函数
 *
 * @param {string} type - 图表类型
 *
 * @param {string} message - 相关消息
 */
async function allowChartRedrawing(type: string, message: string) {
	/**
	 * 获取图表重绘的Markdown内容
	 */
	let markdown = await EntryAPI.fetchMarkdown('/read/resources/prompts/chartRedrawing.md');
	// 替换Markdown中的占位符
	markdown = markdown.replace(/{type}/g, type).replace(/{message}/g, message);
	// 若调试模式开启，则渲染< 动态提示词 >
	if (EntryAPI.OnlyData.isDebugMode) {
		/**
		 * 渲染< 动态提示词 >
		 */
		const messageElement = await tracelessRenderMessage('<think>\n' + markdown + '\n</think>', EntryAPI.chatHistoryPanel);
		// 为think区块添加折叠功能
		(messageElement?.querySelectorAll(".toggle_think_button") as NodeListOf<HTMLButtonElement>).forEach(EntryAPI.bindFoldingButton);
	};
	// 从API加载对话内容
	await EntryAPI.executeDialogueAndParse(EntryAPI.chatHistoryPanel as HTMLElement, markdown);
	// 设置超时状态为用户输入状态
	EntryAPI.setStateWithTimeout(EntryAPI.EmotionalState.AWAIT);
};

/**
 * 禁止图表重绘的回调函数
 */
async function prohibitChartRedrawing() {
	/**
	 * 获取道歉消息的Markdown内容
	 */
	const markdown = await EntryAPI.fetchMarkdown('/read/resources/prompts/apologyMessage.md');
	// 若调试模式开启，则渲染< 动态提示词 >
	if (EntryAPI.OnlyData.isDebugMode) {
		/**
		 * 渲染< 动态提示词 >
		 */
		const messageElement = await tracelessRenderMessage('<think>\n' + markdown + '\n</think>', EntryAPI.chatHistoryPanel);
		// 为think区块添加折叠功能
		(messageElement?.querySelectorAll(".toggle_think_button") as NodeListOf<HTMLButtonElement>).forEach(EntryAPI.bindFoldingButton);
	};
	// 从API加载对话内容
	await EntryAPI.executeDialogueAndParse(EntryAPI.chatHistoryPanel as HTMLElement, markdown);
	// 设置超时状态为用户输入状态
	EntryAPI.setStateWithTimeout(EntryAPI.EmotionalState.AWAIT);
}

/**
 * 重新绘制图表相关操作
 *
 * @param {string} type - 图表类型
 *
 * @param {string} message - 相关消息
 */
export async function chartRedrawing(type: string, message: string) {
	// 如果输入按钮被禁用，则不执行后续逻辑
	if (EntryAPI.getReleaseButtonsDisabledState()) return;
	// 延迟3秒执行图表重绘约束执行器
	await new Promise(resolve => setTimeout(resolve, 3000));
	// 运行图表重绘约束执行器
	chartRedrawConstraint.run(type, message);
};

/**
 * 重新加载助手消息并处理其中的 Markdown 内容，执行一系列渲染和绑定操作
 *
 * @param {string} assistantMessage - 助手返回的消息内容
 *
 * @param {HTMLElement} contentElement - 助手消息的内容元素
 *
 * @returns {void} 该函数不返回任何值
 */
export function reloadMessageAndMarkdown(assistantMessage: string, contentElement: { closest: (arg0: string) => any; }): void {
	/**
	 * 移除 markdown 代码块的标记，只保留代码块内的内容
	 */
	const cleanMessage = assistantMessage.replace(/```markdown([\s\S]*?)```/gi, "$1").replace(/```markdown/gi, "");
	/**
	 *  获取最后一条助手消息的内容
	 */
	const lastMessage = EntryAPI.OnlyData.historyMessage[EntryAPI.OnlyData.historyMessage.length - 1];
	// 更新最后一条助手消息的内容
	lastMessage.content = cleanMessage;
	/**
	 * 获取最后一条助手消息的元素
	 *
	 * 通过查找离 contentElement 最近的 .message 类元素来定位消息元素
	 */
	const messageElement = contentElement.closest('.message');
	// 若消息元素不存在，则说明没有合适的消息需要处理，直接返回
	if (!messageElement) return;
	/**
	 * 在消息元素中查找 .markdown-content 类元素作为内容容器
	 */
	const contentContainer = messageElement.querySelector('.markdown-content');
	// 若消息内容容器不存在，则无法进行内容渲染，直接返回
	if (!contentContainer) return;
	// 重新处理内容，将处理后的消息内容通过 processThinkTags 函数处理后插入到内容容器中
	contentContainer.innerHTML = EntryAPI.processThinkTags(cleanMessage);
	// 重新绑定思考标签的切换事件，确保交互功能正常
	messageElement.querySelectorAll(".toggle_think_button").forEach(EntryAPI.bindFoldingButton);
	// 重新绑定代码块的语法高亮事件，对消息中的代码块进行高亮显示
	messageElement.querySelectorAll('pre code').forEach((block: any) => (window as any).hljs.highlightElement(block));
	// 重新渲染集合渲染相关内容，对消息中的集合渲染代码块进行渲染
	generateCollectionRendering(contentContainer);
	// 重新绑定操作按钮事件，确保消息的操作按钮功能正常
	EntryAPI.bindMessageActionEvents(messageElement, lastMessage);
	// 滚动到消息底部，确保用户能够看到最新的消息
	EntryAPI.chatHistoryPanel?.scrollTo({ top: EntryAPI.chatHistoryPanel.scrollHeight, behavior: 'smooth' });
}

/**
 * 渲染页面占位符消息
 *
 * @param {HTMLElement} container - 占位符消息要渲染到的内容元素
 *
 * @returns {Promise<void>} 该函数不返回任何值
 */
export async function renderingPagePlaceholders(container: HTMLElement): Promise<void> {
	/** 加载随机的占位符图片 */
	const imageUrl = `/read/resources/placeholder/blank-0${EntryAPI.RandomFloor(0, 3)}.png`;
	/** 创建图片消息对象 */
	const imageMessage = EntryAPI.createImageMessage('assistant', '', imageUrl);
	// 渲染占位符图片到内容元素
	addImageRendering(imageMessage, container);
};

/**
 * 加载网页渲染内容
 *
 * @param {string} url - 要加载的网页 URL
 *
 * @param {HTMLElement} container - 用于渲染消息元素的容器元素
 *
 * @returns {Promise<void>}
 */
async function loadWebpageRendering(url: string, container: HTMLElement): Promise<void> {
	/**
	 * 获取演示消息
	 */
	const html = await EntryAPI.fetchMarkdown(url);
	/**
	 * 创建一个用户消息对象，用于渲染消息元素
	 */
	const userMsgObj = await EntryAPI.createMessageObject("user", '', false, true);
	/**
	 * 渲染消息元素
	 */
	const messageElement = renderMessage(userMsgObj, container);
	/**
	 * 创建一个 iframe 元素，用于显示代码块内容
	 */
	const iframe = document.createElement('iframe');
	// 设置 iframe 的样式，使其填满父元素
	iframe.style.cssText = 'width:100%; height:100%; border:0';
	// 将代码块的文本内容设置为 iframe 的文档内容
	iframe.srcdoc = html;
	// 设置 iframe 的沙箱属性，允许脚本执行
	iframe.setAttribute('sandbox', 'allow-modals allow-forms allow-popups allow-scripts');
	// 清空消息元素的内容
	messageElement.innerHTML = '';
	// 将 iframe 添加到消息元素中
	messageElement.appendChild(iframe);
	// 设置消息元素的高度为 100%
	messageElement.style.height = '100%';
	// 设置消息元素的最小高度
	messageElement.style.minHeight = 'calc(100vh - 500px)';
};

/**
 * 随机选择一个边框颜色
 *
 * @returns {string} 随机选择的边框颜色变量名
 */
function randomColorStyle(): string {
	/** 定义边框颜色数组 */
	const colors = [
		'--status-218838',
		'--status-3a5a8a',
		'--status-4a6fa5',
		'--status-6c9bcf',
		'--status-8a2be2',
		'--status-9d6bff',
		'--status-dc3545',
		'--status-fbbf24',
		'--status-ffc107',
		'--status-20c997',
		'--status-ff6b9c',
	];
	/** 随机选择一个边框颜色 */
	const randomColor = colors[EntryAPI.RandomFloor(0, colors.length - 1)];
	return randomColor;
}

/**
 * 添加图片渲染
 *
 * @param {EntryAPI.HistoryMessage} message - 包含图片 URL 和可选文本内容的消息对象
 *
 * @param {HTMLElement} container - 用于渲染消息元素的容器元素，默认值为 EntryAPI.chatHistoryPanel
 */
export async function addImageRendering(message: EntryAPI.HistoryMessage, container: HTMLElement = EntryAPI.chatHistoryPanel) {
	/**
	 * 创建消息的根元素
	 */
	const messageElement = document.createElement("div") as HTMLDivElement;
	// 为消息根元素添加基础类名
	messageElement.classList.add("message");
	// 如果是用户消息，则添加用户消息类名
	if (message.role === "user") messageElement.classList.add("user-message");
	// 否则为助手消息，添加助手消息类名
	else messageElement.classList.add("assistant-message");
	/** 定义图片类 */
	const imageClass = [
		`src="${message.imageUrl.trim()}"`,
		`alt="${message.content.trim() || '本地图片'}"`,
		`class="image-just-drawn"`,
		`id="${message.imageUrl.replace(/\\/g, '/').split('/').pop().split('.')[0].trim()}"`,
		`style="border-color: var(${randomColorStyle()});"`,
		`onerror="this.onerror=null; this.src='/read/resources/placeholder/video_file_icon-0${Math.floor(Math.random() * 5)}.png'"`,
		`onclick="previewImage('${message.imageUrl.replace(/\\/g, '/')}', '${message.content.trim() || '本地图片'}')"`,
	].join(" ");
	// 设置消息元素的初始 HTML 结构
	messageElement.innerHTML = [
		// 消息头
		'<div class="message-header">',
		`<span>${message.role === "user" ? EntryAPI.OnlyData.customConfig.userName || "你" : "月华"}</span>`,
		'</div>',
		// 消息图片内容
		`<div class="labeled-image-container">`,
		`<img ${imageClass}>`,
		'</div>',
		// 消息操作面板（默认顶部对齐）
		'<div class="message-actions-panel top-align">',
		// 删除消息按钮
		'<button class="chat-action-button delete_message_button" title="删除图片">',
		'<i class="fas fa-trash"></i>',
		'</button>',
		'</div>',
	].join("");
	/** 查找图片容器元素 */
	const imageContainer = messageElement.querySelector(".labeled-image-container") as HTMLElement;
	// 如果消息内容存在，设置图片容器的标签文本为消息内容
	if (message.content.trim()) imageContainer.style.setProperty('--image-label', `"${message.content.trim()}"`);
	// 否则，设置图片容器的标签文本为默认值 "图片文件"
	else imageContainer.style.setProperty('--image-label', `"图片文件"`);
	// 将消息元素添加到容器中
	container.appendChild(messageElement);
	// 绑定消息操作事件
	EntryAPI.bindMessageActionEvents(messageElement, message);
	// 滚动容器到最底部，确保新消息可见
	setTimeout(() => container.scrollTo({ top: container.scrollHeight, behavior: 'smooth' }), 1000);
	// 如果图片 URL 是视频格式，执行视频关键帧提取
	if (EntryAPI.OnlyData.videoFormatsExtensions.some(format => message.imageUrl.toLowerCase().endsWith(format))) {
		// 执行获取视频关键帧
		await EntryAPI.loadVideoCoverFrame(message.imageUrl);
	}
}