//go:build windows

package main

// ==== 失败音效（winmm MCI 播放，与主程序启动失败音效同源） ====
//
// 更新器未以运行参数启动（误双击等场景）时播放失败音效提示用户后退出。
// 音效文件与主程序共用：程序根目录下 local_data/audios/cartoon-fail.mp3。

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

// winmmDLL Windows 多媒体库（winmm.dll），提供 mciSendStringW
var winmmDLL = syscall.NewLazyDLL("winmm.dll")

// procMCISendStringW MCI 命令函数句柄：播放 MP3 等 PlaySoundW 不支持的格式
var procMCISendStringW = winmmDLL.NewProc("mciSendStringW")

// playFailSound 播放失败音效 cartoon-fail.mp3（同步播完）；文件不存在时静默跳过
func playFailSound() {
	audioPath := filepath.Join(defaultTarget(), "local_data", "audios", "cartoon-fail.mp3")
	if _, err := os.Stat(audioPath); err != nil {
		return
	}

	const alias = "applierFail"
	if ret := mciCommand(fmt.Sprintf(`open "%s" type mpegvideo alias %s`, audioPath, alias)); ret != 0 {
		return
	}
	defer mciCommand("close " + alias)
	mciCommand("play " + alias + " wait")
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
