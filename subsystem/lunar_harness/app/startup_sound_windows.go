//go:build windows

package app

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/logger"
)

// ==== winmm 播放（后端播放失败音效，绕开浏览器自动播放限制） ====

// winmmDLL Windows 多媒体库（winmm.dll），提供 mciSendStringW
var winmmDLL = syscall.NewLazyDLL("winmm.dll")

// procMCISendStringW MCI 命令函数句柄：播放 MP3 等 PlaySoundW 不支持的格式
var procMCISendStringW = winmmDLL.NewProc("mciSendStringW")

// PlayStartupFailSound 启动失败时播放失败音效 cartoon-fail.mp3。
// Windows 桌面构建使用 -H windowsgui 隐藏控制台，启动失败无可见输出，
// 因此以可听音效提示用户，并同时落一条日志记录。
func PlayStartupFailSound() {
	execPath, err := os.Executable()
	if err != nil {
		logger.Error("LunarHarness", "定位可执行目录失败，无法播放失败音效: %v", err)
		return
	}
	execDir := filepath.Dir(execPath)
	audioPath := filepath.Join(execDir, config.LocalDir, "audios", "cartoon-fail.mp3")
	if _, err := os.Stat(audioPath); err != nil {
		logger.Error("LunarHarness", "失败音效文件不存在: %s", audioPath)
		return
	}

	const alias = "coreFail"
	if ret := mciCommand(fmt.Sprintf("open \"%s\" type mpegvideo alias %s", audioPath, alias)); ret != 0 {
		logger.Error("LunarHarness", "打开失败音效失败 (MCI 错误码 %d): %s", ret, audioPath)
		return
	}
	defer mciCommand(fmt.Sprintf("close %s", alias))
	if ret := mciCommand(fmt.Sprintf("play %s wait", alias)); ret != 0 {
		logger.Error("LunarHarness", "播放失败音效失败 (MCI 错误码 %d): %s", ret, audioPath)
		return
	}
	logger.Info("LunarHarness", "启动失败，已播放失败音效: %s", audioPath)
}

// mciCommand 执行 MCI 命令（mciSendStringW），返回 MCIERROR（0 表示成功）
func mciCommand(cmd string) uintptr {
	cmdPtr, err := syscall.UTF16PtrFromString(cmd)
	if err != nil {
		return 1
	}
	ret, _, _ := procMCISendStringW.Call(uintptr(unsafe.Pointer(cmdPtr)), 0, 0, 0)
	return ret
}
