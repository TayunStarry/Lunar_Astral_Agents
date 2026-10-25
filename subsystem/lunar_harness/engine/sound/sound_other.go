//go:build !windows

package sound

// sound_other.go 非 Windows 平台不播放提示音（无 winmm 等价依赖，静默跳过）。
func playFile(_ string, _ bool) bool { return false }
