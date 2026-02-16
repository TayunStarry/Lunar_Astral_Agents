package core

import (
	"log"                              // 日志记录
	"nap_cat_bridging/internal/openai" // OpenAI 客户端
	"strings"                          // 字符串处理
	"time"                             // 时间处理
)

// MainLoop 执行主循环
func (app *Application) MainLoop() error {
	groupListReceived := false

	for {
		// 读取消息
		messageBytes, err := app.WSClient.ReadMessage()
		if err != nil {
			log.Printf("读取消息失败: %v，尝试重连", err)
			// 尝试重连
			if err = app.WSClient.Connect(); err != nil {
				log.Printf("重连失败: %v，%v秒后重试", err, app.Config.PollInterval)
				time.Sleep(time.Duration(app.Config.PollInterval) * time.Second)
				continue
			}
			continue
		}

		// 尝试解析群成员列表响应
		if err = app.MessageProcessor.ParseGroupMemberListResponse(messageBytes); err == nil {
			// 成功解析群成员列表响应，继续循环
			continue
		}

		// 尝试解析群列表响应
		if !groupListReceived {
			if err = app.MessageProcessor.ParseGroupListResponse(messageBytes); err == nil {
				groupListReceived = true

				log.Printf("%s", strings.Repeat("-=", 28))
				// 为每个需要监听的群发送获取成员列表的请求
				for _, groupID := range app.Config.ListenGroupIDs {
					_, err = app.WSClient.SendMessage("get_group_member_list", map[string]any{
						"group_id": groupID,
						"no_cache": false,
					})
					if err != nil {
						log.Printf("发送get_group_member_list请求失败 (群 ID: %d): %v", groupID, err)
					}
				}
				continue
			}
		}

		// 尝试处理群消息
		groupID, messageContent, err := app.MessageProcessor.HandleGroupMessage(messageBytes)
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

		// 添加用户消息到历史
		app.HistoryManager.AddMessage(groupID, app.createUserMessage(messageContent))

		// 调用OpenAI API
		response, err := app.OpenAIClient.CallAPI(app.HistoryManager.GetMessages(groupID))
		if err != nil {
			log.Printf("调用OpenAI API失败: %v", err)
			// 发送错误消息
			app.MessageProcessor.SendGroupMsg(groupID, "抱歉，处理请求失败，请稍后再试")
			continue
		}

		// 添加助手回复到历史
		app.HistoryManager.AddMessage(groupID, app.createAssistantMessage(response))

		// 发送群消息
		if err := app.MessageProcessor.SendGroupMsg(groupID, response); err != nil {
			log.Printf("发送群消息失败: %v", err)
		}
	}
}

// createUserMessage 创建用户消息
func (app *Application) createUserMessage(content any) openai.Message {
	return openai.CreateMessage("user", content)
}

// createAssistantMessage 创建助手消息
func (app *Application) createAssistantMessage(content string) openai.Message {
	return openai.CreateMessage("assistant", content)
}
