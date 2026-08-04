//go:build ignore

package websearch

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"
)

// Assembly 是深度搜索的顶层编排者，调用 DepthSearcher 采集数据后组织辩论

// 辩论上下文预算（现代LLM普遍支持128K上下文，预算可适当放宽）
const (
	debateResearchBudget = 25000 // 研究数据注入辩论的字符数上限
	debateWebBudget      = 20000 // 网络搜索结果预算
	debateMemBudget      = 5000  // 记忆信息预算
	debateSpeechMaxLen   = 2000  // 单次发言最大字符数（用于历史摘要）
	debatePromptMaxLen   = 45000 // 单次LLM调用的user prompt硬上限
)

// NewAssembly 创建大会辩论系统
func NewAssembly(depth *DepthSearcher, webpageSearcher *WebpageSearcher, llmProvider Provider, memProvider MemoryProvider, cfg DepthConfig) *Assembly {
	// 创建辩论日志文件
	logPath := fmt.Sprintf("log/debate/%s.log", time.Now().Format("2006-01-02"))
	os.MkdirAll("log/debate", 0755)
	logFile, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		logFile = nil
	}

	return &Assembly{
		depth:           depth,
		webpageSearcher: webpageSearcher,
		llmProvider:     llmProvider,
		memProvider:     memProvider,
		cfg:             cfg,
		logFile:         logFile,
	}
}

// debateLog 写入辩论日志文件
func (a *Assembly) debateLog(format string, args ...interface{}) {
	a.logMu.Lock()
	defer a.logMu.Unlock()

	if a.logFile != nil {
		timestamp := time.Now().Format("2006-01-02 15:04:05.000")
		message := fmt.Sprintf(format, args...)
		fmt.Fprintf(a.logFile, "[%s] %s\n", timestamp, message)
		if f, ok := a.logFile.(*os.File); ok {
			f.Sync()
		}
	}
}

// Search 执行大会辩论式深度搜索
func (a *Assembly) Search(ctx context.Context, query string) (string, error) {
	if a.debugLog != nil {
		a.debugLog("[大会辩论] 开始 query=%q", query)
	}

	// 记录辩论开始
	a.debateLog("═══════════════════════════════════════════════════════════════")
	a.debateLog("🎯 大会辩论开始")
	a.debateLog("📅 时间: %s", time.Now().Format("2006-01-02 15:04:05"))
	a.debateLog("🔍 查询: %s", query)
	a.debateLog("⚙️ 配置: 最大轮次=%d, 最大子问题=%d, 补充搜索次数=%d",
		a.cfg.MaxRounds, a.cfg.MaxSubQueries, a.cfg.MaxSupplementarySearches)
	a.debateLog("═══════════════════════════════════════════════════════════════")

	// 检查上下文是否已取消
	if ctx.Err() != nil {
		a.debateLog("❌ 上下文已取消，辩论中止")
		return "", ctx.Err()
	}

	if a.llmProvider == nil {
		if a.debugLog != nil {
			a.debugLog("[大会辩论] LLM Provider为nil，降级到深度搜索")
		}
		return a.fallbackSearch(ctx, query)
	}

	state := &AssemblyState{
		OriginalQuery: query,
	}

	// 阶段1: 深度搜索采集研究数据
	if ctx.Err() != nil {
		a.debateLog("❌ 上下文已取消，研究数据采集中止")
		return "", ctx.Err()
	}
	researchData, err := a.depth.CollectData(ctx, query)
	if err != nil {
		if a.debugLog != nil {
			a.debugLog("[大会辩论] 研究数据采集失败 query=%q err=%v", query, err)
		}
		return a.fallbackSearch(ctx, query)
	}
	state.ResearchData = researchData

	// 检查研究数据是否为空
	if !a.hasResearchData(researchData) {
		if a.debugLog != nil {
			a.debugLog("[大会辩论] 研究数据为空 query=%q", query)
		}
		return fmt.Sprintf("# 深度研究报告\n\n原始问题：%s\n\n## 核心结论\n\n抱歉，网络搜索和记忆库均未找到与「%s」相关的信息。\n\n建议：\n- 尝试更换搜索关键词\n- 使用简易搜索或常规搜索模式\n- 提供更多上下文信息以便更精准地搜索", query, query), nil
	}

	if a.debugLog != nil {
		totalResults := 0
		for _, sq := range researchData.SubQueries {
			totalResults += len(sq.Results)
		}
		a.debugLog("[大会辩论] 研究数据采集完成 子问题数=%d 总结果数=%d", len(researchData.SubQueries), totalResults)
	}

	// 记录研究数据详情
	totalResults := 0
	a.debateLog("📊 研究数据采集完成")
	for i, sq := range researchData.SubQueries {
		a.debateLog("  子问题%d: %s (%d条结果)", i+1, sq.Query, len(sq.Results))
		// 记录每条搜索结果的标题和来源
		for j, r := range sq.Results {
			title := r.Title
			if len([]rune(title)) > 80 {
				title = string([]rune(title)[:77]) + "..."
			}
			url := r.URL
			if len([]rune(url)) > 100 {
				url = string([]rune(url)[:97]) + "..."
			}
			a.debateLog("    ├─ [%d] %s", j+1, title)
			if url != "" {
				a.debateLog("    │   来源: %s", url)
			}
			// 如果是记忆库信息，记录内容摘要
			if r.Title == "记忆库信息" || r.URL == "" {
				snippet := r.Snippet
				if len([]rune(snippet)) > 200 {
					snippet = string([]rune(snippet)[:197]) + "..."
				}
				a.debateLog("    │   内容: %s", snippet)
			}
		}
		totalResults += len(sq.Results)
	}
	a.debateLog("  总结果数: %d", totalResults)

	// 补充记忆检索数据（仅当结果有意义时注入）
	if a.memProvider != nil {
		memResult, memErr := a.memProvider.Query(query)
		if memErr != nil {
			if a.debugLog != nil {
				a.debugLog("[大会辩论] 记忆检索失败 query=%q err=%v", query, memErr)
			}
			a.debateLog("📚 记忆检索失败: %v", memErr)
		} else if memResult != "" && !isMemoryEmpty(memResult) {
			if a.debugLog != nil {
				a.debugLog("[大会辩论] 记忆检索成功 query=%q 结果长度=%d", query, len([]rune(memResult)))
			}
			a.debateLog("📚 记忆检索成功，结果长度=%d", len([]rune(memResult)))
			// 记录记忆检索结果内容
			memContent := memResult
			if len([]rune(memContent)) > 500 {
				memContent = string([]rune(memContent)[:497]) + "..."
			}
			a.debateLog("    内容: %s", memContent)
			state.ResearchData = a.appendMemoryData(researchData, memResult)
		} else if a.debugLog != nil {
			a.debugLog("[大会辩论] 记忆检索无结果 query=%q", query)
		}
	}

	// 阶段2: 串行辩论回合
	a.debateLog("")
	a.debateLog("───────────────────────────────────────────────────────────────")
	a.debateLog("🗣️ 开始辩论回合")
	a.debateLog("───────────────────────────────────────────────────────────────")
	a.phaseDebate(ctx, state)

	if a.debugLog != nil {
		a.debugLog("[大会辩论] 辩论结束 总轮次=%d 收敛=%v 补充搜索次数=%d",
			len(state.Rounds), state.Converged, state.SupplementarySearchCount)
	}

	// 记录辩论结束
	a.debateLog("")
	a.debateLog("───────────────────────────────────────────────────────────────")
	a.debateLog("🏁 辩论结束")
	a.debateLog("📊 统计: 总轮次=%d, 收敛=%v, 补充搜索次数=%d",
		len(state.Rounds), state.Converged, state.SupplementarySearchCount)
	a.debateLog("───────────────────────────────────────────────────────────────")

	// 阶段3: 综合报告
	a.debateLog("")
	a.debateLog("📝 开始生成综合报告...")
	result, err := a.phaseSynthesize(ctx, state)
	if err != nil {
		a.debateLog("❌ 报告生成失败: %v", err)
	} else {
		a.debateLog("✅ 报告生成成功，长度=%d", len([]rune(result)))
		// 记录完整报告内容
		a.debateLog("")
		a.debateLog("───────────────────────────────────────────────────────────────")
		a.debateLog("📄 综合报告内容")
		a.debateLog("───────────────────────────────────────────────────────────────")
		a.debateLog("%s", result)
		a.debateLog("───────────────────────────────────────────────────────────────")
	}
	a.debateLog("")
	a.debateLog("═══════════════════════════════════════════════════════════════")
	a.debateLog("                          辩论结束")
	a.debateLog("═══════════════════════════════════════════════════════════════")

	return result, err
}

// hasResearchData 检查研究数据是否为空
func (a *Assembly) hasResearchData(data *ResearchData) bool {
	if data == nil || len(data.SubQueries) == 0 {
		return false
	}
	for _, sq := range data.SubQueries {
		if len(sq.Results) > 0 {
			return true
		}
	}
	return false
}

// isMemoryEmpty 判断记忆检索结果是否为空/无意义
func isMemoryEmpty(result string) bool {
	emptyMarkers := []string{
		"未找到相关", "没有找到", "无相关", "暂无",
		"记忆库为空", "记忆库中没有", "no results",
	}
	for _, m := range emptyMarkers {
		if strings.Contains(strings.ToLower(result), strings.ToLower(m)) {
			return true
		}
	}
	return len(strings.TrimSpace(result)) < 20
}

// isMemoryResultRelevant 判断记忆查询结果是否与搜索查询相关
// 使用项目中已有的关键词提取和相关性检查逻辑
func isMemoryResultRelevant(query string, result string) bool {
	// 使用项目中已有关键词提取函数（支持中文2-gram）
	keywords := extractQueryKeywords(query)

	// 如果没有提取到关键词，使用宽松匹配：检查查询中任意长度>3的子串是否在结果中
	if len(keywords) == 0 {
		queryRunes := []rune(query)
		if len(queryRunes) < 3 {
			return false
		}
		resultLower := strings.ToLower(result)
		// 检查完整查询是否部分匹配（取前10个字）
		checkLen := min(len(queryRunes), 10)
		checkStr := strings.ToLower(string(queryRunes[:checkLen]))
		return strings.Contains(resultLower, checkStr)
	}

	// 使用项目中已有的相关性检查函数
	score := checkContentRelevance(keywords, result, "", "", "", false)
	return score >= 1
}

// appendMemoryData 将记忆检索结果追加到研究数据中
func (a *Assembly) appendMemoryData(data *ResearchData, memResult string) *ResearchData {
	// 创建副本，追加记忆数据到第一个子问题
	copied := &ResearchData{
		OriginalQuery: data.OriginalQuery,
		SubQueries:    make([]SubQueryResult, len(data.SubQueries)),
	}
	for i, sq := range data.SubQueries {
		copied.SubQueries[i] = sq
	}
	if len(copied.SubQueries) > 0 {
		// 追加记忆数据作为额外的搜索结果
		memItem := SearchResult{
			Title:   "记忆库信息",
			URL:     "",
			Snippet: memResult,
		}
		copied.SubQueries[0].Results = append(copied.SubQueries[0].Results, memItem)
	}
	return copied
}

// formatResearchDataForDebate 完整视图，供反对者和整合者使用
func (a *Assembly) formatResearchDataForDebate(data *ResearchData) string {
	return a.formatResearchDataInternal(data, false, false)
}

// formatResearchDataForOptimist 维新派视角：网络搜索结果优先，强调最新信息和积极趋势
func (a *Assembly) formatResearchDataForOptimist(data *ResearchData) string {
	return a.formatResearchDataInternal(data, true, false)
}

// formatResearchDataForSkeptic 守旧派视角：记忆信息优先，网络结果作为补充验证
func (a *Assembly) formatResearchDataForSkeptic(data *ResearchData) string {
	return a.formatResearchDataInternal(data, false, true)
}

// formatResearchDataInternal 内部格式化方法，带预算控制
// webFirst: 网络结果优先，记忆信息靠后
// memFirst: 记忆信息优先，网络结果靠后
func (a *Assembly) formatResearchDataInternal(data *ResearchData, webFirst bool, memFirst bool) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("=== 研究数据（原始问题：%s） ===\n\n", data.OriginalQuery))

	// 收集记忆信息和网络结果
	var memItems []SearchResult
	var webItems []SearchResult

	for _, sq := range data.SubQueries {
		for _, r := range sq.Results {
			if r.Title == "记忆库信息" || r.URL == "" {
				memItems = append(memItems, r)
			} else {
				webItems = append(webItems, r)
			}
		}
	}

	sb.WriteString("## 📋 研究简报\n\n")
	sb.WriteString(fmt.Sprintf("共获得 %d 条网络结果", len(webItems)))
	if len(memItems) > 0 {
		sb.WriteString(fmt.Sprintf(" + %d 条记忆信息", len(memItems)))
	}
	sb.WriteString("\n\n---\n\n")

	// 按预算分配：记忆信息 5000 字，网络结果 20000 字
	memBudget := debateMemBudget
	webBudget := debateWebBudget

	// 记忆信息区块
	if memFirst && len(memItems) > 0 {
		sb.WriteString("## 📚 记忆库信息（优先参考）\n\n")
		a.writeMemItems(&sb, memItems, memBudget)
	}

	// 网络搜索结果区块（统一列表，不按搜索维度分组）
	if len(webItems) > 0 {
		if webFirst {
			sb.WriteString("## 🌐 网络搜索结果（优先参考）\n\n")
		} else if memFirst {
			sb.WriteString("## 🌐 网络搜索结果（补充验证）\n\n")
		} else {
			sb.WriteString("## 🌐 网络搜索结果\n\n")
		}

		a.writeWebItemsUnified(&sb, webItems, webBudget)
	}

	// 记忆信息（非优先时放在最后）
	if !memFirst && len(memItems) > 0 {
		sb.WriteString("## 📚 记忆库信息\n\n")
		a.writeMemItems(&sb, memItems, memBudget)
	}

	return sb.String()
}

// writeMemItems 写入记忆信息，受预算控制
func (a *Assembly) writeMemItems(sb *strings.Builder, items []SearchResult, budget int) {
	used := 0
	for _, m := range items {
		content := m.Snippet
		if len([]rune(content)) > 1500 {
			content = string([]rune(content)[:1500]) + "..."
		}
		line := fmt.Sprintf("记忆内容: %s\n\n", content)
		lineLen := len([]rune(line))
		if used+lineLen > budget {
			sb.WriteString("[记忆信息已按预算截断]\n")
			return
		}
		sb.WriteString(line)
		used += lineLen
	}
}

// writeWebItemsUnified 写入网络搜索结果（统一列表，不按搜索维度分组）
func (a *Assembly) writeWebItemsUnified(sb *strings.Builder, items []SearchResult, budget int) {
	used := 0
	for _, r := range items {
		content := r.Snippet
		if len([]rune(content)) > 1200 {
			content = string([]rune(content)[:1200]) + "..."
		}
		entry := fmt.Sprintf("- **%s**\n  URL: %s\n  %s\n\n", r.Title, r.URL, content)
		entryLen := len([]rune(entry))
		if used+entryLen > budget {
			sb.WriteString("[网络搜索结果已按预算截断]\n")
			return
		}
		sb.WriteString(entry)
		used += entryLen
	}
}

// phaseDebate 串行辩论回合
func (a *Assembly) phaseDebate(ctx context.Context, state *AssemblyState) {
	maxRounds := a.cfg.MaxRounds
	if maxRounds <= 0 {
		maxRounds = 5
	}
	if maxRounds > 10 {
		maxRounds = 10
	}

	maxSuppSearches := a.cfg.MaxSupplementarySearches
	if maxSuppSearches <= 0 {
		maxSuppSearches = 3
	}

	for round := 1; round <= maxRounds; round++ {
		// 检查上下文是否已取消
		if ctx.Err() != nil {
			a.debateLog("❌ 上下文已取消，辩论中止（第%d轮）", round)
			return
		}

		state.CurrentRound = round

		if a.debugLog != nil {
			a.debugLog("[大会辩论] === 第%d/%d轮辩论开始 ===", round, maxRounds)
		}

		// 记录本轮辩论开始
		a.debateLog("")
		a.debateLog("╔═══════════════════════════════════════════════════════════════")
		a.debateLog("║ 第%d/%d轮辩论开始", round, maxRounds)
		a.debateLog("╚═══════════════════════════════════════════════════════════════")

		debateRound := a.executeDebateRound(ctx, state)
		state.Rounds = append(state.Rounds, debateRound)

		// 记录本轮辩论结束
		a.debateLog("")
		a.debateLog("┌───────────────────────────────────────────────────────────────")
		a.debateLog("│ 第%d轮辩论结束", round)
		a.debateLog("│ 收敛判定: %v", debateRound.Converged)
		a.debateLog("└───────────────────────────────────────────────────────────────")

		// 处理整合者的补充搜索请求
		if !debateRound.Converged && state.SupplementarySearchCount < maxSuppSearches {
			a.handleSupplementarySearch(ctx, state, &debateRound)
		}

		// 收敛判定
		if a.checkConvergence(state, debateRound) {
			state.Converged = true
			a.debateLog("")
			a.debateLog("🔔 辩论收敛，提前结束")
			break
		}
	}
}

// handleSupplementarySearch 解析整合者输出的【仍需信息】并执行补充搜索
func (a *Assembly) handleSupplementarySearch(ctx context.Context, state *AssemblyState, round *DebateRound) {
	if ctx.Err() != nil {
		return
	}

	// 从整合者输出中提取【仍需信息】段落
	missingInfo := extractMissingInfo(round.Synthesizer)
	if missingInfo == "" {
		if a.debugLog != nil {
			a.debugLog("[大会辩论] 整合者未列出仍需信息，跳过补充搜索")
		}
		return
	}

	if a.debugLog != nil {
		a.debugLog("[大会辩论] 整合者列出仍需信息，将执行补充搜索 missing=%q", truncateToRunes(missingInfo, 200))
	}

	// 记录补充搜索开始
	a.debateLog("")
	a.debateLog("🔍 补充搜索 (第%d次)", state.SupplementarySearchCount+1)
	a.debateLog("   缺失信息: %s", missingInfo)

	// 从缺失信息列表中提取第一条作为搜索查询
	searchQuery := extractFirstItemFromList(missingInfo)
	if searchQuery == "" {
		searchQuery = truncateToRunes(missingInfo, 100)
	}
	a.debateLog("   搜索查询: %s", searchQuery)
	state.SupplementarySearchCount++

	// 执行补充网络搜索
	var newResults []SearchResult
	webResults, err := a.supplementaryWebSearch(ctx, searchQuery)
	if err != nil {
		if a.debugLog != nil {
			a.debugLog("[大会辩论] 补充网络搜索失败 query=%q err=%v", searchQuery, err)
		}
		a.debateLog("   ❌ 网络搜索失败: %v", err)
	} else if len(webResults) > 0 {
		if a.debugLog != nil {
			a.debugLog("[大会辩论] 补充网络搜索完成 query=%q 结果数=%d", searchQuery, len(webResults))
		}
		a.debateLog("   ✅ 网络搜索完成，结果数=%d", len(webResults))
		// 记录每条网络搜索结果的详细信息
		for j, r := range webResults {
			title := r.Title
			if len([]rune(title)) > 80 {
				title = string([]rune(title)[:77]) + "..."
			}
			url := r.URL
			if len([]rune(url)) > 100 {
				url = string([]rune(url)[:97]) + "..."
			}
			a.debateLog("    ├─ [%d] %s", j+1, title)
			if url != "" {
				a.debateLog("    │   来源: %s", url)
			}
			if r.Snippet != "" {
				snippet := r.Snippet
				if len([]rune(snippet)) > 150 {
					snippet = string([]rune(snippet)[:147]) + "..."
				}
				a.debateLog("    │   摘要: %s", snippet)
			}
		}
		newResults = append(newResults, webResults...)
	} else {
		a.debateLog("   ⚠️ 网络搜索无有效结果")
	}

	// 执行补充记忆查询
	if a.memProvider != nil {
		memResult, err := a.memProvider.Query(searchQuery)
		if err != nil {
			if a.debugLog != nil {
				a.debugLog("[大会辩论] 补充记忆查询失败 query=%q err=%v", searchQuery, err)
			}
			a.debateLog("   ❌ 记忆查询失败: %v", err)
		} else if memResult != "" && !isMemoryEmpty(memResult) {
			if a.debugLog != nil {
				a.debugLog("[大会辩论] 补充记忆查询完成 query=%q 结果长度=%d", searchQuery, len([]rune(memResult)))
			}
			// 检查记忆结果是否与搜索查询相关（避免注入不相关的记忆信息）
			if !isMemoryResultRelevant(searchQuery, memResult) {
				a.debateLog("   ⚠️ 记忆查询结果与搜索查询不相关，已跳过")
				if a.debugLog != nil {
					a.debugLog("[大会辩论] 补充记忆查询结果不相关，已跳过 query=%q", searchQuery)
				}
			} else {
				a.debateLog("   ✅ 记忆查询完成，结果长度=%d", len([]rune(memResult)))
				// 记录记忆查询结果内容
				memContent := memResult
				if len([]rune(memContent)) > 300 {
					memContent = string([]rune(memContent)[:297]) + "..."
				}
				a.debateLog("    内容: %s", memContent)
				newResults = append(newResults, SearchResult{
					Title:   fmt.Sprintf("补充记忆：%s", truncateToRunes(missingInfo, 80)),
					URL:     "",
					Snippet: memResult,
				})
			}
		} else {
			a.debateLog("   ⚠️ 记忆查询无有效结果")
		}
	}

	if len(newResults) == 0 {
		if a.debugLog != nil {
			a.debugLog("[大会辩论] 补充搜索未获得有效结果")
		}
		a.debateLog("   ⚠️ 补充搜索未获得有效结果")
		return
	}

	// 注入新的研究数据
	state.ResearchData = a.appendSupplementaryData(state.ResearchData, searchQuery, newResults)
	if a.debugLog != nil {
		a.debugLog("[大会辩论] 补充搜索数据已注入 新增结果=%d 累计补充次数=%d",
			len(newResults), state.SupplementarySearchCount)
	}
	a.debateLog("   ✅ 补充数据已注入，新增结果=%d", len(newResults))
}

// extractMissingInfo 从整合者输出中提取【仍需信息】段落内容
// 返回空字符串表示无需补充搜索
func extractMissingInfo(synthesizerOutput string) string {
	// 查找【仍需信息】标记
	markers := []string{"【仍需信息】", "[仍需信息]", "仍需信息：", "仍需信息:"}
	var startIdx int = -1
	for _, m := range markers {
		if idx := strings.Index(synthesizerOutput, m); idx >= 0 {
			startIdx = idx + len(m)
			break
		}
	}
	if startIdx < 0 {
		return ""
	}

	// 提取到输出末尾或下一个【】标记
	text := strings.TrimSpace(synthesizerOutput[startIdx:])
	// 截断到下一个章节标记
	if endIdx := strings.Index(text, "【"); endIdx > 0 {
		text = strings.TrimSpace(text[:endIdx])
	}

	// 过滤空内容：纯"无"、"暂无"、"不需要"等视为无需搜索
	text = strings.TrimSpace(text)
	emptyMarkers := []string{"无", "暂无", "不需要", "无缺失", "无需", "不需要补充", "无补充"}
	for _, m := range emptyMarkers {
		if text == m || strings.HasPrefix(text, m) {
			return ""
		}
	}

	// 内容太短也视为无意义
	if len([]rune(text)) < 4 {
		return ""
	}

	return text
}

// extractFirstItemFromList 从列表格式文本中提取第一条内容
func extractFirstItemFromList(text string) string {
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// 移除列表编号前缀和 Markdown 粗体标记
		line = strings.TrimLeft(line, "0123456789.")
		line = strings.TrimLeft(line, ")")
		line = strings.TrimLeft(line, "-•")
		line = strings.TrimSpace(line)
		line = strings.Trim(line, "*")
		line = strings.TrimSpace(line)
		if len(line) > 4 {
			// 提取核心关键词，限制长度
			cutIdx := strings.IndexAny(line, "（(")
			if cutIdx > 10 {
				line = line[:cutIdx]
			}
			cutIdx = strings.IndexAny(line, "，,")
			if cutIdx > 15 {
				line = line[:cutIdx]
			}
			line = cleanSearchQuery(line)
			if len([]rune(line)) > 40 {
				line = string([]rune(line)[:40])
			}
			return strings.TrimSpace(line)
		}
	}
	return ""
}

// cleanSearchQuery 清理搜索查询中的特殊字符，避免干扰搜索引擎解析
func cleanSearchQuery(query string) string {
	// 移除各种引号和括号类字符
	var builder strings.Builder
	builder.Grow(len(query))
	for _, r := range query {
		switch r {
		case 0x201C, 0x201D, // 中文左右双引号 “ ”
			0x2018, 0x2019, // 中文左右单引号 ‘ ’
			0x0022, 0x0027, // 英文双引号、单引号
			0x300A, 0x300B, // 书名号 《 》
			0x3010, 0x3011, // 方头括号 【 】
			0x300C, 0x300D, // 直角引号 「 」
			0x300E, 0x300F: // 弯引号 『 』
			continue
		case 0x2014, 0x2013: // em dash —, en dash –
			builder.WriteRune(' ')
		default:
			builder.WriteRune(r)
		}
	}
	result := builder.String()

	// 将连续的破折号、空格替换为单个空格
	result = strings.ReplaceAll(result, " - ", " ")
	for strings.Contains(result, "  ") {
		result = strings.ReplaceAll(result, "  ", " ")
	}

	return strings.TrimSpace(result)
}

// supplementaryWebSearch 执行补充网络搜索
// 策略：简单搜索 + 选择性抓取前2条正文
func (a *Assembly) supplementaryWebSearch(ctx context.Context, query string) ([]SearchResult, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	// 优先使用选择性抓取
	if a.webpageSearcher != nil {
		results, err := a.webpageSearcher.SearchRawWithSelectiveFetch(query, 2)
		if err != nil {
			if a.debugLog != nil {
				a.debugLog("[大会辩论] 补充网络搜索(选择性抓取)失败 query=%q err=%v", query, err)
			}
		} else if len(results) > 0 {
			if a.debugLog != nil {
				a.debugLog("[大会辩论] 补充网络搜索(选择性抓取)完成 query=%q 结果数=%d", query, len(results))
			}
			return results, nil
		}
	}

	// 降级1：纯简单搜索（只返回标题+摘要）
	if a.depth != nil && a.depth.simple != nil {
		results, err := a.depth.simple.SearchRaw(query)
		if err != nil {
			if a.debugLog != nil {
				a.debugLog("[大会辩论] 补充网络搜索(simple)失败 query=%q err=%v", query, err)
			}
			return nil, err
		}
		if len(results) > 0 {
			if a.debugLog != nil {
				a.debugLog("[大会辩论] 补充网络搜索(simple)完成 query=%q 结果数=%d", query, len(results))
			}
			return results, nil
		}
	}

	// 降级2：使用深度搜索器的简易搜索
	if a.debugLog != nil {
		a.debugLog("[大会辩论] 选择性抓取和simple都无结果，降级使用深度搜索器简易搜索 query=%q", query)
	}
	return a.depth.SupplementarySearch(query)
}

// appendSupplementaryData 将补充搜索结果追加到研究数据中
func (a *Assembly) appendSupplementaryData(data *ResearchData, query string, results []SearchResult) *ResearchData {
	if len(results) == 0 {
		return data
	}
	copied := &ResearchData{
		OriginalQuery: data.OriginalQuery,
		SubQueries:    make([]SubQueryResult, len(data.SubQueries)+1),
	}
	for i, sq := range data.SubQueries {
		copied.SubQueries[i] = sq
	}
	copied.SubQueries[len(data.SubQueries)] = SubQueryResult{
		Query:   fmt.Sprintf("补充搜索：%s", query),
		Results: results,
	}
	return copied
}

// executeDebateRound 执行一轮串行辩论
func (a *Assembly) executeDebateRound(ctx context.Context, state *AssemblyState) DebateRound {
	round := DebateRound{Round: state.CurrentRound}

	// 检查上下文是否已取消
	if ctx.Err() != nil {
		round.Converged = true
		return round
	}

	// 构建本轮之前的辩论历史（完整，不截断）
	previousHistory := a.buildDebateHistory(state)

	// 步骤1: 乐观派发言（看到：网络搜索结果 + 历史辩论）
	optimistData := a.formatResearchDataForOptimist(state.ResearchData)
	optimistContext := a.buildRoundContext(state.OriginalQuery, optimistData, previousHistory, "", "", "")
	round.Optimist = a.callDelegate(ctx, RoleOptimist, optimistSystemPrompt(), optimistContext)

	// 步骤2: 审慎派发言（看到：记忆信息 + 网络搜索结果 + 历史辩论 + 本轮乐观派发言）
	skepticData := a.formatResearchDataForSkeptic(state.ResearchData)
	skepticContext := a.buildRoundContext(state.OriginalQuery, skepticData, previousHistory, round.Optimist, "", "")
	round.Skeptic = a.callDelegate(ctx, RoleSkeptic, skepticSystemPrompt(), skepticContext)

	// 步骤3: 反对者挑刺（看到：完整研究数据 + 历史辩论 + 本轮乐观派 + 审慎派）
	criticData := a.formatResearchDataForDebate(state.ResearchData)
	criticContext := a.buildRoundContext(state.OriginalQuery, criticData, previousHistory, round.Optimist, round.Skeptic, "")
	round.Critic = a.callDelegate(ctx, RoleCritic, criticSystemPrompt(), criticContext)

	// 步骤4: 整合者判断收敛（看到：完整研究数据 + 历史辩论 + 本轮三方发言）
	synthesizerContext := a.buildRoundContext(state.OriginalQuery, criticData, previousHistory, round.Optimist, round.Skeptic, round.Critic)
	round.Synthesizer = a.callDelegate(ctx, RoleSynthesizer, synthesizerSystemPrompt(), synthesizerContext)

	// 从整合者输出中预判收敛（用于后续补充搜索决策）
	round.Converged = strings.Contains(round.Synthesizer, "CONVERGED")

	return round
}

// buildRoundContext 构建辩论回合上下文
func (a *Assembly) buildRoundContext(query string, researchText string, previousHistory string, optimistSpeech string, skepticSpeech string, criticSpeech string) string {
	var sb strings.Builder

	// 讨论主题 — 最显眼的位置，确保每个代表都知道在讨论什么
	sb.WriteString("━━━━━━━━━━━━━━━━━━━━━━━━\n")
	sb.WriteString(fmt.Sprintf("📌 讨论主题：%s\n", query))
	sb.WriteString("━━━━━━━━━━━━━━━━━━━━━━━━\n\n")

	sb.WriteString(researchText)
	sb.WriteString("\n\n")

	if previousHistory != "" {
		sb.WriteString("=== 辩论历史 ===\n")
		sb.WriteString(previousHistory)
		sb.WriteString("\n")
	}

	sb.WriteString("=== 本轮辩论 ===\n")
	if optimistSpeech != "" {
		sb.WriteString(fmt.Sprintf("维新派发言：%s\n\n", optimistSpeech))
	}
	if skepticSpeech != "" {
		sb.WriteString(fmt.Sprintf("守旧派发言：%s\n\n", skepticSpeech))
	}
	if criticSpeech != "" {
		sb.WriteString(fmt.Sprintf("反对者发言：%s\n\n", criticSpeech))
	}

	return sb.String()
}

// checkConvergence 综合收敛判定
func (a *Assembly) checkConvergence(state *AssemblyState, round DebateRound) bool {
	// 条件1: 整合者明确判定收敛
	if strings.Contains(round.Synthesizer, "CONVERGED") {
		if a.debugLog != nil {
			a.debugLog("[大会辩论] 第%d轮辩论收敛：整合者判定CONVERGED", state.CurrentRound)
		}
		return true
	}

	// 条件2: 反对者连续两轮未提出新问题
	if a.isCriticEmpty(round.Critic) {
		state.CriticEmptyRounds++
		if a.debugLog != nil {
			a.debugLog("[大会辩论] 第%d轮反对者无新问题 连续空轮=%d", state.CurrentRound, state.CriticEmptyRounds)
		}
		if state.CriticEmptyRounds >= 2 {
			if a.debugLog != nil {
				a.debugLog("[大会辩论] 第%d轮辩论收敛：反对者连续%d轮无新问题", state.CurrentRound, state.CriticEmptyRounds)
			}
			return true
		}
	} else {
		state.CriticEmptyRounds = 0
	}

	// 条件3: 如果这是第一轮，不可能收敛（需要至少一轮辩论）
	if state.CurrentRound <= 1 {
		return false
	}

	return false
}

// isCriticEmpty 判断反对者是否未提出实质性新问题
func (a *Assembly) isCriticEmpty(criticSpeech string) bool {
	// 检查反对者发言是否为空或仅为"无问题"等表述
	trimmed := strings.TrimSpace(criticSpeech)
	if trimmed == "" {
		return true
	}
	emptyMarkers := []string{"无新问题", "没有问题", "未发现问题", "没有新问题", "无问题", "没有发现新问题"}
	lower := strings.ToLower(trimmed)
	for _, marker := range emptyMarkers {
		if strings.Contains(lower, strings.ToLower(marker)) {
			return true
		}
	}
	return false
}

// callDelegate 调用一位辩论代表
func (a *Assembly) callDelegate(ctx context.Context, role DelegateRole, systemPrompt string, context string) string {
	roleNames := map[DelegateRole]string{
		RoleOptimist:    "维新派",
		RoleSkeptic:     "守旧派",
		RoleCritic:      "反对者",
		RoleSynthesizer: "整合者",
	}
	roleName := roleNames[role]
	if roleName == "" {
		roleName = string(role)
	}

	// 检查上下文是否已取消
	if ctx.Err() != nil {
		return ""
	}

	roleInstruction := buildRoleInstruction(role)
	userPrompt := context + "\n\n" + roleInstruction

	// 安全截断：总 user prompt 不超过 debatePromptMaxLen 字符
	userPrompt = truncateToRunes(userPrompt, debatePromptMaxLen)

	messages := []ChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}

	response, err := a.llmProvider.Chat(messages)
	if err != nil {
		if a.debugLog != nil {
			a.debugLog("[大会辩论] %s LLM调用失败 err=%v", role, err)
		}
		a.debateLog("   ❌ %s发言失败: %v", roleName, err)
		return fmt.Sprintf("[%s发言失败: %v]", role, err)
	}

	response = strings.TrimSpace(response)
	respLen := len([]rune(response))

	if a.debugLog != nil {
		a.debugLog("[大会辩论] %s 发言完成 长度=%d", role, respLen)
	}

	// 记录完整发言内容到辩论日志
	a.debateLog("")
	a.debateLog("   🗣️ %s发言 (%d字)", roleName, respLen)
	a.debateLog("   ──────────────────────────────────────")
	a.debateLog("   %s", response)
	a.debateLog("   ──────────────────────────────────────")

	return response
}

// buildRoleInstruction 构建角色特定的发言指令
func buildRoleInstruction(role DelegateRole) string {
	switch role {
	case RoleOptimist:
		return "作为维新派，请基于以上研究数据，**优先引用网络搜索结果**（带URL的网页内容），从积极/建设性角度发表你的论点。注意：如果辩论历史中有守旧派或反对者的质疑，请针对性地回应。"
	case RoleSkeptic:
		return "作为守旧派，请基于以上研究数据，**优先引用记忆库信息**，从审慎/批判角度发表你的论点。注意：请针对维新派本轮发言中的观点进行回应，指出其证据不足之处。"
	case RoleCritic:
		return "作为反对者，请同时指出维新派和守旧派本轮论点中的逻辑漏洞、证据链断裂之处，以及双方都未考虑到的问题。"
	case RoleSynthesizer:
		return "作为整合者，请判断本轮辩论是否收敛。如果各方在核心事实上已达成共识、无实质性分歧，请输出 CONVERGED 并附理由。如果仍需继续辩论，请输出 CONTINUE 并在【本轮总结】中总结核心分歧，在【仍需信息】中列出缺失的关键信息方向。"
	default:
		return "请发表你的论点。"
	}
}

// buildDebateHistory 构建辩论历史摘要（用于辩论回合中）
// 压缩策略：最新一轮保留完整发言，旧轮次只保留整合者判断
func (a *Assembly) buildDebateHistory(state *AssemblyState) string {
	if len(state.Rounds) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("原始问题: %s\n\n", state.OriginalQuery))

	lastIdx := len(state.Rounds) - 1

	for i, r := range state.Rounds {
		if i == lastIdx {
			// 最新一轮：保留完整发言
			sb.WriteString(fmt.Sprintf("--- 第%d轮辩论 ---\n", r.Round))
			sb.WriteString(fmt.Sprintf("维新派: %s\n\n", truncateToRunes(r.Optimist, debateSpeechMaxLen)))
			sb.WriteString(fmt.Sprintf("守旧派: %s\n\n", truncateToRunes(r.Skeptic, debateSpeechMaxLen)))
			sb.WriteString(fmt.Sprintf("反对者: %s\n\n", truncateToRunes(r.Critic, debateSpeechMaxLen)))
			sb.WriteString(fmt.Sprintf("整合者: %s\n\n", truncateToRunes(r.Synthesizer, debateSpeechMaxLen)))
		} else {
			// 旧轮次：只保留整合者判断（天然压缩摘要）
			sb.WriteString(fmt.Sprintf("第%d轮摘要: %s\n\n", r.Round,
				truncateToRunes(r.Synthesizer, debateSpeechMaxLen)))
		}
	}

	return sb.String()
}

// truncateToRunes 按 rune 截断字符串
func truncateToRunes(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}

// phaseSynthesize 综合报告阶段
func (a *Assembly) phaseSynthesize(ctx context.Context, state *AssemblyState) (string, error) {
	// 检查上下文是否已取消
	if ctx.Err() != nil {
		return "", ctx.Err()
	}

	debateHistory := a.buildFullDebateHistory(state)
	prompt := buildFinalReportPrompt(state.OriginalQuery, debateHistory)

	messages := []ChatMessage{
		{Role: "system", Content: synthesizerSystemPrompt()},
		{Role: "user", Content: prompt},
	}

	response, err := a.llmProvider.Chat(messages)
	if err != nil {
		return a.fallbackSearch(ctx, state.OriginalQuery)
	}

	// 输出截断保护
	responseRunes := []rune(response)
	if len(responseRunes) > 5000 {
		response = string(responseRunes[:5000]) + "\n\n[报告已截断]"
	}

	return response, nil
}

// buildFullDebateHistory 构建完整辩论历史（用于最终报告，保留完整发言）
func (a *Assembly) buildFullDebateHistory(state *AssemblyState) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("原始问题: %s\n\n", state.OriginalQuery))

	// 辩论过程（完整保留，最终报告是独立LLM调用，上下文充足）
	for _, r := range state.Rounds {
		sb.WriteString(fmt.Sprintf("=== 第%d轮辩论 ===\n", r.Round))
		sb.WriteString(fmt.Sprintf("维新派: %s\n\n", r.Optimist))
		sb.WriteString(fmt.Sprintf("守旧派: %s\n\n", r.Skeptic))
		sb.WriteString(fmt.Sprintf("反对者: %s\n\n", r.Critic))
		sb.WriteString(fmt.Sprintf("整合者判断: %s\n\n", r.Synthesizer))
	}

	return sb.String()
}

// fallbackSearch 降级到深度搜索（无LLM时使用简易搜索）
func (a *Assembly) fallbackSearch(ctx context.Context, query string) (string, error) {
	result, err := a.depth.Search(ctx, query)
	if err != nil {
		return "", err
	}
	return "# 深度搜索（降级）\n\nLLM不可用，使用简易深度搜索：\n\n" + result, nil
}

// filterByKeywords 基于关键词过滤搜索结果，保留相关度 >= 1 的条目
func filterByKeywords(results []SearchResult, keywords []string, maxResults int) []SearchResult {
	filtered := make([]SearchResult, 0, maxResults)
	for _, r := range results {
		if len(filtered) >= maxResults {
			break
		}
		if checkContentRelevance(keywords, "", r.Title, r.Snippet, r.URL, r.IsOfficial) >= 1 {
			filtered = append(filtered, r)
		}
	}
	return filtered
}

// buildAltQueries 为专有名词查询生成替代搜索词（去分隔符变体）
func buildAltQueries(query string) []string {
	withSpace := query
	hasSpecial := false
	for _, old := range []string{"·", "-", "/", "|", "•", "\u2014", "～", "~", "、"} {
		if strings.Contains(withSpace, old) {
			withSpace = strings.ReplaceAll(withSpace, old, " ")
			hasSpecial = true
		}
	}
	noSep := query
	for _, old := range []string{"·", "-", "/", "|", "•", "\u2014", "～", "~", "、"} {
		noSep = strings.ReplaceAll(noSep, old, "")
	}

	if !hasSpecial {
		return nil
	}

	alt := []string{}
	withSpace = strings.TrimSpace(withSpace)
	noSep = strings.TrimSpace(noSep)
	if withSpace != query {
		alt = append(alt, withSpace)
	}
	if noSep != query && noSep != withSpace {
		alt = append(alt, noSep)
	}
	return alt
}
