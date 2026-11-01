package Responder

// loop.go 空回答判定与辅助函数。
//
// 循环输出检测统一由 engine/model/loopguard 承担（流式按节点实时截断、
// 整包按末尾节点静态判定），本文件不再自行实现段落计数。

import (
	"encoding/json"
	"errors"
	"strings"
)

// hasToolCalls 判断响应是否携带工具调用。
func hasToolCalls(raw json.RawMessage) bool {
	text := strings.TrimSpace(string(raw))
	return text != "" && text != "null" && text != "[]"
}

// errEmptyOutput 空回答：无正文、无思考、无工具调用（或仅因长度截断）。
var errEmptyOutput = errors.New("空回答")

// finishLabel 结束原因的可读描述（无值时标注未回传）。
func finishLabel(result Result) string {
	if result.FinishReason == "" {
		return "finish_reason 未回传"
	}
	return "finish_reason=" + result.FinishReason
}

// emptyTurn 判断本次是否没有拿到有效回合。
// 只有「整条回答长度到顶且没有工具调用」才按空回答处理；
// 有正文的正常短回答（例如一句应答）不重试。
func emptyTurn(result Result) bool {
	if hasToolCalls(result.ToolCalls) {
		return false
	}
	if result.Content == "" && result.Reasoning == "" {
		return true
	}
	return result.FinishReason == "length"
}
