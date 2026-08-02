package learner

import (
	"fmt"
	"strings"
	"sync"

	"logger"
	"lunar_astral/hierarchy"
)

// DebateSystem 辩论系统
// 4阶段状态机：问题分析 → 并行搜索 → 辩论循环 → 综合报告
type DebateSystem struct {
	state      *DebateState
	llm        *LLMClient
	search     *SearchManager
	memory     *MemoryManager
	multiAngle bool // 是否启用多角度搜索

	// Prompt 模板缓存
	promptDebate   string
	promptReport   string
	promptMemory   string
	promptStrategy string // 策略评估 prompt
}

// NewDebateSystem 创建辩论系统
func NewDebateSystem(llm *LLMClient, search *SearchManager, memory *MemoryManager) *DebateSystem {
	ds := &DebateSystem{
		llm:    llm,
		search: search,
		memory: memory,
	}

	// 加载 prompt 模板
	ds.loadPrompts()

	return ds
}

// loadPrompts 从嵌入式文件系统加载 prompt 模板
func (d *DebateSystem) loadPrompts() {
	// 尝试从嵌入式文件系统加载
	if data, err := hierarchy.EmbeddedFiles.ReadFile("assets/prompts/learnerDebate.md"); err == nil {
		d.promptDebate = string(data)
	}
	if data, err := hierarchy.EmbeddedFiles.ReadFile("assets/prompts/learnerReport.md"); err == nil {
		d.promptReport = string(data)
	}
	if data, err := hierarchy.EmbeddedFiles.ReadFile("assets/prompts/learnerMemory.md"); err == nil {
		d.promptMemory = string(data)
	}
	if data, err := hierarchy.EmbeddedFiles.ReadFile("assets/prompts/learnerStrategy.md"); err == nil {
		d.promptStrategy = string(data)
	}

	// 如果加载失败，使用内置默认模板
	if d.promptDebate == "" {
		d.promptDebate = defaultDebatePrompt
	}
	if d.promptReport == "" {
		d.promptReport = defaultReportPrompt
	}
	if d.promptMemory == "" {
		d.promptMemory = defaultMemoryPrompt
	}
}

// Execute 执行完整的辩论研究流程（接受策略计划参数）
func (d *DebateSystem) Execute(query string, plan StrategyPlan) (*LearnerResult, error) {
	// 设置多角度搜索标志
	d.multiAngle = plan.MultiAngleSearch

	d.state = &DebateState{
		OriginalQuery: query,
		MaxRounds:     plan.DebateRounds,
		CurrentPhase:  PhaseAnalyze,
	}

	// 从策略计划直接注入子问题（不再调用 analyzeQuestion）
	if len(plan.SubQuestions) > 0 {
		d.state.SubQuestions = plan.SubQuestions
	} else {
		// 降级：使用原始查询作为唯一子问题
		d.state.SubQuestions = []SubQuestion{
			{Question: query, SearchQuery: query, Source: "降级-原始查询"},
		}
	}

	// 注入运行时上下文（时间 + 位置）
	_ = d.getRuntimeContext() // 上下文已由策略评估阶段处理

	// 阶段1: 并行搜索（子问题已由策略计划注入）
	logger.Info("Learner", "=== 阶段2: 并行搜索 ===")
	if err := d.parallelSearch(); err != nil {
		logger.Error("Learner", "并行搜索失败: %v", err)
	}

	// 阶段3: 辩论循环
	logger.Info("Learner", "=== 阶段3: 辩论循环 ===")
	if err := d.runDebate(); err != nil {
		logger.Error("Learner", "辩论循环失败: %v", err)
	}

	// 阶段4: 综合报告
	logger.Info("Learner", "=== 阶段4: 综合报告 ===")
	report, err := d.synthesizeReport()
	if err != nil {
		logger.Error("Learner", "综合报告失败: %v", err)
		return nil, fmt.Errorf("综合报告生成失败: %w", err)
	}

	// 记忆更新
	logger.Info("Learner", "=== 记忆更新 ===")
	d.updateMemory(report)

	// 构建结果
	result := &LearnerResult{
		Report:       report,
		SearchRounds: len(d.state.SubQuestions),
		DebateRounds: len(d.state.Rounds),
	}

	// 收集信息来源
	for _, sq := range d.state.SubQuestions {
		if sq.Source != "" {
			result.Sources = append(result.Sources, sq.Source)
		}
	}

	return result, nil
}

// ============================================================
// 阶段2: 并行搜索
// ============================================================

// parallelSearch 并行搜索所有子问题 + 记忆检索
func (d *DebateSystem) parallelSearch() error {
	d.state.CurrentPhase = PhaseSearch

	searchAvailable := d.search.IsAvailable()
	memoryAvailable := d.memory.IsAvailable()

	if !searchAvailable && !memoryAvailable {
		return fmt.Errorf("搜索和记忆均不可用")
	}

	var wg sync.WaitGroup

	// 并行搜索每个子问题
	for i := range d.state.SubQuestions {
		if d.multiAngle {
			// 多角度搜索：为每个子问题生成角度变体
			angles := []struct {
				suffix, label string
			}{
				{"", "原词"},
				{" 优势 机遇 积极发展", "积极角度"},
				{" 风险 挑战 局限 问题", "审慎角度"},
			}
			for _, angle := range angles {
				wg.Add(1)
				go func(idx int, querySuffix, sourceLabel string) {
					defer wg.Done()
					sq := &d.state.SubQuestions[idx]

					if !searchAvailable {
						sq.SearchResult = "网络搜索不可用"
						sq.Source = "无来源（搜索不可用）"
						return
					}

					result, err := d.search.SearchAndCompress(sq.SearchQuery+querySuffix, SearchResultMaxChars, d.llm)
					if err != nil {
						logger.Warn("Learner", "子问题[%d][%s]搜索失败: %v", idx, sourceLabel, err)
						return // 多角度搜索失败不覆盖已有结果
					}

					sq.SearchResult += "\n\n[" + sourceLabel + "]\n" + result
					sq.Source = "网络搜索(多角度)"
					logger.Info("Learner", "子问题[%d][%s]搜索完成: %d 字符", idx, sourceLabel, len([]rune(result)))
				}(i, angle.suffix, angle.label)
			}
		} else {
			// 单角度搜索（原有逻辑）
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				sq := &d.state.SubQuestions[idx]

				if !searchAvailable {
					sq.SearchResult = "网络搜索不可用"
					sq.Source = "无来源（搜索不可用）"
					return
				}

				result, err := d.search.SearchAndCompress(sq.SearchQuery, SearchResultMaxChars, d.llm)
				if err != nil {
					logger.Warn("Learner", "子问题[%d]搜索失败: %v", idx, err)
					sq.SearchResult = fmt.Sprintf("搜索失败: %v", err)
					sq.Source = "搜索失败"
					return
				}

				sq.SearchResult = result
				sq.Source = "网络搜索"
				logger.Info("Learner", "子问题[%d]搜索完成: %d 字符", idx, len([]rune(result)))
			}(i)
		}
	}

	// 记忆检索
	wg.Add(1)
	go func() {
		defer wg.Done()
		if !memoryAvailable {
			d.state.MemoryResults = nil
			return
		}

		results, err := d.memory.Query(d.state.OriginalQuery, MemoryQueryTopK)
		if err != nil {
			logger.Warn("Learner", "记忆检索失败: %v", err)
			d.state.MemoryResults = nil
			return
		}

		d.state.MemoryResults = results
		logger.Info("Learner", "记忆检索完成: %d 条结果", len(results))
	}()

	wg.Wait()

	// 检查是否有任何有效数据
	hasSearchResult := false
	for _, sq := range d.state.SubQuestions {
		if sq.SearchResult != "" && sq.SearchResult != "网络搜索不可用" && !strings.HasPrefix(sq.SearchResult, "搜索失败") {
			hasSearchResult = true
			break
		}
	}

	if !hasSearchResult && len(d.state.MemoryResults) == 0 {
		return fmt.Errorf("未能检索到任何有效记忆")
	}

	return nil
}

// ============================================================
// 阶段3: 辩论循环
// ============================================================

// runDebate 运行辩论循环
// 每个子问题独立辩论，辩论结果摘要压缩后供后续使用
func (d *DebateSystem) runDebate() error {
	d.state.CurrentPhase = PhaseDebate

	// 如果没有搜索结果也没有记忆数据，跳过辩论
	hasData := len(d.state.MemoryResults) > 0
	for _, sq := range d.state.SubQuestions {
		if sq.SearchResult != "" && !strings.HasPrefix(sq.SearchResult, "搜索失败") && sq.SearchResult != "网络搜索不可用" {
			hasData = true
			break
		}
	}
	if !hasData {
		logger.Warn("Learner", "无有效数据，跳过辩论")
		return nil
	}

	// 对每个子问题进行独立辩论
	var allDebateSummaries []string

	for i, sq := range d.state.SubQuestions {
		logger.Info("Learner", "辩论子问题[%d]: %s", i, sq.Question)

		memForSubQ := d.filterMemoryForSubQuestion(sq)
		rounds, err := d.debateSubQuestion(sq, memForSubQ)
		if err != nil {
			logger.Warn("Learner", "子问题[%d]辩论失败: %v", i, err)
			continue
		}

		d.state.Rounds = append(d.state.Rounds, rounds...)

		// 生成该子问题辩论的综合摘要
		subSummary := d.summarizeSubQuestionDebate(sq, rounds)
		allDebateSummaries = append(allDebateSummaries, subSummary)
	}

	// 设置收敛状态
	d.state.Converged = len(d.state.Rounds) > 0

	return nil
}
