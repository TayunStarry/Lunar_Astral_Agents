package core

import (
	"encoding/json"                  // JSON 编码/解码
	"fmt"                            // 格式化输出
	"log"                            // 日志记录
	"strings"                        // 字符串处理
	"time"                           // 时间处理
	"transponder/internal/openai"    // OpenAI 客户端
	"transponder/internal/websocket" // WebSocket 客户端
)

// MainLoop 执行主循环
func (class *Application) MainLoop() error {
	groupListReceived := false

	for {
		// 读取消息
		messageBytes, err := class.WSClient.ReadMessage()
		if err != nil {
			log.Printf("读取消息失败: %v，尝试重连", err)
			// 尝试重连
			if err = class.WSClient.Connect(); err != nil {
				log.Printf("重连失败: %v，%v秒后重试", err, class.Config.PollInterval)
				time.Sleep(time.Duration(class.Config.PollInterval) * time.Second)
				continue
			}
			continue
		}

		// 尝试解析群成员列表响应
		if err = class.MessageProcessor.ParseGroupMemberListResponse(messageBytes); err == nil {
			// 成功解析群成员列表响应，继续循环
			continue
		}

		// 尝试解析群列表响应
		if !groupListReceived {
			if err = class.MessageProcessor.ParseGroupListResponse(messageBytes); err == nil {
				groupListReceived = true

				log.Printf("%s", strings.Repeat("-=", 28))
				// 为每个需要监听的群发送获取成员列表的请求
				for _, groupID := range class.Config.ListenGroupIDs {
					_, err = class.WSClient.SendMessage("get_group_member_list", map[string]any{"group_id": groupID, "no_cache": false})
					if err != nil {
						log.Printf("发送get_group_member_list请求失败 (群 ID: %d): %v", groupID, err)
					}
				}
				continue
			}
		}

		// 尝试处理群消息
		groupID, messageContent, err := class.MessageProcessor.HandleGroupMessage(messageBytes)
		if err != nil {
			// 不是群消息或处理失败，继续循环
			continue
		}

		// 如果groupID为0，说明消息被过滤了，继续循环
		if groupID == 0 {
			continue
		}

		// 如果messageContent为nil，说明消息处理失败，继续循环
		if messageContent == nil {
			continue
		}

		// 获取历史消息并转换为OpenAI消息格式
		messages, err := class.getMessageHistoryForOpenAI(groupID)
		if err != nil {
			log.Printf("获取历史消息失败: %v", err)
			// 发送错误消息
			class.MessageProcessor.SendGroupMsg(groupID, "抱歉，处理请求失败，请稍后再试")
			continue
		}

		// 调用OpenAI API
		response, err := class.OpenAIClient.CallAPI(messages, class.MessageProcessor)
		if err != nil {
			log.Printf("调用OpenAI API失败: %v", err)
			// 发送错误消息
			class.MessageProcessor.SendGroupMsg(groupID, "抱歉，处理请求失败，请稍后再试")
			continue
		}

		// 发送群消息
		if err := class.MessageProcessor.SendGroupMsg(groupID, response); err != nil {
			log.Printf("发送群消息失败: %v", err)
		}
	}
}

// getMessageHistoryForOpenAI 获取群消息历史并转换为OpenAI消息格式
func (class *Application) getMessageHistoryForOpenAI(groupID int64) ([]openai.Message, error) {
	// 发送获取历史消息的请求
	echo, err := class.WSClient.GetGroupMessageHistory(groupID)
	if err != nil {
		return nil, err
	}

	// 等待并读取响应
	var messages []openai.Message
	responseReceived := false

	// 读取响应，最多等待5秒
	timeout := time.After(5 * time.Second)
	for !responseReceived {
		select {
		case <-timeout:
			return nil, fmt.Errorf("获取历史消息响应超时")
		default:
			// 读取消息
			messageBytes, err := class.WSClient.ReadMessage()
			if err != nil {
				continue
			}

			// 解析响应
			var response websocket.WSResponse
			if err := json.Unmarshal(messageBytes, &response); err != nil {
				continue
			}

			// 检查是否是我们的请求响应
			if !strings.Contains(response.Echo, echo) {
				continue
			}

			// 解析群消息历史数据
			if response.Status == "ok" && response.Data != nil {
				if dataMap, ok := response.Data.(map[string]interface{}); ok {
					if msgList, ok := dataMap["messages"].([]interface{}); ok {
						// 遍历历史消息
						for _, msgItem := range msgList {
							if msg, ok := msgItem.(map[string]interface{}); ok {
								// 提取发送者信息
								senderName := ""
								if sender, ok := msg["sender"].(map[string]interface{}); ok {
									if nickname, ok := sender["nickname"].(string); ok {
										senderName = nickname
									}
									if card, ok := sender["card"].(string); ok && card != "" {
										senderName = card
									}
								}

								// 提取消息内容
								content := class.MessageProcessor.ProcessMessageContent(msg, groupID, senderName)

								// 创建OpenAI消息
								messages = append(messages, openai.Message{
									Role:    "user",
									Content: content,
								})
							}
						}
					}
				}
			}

			responseReceived = true
		}
	}

	return messages, nil
}

// createUserMessage 创建用户消息
func (class *Application) createUserMessage(content any) openai.Message {
	return openai.CreateMessage("user", content)
}

// createAssistantMessage 创建助手消息
func (class *Application) createAssistantMessage(content string) openai.Message {
	return openai.CreateMessage("assistant", content)
}
