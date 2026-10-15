//go:build !windows

package main

// playFailSound 非 Windows 平台无失败音效（占位保持调用点一致）
func playFailSound() {}
