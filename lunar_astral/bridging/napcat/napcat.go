package napcat

// Napcat WebSocket/HTTP 客户端实现

import (
	"LunarSubsystem/LoggerGeneral"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gorilla/websocket"
)

// ConnectToNapcatWebSocket 连接到 Napcat WebSocket 服务器
// 成功连接后持续读取消息，断开时返回
func ConnectToNapcatWebSocket(messageHandler func([]byte)) error {
	url := bridgeConfig.BridgingPath
	token := bridgeConfig.BridgingToken

	LoggerGeneral.SubInfo("LunarCore", "Napcat", "正在连接: %s", url)

	headers := http.Header{}
	if token != "" {
		headers.Set("Authorization", "Bearer "+token)
	}

	conn, _, err := websocket.DefaultDialer.Dial(url, headers)
	if err != nil {
		return fmt.Errorf("连接失败: %v", err)
	}
	defer conn.Close()

	// 连接成功，立即设置状态为已连接
	setBridgeState(BridgeConnected)
	LoggerGeneral.SubInfo("LunarCore", "Napcat", "成功连接到 napcat 服务器")

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			// 连接断开，重置状态
			setBridgeState(BridgeDisconnected)
			return fmt.Errorf("读取消息失败: %v", err)
		}
		messageHandler(message)
	}
}

// getNapcatHTTPBaseURL 将 napcat ws 地址转换为 http 地址
func getNapcatHTTPBaseURL() string {
	return strings.Replace(bridgeConfig.BridgingPath, "ws://", "http://", 1)
}

// SendGroupTextMessage 发送群文本消息
func SendGroupTextMessage(groupID int64, content string) error {
	baseURL := getNapcatHTTPBaseURL()
	url := baseURL + "/send_group_msg"
	token := bridgeConfig.BridgingToken

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
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	LoggerGeneral.SubInfo("LunarCore", "Napcat", "发送群消息响应: %s", respBody)
	return nil
}

// SendGroupImageMessage 发送群图片消息（base64 编码）
func SendGroupImageMessage(groupID int64, images []string) error {
	baseURL := getNapcatHTTPBaseURL()
	url := baseURL + "/send_group_msg"
	token := bridgeConfig.BridgingToken

	message := make([]map[string]interface{}, 0, len(images))
	for _, img := range images {
		message = append(message, map[string]interface{}{
			"type": "image",
			"data": map[string]string{
				"file": "base64://" + img,
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
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	LoggerGeneral.SubInfo("LunarCore", "Napcat", "发送群图片消息响应: %s", respBody)
	return nil
}

// GetMessageContent 通过 get_msg API 获取单条消息的文本内容
func GetMessageContent(messageID string) (string, error) {
	baseURL := getNapcatHTTPBaseURL()
	url := baseURL + "/get_msg"
	token := bridgeConfig.BridgingToken

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
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

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
