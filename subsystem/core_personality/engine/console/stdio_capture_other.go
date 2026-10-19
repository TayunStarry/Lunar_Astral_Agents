//go:build !windows

package console

import "os"

// 进程级标准流接管（非 Windows 平台）：空实现。
//
// 该缺陷的根因只在 Windows 成立——TTS 引擎以 DLL 形式在进程内加载，其 CRT 的
// fprintf(stderr) 直写控制台设备句柄，绕过 Go 层接管（详见 stdio_capture_windows.go）。
// 非 Windows 平台上进程内原生代码不向终端写诊断，因此不去改动进程级文件描述符：
// 未经验证的重定向比不重定向更危险。
func captureNativeStdio(*os.File) string { return "" }
