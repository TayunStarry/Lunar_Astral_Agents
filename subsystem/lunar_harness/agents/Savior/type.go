package Savior

// type.go 拯救者智能体的公共类型定义。

import "time"

// Failure 一条待修复故障
type Failure struct {
	ID       int64
	Agent    string
	Error    string
	At       time.Time
	Attempts int
}

// chatMessage 会话消息
type chatMessage struct {
	Role      string     `json:"role"`
	Content   string     `json:"content"`
	ToolCalls []toolCall `json:"tool_calls,omitempty"`
	ToolID    string     `json:"tool_call_id,omitempty"`
}

// toolCall 模型发起的工具调用
type toolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// toolDefinition 工具声明
type toolDefinition struct {
	Type     string `json:"type"`
	Function struct {
		Name        string         `json:"name"`
		Description string         `json:"description"`
		Parameters  map[string]any `json:"parameters"`
	} `json:"function"`
}

// modelResponse 一轮模型响应
type modelResponse struct {
	Content   string
	Reasoning string
	ToolCalls []toolCall
}

// toolOutcome 工具执行结果
type toolOutcome struct {
	JSON    string
	Summary string
	Done    bool
}
