package handlers

import "encoding/json"

// ProxyRequest 定义代理请求的结构
type ProxyRequest struct {
	URL         string      `json:"url"`
	RequestInit RequestInit `json:"requestInit"`
}

// RequestInit 定义请求初始化参数的结构
type RequestInit struct {
	Method      string            `json:"method,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
	Body        any               `json:"body,omitempty"`
	Redirect    string            `json:"redirect,omitempty"`
	Credentials string            `json:"credentials,omitempty"`
}

// ProxyResponse 代理响应结构
type ProxyResponse struct {
	Status     int               `json:"status"`
	StatusText string            `json:"statusText"`
	Headers    map[string]string `json:"headers"`
	Body       json.RawMessage   `json:"body"`
}
