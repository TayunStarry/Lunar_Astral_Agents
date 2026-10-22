import { CaptureWindowInfo } from '../../config/screenshot';
import { ResizeImageResult } from '../../config/image';
import { pushAssistantImage } from '../../io/conversation';
import { localPathToReadURL } from '../../io/conversation/codec';
import { ScreenshotShot } from './types';

// 截图者能力层：把 Go 侧统一的截图/枚举桥接包装成「可直接理解的文本 + 可展示的图片」
// 图像本体统一经内容仓库落盘后以前端可读 URL 推送，避免 base64 在上下文里重复堆积。

/** 单次列表最多展示的窗口数（超出只给数量，防止上下文被窗口列表撑爆） */
const MAX_WINDOW_LINES = 40;

/** JSON 参数容错解析 */
function parseArgs(args?: Record<string, any> | string): Record<string, any> {
	if (!args) return {};
	if (typeof args === 'string') {
		try {
			const parsed = JSON.parse(args);
			return parsed && typeof parsed === 'object' ? parsed : {};
		}
		catch {
			return {};
		}
	}
	return args;
}

/** 一行窗口描述 */
function formatWindowLine(item: CaptureWindowInfo): string {
	const flags = [item.minimized ? '最小化' : '', item.foreground ? '焦点' : ''].filter(Boolean).join('｜');
	const process = item.process ? `（${item.process}）` : '';
	return `- handle=${item.handle}｜${item.title}${process}｜窗口 ${item.width}x${item.height}｜客户区 ${item.client_w}x${item.client_h}`
		+ (flags ? `｜${flags}` : '');
}

/** 列出显示器与可见窗口，拼成模型可读文本 */
export function describeCaptureTargets(args?: Record<string, any> | string): string {
	const parsed = parseArgs(args);
	const includeMinimized = parsed.include_minimized === true;

	const [displays, displayErr] = screenshotGetDisplays();
	const [windows, windowErr] = screenshotListWindows();

	const sections: string[] = [];

	if (displayErr) sections.push(`显示器枚举失败：${displayErr.message || String(displayErr)}`);
	else if (displays && displays.length > 0) {
		sections.push(`显示器 ${displays.length} 个：`);
		displays.forEach(d => {
			sections.push(`- index=${d.index}｜${d.width}x${d.height}｜屏幕原点 (${d.x}, ${d.y})`);
		});
	}

	if (windowErr) sections.push(`窗口枚举失败：${windowErr.message || String(windowErr)}`);
	else {
		const list = (windows || []).filter(item => includeMinimized || !item.minimized);
		if (list.length === 0) sections.push('没有可截图的可见窗口（可用 mode=fullscreen 截整屏）。');
		else {
			const shown = list.slice(0, MAX_WINDOW_LINES);
			sections.push(`可见窗口 ${list.length} 个：`);
			shown.forEach(item => sections.push(formatWindowLine(item)));
			if (list.length > shown.length) sections.push(`……其余 ${list.length - shown.length} 个窗口未列出。`);
		}
	}

	sections.push('提示：按标题截图用 window_title（包含匹配），需要精确锁定用 window_handle；最小化窗口需先恢复才能截取。');
	return sections.join('\n');
}

/** 解析截图参数：只保留 Go 侧认识的字段，避免把 note 等说明字段送进截图请求 */
function buildCaptureParams(parsed: Record<string, any>): Record<string, any> {
	const params: Record<string, any> = {};
	const numberKeys = [
		'display_index', 'window_handle', 'offset_x', 'offset_y', 'width', 'height',
		'region_x', 'region_y', 'region_w', 'region_h'
	];
	numberKeys.forEach(key => {
		const value = Number(parsed[key]);
		if (Number.isFinite(value)) params[key] = value;
	});
	if (typeof parsed.mode === 'string' && parsed.mode) params.mode = parsed.mode;
	if (typeof parsed.window_title === 'string' && parsed.window_title.trim()) params.window_title = parsed.window_title.trim();
	if (typeof parsed.format === 'string' && parsed.format) params.format = parsed.format;
	if (parsed.client_area === true) params.client_area = true;
	return params;
}

/**
 * 把一帧截图写入内容仓库并返回可访问 URL
 *
 * 必须用 base64Decode 取字节：atob 返回字符串，二进制里的非 UTF-8 字节会被
 * 替换成 U+FFFD，再按 charCodeAt 写盘就变成 0xFD，PNG 直接损坏。
 */
function persistShot(frame: ResizeImageResult, index: number): string {
	try {
		const base64 = String(frame.base64 || '');
		const payload = base64.includes('base64,') ? base64.split('base64,')[1] : base64;
		if (!payload) return '';
		const [decoded, decodeErr] = base64Decode(payload);
		if (decodeErr || !decoded) {
			console.warn('[截图者] base64 解码失败:', decodeErr);
			return '';
		}
		const bytes = new Uint8Array(decoded);
		const ext = frame.format === 'jpg' || frame.format === 'jpeg' ? 'jpg' : 'png';
		const name = `multimedia/screenshot_${Date.now()}_${index}.${ext}`;
		const [, path, error] = saveFile(name, true, bytes);
		if (error) {
			console.warn('[截图者] 落盘失败:', error);
			return '';
		}
		return localPathToReadURL(path);
	}
	catch (error) {
		console.warn('[截图者] 落盘异常:', error);
		return '';
	}
}

/**
 * 执行一次截图
 *
 * @param args 模型给出的截图参数（对齐 Go CaptureRequest，另含 note 说明）
 * @returns 给模型的文本结果与本次截图产物（失败时 shot 为 null）
 */
export function captureScreen(args?: Record<string, any> | string): { text: string; shot: ScreenshotShot | null } {
	const parsed = parseArgs(args);
	const params = buildCaptureParams(parsed);
	const note = typeof parsed.note === 'string' ? parsed.note.trim() : '';

	const [results, captureErr] = screenshotCapture(params) as [ResizeImageResult[], Error | null];
	if (captureErr) {
		return { text: `截图失败：${captureErr.message || String(captureErr)}`, shot: null };
	}
	if (!results || results.length === 0) {
		return { text: '截图失败：未获取到图像数据', shot: null };
	}

	const first = results[0];
	const urls = results.map((frame, index) => persistShot(frame, index)).filter(Boolean);
	if (urls.length > 0) pushAssistantImage(urls);

	const modeText = String(first.mode || params.mode || 'auto');
	const windowText = first.window_title ? `窗口「${first.window_title}」` : '';
	const sourceSize = first.source_width && first.source_height
		? `，原图 ${first.source_width}x${first.source_height}`
		: '';
	const frameText = results.length > 1 ? `，共 ${results.length} 帧` : '';
	const label = note ? `${note}：` : '';
	const text = `截图完成。${label}模式=${modeText}${windowText ? '，' + windowText : ''}，`
		+ `输出 ${first.width}x${first.height}${sourceSize}${frameText}，图片已展示给用户。`;

	return {
		text,
		shot: {
			note: note || `${modeText} 截图`,
			mode: modeText,
			windowTitle: first.window_title || '',
			width: first.width,
			height: first.height,
			base64: first.base64,
			url: urls[0] || '',
		}
	};
}
