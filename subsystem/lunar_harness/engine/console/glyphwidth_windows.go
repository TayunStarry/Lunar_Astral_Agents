//go:build windows

package console

// 在真实控制台上实测字形占位宽度。
//
// 做法：把光标移到被覆盖区域起点，用 WriteFile 写入 1 个字形（走控制台输出路径，
// 光标会按字形实际宽度前进），读光标 X 位移即得占位宽度；探测结束后把碰到的单元格
// （字符 + 属性）与光标位置原样恢复，不留下痕迹。
//
// 注意：必须先把代码页切成 UTF-8 再探测（ensureConsoleEncoding），否则写进去的
// UTF-8 字节会被按 OEM 代码页解释，测出来的是乱码宽度。

import (
	"os"
	"syscall"
	"unsafe"
)

var (
	procReadConsoleOutputCharacterW  = kernel32.NewProc("ReadConsoleOutputCharacterW")
	procWriteConsoleOutputCharacterW = kernel32.NewProc("WriteConsoleOutputCharacterW")
	procReadConsoleOutputAttribute   = kernel32.NewProc("ReadConsoleOutputAttribute")
	procWriteConsoleOutputAttribute  = kernel32.NewProc("WriteConsoleOutputAttribute")
	procSetConsoleCursorPosition     = kernel32.NewProc("SetConsoleCursorPosition")
	procGetConsoleScreenBufferInfo   = kernel32.NewProc("GetConsoleScreenBufferInfo")
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

// calibrateGlyphWidths 实测字形宽度；句柄不可用或环境异常时返回 nil（行为退回内置宽度表）
func calibrateGlyphWidths(f *os.File) map[rune]int {
	if f == nil {
		return nil
	}
	handle := syscall.Handle(f.Fd())

	var csbi csbiT
	r, _, _ := procGetConsoleScreenBufferInfo.Call(uintptr(handle), uintptr(unsafe.Pointer(&csbi)))
	if r == 0 {
		return nil
	}
	width := int(csbi.Window.Right-csbi.Window.Left) + 1
	height := int(csbi.Window.Bottom-csbi.Window.Top) + 1
	if width < 8 || height < 2 {
		return nil
	}

	glyphs := probeGlyphs()
	// 探测区：从窗口首行开始（TUI 随即进入备用屏幕，主缓冲区内容不会被用户看到）
	y := 0
	x := 0

	// 保存探测区（字符 + 属性）以便原样恢复
	type savedRow struct {
		y     int
		chars []uint16
		attrs []uint16
	}
	var saved []savedRow
	rowsNeeded := (len(glyphs)*2 + width - 1) / width
	if rowsNeeded > height {
		rowsNeeded = height
	}
	for i := 0; i < rowsNeeded; i++ {
		chars := make([]uint16, width)
		attrs := make([]uint16, width)
		var got uint32
		pos := uint32(0) | uint32(uint16(i))<<16
		if rr, _, _ := procReadConsoleOutputCharacterW.Call(uintptr(handle),
			uintptr(unsafe.Pointer(&chars[0])), uintptr(width), uintptr(pos),
			uintptr(unsafe.Pointer(&got))); rr == 0 {
			return nil
		}
		if rr, _, _ := procReadConsoleOutputAttribute.Call(uintptr(handle),
			uintptr(unsafe.Pointer(&attrs[0])), uintptr(width), uintptr(pos),
			uintptr(unsafe.Pointer(&got))); rr == 0 {
			return nil
		}
		saved = append(saved, savedRow{y: i, chars: chars, attrs: attrs})
	}
	defer func() {
		// 恢复字符与属性，再把光标放回原处
		for _, row := range saved {
			var written uint32
			pos := uint32(0) | uint32(uint16(row.y))<<16
			procWriteConsoleOutputCharacterW.Call(uintptr(handle),
				uintptr(unsafe.Pointer(&row.chars[0])), uintptr(width), uintptr(pos),
				uintptr(unsafe.Pointer(&written)))
			procWriteConsoleOutputAttribute.Call(uintptr(handle),
				uintptr(unsafe.Pointer(&row.attrs[0])), uintptr(width), uintptr(pos),
				uintptr(unsafe.Pointer(&written)))
		}
		procSetConsoleCursorPosition.Call(uintptr(handle),
			uintptr(uint32(uint16(csbi.CursorPosition.X))|uint32(uint16(csbi.CursorPosition.Y))<<16))
	}()

	measured := make(map[rune]int, len(glyphs))
	for _, g := range glyphs {
		if x >= width-4 {
			if y+1 >= rowsNeeded {
				break
			}
			y++
			x = 0
		}
		moveCursor(handle, x, y)
		if err := writeConsoleString(handle, string(g)); err != nil {
			continue
		}
		var after csbiT
		if rr, _, _ := procGetConsoleScreenBufferInfo.Call(uintptr(handle), uintptr(unsafe.Pointer(&after))); rr == 0 {
			continue
		}
		// 光标应停留在同一行；跨行说明探测位置太靠右，本字形跳过
		if int(after.CursorPosition.Y) != y {
			continue
		}
		w := int(after.CursorPosition.X) - x
		if w >= 1 && w <= 4 {
			measured[g] = w
		}
		x += w
	}
	if len(measured) == 0 {
		return nil
	}
	return measured
}

// moveCursor 定位光标
func moveCursor(handle syscall.Handle, x, y int) {
	procSetConsoleCursorPosition.Call(uintptr(handle), uintptr(uint32(uint16(x))|uint32(uint16(y))<<16))
}

// readConsoleRow 读某行前 n 个字符（测试与恢复校验用）
func readConsoleRow(f *os.File, y, n int) string {
	buf := make([]uint16, n)
	var got uint32
	pos := uint32(0) | uint32(uint16(y))<<16
	r, _, _ := procReadConsoleOutputCharacterW.Call(uintptr(syscall.Handle(f.Fd())),
		uintptr(unsafe.Pointer(&buf[0])), uintptr(n), uintptr(pos), uintptr(unsafe.Pointer(&got)))
	if r == 0 {
		return ""
	}
	return syscall.UTF16ToString(buf[:got])
}

// writeConsoleString 通过控制台输出路径写入字符串（光标会随之前进）
func writeConsoleString(handle syscall.Handle, s string) error {
	// 控制台默认以 UTF-16 交互，但 WriteFile 走的是已设代码页（UTF-8）的字节流
	h := syscall.Handle(handle)
	var written uint32
	return syscall.WriteFile(h, []byte(s), &written, nil)
}
