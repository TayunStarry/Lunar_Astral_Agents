package main

import (
	"LunarSubsystem/GeneralConfig"
	"LunarSubsystem/Kokoro-TTS/module"
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"LunarSubsystem/LoggerGeneral"
)

// main 主函数
func main() {
	modelDir := filepath.Join(*GeneralConfig.LocalDir, "models", "Kokoro-82M-v1.1-zh")
	flag.Parse()

	LoggerGeneral.SetDevMode(*GeneralConfig.Developer)

	// 指定 onnxruntime.dll 路径（随模型目录放置）
	dllPath := filepath.Join(modelDir, "onnxruntime.dll")
	if _, err := os.Stat(dllPath); err == nil {
		module.SetOnnxLibraryPath(dllPath)
		LoggerGeneral.Info("KOKORO-TTS", "使用 onnxruntime.dll: %s", dllPath)
	}

	if err := module.InitEngine(modelDir, filepath.Join(modelDir, "voices"), filepath.Join(modelDir, "espeak")); err != nil {
		LoggerGeneral.Error("KOKORO-TTS", "引擎初始化失败: %v", err)
		os.Exit(1)
	}

	engine := module.GetEngine()
	if engine == nil {
		LoggerGeneral.Error("KOKORO-TTS", "引擎未就绪")
		os.Exit(1)
	}
	engine.LogAvailable()

	addr := fmt.Sprintf(":%d", *GeneralConfig.BasicPort)
	LoggerGeneral.Info("KOKORO-TTS", "监听端口: %s", addr)
	go startServer(addr)

	quit := setupSignalHandling()
	<-quit
	LoggerGeneral.Info("KOKORO-TTS", "接收到中断信号，正在关闭...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if httpServer != nil {
		if err := httpServer.Shutdown(ctx); err != nil {
			LoggerGeneral.Error("KOKORO-TTS", "关闭失败: %v", err)
		}
	}
	LoggerGeneral.Info("KOKORO-TTS", "已成功关闭")
}
