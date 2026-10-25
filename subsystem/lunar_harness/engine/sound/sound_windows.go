//go:build windows

package sound

// sound_windows.go Windows 播放实现：winmm MCI（支持 MP3，绕开浏览器自动播放限制）。

import (
	"fmt"
	"sync/atomic"
	"syscall"
	"unsafe"
)

var (
	// winmmDLL Windows 多媒体库
	winmmDLL = syscall.NewLazyDLL("winmm.dll")
	// procMCISendStringW MCI 命令函数：播放 MP3 等 PlaySoundW 不支持的格式
	procMCISendStringW = winmmDLL.NewProc("mciSendStringW")
	// aliasSeq 每次播放使用独立别名，避免并发播放互相覆盖
	aliasSeq atomic.Int64
)

// playFile 播放指定提示音文件；wait 为 true 时阻塞到播放结束。
func playFile(name string, wait bool) bool {
	path, ok := audioPath(name)
	if !ok {
		return false
	}
	alias := fmt.Sprintf("lunarSound%d", aliasSeq.Add(1))
	if !mciCommand(fmt.Sprintf(`open "%s" type mpegvideo alias %s`, path, alias)) {
		return false
	}
	defer mciCommand("close " + alias)
	command := "play " + alias
	if wait {
		command += " wait"
	}
	return mciCommand(command)
}

// mciCommand 执行 MCI 命令，返回是否成功（MCIERROR 为 0）
func mciCommand(command string) bool {
	commandPtr, err := syscall.UTF16PtrFromString(command)
	if err != nil {
		return false
	}
	ret, _, _ := procMCISendStringW.Call(uintptr(unsafe.Pointer(commandPtr)), 0, 0, 0)
	return ret == 0
}
