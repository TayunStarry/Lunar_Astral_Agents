package DialogueBridging

import (
	"encoding/json"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/websocket"
)

// BridgeStructuredMessage 将统一集线器广播的结构化消息转发到 QQ（桥接未启用时直接返回）。
// 由装配层注入为 websocket.OnStructuredMessage：仅结构化广播触发，原始字节广播不触发。
func BridgeStructuredMessage(response websocket.WSResponse) {
	if !IsBridgingEnabled() {
		return
	}

	switch response.Type {
	case "context":
		// Data 可能是任意结构体，通过 JSON 序列化/反序列化提取字段
		dataBytes, err := json.Marshal(response.Data)
		if err != nil {
			logger.SubError("LunarCore", "Napcat", "桥接序列化数据失败: %v", err)
			return
		}
		var dataMap map[string]any
		if err := json.Unmarshal(dataBytes, &dataMap); err != nil {
			logger.SubError("LunarCore", "Napcat", "桥接解析数据失败: %v", err)
			return
		}
		msgType, _ := dataMap["type"].(string)
		content, _ := dataMap["content"].(string)
		auto, _ := dataMap["auto"].(bool)
		// 音乐消息没有 content，单独取音频字段转发；是否落到 QQ 由当前是否有 QQ 请求决定
		if msgType == "music" {
			audio, _ := dataMap["audio"].(string)
			title, _ := dataMap["title"].(string)
			summary, _ := dataMap["summary"].(string)
			duration, _ := dataMap["duration"].(float64)
			go HandleAgentMusicResponse(audio, title, summary, duration)
			return
		}
		if content != "" {
			go HandleAgentResponse(msgType, content, auto)
		}
	case "image":
		// 图片消息：提取图片 URL 列表并转发到 QQ（本地服务路径会补全为本机 HTTP 地址）
		dataBytes, err := json.Marshal(response.Data)
		if err != nil {
			logger.SubError("LunarCore", "Napcat", "桥接序列化图片数据失败: %v", err)
			return
		}
		var dataMap map[string]any
		if err := json.Unmarshal(dataBytes, &dataMap); err != nil {
			logger.SubError("LunarCore", "Napcat", "桥接解析图片数据失败: %v", err)
			return
		}
		// 提取 images 数组
		rawImages, ok := dataMap["images"].([]any)
		if !ok {
			return
		}
		var images []string
		for _, img := range rawImages {
			if imgStr, ok := img.(string); ok {
				images = append(images, imgStr)
			}
		}
		if len(images) > 0 {
			go HandleAgentImageResponse(images)
		}
	}
}
