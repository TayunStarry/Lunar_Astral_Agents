
// 类型定义
import * as EntryAPI from '../EntryAPI/code';

/** 截图工具类型 */
export type DrawingTool = 'rect' | 'draw' | 'line' | 'circle' | 'text' | 'arrow';
/** 颜色十六进制值 */
export type ColorHex = string;
/** 线宽大小 */
export type LineSize = string;
/** 鼠标位置 */
export interface MousePosition {
    x: number;
    y: number;
}
/** 画布缩放比例 */
export interface CanvasScale {
    scaleX: number;
    scaleY: number;
}
/** 区域截图配置 */
export interface RegionConfig {
    /** 区域截图起始点 X 坐标 */
    x: number;
    /** 区域截图起始点 Y 坐标 */
    y: number;
    /** 区域截图宽度 */
    width: number;
    /** 区域截图高度 */
    height: number;
}
/** 持续捕捉配置 */
export interface ContinuousConfig {
    /** 是否启用持续捕捉 */
    enabled: boolean;
    /** 捕捉间隔（毫秒） */
    interval: number;
    /** 持续捕捉定时器 ID */
    intervalId: number | null;
}
/**
 * 截图核心实例，用于处理屏幕截图、区域配置、捕获间隔等功能
 */
export const screenshotCore = new EntryAPI.ScreenshotCore();
/**
 * 绘制工具实例，用于处理用户在画布上的绘制操作（自由绘制、矩形、直线、圆形、箭头）
 */
export const drawingTools = new EntryAPI.DrawingTools(EntryAPI.drawCanvas, EntryAPI.previewCanvas, screenshotCore);
/**
 * 历史记录管理器实例，用于管理用户绘制操作的历史记录（撤销、重做）
 */
export const historyManager = new EntryAPI.HistoryManager(EntryAPI.drawCanvas, EntryAPI.drawCtx, EntryAPI.undoDrawButton);
/**
 * 工具选择器实例，用于处理用户选择绘制工具（自由绘制、矩形、直线、圆形、箭头）
 */
export const toolSelector = new EntryAPI.ToolSelector(EntryAPI.drawCanvas);
/**
 * 初始化区域控制功能，包括更新滑块值显示、区域滑块事件、间隔滑块事件
 */
export function initRegionControls() {
	// 更新滑块值显示
	function updateSliderDisplay(slider: HTMLInputElement, valueDisplay: HTMLElement) {
		valueDisplay.textContent = slider.value;
	}
	// 区域X滑块事件
	EntryAPI.regionXSlider.addEventListener('input',
		() => {
			updateSliderDisplay(EntryAPI.regionXSlider, EntryAPI.regionXSlider.nextElementSibling as HTMLElement);
			updateRegionConfig();
		}
	);
	// 区域Y滑块事件
	EntryAPI.regionYSlider.addEventListener('input',
		() => {
			updateSliderDisplay(EntryAPI.regionYSlider, EntryAPI.regionYSlider.nextElementSibling as HTMLElement);
			updateRegionConfig();
		}
	);
	// 区域宽度滑块事件
	EntryAPI.regionWidthSlider.addEventListener('input',
		() => {
			updateSliderDisplay(EntryAPI.regionWidthSlider, EntryAPI.regionWidthSlider.nextElementSibling as HTMLElement);
			updateRegionConfig();
		}
	);
	// 区域高度滑块事件
	EntryAPI.regionHeightSlider.addEventListener('input',
		() => {
			updateSliderDisplay(EntryAPI.regionHeightSlider, EntryAPI.regionHeightSlider.nextElementSibling as HTMLElement);
			updateRegionConfig();
		}
	);
	// 捕获间隔滑块事件
	EntryAPI.intervalSlider.addEventListener('input',
		() => {
			const value = parseInt(EntryAPI.intervalSlider.value);
			EntryAPI.intervalValueDisplay.textContent = `${value}ms`;
			screenshotCore.setCaptureInterval(value);
		}
	);
	// 初始化滑块显示
	updateSliderDisplay(EntryAPI.regionXSlider, EntryAPI.regionXSlider.nextElementSibling as HTMLElement);
	updateSliderDisplay(EntryAPI.regionYSlider, EntryAPI.regionYSlider.nextElementSibling as HTMLElement);
	updateSliderDisplay(EntryAPI.regionWidthSlider, EntryAPI.regionWidthSlider.nextElementSibling as HTMLElement);
	updateSliderDisplay(EntryAPI.regionHeightSlider, EntryAPI.regionHeightSlider.nextElementSibling as HTMLElement);
	updateSliderDisplay(EntryAPI.intervalSlider, EntryAPI.intervalSlider.nextElementSibling as HTMLElement);
	updateRegionConfig(false);
}
/**
 * 截图约束执行实例，用于在满足条件时触发截图操作
 */
const captureScreen = new EntryAPI.ConstraintExecution(0.005, 1, () => screenshotCore.captureScreen());
/**
 * 更新区域配置
 * 根据用户在区域滑块上的调整，更新截图核心实例的区域配置
 * @param {boolean} screenshotsAllowed - 是否允许截图，默认值为 true
 */
function updateRegionConfig(screenshotsAllowed: boolean=true) {
	// 停止持续捕捉，确保新配置生效
	screenshotCore.stopContinuousCapture();
	// 更新截图核心实例的区域配置
	screenshotCore.accessScreenshotConfig =
	{
		x: parseInt(EntryAPI.regionXSlider.value),
		y: parseInt(EntryAPI.regionYSlider.value),
		width: parseInt(EntryAPI.regionWidthSlider.value),
		height: parseInt(EntryAPI.regionHeightSlider.value)
	};
	// 触发一次截图，查看新配置是否生效
	if (screenshotsAllowed) captureScreen.run();
}

/**
 * 创建共享视觉内容
 * 从输入框获取内容，若内容不为空则添加消息到历史记录并进行渲染，最后清空输入框
 */
export async function createSimpleVisual(): Promise<void> {
	/** 获取用户当前输入的所有消息 */
	const userMessage = EntryAPI.getUserMessage();
	/** 获取合并后的图片数据 */
	const imageData = screenshotCore.getMergedImageData();
	// 检查按钮是否禁用
	if (EntryAPI.getReleaseButtonsDisabledState()) return;
	// 禁用按钮
	EntryAPI.disabledReleaseButton(true);
	/**
	 * 发送单条消息到聊天面板
	 *
	 * @param {string} message - 消息文本内容
	 *
	 * @param {number} index - 消息索引，用于判断是否为最后一条消息
	 */
	async function SendMessage(message: string, index: number) {
		/** 仅在最后一条消息携带图片 URL，其余传 null */
		const attachImageUrl = index >= userMessage.length - 1 ? imageData : null;
		/** 创建用户消息对象 */
		const messageObject = await EntryAPI.createMessageObject("user", message, true, false, false, attachImageUrl);
		// 创建并渲染消息对象
		EntryAPI.renderMessage(messageObject, EntryAPI.chatHistoryPanel);
		// 等待 1 秒，确保前端渲染完成后再继续
		await new Promise(resolve => setTimeout(resolve, 500));
	}
	// 若用户未输入任何消息，则发送空文本并附带图片
	if (userMessage.length === 0) SendMessage('', 0);
	// 遍历用户消息数组，依次发送每个消息
	else for (let i = 0; i < userMessage.length; i++) {
		await SendMessage(userMessage[i], i);
	}
	// 清除所有配置面板的显示状态
	EntryAPI.eraseAllConfigurePanel();
	// 显示对话和历史记录面板
	EntryAPI.chatHistoryContainerPanel.style.display = "flex";
	// 改变全局变量，表示无配置面板显示
	EntryAPI.OnlyData.configurePanelOption = 'any';
	// 调用后端 API 继续对话流程
	EntryAPI.executeDialogueAndParse(EntryAPI.chatHistoryPanel);
};

document.addEventListener('DOMContentLoaded', async () => {
	// 设置回调函数
	toolSelector.onToolChange = (tool) => {
		drawingTools.setCurrentTool(tool);
	};

	toolSelector.onColorChange = (color) => {
		drawingTools.setCurrentColor(color);
	};

	toolSelector.onSizeChange = (size) => {
		drawingTools.setCurrentSize(size);
	};

	// 初始化区域控制
	initRegionControls();

	// 绘制事件处理
	EntryAPI.drawCanvas.addEventListener('mousedown', (e: MouseEvent) => {
		// 在开始绘制前保存状态
		historyManager.saveState();
		drawingTools.startDrawing(e);
	});

	EntryAPI.drawCanvas.addEventListener('mousemove', (e: MouseEvent) => {
		drawingTools.draw(e);
	});

	EntryAPI.drawCanvas.addEventListener('mouseup', (e: MouseEvent) => {
		const didDraw = drawingTools.stopDrawing(e);
		if (didDraw && drawingTools.currentTool === 'draw') {
			// 自由绘制完成后保存状态
			historyManager.saveState();
		}
	});

	EntryAPI.drawCanvas.addEventListener('mouseleave', (e: MouseEvent) => {
		const didDraw = drawingTools.stopDrawing(e);
		if (didDraw && drawingTools.currentTool === 'draw') {
			// 自由绘制完成后保存状态
			historyManager.saveState();
		}
	});
	// 缩放滑条事件
	EntryAPI.scaleSlider.addEventListener('input', () => {
		const value = parseFloat(EntryAPI.scaleSlider.value);
		EntryAPI.scaleValueDisplay.textContent = `${value.toFixed(1)}x`;
		screenshotCore.updateScale = value;
	});
	// 初始化缩放滑条显示
	EntryAPI.scaleValueDisplay.textContent = `${EntryAPI.scaleSlider.value}x`;
	// 按钮事件处理
	EntryAPI.captureSceneButton.addEventListener('click', () => screenshotCore.captureScreen());
	EntryAPI.continuousCaptureButton.addEventListener('click', () => {
		screenshotCore.toggleContinuousCapture();
	});
	EntryAPI.downloadSceneButton.addEventListener('click', () => screenshotCore.downloadImage());
	EntryAPI.undoDrawButton.addEventListener('click', () => historyManager.undo());

	// 窗口大小变化时重新计算缩放比例
	window.addEventListener('resize', () => screenshotCore.calculateScale());

	// 初始化工具选择器
	toolSelector.initEventListeners();
	toolSelector.initDefaultState();
});