package Responder

// agent.go 响应者对外入口：入队、就绪判断、当前 Tick 内依次处理。

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"LunarSubsystem/LunarHarness/engine/console/logger"
)

// Enqueue 提交一条推理请求；队列已满时立即返回错误。
func Enqueue(req *Request) error {
	if req == nil || len(req.Messages) == 0 {
		return errors.New("messages 不能为空")
	}
	select {
	case inbox <- req:
		return nil
	default:
		return errors.New("响应者队列已满，请稍后再试")
	}
}

// Pending 返回当前排队的请求数。
func Pending() int {
	return len(inbox)
}

// Drain 依次处理当前队列里的每一条请求（每条只写一次回执）。
func Drain(ctx context.Context) {
	if Pending() == 0 {
		return
	}
	running.Store(true)
	defer running.Store(false)
	started := nowMillis()
	stats := drainContext{ctx: ctx}
	for {
		if ctx.Err() != nil {
			break
		}
		select {
		case req := <-inbox:
			if respondOne(&stats, req) {
				stats.handled++
			} else {
				stats.failed++
			}
			continue
		default:
		}
		break
	}
	stats.duration = nowMillis() - started
	if stats.handled+stats.failed > 0 {
		logger.Info(roleLabel, "[通用应答] 外部推理完成 %d 条，失败 %d 条，耗时 %dms", stats.handled, stats.failed, stats.duration)
	}
}

// respondOne 处理单条请求；返回是否成功。
func respondOne(stats *drainContext, req *Request) bool {
	result, err := completeWithRetry(stats.ctx, req)
	reply := result
	if err != nil {
		reply = Result{Err: fmt.Errorf("响应者推理失败: %w", err)}
	}
	sendReply(req, reply)
	if err != nil {
		logger.Warn(roleLabel, "[通用应答] 请求失败: %v", err)
		return false
	}
	preview := strings.TrimSpace(result.Content)
	if preview == "" {
		preview = strings.TrimSpace(result.Reasoning)
	}
	logger.Info(roleLabel, "[通用应答] 推理完成：%s", previewRunes(preview, 80))
	return true
}

// logResult 记录本次推理的关键信息，用于定位「模型提前停住」类问题。
func logResult(result Result, attempt int) {
	logger.Info(roleLabel, "[通用应答] 第 %d 次推理：%s，工具调用 %d 个，思考 %d 字，正文 %d 字",
		attempt+1, finishLabel(result), toolCallCount(result.ToolCalls),
		len([]rune(result.Reasoning)), len([]rune(result.Content)))
}

// toolCall 工具调用（仅取展示所需字段）
type toolCall struct {
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// toolCallCount 工具调用条数。
func toolCallCount(raw json.RawMessage) int {
	return len(parseToolCalls(raw))
}

// toolCallDigest 工具调用的可读摘要（面板展示用）。
func toolCallDigest(raw json.RawMessage) string {
	calls := parseToolCalls(raw)
	if len(calls) == 0 {
		return ""
	}
	lines := make([]string, 0, len(calls))
	for _, call := range calls {
		lines = append(lines, "→ 调用工具 "+call.Function.Name+"："+previewRunes(call.Function.Arguments, 100))
	}
	return strings.Join(lines, "\n")
}

// parseToolCalls 解析工具调用数组（格式异常时返回空）。
func parseToolCalls(raw json.RawMessage) []toolCall {
	if !hasToolCalls(raw) {
		return nil
	}
	var calls []toolCall
	if err := json.Unmarshal(raw, &calls); err != nil {
		return nil
	}
	return calls
}

// sendReply 写回执（不阻塞：调用方可能已超时离开）。
func sendReply(req *Request, result Result) {
	if req.Reply == nil {
		return
	}
	defer func() { _ = recover() }()
	select {
	case req.Reply <- result:
	default:
	}
}

// previewRunes 按字符截断。
func previewRunes(text string, limit int) string {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) <= limit {
		return string(runes)
	}
	return string(runes[:limit]) + "…"
}
