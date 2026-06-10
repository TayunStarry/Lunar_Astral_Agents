package main

import (
	"browser"
	"config"
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

const targetURL = "https://studio-next.mosi.cn"

// reloadPageParameters 重新加载页面参数
func reloadPageParameters() {
	*config.WebViewTitle = "MOSS Studio"
	*config.WebViewWidth = 1210
	*config.WebViewHeight = 830
}

func main() {
	// 解析命令行参数（config 模块在 init 中 flag.Parse）
	fmt.Printf("[Studio WebView] 启动中，目标地址: %s\n", targetURL)
	// 重新加载页面参数
	reloadPageParameters()

	// 启动 WebView 浏览器
	browser.OpenBrowser(targetURL)

	// 等待 WebView 关闭信号或系统中断
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-browser.WebViewClosed():
		fmt.Println("[Studio WebView] 窗口已关闭，终止进程")
	case sig := <-sigCh:
		fmt.Printf("[Studio WebView] 收到信号: %v，正在退出...\n", sig)
		browser.CloseWebView()
		<-browser.WebViewClosed()
	}

	// WebView 关闭后强制终止进程，确保没有残留 goroutine
	os.Exit(0)
}
