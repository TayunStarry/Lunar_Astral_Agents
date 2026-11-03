package Responder

// model.go 响应者模型调用：统一走 protocol/agentbase 的流式调用、
// 推理实况、循环监管与智能修复；外部请求参数原样并入 Extra。

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/protocol/agentbase"
)

// retryable 判定是否值得重试：循环输出、网络错误或 5xx。
func retryable(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, agentbase.ErrLoop) {
		return true
	}
	var statusErr *agentbase.StatusError
	if errors.As(err, &statusErr) {
		return statusErr.Status >= 500
	}
	return true
}

// completeOnce 发起一次流式补全，边收边推送实况，最后拼成完整结果。
func completeOnce(ctx context.Context, req *Request) (Result, error) {
	extra := make(map[string]any, len(req.Params))
	for key, raw := range req.Params {
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "model", "stream", "stream_options", "messages":
			continue
		}
		var value any
		if err := json.Unmarshal(raw, &value); err != nil {
			continue
		}
		extra[key] = value
	}
	reply, err := agentbase.Stream(ctx, agentbase.StreamRequest{
		Agent:       "responder",
		Role:        roleLabel,
		Messages:    req.Messages,
		Extra:       extra,
		Timeout:     requestTimeout,
		ReportTools: true,
	})
	if err != nil {
		return Result{}, err
	}
	var rawCalls json.RawMessage
	if len(reply.ToolCalls) > 0 {
		rawCalls, _ = json.Marshal(reply.ToolCalls)
	}
	return Result{
		Content:      reply.Text,
		Reasoning:    reply.Reasoning,
		ToolCalls:    rawCalls,
		FinishReason: reply.FinishReason,
	}, nil
}

// completeWithRetry 首次 + 三次重试；命中循环输出同样重试。
// 重试耗尽后的失败上报给智能修复（循环截断与主动取消不重复上报）。
func completeWithRetry(ctx context.Context, req *Request) (out Result, err error) {
	defer func() { agentbase.ReportRun("responder", err) }()
	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if ctx.Err() != nil {
			return Result{}, ctx.Err()
		}
		result, err := completeOnce(ctx, req)
		if err == nil {
			// 带工具调用的响应正文往往只有一句引子，不做复读判定
			if hasToolCalls(result.ToolCalls) {
				logResult(result, attempt)
				return result, nil
			}
			// 空回答（无正文、无思考、无工具调用）或长度截断：重试一次拿到完整回合
			if emptyTurn(result) {
				lastErr = fmt.Errorf("%w（第 %d 次尝试：%s）", errEmptyOutput, attempt+1, finishLabel(result))
				continue
			}
			logResult(result, attempt)
			return result, nil
		}
		lastErr = err
		if !retryable(err) {
			return Result{}, err
		}
		select {
		case <-ctx.Done():
			return Result{}, ctx.Err()
		case <-time.After(time.Duration(attempt+1) * 300 * time.Millisecond):
		}
	}
	return Result{}, lastErr
}

// nowMillis 当前毫秒时间戳。
func nowMillis() int64 {
	return time.Now().UnixMilli()
}
