package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"
)

var httpClient = &http.Client{Timeout: 10 * time.Second}

// FetchGroupMembers 获取所有群的成员列表
func FetchGroupMembers() {
	token := AppConfig.QQAdapter.NapcatWsToken
	baseURL := GetNapcatHTTPBaseURL()

	for _, groupID := range AppConfig.QQAdapter.ListenGroupIds {
		log.Printf("正在获取群 %d 的成员列表", groupID)
		err := FetchGroupMemberList(baseURL, token, groupID)
		if err != nil {
			log.Printf("获取群 %d 成员列表失败: %v", groupID, err)
		}
	}
}

// FetchGroupMemberList 获取指定群的成员列表
func FetchGroupMemberList(baseURL, token string, groupID int64) error {
	url := baseURL + "/get_group_member_list"

	body, err := json.Marshal(map[string]interface{}{
		"group_id": groupID,
	})
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	var response NapcatWSResponse
	if err := json.Unmarshal(respBody, &response); err != nil {
		return err
	}

	if response.Status == "ok" && response.Data != nil {
		var data interface{}
		if err := json.Unmarshal(response.Data, &data); err != nil {
			return err
		}

		if memberList, ok := data.([]interface{}); ok {
			if _, ok := GroupMembers[groupID]; !ok {
				GroupMembers[groupID] = make(map[int64]string)
			}

			for _, item := range memberList {
				if member, ok := item.(map[string]interface{}); ok {
					userID := int64(0)
					if uid, ok := member["user_id"].(float64); ok {
						userID = int64(uid)
					}
					nickname := ""
					if card, ok := member["card"].(string); ok && card != "" {
						nickname = card
					} else if nick, ok := member["nickname"].(string); ok {
						nickname = nick
					}

					if userID > 0 && nickname != "" {
						GroupMembers[groupID][userID] = nickname
					}
				}
			}
			log.Printf("群 %d 成员列表获取成功，共 %d 个成员", groupID, len(memberList))
		}
	}

	return nil
}

// GetUserName 获取用户昵称
func GetUserName(groupID int64, userID int64) string {
	nickname := ""
	if groupID > 0 && userID > 0 {
		if members, ok := GroupMembers[groupID]; ok {
			if name, ok := members[userID]; ok {
				nickname = name
			}
		}
	}
	if nickname == "" {
		nickname = fmt.Sprintf("%d", userID)
	}
	return nickname
}

// HandleNapcatMessage 处理 Napcat 消息
func HandleNapcatMessage(message []byte) {
	var napcatMsg NapcatMessage
	if err := json.Unmarshal(message, &napcatMsg); err != nil {
		log.Printf("解析 napcat 消息失败: %v", err)
		return
	}

	if napcatMsg.UserID == napcatMsg.SelfID {
		return
	}

	if !IsInListenGroup(napcatMsg.GroupID) {
		return
	}

	if napcatMsg.MessageType != "group" {
		return
	}

	content, hasImages, err := ParseMessageSegments(napcatMsg.Message, napcatMsg.GroupID)
	if err != nil {
		log.Printf("解析消息段失败: %v", err)
		return
	}

	// 将消息添加到缓存
	cachedMsg := CachedMessage{
		GroupID:   napcatMsg.GroupID,
		UserID:    napcatMsg.UserID,
		Content:   content,
		HasImages: hasImages,
	}
	AddToMessageCache(cachedMsg)

	log.Printf("消息已添加到缓存，当前缓存 %d 条消息", len(MessageCache))

	// 检查是否包含触发关键词
	var contentStr string
	if str, ok := content.(string); ok {
		contentStr = str
	} else {
		// 如果是数组，需要提取文本内容检查
		if arr, ok := content.([]map[string]interface{}); ok {
			for _, item := range arr {
				if item["type"] == "text" {
					if data, ok := item["text"].(string); ok {
						contentStr += data
					}
				}
			}
		}
	}

	if !ContainsTriggerKeyword(contentStr) {
		log.Printf("消息不包含触发关键词，仅缓存: %s", contentStr)
		return
	}

	// 匹配关键词，发送所有缓存消息
	log.Printf("检测到触发关键词，准备发送 %d 条缓存消息", len(MessageCache))

	// 记录最新发送消息的群聊 ID
	LastGroupID = napcatMsg.GroupID

	// 构建 OpenAI 消息数组
	openAIMessages := BuildMessagesFromCache()

	err = SendToLunarCore(openAIMessages)
	if err != nil {
		log.Printf("发送消息到 lunar_core 失败: %v", err)
		return
	}

	// 发送成功后清除缓存
	ClearMessageCache()
	log.Printf("缓存消息已清除")
}

// BuildMessagesFromCache 从缓存构建 OpenAI 消息
func BuildMessagesFromCache() []OpenAIMessage {
	var openAIMessages []OpenAIMessage

	for _, msg := range MessageCache {
		senderName := GetUserName(msg.GroupID, msg.UserID)

		if msg.HasImages {
			// 如果有图片，构建数组格式
			contentArray, _ := msg.Content.([]map[string]interface{})
			withSender := append([]map[string]interface{}{
				{
					"type": "text",
					"text": senderName + " : ",
				},
			}, contentArray...)
			openAIMessages = append(openAIMessages, OpenAIMessage{
				Role:    "user",
				Content: withSender,
			})
		} else {
			// 普通文本消息
			contentStr, _ := msg.Content.(string)
			finalContent := senderName + " : " + contentStr
			openAIMessages = append(openAIMessages, OpenAIMessage{
				Role:    "user",
				Content: finalContent,
			})
		}
	}

	return openAIMessages
}

// HandleLunarMessage 处理 Lunar 消息
func HandleLunarMessage(message []byte) {
	var lunarMsg LunarMessage
	if err := json.Unmarshal(message, &lunarMsg); err != nil {
		log.Printf("解析 lunar 消息失败: %v", err)
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
		log.Printf("未知的 lunar 消息类型: %s", lunarMsg.Type)
	}
}

// handleLunarContextMessage 处理 Lunar 上下文消息
func handleLunarContextMessage(data json.RawMessage) {
	var contextData LunarContextData
	if err := json.Unmarshal(data, &contextData); err != nil {
		log.Printf("解析 lunar 上下文消息失败: %v", err)
		return
	}

	groupID := GetRandomGroupID()
	if groupID == 0 {
		log.Println("没有可用的群组 ID")
		return
	}

	err := SendGroupTextMessage(groupID, contextData.Content)
	if err != nil {
		log.Printf("发送群文本消息失败: %v", err)
	}
}

// handleLunarImageMessage 处理 Lunar 图片消息
func handleLunarImageMessage(data json.RawMessage) {
	var imageData LunarImageData
	if err := json.Unmarshal(data, &imageData); err != nil {
		log.Printf("解析 lunar 图片消息失败: %v", err)
		return
	}

	groupID := GetRandomGroupID()
	if groupID == 0 {
		log.Println("没有可用的群组 ID")
		return
	}

	err := SendGroupImageMessage(groupID, imageData.Images)
	if err != nil {
		log.Printf("发送群图片消息失败: %v", err)
	}
}

// handleLunarResponseMessage 处理 Lunar 响应消息（只发送到最新的群聊）
func handleLunarResponseMessage(data json.RawMessage) {
	var contextData LunarContextData
	if err := json.Unmarshal(data, &contextData); err != nil {
		log.Printf("解析 lunar 响应消息失败: %v", err)
		return
	}

	if LastGroupID == 0 {
		log.Println("没有记录的群聊 ID，使用随机群聊")
		LastGroupID = GetRandomGroupID()
	}

	err := SendGroupTextMessage(LastGroupID, contextData.Content)
	if err != nil {
		log.Printf("发送群文本消息失败: %v", err)
	}
}

// handleLunarActiveMessage 处理 Lunar 主动消息（发送到所有监听的群聊）
func handleLunarActiveMessage(data json.RawMessage) {
	var contextData LunarContextData
	if err := json.Unmarshal(data, &contextData); err != nil {
		log.Printf("解析 lunar 主动消息失败: %v", err)
		return
	}

	for _, groupID := range AppConfig.QQAdapter.ListenGroupIds {
		err := SendGroupTextMessage(groupID, contextData.Content)
		if err != nil {
			log.Printf("发送群文本消息失败 (群 %d): %v", groupID, err)
		} else {
			log.Printf("成功发送主动消息到群 %d", groupID)
		}
	}
}

// ParseMessageSegments 解析消息段，返回内容、是否有图片、错误
func ParseMessageSegments(segments []MessageSegment, groupID int64) (interface{}, bool, error) {
	var contentArray []map[string]interface{}
	var contentStr string
	var hasImages bool

	for _, segment := range segments {
		switch segment.Type {
		case "text":
			var textData TextData
			if err := json.Unmarshal(segment.Data, &textData); err != nil {
				return nil, false, err
			}
			if len(contentArray) > 0 {
				contentArray = append(contentArray, map[string]interface{}{
					"type": "text",
					"text": textData.Text,
				})
			} else {
				contentStr += textData.Text
			}
		case "reply":
			var replyData ReplyData
			if err := json.Unmarshal(segment.Data, &replyData); err != nil {
				return nil, false, err
			}
			replyContent, err := GetMessageContent(replyData.ID)
			if err != nil {
				log.Printf("获取回复消息内容失败: %v", err)
				replyText := "[回复] "
				if len(contentArray) > 0 {
					contentArray = append(contentArray, map[string]interface{}{
						"type": "text",
						"text": replyText,
					})
				} else {
					contentStr += replyText
				}
			} else {
				replyText := "[回复: " + replyContent + "] "
				if len(contentArray) > 0 {
					contentArray = append(contentArray, map[string]interface{}{
						"type": "text",
						"text": replyText,
					})
				} else {
					contentStr += replyText
				}
			}
		case "image":
			var imageData ImageData
			if err := json.Unmarshal(segment.Data, &imageData); err != nil {
				return nil, false, err
			}
			hasImages = true
			contentArray = append(contentArray, map[string]interface{}{
				"type": "image_url",
				"image_url": map[string]string{
					"url": imageData.URL,
				},
			})
		case "at":
			var atData AtData
			if err := json.Unmarshal(segment.Data, &atData); err != nil {
				return nil, false, err
			}
			atUserID, err := strconv.ParseInt(atData.QQ, 10, 64)
			var atText string
			if err != nil {
				atText = "@" + atData.QQ + " "
			} else {
				userName := GetUserName(groupID, atUserID)
				atText = "@" + userName + " "
			}
			if len(contentArray) > 0 {
				contentArray = append(contentArray, map[string]interface{}{
					"type": "text",
					"text": atText,
				})
			} else {
				contentStr += atText
			}
		case "forward":
			var forwardData ForwardData
			if err := json.Unmarshal(segment.Data, &forwardData); err != nil {
				return nil, false, err
			}
			forwardContent, forwardHasImages, err := GetForwardMessageContent(forwardData.ID, groupID)
			if err != nil {
				log.Printf("获取转发消息内容失败: %v", err)
				forwardText := "[转发消息] "
				if len(contentArray) > 0 {
					contentArray = append(contentArray, map[string]interface{}{
						"type": "text",
						"text": forwardText,
					})
				} else {
					contentStr += forwardText
				}
			} else {
				// 添加转发消息前缀
				forwardPrefix := "[转发消息]: "
				if len(contentArray) > 0 {
					contentArray = append(contentArray, map[string]interface{}{
						"type": "text",
						"text": forwardPrefix,
					})
				} else {
					contentStr += forwardPrefix
				}

				// 添加转发消息内容
				if forwardHasImages {
					hasImages = true
					// 将 contentStr 转换为数组
					if len(contentArray) == 0 && contentStr != "" {
						contentArray = append(contentArray, map[string]interface{}{
							"type": "text",
							"text": contentStr,
						})
						contentStr = ""
					}
					// 追加转发消息内容
					if fcArray, ok := forwardContent.([]map[string]interface{}); ok {
						contentArray = append(contentArray, fcArray...)
					}
				} else {
					if len(contentArray) > 0 {
						if fcStr, ok := forwardContent.(string); ok {
							contentArray = append(contentArray, map[string]interface{}{
								"type": "text",
								"text": fcStr,
							})
						}
					} else {
						if fcStr, ok := forwardContent.(string); ok {
							contentStr += fcStr
						}
					}
				}
			}
		case "file":
			var fileData FileData
			if err := json.Unmarshal(segment.Data, &fileData); err != nil {
				return nil, false, err
			}
			fileText := "[文件] " + fileData.File + " " + fileData.URL + " "
			if len(contentArray) > 0 {
				contentArray = append(contentArray, map[string]interface{}{
					"type": "text",
					"text": fileText,
				})
			} else {
				contentStr += fileText
			}
		default:
			log.Printf("未知的消息段类型: %s", segment.Type)
		}
	}

	if hasImages {
		return contentArray, true, nil
	}

	return contentStr, false, nil
}

// GetMessageContent 获取消息内容
func GetMessageContent(messageID string) (string, error) {
	token := AppConfig.QQAdapter.NapcatWsToken
	baseURL := GetNapcatHTTPBaseURL()
	url := baseURL + "/get_msg"

	body, err := json.Marshal(map[string]interface{}{
		"message_id": messageID,
	})
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var response NapcatWSResponse
	if err := json.Unmarshal(respBody, &response); err != nil {
		return "", err
	}

	if response.Status != "ok" {
		return "", fmt.Errorf("get_msg 返回错误: %s", response.Wording)
	}

	if response.Data == nil {
		return "", fmt.Errorf("get_msg 返回空数据")
	}

	var msgData map[string]interface{}
	if err := json.Unmarshal(response.Data, &msgData); err != nil {
		return "", err
	}

	if msg, ok := msgData["message"].([]interface{}); ok {
		var content string
		for _, seg := range msg {
			if segMap, ok := seg.(map[string]interface{}); ok {
				if segType, ok := segMap["type"].(string); ok && segType == "text" {
					if data, ok := segMap["data"].(map[string]interface{}); ok {
						if text, ok := data["text"].(string); ok {
							content += text
						}
					}
				}
			}
		}
		return content, nil
	}

	return "", fmt.Errorf("无法解析消息内容")
}

// GetForwardMessageContent 获取转发消息内容，返回内容、是否有图片、错误
func GetForwardMessageContent(forwardID string, groupID int64) (interface{}, bool, error) {
	token := AppConfig.QQAdapter.NapcatWsToken
	baseURL := GetNapcatHTTPBaseURL()
	url := baseURL + "/get_forward_msg"

	body, err := json.Marshal(map[string]interface{}{
		"message_id": forwardID,
	})
	if err != nil {
		return nil, false, err
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return nil, false, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, false, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, false, err
	}

	var response NapcatWSResponse
	if err := json.Unmarshal(respBody, &response); err != nil {
		return nil, false, err
	}

	if response.Status != "ok" {
		return nil, false, fmt.Errorf("get_forward_msg 返回错误: %s", response.Wording)
	}

	if response.Data == nil {
		return nil, false, fmt.Errorf("get_forward_msg 返回空数据")
	}

	var forwardResp ForwardMessageResponse
	if err := json.Unmarshal(response.Data, &forwardResp); err != nil {
		return nil, false, err
	}

	var contentArray []map[string]interface{}
	var contentStr string
	var hasImages bool
	var messageCount int

	for _, msg := range forwardResp.Messages {
		// 限制消息数量（99条以内）
		if messageCount >= 99 {
			ellipsis := "...(消息过多)"
			if hasImages {
				contentArray = append(contentArray, map[string]interface{}{
					"type": "text",
					"text": ellipsis,
				})
			} else {
				contentStr += ellipsis
			}
			break
		}

		senderName := GetUserName(msg.GroupID, msg.UserID)
		prefix := senderName + ": "

		for _, seg := range msg.Message {
			switch seg.Type {
			case "text":
				var textData TextData
				if err := json.Unmarshal(seg.Data, &textData); err == nil {
					if hasImages {
						contentArray = append(contentArray, map[string]interface{}{
							"type": "text",
							"text": prefix + textData.Text + "\n",
						})
						prefix = ""
					} else {
						contentStr += prefix + textData.Text + "\n"
						prefix = ""
					}
				}
			case "image":
				var imageData ImageData
				if err := json.Unmarshal(seg.Data, &imageData); err == nil {
					hasImages = true
					// 将之前的 contentStr 转换为数组
					if len(contentArray) == 0 && contentStr != "" {
						contentArray = append(contentArray, map[string]interface{}{
							"type": "text",
							"text": contentStr,
						})
						contentStr = ""
					}
					if prefix != "" {
						contentArray = append(contentArray, map[string]interface{}{
							"type": "text",
							"text": prefix,
						})
						prefix = ""
					}
					contentArray = append(contentArray, map[string]interface{}{
						"type": "image_url",
						"image_url": map[string]string{
							"url": imageData.URL,
						},
					})
				}
			}
		}

		messageCount++
	}

	// 限制总长度
	if len(contentArray) > 100 {
		contentArray = contentArray[:100]
		contentArray = append(contentArray, map[string]interface{}{
			"type": "text",
			"text": "...(内容过长)",
		})
	}

	if hasImages {
		return contentArray, true, nil
	}

	return contentStr, false, nil
}

// SendToLunarCore 发送消息到 Lunar Core
func SendToLunarCore(messages []OpenAIMessage) error {
	url := AppConfig.QQAdapter.LunarCoreUrl + "/write/message"

	requestData := BatchMessageRequest{
		Messages: messages,
	}

	body, err := json.Marshal(requestData)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	log.Printf("lunar_core 响应: %s", respBody)

	return nil
}

// SendGroupTextMessage 发送群文本消息
func SendGroupTextMessage(groupID int64, content string) error {
	baseURL := GetNapcatHTTPBaseURL()
	url := baseURL + "/send_group_msg"
	token := AppConfig.QQAdapter.NapcatWsToken

	message := []map[string]interface{}{
		{
			"type": "text",
			"data": map[string]string{
				"text": content,
			},
		},
	}

	requestData := map[string]interface{}{
		"group_id": groupID,
		"message":  message,
	}

	body, err := json.Marshal(requestData)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	log.Printf("发送群消息响应: %s", respBody)

	return nil
}

// SendGroupImageMessage 发送群图片消息
func SendGroupImageMessage(groupID int64, images []string) error {
	baseURL := GetNapcatHTTPBaseURL()
	url := baseURL + "/send_group_msg"
	token := AppConfig.QQAdapter.NapcatWsToken

	message := make([]map[string]interface{}, 0, len(images))
	for _, img := range images {
		message = append(message, map[string]interface{}{
			"type": "image",
			"data": map[string]string{
				"base64": img,
			},
		})
	}

	requestData := map[string]interface{}{
		"group_id": groupID,
		"message":  message,
	}

	body, err := json.Marshal(requestData)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	log.Printf("发送群图片消息响应: %s", respBody)

	return nil
}
