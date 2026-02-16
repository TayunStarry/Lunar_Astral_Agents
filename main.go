package main

// 导入必要的包
import core "Lunar-Astral-Agents/server"

// main 函数是程序的入口点，负责初始化服务器并启动服务
func main() {
	// 初始化服务器
	core.InitializeServer()
	// 设置信号处理，用于在接收到终止信号时优雅关闭服务器
	quit := core.SetupSignalHandling()
	// 创建服务器实例
	server := core.StartServer()
	// 等待关闭信号并优雅关闭服务器
	core.WaitForShutdown(quit, server)
}
