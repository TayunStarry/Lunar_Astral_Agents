import { ToolCall, GlobalConfig, ResizeImageResult, ResizeImageResults } from '../index';

// ==== 工具定义 ====

/** 截图工具定义 */
export const screenshotTools: ToolCall[] = [
	{
		type: "function",
		function: {
			name: "screenshot",
			description: "截取当前屏幕画面。默认优先截取当前焦点应用窗口，无法识别焦点窗口时自动降级为全屏截图。支持指定显示器、全屏、绝对坐标区域，以及窗口内精准子区域（偏移量+区域大小）。截取的图片会自动缩放处理并展示给用户。",
			parameters: {
				type: "object",
				properties: {
					mode: {
						type: "string",
						enum: ["auto", "window", "fullscreen", "display", "region"],
						description: "截图模式：auto=焦点窗口优先（默认，失败降级全屏）；window=强制焦点窗口；fullscreen=全屏；display=指定显示器；region=绝对坐标区域"
					},
					display_index: {
						type: "number",
						description: "显示器索引（mode=display 时生效，-1 表示全部）"
					},
					offset_x: {
						type: "number",
						description: "窗口相对 X 偏移（mode=auto/window，配合 width/height 使用，缺省为 0）"
					},
					offset_y: {
						type: "number",
						description: "窗口相对 Y 偏移（mode=auto/window，配合 width/height 使用，缺省为 0）"
					},
					width: {
						type: "number",
						description: "窗口相对区域宽度（>0 且 height>0 时启用精准区域覆盖）"
					},
					height: {
						type: "number",
						description: "窗口相对区域高度"
					},
					region_x: {
						type: "number",
						description: "绝对屏幕区域 X 坐标（mode=region）"
					},
					region_y: {
						type: "number",
						description: "绝对屏幕区域 Y 坐标（mode=region）"
					},
					region_w: {
						type: "number",
						description: "绝对屏幕区域宽度（mode=region）"
					},
					region_h: {
						type: "number",
						description: "绝对屏幕区域高度（mode=region）"
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

	console.log(`参数解析完成: ${JSON.stringify(parsed)}`);
	console.log(`准备执行截图操作...`);

	// Go 层统一处理截图捕获 + 图片压缩缩放，返回包含 base64/format/width/height 的结果对象数组
	const [results, captureErr] = screenshotCapture(parsed) as [ResizeImageResults, Error | null];
	if (captureErr) {
		console.error(`截图失败: ${captureErr.message || String(captureErr)}`);
		console.log(`========== 截图工具调用结束(失败) ==========`);
		return [`截图失败：${captureErr.message || String(captureErr)}`, ''];
	}

	if (!results || results.length === 0) {
		console.error(`截图失败: 未获取到截图数据`);
		console.log(`========== 截图工具调用结束(失败) ==========`);
		return ['截图失败：未获取到截图数据', ''];
	}

	// 取第一帧作为主要结果信息
	const firstFrame = results[0];
	console.log(`截图处理成功: ${firstFrame.width}x${firstFrame.height}, 格式=${firstFrame.format}, 帧数=${results.length}`);

	// 将所有帧的 base64 推送到前端
	const base64List = results.map((r: ResizeImageResult) => r.base64);
	pushImage(base64List);
	console.log(`图片已推送: ${firstFrame.width}x${firstFrame.height}, 格式=${firstFrame.format}, 帧数=${results.length}`);

	// 返回文本响应 + 首帧 base64 图片数据
	const sizeInfo = `${firstFrame.width}x${firstFrame.height}`;
	const frameInfo = results.length > 1 ? `（共${results.length}帧）` : '';
	const textResponse = `截图完成，已获取当前屏幕画面（${sizeInfo}）${frameInfo}，图片已展示给用户。`;
	console.log(`返回响应: ${sizeInfo}`);
	console.log(`========== 截图工具调用结束(成功) ==========`);
	return [textResponse, firstFrame.base64];
}

// ==== 模块级注册 ====

// 将截图工具注册到月华工具协议映射表
GlobalConfig.LTPfunction.set('screenshot', handleScreenshot);
// 注册截图工具到 LTPdefinition 列表
GlobalConfig.LTPdefinition.push(...screenshotTools);
