package lunar

import (
	"encoding/json"
	"time"

	"bridge_adapter/pkg/config"
	"bridge_adapter/pkg/logger"
	"bridge_adapter/pkg/message"
	"bridge_adapter/pkg/napcat"
	"bridge_adapter/pkg/routing"
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
	case "batch":
		handleLunarBatchMessage(lunarMsg.Data)
	case "context":
		handleLunarContextMessage(lunarMsg.Data)
	case "image":
		handleLunarImageMessage(lunarMsg.Data)
	default:
		logger.Warn("未知的 lunar 消息类型: %s", lunarMsg.Type)
	}
}

// handleLunarBatchMessage 处理来自 lunar_astral 的批量消息推送（单次最多20条）。
// 每条消息独立调用AI路由判定目标群组，然后逐条分发。
func handleLunarBatchMessage(data json.RawMessage) {
	var batchPush types.LunarBatchPush
	if err := json.Unmarshal(data, &batchPush); err != nil {
		// 可能 data 就是 messages 数组本身
		var messages []types.LunarBatchItem
		if err2 := json.Unmarshal(data, &messages); err2 != nil {
			logger.Error("解析 lunar 批量消息失败: %v", err)
			return
		}
		batchPush.Messages = messages
	}

	if len(batchPush.Messages) == 0 {
		logger.Warn("收到空的批量消息推送")
		return
	}

	logger.Info("收到批量消息推送，共 %d 条消息", len(batchPush.Messages))

	availableGroups := config.AppConfig.QQAdapter.ListenGroupIds
	if len(availableGroups) == 0 {
		logger.Error("没有可用的群组 ID")
		return
	}

	// 收集所有文本内容用于AI路由分析
	messages := make([]string, 0, len(batchPush.Messages))
	for _, item := range batchPush.Messages {
		messages = append(messages, item.Content)
	}

	// 批量调用AI路由判定
	routingResults, routingErrs := routing.AnalyzeBatchMessages(messages, availableGroups)

	for i, item := range batchPush.Messages {
		var targetGroups []int64
		if routingErrs[i] != nil {
			// AI路由失败，回退到默认路由
			logger.Warn("第 %d/%d 条消息AI路由失败，使用默认路由: %v", i+1, len(batchPush.Messages), routingErrs[i])
			targetGroups = getDefaultRouteGroups(item.MsgType)
		} else {
			targetGroups = routingResults[i]
		}

		if len(targetGroups) == 0 {
			logger.Warn("第 %d/%d 条消息没有目标群组，跳过", i+1, len(batchPush.Messages))
			continue
		}

		logger.Info("批量消息 [%d/%d] 分发到群组 %v (类型=%s)", i+1, len(batchPush.Messages), targetGroups, item.MsgType)

		switch item.MsgType {
		case "image":
			if len(item.Images) > 0 {
				for _, groupID := range targetGroups {
					if err := napcat.SendGroupImageMessage(groupID, item.Images); err != nil {
						logger.Error("发送群图片消息失败 (群 %d): %v", groupID, err)
					}
				}
			}
		default:
			sendSplitTextMessages(targetGroups, item.Content)
		}
	}

	logger.Info("批量消息推送处理完成，共 %d 条", len(batchPush.Messages))
}

func handleLunarContextMessage(data json.RawMessage) {
	var contextData types.LunarContextData
	if err := json.Unmarshal(data, &contextData); err != nil {
		logger.Error("解析 lunar 上下文消息失败: %v", err)
		return
	}

	availableGroups := config.AppConfig.QQAdapter.ListenGroupIds
	if len(availableGroups) == 0 {
		logger.Error("没有可用的群组 ID")
		return
	}

	// 尝试AI路由判定
	targetGroups, err := routing.AnalyzeMessageRoute(contextData.Content, availableGroups)
	if err != nil {
		logger.Warn("AI路由失败，使用默认路由策略: %v", err)
		// 回退到原有的默认路由逻辑
		targetGroups = getDefaultRouteGroups(contextData.Type)
	}

	if len(targetGroups) == 0 {
		logger.Error("没有可用的目标群组")
		return
	}

	logger.Info("AI路由判定: 消息类型=%s, 目标群组=%v", contextData.Type, targetGroups)
	sendSplitTextMessages(targetGroups, contextData.Content)
}

func handleLunarImageMessage(data json.RawMessage) {
	var imageData types.LunarImageData
	if err := json.Unmarshal(data, &imageData); err != nil {
		logger.Error("解析 lunar 图片消息失败: %v", err)
		return
	}

	availableGroups := config.AppConfig.QQAdapter.ListenGroupIds
	if len(availableGroups) == 0 {
		logger.Error("没有可用的群组 ID")
		return
	}

	// 图片消息：尝试AI路由（基于图片类型描述），失败则使用LastGroupID
	targetGroups, err := routing.AnalyzeMessageRoute("[图片消息] 类型: "+imageData.Type, availableGroups)
	if err != nil {
		logger.Warn("图片消息AI路由失败，使用默认路由: %v", err)
		if config.LastGroupID == 0 {
			config.LastGroupID = config.GetRandomGroupID()
		}
		if config.LastGroupID == 0 {
			logger.Error("没有可用的群组 ID")
			return
		}
		targetGroups = []int64{config.LastGroupID}
	}

	logger.Info("图片消息AI路由: 目标群组=%v", targetGroups)

	for _, groupID := range targetGroups {
		if err := napcat.SendGroupImageMessage(groupID, imageData.Images); err != nil {
			logger.Error("发送群图片消息失败 (群 %d): %v", groupID, err)
		}
	}
}

// getDefaultRouteGroups 默认路由策略：根据消息类型返回目标群组。
// 当AI路由不可用或失败时作为回退方案。
func getDefaultRouteGroups(msgType string) []int64 {
	availableGroups := config.AppConfig.QQAdapter.ListenGroupIds

	switch msgType {
	case "response":
		if config.LastGroupID == 0 {
			logger.Warn("没有记录的群聊 ID，使用随机群聊")
			config.LastGroupID = config.GetRandomGroupID()
		}
		if config.LastGroupID == 0 {
			logger.Error("没有可用的群组 ID")
			return nil
		}
		return []int64{config.LastGroupID}
	case "active":
		return availableGroups
	default:
		groupID := config.GetRandomGroupID()
		if groupID == 0 {
			return nil
		}
		return []int64{groupID}
	}
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
