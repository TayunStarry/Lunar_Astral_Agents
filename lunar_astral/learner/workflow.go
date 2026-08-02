package learner

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"logger"
)

// ============================================================
// 步骤 a: AI 推理完善请求
// ============================================================

// refineQuery 将用户原始请求推理完善为结构化的查询
func (w *WorkflowRunner) refineQuery(rawQuery string) (*RefinedQuery, error) {
	w.state.CurrentPhase = PhaseRefine
	logger.Info("Learner", "=== 步骤 a: AI 推理完善请求 ===")

	prompt := w.prompts.buildRefinePrompt(rawQuery)
	messages := []LLMMessage{
		{Role: "system", Content: prompt},
		{Role: "user", Content: "请完善以下查询：" + rawQuery},
	}

	resp, err := w.llm.Chat(messages, BudgetRefine)
	if err != nil {
		logger.Error("Learner", "查询推理完善失败: %v", err)
		// 降级：使用原始请求作为完善后的查询
		return &RefinedQuery{
			Original:    rawQuery,
			Refined:     rawQuery,
			KeyPoints:   []string{rawQuery},
			SearchTerms: []string{rawQuery},
		}, nil
	}

	jsonStr := extractJSON(strings.TrimSpace(resp.Content))
	var refined RefinedQuery
	if err := json.Unmarshal([]byte(jsonStr), &refined); err != nil {
		logger.Warn("Learner", "查询推理结果解析失败: %v，使用原始查询", err)
		return &RefinedQuery{
			Original:    rawQuery,
			Refined:     rawQuery,
			KeyPoints:   []string{rawQuery},
			SearchTerms: []string{rawQuery},
		}, nil
	}

	refined.Original = rawQuery
	logger.Info("Learner", "查询推理完成: refined=%s, key_points=%d, search_terms=%d",
		truncateRunes(refined.Refined, 100), len(refined.KeyPoints), len(refined.SearchTerms))

	return &refined, nil
}

// ============================================================
// 步骤 b: 查询记忆库（知识 + 经验双表并行查询）
// ============================================================

// queryMemory 同时查询知识记忆和经验记忆
func (w *WorkflowRunner) queryMemory(query string) ([]MemoryMatch, []MemoryMatch, error) {
	w.state.CurrentPhase = PhaseMemoryQuery
	logger.Info("Learner", "=== 步骤 b: 查询记忆库 ===")

	if !w.memory.IsAvailable() {
		logger.Warn("Learner", "记忆库不可用，跳过记忆查询")
		return nil, nil, nil
	}

	knowledgeResults, experienceResults, err := w.memory.QueryBoth(query)
	if err != nil {
		logger.Error("Learner", "记忆查询失败: %v", err)
		return nil, nil, err
	}

	logger.Info("Learner", "记忆查询完成: 知识=%d条, 经验=%d条",
		len(knowledgeResults), len(experienceResults))

	return knowledgeResults, experienceResults, nil
}

// ============================================================
// 步骤 d: 初步网络搜索
// ============================================================

// simpleWebSearch 执行初步网络搜索（轻量摘要模式）
func (w *WorkflowRunner) simpleWebSearch(query string) []SearchItemPreview {
	w.state.CurrentPhase = PhaseWebSearch
	logger.Info("Learner", "=== 步骤 d: 初步网络搜索 ===")

	if !w.search.IsAvailable() {
		logger.Warn("Learner", "搜索子系统不可用，跳过初步搜索")
		return nil
	}

	// 使用完善后的查询的第一个搜索词
	searchQuery := query
	results, err := w.search.SimpleSearch(searchQuery)
	if err != nil {
		logger.Warn("Learner", "初步网络搜索失败: %v", err)
		return nil
	}

	return results
}

// ============================================================
// 步骤 e: AI 总结评估 + 决策
// ============================================================

// evaluateAndDecide 评估已收集信息，决定是否需要深度搜索
// 经验记忆作为上下文注入（策略 B）
func (w *WorkflowRunner) evaluateAndDecide(
	refinedQuery string,
	knowledgeMem []MemoryMatch,
	experienceMem []MemoryMatch,
	searchItems []SearchItemPreview,
) (*EvaluationResult, error) {
	w.state.CurrentPhase = PhaseEvaluate
	logger.Info("Learner", "=== 步骤 e: AI 总结评估 + 决策 ===")

	prompt := w.prompts.buildEvaluatePrompt(refinedQuery, knowledgeMem, experienceMem, searchItems)
	messages := []LLMMessage{
		{Role: "system", Content: prompt},
		{Role: "user", Content: "请评估现有信息是否足以回答用户问题，并输出策略 JSON。"},
	}

	resp, err := w.llm.Chat(messages, BudgetEvaluate)
	if err != nil {
		logger.Error("Learner", "策略评估失败: %v，降级为深度搜索", err)
		return &EvaluationResult{
			Sufficient:     false,
			NeedDeepSearch: true,
			DeepSearchQuery: refinedQuery,
			Reasoning:      fmt.Sprintf("评估失败，降级为深度搜索: %v", err),
		}, nil
	}

	jsonStr := extractJSON(strings.TrimSpace(resp.Content))
	var eval EvaluationResult
	if err := json.Unmarshal([]byte(jsonStr), &eval); err != nil {
		logger.Warn("Learner", "评估结果解析失败: %v，降级为深度搜索", err)
		return &EvaluationResult{
			Sufficient:      false,
			NeedDeepSearch:  true,
			DeepSearchQuery: refinedQuery,
			Reasoning:       fmt.Sprintf("评估结果解析失败，降级为深度搜索: %v", err),
		}, nil
	}

	logger.Info("Learner", "策略评估完成: sufficient=%v, need_deep=%v, query=%s",
		eval.Sufficient, eval.NeedDeepSearch, truncateRunes(eval.DeepSearchQuery, 60))

	return &eval, nil
}

// ============================================================
// 步骤 g/h: 深度搜索循环（最多 5 轮）
// ============================================================

// deepSearchLoop 执行深度搜索循环
// 每轮：搜索 → AI 评估 → 充足则退出 / 不足则补充搜索
// 最多 MaxDeepSearchRounds 轮
func (w *WorkflowRunner) deepSearchLoop(refinedQuery string, eval *EvaluationResult) ([]SearchRound, string, error) {
	w.state.CurrentPhase = PhaseDeepSearch
	logger.Info("Learner", "=== 步骤 g/h: 深度搜索循环 ===")

	searchQuery := eval.DeepSearchQuery
	if searchQuery == "" {
		searchQuery = refinedQuery
	}

	var rounds []SearchRound
	var allPreviousSummaries string

	for roundNum := 1; roundNum <= MaxDeepSearchRounds; roundNum++ {
		logger.Info("Learner", "深度搜索轮次 %d/%d: %s", roundNum, MaxDeepSearchRounds, searchQuery)

		// 执行深度搜索
		searchResult, err := w.search.DepthSearch(searchQuery)
		if err != nil {
			logger.Warn("Learner", "深度搜索第 %d 轮失败: %v", roundNum, err)
			searchResult = fmt.Sprintf("搜索失败: %v", err)
		}

		// 压缩搜索结果
		compressedResult := truncateRunes(searchResult, DeepSearchResultMaxChars)

		// AI 评估搜索结果
		evalPrompt := w.prompts.buildSearchEvalPrompt(refinedQuery, compressedResult, roundNum, allPreviousSummaries)
		evalMessages := []LLMMessage{
			{Role: "system", Content: evalPrompt},
			{Role: "user", Content: "请评估搜索结果并输出 JSON。"},
		}

		evalResp, evalErr := w.llm.Chat(evalMessages, BudgetSearchEval)
		var searchEval SearchEvaluation
		if evalErr != nil {
			logger.Warn("Learner", "搜索评估第 %d 轮失败: %v", roundNum, evalErr)
			searchEval = SearchEvaluation{
				Sufficient: false,
				Summary:    compressedResult,
				Reasoning:  fmt.Sprintf("评估失败: %v", evalErr),
			}
		} else {
			jsonStr := extractJSON(strings.TrimSpace(evalResp.Content))
			if err := json.Unmarshal([]byte(jsonStr), &searchEval); err != nil {
				logger.Warn("Learner", "搜索评估解析失败: %v", err)
				searchEval = SearchEvaluation{
					Sufficient: false,
					Summary:    compressedResult,
					Reasoning:  fmt.Sprintf("评估结果解析失败: %v", err),
				}
			}
		}

		// 记录本轮
		round := SearchRound{
			RoundNum:   roundNum,
			Query:      searchQuery,
			Result:     compressedResult,
			Evaluation: searchEval.Reasoning,
			Sufficient: searchEval.Sufficient,
		}
		rounds = append(rounds, round)

		// 累加摘要
		allPreviousSummaries += fmt.Sprintf("轮次%d: %s\n", roundNum, searchEval.Summary)

		logger.Info("Learner", "深度搜索轮次 %d 完成: sufficient=%v", roundNum, searchEval.Sufficient)

		// 信息充足 → 退出
		if searchEval.Sufficient {
			logger.Info("Learner", "深度搜索第 %d 轮信息充足，退出循环", roundNum)
			// 简历最后一轮的摘要
			return rounds, searchEval.Summary, nil
		}

		// 信息不足 → 准备下一轮搜索
		if searchEval.SupplementaryQuery != "" {
			searchQuery = searchEval.SupplementaryQuery
		} else {
			// 无补充搜索词，退出循环
			logger.Info("Learner", "无补充搜索词，退出循环")
			break
		}
	}

	// 达到最大轮次，返回现有信息
	logger.Info("Learner", "深度搜索达到最大轮次 %d，返回现有信息", MaxDeepSearchRounds)

	// 构建不足时的报告
	partialInfo := buildPartialInfoFromRounds(rounds)
	report := fmt.Sprintf(defaultInsufficientReport, refinedQuery, partialInfo, len(rounds))

	return rounds, report, nil
}

// ============================================================
// 步骤 i: 统一处理工作流（记忆更新 + 返回结果）
// ============================================================

// unifiedProcessing 统一处理：更新记忆库 + 准备最终报告
func (w *WorkflowRunner) unifiedProcessing(
	refinedQuery *RefinedQuery,
	finalReport string,
	searchRounds []SearchRound,
	knowledgeMem []MemoryMatch,
) (*MemoryBatchResult, string) {
	w.state.CurrentPhase = PhaseFinalize
	logger.Info("Learner", "=== 步骤 i: 统一处理工作流 ===")

	batchResult := &MemoryBatchResult{}

	if !w.memory.IsAvailable() {
		logger.Warn("Learner", "记忆库不可用，跳过记忆更新")
		return batchResult, finalReport
	}

	// 1. 更新知识记忆：将网络搜索获取的新知识存入
	knowledgeItems := extractKnowledgeItems(finalReport, searchRounds)
	if len(knowledgeItems) > 0 {
		count, err := w.memory.BatchAddKnowledge(knowledgeItems)
		if err != nil {
			logger.Warn("Learner", "知识记忆批量更新失败: %v", err)
		} else {
			batchResult.KnowledgeAdded = count
			logger.Info("Learner", "知识记忆更新: 新增 %d 条", count)
		}
	}

	// 2. 更新经验记忆：记录本次请求的处理策略
	experienceItem := w.generateExperienceItem(refinedQuery, searchRounds, finalReport)
	if experienceItem != "" {
		_, err := w.memory.AddExperience(experienceItem)
		if err != nil {
			logger.Warn("Learner", "经验记忆更新失败: %v", err)
		} else {
			batchResult.ExperienceAdded = 1
			logger.Info("Learner", "经验记忆更新: 新增 1 条")
		}
	}

	// 3. 检查并替代高相似度的旧知识记忆
	w.supersedeOldKnowledge(finalReport, knowledgeMem)

	logger.Info("Learner", "统一处理完成: 知识新增=%d, 经验新增=%d",
		batchResult.KnowledgeAdded, batchResult.ExperienceAdded)

	return batchResult, finalReport
}

// ============================================================
// 辅助方法
// ============================================================

// extractKnowledgeItems 从最终报告和搜索结果中提取知识条目
func extractKnowledgeItems(report string, searchRounds []SearchRound) []string {
	var items []string

	// 从报告中提取（摘要长度限制）
	reportSummary := truncateRunes(report, 1000)
	if len([]rune(reportSummary)) >= 50 {
		items = append(items, reportSummary)
	}

	// 从每轮搜索结果中提取
	for _, round := range searchRounds {
		if round.Sufficient && len([]rune(round.Result)) >= 50 {
			items = append(items, truncateRunes(round.Result, 800))
		}
	}

	return items
}

// generateExperienceItem 生成经验记忆条目
func (w *WorkflowRunner) generateExperienceItem(
	refinedQuery *RefinedQuery,
	searchRounds []SearchRound,
	finalReport string,
) string {
	now := time.Now().Format("2006-01-02 15:04:05")

	desc := fmt.Sprintf("[经验记忆] 时间: %s\n", now)
	desc += fmt.Sprintf("原始查询: %s\n", truncateRunes(refinedQuery.Original, 200))
	desc += fmt.Sprintf("完善后查询: %s\n", truncateRunes(refinedQuery.Refined, 200))

	if len(refinedQuery.SearchTerms) > 0 {
		desc += fmt.Sprintf("搜索词: %s\n", strings.Join(refinedQuery.SearchTerms, ", "))
	}

	if len(searchRounds) == 0 {
		desc += "搜索策略: 信息已充足，无需深度搜索\n"
		desc += "效果: 直接返回，延迟低\n"
	} else {
		desc += fmt.Sprintf("搜索策略: 深度搜索 %d 轮\n", len(searchRounds))
		for _, round := range searchRounds {
			desc += fmt.Sprintf("  轮次%d: 查询=%s, 充足=%v\n",
				round.RoundNum, round.Query, round.Sufficient)
		}
		lastRound := searchRounds[len(searchRounds)-1]
		desc += fmt.Sprintf("效果: %d轮搜索后信息%s\n",
			len(searchRounds),
			map[bool]string{true: "充足", false: "不充足"}[lastRound.Sufficient])
	}

	// 附上指导建议
	desc += "指导建议: "
	if len(searchRounds) == 0 {
		desc += "此类查询通过记忆库和初步搜索即可满足，建议优先查询记忆库。"
	} else if len(searchRounds) <= 2 {
		desc += "此类查询需要少量深度搜索，建议使用标准搜索策略。"
	} else {
		desc += "此类查询需要多轮深度搜索，建议启用多角度搜索策略。"
	}

	return desc
}

// supersedeOldKnowledge 查找并替代高相似度的旧知识记忆
func (w *WorkflowRunner) supersedeOldKnowledge(newReport string, existingMem []MemoryMatch) {
	if len(existingMem) == 0 {
		return
	}

	superseded, err := w.memory.FindSuperseded(TableKnowledge, newReport, MemoryUpdateSimilarityThreshold)
	if err != nil {
		logger.Warn("Learner", "查找旧知识记忆失败: %v", err)
		return
	}

	for _, old := range superseded {
		if err := w.memory.DeleteEntry(TableKnowledge, old.ID); err != nil {
			logger.Warn("Learner", "删除旧知识记忆失败 id=%s: %v", old.ID, err)
		} else {
			logger.Info("Learner", "已替代旧知识记忆: id=%s, 相似度=%.1f%%", old.ID, old.Similarity*100)
		}
	}
}

// buildPartialInfoFromRounds 从搜索轮次中构建部分信息摘要
func buildPartialInfoFromRounds(rounds []SearchRound) string {
	var parts []string
	for _, round := range rounds {
		parts = append(parts, fmt.Sprintf("- 轮次%d (查询: %s): %s",
			round.RoundNum,
			truncateRunes(round.Query, 50),
			truncateRunes(round.Result, 200)))
	}
	return strings.Join(parts, "\n")
}

// ============================================================
// 工作流运行器
// ============================================================

// WorkflowRunner 工作流执行器
// 封装所有工作流步骤，持有 LLM、搜索、记忆等依赖
type WorkflowRunner struct {
	llm     *LLMClient
	search  *SearchManager
	memory  *MemoryManager
	prompts *PromptTemplates
	state   *WorkflowState
}

// NewWorkflowRunner 创建工作流运行器
func NewWorkflowRunner(llm *LLMClient, search *SearchManager, memory *MemoryManager, prompts *PromptTemplates) *WorkflowRunner {
	return &WorkflowRunner{
		llm:     llm,
		search:  search,
		memory:  memory,
		prompts: prompts,
	}
}

// Run 执行完整工作流
// 返回最终报告、工作流状态和错误
func (w *WorkflowRunner) Run(rawQuery string) (string, *WorkflowState, error) {
	startTime := time.Now()
	w.state = &WorkflowState{
		OriginalQuery: rawQuery,
	}

	// 步骤 a: AI 推理完善请求
	refined, err := w.refineQuery(rawQuery)
	if err != nil {
		return "", w.state, fmt.Errorf("步骤 a 失败: %w", err)
	}
	w.state.RefinedQuery = refined

	// 步骤 b: 查询记忆库（知识 + 经验）
	knowledgeMem, experienceMem, err := w.queryMemory(refined.Refined)
	if err != nil {
		// 记忆查询失败不阻断流程，继续使用搜索
		logger.Warn("Learner", "记忆查询失败，将仅使用搜索: %v", err)
	}
	w.state.KnowledgeMem = knowledgeMem
	w.state.ExperienceMem = experienceMem

	// 步骤 d: 初步网络搜索
	searchItems := w.simpleWebSearch(refined.Refined)
	w.state.SimpleSearch = searchItems

	// 步骤 e: AI 总结评估 + 决策
	eval, err := w.evaluateAndDecide(refined.Refined, knowledgeMem, experienceMem, searchItems)
	if err != nil {
		return "", w.state, fmt.Errorf("步骤 e 失败: %w", err)
	}
	w.state.Evaluation = eval

	// 如果信息充足，直接返回阶段性摘要
	if eval.Sufficient {
		report := eval.Summary
		// 确保报告以 [研究报告] 开头
		if !strings.HasPrefix(report, "[研究报告]") {
			report = "[研究报告]\n\n" + report
		}

		// 步骤 i: 统一处理（即使信息充足也更新记忆）
		batchResult, finalReport := w.unifiedProcessing(refined, report, nil, knowledgeMem)
		w.state.FinalReport = finalReport

		elapsed := time.Since(startTime)
		logger.Info("Learner", "研究完成(直接回答): 耗时=%v, 知识新增=%d, 经验新增=%d",
			elapsed, batchResult.KnowledgeAdded, batchResult.ExperienceAdded)

		return finalReport, w.state, nil
	}

	// 步骤 g/h: 深度搜索循环
	searchRounds, searchReport, err := w.deepSearchLoop(refined.Refined, eval)
	if err != nil {
		logger.Error("Learner", "深度搜索循环失败: %v", err)
		// 降级：使用已有信息构建报告
		searchReport = buildFallbackReport(refined.Refined, knowledgeMem, searchItems)
	}
	w.state.SearchRounds = searchRounds

	// 如果 deepSearchLoop 返回了不足报告（达到最大轮次），直接使用
	finalReport := searchReport
	if !strings.HasPrefix(finalReport, "[研究报告]") {
		// 检查是否已经是格式化报告
		if !strings.Contains(finalReport, "[研究报告]") {
			finalReport = "[研究报告]\n\n" + finalReport
		}
	}

	// 步骤 i: 统一处理
	batchResult, finalReport := w.unifiedProcessing(refined, finalReport, searchRounds, knowledgeMem)
	w.state.FinalReport = finalReport

	elapsed := time.Since(startTime)
	logger.Info("Learner", "研究完成(深度搜索): 耗时=%v, 搜索轮次=%d, 知识新增=%d, 经验新增=%d",
		elapsed, len(searchRounds), batchResult.KnowledgeAdded, batchResult.ExperienceAdded)

	return finalReport, w.state, nil
}

// buildFallbackReport 构建降级报告（深度搜索失败时使用）
func buildFallbackReport(refinedQuery string, knowledgeMem []MemoryMatch, searchItems []SearchItemPreview) string {
	var parts []string
	parts = append(parts, fmt.Sprintf("[研究报告]\n\n## 研究主题\n%s\n\n## 研究结论\n深度搜索未能完成，以下是基于已有信息的部分结果。\n\n## 记忆库信息", refinedQuery))

	if len(knowledgeMem) > 0 {
		parts = append(parts, FormatMemoryResults(knowledgeMem))
	} else {
		parts = append(parts, "记忆库中无相关信息。")
	}

	if len(searchItems) > 0 {
		parts = append(parts, "\n## 初步搜索摘要")
		for i, item := range searchItems {
			parts = append(parts, fmt.Sprintf("[%d] %s: %s", i+1, item.Title, item.Snippet))
		}
	}

	parts = append(parts, "\n## 疑点与未解决问题\n深度搜索未能完成，建议稍后重试或调整查询方向。")

	return strings.Join(parts, "\n")
}