package Responder

// loop.go 循环输出检测：非流式响应拿不到增量，改为在整段文本上识别
// 「同一段内容连续重复」——把文本按空行切成段落，若某一段原文重复出现
// 达到阈值（默认 4 次）且占据大段篇幅，判定为复读。

import (
	"encoding/json"
	"errors"
	"strings"
)

// errLoopOutput 推理循环：同一段落在回复里反复出现。
var errLoopOutput = errors.New("检测到循环输出")

const (
	// loopRepeatLimit 同一段落允许出现的最大次数
	loopRepeatLimit = 4
	// loopMinRunes 触发判定所需的最小段落长度
	loopMinRunes = 24
	// loopMinTextRunes 文本过短时不做判定
	loopMinTextRunes = 240
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

// loopDetected 判断回复是否卡在复读。
func loopDetected(text string) bool {
	text = strings.TrimSpace(text)
	if len([]rune(text)) < loopMinTextRunes {
		return false
	}
	counts := map[string]int{}
	for _, paragraph := range strings.Split(text, "\n\n") {
		key := strings.Join(strings.Fields(paragraph), " ")
		if len([]rune(key)) < loopMinRunes {
			continue
		}
		counts[key]++
		if counts[key] >= loopRepeatLimit {
			return true
		}
	}
	return false
}
