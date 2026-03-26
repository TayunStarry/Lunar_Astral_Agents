/**
 * 主容器面板元素
 */
export const mainContainerPanel = document.getElementById("mainContainerPanel") as HTMLElement;
/**
 * 聊天历史记录容器面板元素
 */
export const chatHistoryContainerPanel = document.getElementById("chatHistoryContainerPanel") as HTMLElement;
/**
 * 语音配置容器面板元素
 */
export const speechConfigContainerPanel = document.getElementById("speechConfigContainerPanel") as HTMLElement;
/**
 * 聊天历史记录面板元素
 */
export const chatHistoryPanel = document.getElementById("chatHistoryPanel") as HTMLElement;
/**
 * Live2D 输入面板元素
 */
export const live2dInputPanel = document.getElementById("live2dInputPanel") as HTMLElement;
/**
 * Live2D 容器元素
 */
export const live2dContainer = document.getElementById("live2dContainer") as HTMLElement;
/**
 * 简单渲染面板元素
 */
export const simpleRenderingPanel = document.getElementById("simpleRenderingPanel") as HTMLElement;
/**
 * 月华笔记面板元素
 */
export const lunarNotesPanel = document.getElementById("lunarNotesPanel") as HTMLElement;
/**
 * 简单渲染容器面板元素
 */
export const simpleRenderingContainerPanel = document.getElementById("simpleRenderingContainerPanel") as HTMLElement;
/**
 * 视觉共享容器面板元素
 */
export const shareScreenContainerPanel = document.getElementById("shareScreenContainerPanel") as HTMLElement;
/**
 * 月华笔记容器面板元素
 */
export const lunarNotesContainerPanel = document.getElementById("lunarNotesContainerPanel") as HTMLElement;
/**
 * 功能控制容器面板元素
 */
export const functionControlContainerPanel = document.getElementById("functionControlContainerPanel") as HTMLElement;
/**
 * 系统语音引擎面板元素
 */
export const systemSpeechEnginePanel = document.getElementById("systemSpeechEnginePanel") as HTMLElement;
/**
 * 自定义语音引擎面板元素
 */
export const customSpeechEnginePanel = document.getElementById("customSpeechEnginePanel") as HTMLElement;
/**
 * 二维码显示区域
 */
export const qrcodeStatusPanel = document.getElementById("qrcodeStatusPanel") as HTMLElement;
/**
 * 模型回应计数器面板元素
 */
export const tokenCounterPanel = document.getElementById("tokenCounterPanel") as HTMLElement;
/**
 * 情感状态面板元素
 */
export const emotionStatusPanel = document.getElementById("emotionStatusPanel") as HTMLElement;
/**
 * Live2D 容器面板元素
 */
export const live2dContainerPanel = document.getElementById("live2dContainerPanel") as HTMLElement;
/**
 * 功能控制面板元素
 */
export const quickControlPanel = document.getElementById("quickControlPanel") as HTMLElement;

import * as EntryAPI from '../EntryAPI/code';

/**
 * 清除所有配置面板的显示状态，将所有配置面板隐藏，并移除配置面板按钮的点击样式，最后重载Live2D容器。
 */
export function eraseAllConfigurePanel() {
	/**
	 * 获取文档中所有的配置面板元素
	 */
	const configurePanel = document.documentElement.querySelectorAll('.configure_panel');
	/**
	 * 获取文档中所有的配置面板按钮元素
	 */
	const configurePanelButton = document.documentElement.querySelectorAll('.power-button.live2d');
	// 检查是否存在配置面板或配置面板按钮，若不存在则直接返回，避免不必要的操作
	if (configurePanel.length === 0 || configurePanelButton.length === 0) return;
	// 遍历所有配置面板，将其显示状态设置为隐藏
	configurePanel.forEach(panel => (panel as HTMLElement).style.display = 'none');
	// 遍历所有配置面板按钮，移除按钮上的点击中的样式类，恢复按钮初始样式
	//configurePanelButton.forEach(button => button.classList.remove("clicking"));
	// 调用 reloadLive2DContainer 函数，重载Live2D容器
	//setTimeout(EntryAPI.reloadLive2DContainer, 500);
};

/**
 * 实现元素拖动功能的函数
 *
 * @param {HTMLElement} targetElement - 需要实现拖动功能的目标元素
 */
export function dragElement(targetElement: HTMLElement) {
	/**
	 * 记录鼠标在 X 轴方向的移动差值
	 */
	let mouseXDelta = 0;
	/**
	 * 记录鼠标在 Y 轴方向的移动差值
	 */
	let mouseYDelta = 0;
	/**
	 * 记录鼠标按下时的初始 X 坐标
	 */
	let initialMouseX = 0;
	/**
	 * 记录鼠标按下时的初始 Y 坐标
	 */
	let initialMouseY = 0;
	/**
	 * 获取标题栏元素
	 */
	const headerElement = document.getElementById(targetElement.id + "-header");
	// 如果存在标题栏，仅允许通过标题栏拖动
	if (headerElement) headerElement.onmousedown = startDrag;
	// 否则允许通过整个元素拖动
	else targetElement.onmousedown = startDrag;
	/**
	 * 开始拖动元素的处理函数
	 *
	 * @param {MouseEvent} event - 鼠标事件对象
	 */
	function startDrag(event: MouseEvent) {
		// 阻止默认事件行为
		event.preventDefault();
		// 获取鼠标初始位置
		initialMouseX = event.clientX;
		initialMouseY = event.clientY;
		// 注册鼠标释放事件，用于停止拖动
		document.onmouseup = stopDrag;
		// 注册鼠标移动事件，用于处理拖动过程
		document.onmousemove = handleElementDrag;
	};

	/**
	 * 处理元素拖动过程的函数
	 *
	 * @param {MouseEvent} event - 鼠标事件对象
	 */
	function handleElementDrag(event: MouseEvent) {
		// 阻止默认事件行为
		event.preventDefault();
		// 计算鼠标位置的差值
		mouseXDelta = initialMouseX - event.clientX;
		mouseYDelta = initialMouseY - event.clientY;
		// 更新鼠标初始位置
		initialMouseX = event.clientX;
		initialMouseY = event.clientY;
		/**
		 * 计算元素新的顶部位置，通过当前顶部位置减去鼠标在 Y 轴的移动差值
		 */
		let newTopPosition = targetElement.offsetTop - mouseYDelta;
		/**
		 * 计算元素新的左侧位置，通过当前左侧位置减去鼠标在 X 轴的移动差值
		 */
		let newLeftPosition = targetElement.offsetLeft - mouseXDelta;
		/**
		 * 获取当前窗口的宽度，用于后续限制元素位置在屏幕范围内
		 */
		const screenWidth = window.innerWidth;
		/**
		 * 获取当前窗口的高度，用于后续限制元素位置在屏幕范围内
		 */
		const screenHeight = window.innerHeight;
		/**
		 * 获取目标元素的宽度，用于后续限制元素位置在屏幕范围内
		 */
		const elementWidth = targetElement.offsetWidth;
		/**
		 * 获取目标元素的高度，用于后续限制元素位置在屏幕范围内
		 */
		const elementHeight = targetElement.offsetHeight;
		// 约束元素位置在屏幕范围内
		newTopPosition = Math.max(0, Math.min(newTopPosition, screenHeight - elementHeight));
		newLeftPosition = Math.max(0, Math.min(newLeftPosition, screenWidth - elementWidth));
		// 设置元素的新位置
		targetElement.style.top = newTopPosition + "px";
		targetElement.style.left = newLeftPosition + "px";
	}

	/**
	 * 停止拖动元素的处理函数
	 */
	function stopDrag() {
		// 移除鼠标释放事件处理函数
		document.onmouseup = null;
		// 移除鼠标移动事件处理函数
		document.onmousemove = null;
	}
};

/**
 * 显示或隐藏文件导入覆盖层
 *
 * @param {Element} container - 包含聊天历史面板的容器元素
 *
 * @param {boolean} [display=true] - 是否显示覆盖层，默认为 true
 */
export function displayImportOverlay(container: Element, display: boolean = true) {
	/**
	 * 获取拖拽区域覆盖层元素
	 */
	let overlay = container.querySelector('.drop-zone-overlay') as HTMLElement;
	// 若覆盖层不存在，则创建一个新的覆盖层元素
	if (!overlay) {
		// 创建一个 div 元素，用于作为拖拽区域的覆盖层
		overlay = document.createElement('div');
		// 为新创建的 div 元素设置类名，便于后续样式控制
		overlay.className = 'drop-zone-overlay';
		// 向覆盖层中插入 HTML 内容，包含一个文件导入图标，设置图标字体大小并添加底部边距
		overlay.innerHTML = `<div style="font-size: 150px; margin-bottom: 16px;"><i class="fas fa-file-import"> 导入文件</i></div><div style="font-size: 24px;">月华目前暂不支持 PDF/PPT/EXE/APK 等格式哦</div>`;
		// 将创建好的覆盖层元素添加到容器中
		container.appendChild(overlay);
	}
	// 显示拖拽区域覆盖层
	if (display) overlay.style.display = 'flex';
	// 若 display 为 false，则隐藏覆盖层
	else overlay.style.display = 'none';
};