package main

import (
	"log"
	"net/http"

	"github.com/gorilla/websocket"
)

// ConnectToNapcatWebSocket 连接到 Napcat WebSocket 服务器
func ConnectToNapcatWebSocket() {
	url := AppConfig.QQAdapter.NapcatWsServer
	token := AppConfig.QQAdapter.NapcatWsToken

	log.Printf("正在连接到 napcat_ws_server: %s", url)

	headers := http.Header{}
	if token != "" {
		headers.Set("Authorization", "Bearer "+token)
	}

	conn, _, err := websocket.DefaultDialer.Dial(url, headers)
	if err != nil {
		log.Printf("连接 napcat_ws_server 失败: %v", err)
		return
	}
	defer conn.Close()

	log.Printf("成功连接到 napcat_ws_server")

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			log.Printf("从 napcat_ws_server 读取消息失败: %v", err)
			break
		}
		if AppConfig.QQAdapter.DisplayLogs {
			log.Printf("收到 napcat_ws_server 消息: %s", message)
		}
		HandleNapcatMessage(message)
	}
}

// ConnectToLunarWebSocket 连接到 Lunar WebSocket 服务器
func ConnectToLunarWebSocket() {
	url := AppConfig.QQAdapter.LunarWsServer

	log.Printf("正在连接到 lunar_ws_server: %s", url)

	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		log.Printf("连接 lunar_ws_server 失败: %v", err)
		return
	}
	defer conn.Close()

	log.Printf("成功连接到 lunar_ws_server")

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			log.Printf("从 lunar_ws_server 读取消息失败: %v", err)
			break
		}
		if AppConfig.QQAdapter.DisplayLogs {
			log.Printf("收到 lunar_ws_server 消息: %s", message)
		}
		HandleLunarMessage(message)
	}
}
