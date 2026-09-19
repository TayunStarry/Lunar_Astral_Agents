package handlers

import (
	"LunarAstral/engine"
	"encoding/json"
)

// ProxyRequest 定义代理请求的结构
type ProxyRequest struct {
	URL         string      `json:"url"`
	RequestInit RequestInit `json:"requestInit"`
}

// RequestInit 定义请求初始化参数的结构
type RequestInit struct {
	Method      string            `json:"method,omitempty"`      // 请求方法，如 GET、POST 等
	Headers     map[string]string `json:"headers,omitempty"`     // 请求头
	Body        any               `json:"body,omitempty"`        // 请求体
	Redirect    string            `json:"redirect,omitempty"`    // 重定向目标，可选
	Credentials string            `json:"credentials,omitempty"` // 认证信息，如用户名:密码
}

// ProxyResponse 代理响应结构
type ProxyResponse struct {
	Status     int               `json:"status"`     // 响应状态码
	StatusText string            `json:"statusText"` // 响应状态文本
	Headers    map[string]string `json:"headers"`    // 响应头
	Body       json.RawMessage   `json:"body"`       // 响应体
}

// MessageBatchRequest 消息批量写入请求（统一时序队列唯一写入口：
// content 为文本/多模态内容数组，视频/音频URL以 MediaUrlContent 内容项直接放入 messages）
type MessageBatchRequest struct {
	Messages []engine.PostMessage `json:"messages"`
}

// BatchResponse 批量操作响应
type BatchResponse struct {
	Success bool `json:"success"`
	Length  int  `json:"length"`
}

// EngineMessage 引擎系统消息（经 /write/engine 提交）
// 格式与 /write/message 同构：顶层带 type/source/payload/timestamp，
// 与对话消息（{messages:[...]}）区分，由服务端按 type 分发
type EngineMessage struct {
	Type      string          `json:"type"`   // 消息类型: animation_list / telemetry / ...
	Source    string          `json:"source"` // 消息来源: engine / base / *-panel
	Payload   json.RawMessage `json:"payload"`
	Timestamp int64           `json:"timestamp"`
}
