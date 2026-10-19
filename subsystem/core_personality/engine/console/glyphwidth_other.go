//go:build !windows

package console

import "os"

// calibrateGlyphWidths 非 Windows 平台不做字形宽度实测：
// 终端普遍按 Unicode 标准宽度渲染（歧义宽度取窄），与内置宽度表一致。
func calibrateGlyphWidths(f *os.File) map[rune]int {
	return nil
}
