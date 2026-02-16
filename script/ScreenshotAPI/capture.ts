/**
 * 主画布元素
 */
export const canvas = document.getElementById('canvas') as HTMLCanvasElement;
/**
 * 主画布的 2D 绘图上下文，设置为频繁读取模式
 */
export const canvasCtx = canvas.getContext('2d', { willReadFrequently: true })!;
/**
 * 画布包装器元素
 */
export const canvasWrapper = document.getElementById('canvasWrapper') as HTMLDivElement;
/**
 * 捕捉画面按钮元素
 */
export const captureSceneButton = document.getElementById('captureSceneButton') as HTMLButtonElement;
/**
 * 持续捕捉按钮元素
 */
export const continuousCaptureButton = document.getElementById('continuousCaptureButton') as HTMLButtonElement;
/**
 * 下载画面按钮元素
 */
export const downloadSceneButton = document.getElementById('downloadSceneButton') as HTMLButtonElement;
/**
 * 撤销绘制按钮元素
 */
export const undoDrawButton = document.getElementById('undoDrawButton') as HTMLButtonElement;
/**
 * 绘图画布元素
 */
export const drawCanvas = document.getElementById('drawCanvas') as HTMLCanvasElement;
/**
 * 绘图画布的 2D 绘图上下文，设置为频繁读取模式
 */
export const drawCtx = drawCanvas.getContext('2d', { willReadFrequently: true })!;
/**
 * 预览画布元素
 */
export const previewCanvas = document.getElementById('previewCanvas') as HTMLCanvasElement;
/**
 * 预览画布的 2D 绘图上下文，设置为频繁读取模式
 */
export const previewCtx = previewCanvas.getContext('2d', { willReadFrequently: true })!;
/**
 * 矩形工具按钮元素
 */
export const rectTool = document.getElementById('rectTool') as HTMLButtonElement;
/**
 * 画笔工具按钮元素
 */
export const drawTool = document.getElementById('drawTool') as HTMLButtonElement;
/**
 * 直线工具按钮元素
 */
export const lineTool = document.getElementById('lineTool') as HTMLButtonElement;
/**
 * 圆形工具按钮元素
 */
export const circleTool = document.getElementById('circleTool') as HTMLButtonElement;
/**
 * 文本工具按钮元素
 */
export const textTool = document.getElementById('textTool') as HTMLButtonElement;
/**
 * 箭头工具按钮元素
 */
export const arrowTool = document.getElementById('arrowTool') as HTMLButtonElement;
/**
 * 所有线条颜色选项元素
 */
export const colorOptions = document.querySelectorAll('.line-color') as NodeListOf<HTMLButtonElement>;
/**
 * 所有线条粗细选项元素
 */
export const sizeOptions = document.querySelectorAll('.line-size') as NodeListOf<HTMLButtonElement>;
/**
 * 区域控制滑块元素 - X 轴
 */
export const regionXSlider = document.getElementById('regionX') as HTMLInputElement;
/**
 * 区域控制滑块元素 - Y 轴
 */
export const regionYSlider = document.getElementById('regionY') as HTMLInputElement;
/**
 * 区域控制滑块元素 - 宽度
 */
export const regionWidthSlider = document.getElementById('regionWidth') as HTMLInputElement;
/**
 * 区域控制滑块元素 - 高度
 */
export const regionHeightSlider = document.getElementById('regionHeight') as HTMLInputElement;
/**
 * 区域控制开关元素
 */
export const regionToggle = document.getElementById('regionToggle') as HTMLInputElement;
/**
 * 持续捕捉控制元素 - 捕捉间隔滑块
 */
export const intervalSlider = document.getElementById('captureInterval') as HTMLInputElement;
/**
 * 持续捕捉控制元素 - 捕捉间隔显示文本
 */
export const intervalValueDisplay = document.getElementById('intervalValue') as HTMLDivElement;
/**
 * 持续捕捉控制元素 - 持续捕捉开关
 */
export const continuousToggle = document.getElementById('continuousToggle') as HTMLInputElement;
/**
 * 缩放比例滑块元素
 */
export const scaleSlider = document.getElementById('scaleSlider') as HTMLInputElement;
/**
 * 缩放比例显示文本
 */
export const scaleValueDisplay = document.getElementById('scaleValue') as HTMLDivElement;
