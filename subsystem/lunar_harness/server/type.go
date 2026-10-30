package server

import (
	"encoding/json"
	"net/http"
)

// StaticRoots 两个桌面前端的静态资源根
type StaticRoots struct {
	// Lunar 月华消息终端（挂载于 /）
	Lunar http.FileSystem
	// Crystal 琉璃桌面（挂载于 /desktop/）
	Crystal http.FileSystem
}

// Ready 两个前端的静态资源是否均已就绪
func (r StaticRoots) Ready() bool {
	return r.Lunar != nil && r.Crystal != nil
}

// EngineResponse /write/engine 响应
type EngineResponse struct {
	// Success 是否受理成功
	Success bool `json:"success"`
	// Length 透传的原始字节长度
	Length int `json:"length"`
	// Observed 被反射观测到的消息类型列表
	Observed []string `json:"observed"`
	// Broadcast 本次广播触达的在线客户端数
	Broadcast int `json:"broadcast"`
}

// WindowFocusRequest /window/focus 请求体
type WindowFocusRequest struct {
	// ID 目标桌面窗口标识（lunar / crystal）
	ID string `json:"id"`
}

// WindowFocusResponse /window/focus 响应体
type WindowFocusResponse struct {
	// Success 是否成功
	Success bool `json:"success"`
	// Message 结果说明
	Message string `json:"message"`
}

// SystemEndpoint 单条系统端点定义（统一路由注册表的元素）。
// Handler 与 Path 由 server 装配层消费，Method/Description 供 /api-docs 与自检使用。
type SystemEndpoint struct {
	// Path 端点路径（ServeMux 模式，末尾带 / 表示子树匹配）
	Path string `json:"path"`
	// Handler 处理函数
	Handler http.HandlerFunc `json:"handler"`
	// Method 允许的方法（"GET"/"POST"/"ANY" 等，仅文档与自检用途）
	Method string `json:"method"`
	// Description 端点说明（供自述文档展示）
	Description string `json:"description"`
}

// AgentChatMessage /agent 非流式补全的助手消息
type AgentChatMessage struct {
	Role             string          `json:"role"`
	Content          string          `json:"content"`
	ReasoningContent string          `json:"reasoning_content,omitempty"`
	ToolCalls        json.RawMessage `json:"tool_calls,omitempty"`
}

// AgentChatChoice /agent 非流式补全的单条选项
type AgentChatChoice struct {
	Index        int              `json:"index"`
	Message      AgentChatMessage `json:"message"`
	FinishReason string           `json:"finish_reason"`
}

// AgentChatResponse /agent 端点响应（非流式补全或错误）
type AgentChatResponse struct {
	Object  string            `json:"object"`
	Model   string            `json:"model,omitempty"`
	Choices []AgentChatChoice `json:"choices,omitempty"`
	Error   *AgentChatError   `json:"error,omitempty"`
}

// AgentChatError /agent 端点错误主体
type AgentChatError struct {
	Message string `json:"message"`
}
