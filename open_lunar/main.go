package main

import "open-lunar/server"

func main() {
	// 初始化服务器
	server.InitializeServer()
	// 设置信号处理，用于在接收到终止信号时优雅关闭服务器
	quit := server.SetupSignalHandling()
	// 等待关闭信号并优雅关闭服务器
	server.WaitForShutdown(quit, server.StartServer())
}
