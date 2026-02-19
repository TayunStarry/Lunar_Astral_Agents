import * as EntryAPI from '../EntryAPI/code';

/** 记录上一次显示预设消息的时间戳，初始值为 0 */
let lastPresetMessageTime = 0;

/** 防抖延迟时间（毫秒），用于控制防抖函数的触发间隔 */
const debounceDelay = 200;

/** 用于存储窗口大小调整时的定时器 ID，用于防抖操作 */
let resizeTimerId: NodeJS.Timeout | null = null;

/** 小屏幕宽度阈值 */
export const smallScreenWidthThreshold = 475;

/**
 * 判断传入的 URL 对象是否为 localhost 格式的地址
 *
 * @param {URL} url - 需要判断的 URL 对象
 *
 * @returns {boolean} - 如果是 localhost 格式的 URL 则返回 true，否则返回 false
 */
function isLocalhostUrl(url: URL): boolean {
	// 验证 URL 协议是否为 HTTP 或 HTTPS，非这两种协议的 URL 直接判定不是 localhost 格式
	if (url.protocol !== 'http:' && url.protocol !== 'https:') return false;
	// 验证主机名是否为 'localhost' 或 '127.0.0.1'，不是则判定不是 localhost 格式
	if (url.hostname !== 'localhost' && url.hostname !== '127.0.0.1') return false;
	// 若 URL 包含端口号，验证端口号是否为有效数字，非有效数字则判定不是 localhost 格式
	if (url.port && isNaN(parseInt(url.port))) return false;
	// 若上述验证都通过，则判定为 localhost 格式的 URL
	return true;
};

/**
 * 转换URL的函数
 *
 * @returns {string|Promise<string>} - 转换后的URL字符串
 */
async function convertUrl(toImage: boolean = false): Promise<string> {
	/** 检查当前URL是否为localhost格式 */
	const isLocalhost = isLocalhostUrl(new URL(window.location.href));
	// 如果是localhost格式且不是图片请求，则直接返回/v1
	if (isLocalhost && !toImage) return '/v1';
	/** 从当前网址中提取主机名和端口号 */
	const baseURL = window.location.origin;
	// 如果是图片请求且当前URL是HTTPS协议，则需要转换为HTTP协议
	if (toImage && window.location.href.startsWith('https')) {
		/** 从当前网址中提取主机名和端口号 */
		const url = new URL(window.location.href)
		/** 从当前URL中提取端口号的数字类型并增加进行偏移 */
		const newPort = Number(url.port) + 5
		/** 构建新的HTTP URL字符串 */
		const newUrl = 'http://' + url.hostname + ':' + newPort
		// 返回新的HTTP URL字符串
		return newUrl;
	}
	// 如果是图片请求且当前URL不是HTTPS协议，则直接返回原始URL
	else if (toImage) return baseURL;
	// 如果不是图片请求，则返回默认的/v1路径
	return baseURL + '/v1';
};

/**
 * 处理键盘按下事件，禁用特定的快捷键组合，当触发这些快捷键时阻止默认行为并显示预设消息。
 *
 * @param {KeyboardEvent} event - 键盘按下事件对象。
 */
function toPresetMessage(event: KeyboardEvent) {
	// 禁用 Ctrl+S / Cmd+S 保存快捷键，触发时显示预设消息
	if ((event.ctrlKey || event.metaKey) && event.key === 's') {
		event.preventDefault();
		presetMessage();
	}
	// 禁用 Ctrl+Shift+S (另存为) 快捷键，触发时显示预设消息
	if ((event.ctrlKey || event.metaKey) && event.shiftKey && event.key === 'S') {
		event.preventDefault();
		presetMessage();
	}
	// 禁用打印快捷键 Ctrl+P，触发时显示预设消息
	if ((event.ctrlKey || event.metaKey) && event.key === 'p') {
		event.preventDefault();
		presetMessage();
	}
	// 禁用 F12 键（仅在非调试模式下）
	if (event.key === 'F12' && !EntryAPI.OnlyData.isDebugMode) {
		event.preventDefault();
	}
};

/**
 * 显示预设消息的异步函数。
 *
 * 获取预设的 Markdown 消息并显示，若开启语音播放则播放语音，最后打开指定链接。
 */
async function presetMessage() {
	/** 获取当前时间戳 */
	const now = Date.now();
	// 若距离上一次显示预设消息的时间小于 30000 毫秒（30 秒），则不执行后续操作，直接返回
	if (now - lastPresetMessageTime < 30000) return;
	// 更新上一次显示预设消息的时间戳为当前时间
	lastPresetMessageTime = now;
	/** 获取预设的 Markdown 消息内容 */
	const markdown = await EntryAPI.fetchMarkdown('/read/resources/prompts/prohibitMessage.md');
	// 将预设消息添加到聊天历史记录并渲染到界面上
	EntryAPI.renderMessage(await EntryAPI.createMessageObject("assistant", markdown, false), EntryAPI.chatHistoryPanel);
	// 若开启了自动语音播放功能，则播放预设消息的语音
	if (EntryAPI.OnlyData.autoPlaySpeech) EntryAPI.playSpeechModel(markdown);
	// 3 秒后在新窗口中打开指定链接
	setTimeout(() => window.open('https://gitee.com/TayunStarry/Lunar-Astral-Agents', '_blank'), 3000);
};

/**
 * 系统初始化事件函数，负责执行一系列系统初始化操作
 */
async function systemInitializationEvent() {
	// 获取 Live2D 相关设置
	EntryAPI.fetchLive2DSetting();
	// 创建 Live2D 状态选择器
	createLive2dStateSelect();
	// 延迟 250 毫秒后初始化 Live2D
	setTimeout(EntryAPI.initLive2D, 250);
	// 异步加载自定义配置文件
	EntryAPI.OnlyData.customConfig = await EntryAPI.fetchDocumentCallback('resources/custom_config.json');
	// 异步加载系统提示词
	EntryAPI.OnlyData.systemPrompt = await EntryAPI.fetchMarkdown('/read/resources/prompts/systemPrompt.md');
	// 异步加载图片描述提示词
	EntryAPI.OnlyData.imagePrompt = await EntryAPI.fetchMarkdown('/read/resources/prompts/imagePrompt.md');
	// 异步加载视频描述提示词
	EntryAPI.OnlyData.videoPrompt = await EntryAPI.fetchMarkdown('/read/resources/prompts/videoPrompt.md');
	// 异步加载视频总结提示词
	EntryAPI.OnlyData.videoSummaryPrompt = await EntryAPI.fetchMarkdown('/read/resources/prompts/videoSummaryPrompt.md');
	// 异步获取文件服务 API 端点
	EntryAPI.OnlyData.fileServiceUrl = await convertUrl(true);
	// 异步获取系统URL
	EntryAPI.OnlyData.systemUrl = await convertUrl();
	// 查找并注册工具
	EntryAPI.EnableLunarToolPackageProtocol();
	// 将连续记忆合并到历史记录中
	EntryAPI.OnlyData.historyMessage.push(...await EntryAPI.captureKnowledgeList('knowledge/continuous_memory.json'));
	// 应用保存的主题样式
	applySavedTheme();
	// 加载语言设置
	loadLanguage();
	// 加载系统语音模型
	EntryAPI.loadSystemSpeechModel();
	// 触发窗口大小调整事件
	windowResizeEvent();
	// 初始化 mermaid 图表
	(window as any).mermaid.initialize(mermaidParameter);
	// 绑定滑块事件
	EntryAPI.bindSlider();
	// 初始化自动调整大小的文本区域
	EntryAPI.initAutoResizeTextareas();
	// 设置动态透明度
	dynamicOpacity();
	// 异步加载演示消息
	await loadDemoMessage();
	// 渲染简单渲染面板占位符
	await EntryAPI.renderingPagePlaceholders(EntryAPI.simpleRenderingPanel)
	// 显示对话继续提示
	showDialogueContinuation(EntryAPI.OnlyData.historyMessage.length);
};

/**
 * 显示对话继续提示的异步函数。
 *
 * @param {number} length - 之前的对话条数。
 */
async function showDialogueContinuation(length: number) {
	// 如果之前没有对话，则不显示继续提示
	if (length === 0) return;
	// 显示系统消息, 提示用户可以继续对话
	//EntryAPI.showSystemMessage('月华还记得之前聊过的内容哦', 'success');
	// 渲染最近 5 条消息到聊天历史面板
	EntryAPI.renderAllMessages(EntryAPI.chatHistoryPanel, false, EntryAPI.OnlyData.historyMessage.slice(-5));
	// 等待 4 秒，确保消息渲染完成
	await new Promise(resolve => setTimeout(resolve, 3500));
	/** 创建助手消息对象 */
	const assistantMsgObj = await EntryAPI.createMessageObject("assistant", '', false);
	// 为助手消息对象设置内容，包含之前的对话条数
	assistantMsgObj.content = `**之前聊过的${length}条对话, 月华还记着呢**`;
	// 创建消息元素并渲染
	EntryAPI.renderMessage(assistantMsgObj, EntryAPI.chatHistoryPanel);
};

/**
 * 加载示例消息
 */
async function loadDemoMessage() {
	/** 获取演示消息 */
	const markdown = await EntryAPI.fetchMarkdown('/read/resources/prompts/demoMessage.md');
	/** 创建助手消息对象 */
	const assistantMsgObj = await EntryAPI.createMessageObject("assistant", '', false);
	// 为助手消息对象设置内容为演示消息
	assistantMsgObj.content = markdown;
	/** 创建消息元素并渲染 */
	let messageElement = EntryAPI.renderMessage(assistantMsgObj, EntryAPI.chatHistoryPanel);
	// 为think区块添加折叠功能
	(messageElement?.querySelectorAll(".toggle_think_button") as NodeListOf<HTMLButtonElement>).forEach(EntryAPI.bindFoldingButton);
};

/**
 * 加载语言相关设置，包括代码高亮配置
 * 对页面中的代码进行高亮处理，并注册自定义的代码高亮语言规则
 */
function loadLanguage() {
	// 对页面中所有符合条件的代码块进行高亮处理
	(window as any).hljs.highlightAll();
	// 注册 mermaid 语言的高亮规则
	(window as any).hljs.registerLanguage('mermaid', () => mermaidHighlight());
	// 注册 echarts 语言的高亮规则，使用 json 语言的高亮规则
	(window as any).hljs.registerLanguage('echarts', () => (window as any).hljs.getLanguage('json'));
	// 注册 powershell 语言的高亮规则，使用 python 语言的高亮规则
	(window as any).hljs.registerLanguage('powershell', () => (window as any).hljs.getLanguage('python'));
};

/**
 * mermaid语言 高亮规则
 */
function mermaidHighlight() {
	return {
		name: 'Mermaid',
		aliases: ['mmd'], // 可选别名
		contains: [
			{
				className: 'keyword',
				begin: '\\b(flowchart|graph|pie|gantt|sequenceDiagram|classDiagram|stateDiagram|erDiagram|journey|gitGraph|subgraph|end|click)\\b',
				relevance: 10
			},
			{
				className: 'title',
				begin: 'title\\s+["\']?',
				end: '["\']?|$',
				excludeBegin: true
			},
			{
				className: 'symbol',
				begin: /[+\-*/%&|=<>^~]|\.\.|\-\-|\|\|/ // 扩展操作符支持
			},
			{
				className: 'comment',
				begin: '%%.*',
				end: '$',
				relevance: 0
			},
			{
				className: 'string',
				begin: /"[^"]*"/,
				end: /[^\\]"/
			},
			{
				className: 'number',
				begin: '\\b\\d+(\\.\\d+)?\\b'
			}
		]
	}
};

/**
 * 定义 mermaid 图表库的初始化参数
 *
 * 该对象包含了图表的基本配置、主题设置、安全级别以及流程图的特定配置
 */
const mermaidParameter = {
	// 页面加载时自动渲染图表
	startOnLoad: true,
	// 根据当前页面主题选择 mermaid 图表主题，若为深色模式则使用深色主题，否则使用默认主题
	theme: document.body.classList.contains("dark-mode") ? "dark" : "default",
	// 设置安全级别为宽松，允许更灵活的渲染配置
	securityLevel: "loose",
	// 使用继承的字体，保持与页面整体字体一致
	fontFamily: "inherit",
	// 流程图配置，包含流程图的布局、样式等相关设置
	flowchart: {
		// 流程图方向为从左到右
		rankDir: 'LR',
		// 使用最大宽度，使流程图充分利用可用空间
		useMaxWidth: true,
		// 曲线类型为阶梯状
		curve: 'stepAfter',
		// 禁用 HTML 标签，防止 XSS 攻击并保持渲染一致性
		htmlLabels: false,
		// 图表内边距为 0，减少不必要的空白
		diagramPadding: 0,
		// 默认渲染器为 canvas，使用 canvas 进行图表渲染
		defaultRenderer: 'canvas',
	}
};

/**
 * 动态透明度效果函数, 按钮距离鼠标指针越远，透明度越低，距离鼠标指针越近，透明度越高。
 *
 * 该函数会在页面加载完成后立即执行，并且会监听鼠标移动事件，实时更新按钮的透明度。
 */
function dynamicOpacity() {
	/**
	 * 按钮事件处理函数
	 *
	 * @param {Element} button 按钮元素
	 *
	 * @param {MouseEvent} event 鼠标事件对象
	 */
	function buttonEvent(button: Element, event: MouseEvent) {
		/** 获取按钮的位置信息 */
		const buttonRect = button.getBoundingClientRect();
		/** 按钮中心的水平与垂直坐标 */
		const [buttonCenterX, buttonCenterY] = [buttonRect.left + buttonRect.width / 2, buttonRect.top + buttonRect.height / 2]
		/** 鼠标指针到按钮中心的水平与垂直距离 */
		const [distanceX, distanceY] = [event.clientX - buttonCenterX, event.clientY - buttonCenterY];
		/** 鼠标指针到按钮中心的距离 */
		const distance = Math.sqrt(distanceX * distanceX + distanceY * distanceY);
		/** 计算透明度：距离越远透明度越低（可根据需要调整最大影响距离） */
		const opacity = Math.max(0, Math.min(1, 1 - (distance / 300)));
		// 应用透明度到按钮
		(button as HTMLElement).style.opacity = opacity.toString();
	};
	/**
	 * 检查设备是否为触摸设备
	 *
	 * @returns {boolean} 如果设备支持触摸事件，则返回 true；否则返回 false
	 */
	function isTouchDevice(): boolean {
		return ('ontouchstart' in window) || (navigator.maxTouchPoints > 0);
	};
	/**
	 * 鼠标移动事件处理函数
	 *
	 * @param {MouseEvent} event 鼠标事件对象
	 */
	function handleMouseMove(event: MouseEvent) {
		// 遍历所有按钮
		document.querySelectorAll('.power-button.live2d').forEach(button => buttonEvent(button, event));
		document.querySelectorAll('.message-actions-panel').forEach(button => buttonEvent(button, event));
	};
	// 如果当前设备为触控设备（如手机、平板）
	if (isTouchDevice()) {
		// 直接显示所有 Live2D 电源按钮，避免鼠标悬停才显示
		document.querySelectorAll('.power-button.live2d').forEach(button => (button as HTMLElement).style.opacity = '1');
		// 直接显示所有消息操作面板（如删除、复制按钮），方便触控操作
		document.querySelectorAll('.message-actions-panel').forEach(button => (button as HTMLElement).style.opacity = '1');
	}
	// 非触控设备（桌面端）则监听鼠标移动事件，由 handleMouseMove 控制按钮显隐
	else document.addEventListener('mousemove', handleMouseMove);
};

/**
 * 窗口大小调整事件处理函数
 *
 * 当窗口宽度小于等于 SMALL_SCREEN_WIDTH_THRESHOLD 时，执行一系列界面布局调整操作
 */
function windowResizeEvent() {
	// 检查窗口宽度是否大于小屏幕阈值，若是则直接返回，不执行后续操作
	if (window.innerWidth > smallScreenWidthThreshold) {
		// 隐藏 Live2D 输入面板
		EntryAPI.live2dInputPanel.style.display = "none";
		/** 捕获所有配置面板 */
		const configurePanels = document.querySelectorAll('.configure_panel') as NodeListOf<HTMLElement>;
		/** 检查所有配置面板是否都为隐藏状态 */
		const allHidden = Array.from(configurePanels).every(panel => panel.style.display === 'none');
		// 若所有配置面板都为隐藏状态，则显示聊天历史容器面板
		if (allHidden) EntryAPI.chatHistoryContainerPanel.style.display = "flex";
		return;
	};
	// 关闭调试模式
	EntryAPI.OnlyData.isDebugMode = false;
	// 清除所有配置面板
	EntryAPI.eraseAllConfigurePanel();
	// 关闭配置面板选项
	EntryAPI.OnlyData.configurePanelOption = "none";
	// 显示 Live2D 输入面板
	EntryAPI.live2dInputPanel.style.display = "flex";
	// 隐藏聊天历史容器面板
	EntryAPI.chatHistoryContainerPanel.style.display = "none";
	// 移除调试模式切换按钮的点击状态样式
	EntryAPI.debugModeButton.classList.remove("clicking");
	// 重置调试模式切换按钮的图标
	EntryAPI.debugModeButton.innerHTML = '<i class="fas fa-star-and-crescent"></i> 启用 调试模式';
};

/**
 * 窗口大小变化时的防抖处理函数
 * 清除之前的定时器，避免窗口大小频繁变化时重复触发事件，设置新的防抖定时器执行窗口大小调整逻辑
 */
function resizeEvent() {
	// 清除之前设置的定时器，防止重复触发窗口大小调整事件，保证防抖效果
	clearTimeout(resizeTimerId);
	// 设置防抖定时器，在 DEBOUNCE_DELAY 毫秒无窗口大小变化后，执行窗口大小调整事件处理函数
	resizeTimerId = setTimeout(() => windowResizeEvent(), debounceDelay);
};

/**
 * 创建 Live2D 状态选择下拉框的选项
 *
 * 此函数会从 EmotionalState 中获取所有大写的 getter 方法，
 *
 * 并为每个 getter 创建一个 option 元素添加到 live2dStateDropdown 下拉选择框中
 */
function createLive2dStateSelect() {
	/** 从 EmotionalState 获取所有大写的 getter 方法 */
	const allUppercaseGetters = EntryAPI.EmotionalState.getAllUppercaseGetters();
	// 遍历所有大写的 getter 方法
	allUppercaseGetters.forEach(
		getter => {
			/** 创建一个新的 option 元素 */
			const option = document.createElement("option");
			// 设置 option 的值为模型的 id，通过 EmotionalState 的 getter 获取
			option.value = EntryAPI.EmotionalState[getter] as string;
			// 设置 option 显示的文本为模型的对应名称，通过双重索引 EmotionalState 获取
			option.textContent = EntryAPI.EmotionalState[EntryAPI.EmotionalState[getter] as string] as string;
			// 将创建的 option 元素添加到 live2dStateDropdown 下拉选择框中
			EntryAPI.live2dStateDropdown.appendChild(option);
		}
	);
};

/**
 * 应用已保存的主题
 * 从本地存储中获取之前保存的主题，若为暗色模式则应用相应样式，若为亮色模式则移除暗色模式类名
 */
function applySavedTheme() {
	/** 从本地存储中获取已保存的主题 */
	const savedTheme = localStorage.getItem("theme");
	// 如果之前保存的是暗色模式，则应用相应样式
	if (savedTheme === "dark") {
		// 添加点击中的样式类
		EntryAPI.themeButton?.classList.add("clicking");
		// 添加暗色模式类名以启用暗色主题样式
		document.documentElement.classList.add("dark-mode");
		// 修改按钮图标为太阳图标（表示当前为暗色模式）
		if (EntryAPI.themeButton) EntryAPI.themeButton.innerHTML = '<i class="fas fa-sun"></i>';
	}
};

//* 绑定 系统初始化事件
document.addEventListener("DOMContentLoaded", systemInitializationEvent);
//* 绑定 窗口大小调整事件
document.addEventListener("DOMContentLoaded", windowResizeEvent);
//* 绑定 窗口大小改变事件
window.addEventListener('resize', () => resizeEvent());
//* 添加 beforeunload 事件监听器，在页面即将卸载时取消所有延迟执行的任务，防止页面卸载后仍有未完成的定时任务
window.addEventListener('beforeunload', () => EntryAPI.DelayExecutionManager.cancelAll());
//* 页面加载完成后生成二维码
window.addEventListener('load', () => EntryAPI.generateQRCode(document.getElementById('qrcodePanel') as HTMLElement));
//* 监听键盘按下被禁用的快捷键组合时，阻止默认行为并调用预设消息处理函数。
document.addEventListener('keydown', event => toPresetMessage(event));
//* 禁用鼠标右键菜单
document.addEventListener('contextmenu', event => event.preventDefault());