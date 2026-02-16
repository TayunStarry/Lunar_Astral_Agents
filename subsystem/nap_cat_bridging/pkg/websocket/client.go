package websocket

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

// WSAPIRequest WebSocket API 请求结构体
type WSAPIRequest struct {
	// Action 操作
	Action string `json:"action"`
	// Params 参数
	Params any `json:"params"`
	// Echo 回显
	Echo string `json:"echo,omitempty"`
}

// WSResponse WebSocket 响应结构体
type WSResponse struct {
	// Status 状态
	Status string `json:"status"`
	// Retcode 返回码
	Retcode int `json:"retcode"`
	// Data 数据
	Data any `json:"data"`
	// Message 消息
	Message string `json:"message"`
	// Echo 回显
	Echo string `json:"echo"`
	// Wording 描述
	Wording string `json:"wording"`
	// Stream 流
	Stream string `json:"stream"`
}

// Client WebSocket客户端
type Client struct {
	// conn WebSocket连接
	conn *websocket.Conn
	// serverURL WebSocket服务器URL
	serverURL string
	// token 认证令牌
	token string
}

// NewClient 创建新的WebSocket客户端
func NewClient(serverURL, token string) *Client {
	return &Client{
		serverURL: serverURL,
		token:     token,
	}
}

// Connect 连接到WebSocket服务器
func (c *Client) Connect() error {
	log.Printf("正在连接到 WebSocket 服务器: %s", c.serverURL)

	// 设置HTTP头
	headers := http.Header{}
	headers.Add("Authorization", fmt.Sprintf("Bearer %s", c.token))
	headers.Add("Token", c.token)

	// 连接到服务器
	conn, _, err := websocket.DefaultDialer.Dial(c.serverURL, headers)
	if err != nil {
		return fmt.Errorf("连接失败: %v", err)
	}

	c.conn = conn
	log.Println("成功连接到 WebSocket 服务器")
	return nil
}

// Close 关闭WebSocket连接
func (c *Client) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// SendMessage 发送消息
func (c *Client) SendMessage(action string, params any) (string, error) {
	if c.conn == nil {
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

	if err := c.conn.WriteMessage(websocket.TextMessage, requestJSON); err != nil {
		return "", fmt.Errorf("发送消息失败: %v", err)
	}

	return echo, nil
}

// ReadMessage 读取消息
func (c *Client) ReadMessage() ([]byte, error) {
	if c.conn == nil {
		return nil, fmt.Errorf("WebSocket连接未建立")
	}

	_, message, err := c.conn.ReadMessage()
	if err != nil {
		return nil, fmt.Errorf("读取消息失败: %v", err)
	}

	return message, nil
}

// IsConnected 检查连接是否活跃
func (c *Client) IsConnected() bool {
	return c.conn != nil
}
