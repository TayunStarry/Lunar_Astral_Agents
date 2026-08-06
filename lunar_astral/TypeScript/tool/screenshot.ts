import { ToolCall, GlobalConfig, ResizeImageResult } from '../index';

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
async function handleScreenshot(args?: Record<string, any> | string): Promise<string[]> {
	console.log(`========== 开始处理截图工具调用 ==========`);
	console.log(`原始参数: ${JSON.stringify(args)}`);

	const parsed = typeof args === 'string' ? JSON.parse(args) : (args || {});
	const { display_index, region, scale, format } = parsed;

	console.log(`参数解析完成: display_index=${display_index}, region=${region}, scale=${scale}, format=${format}`);

	const displayIndex = display_index ?? 0;
	const captureFormat = format || 'png';

	console.log(`最终参数: display=${displayIndex}, region="${region || ''}", scale="${scale || ''}", format="${captureFormat}"`);
	console.log(`准备执行截图操作...`);

	// Go 层统一处理截图捕获 + 图片压缩缩放，返回包含 base64/format/width/height 的结果对象
	const [result, captureErr] = screenshotCapture(displayIndex, region || '', scale || '', captureFormat, 0) as [ResizeImageResult, Error | null];
	if (captureErr) {
		console.error(`截图失败: ${captureErr.message || String(captureErr)}`);
		console.log(`========== 截图工具调用结束(失败) ==========`);
		return [`截图失败：${captureErr.message || String(captureErr)}`, ''];
	}

	console.log(`截图处理成功: ${result?.width}x${result?.height}, 格式=${result?.format}`);

	if (!result || !result.base64) {
		console.error(`截图失败: 未获取到截图数据`);
		console.log(`========== 截图工具调用结束(失败) ==========`);
		return ['截图失败：未获取到截图数据', ''];
	}

	// 将处理后的图片推送到前端，base64 格式为 "data:image/[format];base64,[data]"
	pushImage([result.base64]);
	console.log(`图片已推送: ${result.width}x${result.height}, 格式=${result.format}, 数据长度=${result.base64.length} 字节`);

	// 返回文本响应 + base64 图片数据
	const sizeInfo = `${result.width}x${result.height}`;
	const textResponse = `截图完成，已获取当前屏幕画面（${sizeInfo}），图片已展示给用户。`;
	console.log(`返回响应: ${sizeInfo}`);
	console.log(`========== 截图工具调用结束(成功) ==========`);
	return [textResponse, result.base64];
}

// ==== 模块级注册 ====

// 将截图工具注册到月华工具协议映射表
GlobalConfig.LTPfunction.set('screenshot', handleScreenshot);
// 注册截图工具到 LTPdefinition 列表
GlobalConfig.LTPdefinition.push(...screenshotTools);
