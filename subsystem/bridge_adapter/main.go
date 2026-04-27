package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
)

// 主函数
func main() {
	// 加载配置
	LoadConfig()

	// 启动群成员列表获取
	go FetchGroupMembers()

	// 启动 WebSocket 客户端
	go ConnectToNapcatWebSocket()
	go ConnectToLunarWebSocket()

	// 等待中断信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("程序退出")
}
