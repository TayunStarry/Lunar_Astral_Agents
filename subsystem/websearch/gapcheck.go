package websearch

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// GapCheckConfig 研究校验配置
type GapCheckConfig struct {
	MaxRounds     int // 最大校验轮次（默认3）
	MaxSubQueries int // 最大子问题数
}

// runGapCheckSearch 研究校验式深度搜索：采集→校验→补充→循环→综合报告
func (s *DepthSearcher) runGapCheckSearch(ctx context.Context, query string, webpageSearcher *WebpageSearcher, memProvider MemoryProvider, cfg GapCheckConfig, forceRefresh ...bool) (string, error) {
	maxRounds := cfg.MaxRounds
	if maxRounds <= 0 {
		maxRounds = 3
	}

	// 阶段1：采集研究数据
	researchData, err := s.CollectData(ctx, query, forceRefresh...)
	if err != nil {
		return "", fmt.Errorf("数据采集失败: %w", err)
	}
	if !s.hasResearchData(researchData) {
		// 无数据时降级到网页搜索
		if webpageSearcher != nil {
			return webpageSearcher.Search(query)
		}
		return fmt.Sprintf("搜索 %q 没有找到相关内容。", query), nil
	}

	// 注入记忆信息
	if memProvider != nil {
		if memResult, err := memProvider.Query(query); err == nil && memResult != "" && !isMemoryEmpty(memResult) {
			researchData = s.appendMemoryData(researchData, memResult)
		}
	}

	// 阶段2：研究校验循环
	for round := 1; round <= maxRounds; round++ {
		if ctx.Err() != nil {
			break
		}

		if s.debugLog != nil {
			s.debugLog("[研究校验] === 第%d/%d轮校验 ===", round, maxRounds)
		}

		// LLM 检查信息缺口
		missingInfo := s.checkGap(ctx, query, researchData)
		if missingInfo == "" {
			if s.debugLog != nil {
				s.debugLog("[研究校验] 信息充足，进入综合报告")
			}
			break
		}

		if s.debugLog != nil {
			s.debugLog("[研究校验] 发现信息缺口: %s", truncateToRunes(missingInfo, 200))
		}

		// 从缺失信息中提取搜索查询
		searchQuery := extractFirstItemFromList(missingInfo)
		if searchQuery == "" {
			searchQuery = truncateToRunes(missingInfo, 100)
		}

		// 补充搜索
		var newResults []SearchResult
		if webpageSearcher != nil {
			if results, err := webpageSearcher.SearchRawWithSelectiveFetch(searchQuery, 2); err == nil && len(results) > 0 {
				newResults = results
			}
		}
		// 降级：使用简易搜索
		if len(newResults) == 0 {
			if results, err := s.simple.SearchRaw(searchQuery); err == nil && len(results) > 0 {
				newResults = results
			}
		}

		if len(newResults) == 0 {
			if s.debugLog != nil {
				s.debugLog("[研究校验] 补充搜索无结果，跳过本轮")
			}
			continue
		}

		// 注入补充数据
		researchData = s.appendSupplementaryData(researchData, searchQuery, newResults)
		if s.debugLog != nil {
			s.debugLog("[研究校验] 补充数据已注入 新增结果=%d", len(newResults))
		}
	}

	// 阶段3：综合报告
	return s.synthesizeReport(query, researchData)
}

// checkGap 检查研究数据是否存在信息缺口，返回缺失信息描述
func (s *DepthSearcher) checkGap(ctx context.Context, originalQuery string, data *ResearchData) string {
	if s.llmProvider == nil {
		return ""
	}
	if ctx.Err() != nil {
		return ""
	}

	researchText := s.formatResearchDataForGapCheck(data)

	today := time.Now().Format("2006年1月2日")

	prompt := fmt.Sprintf(`你是一个研究校验员。请检查以下研究数据是否足以回答用户问题。

当前日期：%s

用户问题：%s

研究数据：
%s

请判断：
1. 当前数据是否覆盖了问题的各个方面？
2. 是否存在关键信息缺口？
3. 检查每条结果的"内容"字段，看是否包含与用户问题直接相关的具体信息（如卡池名称、时间、角色等）。如果所有结果都没有提到具体的卡池信息，说明信息不足。
4. 注意当前日期，如果搜索结果中的时间敏感信息（如公测日期、活动截止日期）已经过期，这些信息仍然有效（只是过期了，需要说明），但搜索结果中的预告信息不能代表当前状态。

如果信息充足（有具体的、可直接回答用户问题的内容），请回复"信息充足"。
如果存在缺口或信息太笼统/无关，请按以下格式列出缺失的关键信息（每行一个，用于后续补充搜索）：
- 缺失信息1
- 缺失信息2

注意：
- 只列出最关键的1-2个缺失信息，确保它们是具体可搜索的关键词
- 如果所有结果都来自字典站、百科站或无关网站，即使内容很多也应视为信息不足
- 如果没有任何结果包含查询关键词（如"卡池""角色""活动"等），应视为信息不足
- 注意当前日期：如果搜索结果提到了某些活动/功能"即将推出""即将上线"，而当前日期已经过了那个时间，说明信息可能过时，但不代表功能不存在`, today, originalQuery, researchText)

	messages := []ChatMessage{
		{Role: "user", Content: prompt},
	}

	response, err := s.llmProvider.Chat(messages)
	if err != nil {
		if s.debugLog != nil {
			s.debugLog("[研究校验] LLM调用失败 err=%v", err)
		}
		return ""
	}

	response = strings.TrimSpace(response)

	// 判断是否信息充足
	if strings.Contains(response, "信息充足") || strings.Contains(response, "信息充分") {
		return ""
	}

	// 如果回复很短且不含列表项，视为信息充足
	if len([]rune(response)) < 20 {
		return ""
	}

	return response
}

// synthesizeReport 基于研究数据生成综合报告
func (s *DepthSearcher) synthesizeReport(originalQuery string, data *ResearchData) (string, error) {
	if s.llmProvider == nil {
		return s.formatResearchDataFallback(originalQuery, data), nil
	}

	researchText := s.formatResearchDataForReport(data)

	today := time.Now().Format("2006年1月2日")

	prompt := fmt.Sprintf(`请基于以下研究数据，生成一份关于用户问题的深度研究报告。

当前日期：%s

原始问题：%s

研究数据：
%s

要求：
1. 以"# 深度研究报告"开头
2. 包含"核心结论"章节：直接回答用户问题，列出3-5个关键事实
3. 包含"详细分析"章节：按维度展开分析，引用具体来源
4. 包含"信息来源"章节：列出所有引用的来源URL
5. 用markdown格式输出

【严格约束】
- 只陈述研究数据中明确提及的信息，不要添加搜索结果中没有的内容
- 如果研究数据中没有找到某个信息（如卡池详情、公测时间等），直接说"未找到相关信息"，不要编造
- 不要推测、猜测或补充搜索数据中不存在的事实
- 每条结论必须有对应的研究数据支撑
- 注意当前日期，判断搜索结果中的时间敏感信息（如公测日期、活动截止日期）是否仍然有效`, today, originalQuery, researchText)

	// prompt预算控制
	promptRunes := []rune(prompt)
	if len(promptRunes) > depthMaxPromptChars {
		overhead := len([]rune(fmt.Sprintf(`请基于以下研究数据，生成一份关于用户问题的深度研究报告。

当前日期：%s

原始问题：%s

研究数据：
`, today, originalQuery)))
		available := depthMaxPromptChars - overhead
		if available < 500 {
			available = 500
		}
		researchRunes := []rune(researchText)
		if len(researchRunes) > available {
			researchText = string(researchRunes[:available]) + "\n\n[研究数据已按预算截断]"
		}
		prompt = fmt.Sprintf(`请基于以下研究数据，生成一份关于用户问题的深度研究报告。

当前日期：%s

原始问题：%s

研究数据：
%s

要求：
1. 以"# 深度研究报告"开头
2. 包含"核心结论"章节：直接回答用户问题，列出3-5个关键事实
3. 包含"详细分析"章节：按维度展开分析，引用具体来源
4. 包含"信息来源"章节：列出所有引用的来源URL
5. 用markdown格式输出

【严格约束】
- 只陈述研究数据中明确提及的信息，不要添加搜索结果中没有的内容
- 如果研究数据中没有找到某个信息（如卡池详情、公测时间等），直接说"未找到相关信息"，不要编造
- 不要推测、猜测或补充搜索数据中不存在的事实
- 每条结论必须有对应的研究数据支撑
- 注意当前日期，判断搜索结果中的时间敏感信息（如公测日期、活动截止日期）是否仍然有效`, today, originalQuery, researchText)
	}

	messages := []ChatMessage{
		{Role: "user", Content: prompt},
	}

	response, err := s.llmProvider.Chat(messages)
	if err != nil {
		return s.formatResearchDataFallback(originalQuery, data), nil
	}

	// 输出截断
	responseRunes := []rune(response)
	if len(responseRunes) > depthMaxOutputChars {
		response = string(responseRunes[:depthMaxOutputChars]) + "\n\n[报告已截断]"
	}

	return response, nil
}

// formatResearchDataForGapCheck 格式化研究数据供缺口检查使用
func (s *DepthSearcher) formatResearchDataForGapCheck(data *ResearchData) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("原始问题：%s\n\n", data.OriginalQuery))

	totalChars := 0
	maxChars := 8000

	for i, sq := range data.SubQueries {
		sb.WriteString(fmt.Sprintf("## 子问题%d：%s\n", i+1, sq.Query))
		for j, r := range sq.Results {
			entry := fmt.Sprintf("  [%d] %s\n", j+1, r.Title)
			if r.URL != "" {
				entry += fmt.Sprintf("      来源: %s\n", r.URL)
			}
			snippet := r.Snippet
			if len([]rune(snippet)) > 300 {
				snippet = string([]rune(snippet)[:300]) + "..."
			}
			entry += fmt.Sprintf("      内容: %s\n", snippet)
			entryLen := len([]rune(entry))
			if totalChars+entryLen > maxChars {
				sb.WriteString("  [内容已按预算截断]\n")
				return sb.String()
			}
			sb.WriteString(entry)
			totalChars += entryLen
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// formatResearchDataForReport 格式化研究数据供综合报告使用
func (s *DepthSearcher) formatResearchDataForReport(data *ResearchData) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("原始问题：%s\n\n", data.OriginalQuery))

	totalChars := 0
	maxChars := depthMaxSubResultsChars

	for i, sq := range data.SubQueries {
		header := fmt.Sprintf("## 子问题%d：%s\n", i+1, sq.Query)
		headerLen := len([]rune(header))
		if totalChars+headerLen > maxChars {
			sb.WriteString("[内容已按预算截断]\n")
			break
		}
		sb.WriteString(header)
		totalChars += headerLen

		for j, r := range sq.Results {
			entry := fmt.Sprintf("[来源%d] %s\n", j+1, r.Title)
			if r.URL != "" {
				entry += fmt.Sprintf("链接: %s\n", r.URL)
			}
			snippet := r.Snippet
			if len([]rune(snippet)) > 500 {
				snippet = string([]rune(snippet)[:500]) + "..."
			}
			entry += fmt.Sprintf("内容: %s\n\n", snippet)
			entryLen := len([]rune(entry))
			if totalChars+entryLen > maxChars {
				sb.WriteString("[内容已按预算截断]\n")
				return sb.String()
			}
			sb.WriteString(entry)
			totalChars += entryLen
		}
	}

	return sb.String()
}

// formatResearchDataFallback 无 LLM 时的回退格式化
func (s *DepthSearcher) formatResearchDataFallback(query string, data *ResearchData) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# 深度研究报告\n\n原始问题：%s\n\n", query))

	for i, sq := range data.SubQueries {
		sb.WriteString(fmt.Sprintf("## 子问题%d：%s\n", i+1, sq.Query))
		formatted := formatResultsTruncated(sq.Results, depthMaxSnippetLen)
		sb.WriteString(formatted)
		sb.WriteString("\n")
	}

	text := sb.String()
	runes := []rune(text)
	if len(runes) > depthMaxOutputChars {
		text = string(runes[:depthMaxOutputChars]) + "\n\n[报告已截断]"
	}
	return text
}

// hasResearchData 检查研究数据是否为空
func (s *DepthSearcher) hasResearchData(data *ResearchData) bool {
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

// appendMemoryData 追加记忆数据到研究数据
func (s *DepthSearcher) appendMemoryData(data *ResearchData, memResult string) *ResearchData {
	copied := &ResearchData{
		OriginalQuery: data.OriginalQuery,
		SubQueries:    make([]SubQueryResult, len(data.SubQueries)),
	}
	copy(copied.SubQueries, data.SubQueries)
	if len(copied.SubQueries) > 0 {
		memItem := SearchResult{
			Title:   "记忆库信息",
			URL:     "",
			Snippet: memResult,
		}
		copied.SubQueries[0].Results = append(copied.SubQueries[0].Results, memItem)
	}
	return copied
}

// appendSupplementaryData 追加补充搜索结果到研究数据
func (s *DepthSearcher) appendSupplementaryData(data *ResearchData, query string, results []SearchResult) *ResearchData {
	if len(results) == 0 {
		return data
	}
	copied := &ResearchData{
		OriginalQuery: data.OriginalQuery,
		SubQueries:    make([]SubQueryResult, len(data.SubQueries)+1),
	}
	copy(copied.SubQueries, data.SubQueries)
	copied.SubQueries[len(data.SubQueries)] = SubQueryResult{
		Query:   fmt.Sprintf("补充搜索：%s", query),
		Results: results,
	}
	return copied
}
