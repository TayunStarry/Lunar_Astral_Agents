//go:build !windows

package app

// PlayStartupFailSound 非 Windows 平台无失败音效（启动失败信息经日志输出）
func PlayStartupFailSound() {}
