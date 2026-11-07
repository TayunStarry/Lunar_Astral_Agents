package Dialogue

// type.go 对话者的调用与循环钩子类型。

import (
	"context"
	"time"

	"LunarSubsystem/LunarHarness/protocol/agentbase"
)

// Request 一次对话者模型请求。
type Request struct {
	// Role 推理实况角色标签
	Role string
	// Messages 完整消息数组（含 system 段）
	Messages any
	// Tools 工具声明（由链侧注册表提供）
	Tools []agentbase.ToolDefinition
	// Extra 采样参数附加字段
	Extra map[string]any
	// Headers 附加请求头
	Headers map[string]string
	// InsecureTLS 是否跳过证书校验
	InsecureTLS bool
	// Timeout 单次请求超时
	Timeout time.Duration
	// ReportTools 是否上报工具调用（推理实况）
	ReportTools bool
}

// LoopHook 对话者工具循环的钩子。
type LoopHook struct {
	// Role 推理实况角色标签
	Role string
	// MaxRounds 允许的工具调用轮次上限（含收尾轮）
	MaxRounds int
	// MaxToolRounds 允许实际执行工具的轮次上限；<=0 时取 MaxRounds（不额外限制）。
	// 轮次上限的判定只在此处（RunToolLoop）生效，调用方不再自行计数。
	MaxToolRounds int
	// MaxToolResultRunes 工具结果截断上限（负数表示不截断）
	MaxToolResultRunes int
	// Call 单轮模型调用（由链侧组装上下文）
	Call func(ctx context.Context, round int) (agentbase.Reply, error)
	// OnToolRoundsExhausted 工具轮次用尽时的收尾回调：
	// 本轮工具调用不再执行，由调用方回填未执行原因并返回不带工具调用的最终回复。
	OnToolRoundsExhausted func(ctx context.Context, round int, reply agentbase.Reply) (agentbase.Reply, error)
	// Execute 执行一次工具调用
	Execute func(ctx context.Context, call agentbase.ToolCall) (string, bool, error)
	// OnLoop 流式输出循环被中断时回调（参数为已执行轮次）
	OnLoop func(rounds int, err error)
	// OnFailure 其它失败时回调
	OnFailure func(err error)
}
