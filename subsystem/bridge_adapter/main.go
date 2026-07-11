package main

import (
	"time"

	"bridge_adapter/pkg/config"
	"bridge_adapter/pkg/logger"
	"bridge_adapter/pkg/lunar"
	"bridge_adapter/pkg/message"
	"bridge_adapter/pkg/napcat"
)

func main() {
	logger.Info("Bridge Adapter 启动中...")

	// 加载配置
	config.LoadConfig()

	// 获取群成员列表
	napcat.FetchGroupMembers()

	// 启动双 WebSocket 连接
	go func() {
		for {
			lunar.ConnectToLunarWebSocket(lunar.HandleLunarMessage)
			logger.Warn("Lunar WebSocket 连接断开，5秒后重连...")
			time.Sleep(5 * time.Second)
		}
	}()

	go func() {
		for {
			napcat.ConnectToNapcatWebSocket(message.HandleNapcatMessage)
			logger.Warn("Napcat WebSocket 连接断开，5秒后重连...")
			time.Sleep(5 * time.Second)
		}
	}()

	// 阻塞主协程
	select {}
}
