package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
)

type Config struct {
	QQAdapter QQAdapter `json:"qq_adapter"`
}

type QQAdapter struct {
	NapcatWsServer  string   `json:"napcat_ws_server"`
	NapcatWsToken   string   `json:"napcat_ws_token"`
	LunarCoreUrl    string   `json:"lunar_core_url"`
	LunarWsServer   string   `json:"lunar_ws_server"`
	ListenGroupIds  []int64  `json:"listen_group_ids"`
	PollInterval    int      `json:"poll_interval"`
	TriggerKeywords []string `json:"trigger_keywords"`
	DefaultReply    string   `json:"default_reply"`
}

type NapcatMessage struct {
	SelfID      int64            `json:"self_id"`
	UserID      int64            `json:"user_id"`
	MessageID   int64            `json:"message_id"`
	Sender      Sender           `json:"sender"`
	GroupID     int64            `json:"group_id"`
	Message     []MessageSegment `json:"message"`
	PostType    string           `json:"post_type"`
	MessageType string           `json:"message_type"`
}

type Sender struct {
	UserID   int64  `json:"user_id"`
	Nickname string `json:"nickname"`
	Role     string `json:"role"`
}

type MessageSegment struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

type TextData struct {
	Text string `json:"text"`
}

type ReplyData struct {
	ID string `json:"id"`
}

type ImageData struct {
	Summary  string `json:"summary"`
	File     string `json:"file"`
	SubType  int    `json:"sub_type"`
	URL      string `json:"url"`
	FileSize string `json:"file_size"`
}

type AtData struct {
	QQ string `json:"qq"`
}

type ForwardData struct {
	ID string `json:"id"`
}

type FileData struct {
	File     string `json:"file"`
	FileID   string `json:"file_id"`
	FileSize string `json:"file_size"`
	URL      string `json:"url"`
}

type LunarMessage struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

type LunarContextData struct {
	Type    string `json:"type"`
	Content string `json:"content"`
}

type LunarImageData struct {
	Type   string   `json:"type"`
	Images []string `json:"images"`
}

type OpenAIMessage struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"`
}

type BatchMessageRequest struct {
	Messages []OpenAIMessage `json:"messages"`
}

type NapcatWSResponse struct {
	Status  string          `json:"status"`
	Retcode int             `json:"retcode"`
	Data    json.RawMessage `json:"data"`
	Message string          `json:"message"`
	Wording string          `json:"wording"`
	Echo    string          `json:"echo"`
	Stream  string          `json:"stream"`
}

type ForwardMessageResponse struct {
	Messages []NapcatMessage `json:"messages"`
}

var (
	config       Config
	httpClient   = &http.Client{Timeout: 10 * time.Second}
	groupMembers = make(map[int64]map[int64]string)
	displayLogs  = false
)

func main() {
	configFile := "local_data/lunar_config.json"

	configData, err := os.ReadFile(configFile)
	if err != nil {
		log.Fatalf("读取配置文件失败: %v", err)
	}

	if err := json.Unmarshal(configData, &config); err != nil {
		log.Fatalf("解析配置文件失败: %v", err)
	}

	log.Println("成功读取配置文件")
	log.Printf("napcat_ws_server: %s", config.QQAdapter.NapcatWsServer)
	log.Printf("lunar_ws_server: %s", config.QQAdapter.LunarWsServer)
	log.Printf("listen_group_ids: %v", config.QQAdapter.ListenGroupIds)
	log.Printf("trigger_keywords: %v", config.QQAdapter.TriggerKeywords)

	go fetchGroupMembers()
	go connectToNapcatWebSocket()
	go connectToLunarWebSocket()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("程序退出")
}

func getNapcatHTTPBaseURL() string {
	return strings.Replace(config.QQAdapter.NapcatWsServer, "ws://", "http://", 1)
}

func fetchGroupMembers() {
	token := config.QQAdapter.NapcatWsToken
	baseURL := getNapcatHTTPBaseURL()

	for _, groupID := range config.QQAdapter.ListenGroupIds {
		log.Printf("正在获取群 %d 的成员列表", groupID)
		err := fetchGroupMemberList(baseURL, token, groupID)
		if err != nil {
			log.Printf("获取群 %d 成员列表失败: %v", groupID, err)
		}
	}
}

func fetchGroupMemberList(baseURL, token string, groupID int64) error {
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
			if _, ok := groupMembers[groupID]; !ok {
				groupMembers[groupID] = make(map[int64]string)
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
						groupMembers[groupID][userID] = nickname
					}
					if displayLogs {
						log.Printf("群 %d 成员: %d - %s", groupID, userID, nickname)
					}
				}
			}
			log.Printf("群 %d 成员列表获取成功，共 %d 个成员", groupID, len(memberList))
		}
	}

	return nil
}

func getUserName(groupID int64, userID int64) string {
	nickname := ""
	if groupID > 0 && userID > 0 {
		if members, ok := groupMembers[groupID]; ok {
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

func connectToNapcatWebSocket() {
	url := config.QQAdapter.NapcatWsServer
	token := config.QQAdapter.NapcatWsToken

	log.Printf("正在连接到 napcat_ws_server: %s", url)

	headers := http.Header{}
	if token != "" {
		headers.Set("Authorization", "Bearer "+token)
	}

	conn, _, err := websocket.DefaultDialer.Dial(url, headers)
	if err != nil {
		log.Printf("连接 napcat_ws_server 失败: %v", err)
		return
	}
	defer conn.Close()

	log.Printf("成功连接到 napcat_ws_server")

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			log.Printf("从 napcat_ws_server 读取消息失败: %v", err)
			break
		}

		if displayLogs {
			log.Printf("收到 napcat_ws_server 消息: %s", message)
		}
		handleNapcatMessage(message)
	}
}

func connectToLunarWebSocket() {
	url := config.QQAdapter.LunarWsServer

	log.Printf("正在连接到 lunar_ws_server: %s", url)

	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		log.Printf("连接 lunar_ws_server 失败: %v", err)
		return
	}
	defer conn.Close()

	log.Printf("成功连接到 lunar_ws_server")

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			log.Printf("从 lunar_ws_server 读取消息失败: %v", err)
			break
		}
		if displayLogs {
			log.Printf("收到 lunar_ws_server 消息: %s", message)
		}
		handleLunarMessage(message)
	}
}

func handleNapcatMessage(message []byte) {
	var napcatMsg NapcatMessage
	if err := json.Unmarshal(message, &napcatMsg); err != nil {
		log.Printf("解析 napcat 消息失败: %v", err)
		return
	}

	if napcatMsg.UserID == napcatMsg.SelfID {
		return
	}

	if !isInListenGroup(napcatMsg.GroupID) {
		return
	}

	if napcatMsg.MessageType != "group" {
		return
	}

	content, err := parseMessageSegments(napcatMsg.Message, napcatMsg.GroupID)
	if err != nil {
		log.Printf("解析消息段失败: %v", err)
		return
	}

	contentStr, ok := content.(string)
	if !ok {
		log.Printf("消息内容不是字符串类型")
		return
	}

	if !containsTriggerKeyword(contentStr) {
		log.Printf("消息不包含触发关键词，跳过发送: %s", contentStr)
		return
	}

	// 获取发送者名称
	senderName := getUserName(napcatMsg.GroupID, napcatMsg.UserID)
	// 添加发送者名称前缀
	finalContent := senderName + " : " + contentStr

	openAIMessages := []OpenAIMessage{
		{
			Role:    "user",
			Content: finalContent,
		},
	}

	err = sendToLunarCore(openAIMessages)
	if err != nil {
		log.Printf("发送消息到 lunar_core 失败: %v", err)
	}
}

func handleLunarMessage(message []byte) {
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
	default:
		log.Printf("未知的 lunar 消息类型: %s", lunarMsg.Type)
	}
}

func handleLunarContextMessage(data json.RawMessage) {
	var contextData LunarContextData
	if err := json.Unmarshal(data, &contextData); err != nil {
		log.Printf("解析 lunar 上下文消息失败: %v", err)
		return
	}

	groupID := getRandomGroupID()
	if groupID == 0 {
		log.Println("没有可用的群组 ID")
		return
	}

	err := sendGroupTextMessage(groupID, contextData.Content)
	if err != nil {
		log.Printf("发送群文本消息失败: %v", err)
	}
}

func handleLunarImageMessage(data json.RawMessage) {
	var imageData LunarImageData
	if err := json.Unmarshal(data, &imageData); err != nil {
		log.Printf("解析 lunar 图片消息失败: %v", err)
		return
	}

	groupID := getRandomGroupID()
	if groupID == 0 {
		log.Println("没有可用的群组 ID")
		return
	}

	err := sendGroupImageMessage(groupID, imageData.Images)
	if err != nil {
		log.Printf("发送群图片消息失败: %v", err)
	}
}

func parseMessageSegments(segments []MessageSegment, groupID int64) (interface{}, error) {
	var content string
	var hasImages bool
	var imageContents []map[string]interface{}

	for _, segment := range segments {
		switch segment.Type {
		case "text":
			var textData TextData
			if err := json.Unmarshal(segment.Data, &textData); err != nil {
				return nil, err
			}
			content += textData.Text
		case "reply":
			var replyData ReplyData
			if err := json.Unmarshal(segment.Data, &replyData); err != nil {
				return nil, err
			}
			replyContent, err := getMessageContent(replyData.ID)
			if err != nil {
				log.Printf("获取回复消息内容失败: %v", err)
				content += "[回复] "
			} else {
				content += "[回复: " + replyContent + "] "
			}
		case "image":
			var imageData ImageData
			if err := json.Unmarshal(segment.Data, &imageData); err != nil {
				return nil, err
			}
			hasImages = true
			imageContents = append(imageContents, map[string]interface{}{
				"type": "image_url",
				"image_url": map[string]string{
					"url": imageData.URL,
				},
			})
		case "at":
			var atData AtData
			if err := json.Unmarshal(segment.Data, &atData); err != nil {
				return nil, err
			}
			atUserID, err := strconv.ParseInt(atData.QQ, 10, 64)
			if err != nil {
				content += "@" + atData.QQ + " "
			} else {
				userName := getUserName(groupID, atUserID)
				content += "@" + userName + " "
			}
		case "forward":
			var forwardData ForwardData
			if err := json.Unmarshal(segment.Data, &forwardData); err != nil {
				return nil, err
			}
			forwardContent, err := getForwardMessageContent(forwardData.ID)
			if err != nil {
				log.Printf("获取转发消息内容失败: %v", err)
				content += "[转发消息] "
			} else {
				content += "[转发消息: " + forwardContent + "] "
			}
		case "file":
			var fileData FileData
			if err := json.Unmarshal(segment.Data, &fileData); err != nil {
				return nil, err
			}
			content += "[文件] " + fileData.File + " " + fileData.URL + " "
		default:
			log.Printf("未知的消息段类型: %s", segment.Type)
		}
	}

	if hasImages {
		if content != "" {
			imageContents = append([]map[string]interface{}{
				{
					"type": "text",
					"text": content,
				},
			}, imageContents...)
		}
		return imageContents, nil
	}

	return content, nil
}

func getMessageContent(messageID string) (string, error) {
	token := config.QQAdapter.NapcatWsToken
	baseURL := getNapcatHTTPBaseURL()
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

func getForwardMessageContent(forwardID string) (string, error) {
	token := config.QQAdapter.NapcatWsToken
	baseURL := getNapcatHTTPBaseURL()
	url := baseURL + "/get_forward_msg"

	body, err := json.Marshal(map[string]interface{}{
		"message_id": forwardID,
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
		return "", fmt.Errorf("get_forward_msg 返回错误: %s", response.Wording)
	}

	if response.Data == nil {
		return "", fmt.Errorf("get_forward_msg 返回空数据")
	}

	var forwardResp ForwardMessageResponse
	if err := json.Unmarshal(response.Data, &forwardResp); err != nil {
		return "", err
	}

	var content string
	for _, msg := range forwardResp.Messages {
		senderName := getUserName(msg.GroupID, msg.UserID)
		for _, seg := range msg.Message {
			if seg.Type == "text" {
				var textData TextData
				if err := json.Unmarshal(seg.Data, &textData); err == nil {
					content += senderName + ": " + textData.Text + "\n"
				}
			}
		}
	}

	return content, nil
}

func sendToLunarCore(messages []OpenAIMessage) error {
	url := config.QQAdapter.LunarCoreUrl + "/write/message"

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

func sendGroupTextMessage(groupID int64, content string) error {
	baseURL := getNapcatHTTPBaseURL()
	url := baseURL + "/send_group_msg"
	token := config.QQAdapter.NapcatWsToken

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

func sendGroupImageMessage(groupID int64, images []string) error {
	baseURL := getNapcatHTTPBaseURL()
	url := baseURL + "/send_group_msg"
	token := config.QQAdapter.NapcatWsToken

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

func isInListenGroup(groupID int64) bool {
	for _, id := range config.QQAdapter.ListenGroupIds {
		if id == groupID {
			return true
		}
	}
	return false
}

func getRandomGroupID() int64 {
	if len(config.QQAdapter.ListenGroupIds) == 0 {
		return 0
	}
	rand.Seed(time.Now().UnixNano())
	return config.QQAdapter.ListenGroupIds[rand.Intn(len(config.QQAdapter.ListenGroupIds))]
}

func containsTriggerKeyword(message string) bool {
	if len(config.QQAdapter.TriggerKeywords) == 0 {
		return true
	}
	for _, keyword := range config.QQAdapter.TriggerKeywords {
		if strings.Contains(message, keyword) {
			return true
		}
	}
	return false
}
