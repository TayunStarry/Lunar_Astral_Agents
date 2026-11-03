package agentbase

// repeat.go 工具调用签名与循环输出跟踪。
//
// 各智能体的工具循环不再各写一份 map[string]int 计数：统一用 RepeatTracker，
// 同一轮响应（正文 + 工具名 + 参数）重复达到上限即判定循环输出。

import (
	"fmt"
	"strings"
)

// NewRepeatTracker 创建重复跟踪器；limit <= 0 时使用统一默认上限。
func NewRepeatTracker(limit int) *RepeatTracker {
	if limit <= 0 {
		limit = defaultMaxRepeats
	}
	return &RepeatTracker{limit: limit, seen: map[string]int{}}
}

// Observe 记录一次响应签名；返回当前次数与是否达到上限。
func (t *RepeatTracker) Observe(signature string) (int, bool) {
	if t == nil {
		return 0, false
	}
	if t.seen == nil {
		t.seen = map[string]int{}
	}
	t.seen[signature]++
	count := t.seen[signature]
	return count, count >= t.limit
}

// Limit 返回当前判定上限。
func (t *RepeatTracker) Limit() int {
	if t == nil || t.limit <= 0 {
		return defaultMaxRepeats
	}
	return t.limit
}

// Signature 生成一轮响应的签名（正文 + 工具调用）。
func Signature(content string, calls []ToolCall) string {
	var builder strings.Builder
	builder.WriteString(content)
	builder.WriteByte('|')
	for _, call := range calls {
		builder.WriteString(call.Function.Name)
		builder.WriteByte(':')
		builder.WriteString(call.Function.Arguments)
		builder.WriteByte(';')
	}
	return builder.String()
}

// LoopError 构造带角色与轮次的循环输出错误。
func LoopError(role string, round int) error {
	if strings.TrimSpace(role) == "" {
		role = "智能体"
	}
	return fmt.Errorf("%w（%s 第 %d 轮出现重复的工具调用）", ErrLoop, role, round)
}
