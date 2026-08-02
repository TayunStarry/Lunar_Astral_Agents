package learner

import (
	"encoding/json"
	"fmt"
	"strings"

	"logger"
)

// ============================================================
// 记忆更新
// ============================================================

// updateMemory 基于研究结果更新记忆库
func (d *DebateSystem) updateMemory(report string) {
	if !d.memory.IsAvailable() {
		logger.Warn("Learner", "记忆库不可用，跳过记忆更新")
		return
	}

	// 将研究报告摘要存入记忆
	summary := truncateText(report, 500)
	if _, err := d.memory.Add(summary); err != nil {
		logger.Warn("Learner", "研究报告存入记忆失败: %v", err)
	} else {
		logger.Info("Learner", "研究报告已存入记忆")
	}

	// 检查是否有需要更新的旧记忆（d.state 可能为 nil，如 Webpage 分支未走辩论流程）
	if d.state == nil || len(d.state.MemoryResults) == 0 {
		return
	}

	// 收集高相似度的记忆条目
	var highSimMatches []MemoryMatch
	for _, match := range d.state.MemoryResults {
		if match.Similarity >= MemoryUpdateSimilarityThreshold {
			highSimMatches = append(highSimMatches, match)
		}
	}

	if len(highSimMatches) == 0 {
		return
	}

	// 使用 LLM 判断哪些记忆需要更新
	updates := d.judgeMemoryUpdates(report, highSimMatches)
	if len(updates) == 0 {
		return
	}

	// 执行批量更新
	if err := d.memory.BatchUpdate(updates); err != nil {
		logger.Warn("Learner", "记忆批量更新失败: %v", err)
	}
}

// judgeMemoryUpdates 使用 LLM 判断记忆更新
func (d *DebateSystem) judgeMemoryUpdates(report string, matches []MemoryMatch) []MemoryUpdate {
	var memInfo []string
	for _, m := range matches {
		memInfo = append(memInfo, fmt.Sprintf("ID: %s\n内容: %s\n相似度: %.2f", m.ID, m.Content, m.Similarity))
	}

	prompt := fmt.Sprintf(`%s

研究报告摘要：
%s

需要评估的现有记忆条目：
%s

请判断哪些记忆需要更新，以 JSON 数组格式输出更新指令。`, d.promptMemory, truncateText(report, 1000), strings.Join(memInfo, "\n\n"))

	messages := []LLMMessage{
		{Role: "system", Content: d.promptMemory},
		{Role: "user", Content: prompt},
	}

	resp, err := d.llm.Chat(messages, BudgetMemoryUpdate)
	if err != nil {
		logger.Warn("Learner", "记忆更新判断失败: %v", err)
		return nil
	}

	// 解析 JSON
	jsonStr := extractJSON(strings.TrimSpace(resp.Content))
	if jsonStr == "" || jsonStr == "[]" {
		return nil
	}

	var updates []MemoryUpdate
	if err := json.Unmarshal([]byte(jsonStr), &updates); err != nil {
		logger.Warn("Learner", "记忆更新指令解析失败: %v", err)
		return nil
	}

	return updates
}
