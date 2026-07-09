package websearch

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// NewWebpageSearcher 创建网页搜索器
func NewWebpageSearcher(simpleSearcher *SimpleSearcher, llmProvider Provider, cfg WebpageConfig, httpCfg HTTPConfig) *WebpageSearcher {
	return &WebpageSearcher{
		simple:      simpleSearcher,
		llmProvider: llmProvider,
		cfg:         cfg,
		httpClient:  &http.Client{Timeout: time.Duration(cfg.FetchTimeout) * time.Second},
	}
}

// Search 执行网页搜索
func (s *WebpageSearcher) Search(query string) (string, error) {
	// 硬性上限：最少抓取10条，最多抓取webpageMaxFetchResults条
	limit := min(max(s.cfg.MaxResults, 10), webpageMaxFetchResults)

	// 第一步：搜索
	results, err := s.simple.SearchRaw(query)
	if err != nil {
		return "", fmt.Errorf("网页搜索失败: %w", err)
	}

	if len(results) == 0 {
		return fmt.Sprintf("未找到与 %q 相关的搜索结果。", query), nil
	}
	if len(results) > limit {
		results = results[:limit]
	}

	// 第二步：抓取网页内容 + 相关性过滤
	queryKeywords := extractQueryKeywords(query)

	contentParts := make([]string, 0, len(results))
	skipParts := make([]string, 0) // 不相关的来源记录
	fetchSuccess := 0
	fetchFail := 0
	totalContentChars := 0

	if s.cfg.FetchContent {
		for i, r := range results {
			body, fetchErr := s.fetchContent(r.URL)

			if fetchErr == nil && len(body) > 0 {
				body = truncateText(body, webpageMaxPerPageLen)

				// 相关性过滤：检查抓取内容是否与查询关键词匹配
				relevanceScore := checkContentRelevance(queryKeywords, body, r.Title, r.Snippet)
				if relevanceScore < 1 {
					skipParts = append(skipParts, fmt.Sprintf(
						"[来源%d] %s (不相关，已跳过)", i+1, r.URL,
					))
					continue
				}

				fetchSuccess++
				contentLen := len([]rune(body))
				// token 预算检查
				if totalContentChars+contentLen > webpageMaxTotalContentLen {
					break
				}
				totalContentChars += contentLen

				contentParts = append(contentParts, fmt.Sprintf(
					"[来源%d] %s\nURL: %s\n内容:\n%s",
					i+1, r.Title, r.URL, body,
				))
			} else {
				fetchFail++
				// 抓取失败时也对标题/摘要做相关性检查
				relevanceScore := checkContentRelevance(queryKeywords, "", r.Title, r.Snippet)
				if relevanceScore < 1 {
					skipParts = append(skipParts, fmt.Sprintf(
						"[来源%d] %s (抓取失败且不相关，已跳过)", i+1, r.URL,
					))
					continue
				}
				// 抓取失败时用摘要，也做截断
				snippetLen := len([]rune(r.Snippet))
				if snippetLen > 300 {
					r.Snippet = string([]rune(r.Snippet)[:300]) + "..."
				}
				contentParts = append(contentParts, fmt.Sprintf(
					"[来源%d] %s\n摘要: %s\nURL: %s",
					i+1, r.Title, r.Snippet, r.URL,
				))
			}
		}
	} else {
		for i, r := range results {
			contentParts = append(contentParts, fmt.Sprintf(
				"[来源%d] %s\n摘要: %s\nURL: %s",
				i+1, r.Title, r.Snippet, r.URL,
			))
		}
	} // 第三步：LLM 总结 or 兜底
	if len(contentParts) == 0 {
		// 所有结果被过滤但仍有搜索结果 → 兜底返回前3条原始结果
		if len(results) > 0 {
			fallbackParts := make([]string, 0, 3)
			for i, r := range results {
				if i >= 3 {
					break
				}
				fallbackParts = append(fallbackParts, fmt.Sprintf(
					"[来源%d] %s\n摘要: %s\nURL: %s",
					i+1, r.Title, truncateText(r.Snippet, 200), r.URL,
				))
			}
			searchContent := strings.Join(fallbackParts, "\n\n---\n\n")
			return fmt.Sprintf("搜索 %q 的原始结果（相关性过滤较严格，以下为未过滤的原始结果）：\n\n%s", query, searchContent), nil
		}
		return fmt.Sprintf("搜索 %q 没有找到相关内容，所有结果已被相关性过滤。", query), nil
	}

	// 第三步：LLM 总结
	if s.llmProvider == nil {
		return formatWebpageResultsFallback(query, contentParts), nil
	}

	result, err := s.summarizeWithLLM(query, contentParts)
	if err != nil {
		return formatWebpageResultsFallback(query, contentParts), nil
	}

	return result, nil
}

func (s *WebpageSearcher) summarizeWithLLM(query string, contentParts []string) (string, error) {
	// 构建搜索结果部分，受 maxPromptChars 限制
	searchContent := strings.Join(contentParts, "\n\n---\n\n")

	promptTemplate := `请基于以下搜索结果，对用户问题"%s"进行综合分析回答。

要求：
1. 先给出一个简洁的总结（2-3句话）
2. 然后分点列出关键信息，每个要点标注来源编号
3. 最后列出所有引用来源的URL

搜索结果：
%s`

	// 计算可用空间并截断
	templateOverhead := len([]rune(fmt.Sprintf(promptTemplate, query, "")))
	availableForContent := webpageMaxPromptChars - templateOverhead
	if availableForContent < 500 {
		availableForContent = 500
	}

	searchRunes := []rune(searchContent)
	if len(searchRunes) > availableForContent {
		// 智能截断：在句子边界处截断
		truncated := string(searchRunes[:availableForContent])
		if lastPeriod := strings.LastIndexAny(truncated, ".\n。"); lastPeriod > availableForContent/2 {
			truncated = truncated[:lastPeriod+3]
		}
		truncated += "\n\n[内容已按token预算截断]"
		searchContent = truncated
	}

	prompt := fmt.Sprintf(promptTemplate, query, searchContent)

	messages := []ChatMessage{
		{Role: "user", Content: prompt},
	}

	response, err := s.llmProvider.Chat(messages)
	if err != nil {
		return "", err
	}

	// 输出截断
	responseRunes := []rune(response)
	if len(responseRunes) > webpageMaxLLMOutputLen {
		response = string(responseRunes[:webpageMaxLLMOutputLen]) + "\n\n[回复已截断]"
	}

	return response, nil
}

func (s *WebpageSearcher) fetchContent(pageURL string) (string, error) {
	req, err := http.NewRequest("GET", pageURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", defaultConfig.HTTP.UserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}

	return extractTextContent(string(body)), nil
}

// ── 相关性过滤 ──

// extractQueryKeywords 从查询字符串中提取关键词用于相关性判定
// 策略：优先保留完整查询词作为关键词，再补充子词和2-gram
func extractQueryKeywords(query string) []string {
	var keywords []string

	// 始终将完整查询词作为第一个关键词（专有名词整体匹配）
	queryRunes := []rune(query)
	if len(queryRunes) >= 2 {
		keywords = append(keywords, strings.ToLower(query))
	}

	// 按空格拆分为独立词
	parts := strings.Fields(query)
	for _, part := range parts {
		runes := []rune(part)
		if len(runes) >= 2 {
			keywords = append(keywords, strings.ToLower(part))
		}
	}

	// 如果没有空格分隔的独立词，补充2-gram
	if len(parts) <= 1 {
		runes := []rune(query)
		for i := 0; i < len(runes)-1; i++ {
			keywords = append(keywords, strings.ToLower(string(runes[i:i+2])))
		}
	}

	return keywords
}

// checkContentRelevance 检查内容与查询的相关性
// 返回值：匹配到的关键词数量
// 判定逻辑：
//   - 标题/摘要匹配完整查询词 → 直接判定为相关（搜索引擎已排序）
//   - 标题/摘要匹配任意关键词 → 相关（搜索结果本身已由引擎筛选）
//   - 仅内容匹配 → 需至少1个关键词匹配
func checkContentRelevance(queryKeywords []string, content, title, snippet string) int {
	if len(queryKeywords) == 0 {
		return 2 // 无法判断时，默认保留
	}

	// 合并标题和摘要作为轻量级预检
	meta := strings.ToLower(title + " " + snippet)

	// 检查完整查询词是否在标题/摘要中（queryKeywords[0] 是完整查询词）
	if len(queryKeywords) > 0 && strings.Contains(meta, queryKeywords[0]) {
		return 2 // 完整查询词匹配，直接判定为相关
	}

	matchCount := 0
	for _, kw := range queryKeywords {
		if strings.Contains(meta, kw) {
			matchCount++
		}
	}

	// 标题/摘要匹配任意关键词 → 搜索引擎已筛选，判定为相关
	if matchCount >= 1 {
		return matchCount
	}

	// 标题/摘要无匹配 → 检查实际内容
	contentLower := strings.ToLower(content)
	for _, kw := range queryKeywords {
		if strings.Contains(contentLower, kw) {
			matchCount++
		}
	}

	return matchCount
}
