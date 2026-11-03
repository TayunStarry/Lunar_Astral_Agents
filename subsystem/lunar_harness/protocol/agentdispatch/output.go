package agentdispatch

// output.go 统一输出的构造与序列化。
//
// 序列化结果固定包含 result / handle / error 三个字段：
// handle 永远是非 nil 数组，error 无错误时为空字符串，不追加任何额外字段。

import (
	"encoding/json"
	"strings"
)

// Success 构造成功结果。
func Success(result string, handles []string) Result {
	return Result{Result: result, Handle: normalizeHandles(handles)}
}

// Failure 构造失败结果。
func Failure(message string, handles []string) Result {
	return Result{Handle: normalizeHandles(handles), Error: message}
}

// PartialFailure 构造带已完成部分的失败结果。
func PartialFailure(message, partial string, handles []string) Result {
	return Result{Result: partial, Handle: normalizeHandles(handles), Error: message}
}

// JSON 序列化为规范输出字符串。
func (r Result) JSON() string {
	normalized := Result{Result: r.Result, Handle: normalizeHandles(r.Handle), Error: r.Error}
	raw, err := json.Marshal(normalized)
	if err != nil {
		return `{"result":"","handle":[],"error":"结果序列化失败"}`
	}
	return string(raw)
}

// Truncate 在 result 字段内部截断过长文本，保证序列化后仍是合法 JSON。
func (r Result) Truncate(limit int) Result {
	if limit <= 0 {
		return r
	}
	runes := []rune(r.Result)
	if len(runes) <= limit {
		return r
	}
	return Result{
		Result: string(runes[:limit]) + "…（结果过长，已截断）",
		Handle: normalizeHandles(r.Handle),
		Error:  r.Error,
	}
}

// normalizeHandles 保证句柄列表为非 nil 数组，并去掉空白项。
func normalizeHandles(handles []string) []string {
	normalized := make([]string, 0, len(handles))
	for _, handle := range handles {
		if text := strings.TrimSpace(handle); text != "" {
			normalized = append(normalized, text)
		}
	}
	return normalized
}
