//go:build windows

package desktop

// ==== 全局变量与常量集中定义 ====
// 项目所有包级变量/常量统一在此声明，禁止与业务逻辑混在同一文件。

import (
	"errors"
	"sync"
	"time"

	"LunarSubsystem/LunarHarness/protocol/agentinfo"
)

// DisplayName 导出、维修与工具目录展示用的显示名。
const DisplayName = agentinfo.NameDesktopOperator

// ErrCanceled 桌面操纵任务被调用方中断（控制台手动中断 / 对话者开始应答时的自动终止）。
var ErrCanceled = errors.New(DisplayName + "已中断")

// autoLoopMaxRetries 模型输出命中重复后，单次模型调用的额外重试次数。
const autoLoopMaxRetries = 2

// autoMaxLoops 执行循环上限：配合「连续无变化即停滞」检测，避免无限空转。
const autoMaxLoops = 10

// autoMaxAttempts 交接记录中保留的最近尝试条数（供规划者避免重复）。
const autoMaxAttempts = 6

// 各角色在「推理实况」中的标签。
const (
	autoRoleEditor   = "桌面操纵·编纂"
	autoRoleLauncher = "桌面操纵·启动"
	autoRoleUIA      = "桌面操纵·UIA"
	autoRolePlanner  = "桌面操纵·规划"
	autoRoleKeyboard = "桌面操纵·键盘"
	autoRoleMouse    = "桌面操纵·鼠标"
	autoRoleUIAOp    = "桌面操纵·UIA操作"
)

// 模型实况观察者：由 app 注入控制台的 StreamBegin/Delta/End 与模型计量。
var (
	autoObserverMu sync.Mutex
	autoObsBegin   func(role string)
	autoObsDelta   func(role, kind, text string)
	autoObsEnd     func(role string)
	autoObsTool    func(role, name, args string)
	autoObsMeter   func(prompt, completion, cached int64, generation time.Duration, estimated bool)
)

// 运行状态（Start / Stop / Snapshot）已上移到 chain/desktop，由桌控链统一维护。

const (
	// desktopCallTimeout 单个桌面原语（UIA / 键鼠 / 截图）的最长执行时间。
	desktopCallTimeout = 10 * time.Second
	// uiaDumpTimeout 读取 UIA 树的最长执行时间。
	uiaDumpTimeout = 5 * time.Second
)

// 各角色提示词统一存放在 prompts/ 下，运行期经 prompts.Read 加载（见 prompts.go）。
var (
	// promptEditor 提示词编纂者，负责将用户原始/口语化请求解析为结构化任务说明。
	promptEditor = mustPrompt("desktopEditor.md")
	// promptLauncher 软件启动者，负责按任务说明启动应用。
	promptLauncher = mustPrompt("desktopLauncher.md")
	// promptUIAReader UIA 理解者，负责用 UI Automation 读当前应用控件结构。
	promptUIAReader = mustPrompt("desktopUIAReader.md")
	// promptPlanner 任务规划者，负责综合 UIA 情报与真实窗口状态做出下一步决策。
	promptPlanner = mustPrompt("desktopPlanner.md")
	// promptKeyboard 键盘操作者，负责按规划键入文本/按键/组合键。
	promptKeyboard = mustPrompt("desktopKeyboard.md")
	// promptMouse 鼠标操作者，负责按规划点击/拖拽/滚动鼠标。
	promptMouse = mustPrompt("desktopMouse.md")
	// promptUIAOp UIA 操作者，负责按规划点击/写入 UI 元素。
	promptUIAOp = mustPrompt("desktopUIAOperator.md")
)

// 软件启动者允许使用的工具白名单。
var launcherTools = []string{"list_windows", "activate_window", "launch_program", "open_folder", "close_window"}

// UIA 理解者允许使用的工具白名单。
var uiaReaderTools = []string{"uia_dump", "uia_find"}

// 任务规划者允许使用的工具白名单。
var plannerTools = []string{}

// 键盘操作者允许使用的工具白名单。
var keyboardTools = []string{"type_text", "type_and_send", "press_key"}

// 鼠标操作者允许使用的工具白名单。
var mouseTools = []string{"click", "mouse_hold", "mouse_drag", "move_mouse", "scroll_wheel"}

// UIA 操作者允许使用的工具白名单。
var uiaOpTools = []string{"uia_click", "uia_input"}

// 进度书记者允许使用的工具白名单。
var scribeTools = []string{"capture_screenshot"}

// allToolDefs 全量工具定义表（名称 → function calling 描述），按角色白名单取子集。
var allToolDefs = map[string]ltpToolDef{
	"list_windows": {
		Type: "function",
		Function: ltpFuncDef{
			Name: "list_windows", Description: "枚举当前 Windows 桌面上的顶层窗口（带标题/进程名/类名/PID），并给出当前前台窗口。用于定位已打开的程序窗口。",
			Parameters: map[string]any{"type": "object", "properties": map[string]any{}, "required": []string{}},
		},
	},
	"activate_window": {
		Type: "function",
		Function: ltpFuncDef{
			Name: "activate_window", Description: "将指定标题或进程名的窗口置于前台并聚焦。title 传窗口标题关键字（如 记事本、Chrome）或进程名（如 QQ.exe）。",
			Parameters: map[string]any{"type": "object", "properties": map[string]any{"title": map[string]any{"type": "string", "description": "要激活的窗口标题/进程关键字"}}, "required": []string{"title"}},
		},
	},
	"launch_program": {
		Type: "function",
		Function: ltpFuncDef{
			Name: "launch_program", Description: "按名称搜索并启动 Windows 程序（开始菜单/常见目录/PATH/协议）；同名窗口已存在则直接激活。name 传程序名关键字（如 记事本/notepad/Chrome）。",
			Parameters: map[string]any{"type": "object", "properties": map[string]any{"name": map[string]any{"type": "string", "description": "要启动的程序名关键字"}}, "required": []string{"name"}},
		},
	},
	"open_folder": {
		Type: "function",
		Function: ltpFuncDef{
			Name: "open_folder", Description: "在文件资源管理器中直接打开指定文件夹绝对路径。path 传要打开的路径，如 D:\\Lunar_Astral_Agents。",
			Parameters: map[string]any{"type": "object", "properties": map[string]any{"path": map[string]any{"type": "string", "description": "文件夹绝对路径"}}, "required": []string{"path"}},
		},
	},
	"close_window": {
		Type: "function",
		Function: ltpFuncDef{
			Name: "close_window", Description: "关闭标题含指定关键字的所有顶层窗口（发送 WM_CLOSE）。title 传窗口标题/进程关键字。",
			Parameters: map[string]any{"type": "object", "properties": map[string]any{"title": map[string]any{"type": "string", "description": "要关闭的窗口标题/进程关键字"}}, "required": []string{"title"}},
		},
	},
	"type_text": {
		Type: "function",
		Function: ltpFuncDef{
			Name: "type_text", Description: "在当前焦点窗口模拟键入文本（支持中文与标点）。text 传要键入的内容。",
			Parameters: map[string]any{"type": "object", "properties": map[string]any{"text": map[string]any{"type": "string", "description": "要键入的文本"}}, "required": []string{"text"}},
		},
	},
	"type_and_send": {
		Type: "function",
		Function: ltpFuncDef{
			Name: "type_and_send", Description: "在焦点窗口键入文本并回车发送。text 传要发送的内容。",
			Parameters: map[string]any{"type": "object", "properties": map[string]any{"text": map[string]any{"type": "string", "description": "要键入并发送的文本"}}, "required": []string{"text"}},
		},
	},
	"press_key": {
		Type: "function",
		Function: ltpFuncDef{
			Name: "press_key", Description: "模拟按键/组合键，如 enter、tab、esc、ctrl+a、alt+f4、ctrl+tab。key 传键名或组合键；支持按住语义前缀 short: 短按1秒、long:/hold: 长按10秒（如 short:down 短按方向键）。",
			Parameters: map[string]any{"type": "object", "properties": map[string]any{"key": map[string]any{"type": "string", "description": "键名或组合键，可带 short:/long:/hold: 前缀"}}, "required": []string{"key"}},
		},
	},
	"click": {
		Type: "function",
		Function: ltpFuncDef{
			Name: "click", Description: "在焦点窗口内相对坐标 (x,y) 点击。button 传 left/right/middle（默认 left）；double=true 双击",
			Parameters: map[string]any{"type": "object", "properties": map[string]any{"x": map[string]any{"type": "integer", "description": "窗口内相对X"}, "y": map[string]any{"type": "integer", "description": "窗口内相对Y"}, "button": map[string]any{"type": "string", "description": "left/right/middle，默认 left"}, "double": map[string]any{"type": "boolean", "description": "是否双击"}}, "required": []string{"x", "y"}},
		},
	},
	"mouse_hold": {
		Type: "function",
		Function: ltpFuncDef{
			Name: "mouse_hold", Description: "在窗口内相对坐标 (x,y) 按住指定鼠标按键一段时长。用于右键/中键或按住（如拖住重命名、长按）。hold_ms 传按住毫秒数；button 传 left/right/middle。",
			Parameters: map[string]any{"type": "object", "properties": map[string]any{"x": map[string]any{"type": "integer"}, "y": map[string]any{"type": "integer"}, "button": map[string]any{"type": "string", "description": "left/right/middle，默认 left"}, "hold_ms": map[string]any{"type": "integer", "description": "按住毫秒数"}}, "required": []string{"x", "y"}},
		},
	},
	"mouse_drag": {
		Type: "function",
		Function: ltpFuncDef{
			Name: "mouse_drag", Description: "在焦点窗口内按住左键从 (x1,y1) 拖到 (x2,y2) 松开，用于画图/框选/拖动。",
			Parameters: map[string]any{"type": "object", "properties": map[string]any{"x1": map[string]any{"type": "integer"}, "y1": map[string]any{"type": "integer"}, "x2": map[string]any{"type": "integer"}, "y2": map[string]any{"type": "integer"}}, "required": []string{"x1", "y1", "x2", "y2"}},
		},
	},
	"move_mouse": {
		Type: "function",
		Function: ltpFuncDef{
			Name: "move_mouse", Description: "仅把光标移动到窗口内相对坐标 (x,y)，不点击。",
			Parameters: map[string]any{"type": "object", "properties": map[string]any{"x": map[string]any{"type": "integer"}, "y": map[string]any{"type": "integer"}}, "required": []string{"x", "y"}},
		},
	},
	"scroll_wheel": {
		Type: "function",
		Function: ltpFuncDef{
			Name: "scroll_wheel", Description: "在窗口内坐标 (x,y) 滚动滚轮。direction 传 up/down，amount 传格数。",
			Parameters: map[string]any{"type": "object", "properties": map[string]any{"x": map[string]any{"type": "integer"}, "y": map[string]any{"type": "integer"}, "direction": map[string]any{"type": "string", "description": "up/down"}, "amount": map[string]any{"type": "integer"}}, "required": []string{"x", "y", "direction"}},
		},
	},
	"uia_dump": {
		Type: "function",
		Function: ltpFuncDef{
			Name: "uia_dump", Description: "读取当前前台窗口的 UI 树（元素含名称/控件类型/id/是否禁用），供理解结构。",
			Parameters: map[string]any{"type": "object", "properties": map[string]any{}, "required": []string{}},
		},
	},
	"uia_find": {
		Type: "function",
		Function: ltpFuncDef{
			Name: "uia_find", Description: "在 UI 树中按名称子串或控件类型定向查找元素，返回带序号匹配列表。name 传名称，control_type 传类型（如 按钮/输入框）。",
			Parameters: map[string]any{"type": "object", "properties": map[string]any{"name": map[string]any{"type": "string", "description": "元素名称子串"}, "control_type": map[string]any{"type": "string", "description": "控件类型（按钮/输入框等）"}}, "required": []string{}},
		},
	},
	"uia_click": {
		Type: "function",
		Function: ltpFuncDef{
			Name: "uia_click", Description: "对 UIA 元素执行激活（点击）。target 传元素名称子串或序号，control_type 传类型。",
			Parameters: map[string]any{"type": "object", "properties": map[string]any{"target": map[string]any{"type": "string", "description": "元素名称子串或序号"}, "control_type": map[string]any{"type": "string", "description": "控件类型"}}, "required": []string{"target"}},
		},
	},
	"uia_input": {
		Type: "function",
		Function: ltpFuncDef{
			Name: "uia_input", Description: "向 UIA 元素写入文本。target 传元素名称子串或序号，text 传内容。",
			Parameters: map[string]any{"type": "object", "properties": map[string]any{"target": map[string]any{"type": "string", "description": "元素名称子串或序号"}, "text": map[string]any{"type": "string", "description": "要写入的文本"}}, "required": []string{"target", "text"}},
		},
	},
	"capture_screenshot": {
		Type: "function",
		Function: ltpFuncDef{
			Name: "capture_screenshot", Description: "截取当前窗口，叠加带原始窗口像素坐标（顶边 X 刻度、左边 Y 刻度）的高对比网格并刷新为最新截图，用于视觉定位（报告/使用网格刻度对应的窗口内相对坐标）与记录。",
			Parameters: map[string]any{"type": "object", "properties": map[string]any{}, "required": []string{}},
		},
	},
}
