package main

import (
	"LunarSubsystem/GeneralConfig"
	"LunarSubsystem/LoggerGeneral"
	"LunarSubsystem/Qwen3-TTS/module"
	"context"
	"flag"
	"fmt"
	"os"
	"time"
)

// main 主函数
func main() {
	addr := fmt.Sprintf(":%d", *GeneralConfig.BasicPort)
	modelDir := *GeneralConfig.LocalDir + "/models/Qwen3-TTS"
	refAudio := *GeneralConfig.LocalDir + "/audios/lunar-template.wav"
	flag.Parse()

	LoggerGeneral.SetDevMode(*GeneralConfig.Developer, "local_data/documents/debug")

	if _, err := os.Stat(modelDir); os.IsNotExist(err) {
		LoggerGeneral.Error("QWEN-TTS", "模型目录不存在: %s", modelDir)
	}

	module.InitTTSEngine(modelDir, refAudio)

	LoggerGeneral.Info("QWEN-TTS", "监听端口: %s", addr)
	go startServer(addr)

	quit := setupSignalHandling()

	<-quit
	LoggerGeneral.Info("QWEN-TTS", "接收到中断信号，正在关闭...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if httpServer != nil {
		if err := httpServer.Shutdown(ctx); err != nil {
			LoggerGeneral.Error("QWEN-TTS", "关闭失败: %v", err)
		}
	}

	shutdownServer()

	LoggerGeneral.Info("QWEN-TTS", "已成功关闭")
}
