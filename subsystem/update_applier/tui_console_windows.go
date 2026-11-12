//go:build windows

package main

// ==== Windows 控制台适配 ====
//
// 1) 代码页切 UTF-8：否则双击启动时 conhost 按 GBK 解释 UTF-8 字节，界面全乱；
// 2) 开启 VT 处理：否则光标定位/清屏不生效，界面叠着画；
// 3) 实测歧义字形宽度：中文点阵字体下部分符号占 2 列，按内置宽度表排版会错位。

import (
	"os"
	"syscall"
	"unsafe"
)

var (
	kernel32                          = syscall.NewLazyDLL("kernel32.dll")
	procSetConsoleOutputCP            = kernel32.NewProc("SetConsoleOutputCP")
	procSetConsoleCP                  = kernel32.NewProc("SetConsoleCP")
	procGetConsoleOutputCP            = kernel32.NewProc("GetConsoleOutputCP")
	procGetConsoleCP                  = kernel32.NewProc("GetConsoleCP")
	procGetConsoleMode                = kernel32.NewProc("GetConsoleMode")
	procSetConsoleMode                = kernel32.NewProc("SetConsoleMode")
	procGetConsoleScreenBufferInfo    = kernel32.NewProc("GetConsoleScreenBufferInfo")
	procReadConsoleOutputCharacterW   = kernel32.NewProc("ReadConsoleOutputCharacterW")
	procWriteConsoleOutputCharacterW  = kernel32.NewProc("WriteConsoleOutputCharacterW")
	procReadConsoleOutputAttribute    = kernel32.NewProc("ReadConsoleOutputAttribute")
	procWriteConsoleOutputAttribute   = kernel32.NewProc("WriteConsoleOutputAttribute")
	procSetConsoleCursorPosition      = kernel32.NewProc("SetConsoleCursorPosition")
)

const (
	utf8CodePage                    = 65001
	enableVirtualTerminalProcessing = 0x0004
)

type coordT struct{ X, Y int16 }
type smallRectT struct{ Left, Top, Right, Bottom int16 }
type csbiT struct {
	Size              coordT
	CursorPosition    coordT
	Attributes        uint16
	Window            smallRectT
	MaximumWindowSize coordT
}

// ensureConsole 切换 UTF-8 与 VT，返回恢复函数
func ensureConsole(out *os.File) func() {
	noop := func() {}
	if out == nil {
		return noop
	}
	oldOut, _, _ := procGetConsoleOutputCP.Call()
	oldIn, _, _ := procGetConsoleCP.Call()
	procSetConsoleOutputCP.Call(uintptr(utf8CodePage))
	procSetConsoleCP.Call(uintptr(utf8CodePage))

	var mode uint32
	if r, _, _ := procGetConsoleMode.Call(out.Fd(), uintptr(unsafe.Pointer(&mode))); r != 0 {
		procSetConsoleMode.Call(out.Fd(), uintptr(mode|enableVirtualTerminalProcessing))
	}

	return func() {
		if oldOut != 0 && oldOut != utf8CodePage {
			procSetConsoleOutputCP.Call(oldOut)
		}
		if oldIn != 0 && oldIn != utf8CodePage {
			procSetConsoleCP.Call(oldIn)
		}
	}
}

// calibrateGlyphWidths 在真实控制台上实测字形宽度并写入宽字形表
func calibrateGlyphWidths(out *os.File) {
	if out == nil {
		return
	}
	handle := syscall.Handle(out.Fd())
	var csbi csbiT
	if r, _, _ := procGetConsoleScreenBufferInfo.Call(uintptr(handle), uintptr(unsafe.Pointer(&csbi))); r == 0 {
		return
	}
	width := int(csbi.Window.Right-csbi.Window.Left) + 1
	height := int(csbi.Window.Bottom-csbi.Window.Top) + 1
	if width < 8 || height < 2 {
		return
	}

	type savedRow struct {
		y     int
		chars []uint16
		attrs []uint16
	}
	var saved []savedRow
	rowsNeeded := (len(probeGlyphs)*2 + width - 1) / width
	if rowsNeeded > height {
		rowsNeeded = height
	}
	for i := 0; i < rowsNeeded; i++ {
		chars := make([]uint16, width)
		attrs := make([]uint16, width)
		var got uint32
		pos := uintptr(uint32(uint16(i)) << 16)
		if r, _, _ := procReadConsoleOutputCharacterW.Call(uintptr(handle),
			uintptr(unsafe.Pointer(&chars[0])), uintptr(width), pos,
			uintptr(unsafe.Pointer(&got))); r == 0 {
			return
		}
		if r, _, _ := procReadConsoleOutputAttribute.Call(uintptr(handle),
			uintptr(unsafe.Pointer(&attrs[0])), uintptr(width), pos,
			uintptr(unsafe.Pointer(&got))); r == 0 {
			return
		}
		saved = append(saved, savedRow{y: i, chars: chars, attrs: attrs})
	}
	defer func() {
		for _, row := range saved {
			var written uint32
			pos := uintptr(uint32(uint16(row.y)) << 16)
			procWriteConsoleOutputCharacterW.Call(uintptr(handle),
				uintptr(unsafe.Pointer(&row.chars[0])), uintptr(width), pos,
				uintptr(unsafe.Pointer(&written)))
			procWriteConsoleOutputAttribute.Call(uintptr(handle),
				uintptr(unsafe.Pointer(&row.attrs[0])), uintptr(width), pos,
				uintptr(unsafe.Pointer(&written)))
		}
		procSetConsoleCursorPosition.Call(uintptr(handle),
			uintptr(uint32(uint16(csbi.CursorPosition.X))|uint32(uint16(csbi.CursorPosition.Y))<<16))
	}()

	x, y := 0, 0
	for _, g := range probeGlyphs {
		if x >= width-4 {
			if y+1 >= rowsNeeded {
				break
			}
			y++
			x = 0
		}
		procSetConsoleCursorPosition.Call(uintptr(handle), uintptr(uint32(uint16(x))|uint32(uint16(y))<<16))
		var written uint32
		if err := syscall.WriteFile(handle, []byte(string(g)), &written, nil); err != nil {
			continue
		}
		var after csbiT
		if r, _, _ := procGetConsoleScreenBufferInfo.Call(uintptr(handle), uintptr(unsafe.Pointer(&after))); r == 0 {
			continue
		}
		if int(after.CursorPosition.Y) != y {
			continue
		}
		w := int(after.CursorPosition.X) - x
		if w >= 1 && w <= 4 {
			setMeasuredGlyphWidth(g, w)
		}
		x += w
	}
}
