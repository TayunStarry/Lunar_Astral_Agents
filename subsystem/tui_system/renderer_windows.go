//go:build windows

package TuiSystem

import (
	"io"
	"os"
	"unsafe"
)

// detectTermCaps 在 Windows 上按「控制台模式 + 终端类型」判断颜色能力。
//
// 判据刻意选**控制台模式**而不是 isatty：传入的 w 通常是我们自己另取的 CONOUT$ 句柄
// （console.go），它同时能回答「这是不是控制台」和「ANSI 处理开没开」两件事——
// 后者是写 SGR 的前提，没开时彩色序列会原样打在屏幕上。
func detectTermCaps(w io.Writer) termCaps {
	f, ok := w.(*os.File)
	if !ok || f == nil {
		return termCaps{}
	}
	var mode uint32
	if r, _, _ := procGetConsoleMode.Call(uintptr(f.Fd()), uintptr(unsafe.Pointer(&mode))); r == 0 {
		return termCaps{}
	}
	return termCaps{
		TTY: true,
		VT:  mode&enableVirtualTerminalProcessing != 0,
		// 24 位色只在 Windows Terminal 上启用：经典 conhost 的 24 位色支持随版本波动，
		// 而本程序色板本来就全是 256 色索引，提到 TrueColor 没有收益、只有风险。
		TrueColor: isWindowsTerminal(),
	}
}
