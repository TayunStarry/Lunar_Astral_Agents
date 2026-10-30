package Creator

// context_budget.go 工具循环的上下文预算：单条工具结果截断 + 整体超预算时收紧较早的结果。
//
// 创作链一次任务会连续调用多个工具（读文件、检索、图片理解、写入），每轮都把完整结果
// 追加进 messages，很容易把单次请求推到模型上下文上限（llama.cpp 会直接 400）。
// 这里做两层保护：单条结果不超过 toolMessageMaxRunes；全部工具结果累计超过
// toolBudgetRunes 时，从最旧的一条开始压缩，最近两条保持原样。

import (
	"fmt"
	"strings"
)

// isContextOverflow 判断模型报错是否为上下文超限（llama.cpp 返回 HTTP 400 + "exceeds"）。
func isContextOverflow(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	for _, keyword := range []string{"exceeds", "context", "too long", "too many tokens"} {
		if strings.Contains(text, keyword) {
			return true
		}
	}
	return false
}

// shrinkMessages 上下文超限时的兜底收缩：保留系统提示与前两条（系统 + 任务），
// 其余历史压到极短，只保留最近两条工具结果的头部。
func shrinkMessages(messages []chatMessage) {
	keepHead := 2
	if len(messages) < keepHead {
		return
	}
	for index := keepHead; index < len(messages); index++ {
		current := len([]rune(messages[index].Content))
		if current <= toolPrunedRunes {
			continue
		}
		messages[index].Content = previewRunes(messages[index].Content, toolPrunedRunes) + "…（上下文超限，已压缩）"
	}
}

// toolResultContent 把工具结果回填为单条消息正文，超出单条上限的部分截断。
func toolResultContent(outcome toolOutcome) string {
	text := outcome.JSON
	if text == "" {
		text = "{}"
	}
	if len([]rune(text)) <= toolMessageMaxRunes {
		return text
	}
	head := previewRunes(text, toolMessageMaxRunes)
	return head + fmt.Sprintf("…（结果过长，已截断，剩余约 %d 字；需要更多内容请用 offset 分批读取）",
		len([]rune(text))-toolMessageMaxRunes)
}

// compactToolResults 在累计工具结果超出预算时收紧较早的结果。
func compactToolResults(messages []chatMessage) {
	total := 0
	indexes := make([]int, 0, len(messages))
	for index, message := range messages {
		total += len([]rune(message.Content))
		if message.Role == "tool" {
			indexes = append(indexes, index)
		}
	}
	if total <= toolBudgetRunes {
		return
	}
	// 最近两条工具结果通常是当前步骤的依据，保持完整
	prunable := len(indexes) - 2
	for cursor := 0; cursor < prunable && total > toolBudgetRunes; cursor++ {
		position := indexes[cursor]
		current := len([]rune(messages[position].Content))
		if current <= toolPrunedRunes {
			continue
		}
		short := previewRunes(messages[position].Content, toolPrunedRunes) + "…（较早的工具结果已压缩）"
		messages[position].Content = short
		total -= current - len([]rune(short))
	}
}
