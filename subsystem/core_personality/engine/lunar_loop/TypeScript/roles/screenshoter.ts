import { ToolCall } from '../config/tool';
import { ToolCallItem } from '../config/model';
import { CreativeRoleBase } from '../agent/creative';
import { screenshoterTools } from './screenshoter/tools';
import { captureScreen, describeCaptureTargets } from './screenshoter/shot';
import { buildScreenshotSummary, collectScreenshotShot, pickPrimaryShot } from './screenshoter/summary';
import { ScreenshotShot } from './screenshoter/types';

/** 组装截图者系统提示词 */
function composeScreenshoterPrompt(): string {
	return fileView('prompts/screenshoterRole.md')[0].trim();
}

/**
 * 截图者子智能体
 *
 * 承接对话者的截图诉求：把「截一下那个窗口 / 截屏幕下半部分 / 给我看看浏览器」这类
 * 自然语言要求，翻译成一次或多次精确截图（全屏 / 显示器 / 指定窗口 / 窗口内区域 /
 * 绝对屏幕区域），图片直接推送前端并把主图交回对话者查看。
 */
export class ScreenshoterRole extends CreativeRoleBase<ScreenshotShot> {
	/** 智能体角色标签（TUI 实况屏显示） */
	protected roleTag: string = '截图者';
	/** 允许「列目标 → 截图 → 校正重截」的轮次 */
	protected MAX_ITERATIONS = 4;
	/** 最近一次工具调用产生的截图（collectDetail 取用后清空） */
	private pendingShot: ScreenshotShot | null = null;
	/** 最近一次任务的主图（base64 data URI，供对话者注入上下文） */
	private primaryImage = '';

	/** 构造函数 */
	public constructor() {
		super(composeScreenshoterPrompt());
	}

	/** 角色名称 */
	protected get roleName(): string { return '截图者' }

	/** 获取工具定义 */
	protected getToolDefinitions(): ToolCall[] {
		return screenshoterTools;
	}

	/** 执行截图工具调用 */
	protected executeTool(toolCall: ToolCallItem): string {
		const funcName = toolCall.function.name;
		let args: Record<string, any> = {};
		try {
			args = typeof toolCall.function.arguments === 'string'
				? JSON.parse(toolCall.function.arguments)
				: (toolCall.function.arguments || {});
		}
		catch (parseError) {
			console.error(`[截图者] 工具调用参数解析失败:`, toolCall.function.arguments);
			return `工具调用参数解析失败，请确保传入合法的 JSON 字符串。错误: ${parseError}`;
		}

		switch (funcName) {
			case 'list_capture_targets':
				return describeCaptureTargets(args);
			case 'capture_screen': {
				const result = captureScreen(args);
				this.pendingShot = result.shot;
				return result.text;
			}
			default:
				return `未知工具: ${funcName}，可用工具为 list_capture_targets、capture_screen`;
		}
	}

	/** 从工具调用中收集截图产物 */
	protected collectDetail(toolCall: ToolCallItem, shots: ScreenshotShot[]): void {
		collectScreenshotShot(toolCall, shots, this.pendingShot);
		this.pendingShot = null;
	}

	/** 构建截图汇报，并保留主图供对话者查看 */
	protected buildSummary(shots: ScreenshotShot[]): string {
		const primary = pickPrimaryShot(shots);
		this.primaryImage = primary?.base64 || '';
		return buildScreenshotSummary(shots);
	}

	/**
	 * 取走本次任务的主图（base64 data URI）并清空
	 *
	 * 只在汇报成功后由调度方读取一次，避免同一张图被重复注入对话上下文。
	 */
	public takePrimaryImage(): string {
		const image = this.primaryImage;
		this.primaryImage = '';
		return image;
	}
}
