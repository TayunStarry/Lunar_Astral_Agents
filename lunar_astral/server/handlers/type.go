package handlers

import "encoding/json"

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
