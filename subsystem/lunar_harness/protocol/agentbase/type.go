package agentbase

// type.go 共享智能体模型调用的类型定义（OpenAI v1 兼容）。
//
// 本包只承载「模型怎么调用」这一层基础设施：视觉理解与文献解析等智能体各自
// 持有自己的提示词、工具与处理管线，但不必再各写一份 SSE 解析与工具调用回填。

import (
	"fmt"
	"time"

	"LunarSubsystem/LunarHarness/protocol/loopguard"
)

// Message OpenAI v1 协议中的单条对话消息
type Message struct {
	// Role 角色（system/user/assistant/tool）
	Role string `json:"role"`
	// Content 内容（文本或图文片段数组）
	Content any `json:"content"`
	// ToolCalls 模型请求的工具调用（assistant 消息）
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
	// ToolCallID 工具结果对应的调用 ID（tool 消息）
	ToolCallID string `json:"tool_call_id,omitempty"`
}

// ContentPart 图文混排内容片段
type ContentPart struct {
	// Type 内容类型（text/image_url）
	Type string `json:"type"`
	// Text 文本内容（type=text）
	Text string `json:"text,omitempty"`
	// ImageURL 图片地址（type=image_url）
	ImageURL *ImageURL `json:"image_url,omitempty"`
}

// ImageURL 图片地址（data URI）
type ImageURL struct {
	// URL 图片地址
	URL string `json:"url"`
}

// ToolCall 模型返回的工具调用
type ToolCall struct {
	// ID 调用 ID
	ID string `json:"id"`
	// Type 调用类型（function）
	Type string `json:"type"`
	// Function 函数名与 JSON 参数串
	Function ToolFunction `json:"function"`
}

// ToolFunction 工具调用的函数体
type ToolFunction struct {
	// Name 函数名
	Name string `json:"name"`
	// Arguments JSON 参数串
	Arguments string `json:"arguments"`
}

// ToolDefinition 暴露给模型的工具定义
type ToolDefinition struct {
	// Type 工具类型（function）
	Type string `json:"type"`
	// Function 函数描述
	Function ToolFunctionDef `json:"function"`
}

// ToolFunctionDef 工具的函数描述体
type ToolFunctionDef struct {
	// Name 函数名
	Name string `json:"name"`
	// Description 函数说明
	Description string `json:"description"`
	// Parameters JSON Schema 参数结构
	Parameters map[string]any `json:"parameters"`
}

// ChatOptions 单次模型调用的可选项
type ChatOptions struct {
	// Model 覆盖模型名（空 = 主多模态模型）
	Model string
	// Temperature 采样温度（nil = 沿用服务端默认）
	Temperature *float64
	// MaxTokens 最大输出 token（0 = 不限制）
	MaxTokens int
	// HoldLease 调用方已持有模型切换租约（如轻量扫描通道），
	// 不再获取普通调用租约，避免写锁与读锁在同一协程上重入死锁。
	HoldLease bool
}

// Reply 一次模型调用的结果（助手消息 + 纯文本 + 思考文本 + 服务端用量）
type Reply struct {
	// Message 助手消息（可能携带工具调用）
	Message Message
	// Text 助手正文的纯文本（已去除首尾空白）
	Text string
	// Reasoning 思考全文（仅流式响应有值）
	Reasoning string
	// ToolCalls 本次响应中的工具调用（按 index 顺序）
	ToolCalls []ToolCall
	// Usage 服务端回传的 token 用量（未回传时为 nil）
	Usage *Usage
	// FinishReason 服务端给出的结束原因（可能为空）
	FinishReason string
}

// StreamRequest 统一模型调用请求：消息与工具允许传入各智能体自己的结构，
// 只要可 JSON 序列化即可，避免每个智能体再写一份 SSE / 实况 / 循环监管。
type StreamRequest struct {
	// Agent 故障归属 ID（交给智能修复；空时用 Role）
	Agent string
	// Role 实况角色标签（推理实况屏显示名）
	Role string
	// Messages 完整消息数组（任意可 JSON 序列化的结构）
	Messages any
	// Tools 工具定义数组（空或 nil 表示不启用工具）
	Tools any
	// Model 覆盖模型名（空 = 主多模态模型）
	Model string
	// Temperature 采样温度（nil = 服务端默认）
	Temperature *float64
	// TopP 核采样（nil = 服务端默认）
	TopP *float64
	// RepeatPenalty 重复惩罚（nil = 服务端默认）
	RepeatPenalty *float64
	// MaxTokens 最大输出 token（0 = 不限制）
	MaxTokens int
	// Extra 额外请求参数，原样并入请求体（不会覆盖 messages / tools / stream / model）
	Extra map[string]any
	// Headers 额外请求头（如 X-Lunar-Role）
	Headers map[string]string
	// NonStream 为 true 时强制非流式调用
	NonStream bool
	// Silent 为 true 时不登记推理实况（仍执行循环监管与计量）
	Silent bool
	// ManageLive 为 true 时由调用方自行管理实况起止（循环内多轮合并为一轮）
	ManageLive bool
	// Display 仅用于实况推送的正文变换（返回空串表示不推送这段）
	Display func(text string) string
	// DisplayAtEnd 为 true 时正文不逐段推送，流结束后统一规整再推送一次
	DisplayAtEnd bool
	// HoldLease 调用方已持有模型切换租约
	HoldLease bool
	// InsecureTLS 跳过 TLS 校验（本地自签代理等场景）
	InsecureTLS bool
	// Timeout 单次调用超时（0 = 默认）
	Timeout time.Duration
	// LoopOptions 循环输出检测参数（零值回退默认）
	LoopOptions loopguard.Options
	// ReportTools 为 true 时把工具调用同步登记到推理实况
	ReportTools bool
}

// StatusError 模型服务返回非 200 状态。
type StatusError struct {
	// Status HTTP 状态码
	Status int
	// Body 响应正文（截断后）
	Body string
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("模型 HTTP %d: %s", e.Status, TruncateRunes(e.Body, 240))
}

// RetryOptions 统一重试策略：循环输出 / 5xx / 网络错误可按需重试。
type RetryOptions struct {
	// MaxAttempts 总尝试次数（含首次；<1 视为 1）
	MaxAttempts int
	// RetryLoop 命中循环输出时重试
	RetryLoop bool
	// RetryStatus 命中 5xx 时重试
	RetryStatus bool
	// RetryNetwork 网络错误（非 StatusError）时重试
	RetryNetwork bool
	// Backoff 第 n 次失败后的等待（n 从 1 开始；nil 不等待）
	Backoff func(attempt int) time.Duration
	// OnRetry 每次将要重试时回调
	OnRetry func(attempt int, err error)
}

// AgentConfig 标准智能体基座的元数据。
type AgentConfig struct {
	// ID 故障归属 ID（智能修复按此去重）
	ID string
	// Label 实况角色标签
	Label string
	// Timeout 默认模型调用超时
	Timeout time.Duration
	// MaxRounds 工具循环默认上限
	MaxRounds int
	// MaxRepeats 同一工具调用签名重复上限（负值 = 关闭重复检测）
	MaxRepeats int
	// MaxToolResultRunes 单条工具结果回填上限（负值 = 关闭截断）
	MaxToolResultRunes int
}

// Agent 统一智能体基座：所有 Go 智能体共享的流式调用、实况登记、
// 循环监管、重试与智能修复上报入口。
type Agent struct {
	// Config 基座配置
	Config AgentConfig
}

// RepeatTracker 工具调用签名重复跟踪器。
type RepeatTracker struct {
	// limit 同一签名允许出现的次数上限
	limit int
	// seen 签名 → 出现次数
	seen map[string]int
}

// Usage OpenAI 兼容响应中的 token 用量
type Usage struct {
	// Prompt 上下文 token 数
	Prompt int64 `json:"prompt_tokens"`
	// Completion 输出 token 数
	Completion int64 `json:"completion_tokens"`
	// PromptTokensDetails 输入 token 的细分（提示缓存命中数）
	PromptTokensDetails *PromptTokenDetails `json:"prompt_tokens_details,omitempty"`
	// PromptCacheHitTokens 部分服务端使用的缓存命中字段
	PromptCacheHitTokens int64 `json:"prompt_cache_hit_tokens,omitempty"`
	// CachedTokens 部分服务端使用的缓存命中字段
	CachedTokens int64 `json:"cached_tokens,omitempty"`
}

// PromptTokenDetails 输入 token 的细分
type PromptTokenDetails struct {
	// CachedTokens 命中提示缓存的输入 token 数
	CachedTokens int64 `json:"cached_tokens"`
}

// chatResponse 非流式响应体
type chatResponse struct {
	// Choices 选择项
	Choices []struct {
		// Message 助手消息
		Message Message `json:"message"`
	} `json:"choices"`
	// Usage token 用量
	Usage *Usage `json:"usage"`
	// Error 错误信息
	Error *ChatError `json:"error"`
}

// ChatError 模型错误信息
type ChatError struct {
	// Message 错误文本
	Message string `json:"message"`
}

// streamChunk 流式响应分片
type streamChunk struct {
	// Choices 选择项
	Choices []struct {
		// Delta 增量
		Delta struct {
			// Content 正文增量
			Content string `json:"content"`
			// ReasoningContent 思考增量
			ReasoningContent string `json:"reasoning_content"`
			// Reasoning 部分服务端使用的思考增量字段
			Reasoning string `json:"reasoning"`
			// ToolCalls 工具调用增量
			ToolCalls []struct {
				// Index 调用序号
				Index int `json:"index"`
				// ID 调用 ID（首个分片携带）
				ID string `json:"id"`
				// Type 调用类型
				Type string `json:"type"`
				// Function 函数增量
				Function struct {
					// Name 函数名
					Name string `json:"name"`
					// Arguments 参数串增量
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
		// FinishReason 结束原因
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
	// Usage token 用量（include_usage 时随最后一个分片回传）
	Usage *Usage `json:"usage"`
	// Error 错误信息
	Error *ChatError `json:"error"`
}
