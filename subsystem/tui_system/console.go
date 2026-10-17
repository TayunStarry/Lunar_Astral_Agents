package TuiSystem

// ==== 控制台句柄自愈 ====
//
// 控制台界面依赖「输出/输入句柄确实是控制台」。但这条前提在自动重启链路上并不可靠：
// 更新器（或任何上游）可能把标准流换成管道、NUL 设备，甚至以**只写**方式打开 CONOUT$——
// 而 GetConsoleMode / GetConsoleScreenBufferInfo 都要求 **读** 权限，只写句柄一律
// "Access is denied"，于是界面既画不出来、也量不到尺寸（实测故障）。
//
// 结论：不要相信继承来的句柄。只要本进程还挂在某个控制台上，就自己重新取一个
// 读写句柄（CONOUT$/CONIN$），把「窗口一闪就没」变成「照常显示」。

import (
	"fmt"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/term"
)

// terminalSizePollInterval 终端尺寸轮询间隔（Windows 上 bubbletea 不会主动上报缩放）。
// 允许测试改小以加快用例。
var terminalSizePollInterval = 250 * time.Millisecond

// isConsoleFile 句柄是否为可用的控制台句柄。
// 注意：只写打开的 CONOUT$ 会在这里返回 false——这已是本文件要处理的输入条件之一。
func isConsoleFile(f *os.File) bool {
	return f != nil && term.IsTerminal(f.Fd())
}

// consoleOut 取控制台输出句柄。
// 优先 O_RDWR：只写句柄过不了 GetConsoleMode/尺寸查询；取不到再退而求其次用只写。
func consoleOut() *os.File {
	if f, err := os.OpenFile("CONOUT$", os.O_RDWR, 0); err == nil {
		return f
	}
	if f, err := os.OpenFile("CONOUT$", os.O_WRONLY, 0); err == nil {
		return f
	}
	return nil
}

// consoleIn 取控制台输入句柄（键盘）。
// 必须 O_RDWR：bubbletea 进入原始模式要调 SetConsoleMode（需写权限），
// 只用 O_RDONLY 打开会得到 "error making raw: Access is denied"（实测故障）。
func consoleIn() *os.File {
	if f, err := os.OpenFile("CONIN$", os.O_RDWR, 0); err == nil {
		return f
	}
	if f, err := os.OpenFile("CONIN$", os.O_RDONLY, 0); err == nil {
		return f
	}
	return nil
}

// resolveRenderTarget 决定 TUI 的渲染目标：自取的控制台句柄优先，取不到才退回继承的 stdout
//
// 为什么优先自取：Install 随后会把**进程级**标准流（STD_OUTPUT/STD_ERROR 句柄槽与 CRT 的
// fd 1/2）也接到日志管道上，用于拦下进程内原生库（TTS 引擎 DLL）直写终端的诊断——那一步会
// 关掉 CRT 表里原有的继承句柄；渲染目标若正是那个句柄，接管瞬间界面就再无输出。
// 自取的 CONOUT$ 句柄由本进程独立持有，不参与标准流接管，因此不受影响（与上文
// 「不要相信继承来的句柄」同一结论）。
//
// 两者都取不到时（服务/计划任务等无控制台环境）退回继承句柄，此时界面不可见，
// 由 Run 中的终端守卫给出明确提示后退出。
func resolveRenderTarget() interface{ Write([]byte) (int, error) } {
	if f := consoleOut(); f != nil {
		return f
	}
	return os.Stdout
}

// resolveInputTarget 决定 TUI 的输入来源：继承的 stdin 可用就用它，否则用 CONIN$。
func resolveInputTarget() *os.File {
	if isConsoleFile(os.Stdin) {
		return os.Stdin
	}
	if f := consoleIn(); f != nil {
		return f
	}
	return os.Stdin
}

// consoleSize 读取终端可视窗口尺寸（列 × 行）；取不到时返回 ok=false
func consoleSize(f *os.File) (w, h int, ok bool) {
	if !isConsoleFile(f) {
		return 0, 0, false
	}
	w, h, err := term.GetSize(f.Fd())
	if err != nil || w <= 0 || h <= 0 {
		return 0, 0, false
	}
	return w, h, true
}

// watchTerminalSize 轮询终端尺寸，变化时投递 WindowSizeMsg。
//
// 为什么必须自己轮询：bubbletea 在 Windows 上没有 SIGWINCH（signals_windows.go 里
// listenForResize 直接关闭），**尺寸只在启动时取一次**。用户一旦把窗口改小，程序仍按
// 旧尺寸渲染整屏——每帧都高出可视区域，而终端的备用屏幕放不下就会向上滚：表现为
// 「控制台不停向上滚、把界面滚没了」，而且因为尺寸再也不会被重新读取，它永远不会恢复。
//
// 顺带补上了 Windows 上另一个缺口：目前 TUI 完全无法跟随窗口缩放（放大了也不会用上多出来的空间）。
//
// send 与 size 以参数注入，便于单测直接喂入尺寸序列验证「变化必须上报」。
func watchTerminalSize(send func(tea.Msg), size func() (int, int, bool), stop <-chan struct{}) {
	lastW, lastH, ok := size()
	if !ok {
		return
	}
	ticker := time.NewTicker(terminalSizePollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			w, h, valid := size()
			if !valid || (w == lastW && h == lastH) {
				continue
			}
			prevW, prevH := lastW, lastH
			lastW, lastH = w, h
			// 记一条日志：既方便用户自救（缩放后应看到这条），也让「控制台自行滚动」
			// 这类反馈能在 console.log 里对时间线（尺寸何时变、界面何时该重排）。
			// 经 pushScrubbedLog 入缓冲：入口统一，避免出现"某条路径没清洗"的两套标准。
			pushScrubbedLog(SourceRaw, "RAW", "", "",
				fmt.Sprintf("[TUI] 终端尺寸变化: %dx%d → %dx%d，界面按新尺寸重排", prevW, prevH, w, h))
			// 程序退出后 Send 可能无人接收：先看停止信号，避免 goroutine 卡在发送上
			select {
			case <-stop:
				return
			default:
			}
			send(tea.WindowSizeMsg{Width: w, Height: h})
		}
	}
}
