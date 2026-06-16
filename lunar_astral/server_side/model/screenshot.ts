import { ToolCall, OnlyData, ResizeImageResult } from '../index';

// ==== 工具定义 ====

/** 截图工具定义 */
export const screenshotTools: ToolCall[] = [
	{
		type: "function",
		function: {
			name: "screenshot",
			description: "截取当前屏幕画面。当用户要求查看屏幕内容、确认屏幕显示状态、或需要获取当前屏幕画面时，应使用此工具。支持指定显示器索引、截取区域、缩放比例和图片格式。截取的图片会自动缩放处理并展示给用户。",
			parameters: {
				type: "object",
				properties: {
					display_index: {
						type: "number",
						description: "显示器索引：0 表示主显示器，-1 表示截取所有显示器拼接画面，默认为 0"
					},
					region: {
						type: "string",
						description: "截图区域，格式为 'x,y,width,height'，例如 '100,200,800,600'。留空则截取整个显示器"
					},
					scale: {
						type: "string",
						description: "缩放参数：可以是比例（如 '0.5'）或指定宽高（如 '800,600'）。留空则自动缩放"
					},
					format: {
						type: "string",
						description: "输出图片格式：'png' 或 'jpg'，默认为 'png'",
						enum: ["png", "jpg"]
					}
				},
				required: []
			}
		}
	}
];

// ==== 工具处理函数 ====

/** 处理截图工具调用 */
async function handleScreenshot(args?: Record<string, any> | string): Promise<string> {
	const parsed = typeof args === 'string' ? JSON.parse(args) : (args || {});
	const { display_index, region, scale, format } = parsed;

	const displayIndex = display_index ?? 0;
	const captureFormat = format || 'png';

	console.log(`[截图] 工具调用: display=${displayIndex}, region="${region || ''}", scale="${scale || ''}", format="${captureFormat}"`);

	// 执行截图
	const [dataURI, captureErr] = screenshotCapture(displayIndex, region || '', scale || '', captureFormat, 0);
	if (captureErr) {
		console.error(`[截图] 截图失败: ${captureErr.message || String(captureErr)}`);
		return `截图失败：${captureErr.message || String(captureErr)}`;
	}

	if (!dataURI || dataURI.length === 0) {
		return '截图失败：未获取到截图数据';
	}

	// 通过 resizeImage 缩放处理
	const [resizeResult, resizeErr] = resizeImage(dataURI) as [ResizeImageResult, Error | null];
	if (resizeErr) {
		console.error(`[截图] 图片缩放失败: ${resizeErr.message || String(resizeErr)}`);
		return `截图失败：图片缩放处理出错 - ${resizeErr.message || String(resizeErr)}`;
	}

	// 将缩放后的图片推送到未读消息中
	if (resizeResult?.base64) {
		pushImage([resizeResult.base64]);
		console.log(`[截图] 图片已推送: ${resizeResult.width}x${resizeResult.height}, 格式=${resizeResult.format}`);
	}

	// 返回文本响应
	const sizeInfo = resizeResult ? `${resizeResult.width}x${resizeResult.height}` : '未知';
	return `截图完成，已获取当前屏幕画面（${sizeInfo}），图片已展示给用户。`;
}

// ==== 模块级注册 ====

// 将截图工具注册到月华工具协议映射表
OnlyData.LTPfunction.set('screenshot', handleScreenshot);
// 注册截图工具到 LTPdefinition 列表
OnlyData.LTPdefinition.push(...screenshotTools);
