package lunar

import (
	"encoding/json"
	"time"

	"bridge_adapter/pkg/config"
	"bridge_adapter/pkg/logger"
	"bridge_adapter/pkg/message"
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

	sendSplitTextMessages([]int64{groupID}, contextData.Content)
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

	sendSplitTextMessages([]int64{config.LastGroupID}, contextData.Content)
}

func handleLunarActiveMessage(data json.RawMessage) {
	var contextData types.LunarContextData
	if err := json.Unmarshal(data, &contextData); err != nil {
		logger.Error("解析 lunar 主动消息失败: %v", err)
		return
	}

	sendSplitTextMessages(config.AppConfig.QQAdapter.ListenGroupIds, contextData.Content)
}

// sendSplitTextMessages 将文本内容按句末标点拆分，然后在后台协程中逐条推送到指定群组。
// 推送间隔 = 500ms + len(part) * 10ms，上限 2000ms，消息越长等待越久，确保顺序发送。
func sendSplitTextMessages(groupIDs []int64, content string) {
	if len(groupIDs) == 0 {
		logger.Warn("没有可用的群组 ID，跳过消息发送")
		return
	}

	parts := message.SplitMessageByPunctuation(content)
	if len(parts) == 0 {
		return
	}

	// 只有一条消息，直接同步发送
	if len(parts) == 1 {
		for _, groupID := range groupIDs {
			if err := napcat.SendGroupTextMessage(groupID, parts[0]); err != nil {
				logger.Error("发送群文本消息失败 (群 %d): %v", groupID, err)
			}
		}
		return
	}

	logger.Info("消息已拆分为 %d 条，将逐条发送 (间隔 500ms~2000ms，随消息长度递增)", len(parts))

	// 在后台协程中逐条发送，避免阻塞 WebSocket 读取
	go func() {
		for i, part := range parts {
			for _, groupID := range groupIDs {
				if err := napcat.SendGroupTextMessage(groupID, part); err != nil {
					logger.Error("发送群文本消息失败 (群 %d, 第 %d/%d 条): %v", groupID, i+1, len(parts), err)
				}
			}
			// 最后一条不需要等待
			if i < len(parts)-1 {
				delay := 500 + min(len(part)*10, 1500)
				time.Sleep(time.Duration(delay) * time.Millisecond)
			}
		}
		logger.Info("拆分消息推送完成，共 %d 条", len(parts))
	}()
}
