package main

import (
	"os"
	"os/signal"
	"syscall"

	"bridge_adapter/pkg/config"
	"bridge_adapter/pkg/logger"
	"bridge_adapter/pkg/lunar"
	"bridge_adapter/pkg/message"
	"bridge_adapter/pkg/napcat"
)

func main() {
	logger.Info("========== Bridge Adapter 启动 ==========")

	config.LoadConfig()

	go napcat.FetchGroupMembers()

	go napcat.ConnectToNapcatWebSocket(message.HandleNapcatMessage)
	go lunar.ConnectToLunarWebSocket(lunar.HandleLunarMessage)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	logger.Info("程序退出")
}
