//go:build windows

package desktop

// ==== 全局变量与常量集中定义 ====
// 项目所有包级变量/常量统一在此声明，禁止与业务逻辑混在同一文件。

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"syscall"
	"time"

	uia "github.com/auuunya/go-element"
)

// ErrCanceled 桌面操纵任务被调用方中断（控制台手动中断 / 对话者开始应答时的自动终止）。
var ErrCanceled = errors.New("桌面操纵已中断")

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

// dtBuiltinPrograms 常见系统程序的中文/口语别名 → 实际可执行文件名。
// 未覆盖的程序仍走开始菜单/安装目录搜索。
var dtBuiltinPrograms = map[string]string{
	"记事本":      "notepad.exe",
	"notepad":  "notepad.exe",
	"计算器":      "calc.exe",
	"calc":     "calc.exe",
	"画图":       "mspaint.exe",
	"写字板":      "wordpad.exe",
	"cmd":      "cmd.exe",
	"命令提示符":    "cmd.exe",
	"资源管理器":    "explorer.exe",
	"explorer": "explorer.exe",
	"任务管理器":    "taskmgr.exe",
	"控制面板":     "control.exe",
	"截图工具":     "snippingtool.exe",
	"mspaint":  "mspaint.exe",
}

// runMu 保护运行状态与取消句柄（同一时间只允许一个任务）。
var runMu sync.Mutex

// runCancel 取消当前运行中的任务（nil 表示没有任务在运行）。
var runCancel context.CancelFunc

// runDone 当前任务的退出信号；任务 goroutine 在真正收尾时关闭。
var runDone chan struct{}

// runSeq 运行代次：强制结束后递增，用于忽略旧任务迟到的收尾。
var runSeq int64

// runState 当前/最近一次运行状态。
var runState RunState

const (
	// stopWaitTimeout 请求中断后等待任务自行退出的时长；超时则强制收尾。
	stopWaitTimeout = 4 * time.Second
	// desktopCallTimeout 单个桌面原语（UIA / 键鼠 / 截图）的最长执行时间。
	desktopCallTimeout = 10 * time.Second
	// uiaDumpTimeout 读取 UIA 树的最长执行时间。
	uiaDumpTimeout = 5 * time.Second
)

// 模型配置缓存：惰性读取一次后端 lunar_config.json 中 agent 多模态字段。
var autoConfigOnce sync.Once

// autoModel 模型名称
var autoModel string

// autoURL 模型 API 地址
var autoURL string

// autoHTTPClient 共享的模型 API 客户端，带 120 秒超时。
var autoHTTPClient = &http.Client{Timeout: 120 * time.Second}

// uiaOnceDT UIA 客户端实例初始化一次
var uiaOnceDT sync.Once

// uiaClientDT UIA 客户端实例（单例初始化）。
var uiaClientDT *uia.IUIAutomation

// 加载 user32.dll 库
var user32SW = syscall.NewLazyDLL("user32.dll")

// 获取 EnumWindows 函数指针
var procEnumWindowsSW = user32SW.NewProc("EnumWindows")

// 获取 GetWindowTextW 函数指针
var procGetWindowTextW_SW = user32SW.NewProc("GetWindowTextW")

// 获取 GetWindowTextLengthW 函数指针
var procGetWindowTextLenW_SW = user32SW.NewProc("GetWindowTextLengthW")

// 获取 GetClassNameW 函数指针
var procGetClassNameW_SW = user32SW.NewProc("GetClassNameW")

// 获取 mouse_event 函数指针
var procMouseEventSW = user32SW.NewProc("mouse_event")

// 获取 keybd_event 函数指针
var procKeybdEventSW = user32SW.NewProc("keybd_event")

// 加载 kernel32.dll 库
var kernel32SW = syscall.NewLazyDLL("kernel32.dll")

// 获取 OpenProcess 函数指针
var procOpenProcessSW = kernel32SW.NewProc("OpenProcess")

// 获取 QueryFullProcessImageNameW 函数指针
var procQueryFullProcessImageNameSW = kernel32SW.NewProc("QueryFullProcessImageNameW")

// 获取 CloseHandle 函数指针
var procCloseHandleSW = kernel32SW.NewProc("CloseHandle")

// mouse_event 使用的按键动作标志。
const (
	mswLeftdown   = 0x0002 // 左键按下
	mswLeftup     = 0x0004 // 左键松开
	mswRightdown  = 0x0008 // 右键按下
	mswRightup    = 0x0010 // 右键松开
	mswMiddledown = 0x0020 // 中键按下
	mswMiddleup   = 0x0040 // 中键松开
	mswWheel      = 0x0800 // 滚轮轮按下
	mswKeyup      = 0x0002 // 键松开
)

// dtKnownProtocols 常见需用协议（URL scheme）启动的 UWP/商店应用映射。
var dtKnownProtocols = map[string]string{
	"store": "ms-windows-store:",
	"商店":    "ms-windows-store:",
	"商城":    "ms-windows-store:",
}

// dtVkMap 常用键名到虚拟键码（VK）的映射表。
var dtVkMap = map[string]uint16{
	"enter": 0x0D, "return": 0x0D, "tab": 0x09, "esc": 0x1B, "escape": 0x1B,
	"backspace": 0x08, "delete": 0x2E, "del": 0x2E, "space": 0x20,
	"up": 0x26, "down": 0x28, "left": 0x25, "right": 0x27,
	"home": 0x24, "end": 0x23, "pageup": 0x21, "pagedown": 0x22,
	"ctrl": 0x11, "control": 0x11, "shift": 0x10, "alt": 0x12, "menu": 0x12,
	"win": 0x5B, "cmd": 0x5B, "meta": 0x5B,
	"f1": 0x70, "f2": 0x71, "f3": 0x72, "f4": 0x73, "f5": 0x74, "f6": 0x75,
	"f7": 0x76, "f8": 0x77, "f9": 0x78, "f10": 0x79, "f11": 0x7A, "f12": 0x7B,
}

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
