package lunar_chromedp

import (
	"fmt"
	"logger"
	"strings"
	"time"

	"storage/module"
)

// =============================================================================
// 搜索智能体 — 主控流程编排
// =============================================================================

// =============================================================================
// 公开 API
// =============================================================================

// InitSearch 初始化搜索智能体
// 必须在使用 Search 之前调用，传入多模态模型和嵌入模型的配置
// 初始化失败时返回错误（如模型不可用、配置无效等）
func InitSearch(config SearchConfig) error {
	searchAgentMu.Lock()
	defer searchAgentMu.Unlock()

	if searchAgentInit {
		return fmt.Errorf("搜索智能体已初始化，请勿重复调用 InitSearch")
	}

	// 参数校验
	if strings.TrimSpace(config.MultimodalURL) == "" {
		return fmt.Errorf("多模态模型 URL 不能为空")
	}
	if strings.TrimSpace(config.MultimodalName) == "" {
		return fmt.Errorf("多模态模型名称不能为空")
	}
	if strings.TrimSpace(config.EmbeddingURL) == "" {
		return fmt.Errorf("嵌入模型 URL 不能为空")
	}
	if strings.TrimSpace(config.EmbeddingName) == "" {
		return fmt.Errorf("嵌入模型名称不能为空")
	}
	if config.MaxContextTokens <= 0 {
		config.MaxContextTokens = MaxContextTokensDefault
	}

	// 提前设置全局配置（AI 调用层在连通性测试中需要读取）
	configMutex.Lock()
	activeConfig = &config
	configMutex.Unlock()

	// 验证多模态模型连通性
	if aiCall != nil {
		testPrompt := "请回复 'OK'"
		resp, err := aiCall("你是一个测试助手。", testPrompt, nil)
		if err != nil {
			return fmt.Errorf("多模态模型连通性测试失败 [%s @ %s]: %w",
				config.MultimodalName, config.MultimodalURL, err)
		}
		if !strings.Contains(strings.ToUpper(resp), "OK") {
			return fmt.Errorf("多模态模型响应异常，未返回预期内容")
		}
		fmt.Printf("[%s] 多模态模型连接验证通过: %s\n", ModuleName, config.MultimodalName)
	}

	// 验证嵌入模型连通性 + 初始化 search_memory 集合
	if memoryInitCollection != nil {
		// 初始化 MemoryDB 全局实例（仅首次调用生效，后续调用幂等）
		if config.MemoryDBDir != "" {
			module.InitMemoryDB(config.MemoryDBDir)
		}
		if err := memoryInitCollection(config.EmbeddingURL, config.EmbeddingName, config.EmbeddingKey, config.MultimodalName); err != nil {
			return fmt.Errorf("嵌入模型连通性测试/记忆集合初始化失败 [%s @ %s]: %w",
				config.EmbeddingName, config.EmbeddingURL, err)
		}
		fmt.Printf("[%s] 嵌入模型连接验证通过: %s\n", ModuleName, config.EmbeddingName)
	}

	// 启动浏览器
	if err := LaunchBrowser(); err != nil {
		return fmt.Errorf("浏览器启动失败: %w", err)
	}

	searchAgent = &SearchAgent{
		config: config,
	}
	searchAgentInit = true

	// 设置全局配置（供 AI 调用层读取）
	configMutex.Lock()
	activeConfig = &config
	configMutex.Unlock()

	fmt.Printf("[%s] 搜索智能体初始化完成\n", ModuleName)
	fmt.Printf("[%s]   多模态: %s @ %s\n", ModuleName, config.MultimodalName, config.MultimodalURL)
	fmt.Printf("[%s]   嵌入:   %s @ %s\n", ModuleName, config.EmbeddingName, config.EmbeddingURL)
	fmt.Printf("[%s]   上下文上限: %d tokens\n", ModuleName, config.MaxContextTokens)

	return nil
}

// Search 执行搜索查询（阻塞，串行执行）
// 返回结构化的搜索报告，包含 AI 生成的答案和引用来源
// 调用前必须先调用 InitSearch
func Search(query string) (*SearchReport, error) {
	if !searchAgentInit {
		return nil, fmt.Errorf("搜索智能体未初始化，请先调用 InitSearch")
	}

	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("搜索查询不能为空")
	}

	// 串行执行：获取查询锁
	queryMutex.Lock()
	defer queryMutex.Unlock()

	fmt.Printf("\n[%s] ===== 开始搜索: %s =====\n", ModuleName, query)

	// 浏览器健康检查
	if err := ensureBrowserHealthy(); err != nil {
		return nil, err
	}

	// 执行搜索流水线
	return searchAgent.executeSearchPipeline(query)
}

// =============================================================================
// 搜索流水线
// =============================================================================

// executeSearchPipeline 执行完整的搜索流水线
func (a *SearchAgent) executeSearchPipeline(query string) (*SearchReport, error) {
	// 重置搜索状态
	a.usedKeywords = nil
	a.accumulatedSummaries = nil
	a.accumulatedSources = nil

	// ---- Phase 1: 记忆检索 ----
	memoryResult := a.phaseMemoryLookup(query)
	if memoryResult != nil {
		// 记忆足够且无时效性冲突，直接返回
		return memoryResult, nil
	}

	// ---- Phase 2: 初始搜索 ----
	initialSummaries, initialSources, err := a.phaseInitialSearch(query)
	if err != nil {
		return nil, err
	}

	if len(initialSummaries) == 0 {
		return &SearchReport{
			Query:       query,
			Answer:      "月华找不到你想要的信息",
			FromMemory:  false,
			GeneratedAt: time.Now(),
		}, nil
	}

	a.accumulatedSummaries = append(a.accumulatedSummaries, initialSummaries...)
	a.accumulatedSources = append(a.accumulatedSources, initialSources...)

	// ---- Phase 2.5: 信息充分性评估 ----
	sufficient, _, err := a.evaluateInformation(query)
	if err != nil {
		fmt.Printf("[%s] 信息评估失败: %v，继续深度搜索\n", ModuleName, err)
		sufficient = false
	}

	// ---- Phase 3: 深度搜索 ----
	searchRounds := 1
	if !sufficient {
		fmt.Printf("[%s] 初始搜索信息不足，启动深度搜索（最多 %d 轮）\n", ModuleName, MaxSearchRounds-1)
		deepSummaries, deepSources, deepRounds := a.phaseDeepSearch(query)
		searchRounds += deepRounds

		if len(deepSummaries) > 0 {
			a.accumulatedSummaries = append(a.accumulatedSummaries, deepSummaries...)
			a.accumulatedSources = append(a.accumulatedSources, deepSources...)
		}
	}

	// ---- Phase 4: 报告生成 ----
	report, err := a.phaseGenerateReport(query, searchRounds)
	if err != nil {
		return nil, err
	}

	// ---- Phase 5: 记忆存储 ----
	a.phaseStoreToMemory(query, report)

	return report, nil
}

// =============================================================================
// Phase 1: 记忆检索
// =============================================================================

// phaseMemoryLookup 查询记忆库，判定是否可以直接使用历史答案
// 返回非 nil 表示记忆足够，跳过后续搜索直接返回
func (a *SearchAgent) phaseMemoryLookup(query string) *SearchReport {
	if memoryLookup == nil {
		fmt.Printf("[%s] 记忆库未集成，跳过记忆检索\n", ModuleName)
		return nil
	}

	fmt.Printf("[%s] [阶段 1/5] 记忆检索中...\n", ModuleName)

	entries, err := memoryLookup(query, 5)
	if err != nil {
		fmt.Printf("[%s] 记忆检索失败: %v，继续网络搜索\n", ModuleName, err)
		return nil
	}

	if len(entries) == 0 {
		fmt.Printf("[%s] 记忆库无相关记录\n", ModuleName)
		return nil
	}

	// 过滤低相似度结果
	var relevantEntries []memoryEntry
	for _, e := range entries {
		if e.Similarity >= float32(MemorySimilarityMin) {
			relevantEntries = append(relevantEntries, e)
		}
	}

	if len(relevantEntries) == 0 {
		fmt.Printf("[%s] 记忆库结果相似度过低（< %.0f%%），继续网络搜索\n", ModuleName, MemorySimilarityMin*100)
		return nil
	}

	// 构建记忆上下文
	var memContext strings.Builder
	memContext.WriteString("以下是历史搜索记录：\n\n")
	for i, e := range relevantEntries {
		memContext.WriteString(fmt.Sprintf("记录 %d [相似度: %.0f%%]:\n%s\n\n",
			i+1, e.Similarity*100, e.Content))
	}

	// AI 判定：记忆是否足够
	if aiJudgeMemory == nil {
		fmt.Printf("[%s] AI 判定未集成，跳过记忆评估\n", ModuleName)
		return nil
	}

	sufficient, timeSensitive, err := aiJudgeMemory(memContext.String(), query)
	if err != nil {
		fmt.Printf("[%s] 记忆评估失败: %v，继续网络搜索\n", ModuleName, err)
		return nil
	}

	if timeSensitive {
		fmt.Printf("[%s] 查询具有时效性要求，跳过记忆结果，执行网络搜索\n", ModuleName)
		return nil
	}

	if sufficient {
		fmt.Printf("[%s] 记忆库内容足够回答，跳过网络搜索\n", ModuleName)

		// 从记忆记录中提取答案（取相似度最高的记录的 Content 作为答案）
		bestAnswer := relevantEntries[0].Content
		return &SearchReport{
			Query:       query,
			Answer:      bestAnswer,
			FromMemory:  true,
			GeneratedAt: time.Now(),
		}
	}

	fmt.Printf("[%s] 记忆库内容不足，继续网络搜索\n", ModuleName)
	return nil
}

// =============================================================================
// Phase 2: 初始搜索
// =============================================================================

// phaseInitialSearch 执行初始搜索：生成关键词 → 搜索 → 提取内容 → 摘要
func (a *SearchAgent) phaseInitialSearch(query string) (summaries []string, sources []string, err error) {
	fmt.Printf("[%s] [阶段 2/5] 初始网络搜索...\n", ModuleName)

	// 生成搜索关键词
	keywords, err := a.generateKeywords(query)
	if err != nil {
		return nil, nil, fmt.Errorf("关键词生成失败: %w", err)
	}
	a.usedKeywords = append(a.usedKeywords, keywords...)

	fmt.Printf("[%s] 搜索关键词: %v\n", ModuleName, keywords)

	// 执行搜索并处理结果
	return a.executeSearchAndExtract(keywords)
}

// =============================================================================
// Phase 3: 深度搜索
// =============================================================================

// phaseDeepSearch 执行多轮深度搜索，返回积累的摘要和实际执行轮数
func (a *SearchAgent) phaseDeepSearch(query string) (summaries []string, sources []string, rounds int) {
	for round := 1; round < MaxSearchRounds; round++ {
		fmt.Printf("[%s] [阶段 3/5] 深度搜索 第 %d/%d 轮...\n", ModuleName, round, MaxSearchRounds-1)

		// 生成新的搜索关键词（基于已有摘要，排除已用过的高相似度关键词）
		keywords, err := a.generateDeepKeywords(query)
		if err != nil {
			fmt.Printf("[%s] 深度关键词生成失败: %v，终止深度搜索\n", ModuleName, err)
			break
		}

		if len(keywords) == 0 {
			fmt.Printf("[%s] 无法生成新的搜索角度，终止深度搜索\n", ModuleName)
			break
		}

		fmt.Printf("[%s] 深度搜索关键词: %v\n", ModuleName, keywords)
		a.usedKeywords = append(a.usedKeywords, keywords...)

		// 执行搜索
		roundSummaries, roundSources, err := a.executeSearchAndExtract(keywords)
		if err != nil {
			fmt.Printf("[%s] 深度搜索执行失败: %v\n", ModuleName, err)
			continue
		}

		if len(roundSummaries) == 0 {
			fmt.Printf("[%s] 本轮无有效结果\n", ModuleName)
			rounds++
			continue
		}

		// 筛选有价值的结果（AI 评估每条摘要的相关性）
		valuableSummaries, valuableSources := a.filterValuableResults(query, roundSummaries, roundSources)
		if len(valuableSummaries) == 0 {
			fmt.Printf("[%s] 本轮结果与查询无关，已舍弃\n", ModuleName)
			rounds++
			continue
		}

		summaries = append(summaries, valuableSummaries...)
		sources = append(sources, valuableSources...)
		rounds++

		// 评估信息是否已足够
		sufficient, reasoning, err := a.evaluateInformation(query)
		if err != nil {
			fmt.Printf("[%s] 信息评估失败: %v\n", ModuleName, err)
			continue
		}

		fmt.Printf("[%s] 信息充分性评估: %s\n", ModuleName, reasoning)

		if sufficient {
			fmt.Printf("[%s] 信息已足够，结束深度搜索\n", ModuleName)
			break
		}
	}

	return summaries, sources, rounds
}

// =============================================================================
// Phase 4: 报告生成
// =============================================================================

// phaseGenerateReport 基于所有积累的摘要生成最终搜索报告
func (a *SearchAgent) phaseGenerateReport(query string, searchRounds int) (*SearchReport, error) {
	fmt.Printf("[%s] [阶段 4/5] 报告生成中...\n", ModuleName)

	if aiGenerateReport == nil {
		// 无 AI 时返回原始摘要拼接
		rawAnswer := strings.Join(a.accumulatedSummaries, "\n\n---\n\n")
		return &SearchReport{
			Query:        query,
			Answer:       rawAnswer,
			UsedSources:  a.accumulatedSources,
			SearchRounds: searchRounds,
			GeneratedAt:  time.Now(),
		}, nil
	}

	answer, err := aiGenerateReport(query, a.accumulatedSummaries, a.accumulatedSources)
	if err != nil {
		return nil, fmt.Errorf("报告生成失败: %w", err)
	}

	return &SearchReport{
		Query:        query,
		Answer:       answer,
		FromMemory:   false,
		UsedSources:  a.accumulatedSources,
		SearchRounds: searchRounds,
		GeneratedAt:  time.Now(),
	}, nil
}

// =============================================================================
// Phase 5: 记忆存储
// =============================================================================

// phaseStoreToMemory 将搜索结果存入记忆库
func (a *SearchAgent) phaseStoreToMemory(query string, report *SearchReport) {
	if memoryStore == nil || report.FromMemory {
		return
	}

	fmt.Printf("[%s] [阶段 5/5] 存储搜索结果到记忆库...\n", ModuleName)

	record := MemorySearchRecord{
		Question:    query,
		Keywords:    a.usedKeywords,
		KeyFindings: strings.Join(a.accumulatedSummaries, "\n"),
		Answer:      report.Answer,
		Timestamp:   time.Now().Unix(),
	}

	if err := memoryStore(record); err != nil {
		fmt.Printf("[%s] 记忆存储失败: %v\n", ModuleName, err)
	} else {
		fmt.Printf("[%s] 搜索结果已存入 search_memory\n", ModuleName)
	}
}

// =============================================================================
// 内部辅助方法
// =============================================================================

// executeSearchAndExtract 执行搜索 → 提取页面内容 → AI 摘要
// 返回有效摘要列表和来源 URL 列表
func (a *SearchAgent) executeSearchAndExtract(keywords []string) (summaries []string, sources []string, err error) {
	// 合并所有关键词的搜索结果
	var allResults []SearchResult
	for _, kw := range keywords {
		results, searchErr := ExecuteSearch(kw)
		if searchErr != nil {
			fmt.Printf("[%s] 关键词 '%s' 搜索失败: %v\n", ModuleName, kw, searchErr)
			continue
		}
		allResults = append(allResults, results...)
	}

	if len(allResults) == 0 {
		return nil, nil, fmt.Errorf("所有关键词均无搜索结果")
	}

	// 去重（按 URL）
	seenURLs := make(map[string]bool)
	var uniqueResults []SearchResult
	for _, r := range allResults {
		if !seenURLs[r.URL] {
			seenURLs[r.URL] = true
			uniqueResults = append(uniqueResults, r)
		}
	}

	fmt.Printf("[%s] 去重后共 %d 条搜索结果\n", ModuleName, len(uniqueResults))

	// 过滤字典网站（搜索智能体自身具备字典能力，无需查阅外部字典网站）
	uniqueResults = filterDictionarySites(uniqueResults)
	fmt.Printf("[%s] 过滤字典网站后剩余 %d 条结果\n", ModuleName, len(uniqueResults))

	// 提取每个页面内容并摘要
	var unreachableCount int
	for i, result := range uniqueResults {
		fmt.Printf("[%s] 处理结果 [%d/%d]: %s\n", ModuleName, i+1, len(uniqueResults), result.Title)

		content, extractErr := ExtractPageContent(result.URL)
		if extractErr != nil {
			fmt.Printf("[%s] 页面不可达: %s (%v)，跳过\n", ModuleName, result.URL, extractErr)
			unreachableCount++
			continue
		}

		// 准备截图数据（仅 visual 类型）
		var screenshotData [][]byte
		if content.ContentType == "visual" && len(content.Screenshots) > 0 {
			for _, ss := range content.Screenshots {
				screenshotData = append(screenshotData, ss.ImageData)
			}
		}

		// AI 摘要
		var summary string
		if aiSummarizeContent != nil {
			summary, extractErr = aiSummarizeContent(content.TextContent, screenshotData)
			if extractErr != nil {
				fmt.Printf("[%s] 摘要生成失败: %v，使用原始文本\n", ModuleName, extractErr)
				summary = truncateText(content.TextContent, 500)
			}
		} else {
			// 无 AI 时直接使用清洗后的文本（截断到合理长度）
			summary = truncateText(content.TextContent, 500)
		}

		if summary != "" {
			summaries = append(summaries, summary)
			sources = append(sources, content.URL)
		}
	}

	// 全部不可达
	if len(uniqueResults) > 0 && len(summaries) == 0 {
		return nil, nil, fmt.Errorf("所有搜索结果页面均不可达 (%d 条)", len(uniqueResults))
	}

	return summaries, sources, nil
}

// generateKeywords 调用 AI 生成初始搜索关键词
func (a *SearchAgent) generateKeywords(query string) ([]string, error) {
	if aiGenerateKeywords != nil {
		return aiGenerateKeywords(query)
	}

	// 降级方案：直接使用原始查询作为关键词
	fmt.Printf("[%s] 关键词生成未集成，使用原始查询作为关键词\n", ModuleName)
	return []string{query}, nil
}

// generateDeepKeywords 调用 AI 生成深度搜索关键词
func (a *SearchAgent) generateDeepKeywords(query string) ([]string, error) {
	if aiGenerateDeepKeywords == nil {
		return nil, nil
	}

	accumulatedText := strings.Join(a.accumulatedSummaries, "\n")
	return aiGenerateDeepKeywords(query, accumulatedText, a.usedKeywords)
}

// evaluateInformation 调用 AI 评估当前积累的信息是否足以回答用户问题
func (a *SearchAgent) evaluateInformation(query string) (sufficient bool, reasoning string, err error) {
	if aiEvaluateSufficiency == nil {
		// 无 AI 时简单判定：有至少 1 条摘要就认为足够
		return len(a.accumulatedSummaries) > 0, "默认判定", nil
	}

	accumulatedText := strings.Join(a.accumulatedSummaries, "\n\n---\n\n")
	return aiEvaluateSufficiency(query, accumulatedText)
}

// filterValuableResults 筛选与查询相关的有效摘要
// 对每条摘要调用 AI 评估相关性，丢弃无关结果
func (a *SearchAgent) filterValuableResults(query string, summaries []string, sources []string) ([]string, []string) {
	if aiEvaluateSufficiency == nil || len(summaries) <= 1 {
		// 无法评估时保留全部
		return summaries, sources
	}

	var filteredSummaries []string
	var filteredSources []string

	for i, summary := range summaries {
		// 将单条摘要作为上下文评估相关性
		relevant, _, err := aiEvaluateSufficiency(query, summary)
		if err != nil {
			// 评估失败，保守保留
			filteredSummaries = append(filteredSummaries, summary)
			if i < len(sources) {
				filteredSources = append(filteredSources, sources[i])
			}
			continue
		}

		if relevant || len(summaries) == 1 {
			// 相关 或 唯一结果（保留最后一条）
			filteredSummaries = append(filteredSummaries, summary)
			if i < len(sources) {
				filteredSources = append(filteredSources, sources[i])
			}
		} else {
			fmt.Printf("[%s] 舍弃无关结果: %s\n", ModuleName, truncateText(summary, 80))
		}
	}

	return filteredSummaries, filteredSources
}

// filterDictionarySites 过滤字典/词典类网站
// 搜索智能体自身具备字典能力，无需浪费浏览器时间和模型 token 查阅外部字典网站
func filterDictionarySites(results []SearchResult) []SearchResult {
	if len(results) == 0 {
		return results
	}

	filtered := make([]SearchResult, 0, len(results))
	for _, r := range results {
		if isDictionarySite(r.URL, r.Title) {
			fmt.Printf("[%s] 过滤字典网站: %s\n", ModuleName, r.URL)
			continue
		}
		filtered = append(filtered, r)
	}
	return filtered
}

// isDictionarySite 检查 URL 或标题是否包含字典关键词
func isDictionarySite(url, title string) bool {
	combined := strings.ToLower(url + " " + title)
	for _, kw := range dictionaryKeywords {
		if strings.Contains(combined, kw) {
			return true
		}
	}
	return false
}

// =============================================================================
// 浏览器健康管理
// =============================================================================

// ensureBrowserHealthy 确保浏览器处于健康状态
// 浏览器刚启动/重启后首次查询跳过健康检查，避免进程注册延迟导致误判
func ensureBrowserHealthy() error {
	if browserJustLaunched {
		browserJustLaunched = false
		return nil
	}

	if !browserLaunched {
		return tryRestartBrowser()
	}

	if checkBrowserHealth == nil {
		// 无监控时简单检查浏览器是否启动
		if !browserLaunched {
			return tryRestartBrowser()
		}
		return nil
	}

	health := checkBrowserHealth()
	if health.Healthy {
		return nil
	}

	fmt.Printf("[%s] 浏览器不健康 (内存=%dMB CPU=%.1f%%)，尝试重启...\n",
		ModuleName, health.MemMB, health.CPUPercent)

	return tryRestartBrowser()
}

// tryRestartBrowser 尝试重启浏览器
func tryRestartBrowser() error {
	fmt.Printf("[%s] 正在重启浏览器...\n", ModuleName)

	CloseBrowser()
	ResetCPUTracking() // 重置 CPU 追踪状态

	if err := LaunchBrowser(); err != nil {
		return fmt.Errorf("月华的浏览器崩了，找不到你想要的内容")
	}

	fmt.Printf("[%s] 浏览器重启成功\n", ModuleName)
	return nil
}

// =============================================================================
// 通用辅助函数
// =============================================================================

// logProgress 输出搜索进度到终端日志
// 根据阶段类型使用不同的 logger 层级：
//   - 正常阶段 → logger.SubInfo
//   - 警告/错误 → logger.SubWarn / logger.SubError
func logProgress(event ProgressEvent) {
	switch event.Phase {
	case "memory_lookup":
		logger.Info(ModuleName, "[记忆检索] %s", event.Message)
	case "searching":
		logger.SubInfo(ModuleName, "搜索", "[轮次 %d/%d] %s", event.Round, event.Total, event.Message)
	case "extracting":
		logger.SubInfo(ModuleName, "提取", "%s", event.Message)
	case "summarizing":
		logger.SubInfo(ModuleName, "摘要", "%s", event.Message)
	case "evaluating":
		logger.Info(ModuleName, "[充分性评估] %s", event.Message)
	case "deep_search":
		logger.SubInfo(ModuleName, "深度搜索", "[轮次 %d/%d] %s", event.Round, event.Total, event.Message)
	case "generating_report":
		logger.Info(ModuleName, "[报告生成] %s", event.Message)
	case "warning":
		logger.Warn(ModuleName, "%s", event.Message)
	case "error":
		logger.Error(ModuleName, "%s", event.Message)
	default:
		logger.Info(ModuleName, "%s", event.Message)
	}
}

// truncateText 截断文本到指定最大字符数（保留完整 rune）
func truncateText(text string, maxLen int) string {
	runes := []rune(text)
	if len(runes) <= maxLen {
		return text
	}
	return string(runes[:maxLen]) + "\n...(内容已截断)"
}
