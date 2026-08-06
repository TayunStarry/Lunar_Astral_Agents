package main

import (
	"lunar_astral/server"
	"net/http"
	"os"
)

func main() {
	// 初始化服务器
	server.InitializeServer()
	// 设置信号处理，用于在接收到终止信号时优雅关闭服务器
	quit := server.SetupSignalHandling()
	// 创建一个新的 HTTP 服务器实例
	project := &http.Server{}
	// 启动服务器监听
	go server.StartServerListener(project)
	// 等待关闭信号（系统信号或 WebView 关闭）并优雅关闭服务器
	server.WaitForShutdown(quit, project)
	// 确保进程彻底退出（Windows 上 WebView2 COM 可能残留引用）
	os.Exit(0)
}
