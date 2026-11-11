//go:build windows

package execute

// variable.go 「桌面执行」成员的身份、提示词与工具白名单。

import (
	"time"

	"LunarSubsystem/LunarHarness/agents/desktop"
	"LunarSubsystem/LunarHarness/protocol/agentinfo"
)

// AgentID 成员唯一标识：推理实况、故障修复、数据导出与工具目录共用这一个 ID。
const AgentID = "desktop_execute"

// modelRole 实况显示名（四字，取自 protocol/agentinfo 统一约定）。
const modelRole = agentinfo.NameDesktopExecute

// autoLoopMaxRetries 模型输出命中重复后，单次模型调用的额外重试次数。
const autoLoopMaxRetries = 2

// modelTimeout 单次模型调用的最长等待时间。
const modelTimeout = 120 * time.Second

// maxRounds 单次模型调用的最大工具轮次（沿用本成员既有行为，不额外收紧或放宽）。
const maxRounds = 3

// maxActionsPerReply 单次模型回复允许执行的动作上限。
// 多出来的动作既不静默丢弃也不继续执行，由调用方显式报错。
const maxActionsPerReply = 3

// 各意图的提示词：启动与三种操作各自独立，避免语气与输出格式互相污染。
var (
	promptLauncher = desktop.MustPrompt("desktopLauncher.md")
	promptKeyboard = desktop.MustPrompt("desktopKeyboard.md")
	promptMouse    = desktop.MustPrompt("desktopMouse.md")
	promptUIAOp    = desktop.MustPrompt("desktopUIAOperator.md")
)

// 各意图允许使用的工具白名单。
var (
	launcherTools = []string{"list_windows", "activate_window", "launch_program", "open_folder", "close_window"}
	keyboardTools = []string{"type_text", "type_and_send", "press_key"}
	mouseTools    = []string{"click", "mouse_hold", "mouse_drag", "move_mouse", "scroll_wheel"}
	uiaOpTools    = []string{"uia_click", "uia_input"}
)

// toolsByIntent 意图 → 工具白名单。
var toolsByIntent = map[Intent][]string{
	IntentLaunch:   launcherTools,
	IntentKeyboard: keyboardTools,
	IntentMouse:    mouseTools,
	IntentUIA:      uiaOpTools,
}

// promptsByIntent 意图 → 提示词。
var promptsByIntent = map[Intent]string{
	IntentLaunch:   promptLauncher,
	IntentKeyboard: promptKeyboard,
	IntentMouse:    promptMouse,
	IntentUIA:      promptUIAOp,
}

// allToolDefs 全量工具定义表（名称 → function calling 描述），按意图白名单取子集。
var allToolDefs = map[string]desktop.ToolDef{
	"list_windows": {
		Type: "function",
		Function: desktop.FuncDef{
			Name: "list_windows", Description: "枚举当前 Windows 桌面上的顶层窗口（带标题/进程名/类名/PID），并给出当前前台窗口。用于定位已打开的程序窗口。",
			Parameters: map[string]any{"type": "object", "properties": map[string]any{}, "required": []string{}},
		},
	},
	"activate_window": {
		Type: "function",
		Function: desktop.FuncDef{
			Name: "activate_window", Description: "将指定标题或进程名的窗口置于前台并聚焦。title 传窗口标题关键字（如 记事本、Chrome）或进程名（如 QQ.exe）。",
			Parameters: map[string]any{"type": "object", "properties": map[string]any{"title": map[string]any{"type": "string", "description": "要激活的窗口标题/进程关键字"}}, "required": []string{"title"}},
		},
	},
	"launch_program": {
		Type: "function",
		Function: desktop.FuncDef{
			Name: "launch_program", Description: "按名称搜索并启动 Windows 程序（开始菜单/常见目录/PATH/协议）；同名窗口已存在则直接激活。name 传程序名关键字（如 记事本/notepad/Chrome）。",
			Parameters: map[string]any{"type": "object", "properties": map[string]any{"name": map[string]any{"type": "string", "description": "要启动的程序名关键字"}}, "required": []string{"name"}},
		},
	},
	"open_folder": {
		Type: "function",
		Function: desktop.FuncDef{
			Name: "open_folder", Description: "在文件资源管理器中直接打开指定文件夹绝对路径。path 传要打开的路径，如 D:\\Lunar_Astral_Agents。",
			Parameters: map[string]any{"type": "object", "properties": map[string]any{"path": map[string]any{"type": "string", "description": "文件夹绝对路径"}}, "required": []string{"path"}},
		},
	},
	"close_window": {
		Type: "function",
		Function: desktop.FuncDef{
			Name: "close_window", Description: "关闭标题含指定关键字的所有顶层窗口（发送 WM_CLOSE）。title 传窗口标题/进程关键字。",
			Parameters: map[string]any{"type": "object", "properties": map[string]any{"title": map[string]any{"type": "string", "description": "要关闭的窗口标题/进程关键字"}}, "required": []string{"title"}},
		},
	},
	"type_text": {
		Type: "function",
		Function: desktop.FuncDef{
			Name: "type_text", Description: "在当前焦点窗口模拟键入文本（支持中文与标点）。text 传要键入的内容。",
			Parameters: map[string]any{"type": "object", "properties": map[string]any{"text": map[string]any{"type": "string", "description": "要键入的文本"}}, "required": []string{"text"}},
		},
	},
	"type_and_send": {
		Type: "function",
		Function: desktop.FuncDef{
			Name: "type_and_send", Description: "在焦点窗口键入文本并回车发送。text 传要发送的内容。",
			Parameters: map[string]any{"type": "object", "properties": map[string]any{"text": map[string]any{"type": "string", "description": "要键入并发送的文本"}}, "required": []string{"text"}},
		},
	},
	"press_key": {
		Type: "function",
		Function: desktop.FuncDef{
			Name: "press_key", Description: "模拟按键/组合键，如 enter、tab、esc、ctrl+a、alt+f4、ctrl+tab。key 传键名或组合键；支持按住语义前缀 short: 短按1秒、long:/hold: 长按10秒（如 short:down 短按方向键）。",
			Parameters: map[string]any{"type": "object", "properties": map[string]any{"key": map[string]any{"type": "string", "description": "键名或组合键，可带 short:/long:/hold: 前缀"}}, "required": []string{"key"}},
		},
	},
	"click": {
		Type: "function",
		Function: desktop.FuncDef{
			Name: "click", Description: "在焦点窗口内相对坐标 (x,y) 点击。button 传 left/right/middle（默认 left）；double=true 双击",
			Parameters: map[string]any{"type": "object", "properties": map[string]any{"x": map[string]any{"type": "integer", "description": "窗口内相对X"}, "y": map[string]any{"type": "integer", "description": "窗口内相对Y"}, "button": map[string]any{"type": "string", "description": "left/right/middle，默认 left"}, "double": map[string]any{"type": "boolean", "description": "是否双击"}}, "required": []string{"x", "y"}},
		},
	},
	"mouse_hold": {
		Type: "function",
		Function: desktop.FuncDef{
			Name: "mouse_hold", Description: "在窗口内相对坐标 (x,y) 按住指定鼠标按键一段时长。用于右键/中键或按住（如拖住重命名、长按）。hold_ms 传按住毫秒数；button 传 left/right/middle。",
			Parameters: map[string]any{"type": "object", "properties": map[string]any{"x": map[string]any{"type": "integer"}, "y": map[string]any{"type": "integer"}, "button": map[string]any{"type": "string", "description": "left/right/middle，默认 left"}, "hold_ms": map[string]any{"type": "integer", "description": "按住毫秒数"}}, "required": []string{"x", "y"}},
		},
	},
	"mouse_drag": {
		Type: "function",
		Function: desktop.FuncDef{
			Name: "mouse_drag", Description: "在焦点窗口内按住左键从 (x1,y1) 拖到 (x2,y2) 松开，用于画图/框选/拖动。",
			Parameters: map[string]any{"type": "object", "properties": map[string]any{"x1": map[string]any{"type": "integer"}, "y1": map[string]any{"type": "integer"}, "x2": map[string]any{"type": "integer"}, "y2": map[string]any{"type": "integer"}}, "required": []string{"x1", "y1", "x2", "y2"}},
		},
	},
	"move_mouse": {
		Type: "function",
		Function: desktop.FuncDef{
			Name: "move_mouse", Description: "仅把光标移动到窗口内相对坐标 (x,y)，不点击。",
			Parameters: map[string]any{"type": "object", "properties": map[string]any{"x": map[string]any{"type": "integer"}, "y": map[string]any{"type": "integer"}}, "required": []string{"x", "y"}},
		},
	},
	"scroll_wheel": {
		Type: "function",
		Function: desktop.FuncDef{
			Name: "scroll_wheel", Description: "在窗口内坐标 (x,y) 滚动滚轮。direction 传 up/down，amount 传格数。",
			Parameters: map[string]any{"type": "object", "properties": map[string]any{"x": map[string]any{"type": "integer"}, "y": map[string]any{"type": "integer"}, "direction": map[string]any{"type": "string", "description": "up/down"}, "amount": map[string]any{"type": "integer"}}, "required": []string{"x", "y", "direction"}},
		},
	},
	"uia_click": {
		Type: "function",
		Function: desktop.FuncDef{
			Name: "uia_click", Description: "对 UIA 元素执行激活（点击）。target 传元素名称子串或序号，control_type 传类型。",
			Parameters: map[string]any{"type": "object", "properties": map[string]any{"target": map[string]any{"type": "string", "description": "元素名称子串或序号"}, "control_type": map[string]any{"type": "string", "description": "控件类型"}}, "required": []string{"target"}},
		},
	},
	"uia_input": {
		Type: "function",
		Function: desktop.FuncDef{
			Name: "uia_input", Description: "向 UIA 元素写入文本。target 传元素名称子串或序号，text 传内容。",
			Parameters: map[string]any{"type": "object", "properties": map[string]any{"target": map[string]any{"type": "string", "description": "元素名称子串或序号"}, "text": map[string]any{"type": "string", "description": "要写入的文本"}}, "required": []string{"target", "text"}},
		},
	},
}
