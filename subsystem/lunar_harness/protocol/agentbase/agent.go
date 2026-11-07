package agentbase

// agent.go 统一智能体基座：流式调用、实况登记、循环监管、重试与智能修复。

import (
	"context"
	"time"
)

// NewAgent 创建标准智能体基座。
func NewAgent(cfg AgentConfig) *Agent {
	if cfg.Timeout <= 0 {
		cfg.Timeout = defaultStreamTimeout
	}
	if cfg.MaxRounds <= 0 {
		cfg.MaxRounds = defaultMaxRounds
	}
	if cfg.MaxToolResultRunes == 0 {
		cfg.MaxToolResultRunes = defaultMaxToolResultRunes
	}
	return &Agent{Config: cfg}
}

// Stream 以基座默认角色/超时发起一次模型调用。
func (a *Agent) Stream(ctx context.Context, req StreamRequest) (Reply, error) {
	if req.Role == "" {
		req.Role = a.Config.Label
	}
	if req.Agent == "" {
		req.Agent = a.Config.ID
	}
	if req.Timeout <= 0 {
		req.Timeout = a.Config.Timeout
	}
	return Stream(ctx, req)
}

// StreamWithRetry 以基座默认角色/超时发起带重试的模型调用。
func (a *Agent) StreamWithRetry(ctx context.Context, req StreamRequest, opts RetryOptions) (Reply, error) {
	if req.Role == "" {
		req.Role = a.Config.Label
	}
	if req.Agent == "" {
		req.Agent = a.Config.ID
	}
	if req.Timeout <= 0 {
		req.Timeout = a.Config.Timeout
	}
	return StreamWithRetry(ctx, req, opts)
}

// Report 上报一次故障（由装配层注入的智能修复回调处理）。
func (a *Agent) Report(err error) {
	Report(a.Config.ID, err)
}

// ReportRun 上报一次运行失败（忽略主动取消与循环截断）。
func (a *Agent) ReportRun(err error) {
	ReportRun(a.Config.ID, err)
}

// TruncateToolResult 按基座默认上限截断工具结果。
func (a *Agent) TruncateToolResult(text string) string {
	return TruncateRunes(text, a.Config.MaxToolResultRunes)
}

// Timeout 返回基座默认超时（未配置时回退统一默认值）。
func (a *Agent) Timeout() time.Duration {
	if a.Config.Timeout > 0 {
		return a.Config.Timeout
	}
	return defaultStreamTimeout
}

// MaxRounds 返回基座默认工具循环上限。
func (a *Agent) MaxRounds() int {
	if a.Config.MaxRounds > 0 {
		return a.Config.MaxRounds
	}
	return defaultMaxRounds
}
