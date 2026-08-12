package handlers

import (
	"LunarAstral/adapters"
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

// LTPXLoadRequest 加载工具请求
type LTPXLoadRequest struct {
	Name       string `json:"name"`
	Definition string `json:"tool_definition"` // 工具定义 JSON
	JS         string `json:"tool_js"`         // 工具实现 JS 代码
}

// LTPXUnloadRequest 卸载工具请求
type LTPXUnloadRequest struct {
	Name string `json:"name"`
}

// LTPXResponse 通用响应
type LTPXResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
	Data    any    `json:"data,omitempty"`
}

// MessageBatchRequest 消息批量写入请求
type MessageBatchRequest struct {
	Messages []adapters.PostMessage `json:"messages"`
}

// VideoUrlBatchRequest 视频URL批量写入请求
type VideoUrlBatchRequest struct {
	Urls []string `json:"urls"`
}

// BatchResponse 批量操作响应
type BatchResponse struct {
	Success bool `json:"success"`
	Length  int  `json:"length"`
}

// AgentPositionRequest 智能体位置更新请求
type AgentPositionRequest struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	Z float64 `json:"z"`
}

// AgentEventRequest 智能体引擎事件请求
type AgentEventRequest struct {
	Event string `json:"event"` // 事件类型: movement_complete, action_started
	Data  string `json:"data"`  // 事件数据 JSON 字符串
}
