package main

import (
	"context"
	"flag"
	"fmt"
	"logger"
	"os"
	"qwen3_tts_lunar/module"
	"time"

	"config"
)

// main 主函数
func main() {
	addr := fmt.Sprintf(":%d", *config.BasicPort)
	modelDir := *config.LocalDir + "/models/Qwen3-TTS"
	refAudio := *config.LocalDir + "/audios/lunar-template.wav"
	flag.Parse()

	logger.SetDevMode(*config.Developer)

	if _, err := os.Stat(modelDir); os.IsNotExist(err) {
		logger.Error("QWEN-TTS", "模型目录不存在: %s", modelDir)
	}

	module.InitTTSEngine(modelDir, refAudio)

	logger.Info("QWEN-TTS", "监听端口: %s", addr)
	go startServer(addr)

	quit := setupSignalHandling()

	select {
	case <-quit:
		logger.Info("QWEN-TTS", "接收到中断信号，正在关闭...")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if httpServer != nil {
		if err := httpServer.Shutdown(ctx); err != nil {
			logger.Error("QWEN-TTS", "关闭失败: %v", err)
		}
	}

	shutdownServer()

	logger.Info("QWEN-TTS", "已成功关闭")
}
