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
	// MaxRepeats 同一响应签名允许重复出现的次数上限
	MaxRepeats int
	// MaxToolResultRunes 工具结果截断上限（负数表示不截断）
	MaxToolResultRunes int
	// Call 单轮模型调用（由链侧组装上下文）
	Call func(ctx context.Context, round int) (agentbase.Reply, error)
	// Execute 执行一次工具调用
	Execute func(ctx context.Context, call agentbase.ToolCall) (string, bool, error)
	// OnLoop 循环重复被中断时回调（参数为已执行轮次）
	OnLoop func(rounds int, err error)
	// OnFailure 其它失败时回调
	OnFailure func(err error)
}
