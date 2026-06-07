package napcat

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"bridge_adapter/pkg/config"
	"bridge_adapter/pkg/logger"
	"bridge_adapter/pkg/types"

	"github.com/gorilla/websocket"
)

var HTTPClient = &http.Client{Timeout: 10 * time.Second}

func NewHTTPRequest(method, url string, body io.Reader) (*http.Request, error) {
	return http.NewRequest(method, url, body)
}

func ReadResponseBody(resp *http.Response) ([]byte, error) {
	return io.ReadAll(resp.Body)
}

func FetchGroupMembers() {
	token := config.AppConfig.QQAdapter.NapcatWsToken
	baseURL := config.GetNapcatHTTPBaseURL()

	for _, groupID := range config.AppConfig.QQAdapter.ListenGroupIds {
		logger.Info("正在获取群 %d 的成员列表", groupID)
		err := FetchGroupMemberList(baseURL, token, groupID)
		if err != nil {
			logger.Error("获取群 %d 成员列表失败: %v", groupID, err)
		}
	}
}

func FetchGroupMemberList(baseURL, token string, groupID int64) error {
	url := baseURL + "/get_group_member_list"

	body, err := json.Marshal(map[string]interface{}{
		"group_id": groupID,
	})
	if err != nil {
		return err
	}

	req, err := NewHTTPRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, err := ReadResponseBody(resp)
	if err != nil {
		return err
	}

	var response types.NapcatWSResponse
	if err := json.Unmarshal(respBody, &response); err != nil {
		return err
	}

	if response.Status == "ok" && response.Data != nil {
		var data interface{}
		if err := json.Unmarshal(response.Data, &data); err != nil {
			return err
		}

		if memberList, ok := data.([]interface{}); ok {
			if _, ok := config.GroupMembers[groupID]; !ok {
				config.GroupMembers[groupID] = make(map[int64]string)
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
						config.GroupMembers[groupID][userID] = nickname
					}
				}
			}
			logger.Info("群 %d 成员列表获取成功，共 %d 个成员", groupID, len(memberList))
		}
	}

	return nil
}

func GetMessageContent(messageID string) (string, error) {
	token := config.AppConfig.QQAdapter.NapcatWsToken
	baseURL := config.GetNapcatHTTPBaseURL()
	url := baseURL + "/get_msg"

	body, err := json.Marshal(map[string]interface{}{
		"message_id": messageID,
	})
	if err != nil {
		return "", err
	}

	req, err := NewHTTPRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := HTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBody, err := ReadResponseBody(resp)
	if err != nil {
		return "", err
	}

	var response types.NapcatWSResponse
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

func GetForwardMessageContent(forwardID string, groupID int64) (interface{}, bool, error) {
	token := config.AppConfig.QQAdapter.NapcatWsToken
	baseURL := config.GetNapcatHTTPBaseURL()
	url := baseURL + "/get_forward_msg"

	body, err := json.Marshal(map[string]interface{}{
		"message_id": forwardID,
	})
	if err != nil {
		return nil, false, err
	}

	req, err := NewHTTPRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return nil, false, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := HTTPClient.Do(req)
	if err != nil {
		return nil, false, err
	}
	defer resp.Body.Close()

	respBody, err := ReadResponseBody(resp)
	if err != nil {
		return nil, false, err
	}

	var response types.NapcatWSResponse
	if err := json.Unmarshal(respBody, &response); err != nil {
		return nil, false, err
	}

	if response.Status != "ok" {
		return nil, false, fmt.Errorf("get_forward_msg 返回错误: %s", response.Wording)
	}

	if response.Data == nil {
		return nil, false, fmt.Errorf("get_forward_msg 返回空数据")
	}

	var forwardResp types.ForwardMessageResponse
	if err := json.Unmarshal(response.Data, &forwardResp); err != nil {
		return nil, false, err
	}

	return ParseForwardMessages(forwardResp.Messages, groupID)
}

func ParseForwardMessages(messages []types.NapcatMessage, groupID int64) (interface{}, bool, error) {
	var contentArray []map[string]interface{}
	var contentStr string
	var hasImages bool
	var messageCount int

	for _, msg := range messages {
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

		senderName := config.GetUserName(msg.GroupID, msg.UserID)
		prefix := senderName + ": "

		for _, seg := range msg.Message {
			switch seg.Type {
			case "text":
				var textData types.TextData
				if err := json.Unmarshal(seg.Data, &textData); err == nil {
					textContent := prefix + textData.Text + "\n"
					if hasImages {
						contentArray = append(contentArray, map[string]interface{}{
							"type": "text",
							"text": textContent,
						})
					} else {
						contentStr += textContent
					}
					prefix = ""
				}
			case "image":
				var imageData types.ImageData
				if err := json.Unmarshal(seg.Data, &imageData); err == nil {
					hasImages = true
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
			case "at":
				var atData types.AtData
				if err := json.Unmarshal(seg.Data, &atData); err == nil {
					atUserID, _ := strconv.ParseInt(atData.QQ, 10, 64)
					userName := config.GetUserName(msg.GroupID, atUserID)
					atText := prefix + "@" + userName + " "
					if hasImages {
						contentArray = append(contentArray, map[string]interface{}{
							"type": "text",
							"text": atText,
						})
					} else {
						contentStr += atText
					}
					prefix = ""
				}
			case "reply":
				var replyData types.ReplyData
				if err := json.Unmarshal(seg.Data, &replyData); err == nil {
					replyContent, _ := GetMessageContent(replyData.ID)
					replyText := prefix + "[回复: " + replyContent + "] "
					if hasImages {
						contentArray = append(contentArray, map[string]interface{}{
							"type": "text",
							"text": replyText,
						})
					} else {
						contentStr += replyText
					}
					prefix = ""
				}
			case "file":
				var fileData types.FileData
				if err := json.Unmarshal(seg.Data, &fileData); err == nil {
					fileText := prefix + "[文件] " + fileData.File + " "
					if hasImages {
						contentArray = append(contentArray, map[string]interface{}{
							"type": "text",
							"text": fileText,
						})
					} else {
						contentStr += fileText
					}
					prefix = ""
				}
			}
		}

		messageCount++
	}

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

func SendGroupTextMessage(groupID int64, content string) error {
	baseURL := config.GetNapcatHTTPBaseURL()
	url := baseURL + "/send_group_msg"
	token := config.AppConfig.QQAdapter.NapcatWsToken

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

	req, err := NewHTTPRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, err := ReadResponseBody(resp)
	if err != nil {
		return err
	}

	logger.Debug("发送群消息响应: %s", respBody)

	return nil
}

func SendGroupImageMessage(groupID int64, images []string) error {
	baseURL := config.GetNapcatHTTPBaseURL()
	url := baseURL + "/send_group_msg"
	token := config.AppConfig.QQAdapter.NapcatWsToken

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

	req, err := NewHTTPRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, err := ReadResponseBody(resp)
	if err != nil {
		return err
	}

	logger.Debug("发送群图片消息响应: %s", respBody)

	return nil
}

func ConnectToNapcatWebSocket(messageHandler func([]byte)) {
	url := config.AppConfig.QQAdapter.NapcatWsServer
	token := config.AppConfig.QQAdapter.NapcatWsToken

	logger.Info("正在连接到 napcat_ws_server: %s", url)

	headers := http.Header{}
	if token != "" {
		headers.Set("Authorization", "Bearer "+token)
	}

	conn, _, err := websocket.DefaultDialer.Dial(url, headers)
	if err != nil {
		logger.Error("连接 napcat_ws_server 失败: %v", err)
		return
	}
	defer conn.Close()

	logger.Info("成功连接到 napcat_ws_server")

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			logger.Error("从 napcat_ws_server 读取消息失败: %v", err)
			break
		}
		if config.AppConfig.QQAdapter.DisplayLogs {
			logger.Debug("收到 napcat_ws_server 消息: %s", message)
		}
		messageHandler(message)
	}
}
