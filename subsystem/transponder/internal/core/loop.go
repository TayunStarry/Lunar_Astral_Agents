package core

import (
	"encoding/json"              // JSON 编码/解码
	"fmt"                        // 格式化输出
	"log"                        // 日志记录
	"strings"                    // 字符串处理
	"time"                       // 时间处理
	"transponder/internal/agent" // Agent 客户端
	"transponder/internal/utils" // 工具包
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
		if err = class.Processor.ParseGroupMemberListResponse(messageBytes); err == nil {
			// 成功解析群成员列表响应，继续循环
			continue
		}

		// 尝试解析群列表响应
		if !groupListReceived {
			if err = class.Processor.ParseGroupListResponse(messageBytes); err == nil {
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
		groupID, messageContent, err := class.Processor.HandleGroupMessage(messageBytes)
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
			class.Processor.SendGroupMsg(groupID, "抱歉，处理请求失败，请稍后再试")
			continue
		}

		// 调用Agent API
		response, err := class.AgentClient.CallAgent(messages, class.Processor, 0)
		if err != nil {
			log.Printf("调用Agent API失败: %v", err)
			// 发送错误消息
			class.Processor.SendGroupMsg(groupID, "抱歉，处理请求失败，请稍后再试")
			continue
		}

		// 发送群消息
		if err := class.Processor.SendGroupMsg(groupID, response); err != nil {
			log.Printf("发送群消息失败: %v", err)
		}
	}
}

// getMessageHistoryForOpenAI 获取群消息历史并转换为Agent消息格式
func (class *Application) getMessageHistoryForOpenAI(groupID int64) ([]agent.Message, error) {
	// 发送获取历史消息的请求
	echo, err := class.WSClient.GetGroupMessageHistory(groupID)
	if err != nil {
		return nil, err
	}

	// 等待并读取响应
	var messages []agent.Message
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
			var response utils.WSResponse
			if err := json.Unmarshal(messageBytes, &response); err != nil {
				continue
			}

			// 检查是否是我们的请求响应
			if !strings.Contains(response.Echo, echo) {
				continue
			}
			// 解析群消息历史数据
			if ok, msgList := class.getMessageHistoryValidity(response); ok {
				// 遍历历史消息
				for _, msgItem := range msgList {
					if _, ok := msgItem.(map[string]any); !ok {
						continue
					}
					// 提取发送者信息
					senderName := ""
					// 检查是否包含发送者信息
					if _, ok := msgItem.(map[string]any)["sender"].(map[string]any); !ok {
						continue
					}
					// 提取昵称作为发送者名称
					if nickname, ok := msgItem.(map[string]any)["sender"].(map[string]any)["nickname"].(string); ok {
						senderName = nickname
					}
					// 提取群名片作为发送者名称
					if card, ok := msgItem.(map[string]any)["sender"].(map[string]any)["card"].(string); ok && card != "" {
						senderName = card
					}
					// 提取消息角色
					roleName := "user"
					// 提取消息内容
					content := class.Processor.ProcessMessageContent(msgItem.(map[string]any), groupID, senderName)
					// 检查是否包含触发关键词
					if class.Processor.ContainsTriggerKeyword(senderName) {
						roleName = "assistant"
					}
					// 创建Agent消息
					messages = append(messages, agent.Message{Role: roleName, Content: content})
				}
			}

			responseReceived = true
		}
	}

	return messages, nil
}

// getMessageHistoryValidity 解析历史数据有效性
func (class *Application) getMessageHistoryValidity(response utils.WSResponse) (bool, []any) {
	if response.Status != "ok" || response.Data == nil {
		return false, nil
	}
	if _, ok := response.Data.(map[string]any); !ok {
		return false, nil
	}
	if _, ok := response.Data.(map[string]any)["messages"].([]any); !ok {
		return false, nil
	}
	return true, response.Data.(map[string]any)["messages"].([]any)
}
