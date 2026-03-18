import * as EntryAPI from '../EntryAPI/code';

/**
 * 绑定消息操作按钮事件
 *
 * @param {HTMLElement} messageElement - 消息元素
 *
 * @param {Object} message - 消息对象
 */
export function bindMessageActionEvents(messageElement: HTMLElement, message: EntryAPI.HistoryMessage) {
	/**
	 * 获取复制按钮
	 */
	const copyButton = messageElement.querySelector(".copy_message_button");
	// 绑定复制功能
	copyButton?.addEventListener("click",
		() => {
			// 创建临时 textarea
			const textArea = document.createElement('textarea');
			try {
				textArea.value = message.content;

				// 设置样式防止页面跳动
				textArea.style.position = 'fixed';
				textArea.style.top = '-9999px';
				textArea.style.left = '-9999px';

				document.body.appendChild(textArea);
				textArea.select();
				textArea.setSelectionRange(0, 99999); // 移动设备兼容
				// 复制文本
				document.execCommand('copy');

				showCopySuccess(copyButton);
			}
			catch (error) {
				if (error instanceof Error)
					// 捕获异常并显示错误信息
					EntryAPI.showSystemMessage(`${error.name} | ${error.message} | ${error.stack}`, "error");
			}
			finally {
				// 清理 DOM
				document.body.removeChild(textArea);
			}
		}
	);
	// 显示复制成功状态
	function showCopySuccess(button: Element) {
		const originalIcon = button.innerHTML;
		button.innerHTML = '<i class="fas fa-check"></i>';
		setTimeout(() => { button.innerHTML = originalIcon; }, 2000);
	};
	/** 在消息元素中查找删除按钮 */
	const deleteButton = messageElement.querySelector(".delete_message_button");
	// 绑定删除功能
	deleteButton?.addEventListener("click",
		async () => {
			/** 创建对话历史副本，避免直接修改原数组 */
			const temporaryHistory = EntryAPI.OnlyData.historyMessage.slice();
			// 清空原对话历史
			EntryAPI.OnlyData.historyMessage = [];
			// 过滤掉被删除消息后的对话历史重新赋值
			EntryAPI.OnlyData.historyMessage.push(...temporaryHistory.filter(msg => msg.uuid !== message.uuid));
			// 发送删除请求到服务器
			EntryAPI.batchProcessingKnowledgeDelete('knowledge/continuous_memory.json', [message.uuid]);
			EntryAPI.batchProcessingKnowledgeDelete('knowledge/lunar_notes.json', [message.uuid]);
			// 中止可能正在进行的请求
			EntryAPI.OnlyData.abortController?.abort();
			// 从DOM中移除对应消息元素
			messageElement.remove();
		}
	);
	/** 在消息元素中查找播放按钮 */
	const playButton = messageElement.querySelector(".play_speech_button");
	// 绑定播放事件
	playButton?.addEventListener("click",
		() => {
			/** 从消息内容中提取结论部分，用于文本转语音 */
			const content = EntryAPI.cleanTextForTTS(EntryAPI.extractConclusion(message.content));
			// 播放TTS
			EntryAPI.playSpeechModel(content);
		}
	);
};

/**
 * 绑定折叠思考区按钮
 *
 * @param {HTMLButtonElement} button 折叠按钮
 */
export function bindFoldingButton(button: HTMLButtonElement) {
	// 绑定点击事件
	button.addEventListener("click",
		() => {
			/** 在消息元素中查找 think 区块内容元素 */
			const thinkContent = button.closest(".think-block")?.querySelector(".think-content");
			// 切换内容
			thinkContent?.classList.toggle("collapsed");
			// 切换按钮图标
			if (thinkContent?.classList.contains("collapsed")) button.innerHTML = '<i class="fas fa-angle-down"></i>';
			else button.innerHTML = '<i class="fas fa-angle-up"></i>';
		}
	);
	/** 在消息元素中查找 think 区块内容元素 */
	const thinkContent = button.closest(".think-block")?.querySelector(".think-content");
	// 折叠think内容
	thinkContent?.classList.toggle("collapsed");
};

/**
 * 创建并配置停止生成按钮，将其添加到指定消息元素的操作面板中，
 *
 * 同时绑定点击事件以支持中止当前的API请求。
 *
 * @param {HTMLElement} messageElement - 消息元素，用于查找消息操作面板并添加停止按钮
 */
export function createStopButton(messageElement: HTMLElement) {
	/** 创建停止生成按钮元素，用于在用户需要时中止API请求 */
	const stopButton = document.createElement("button");
	// 为停止按钮添加样式类，方便进行样式控制
	stopButton.className = "chat-action-button stop_generation_button";
	// 设置停止按钮的鼠标悬停提示，告知用户该按钮的功能
	stopButton.title = "停止生成";
	// 设置停止按钮的内部 HTML，显示停止图标
	stopButton.innerHTML = '<i class="fas fa-stop"></i>';
	/** 在消息元素中查找消息操作面板，该面板用于放置操作按钮 */
	const messageActionsPanel = messageElement.querySelector('.message-actions-panel');
	// 如果找到消息操作面板，则将停止按钮添加到该面板中
	if (messageActionsPanel) messageActionsPanel.appendChild(stopButton);
	// 绑定停止按钮事件，用于在用户点击时中止当前的API请求
	stopButton.addEventListener('click',
		() => {
			// 若中止控制器不存在，则不执行后续操作
			if (!EntryAPI.OnlyData.abortController) return;
			// 调用中止控制器的abort方法，中止当前正在进行的API请求
			EntryAPI.OnlyData.abortController.abort();
			// 禁用停止按钮，防止用户重复点击导致意外行为
			stopButton.disabled = true;
			// 更改停止按钮的图标为禁止图标，直观提示用户请求已中止
			stopButton.innerHTML = '<i class="fas fa-ban"></i>';
		}
	);
};

/**
 * 绑定代码执行按钮事件
 *
 * @param {HTMLElement} container - 包含代码块的容器元素
 */
export function bindCodeExecuteButtons(container: HTMLElement) {
	/** 为单个代码块创建并绑定执行按钮 */
	function createBinding(codeBlock: HTMLPreElement) {
		/** 创建代码执行按钮 */
		const executeButton = document.createElement("button");
		// 设置按钮的类名，方便样式控制
		executeButton.className = "execute-code-button";
		// 设置按钮的内部 HTML，显示播放图标
		executeButton.innerHTML = '<i class="fas fa-play"></i>';
		// 设置按钮的鼠标悬停提示，告知用户该按钮的功能
		executeButton.title = "执行";
		/** 创建并渲染一个新的页面，将代码块内容嵌入到 iframe 中显示 */
		async function createPageRender() {
			// 清空简单渲染面板的内容，准备新的渲染
			EntryAPI.simpleRenderingPanel.innerHTML = '';
			/** 渲染消息元素 */
			const messageElement = EntryAPI.renderMessage(await EntryAPI.createMessageObject("user", '', false, true, false, null, null), EntryAPI.simpleRenderingPanel);
			/** 创建一个 iframe 元素，用于显示代码块内容 */
			const iframe = document.createElement('iframe') as HTMLIFrameElement;
			// 设置 iframe 的样式，使其填满父元素
			iframe.style.cssText = 'width:100%; height:100%; border:0';
			// 将代码块的文本内容设置为 iframe 的文档内容
			iframe.srcdoc = codeBlock.textContent || '';
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
			// 设置配置面板选项
			EntryAPI.OnlyData.configurePanelOption = 'simpleRenderingButton';
			// 清除所有配置面板
			EntryAPI.eraseAllConfigurePanel();
			// 显示轻量渲染容器面板
			EntryAPI.simpleRenderingContainerPanel.style.display = "flex";
		};
		// 为执行按钮绑定点击事件，点击时调用 createPageRender 函数
		executeButton.addEventListener("click", () => createPageRender());
		// 若代码块的定位方式不是 relative 或 absolute，则将其设置为 relative
		if (codeBlock.style.position !== "relative" && codeBlock.style.position !== "absolute") {
			codeBlock.style.position = "relative";
		};
		// 将执行按钮添加到代码块中
		codeBlock.appendChild(executeButton);
	}
	// 遍历容器内所有指定类名的代码块，并为其创建绑定
	(container.querySelectorAll(".language-html.hljs.language-xml") as NodeListOf<HTMLPreElement>).forEach(codeBlock => createBinding(codeBlock));
};

/**
 * 绑定聊天发送
 *
 * @param {KeyboardEvent} event - 按键事件
 */
export function bindChatSend(event: { key: string; ctrlKey: any; shiftKey: any; preventDefault: () => void; }) {
	// 判断是否正在生成
	if (EntryAPI.getReleaseButtonsDisabledState()) return;
	// 仅处理 Enter 键
	if (event.key !== "Enter") return;
	// Ctrl + Enter: 允许默认换行行为
	if (event.ctrlKey) return;
	// Shift + Enter: 允许默认换行行为
	if (event.shiftKey) return;
	// 禁用回车行为
	event.preventDefault();
	// 发送消息
	EntryAPI.sendChatMessageToBackendModel();
};

// 角色互动模式 聊天发送事件
EntryAPI.live2dWriteArea.addEventListener("keypress", bindChatSend);
// 常规聊天模式 聊天发送事件
EntryAPI.chatWriteArea.addEventListener("keypress", bindChatSend);
