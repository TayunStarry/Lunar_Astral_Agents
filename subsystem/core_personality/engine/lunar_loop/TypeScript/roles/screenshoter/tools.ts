import { ToolCall } from '../../config/tool';

// ==== 截图者工具定义 ====
// 截图 = 先看清有哪些可截目标（显示器/窗口）→ 再按自然语言要求精确取景。
// 两个工具覆盖系统全部截图能力：全屏、指定显示器、指定窗口（含被遮挡窗口）、
// 窗口内相对区域、绝对屏幕区域。

/** 截图者工具集 */
export const screenshoterTools: ToolCall[] = [
	{
		type: 'function',
		function: {
			name: 'list_capture_targets',
			description: '列出当前所有显示器与可见窗口（标题、进程、窗口矩形、客户区尺寸、是否最小化、是否焦点）。当用户说的「那个窗口」不明确、或按标题截图前需要确认目标时先调用它。',
			parameters: {
				type: 'object',
				properties: {
					include_minimized: {
						type: 'boolean',
						description: '是否也列出已最小化的窗口（最小化窗口无法截图，仅用于告知用户需要先恢复），默认 false'
					}
				},
				required: []
			}
		}
	},
	{
		type: 'function',
		function: {
			name: 'capture_screen',
			description: [
				'执行一次截图并立即展示给用户。按用户要求选择模式：',
				'mode=fullscreen 截全部屏幕（多显示器会拼成一张）；',
				'mode=display 截单个显示器（display_index）；',
				'mode=window 截指定窗口：window_title 按标题包含匹配，window_handle 按句柄精确匹配，两者都不填则截当前焦点窗口；被其他窗口遮挡但不处于最小化的窗口同样能截到；',
				'mode=region 截绝对屏幕坐标区域（region_x/region_y/region_w/region_h）；',
				'在 mode=window 下再给 width/height 可只截窗口内相对区域（offset_x/offset_y 为窗口左上角偏移，client_area=true 时以客户区左上角为原点）。',
				'mode=auto 为默认：截焦点窗口，失败时自动降级为全屏。'
			].join('\n'),
			parameters: {
				type: 'object',
				properties: {
					mode: {
						type: 'string',
						enum: ['auto', 'window', 'fullscreen', 'display', 'region'],
						description: '截图模式，默认 auto'
					},
					window_title: {
						type: 'string',
						description: '目标窗口标题关键字（包含匹配，忽略大小写）。用于截不在前台的指定窗口'
					},
					window_handle: {
						type: 'number',
						description: '目标窗口句柄（优先于 window_title）。来自 list_capture_targets 的 handle'
					},
					client_area: {
						type: 'boolean',
						description: '是否只截客户区（去掉标题栏与边框），默认 false；截网页/应用内容时通常设为 true'
					},
					display_index: {
						type: 'number',
						description: '显示器索引（mode=display 时生效）；-1 表示全部显示器拼接'
					},
					offset_x: {
						type: 'number',
						description: '窗口内相对 X 偏移（配合 width/height）'
					},
					offset_y: {
						type: 'number',
						description: '窗口内相对 Y 偏移（配合 width/height）'
					},
					width: {
						type: 'number',
						description: '窗口内区域宽度（>0 且 height>0 时只截该子区域）'
					},
					height: {
						type: 'number',
						description: '窗口内区域高度'
					},
					region_x: {
						type: 'number',
						description: '绝对屏幕区域 X（mode=region）'
					},
					region_y: {
						type: 'number',
						description: '绝对屏幕区域 Y（mode=region）'
					},
					region_w: {
						type: 'number',
						description: '绝对屏幕区域宽度（mode=region）'
					},
					region_h: {
						type: 'number',
						description: '绝对屏幕区域高度（mode=region）'
					},
					format: {
						type: 'string',
						enum: ['png', 'jpg'],
						description: '输出格式，默认 png。内容型截图（文字/界面）建议 png'
					},
					note: {
						type: 'string',
						description: '本次截图的一句话说明（如「Chrome 主窗口」「任务栏右下角」），会写进给用户的汇报'
					}
				},
				required: []
			}
		}
	}
];
