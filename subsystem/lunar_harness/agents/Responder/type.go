package Responder

// type.go 响应者智能体的公共类型定义。
//
// 响应者只做一件事：把外部（前端 / 其他进程）提交的多模态推理请求
// 排队，在后勤系统的 Tick 里依次交给 llama.cpp 完成非流式推理。
// 它没有工具链、没有系统提示词，请求体原样透传（模型名与流式开关除外）。

import (
	"context"
	"encoding/json"
)

// Request 一条外部推理请求
type Request struct {
	// Messages 完整消息数组（原样透传给模型）
	Messages json.RawMessage
	// Params 其余请求参数（temperature / top_p / max_tokens 等），原样透传
	Params map[string]json.RawMessage
	// Reply 结果回执（有且仅有一次写入）
	Reply chan Result
}

// Result 推理结果
type Result struct {
	Content   string
	Reasoning string
	// ToolCalls 模型要求的工具调用（原样保留，外部智能体靠它继续跑工具循环）
	ToolCalls json.RawMessage
	// FinishReason 结束原因（stop / tool_calls / length）
	FinishReason string
	Err          error
}

// chatMessage 模型返回的消息体
type chatMessage struct {
	Role             string          `json:"role"`
	Content          string          `json:"content"`
	ReasoningContent string          `json:"reasoning_content"`
	ToolCalls        json.RawMessage `json:"tool_calls"`
}

// chatChoice 模型返回的选项
type chatChoice struct {
	Message      chatMessage `json:"message"`
	FinishReason string      `json:"finish_reason"`
}

// chatCompletion 非流式补全响应
type chatCompletion struct {
	Choices []chatChoice `json:"choices"`
	Error   *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// drainContext 一轮处理所需的上下文与统计
type drainContext struct {
	ctx      context.Context
	handled  int
	failed   int
	duration int64
}
