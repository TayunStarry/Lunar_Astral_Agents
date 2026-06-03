package main

import (
	"context"
	"fmt"
	"logger"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"browser"
	"config"
	"gguf_metadata_viewer/server"
)

func main() {
	// 根据config配置初始化日志系统
	logger.SetDevMode(*config.Developer)
	logger.Info("GGUFViewer", "GGUF元数据查看器启动中...")
	logger.Info("GGUFViewer", "开发者模式: %v", *config.Developer)

	// 使用独立端口（在config.BasicPort基础上偏移）
	port := *config.BasicPort + 100
	if port > 65535 {
		port = 36900
	}

	logger.Info("GGUFViewer", "HTTP服务端口: %d", port)

	// 创建服务器实例
	srv := server.New(port)

	// 在goroutine中启动HTTP服务
	go func() {
		logger.Info("GGUFViewer", "HTTP服务正在启动 http://127.0.0.1:%d", port)
		if err := srv.Start(); err != nil && err != http.ErrServerClosed {
			logger.Error("GGUFViewer", "HTTP服务启动失败: %v", err)
			os.Exit(1)
		}
	}()

	// 等待HTTP服务就绪
	time.Sleep(500 * time.Millisecond)

	// 使用嵌入式浏览器打开前端界面
	url := fmt.Sprintf("http://127.0.0.1:%d", port)
	logger.Info("GGUFViewer", "正在打开嵌入式浏览器: %s", url)
	browser.OpenBrowser(url)

	// 等待信号处理
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// 监听WebView关闭或系统信号
	select {
	case <-browser.WebViewClosed():
		logger.Info("GGUFViewer", "WebView窗口已关闭")
	case sig := <-quit:
		logger.Info("GGUFViewer", "收到系统信号: %v", sig)
	}

	// 优雅关闭HTTP服务
	logger.Info("GGUFViewer", "正在停止HTTP服务...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("GGUFViewer", "HTTP服务关闭失败: %v", err)
	}

	logger.Info("GGUFViewer", "GGUF元数据查看器已安全退出")
}