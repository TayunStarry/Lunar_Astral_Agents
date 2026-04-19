package utils

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

// NewClient 创建新的WebSocket客户端
func NewClient(serverURL, token string) *Client {
	return &Client{
		serverURL: serverURL,
		token:     token,
	}
}

// Connect 连接到WebSocket服务器
func (class *Client) Connect() error {
	log.Printf("正在连接到 WebSocket 服务器: %s", class.serverURL)
	// 设置HTTP头
	headers := http.Header{}
	headers.Add("Authorization", fmt.Sprintf("Bearer %s", class.token))
	headers.Add("Token", class.token)

	// 连接到服务器
	conn, _, err := websocket.DefaultDialer.Dial(class.serverURL, headers)
	if err != nil {
		return fmt.Errorf("连接失败: %v", err)
	}

	class.conn = conn
	log.Println("成功连接到 WebSocket 服务器")
	return nil
}

// SendMessage 发送消息
func (class *Client) SendMessage(action string, params any) (string, error) {
	if class.conn == nil {
		return "", fmt.Errorf("WebSocket连接未建立")
	}

	// 创建请求
	echo := fmt.Sprintf("%s_%s", action, time.Now().Format("20060102150405"))
	request := WSAPIRequest{
		Action: action,
		Params: params,
		Echo:   echo,
	}

	// 序列化请求
	requestJSON, err := json.Marshal(request)
	if err != nil {
		return "", fmt.Errorf("序列化请求失败: %v", err)
	}

	if err := class.conn.WriteMessage(websocket.TextMessage, requestJSON); err != nil {
		return "", fmt.Errorf("发送消息失败: %v", err)
	}

	return echo, nil
}

// GetGroupMessageHistory 获取群消息历史
func (class *Client) GetGroupMessageHistory(groupID int64) (string, error) {
	params := map[string]any{
		"group_id":     groupID,
		"message_seq":  0,
		"count":        10,
		"reverseOrder": false,
	}
	return class.SendMessage("get_group_msg_history", params)
}

// ReadMessage 读取消息
func (class *Client) ReadMessage() ([]byte, error) {
	if class.conn == nil {
		return nil, fmt.Errorf("WebSocket连接未建立")
	}

	// 设置读取超时
	class.conn.SetReadDeadline(time.Now().Add(60 * time.Second))

	_, message, err := class.conn.ReadMessage()
	if err != nil {
		return nil, fmt.Errorf("读取消息失败: %v", err)
	}

	return message, nil
}
