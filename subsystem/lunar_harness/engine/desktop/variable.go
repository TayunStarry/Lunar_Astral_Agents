//go:build windows

package desktop

// ==== 全局变量与常量集中定义 ====
// 本包所有包级变量/常量统一在此声明，禁止与业务逻辑混在同一文件。

import (
	"sync"
	"syscall"

	uia "github.com/auuunya/go-element"
)

// builtinPrograms 常见系统程序的中文/口语别名 → 实际可执行文件名。
// 未覆盖的程序仍走开始菜单/安装目录搜索。
var builtinPrograms = map[string]string{
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

// knownProtocols 常见需用协议（URL scheme）启动的 UWP/商店应用映射。
var knownProtocols = map[string]string{
	"store": "ms-windows-store:",
	"商店":    "ms-windows-store:",
	"商城":    "ms-windows-store:",
}

// vkMap 常用键名到虚拟键码（VK）的映射表。
var vkMap = map[string]uint16{
	"enter": 0x0D, "return": 0x0D, "tab": 0x09, "esc": 0x1B, "escape": 0x1B,
	"backspace": 0x08, "delete": 0x2E, "del": 0x2E, "space": 0x20,
	"up": 0x26, "down": 0x28, "left": 0x25, "right": 0x27,
	"home": 0x24, "end": 0x23, "pageup": 0x21, "pagedown": 0x22,
	"ctrl": 0x11, "control": 0x11, "shift": 0x10, "alt": 0x12, "menu": 0x12,
	"win": 0x5B, "cmd": 0x5B, "meta": 0x5B,
	"f1": 0x70, "f2": 0x71, "f3": 0x72, "f4": 0x73, "f5": 0x74, "f6": 0x75,
	"f7": 0x76, "f8": 0x77, "f9": 0x78, "f10": 0x79, "f11": 0x7A, "f12": 0x7B,
}

// terminalProcs 常见终端/控制台进程，前置判定时需跳过。
var terminalProcs = map[string]bool{
	"powershell.exe": true, "pwsh.exe": true, "cmd.exe": true, "conhost.exe": true,
	"windowsterminal.exe": true, "wt.exe": true, "bash.exe": true, "wsl.exe": true,
}

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

// 截图坐标网格绘制参数。
const (
	// gridDivs 每轴的网格划分数。格数固定且较少，保证缩放图上格距足够大，缩小后依旧可辨。
	gridDivs = 10
	// gridLineW 网格线厚度（像素）。加粗绘制以增强截图/模型缩放后的可见性。
	gridLineW = 2
)

// uiaOnce UIA 客户端实例初始化一次。
var uiaOnce sync.Once

// uiaClient UIA 客户端实例（单例初始化）。
var uiaClient *uia.IUIAutomation

// 加载 user32.dll 库。
var user32Lib = syscall.NewLazyDLL("user32.dll")

// EnumWindows 函数指针。
var procEnumWindows = user32Lib.NewProc("EnumWindows")

// GetWindowTextW 函数指针。
var procGetWindowTextW = user32Lib.NewProc("GetWindowTextW")

// GetWindowTextLengthW 函数指针。
var procGetWindowTextLenW = user32Lib.NewProc("GetWindowTextLengthW")

// GetClassNameW 函数指针。
var procGetClassNameW = user32Lib.NewProc("GetClassNameW")

// mouse_event 函数指针。
var procMouseEvent = user32Lib.NewProc("mouse_event")

// keybd_event 函数指针。
var procKeybdEvent = user32Lib.NewProc("keybd_event")

// 加载 kernel32.dll 库。
var kernel32Lib = syscall.NewLazyDLL("kernel32.dll")

// OpenProcess 函数指针。
var procOpenProcess = kernel32Lib.NewProc("OpenProcess")

// QueryFullProcessImageNameW 函数指针。
var procQueryFullProcessImageName = kernel32Lib.NewProc("QueryFullProcessImageNameW")

// CloseHandle 函数指针。
var procCloseHandle = kernel32Lib.NewProc("CloseHandle")
