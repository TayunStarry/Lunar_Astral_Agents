package message

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"strings"

	"nap_cat_bridging/internal/config"
	"nap_cat_bridging/internal/models"
	"nap_cat_bridging/internal/utils"
	"nap_cat_bridging/pkg/cqcode"
	"nap_cat_bridging/pkg/websocket"
)

// Handler 消息处理器
type Handler struct {
	config       *config.Config
	wsClient     *websocket.Client
	cqProcessor  *cqcode.Processor
	groupInfos   []models.GroupInfo
	groupMembers map[int64]map[int64]string
	parser       *Parser
	processor    *Processor
}

// NewHandler 创建消息处理器
func NewHandler(config *config.Config, wsClient *websocket.Client) *Handler {
	cqProcessor := cqcode.NewProcessor(wsClient)
	handler := &Handler{
		config:       config,
		wsClient:     wsClient,
		cqProcessor:  cqProcessor,
		groupInfos:   make([]models.GroupInfo, 0),
		groupMembers: make(map[int64]map[int64]string),
	}
	handler.parser = NewParser(handler)
	handler.processor = NewProcessor(handler)
	return handler
}

// ParseGroupListResponse 解析群列表响应
func (h *Handler) ParseGroupListResponse(message []byte) error {
	return h.parser.ParseGroupListResponse(message)
}

// ParseGroupMemberListResponse 解析群成员列表响应
func (h *Handler) ParseGroupMemberListResponse(message []byte) error {
	return h.parser.ParseGroupMemberListResponse(message)
}

// HandleGroupMessage 处理群消息
func (h *Handler) HandleGroupMessage(message []byte) (int64, any, error) {
	// 首先解析为原始消息
	var rawMsg map[string]any
	if err := json.Unmarshal(message, &rawMsg); err != nil {
		return 0, "", fmt.Errorf("解析消息失败: %v", err)
	}

	// 检查是否是群消息
	if postType, ok := rawMsg["post_type"].(string); !ok || postType != "message" {
		return 0, "", fmt.Errorf("不是消息")
	}
	if msgType, ok := rawMsg["message_type"].(string); !ok || msgType != "group" {
		return 0, "", fmt.Errorf("不是群消息")
	}

	// 提取基本信息
	groupID := int64(utils.GetFloat64Value(rawMsg, "group_id"))
	selfID := int64(utils.GetFloat64Value(rawMsg, "self_id"))
	senderUserID := int64(0)

	if sender, ok := rawMsg["sender"].(map[string]interface{}); ok {
		senderUserID = int64(utils.GetFloat64Value(sender, "user_id"))
	}

	// 提取发送者昵称
	senderName := h.getSenderName(rawMsg)

	// 提取原始消息内容
	rawMessage := ""
	if rm, ok := rawMsg["raw_message"].(string); ok {
		rawMessage = rm
	}

	log.Printf("%s", strings.Repeat("-=", 28))
	log.Printf("接收到来自< QQ群 %d >的消息 | 发送者: %s", groupID, senderName)
	log.Printf("消息内容: %s", rawMessage)

	// 群过滤：若group_id不属于配置的目标群，则直接忽略该消息
	groupFound := false
	for _, gid := range h.config.ListenGroupIDs {
		if gid == groupID {
			groupFound = true
			break
		}
	}
	if !groupFound {
		log.Printf("群 ID %d 不在订阅列表中，忽略消息", groupID)
		return 0, nil, nil
	}

	// 自过滤：若sender.user_id等于< QQ 智能体 >自身的self_id，则忽略该消息
	if senderUserID == selfID {
		log.Printf("消息来自< QQ 智能体 >自身，忽略消息")
		return 0, nil, nil
	}

	// 检查是否包含触发关键词或随机触发
	if !h.containsTriggerKeyword(rawMessage) {
		// 15%概率随机触发
		randomChance := rand.Intn(100)
		if randomChance > 15 {
			log.Printf("消息不包含触发关键词，忽略消息")
			return 0, nil, nil
		}
	}

	// 处理消息内容
	messageContent := h.processor.ProcessMessageContent(rawMsg, groupID, senderName)

	return groupID, messageContent, nil
}

// getSenderName 获取发送者昵称
func (h *Handler) getSenderName(rawMsg map[string]any) string {
	if sender, ok := rawMsg["sender"].(map[string]interface{}); ok {
		// 优先使用群名片
		if card, ok := sender["card"].(string); ok && card != "" {
			return card
		} else if nickname, ok := sender["nickname"].(string); ok {
			return nickname
		}
	}
	return ""
}

// getImageURL 获取图片URL
func (h *Handler) getImageURL(data map[string]any) string {
	var imageURL string
	// 尝试从不同的字段获取图片URL
	if url, ok := data["url"].(string); ok {
		// 处理HTML实体
		imageURL = utils.ProcessImageURL(url)
	} else if _, ok := data["file"].(string); ok {
		// 如果只有文件路径，使用占位符
		imageURL = "http://localhost/placeholder.jpg"
	}
	return imageURL
}

// getUserName 获取用户昵称
func (h *Handler) getUserName(groupID int64, userID int64) string {
	// 尝试获取用户昵称
	nickname := ""
	if groupID > 0 && userID > 0 {
		if members, ok := h.groupMembers[groupID]; ok {
			if name, ok := members[userID]; ok {
				nickname = name
			}
		}
	}

	// 如果找不到昵称，使用用户ID
	if nickname == "" {
		nickname = fmt.Sprintf("%d", userID)
	}

	return nickname
}

// containsTriggerKeyword 检查消息是否包含触发关键词
func (h *Handler) containsTriggerKeyword(message string) bool {
	for _, keyword := range h.config.TriggerKeywords {
		if strings.Contains(message, keyword) {
			return true
		}
	}
	return false
}

// SendGroupMsg 发送群消息
func (h *Handler) SendGroupMsg(groupID int64, content string) error {
	// 创建消息项
	message := []models.MsgItem{
		{
			Type: "text",
			Data: map[string]string{
				"text": content,
			},
		},
	}

	// 创建请求参数
	params := models.SendGroupMsgParams{
		GroupID: groupID,
		Message: message,
	}

	// 发送消息
	_, err := h.wsClient.SendMessage("send_group_msg", params)
	if err != nil {
		return fmt.Errorf("发送群消息失败: %v", err)
	}

	log.Printf("已发送群消息 (群 ID: %d)", groupID)
	return nil
}

// GetGroupInfos 获取群信息列表
func (h *Handler) GetGroupInfos() []models.GroupInfo {
	return h.groupInfos
}

// processMessageItem 处理单个消息项
func (h *Handler) processMessageItem(item any, data map[string]any) []map[string]any {
	content := make([]map[string]any, 0)
	if msgMap, ok := item.(map[string]any); ok {
		msgType := ""
		if t, ok := msgMap["type"].(string); ok {
			msgType = t
		}

		switch msgType {
		case "text":
			if msgData, ok := msgMap["data"].(map[string]any); ok {
				content = append(content, h.processor.ProcessTextMessage(msgData)...)
			}
		case "image":
			if msgData, ok := msgMap["data"].(map[string]any); ok {
				content = append(content, h.processor.ProcessImageMessage(msgData)...)
			}
		case "at":
			if msgData, ok := msgMap["data"].(map[string]any); ok {
				content = append(content, h.processor.ProcessAtMessage(msgData, data)...)
			}
		case "reply":
			if msgData, ok := msgMap["data"].(map[string]any); ok {
				content = append(content, h.processReplyMessage(msgData)...)
			}
		}
	}
	return content
}

// getOriginalMessage 获取原始消息内容
func (h *Handler) getOriginalMessage(messageID int64) (map[string]any, error) {
	// 创建请求参数
	params := map[string]interface{}{
		"message_id": messageID,
	}

	// 发送消息
	echo, err := h.wsClient.SendMessage("get_msg", params)
	if err != nil {
		return nil, fmt.Errorf("发送get_msg请求失败: %v", err)
	}

	// 等待响应
	for {
		messageBytes, err := h.wsClient.ReadMessage()
		if err != nil {
			return nil, fmt.Errorf("读取消息失败: %v", err)
		}

		// 解析响应
		var response websocket.WSResponse
		if err := json.Unmarshal(messageBytes, &response); err != nil {
			continue
		}

		// 检查是否是get_msg的响应
		if strings.Contains(response.Echo, echo) {
			if response.Status == "ok" && response.Data != nil {
				// 解析数据
				if data, ok := response.Data.(map[string]any); ok {
					return h.parser.ParseMessageResponse(data)
				}
			}
			return nil, fmt.Errorf("获取原始消息失败: %s", response.Message)
		}
	}
}

// processReplyMessage 处理引用消息
func (h *Handler) processReplyMessage(msgData map[string]any) []map[string]any {
	content := make([]map[string]any, 0)
	var messageID int64
	if id, ok := msgData["id"].(string); ok {
		fmt.Sscanf(id, "%d", &messageID)
	} else if id, ok := msgData["id"].(float64); ok {
		messageID = int64(id)
	}

	if messageID > 0 {
		// 获取原始消息内容
		originalMsg, err := h.getOriginalMessage(messageID)
		if err != nil {
			log.Printf("获取原始消息失败 (消息 ID: %d): %v", messageID, err)
			// 如果获取失败，添加一个占位符
			content = append(content, map[string]any{
				"type": "text",
				"text": "[回复消息]",
			})
		} else {
			// 解析并添加原始消息内容
			if originalContent, ok := originalMsg["content"].([]map[string]any); ok {
				// 添加回复前缀
				senderName := ""
				if name, ok := originalMsg["sender"].(string); ok {
					senderName = name
				}

				if senderName != "" {
					content = append(content, map[string]any{
						"type": "text",
						"text": fmt.Sprintf("[回复 %s]: ", senderName),
					})
				} else {
					content = append(content, map[string]any{
						"type": "text",
						"text": "[回复]: ",
					})
				}

				// 添加原始消息内容
				content = append(content, originalContent...)
			}
		}
	}
	return content
}

// ValidateListenGroups 校验需要监听的群
func (h *Handler) ValidateListenGroups() []int64 {
	validGroupIDs := make([]int64, 0)

	// 如果没有配置需要监听的群，返回空切片
	if len(h.config.ListenGroupIDs) == 0 {
		log.Println("没有配置需要监听的群")
		return validGroupIDs
	}

	// 遍历需要监听的群ID
	for _, groupID := range h.config.ListenGroupIDs {
		// 检查群是否在群列表中
		found := false
		for _, group := range h.groupInfos {
			if group.GroupID == groupID {
				found = true
				break
			}
		}

		if found {
			validGroupIDs = append(validGroupIDs, groupID)
			log.Printf("群 ID %d 校验通过", groupID)
		} else {
			log.Printf("群 ID %d 不在群列表中，跳过", groupID)
		}
	}

	return validGroupIDs
}
