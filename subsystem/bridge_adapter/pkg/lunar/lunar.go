package lunar

import (
	"encoding/json"

	"bridge_adapter/pkg/config"
	"bridge_adapter/pkg/logger"
	"bridge_adapter/pkg/napcat"
	"bridge_adapter/pkg/types"

	"github.com/gorilla/websocket"
)

func ConnectToLunarWebSocket(messageHandler func([]byte)) {
	url := config.AppConfig.QQAdapter.LunarWsServer

	logger.Info("正在连接到 lunar_ws_server: %s", url)

	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		logger.Error("连接 lunar_ws_server 失败: %v", err)
		return
	}
	defer conn.Close()

	logger.Info("成功连接到 lunar_ws_server")

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			logger.Error("从 lunar_ws_server 读取消息失败: %v", err)
			break
		}
		if config.AppConfig.QQAdapter.DisplayLogs {
			logger.Debug("收到 lunar_ws_server 消息: %s", message)
		}
		messageHandler(message)
	}
}

func HandleLunarMessage(message []byte) {
	var lunarMsg types.LunarMessage
	if err := json.Unmarshal(message, &lunarMsg); err != nil {
		logger.Error("解析 lunar 消息失败: %v", err)
		return
	}

	switch lunarMsg.Type {
	case "context":
		handleLunarContextMessage(lunarMsg.Data)
	case "image":
		handleLunarImageMessage(lunarMsg.Data)
	case "response":
		handleLunarResponseMessage(lunarMsg.Data)
	case "active":
		handleLunarActiveMessage(lunarMsg.Data)
	default:
		logger.Warn("未知的 lunar 消息类型: %s", lunarMsg.Type)
	}
}

func handleLunarContextMessage(data json.RawMessage) {
	var contextData types.LunarContextData
	if err := json.Unmarshal(data, &contextData); err != nil {
		logger.Error("解析 lunar 上下文消息失败: %v", err)
		return
	}

	groupID := config.GetRandomGroupID()
	if groupID == 0 {
		logger.Error("没有可用的群组 ID")
		return
	}

	err := napcat.SendGroupTextMessage(groupID, contextData.Content)
	if err != nil {
		logger.Error("发送群文本消息失败: %v", err)
	}
}

func handleLunarImageMessage(data json.RawMessage) {
	var imageData types.LunarImageData
	if err := json.Unmarshal(data, &imageData); err != nil {
		logger.Error("解析 lunar 图片消息失败: %v", err)
		return
	}

	groupID := config.GetRandomGroupID()
	if groupID == 0 {
		logger.Error("没有可用的群组 ID")
		return
	}

	err := napcat.SendGroupImageMessage(groupID, imageData.Images)
	if err != nil {
		logger.Error("发送群图片消息失败: %v", err)
	}
}

func handleLunarResponseMessage(data json.RawMessage) {
	var contextData types.LunarContextData
	if err := json.Unmarshal(data, &contextData); err != nil {
		logger.Error("解析 lunar 响应消息失败: %v", err)
		return
	}

	if config.LastGroupID == 0 {
		logger.Warn("没有记录的群聊 ID，使用随机群聊")
		config.LastGroupID = config.GetRandomGroupID()
	}

	err := napcat.SendGroupTextMessage(config.LastGroupID, contextData.Content)
	if err != nil {
		logger.Error("发送群文本消息失败: %v", err)
	}
}

func handleLunarActiveMessage(data json.RawMessage) {
	var contextData types.LunarContextData
	if err := json.Unmarshal(data, &contextData); err != nil {
		logger.Error("解析 lunar 主动消息失败: %v", err)
		return
	}

	for _, groupID := range config.AppConfig.QQAdapter.ListenGroupIds {
		err := napcat.SendGroupTextMessage(groupID, contextData.Content)
		if err != nil {
			logger.Error("发送群文本消息失败 (群 %d): %v", groupID, err)
		} else {
			logger.Info("成功发送主动消息到群 %d", groupID)
		}
	}
}
