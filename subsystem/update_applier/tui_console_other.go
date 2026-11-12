//go:build !windows

package main

import "os"

// ensureConsole 非 Windows 平台无需切换代码页
func ensureConsole(out *os.File) func() { return func() {} }

// calibrateGlyphWidths 非 Windows 平台使用内置宽度表
func calibrateGlyphWidths(out *os.File) {}
