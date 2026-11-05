package napcat

// client.go 传输客户端构造、默认实例注入与 WebSocket 连接。

import (
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gorilla/websocket"
)

// New 构造传输客户端。
func New(cfg Config) *Client {
	return &Client{
		endpoint:         cfg.Endpoint,
		token:            cfg.Token,
		httpClient:       &http.Client{Timeout: defaultHTTPTimeout},
		uploadHTTPClient: &http.Client{Timeout: uploadHTTPTimeout},
		groupNameCache:   make(map[int64]string),
		memberNameCache:  make(map[int64]map[int64]string),
		memberListWarmed: make(map[int64]bool),
	}
}

// SetDefault 注入默认客户端（由装配层在加载配置后调用）。
func SetDefault(client *Client) {
	if client == nil {
		client = New(Config{})
	}
	defaultMu.Lock()
	defaultClient = client
	defaultMu.Unlock()
}

// Default 返回当前默认客户端。
func Default() *Client {
	defaultMu.RLock()
	defer defaultMu.RUnlock()
	return defaultClient
}

// Connect 连接到 NapCat WebSocket 服务器并持续读取消息。
// 连接建立/断开时分别回调 onConnected / onDisconnected，由调用方维护上层状态。
func Connect(messageHandler func([]byte), onConnected func(), onDisconnected func()) error {
	return Default().connect(messageHandler, onConnected, onDisconnected)
}

func (c *Client) connect(messageHandler func([]byte), onConnected func(), onDisconnected func()) error {
	url := c.endpoint
	token := c.token

	logger.SubInfo("LunarCore", "Napcat", "正在连接: %s", url)

	headers := http.Header{}
	if token != "" {
		headers.Set("Authorization", "Bearer "+token)
	}

	conn, _, err := websocket.DefaultDialer.Dial(url, headers)
	if err != nil {
		return fmt.Errorf("连接失败: %v", err)
	}
	defer conn.Close()

	if onConnected != nil {
		onConnected()
	}
	logger.SubInfo("LunarCore", "Napcat", "成功连接到 napcat 服务器")
	// 预取登录账号与群/好友目录：让 network_explore 的 qq_send 尽快可用，
	// 避免第一轮对话在构建工具列表时同步等待这几次接口。
	go func() {
		_, _ = c.resolveBotInfo()
		_, _, _ = c.directory()
	}()

	// 消息解析含 API 调用（回复引用、合并转发、成员名解析）可能阻塞，
	// 用单协程顺序消费与 WS 读取循环解耦：保证消息顺序的同时不拖慢读取
	msgCh := make(chan []byte, 64)
	done := make(chan struct{})
	defer close(done)
	go func() {
		for {
			select {
			case msg := <-msgCh:
				messageHandler(msg)
			case <-done:
				return
			}
		}
	}()

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			if onDisconnected != nil {
				onDisconnected()
			}
			return fmt.Errorf("读取消息失败: %v", err)
		}
		select {
		case msgCh <- message:
		case <-done:
			return fmt.Errorf("读取消息失败: 连接已关闭")
		}
	}
}

// httpBaseURL 将 napcat ws 地址转换为 http 地址
func (c *Client) httpBaseURL() string {
	return strings.Replace(c.endpoint, "ws://", "http://", 1)
}

// callAPI 统一调用 Napcat HTTP API，返回完整响应
func (c *Client) callAPI(action string, params map[string]any) (*NapcatWSResponse, error) {
	return c.callAPIWithClient(action, params, c.httpClient)
}

// callAPIWithClient 指定客户端的 API 调用（文件上传需要更长的超时）
func (c *Client) callAPIWithClient(action string, params map[string]any, client *http.Client) (*NapcatWSResponse, error) {
	url := c.httpBaseURL() + action
	token := c.token

	body, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var response NapcatWSResponse
	if err := json.Unmarshal(respBody, &response); err != nil {
		return nil, err
	}
	if response.Status != "ok" {
		return nil, fmt.Errorf("%s 返回错误: %s", action, response.Wording)
	}
	return &response, nil
}
