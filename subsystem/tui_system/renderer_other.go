//go:build !windows

package TuiSystem

import (
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/x/term"
)

// detectTermCaps 在类 Unix 终端上判断颜色能力。
//
// VT 一律视为可用（POSIX 终端本就以转义序列为接口）；24 位色看 COLORTERM/TERM，
// 这是各终端事实上的通行约定。
func detectTermCaps(w io.Writer) termCaps {
	f, ok := w.(*os.File)
	if !ok || f == nil || !term.IsTerminal(f.Fd()) {
		return termCaps{}
	}
	ct := strings.ToLower(os.Getenv("COLORTERM"))
	tm := strings.ToLower(os.Getenv("TERM"))
	trueColor := ct == "truecolor" || ct == "24bit" || strings.Contains(tm, "direct")
	return termCaps{TTY: true, VT: true, TrueColor: trueColor}
}
