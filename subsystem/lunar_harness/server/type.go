package server

import (
	"encoding/json"
	"net/http"
)

// StaticRoots 前端静态资源根（各前端 + 共享依赖）
type StaticRoots struct {
	// Lunar 月华消息终端（挂载于 /）
	Lunar http.FileSystem
	// Mobile 月华移动端（挂载于 /mobile/）
	Mobile http.FileSystem
	// Crystal 琉璃桌面（挂载于 /desktop/）
	Crystal http.FileSystem
	// Library 资源库（挂载于 /library/）
	Library http.FileSystem
	// Reliance 前端共享依赖（挂载于 /reliance/）
	Reliance http.FileSystem
	// Shared 桌面端与移动端共用 ES Module 共享层（挂载于 /shared/）
	Shared http.FileSystem
}

// Ready 月华消息终端与琉璃桌面的静态资源是否均已就绪
func (r StaticRoots) Ready() bool {
	return r.Lunar != nil && r.Crystal != nil
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

// MediaPlatformHostsResponse 共享媒体平台域名表响应（前端抽取 webpage 信封时使用）
type MediaPlatformHostsResponse struct {
	// VideoPlatformHosts 共享视频平台域名（与 engine/mediaobject.VideoPlatformHosts 同源）
	VideoPlatformHosts []string `json:"video_platform_hosts"`
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

// AgentRequest 外部推理排队请求（server 只声明结构，入队实现由装配层注入）
type AgentRequest struct {
	// Messages 完整消息数组（原样透传给模型）
	Messages json.RawMessage
	// Params 其余请求参数（temperature / top_p / max_tokens 等）
	Params map[string]json.RawMessage
	// Reply 结果回执（有且仅有一次写入）
	Reply chan AgentResult
}

// AgentResult 外部推理结果
type AgentResult struct {
	Content      string
	Reasoning    string
	ToolCalls    json.RawMessage
	FinishReason string
	Err          error
}

// DataDumpKind 数据导出类型
type DataDumpKind string

// DataDumpRequest 一次数据导出请求（TUI 与 HTTP handler 共用同一服务入口）
type DataDumpRequest struct {
	// Kind 导出类型
	Kind DataDumpKind
	// Agent 智能体上下文导出目标（仅 DataDumpContext 使用）
	Agent string
	// IncludeImages 是否保留图片原始 base64（仅 DataDumpContext 使用）
	IncludeImages bool
}
