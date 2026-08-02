package learner

import (
	"encoding/json"
	"fmt"
	"strings"

	"logger"
)

// ============================================================
// 阶段4: 综合报告
// ============================================================

// synthesizeReport 生成综合研究报告（先提纲、再细化）
func (d *DebateSystem) synthesizeReport() (string, error) {
	d.state.CurrentPhase = PhaseSynthesize

	// 第一步：生成报告提纲
	outline, err := d.generateOutline()
	if err != nil {
		logger.Warn("Learner", "提纲生成失败: %v，跳过提纲阶段", err)
		// 降级：直接生成报告
		return d.generateFullReport(nil)
	}

	logger.Info("Learner", "报告提纲: %s, %d 个要点", outline.Topic, len(outline.Sections))

	// 第二步：基于提纲生成完整报告
	report, err := d.generateFullReport(outline)
	if err != nil {
		return "", err
	}

	return report, nil
}

// generateOutline 生成报告提纲
func (d *DebateSystem) generateOutline() (*ReportOutline, error) {
	dataSummary := d.buildDataSummary()

	prompt := fmt.Sprintf(`%s

请基于以下研究数据生成报告提纲。

原始问题：%s

研究数据摘要：
%s

要求：
1. 明确研究主题
2. 给出核心结论方向
3. 列出3-5个提纲要点
4. 标记待解决的疑点
5. 以 JSON 格式输出：{"topic":"...","conclusion":"...","sections":["要点1","要点2","要点3"],"doubts":["疑点1","疑点2"]}`,
		d.promptReport, d.state.OriginalQuery, dataSummary)

	messages := []LLMMessage{
		{Role: "system", Content: d.promptReport},
		{Role: "user", Content: prompt},
	}

	resp, err := d.llm.Chat(messages, BudgetReportOutline)
	if err != nil {
		return nil, err
	}

	// 解析提纲 JSON
	jsonStr := extractJSON(strings.TrimSpace(resp.Content))

	var outline ReportOutline
	if err := json.Unmarshal([]byte(jsonStr), &outline); err != nil {
		return nil, fmt.Errorf("提纲解析失败: %w", err)
	}

	return &outline, nil
}

// generateFullReport 基于提纲生成完整报告
func (d *DebateSystem) generateFullReport(outline *ReportOutline) (string, error) {
	dataSummary := d.buildDataSummary()

	var outlineText string
	if outline != nil {
		outlineText = fmt.Sprintf("主题：%s\n结论方向：%s\n提纲要点：%s\n疑点：%s",
			outline.Topic, outline.Conclusion,
			strings.Join(outline.Sections, "、"),
			strings.Join(outline.Doubts, "、"))
	} else {
		outlineText = "（提纲生成失败，请直接根据数据生成报告）"
	}

	prompt := fmt.Sprintf(`%s

原始问题：%s

报告提纲：
%s

详细研究数据：
%s

请按照 [研究报告] 格式输出完整的研究报告。`,
		d.promptReport, d.state.OriginalQuery, outlineText, dataSummary)

	messages := []LLMMessage{
		{Role: "system", Content: d.promptReport},
		{Role: "user", Content: prompt},
	}

	resp, err := d.llm.Chat(messages, BudgetReportFull)
	if err != nil {
		return "", fmt.Errorf("报告生成失败: %w", err)
	}

	report := strings.TrimSpace(resp.Content)

	// 确保报告以 [研究报告] 开头
	if !strings.HasPrefix(report, "[研究报告]") {
		report = "[研究报告]\n\n" + report
	}

	return report, nil
}

// buildDataSummary 构建研究数据摘要（用于报告生成）
func (d *DebateSystem) buildDataSummary() string {
	var parts []string

	// 子问题搜索结果
	for i, sq := range d.state.SubQuestions {
		parts = append(parts, fmt.Sprintf("### 子问题%d：%s\n搜索结果：%s",
			i+1, sq.Question, truncateText(sq.SearchResult, SearchResultMaxChars)))
	}

	// 记忆数据
	if len(d.state.MemoryResults) > 0 {
		parts = append(parts, fmt.Sprintf("### 记忆库数据\n%s",
			FormatMemoryResults(d.state.MemoryResults)))
	}

	// 辩论摘要
	if len(d.state.Rounds) > 0 {
		var debateParts []string
		for _, r := range d.state.Rounds {
			if r.Summary != "" {
				debateParts = append(debateParts, r.Summary)
			}
		}
		if len(debateParts) > 0 {
			parts = append(parts, fmt.Sprintf("### 辩论过程摘要\n%s",
				strings.Join(debateParts, "\n")))
		}
	}

	return strings.Join(parts, "\n\n")
}
