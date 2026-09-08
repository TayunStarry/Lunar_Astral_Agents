package main

import (
	"LunarSubsystem/GeneralConfig"
	"LunarSubsystem/LoggerGeneral"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

func main() {
	flag.Parse()
	// 使用固定端点：GeneralConfig.EnginePort（不再随机分配端口）
	port := *GeneralConfig.EnginePort
	// 服务名称
	name := "< 星月智能 > 钛宇-琉璃 在此为您提供服务支持"
	// 启动服务（端口被占用时 StartServer 同步返回错误）
	if err := StartServer(port, Gethierarchy(), name); err != nil {
		LoggerGeneral.Error("CrystalAstral", "%s 启动失败: %v", name, err)
		// 端口被占用：先播放失败音效，再关闭程序
		playFailSound()
		os.Exit(1)
	}
}

// playFailSound 端口被占用时由后端播放失败音效 cartoon-fail.mp3
// PlaySoundW 仅支持 WAV，MP3 走 winmm 的 mciSendStringW（play wait 同步播放，播完再退出）
func playFailSound() {
	execPath, err := os.Executable()
	if err != nil {
		LoggerGeneral.Error("CrystalAstral", "定位可执行目录失败，无法播放失败音效: %v", err)
		return
	}
	execDir := filepath.Dir(execPath)
	audioPath := filepath.Join(execDir, *GeneralConfig.LocalDir, "audios", "cartoon-fail.mp3")
	if _, err := os.Stat(audioPath); err != nil {
		LoggerGeneral.Error("CrystalAstral", "失败音效文件不存在: %s", audioPath)
		return
	}
	const alias = "crystalFail"
	if ret := mciCommand(fmt.Sprintf("open \"%s\" type mpegvideo alias %s", audioPath, alias)); ret != 0 {
		LoggerGeneral.Error("CrystalAstral", "打开失败音效失败 (MCI 错误码 %d): %s", ret, audioPath)
		return
	}
	defer mciCommand(fmt.Sprintf("close %s", alias))
	if ret := mciCommand(fmt.Sprintf("play %s wait", alias)); ret != 0 {
		LoggerGeneral.Error("CrystalAstral", "播放失败音效失败 (MCI 错误码 %d): %s", ret, audioPath)
		return
	}
	LoggerGeneral.Info("CrystalAstral", "端口被占用，已播放失败音效: %s", audioPath)
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
