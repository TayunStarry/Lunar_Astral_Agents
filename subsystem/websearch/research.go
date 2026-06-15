package websearch

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

// NewResearchSearcher 创建研究搜索器
func NewResearchSearcher(shallowSearcher *ShallowSearcher, llmProvider Provider, cfg ResearchConfig) *ResearchSearcher {
	maxResults := cfg.MaxResults
	if maxResults <= 0 || maxResults > researchMaxResultsPerSub {
		maxResults = researchMaxResultsPerSub
	}
	return &ResearchSearcher{
		shallow:     shallowSearcher,
		llmProvider: llmProvider,
		cfg:         ResearchConfig{MaxResults: maxResults, MaxSubQueries: cfg.MaxSubQueries},
	}
}

// Search 执行研究搜索：拆解子问题 → 并行搜索 → URL去重 → 综合报告
func (s *ResearchSearcher) Search(query string) (string, error) {
	// 第一步：拆解子问题
	subQueries, err := s.decomposeQuery(query)
	if err != nil || len(subQueries) == 0 {
		subQueries = []string{query}
	}

	// 限制子问题数量
	maxSub := s.cfg.MaxSubQueries
	if maxSub <= 0 {
		maxSub = 6
	}
	if len(subQueries) > maxSub {
		subQueries = subQueries[:maxSub]
	}

	// 第二步：并行搜索每个子问题（返回原始结果用于去重）
	var wg sync.WaitGroup
	resultCh := make(chan subResult, len(subQueries))

	for _, sq := range subQueries {
		wg.Add(1)
		go func(q string) {
			defer wg.Done()
			results, searchErr := s.shallow.SearchRaw(q)
			resultCh <- subResult{Query: q, Results: results, Error: searchErr}
		}(sq)
	}

	go func() {
		wg.Wait()
		close(resultCh)
	}()

	var allResults []subResult
	for r := range resultCh {
		allResults = append(allResults, r)
	}

	// URL去重：跨子问题去重，同一URL只保留首次出现的子问题结果
	seenURLs := make(map[string]bool)
	for i := range allResults {
		if allResults[i].Error != nil {
			continue
		}
		var deduped []SearchResult
		for _, r := range allResults[i].Results {
			normalizedURL := strings.TrimRight(strings.TrimSpace(r.URL), "/")
			if seenURLs[normalizedURL] {
				continue
			}
			seenURLs[normalizedURL] = true
			deduped = append(deduped, r)
		}
		allResults[i].Results = deduped
	}

	// 第三步：汇总生成报告
	return s.generateReport(query, allResults)
}

func (s *ResearchSearcher) decomposeQuery(query string) ([]string, error) {
	if s.llmProvider == nil {
		return []string{query}, nil
	}

	prompt := fmt.Sprintf(`你是一个研究助手。请将以下用户问题拆解为3-6个更具体的子问题，以便进行全面深入的网络搜索。

用户问题：%s

要求：
1. 每个子问题应聚焦于问题的不同方面或维度
2. 子问题之间应有互补性，避免重复
3. 用JSON数组格式输出，只输出数组，不要其他内容

示例输出：["子问题1", "子问题2", "子问题3"]`, query)

	messages := []ChatMessage{
		{Role: "user", Content: prompt},
	}

	response, err := s.llmProvider.Chat(messages)
	if err != nil {
		return nil, err
	}

	// 解析 JSON 数组
	response = strings.TrimSpace(response)
	if idx := strings.Index(response, "["); idx >= 0 {
		if endIdx := strings.LastIndex(response, "]"); endIdx > idx {
			response = response[idx : endIdx+1]
		}
	}

	var subQueries []string
	if err := json.Unmarshal([]byte(response), &subQueries); err != nil {
		return []string{query}, nil
	}

	return subQueries, nil
}

// generateReport 使用 LLM 汇总所有子问题搜索结果，生成结构化报告
// 包含prompt预算控制和输出截断保护
func (s *ResearchSearcher) generateReport(originalQuery string, results []subResult) (string, error) {
	// 格式化子问题结果，控制总注入量
	var parts []string
	totalChars := 0

	for i, r := range results {
		var part string
		if r.Error != nil {
			part = fmt.Sprintf("## 子问题%d：%s\n搜索失败：%v", i+1, r.Query, r.Error)
		} else {
			// 使用截断格式化
			formatted := formatResultsTruncated(r.Results, researchMaxSnippetLen)
			part = fmt.Sprintf("## 子问题%d：%s\n%s", i+1, r.Query, formatted)
		}

		partLen := len([]rune(part))
		// 预算检查：超过上限则截断
		if totalChars+partLen > researchMaxSubResultsChars {
			remaining := researchMaxSubResultsChars - totalChars
			if remaining > 200 {
				partRunes := []rune(part)
				if len(partRunes) > remaining {
					part = string(partRunes[:remaining]) + "\n...[内容已按预算截断]"
				}
			} else {
				// 空间不足，标记截断
				parts = append(parts, fmt.Sprintf("[还有%d个子问题的结果已省略]", len(results)-i))
				break
			}
		}

		parts = append(parts, part)
		totalChars += partLen
	}

	allResults := strings.Join(parts, "\n\n")

	if s.llmProvider == nil {
		fallback := fmt.Sprintf("# 研究搜索报告\n\n原始问题：%s\n\n%s", originalQuery, allResults)
		// 回退也截断
		fallbackRunes := []rune(fallback)
		if len(fallbackRunes) > researchMaxOutputChars {
			fallback = string(fallbackRunes[:researchMaxOutputChars]) + "\n\n[报告已截断]"
		}
		return fallback, nil
	}

	// 构建prompt，带预算控制
	promptTemplate := `你是一个研究助手。请基于以下多个子问题的搜索结果，生成一份结构化的研究报告。

原始问题：%s

各子问题搜索结果：
%s

要求：
1. 以"# 研究搜索报告"开头
2. 包含"核心发现"章节（3-5点总结）
3. 包含"详细分析"章节，按维度分点阐述
4. 包含"信息来源"章节，列出所有引用的来源
5. 保持专业、客观的语气
6. 用markdown格式输出`

	prompt := fmt.Sprintf(promptTemplate, originalQuery, allResults)

	// prompt预算控制
	promptRunes := []rune(prompt)
	if len(promptRunes) > researchMaxPromptChars {
		// 计算需要从allResults中砍掉多少
		overhead := len([]rune(fmt.Sprintf(promptTemplate, originalQuery, "")))
		available := researchMaxPromptChars - overhead
		if available < 500 {
			available = 500
		}
		allResultsRunes := []rune(allResults)
		if len(allResultsRunes) > available {
			allResults = string(allResultsRunes[:available]) + "\n\n[搜索结果已按token预算截断]"
		}
		prompt = fmt.Sprintf(promptTemplate, originalQuery, allResults)
	}

	messages := []ChatMessage{
		{Role: "user", Content: prompt},
	}

	response, err := s.llmProvider.Chat(messages)
	if err != nil {
		fallback := fmt.Sprintf("# 研究搜索报告\n\n原始问题：%s\n\n%s", originalQuery, allResults)
		fallbackRunes := []rune(fallback)
		if len(fallbackRunes) > researchMaxOutputChars {
			fallback = string(fallbackRunes[:researchMaxOutputChars]) + "\n\n[报告已截断]"
		}
		return fallback, nil
	}

	// 输出截断
	responseRunes := []rune(response)
	if len(responseRunes) > researchMaxOutputChars {
		response = string(responseRunes[:researchMaxOutputChars]) + "\n\n[报告已截断]"
	}

	return response, nil
}
