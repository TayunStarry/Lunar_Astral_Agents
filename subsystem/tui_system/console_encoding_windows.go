//go:build windows

package TuiSystem

// 控制台编码：把代码页切到 UTF-8。
//
// 为什么必须做：TUI 输出的是 UTF-8 字节。经典 conhost（双击 exe 或 conhost 承载）默认
// 用系统 OEM 代码页——中文 Windows 上是 936/GBK。不切代码页时，中文与 ╭ ─ │ ◆ 这类字形
// 会被按 GBK 逐字节解释：屏幕上全是乱码，而且「一个 UTF-8 字形」被解成别的字符后，实际
// 占位宽度与排版计算完全对不上，每一行都超出终端宽度 → 折行 → 整屏高度翻倍 → 备用屏幕
// 放不下就每帧向上滚。用户实测正是「一打开就滚动 + 显示混乱」。
//
// 输入侧一并切换：否则中文输入法与部分组合键会被输入法代码页吞掉。

import (
	"fmt"
	"os"
	"unsafe"
)

// kernel32 与 proc* 惰性加载（kernel32 变量已在 stats_windows.go 声明，此处只加过程）
var (
	procSetConsoleOutputCP = kernel32.NewProc("SetConsoleOutputCP")
	procSetConsoleCP       = kernel32.NewProc("SetConsoleCP")
	procGetConsoleOutputCP = kernel32.NewProc("GetConsoleOutputCP")
	procGetConsoleCP       = kernel32.NewProc("GetConsoleCP")
	procGetConsoleMode     = kernel32.NewProc("GetConsoleMode")
	procSetConsoleMode     = kernel32.NewProc("SetConsoleMode")
)

const (
	utf8CodePage = 65001
	// enableVirtualTerminalProcessing 让 conhost 解释 ANSI/VT 转义序列
	enableVirtualTerminalProcessing = 0x0004
)

// consoleEncoding 记录切换前的代码页，供退出时恢复
type consoleEncoding struct {
	outCP uint32
	inCP  uint32
}

// ensureConsoleEncoding 把控制台切换到 UTF-8 并开启 VT 处理，返回恢复函数。
//
// 非控制台句柄（管道/重定向）下调用会失败，此时静默返回空恢复函数——
// 编码不是我们要管的事，交给上游。
//
// 第二个返回值是环境指纹（代码页变化、是否 Windows Terminal），供日志留痕：
// 「显示乱码 / 错位」这类反馈的第一手证据就是它——经典 conhost（cmd 双击窗口）
// 默认 936，而 Windows Terminal 走 UTF-8，两者表现天差地别。
func ensureConsoleEncoding(f *os.File) (restore func(), fingerprint string) {
	noop := func() {}
	if f == nil {
		return noop, ""
	}

	var enc consoleEncoding
	if v, _, _ := procGetConsoleOutputCP.Call(); v != 0 {
		enc.outCP = uint32(v)
	}
	if v, _, _ := procGetConsoleCP.Call(); v != 0 {
		enc.inCP = uint32(v)
	}

	changed := false
	if enc.outCP != utf8CodePage {
		if r, _, _ := procSetConsoleOutputCP.Call(uintptr(utf8CodePage)); r != 0 {
			changed = true
		}
	}
	if enc.inCP != utf8CodePage {
		if r, _, _ := procSetConsoleCP.Call(uintptr(utf8CodePage)); r != 0 {
			changed = true
		}
	}

	// VT 处理：不加这一步，光标定位/清屏等转义序列在经典 conhost 上不生效，
	// 界面会「叠着画」。Windows Terminal 默认开启，失败可忽略。
	var mode uint32
	vtOK := false
	if r, _, _ := procGetConsoleMode.Call(uintptr(f.Fd()), uintptr(unsafe.Pointer(&mode))); r != 0 {
		if mode&enableVirtualTerminalProcessing == 0 {
			if r2, _, _ := procSetConsoleMode.Call(uintptr(f.Fd()), uintptr(mode|enableVirtualTerminalProcessing)); r2 != 0 {
				vtOK = true
			}
		} else {
			vtOK = true
		}
	}

	fingerprint = fmt.Sprintf("控制台环境: 代码页 %d→%d · VT=%v · Windows Terminal=%v",
		enc.outCP, currentOutputCP(), vtOK, isWindowsTerminal())

	if !changed {
		return noop, fingerprint
	}
	return func() {
		if enc.outCP != 0 && enc.outCP != utf8CodePage {
			procSetConsoleOutputCP.Call(uintptr(enc.outCP))
		}
		if enc.inCP != 0 && enc.inCP != utf8CodePage {
			procSetConsoleCP.Call(uintptr(enc.inCP))
		}
	}, fingerprint
}

// currentOutputCP 当前输出代码页
func currentOutputCP() uint32 {
	if v, _, _ := procGetConsoleOutputCP.Call(); v != 0 {
		return uint32(v)
	}
	return 0
}

// isWindowsTerminal 判断是否运行在 Windows Terminal（ConPTY）里。
//
// 判据用 WT_SESSION 环境变量：它是 Windows Terminal 注入的会话标识。
// 这个区分很重要——经典 conhost 与 Windows Terminal 在「代码页默认值、字体是否为
// 中文点阵、末列换行行为」上都不一样，同一份界面在两边可能一边正常一边乱码折行。
func isWindowsTerminal() bool {
	return os.Getenv("WT_SESSION") != ""
}
