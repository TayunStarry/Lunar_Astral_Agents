package learner

import (
	"fmt"
	"lunar_astral/hierarchy"
)

// ============================================================
// Prompt 模板管理
// ============================================================

// PromptTemplates 所有 prompt 模板的集合
type PromptTemplates struct {
	Refine     string // 步骤 a: 查询推理完善
	Evaluate   string // 步骤 e: 策略评估
	SearchEval string // 步骤 h: 搜索内容评估
	Memory     string // 步骤 i: 记忆更新
	Report     string // 步骤 i: 报告生成
}

// loadPrompts 从嵌入式文件系统加载 prompt 模板
// 加载失败时回退到内置默认模板
func loadPrompts() *PromptTemplates {
	pt := &PromptTemplates{}

	// 尝试从嵌入式文件系统加载
	if data, err := hierarchy.EmbeddedFiles.ReadFile("assets/prompts/learnerRefine.md"); err == nil {
		pt.Refine = string(data)
	}
	if data, err := hierarchy.EmbeddedFiles.ReadFile("assets/prompts/learnerEvaluate.md"); err == nil {
		pt.Evaluate = string(data)
	}
	if data, err := hierarchy.EmbeddedFiles.ReadFile("assets/prompts/learnerSearchEval.md"); err == nil {
		pt.SearchEval = string(data)
	}
	if data, err := hierarchy.EmbeddedFiles.ReadFile("assets/prompts/learnerMemory.md"); err == nil {
		pt.Memory = string(data)
	}
	if data, err := hierarchy.EmbeddedFiles.ReadFile("assets/prompts/learnerReport.md"); err == nil {
		pt.Report = string(data)
	}

	// 回退到内置默认模板
	if pt.Refine == "" {
		pt.Refine = defaultRefinePrompt
	}
	if pt.Evaluate == "" {
		pt.Evaluate = defaultEvaluatePrompt
	}
	if pt.SearchEval == "" {
		pt.SearchEval = defaultSearchEvalPrompt
	}
	if pt.Memory == "" {
		pt.Memory = defaultMemoryPrompt
	}
	if pt.Report == "" {
		pt.Report = defaultReportPrompt
	}

	return pt
}

// buildRefinePrompt 构建步骤 a 的 refine prompt
func (pt *PromptTemplates) buildRefinePrompt(rawQuery string) string {
	return fmt.Sprintf(`%s

用户的原始请求：
%s

请输出完善后的结构化查询 JSON。`, pt.Refine, rawQuery)
}

// buildEvaluatePrompt 构建步骤 e 的评估 prompt
// 经验记忆作为上下文注入（策略 B）
func (pt *PromptTemplates) buildEvaluatePrompt(
	refinedQuery string,
	knowledgeMem []MemoryMatch,
	experienceMem []MemoryMatch,
	searchItems []SearchItemPreview,
) string {
	// 格式化知识记忆
	knowledgeText := formatMemoryForPrompt(knowledgeMem, "知识记忆")
	if knowledgeText == "" {
		knowledgeText = "（知识记忆库中无相关记录）"
	}

	// 格式化经验记忆（策略 B：注入为评估上下文）
	experienceText := formatMemoryForPrompt(experienceMem, "经验记忆（之前类似请求的处理策略）")
	if experienceText == "" {
		experienceText = "（经验记忆库中无相关记录，这是首次处理此类请求）"
	}

	// 格式化搜索摘要
	searchText := formatSearchItemsForPrompt(searchItems)
	if searchText == "" {
		searchText = "（初步网络搜索无结果）"
	}

	return fmt.Sprintf(`%s

用户完善后的查询：
%s

%s

%s

初步网络搜索摘要：
%s

请评估信息是否足以回答用户问题，并输出策略 JSON。`,
		pt.Evaluate, refinedQuery, experienceText, knowledgeText, searchText)
}

// buildSearchEvalPrompt 构建步骤 h 的搜索内容评估 prompt
func (pt *PromptTemplates) buildSearchEvalPrompt(
	refinedQuery string,
	searchResult string,
	roundNum int,
	allPreviousResults string,
) string {
	previousText := ""
	if allPreviousResults != "" {
		previousText = fmt.Sprintf("\n前序搜索轮次摘要：\n%s\n", allPreviousResults)
	}

	return fmt.Sprintf(`%s

用户查询：
%s

当前第%d轮深度搜索结果：
%s
%s
请评估搜索内容是否足以回答用户问题，并输出评估 JSON。`,
		pt.SearchEval, refinedQuery, roundNum, searchResult, previousText)
}

// buildMemoryUpdatePrompt 构建步骤 i 的记忆更新 prompt
func (pt *PromptTemplates) buildMemoryUpdatePrompt(
	originalQuery string,
	refinedQuery string,
	finalReport string,
	searchRounds []SearchRound,
	knowledgeMem []MemoryMatch,
) string {
	// 构建搜索策略描述
	strategyDesc := buildStrategyDescription(originalQuery, refinedQuery, searchRounds)

	return fmt.Sprintf(`%s

原始查询：%s
完善后查询：%s

最终研究报告：
%s

搜索策略摘要：
%s

现有知识记忆条目数：%d

请生成记忆更新指令 JSON。`,
		pt.Memory, originalQuery, refinedQuery,
		truncateRunes(finalReport, 2000),
		strategyDesc,
		len(knowledgeMem))
}

// buildReportPrompt 构建报告生成 prompt
func (pt *PromptTemplates) buildReportPrompt(
	refinedQuery string,
	knowledgeMem []MemoryMatch,
	searchRounds []SearchRound,
	evalSummary string,
) string {
	// 汇总所有搜索轮次结果
	var searchResultsText string
	for _, round := range searchRounds {
		searchResultsText += fmt.Sprintf("\n### 搜索轮次 %d\n查询词：%s\n结果：%s\n评估：%s\n",
			round.RoundNum, round.Query,
			truncateRunes(round.Result, 1000),
			round.Evaluation)
	}

	// 知识记忆
	knowledgeText := formatMemoryForPrompt(knowledgeMem, "知识记忆")
	if knowledgeText == "" {
		knowledgeText = "（无相关知识记忆）"
	}

	return fmt.Sprintf(`%s

用户查询：%s

评估摘要：%s

%s

搜索结果：
%s

请按照 [研究报告] 格式输出完整的研究报告。`,
		pt.Report, refinedQuery, evalSummary, knowledgeText, searchResultsText)
}

// ============================================================
// 辅助格式化函数
// ============================================================

// formatMemoryForPrompt 格式化记忆结果为 prompt 可读文本
func formatMemoryForPrompt(matches []MemoryMatch, label string) string {
	if len(matches) == 0 {
		return ""
	}

	var result string
	result += fmt.Sprintf("## %s\n", label)
	for i, m := range matches {
		result += fmt.Sprintf("[%d] 相关度:%.1f%% | %s\n",
			i+1, m.Similarity*100, truncateRunes(m.Content, 500))
	}
	return result
}

// formatSearchItemsForPrompt 格式化搜索摘要为 prompt 可读文本
func formatSearchItemsForPrompt(items []SearchItemPreview) string {
	if len(items) == 0 {
		return ""
	}

	var result string
	for i, item := range items {
		result += fmt.Sprintf("[%d] %s\n    URL: %s\n    摘要: %s\n\n",
			i+1, item.Title, item.URL, truncateRunes(item.Snippet, 200))
	}
	return result
}

// buildStrategyDescription 构建搜索策略描述（用于经验记忆存储）
func buildStrategyDescription(originalQuery, refinedQuery string, searchRounds []SearchRound) string {
	desc := fmt.Sprintf("查询类型：%s\n", classifyQueryType(originalQuery))
	desc += fmt.Sprintf("完善后查询：%s\n", truncateRunes(refinedQuery, 200))

	if len(searchRounds) == 0 {
		desc += "搜索策略：无需深度搜索，信息已充足\n"
	} else {
		desc += fmt.Sprintf("搜索策略：深度搜索 %d 轮\n", len(searchRounds))
		for _, round := range searchRounds {
			desc += fmt.Sprintf("  轮次%d: 查询词=%s, 充足=%v\n",
				round.RoundNum, round.Query, round.Sufficient)
		}
	}

	desc += fmt.Sprintf("效果评估：共%d轮搜索，最终信息%s",
		len(searchRounds),
		map[bool]string{true: "充足", false: "不充足"}[len(searchRounds) > 0 && searchRounds[len(searchRounds)-1].Sufficient])

	return desc
}

// classifyQueryType 简单分类查询类型
func classifyQueryType(query string) string {
	runes := []rune(query)
	if len(runes) > 50 {
		return "复杂查询"
	}
	return "简单查询"
}