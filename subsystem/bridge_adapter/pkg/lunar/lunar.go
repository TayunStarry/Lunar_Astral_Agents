package lunar

// Lunar WebSocket 客户端与消息分发逻辑

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

// ConnectToLunarWebSocket 连接到 lunar_astral 的 WebSocket 服务器
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
		_, msg, err := conn.ReadMessage()
		if err != nil {
			logger.Error("从 lunar_ws_server 读取消息失败: %v", err)
			break
		}
		if config.AppConfig.QQAdapter.DisplayLogs {
			logger.Debug("收到 lunar_ws_server 消息: %s", msg)
		}
		messageHandler(msg)
	}
}

// HandleLunarMessage 处理从 lunar_astral 接收到的消息
func HandleLunarMessage(rawMessage []byte) {
	var lunarMsg types.LunarMessage
	if err := json.Unmarshal(rawMessage, &lunarMsg); err != nil {
		logger.Error("解析 lunar 消息失败: %v", err)
		return
	}

	switch lunarMsg.Type {
	case "context":
		handleLunarContextMessage(lunarMsg.Data)
	case "image":
		handleLunarImageMessage(lunarMsg.Data)
	default:
		logger.Warn("未知的 lunar 消息类型: %s", lunarMsg.Type)
	}
}

// handleLunarContextMessage 处理文本类型的响应消息。
// response/active/context/image 仅作为类型参考，所有消息均走AI路由判定。
func handleLunarContextMessage(data json.RawMessage) {
	var contextData types.LunarContextData
	if err := json.Unmarshal(data, &contextData); err != nil {
		logger.Error("解析 lunar 上下文消息失败: %v", err)
		return
	}

	// 拦截 music 类型消息，禁止推送给群聊
	if contextData.Type == "music" {
		logger.Info("music类型消息已拦截，跳过推送")
		return
	}

	availableGroups := config.AppConfig.QQAdapter.ListenGroupIds
	if len(availableGroups) == 0 {
		logger.Error("没有可用的群组 ID")
		return
	}

	// 构建路由上下文 — MsgType 仅作为参考信息传入
	routeCtx := routing.RouteContext{
		SourceGroupID:   config.LastGroupID,
		AvailableGroups: availableGroups,
		MsgType:         contextData.Type,
	}

	// 调用AI路由判定（基于消息内容 + 群聊摘要上下文）
	targetGroups, err := routing.AnalyzeMessageRoute(contextData.Content, routeCtx)
	if err != nil {
		logger.Warn("AI路由失败，使用默认路由策略 (类型=%s): %v", contextData.Type, err)
		targetGroups = getDefaultRouteGroups(contextData.Type)
	}

	if len(targetGroups) == 0 {
		logger.Error("没有可用的目标群组")
		return
	}

	logger.Info("路由判定: 消息类型=%s, 目标群组=%v", contextData.Type, targetGroups)
	sendSplitTextMessages(targetGroups, contextData.Content)
}

// handleLunarImageMessage 处理图片类型的响应消息。
// 图片消息同样走AI路由，image类型仅作为参考。
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

	// 图片消息也走AI路由，MsgType 仅作参考
	routeCtx := routing.RouteContext{
		SourceGroupID:   config.LastGroupID,
		AvailableGroups: availableGroups,
		MsgType:         "image",
	}

	// 使用图片类型信息作为路由分析内容
	routeContent := "[图片消息] 类型: " + imageData.Type
	targetGroups, err := routing.AnalyzeMessageRoute(routeContent, routeCtx)
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

	logger.Info("图片消息路由: 目标群组=%v", targetGroups)

	for _, groupID := range targetGroups {
		if err := napcat.SendGroupImageMessage(groupID, imageData.Images); err != nil {
			logger.Error("发送群图片消息失败 (群 %d): %v", groupID, err)
		}
	}
}

// getDefaultRouteGroups 默认路由策略：AI路由失败时的回退方案。
// response → 来源群, active → 全部群, 其他 → 来源群或首个监听群
func getDefaultRouteGroups(msgType string) []int64 {
	availableGroups := config.AppConfig.QQAdapter.ListenGroupIds

	switch msgType {
	case "active":
		// active 类型传统上是广播，作为回退行为保留
		return availableGroups
	default:
		// response/context 等类型默认回源群
		if config.LastGroupID != 0 {
			return []int64{config.LastGroupID}
		}
		groupID := config.GetRandomGroupID()
		if groupID == 0 {
			return nil
		}
		return []int64{groupID}
	}
}

// sendSplitTextMessages 将文本内容按句末标点拆分，然后在后台协程中逐条推送到指定群组。
func sendSplitTextMessages(groupIDs []int64, content string) {
	if len(groupIDs) == 0 {
		logger.Warn("没有可用的群组 ID，跳过消息发送")
		return
	}

	parts := message.SplitMessageByPunctuation(content)
	if len(parts) == 0 {
		return
	}

	if len(parts) == 1 {
		for _, groupID := range groupIDs {
			if err := napcat.SendGroupTextMessage(groupID, parts[0]); err != nil {
				logger.Error("发送群文本消息失败 (群 %d): %v", groupID, err)
			}
		}
		return
	}

	logger.Info("消息已拆分为 %d 条，将逐条发送 (间隔 500ms~2000ms)", len(parts))

	go func() {
		for i, part := range parts {
			for _, groupID := range groupIDs {
				if err := napcat.SendGroupTextMessage(groupID, part); err != nil {
					logger.Error("发送群文本消息失败 (群 %d, 第 %d/%d 条): %v", groupID, i+1, len(parts), err)
				}
			}
			if i < len(parts)-1 {
				delay := 500 + min(len(part)*10, 1500)
				time.Sleep(time.Duration(delay) * time.Millisecond)
			}
		}
		logger.Info("拆分消息推送完成，共 %d 条", len(parts))
	}()
}
