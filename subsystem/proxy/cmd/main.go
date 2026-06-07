package main

import (
	"context"
	"os/signal"
	"proxy"
	"syscall"
	"time"

	"browser"
	"logger"
)

func main() {
	server := proxy.Run()
	if server == nil {
		return
	}

	// 监听系统中断信号
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	select {
	case <-ctx.Done():
		logger.Info("ProxySvr", "接收到中断信号，正在关闭...")
	case <-browser.WebViewClosed():
		logger.Info("ProxySvr", "检测到 WebView 关闭，正在关闭...")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("ProxySvr", "关闭失败: %v", err)
	}

	browser.CloseWebView()
	logger.Info("ProxySvr", "已成功关闭")
}
