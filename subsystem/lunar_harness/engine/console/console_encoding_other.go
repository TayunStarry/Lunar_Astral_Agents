//go:build !windows

package console

import "os"

// ensureConsoleEncoding 非 Windows 平台无需切换代码页（终端普遍按 UTF-8 解释）
func ensureConsoleEncoding(f *os.File) (restore func(), fingerprint string) {
	return func() {}, ""
}
