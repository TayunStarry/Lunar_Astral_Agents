package main

import (
	"browser"
	"config"
	"context"
	"flag"
	"log"
	"os"
	"time"
)

func main() {
	addr := ":36365"
	modelDir := *config.LocalDir + "/models"
	refAudio := *config.LocalDir + "/audios/lunar-template.wav"
	flag.Parse()

	log.Println("[SimpleTTS] 正在启动 Simple TTS 服务...")

	log.Printf("[SimpleTTS] 模型目录: %s", modelDir)
	log.Printf("[SimpleTTS] 参考音频: %s", refAudio)

	if _, err := os.Stat(modelDir); os.IsNotExist(err) {
		log.Printf("[SimpleTTS] 警告: 模型目录不存在: %s", modelDir)
	}

	initTTSEngine(modelDir, refAudio)

	log.Printf("[SimpleTTS] 监听端口: %s", addr)
	go startServer(addr)

	if waitForServerReady(addr, 10) {
		url := "http://localhost" + addr
		browser.OpenBrowser(url)
	}

	quit := setupSignalHandling()

	select {
	case <-quit:
		log.Println("[SimpleTTS] 接收到中断信号，正在关闭...")
	case <-browser.WebViewClosed():
		log.Println("[SimpleTTS] 检测到 WebView 关闭，正在关闭...")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if httpServer != nil {
		if err := httpServer.Shutdown(ctx); err != nil {
			log.Printf("[SimpleTTS] 关闭失败: %v\n", err)
		}
	}

	browser.CloseWebView()
	log.Println("[SimpleTTS] 已成功关闭")
}
