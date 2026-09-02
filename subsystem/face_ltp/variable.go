package FaceLTP

import (
	"net/http"
	"sync"
	"time"
)

// ==== 常量 ====

const (
	// AgentMaxRounds 独立上下文历史最多保留的对话轮数（1 轮 = 指令 + 答复）
	AgentMaxRounds = 40
	// AgentMaxToolLoops 单条指令允许的最大工具调用循环次数（规范+执行+观察+步进+重试，防死循环）
	AgentMaxToolLoops = 30
	// gridStep 截图坐标网格间隔（像素），用于视觉定位
	gridStep = 100
	// aiTimeout 单次模型调用的超时
	aiTimeout = 120 * time.Second
	// faceLTPUseVision 是否启用多模态视觉识别。
	// 视觉输入复用 image_processor 的焦点窗口截图（体积远小于全屏截图），见 agent.go screenshotImagePart。
	faceLTPUseVision = true
	// visionMaxDim 发送给视觉模型的截图最大边长（像素），超出则等比缩小，控制体积避免代理超时/502。
	visionMaxDim = 768
	// visionJPEGQuality 视觉截图 JPEG 编码质量（1-100），越小体积越小。
	visionJPEGQuality = 60
	// diffThumbDim 画面差异检测用的缩略图最大边长（像素），越小比对越快、对微小像素级扰动越不敏感。
	diffThumbDim = 96
	// stagePlanning / stageExecuting 智能体执行阶段：规范（计划）阶段与执行阶段。
	// 规范阶段只允许 set_plan；执行阶段逐项执行原子操作。
	stagePlanning  = "planning"
	stageExecuting = "executing"
)

// ==== 智能体独立上下文 ====

// agentHistory 独立上下文历史（保留最近 AgentMaxRounds 轮）
var agentHistory []agentRound

// historyMu 保护 agentHistory 的并发读写
var historyMu sync.Mutex

// ==== 执行计划与阶段状态（每条指令运行期间维护） ====

// agentTaskHistory 当前指令的执行计划项（由 set_plan 建立，confirm_step 标记状态）
var agentTaskHistory []agentPlanItem

// agentStage 当前执行阶段：stagePlanning（规范/计划）或 stageExecuting（执行）。
// 规范阶段只允许 set_plan；执行阶段每轮只执行一个原子操作。
var agentStage = stagePlanning

// agentTaskMu Face-LTP 单任务执行互斥锁：保证同一时刻只有一条月华指令在跑，
// 因为桌面存在全局性（前台窗口/鼠标/键盘/截图）单点状态，多个 /ltpx/call 并发会相互污染
// （agentTaskHistory/agentStage 等也是处理器内共享状态）。
var agentTaskMu sync.Mutex

// loadOnce 保证模型配置（lunar_config.json）只加载一次
var loadOnce sync.Once

// agentHTTPClient 多模态模型调用客户端
var agentHTTPClient = &http.Client{Timeout: aiTimeout}

// ==== 模型配置（从 lunar_config.json 的 agent 字段读取，见 model.go） ====

// modelConfig 缓存的模型配置（启动时读取一次）
var modelConfig struct {
	Model string // 多模态模型名
	URL   string // 多模态服务 API 地址（已含 /v1）
	Key   string // 多模态服务 API 密钥
}

// ==== 工具定义（OpenAI function calling schema） ====

// agentTools Face-LTP 智能体的工具集。
// 工作流程严格遵循「规范→执行→观察→步进」：set_plan 建立计划 → 每轮只执行一个原子操作
// → 观察最新截图核实 → confirm_step 标记完成/重试 → finish 结束。
// 鼠标原语统一为：click=左键单击、mouse=通用按键（非左键/按住）、mouse_drag=拖拽、
// move_mouse=移动光标、scroll_wheel=指定位置滚轮（自动先移光标），坐标一律为窗口内相对坐标。
var agentTools = []toolDef{
	{
		Type: "function",
		Function: function{
			Name:        "set_plan",
			Description: "(规范阶段·首选调用) 根据【月华指令】与当前桌面环境，把任务拆解为一连串具体的执行计划项，每项对应桌面上的一个具体操作（如：找到并激活窗口、点击输入框、输入文本、点击发送按钮）。调用后系统会建立任务历史，随后进入执行阶段逐项操作。",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"plan": map[string]any{
						"type":        "array",
						"description": "计划项列表（按执行顺序）",
						"items": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"no":     map[string]any{"type": "integer", "description": "步骤序号（从 1 起）"},
								"action": map[string]any{"type": "string", "description": "该步骤操作描述（简短，如：激活QQ窗口）"},
								"detail": map[string]any{"type": "string", "description": "具体落点/定位说明（目标窗口标题、按钮位置等）"},
							},
							"required": []string{"no", "action"},
						},
					},
				},
				"required": []string{"plan"},
			},
		},
	},
	{
		Type: "function",
		Function: function{
			Name:        "list_windows",
			Description: "枚举当前 Windows 桌面上的顶层窗口（带窗口标题、进程名、类名、进程ID）。用于定位已打开的程序窗口；窗口标题不含应用名时（如 QQ 聊天窗口），按进程名（如 QQ.exe）也能找到。找不到目标程序先用它确认窗口是否已存在。",
			Parameters: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
				"required":   []string{},
			},
		},
	},
	{
		Type: "function",
		Function: function{
			Name:        "activate_window",
			Description: "将指定标题或进程名的窗口置于前台并聚焦，供后续点击/键入操作。title 传窗口标题关键字（如 记事本、Chrome）或进程名（如 QQ.exe、TIM.exe）。",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"title": map[string]any{"type": "string", "description": "要激活的窗口标题关键字"},
				},
				"required": []string{"title"},
			},
		},
	},
	{
		Type: "function",
		Function: function{
			Name:        "close_window",
			Description: "关闭指定标题或进程名的窗口（发送 WM_CLOSE）。关闭某个窗口/程序时用它；关闭多个窗口要逐个调用，不要只关一个。title 传窗口标题关键字（如 任务管理器、画图）或进程名（如 QQ.exe）。",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"title": map[string]any{"type": "string", "description": "要关闭的窗口标题关键字"},
				},
				"required": []string{"title"},
			},
		},
	},
	{
		Type: "function",
		Function: function{
			Name:        "launch_program",
			Description: "按名称搜索并启动 Windows 程序（注意：仅用于启动应用程序本身，如 记事本/Chrome，不要用它来打开某个文件夹或路径）：先查开始菜单快捷方式，再查常见程序目录；若同名窗口已存在则直接激活。name 传程序名关键字（如 记事本 / notepad / Chrome）。",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name": map[string]any{"type": "string", "description": "要启动的程序名关键字"},
				},
				"required": []string{"name"},
			},
		},
	},
	{
		Type: "function",
		Function: function{
			Name:        "open_folder",
			Description: "在文件资源管理器中直接打开指定文件夹路径（一步到位，无需手动键入）。打开某个目录/文件夹/路径时优先用它。path 传要打开的绝对路径，如 D:\\Lunar_Astral_Agents 或 C:\\Windows。",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{"type": "string", "description": "要打开的文件夹绝对路径"},
				},
				"required": []string{"path"},
			},
		},
	},
	{
		Type: "function",
		Function: function{
			Name:        "click",
			Description: "左键单击：在「焦点窗口内相对坐标」(x, y) 单击左键（坐标与截图网格刻度一致、原点为窗口左上角，系统自动换算成屏幕绝对坐标）。用于绝大多数点击（按钮/输入框/发送按钮/标签页等）。double=true 表示双击。需要右键/中键或按住时长时改用 mouse。",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"x":      map[string]any{"type": "integer", "description": "窗口内相对 X 坐标（与截图网格一致）"},
					"y":      map[string]any{"type": "integer", "description": "窗口内相对 Y 坐标（与截图网格一致）"},
					"double": map[string]any{"type": "boolean", "description": "是否双击，默认 false"},
				},
				"required": []string{"x", "y"},
			},
		},
	},
	{
		Type: "function",
		Function: function{
			Name:        "mouse",
			Description: "通用鼠标按键：在「焦点窗口内相对坐标」(x, y) 按下并释放指定按键。**仅**当需要非左键（button=right 右键 / middle 中键）或按住时长（hold: short=短按1秒 / long=长按10秒）时使用；普通左键单击一律用 click，不要用 mouse 代替。",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"x":      map[string]any{"type": "integer", "description": "窗口内相对 X 坐标（与截图网格一致）"},
					"y":      map[string]any{"type": "integer", "description": "窗口内相对 Y 坐标（与截图网格一致）"},
					"button": map[string]any{"type": "string", "enum": []string{"left", "right", "middle"}, "description": "按键，默认 left"},
					"hold":   map[string]any{"type": "string", "enum": []string{"tap", "short", "long"}, "description": "按住语义：tap=瞬时、short=短按1秒、long=长按10秒，默认 tap"},
				},
				"required": []string{"x", "y"},
			},
		},
	},
	{
		Type: "function",
		Function: function{
			Name:        "mouse_drag",
			Description: "在「焦点窗口内」按住左键从起点 (x1,y1) 拖到终点 (x2,y2) 后松开（窗口内相对坐标，与截图网格一致）。用于画图/绘图、框选、拖动对象等需要按住拖动的操作。steps 为拖拽平滑步数（默认 10）。",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"x1":    map[string]any{"type": "integer", "description": "拖拽起点窗口内 X 坐标"},
					"y1":    map[string]any{"type": "integer", "description": "拖拽起点窗口内 Y 坐标"},
					"x2":    map[string]any{"type": "integer", "description": "拖拽终点窗口内 X 坐标"},
					"y2":    map[string]any{"type": "integer", "description": "拖拽终点窗口内 Y 坐标"},
					"steps": map[string]any{"type": "integer", "description": "拖拽平滑步数，默认 10"},
				},
				"required": []string{"x1", "y1", "x2", "y2"},
			},
		},
	},
	{
		Type: "function",
		Function: function{
			Name:        "move_mouse",
			Description: "仅把光标移动到「焦点窗口内相对坐标」(x, y)，不点击、不滚动。用于把光标定位到目标区域后再做其它操作（如滚动前先定位到滚动区域）。",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"x": map[string]any{"type": "integer", "description": "窗口内相对 X 坐标（与截图网格一致）"},
					"y": map[string]any{"type": "integer", "description": "窗口内相对 Y 坐标（与截图网格一致）"},
				},
				"required": []string{"x", "y"},
			},
		},
	},
	{
		Type: "function",
		Function: function{
			Name:        "scroll_wheel",
			Description: "在「焦点窗口内相对坐标」(x, y) 处滚动滚轮（自动先把光标移过去再滚，无需先调 move_mouse）。direction 传 up 或 down，amount 传滚动的格数（每格约 120 单位，默认 3）。",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"x":         map[string]any{"type": "integer", "description": "窗口内相对 X 坐标（与截图网格一致）"},
					"y":         map[string]any{"type": "integer", "description": "窗口内相对 Y 坐标（与截图网格一致）"},
					"direction": map[string]any{"type": "string", "description": "up 或 down"},
					"amount":    map[string]any{"type": "integer", "description": "滚动格数，默认 3"},
				},
				"required": []string{"x", "y", "direction"},
			},
		},
	},
	{
		Type: "function",
		Function: function{
			Name:        "type_text",
			Description: "在当前焦点窗口模拟键入文本（支持中文与标点，逐个 Unicode 输入）。",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"text": map[string]any{"type": "string", "description": "要键入的文本内容"},
				},
				"required": []string{"text"},
			},
		},
	},
	{
		Type: "function",
		Function: function{
			Name:        "type_and_send",
			Description: "在当前焦点窗口原子化地键入文本并立即回车发送（中间不做任何窗口/焦点操作，避免焦点被切跑）。仅用于回车即可发送的应用；新版 QQ 等回车不发送的应用请改用「type_text + click 发送按钮」。",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"text": map[string]any{"type": "string", "description": "要键入并发送的文本内容"},
				},
				"required": []string{"text"},
			},
		},
	},
	{
		Type: "function",
		Function: function{
			Name:        "press_key",
			Description: "模拟键盘按键，支持组合键（用 + 连接，如 Ctrl+A、Alt+F4、Ctrl+Tab）与按住语义（前缀 short: 短按1秒 / long: 或 hold: 长按10秒，如 short:W、long:Ctrl+A）。",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"key": map[string]any{"type": "string", "description": "键名或组合键，如 enter、tab、esc、ctrl+a、alt+f4、ctrl+tab、short:W"},
				},
				"required": []string{"key"},
			},
		},
	},
	{
		Type: "function",
		Function: function{
			Name:        "wait",
			Description: "等待 ms 毫秒（应用启动/页面加载/操作反应完成），之后会重新观测桌面。",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"ms": map[string]any{"type": "integer", "description": "毫秒数，如 2000"},
				},
				"required": []string{"ms"},
			},
		},
	},
	{
		Type: "function",
		Function: function{
			Name:        "capture_screenshot",
			Description: "截取当前「焦点窗口」并叠加 100px 坐标网格，刷新为最新一张截图（用于视觉定位与观察验证）。每轮系统会自动附带最新截图，通常无需显式调用；当需要立即重新观察时调用。",
			Parameters: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
				"required":   []string{},
			},
		},
	},
	{
		Type: "function",
		Function: function{
			Name:        "confirm_step",
			Description: "确认某计划项已通过观察验证（passed=true）或仍未命中需重试（passed=false）。在一次操作执行并观察结果后，明确标记该关键步骤完成或需重试。",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"no":     map[string]any{"type": "integer", "description": "计划步骤序号"},
					"passed": map[string]any{"type": "boolean", "description": "true=该步已确认完成；false=未命中，需重试"},
					"note":   map[string]any{"type": "string", "description": "说明（可选）"},
				},
				"required": []string{"no", "passed"},
			},
		},
	},
	{
		Type: "function",
		Function: function{
			Name:        "finish",
			Description: "结束本次桌面任务并给出总结。当所有计划步骤完成、或确实无法继续推进时调用。",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"summary": map[string]any{"type": "string", "description": "一两句简洁中文总结"},
				},
				"required": []string{"summary"},
			},
		},
	},
}
