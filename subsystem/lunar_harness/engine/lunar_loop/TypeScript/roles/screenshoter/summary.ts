import type { ToolCallItem } from '../../config/model';
import { ScreenshotShot } from './types';

/** 从工具调用中收集截图产物（executeTool 已把产物暂存到 pendingShot） */
export function collectScreenshotShot(toolCall: ToolCallItem, shots: ScreenshotShot[], pending: ScreenshotShot | null): void {
	if (!pending) return;
	shots.push(pending);
}

/** 取最后一张截图作为「主图」返回给对话者 */
export function pickPrimaryShot(shots: ScreenshotShot[]): ScreenshotShot | null {
	return shots.length > 0 ? shots[shots.length - 1] : null;
}

/** 构建截图汇报（月华口吻，一句话说清截了什么） */
export function buildScreenshotSummary(shots: ScreenshotShot[]): string {
	if (shots.length === 0) return '月华没有截到画面';
	const primary = shots[shots.length - 1];
	const target = primary.windowTitle ? `「${primary.windowTitle}」` : '屏幕';
	const size = `${primary.width}x${primary.height}`;
	if (shots.length === 1) {
		return `截好了，${primary.note}（${target}，${size}），已经给你看到了。`;
	}
	return `截好了，一共 ${shots.length} 张，最后一张是${primary.note}（${target}，${size}），都已经给你看到了。`;
}
