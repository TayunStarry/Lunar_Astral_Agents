package main

import (
	"browser"
	"config"
	"context"
	"flag"
	"logger"
	"os"
	"qwen3_tts_lunar/module"
	"time"
)

// reloadPageParameters 重新加载页面参数
func reloadPageParameters() {
	*config.WebViewTitle = "星月智能 -> 轻量级-神经网络-本地部署方案"
	*config.WebViewWidth = 1150
	*config.WebViewHeight = 960
}

// main 主函数
func main() {
	addr := ":36365"
	modelDir := *config.LocalDir + "/models"
	refAudio := *config.LocalDir + "/audios/lunar-template.wav"
	flag.Parse()

	logger.SetDevMode(*config.Developer)

	if _, err := os.Stat(modelDir); os.IsNotExist(err) {
		logger.Error("QWEN-TTS", "模型目录不存在: %s", modelDir)
	}

	module.InitTTSEngine(modelDir, refAudio)

	logger.Info("QWEN-TTS", "监听端口: %s", addr)
	go startServer(addr)

	if waitForServerReady(addr, 10) {
		url := "http://localhost" + addr
		reloadPageParameters()
		browser.OpenBrowser(url)
	}

	quit := setupSignalHandling()

	select {
	case <-quit:
		logger.Info("QWEN-TTS", "接收到中断信号，正在关闭...")
	case <-browser.WebViewClosed():
		logger.Info("QWEN-TTS", "检测到 WebView 关闭，正在关闭...")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if httpServer != nil {
		if err := httpServer.Shutdown(ctx); err != nil {
			logger.Error("QWEN-TTS", "关闭失败: %v", err)
		}
	}

	shutdownServer()

	browser.CloseWebView()
	logger.Info("QWEN-TTS", "已成功关闭")
}
