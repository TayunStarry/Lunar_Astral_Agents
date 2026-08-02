package websearch

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// NewWebpageSearcher 创建网页搜索器
func NewWebpageSearcher(simpleSearcher *SimpleSearcher, llmProvider Provider, cfg WebpageConfig, httpCfg HTTPConfig, debugLog func(format string, args ...interface{})) *WebpageSearcher {
	return &WebpageSearcher{
		simple:      simpleSearcher,
		llmProvider: llmProvider,
		cfg:         cfg,
		httpClient:  &http.Client{Timeout: time.Duration(cfg.FetchTimeout) * time.Second},
		debugLog:    debugLog,
	}
}

// Search 执行网页搜索
func (s *WebpageSearcher) Search(query string) (string, error) {
	if s.debugLog != nil {
		s.debugLog("[网页搜索] 开始搜索 query=%q", query)
	}

	limit := min(max(s.cfg.MaxResults, 15), webpageMaxFetchResults)

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

	if s.debugLog != nil {
		var urlList []string
		for _, r := range results {
			urlList = append(urlList, r.URL)
		}
		s.debugLog("[网页搜索] 查询=%q 搜索引擎返回%d条结果 URLs=%v", query, len(results), urlList)
	}

	// 过滤字典/百科网站，这些网站通常是解释单个词语，与查询无关
	filteredResults := make([]SearchResult, 0, len(results))
	for _, r := range results {
		if isDictionarySite(r.URL) {
			if s.debugLog != nil {
				s.debugLog("[网页搜索] 字典网站过滤跳过 URL=%s", r.URL)
			}
			continue
		}
		filteredResults = append(filteredResults, r)
	}
	if len(filteredResults) < len(results) {
		if s.debugLog != nil {
			s.debugLog("[网页搜索] 字典网站过滤后剩余%d条结果", len(filteredResults))
		}
		results = filteredResults
	}

	// 查询降级：若结果标题/摘要均不含完整查询词，用核心实体重搜
	effectiveQuery := query
	fullQuery := strings.ToLower(query)
	metaMatch := 0
	for _, r := range results {
		if strings.Contains(strings.ToLower(r.Title+" "+r.Snippet), fullQuery) {
			metaMatch++
		}
	}
	if metaMatch == 0 {
		if coreEntity := extractCoreEntity(query); coreEntity != "" {
			if s.debugLog != nil {
				s.debugLog("[网页搜索] 查询降级 原查询=%q 核心实体=%q", query, coreEntity)
			}
			if newResults, err := s.simple.SearchRaw(coreEntity); err == nil && len(newResults) > 0 {
				coreLower := strings.ToLower(coreEntity)
				degraded := make([]SearchResult, 0, len(newResults))
				seen := make(map[string]bool, len(newResults))
				for _, r := range newResults {
					if isDictionarySite(r.URL) || seen[r.URL] {
						continue
					}
					// 仅保留标题/摘要包含核心实体的结果
					if strings.Contains(strings.ToLower(r.Title+" "+r.Snippet), coreLower) {
						degraded = append(degraded, r)
						seen[r.URL] = true
					}
				}
				// 降级结果有效则替换原结果
				if len(degraded) > 0 {
					results = degraded
					effectiveQuery = coreEntity
					if s.debugLog != nil {
						s.debugLog("[网页搜索] 查询降级完成 替换为%d条相关结果 effectiveQuery=%q", len(results), effectiveQuery)
					}
				}
			}
		}
	}
	if len(results) > limit {
		results = results[:limit]
	}

	// 第二步：抓取网页内容 + 相关性过滤（降级后用核心实体的关键词）
	queryKeywords := extractQueryKeywords(effectiveQuery)

	contentParts := make([]string, 0, len(results))
	skipParts := make([]string, 0) // 不相关的来源记录
	fetchSuccess := 0
	fetchFail := 0
	totalContentChars := 0

	if s.cfg.FetchContent {
		// 阶段1：HTTP 抓取所有结果
		type fetchItem struct {
			idx    int
			result SearchResult
			body   string
			isSPA  bool
			err    error
		}
		items := make([]fetchItem, len(results))
		for i := range results {
			body, err := s.fetchContentHTTPOnly(results[i].URL)
			isSPA := isSPAShell(body)
			items[i] = fetchItem{i, results[i], body, isSPA, err}
			if s.debugLog != nil {
				if err == nil && isSPA {
					s.debugLog("[网页搜索] 来源%d HTTP抓取到SPA空壳 URL=%s body_len=%d",
						i+1, results[i].URL, len([]rune(body)))
				}
			}
		}

		// 阶段2：并行浏览器渲染所有 SPA 页面
		if s.browserRenderer != nil {
			// 收集需要渲染的 SPA 索引
			var spaIndices []int
			for i := range items {
				if items[i].err == nil {
					isSPAEmpty := len([]rune(items[i].body)) < 200

					hasCoreKeywords := false
					if !isSPAEmpty && len(queryKeywords) > 0 {
						bodyLower := strings.ToLower(items[i].body)
						matchCount := 0
						for _, kw := range queryKeywords {
							if strings.Contains(bodyLower, strings.ToLower(kw)) {
								matchCount++
							}
						}
						hasCoreKeywords = matchCount >= (len(queryKeywords)+1)/2
					}

					// 需要浏览器渲染：SPA空壳或内容不匹配核心关键词
					if isSPAEmpty || !hasCoreKeywords {
						spaIndices = append(spaIndices, i)
					}
				}
			}

			if len(spaIndices) > 0 {
				if s.debugLog != nil {
					s.debugLog("[网页搜索] 开始并发渲染 %d 个SPA页面", len(spaIndices))
				}

				var spaWg sync.WaitGroup
				sem := make(chan struct{}, 2) // 最多2个并发渲染

				for _, idx := range spaIndices {
					spaWg.Add(1)
					go func(i int) {
						defer spaWg.Done()
						sem <- struct{}{}
						defer func() { <-sem }()

						url := items[i].result.URL
						if rendered := renderWithBrowser(s.browserRenderer, url, s.debugLog, effectiveQuery); rendered != "" {
							items[i].body = rendered
							items[i].isSPA = false
						} else if s.debugLog != nil {
							s.debugLog("[网页搜索] SPA浏览器渲染失败 将回退到摘要 URL=%s body_len=%d",
								url, len([]rune(items[i].body)))
						}
					}(idx)
				}
				spaWg.Wait()
			}
		}

		// 阶段3：相关性过滤 + 组装结果
		for _, item := range items {
			i := item.idx
			r := item.result

			if item.err == nil && len(item.body) > 0 {
				body := truncateText(item.body, webpageMaxPerPageLen)
				wasSPA := item.isSPA

				if wasSPA && r.Snippet != "" {
					body = r.Snippet
				}

				relevanceScore := checkContentRelevance(queryKeywords, body, r.Title, r.Snippet, r.URL, r.IsOfficial)
				if wasSPA {
					relevanceScore = 1 // SPA渲染失败不杀，搜索引擎已排序
				}
				if relevanceScore < 1 {
					if s.debugLog != nil {
						s.debugLog("[网页搜索] 来源%d 相关性过滤跳过 URL=%s title=%q score=%d body_len=%d SPA=%v",
							i+1, r.URL, r.Title, relevanceScore, len([]rune(body)), wasSPA)
					}
					skipParts = append(skipParts, fmt.Sprintf(
						"[来源%d] %s (不相关，已跳过)", i+1, r.URL,
					))
					continue
				}

				if s.debugLog != nil {
					s.debugLog("[网页搜索] 来源%d 抓取成功 URL=%s title=%q body_len=%d SPA=%v score=%d",
						i+1, r.URL, r.Title, len([]rune(body)), wasSPA, relevanceScore)
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
				if s.debugLog != nil {
					s.debugLog("[网页搜索] 来源%d 抓取失败 URL=%s title=%q err=%v",
						i+1, r.URL, r.Title, item.err)
				}
				// 抓取失败时也对标题/摘要做相关性检查
				relevanceScore := checkContentRelevance(queryKeywords, "", r.Title, r.Snippet, r.URL, r.IsOfficial)
				if relevanceScore < 1 {
					skipParts = append(skipParts, fmt.Sprintf(
						"[来源%d] %s (抓取失败且不相关，已跳过)", i+1, r.URL,
					))
					continue
				}
				// 抓取失败时用摘要
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

	if s.debugLog != nil {
		s.debugLog("[网页搜索] 汇总 查询=%q 成功=%d 失败=%d 跳过=%d 总字符=%d",
			query, fetchSuccess, fetchFail, len(skipParts), totalContentChars)
	}

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
3. 直接呈现搜索结果中的信息，不要对信息来源做"可靠/不可靠"的主观判断
4. 如果信息来自非官方渠道，只需标注来源类型（如"第三方网站"），由用户自行判断可信度
5. 最后列出所有引用来源的URL

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

// SearchRaw 执行网页搜索+内容抓取，返回原始结果列表（不含LLM总结）
// 供大会辩论补充搜索使用：搜索 → 抓取全文 → 相关性过滤 → 返回带正文的SearchResult
func (s *WebpageSearcher) SearchRaw(query string) ([]SearchResult, error) {
	if s.debugLog != nil {
		s.debugLog("[网页搜索-Raw] 补充搜索 query=%q", query)
	}

	limit := min(max(s.cfg.MaxResults, 15), webpageMaxFetchResults)

	// 第一步：搜索
	results, err := s.simple.SearchRaw(query)
	if err != nil {
		return nil, fmt.Errorf("网页搜索失败: %w", err)
	}
	if len(results) == 0 {
		return nil, nil
	}
	if len(results) > limit {
		results = results[:limit]
	}

	// 过滤字典网站
	filteredResults := make([]SearchResult, 0, len(results))
	for _, r := range results {
		if isDictionarySite(r.URL) {
			continue
		}
		filteredResults = append(filteredResults, r)
	}
	results = filteredResults

	// 查询降级
	effectiveQuery := query
	fullQuery := strings.ToLower(query)
	metaMatch := 0
	for _, r := range results {
		if strings.Contains(strings.ToLower(r.Title+" "+r.Snippet), fullQuery) {
			metaMatch++
		}
	}
	if metaMatch == 0 {
		if coreEntity := extractCoreEntity(query); coreEntity != "" {
			if newResults, err := s.simple.SearchRaw(coreEntity); err == nil && len(newResults) > 0 {
				coreLower := strings.ToLower(coreEntity)
				degraded := make([]SearchResult, 0, len(newResults))
				seen := make(map[string]bool, len(newResults))
				for _, r := range newResults {
					if isDictionarySite(r.URL) || seen[r.URL] {
						continue
					}
					if strings.Contains(strings.ToLower(r.Title+" "+r.Snippet), coreLower) {
						degraded = append(degraded, r)
						seen[r.URL] = true
					}
				}
				if len(degraded) > 0 {
					results = degraded
					effectiveQuery = coreEntity
				}
			}
		}
	}
	if len(results) > limit {
		results = results[:limit]
	}

	// 第二步：抓取网页内容
	queryKeywords := extractQueryKeywords(effectiveQuery)
	var rawResults []SearchResult

	if s.cfg.FetchContent {
		type fetchItem struct {
			idx    int
			result SearchResult
			body   string
			isSPA  bool
			err    error
		}
		items := make([]fetchItem, len(results))
		for i := range results {
			body, err := s.fetchContentHTTPOnly(results[i].URL)
			isSPA := isSPAShell(body)
			items[i] = fetchItem{i, results[i], body, isSPA, err}
		}

		// SPA 浏览器渲染
		if s.browserRenderer != nil {
			var spaIndices []int
			for i := range items {
				if items[i].err == nil {
					isSPAEmpty := len([]rune(items[i].body)) < 200
					hasCoreKeywords := false
					if !isSPAEmpty && len(queryKeywords) > 0 {
						bodyLower := strings.ToLower(items[i].body)
						matchCount := 0
						for _, kw := range queryKeywords {
							if strings.Contains(bodyLower, strings.ToLower(kw)) {
								matchCount++
							}
						}
						hasCoreKeywords = matchCount >= (len(queryKeywords)+1)/2
					}
					if isSPAEmpty || !hasCoreKeywords {
						spaIndices = append(spaIndices, i)
					}
				}
			}
			if len(spaIndices) > 0 {
				var spaWg sync.WaitGroup
				sem := make(chan struct{}, 2)
				for _, idx := range spaIndices {
					spaWg.Add(1)
					go func(i int) {
						defer spaWg.Done()
						sem <- struct{}{}
						defer func() { <-sem }()
						url := items[i].result.URL
						if rendered := renderWithBrowser(s.browserRenderer, url, s.debugLog, effectiveQuery); rendered != "" {
							items[i].body = rendered
							items[i].isSPA = false
						}
					}(idx)
				}
				spaWg.Wait()
			}
		}

		// 相关性过滤 + 组装结果
		for _, item := range items {
			r := item.result
			if item.err == nil && len(item.body) > 0 {
				body := truncateText(item.body, webpageMaxPerPageLen)
				wasSPA := item.isSPA
				if wasSPA && r.Snippet != "" {
					body = r.Snippet
				}
				relevanceScore := checkContentRelevance(queryKeywords, body, r.Title, r.Snippet, r.URL, r.IsOfficial)
				if wasSPA {
					relevanceScore = 1
				}
				if relevanceScore < 1 {
					continue
				}
				r.Snippet = body // 用正文替换摘要
				rawResults = append(rawResults, r)
			} else {
				// 抓取失败时用摘要
				relevanceScore := checkContentRelevance(queryKeywords, "", r.Title, r.Snippet, r.URL, r.IsOfficial)
				if relevanceScore < 1 {
					continue
				}
				r.Snippet = truncateText(r.Snippet, 300)
				rawResults = append(rawResults, r)
			}
		}
	} else {
		for _, r := range results {
			r.Snippet = truncateText(r.Snippet, 300)
			rawResults = append(rawResults, r)
		}
	}

	if s.debugLog != nil {
		s.debugLog("[网页搜索-Raw] 补充搜索完成 query=%q 结果数=%d", query, len(rawResults))
	}

	return rawResults, nil
}

// SearchRawWithSelectiveFetch 补充搜索专用：简单搜索 + 选择性抓取前N条正文
// 策略：先用简单搜索拿到所有结果的标题+摘要（快），然后只对最相关的前2条抓取网页正文（有深度）
// 这样既保证了信息覆盖面（多条摘要），又保证了关键信息的深度（前2条正文），且节省时间和token
func (s *WebpageSearcher) SearchRawWithSelectiveFetch(query string, maxFetch int) ([]SearchResult, error) {
	if s.debugLog != nil {
		s.debugLog("[网页搜索-选择性抓取] 补充搜索 query=%q maxFetch=%d", query, maxFetch)
	}

	if maxFetch <= 0 {
		maxFetch = 2
	}

	// 第一步：简单搜索获取所有结果的标题+摘要
	results, err := s.simple.SearchRaw(query)
	if err != nil {
		return nil, fmt.Errorf("简单搜索失败: %w", err)
	}
	if len(results) == 0 {
		return nil, nil
	}

	// 过滤字典网站和重复URL
	seen := make(map[string]bool, len(results))
	filtered := make([]SearchResult, 0, len(results))
	for _, r := range results {
		if isDictionarySite(r.URL) || seen[r.URL] {
			continue
		}
		seen[r.URL] = true
		filtered = append(filtered, r)
	}
	results = filtered

	// 第二步：按相关性排序（标题匹配优先，其次摘要匹配）
	queryKeywords := extractQueryKeywords(query)
	queryLower := strings.ToLower(query)
	scored := make([]struct {
		result SearchResult
		score  int
	}, len(results))
	for i, r := range results {
		score := 0
		titleLower := strings.ToLower(r.Title)
		snippetLower := strings.ToLower(r.Snippet)
		// 完整查询在标题中出现：最高优先级
		if strings.Contains(titleLower, queryLower) {
			score += 10
		}
		// 完整查询在摘要中出现
		if strings.Contains(snippetLower, queryLower) {
			score += 5
		}
		// 关键词匹配
		for _, kw := range queryKeywords {
			kwLower := strings.ToLower(kw)
			if strings.Contains(titleLower, kwLower) {
				score += 3
			}
			if strings.Contains(snippetLower, kwLower) {
				score += 1
			}
		}
		// 官方网站加分
		if r.IsOfficial {
			score += 2
		}
		scored[i] = struct {
			result SearchResult
			score  int
		}{r, score}
	}

	// 按分数降序排序
	for i := 0; i < len(scored); i++ {
		for j := i + 1; j < len(scored); j++ {
			if scored[j].score > scored[i].score {
				scored[i], scored[j] = scored[j], scored[i]
			}
		}
	}

	// 第三步：对前 maxFetch 条结果抓取网页正文
	fetchCount := min(maxFetch, len(scored))
	for i := 0; i < fetchCount; i++ {
		r := scored[i].result
		if r.URL == "" {
			continue
		}
		body, err := s.fetchContentHTTPOnly(r.URL)
		if err != nil {
			if s.debugLog != nil {
				s.debugLog("[网页搜索-选择性抓取] 抓取失败 url=%q err=%v", r.URL, err)
			}
			continue
		}
		if len([]rune(body)) > 100 {
			// 抓取成功，用正文替换摘要（截断到合理长度）
			scored[i].result.Snippet = truncateText(body, 1500)
			if s.debugLog != nil {
				s.debugLog("[网页搜索-选择性抓取] 抓取成功 url=%q 正文长度=%d", r.URL, len([]rune(body)))
			}
		}
	}

	// 组装最终结果
	finalResults := make([]SearchResult, 0, len(scored))
	for _, s := range scored {
		finalResults = append(finalResults, s.result)
	}

	if s.debugLog != nil {
		s.debugLog("[网页搜索-选择性抓取] 补充搜索完成 query=%q 结果数=%d 抓取正文数=%d", query, len(finalResults), fetchCount)
	}

	return finalResults, nil
}

// fetchContentHTTPOnly 仅 HTTP 抓取 + 文本提取，不触发浏览器渲染
// 用于并行渲染前的批量 HTTP 抓取阶段
func (s *WebpageSearcher) fetchContentHTTPOnly(pageURL string) (string, error) {
	// 搜狗跳转链接：先解析真实URL
	realURL := pageURL
	if isSogouRedirect(pageURL) {
		if resolved := resolveSogouURL(s.httpClient, pageURL); resolved != "" {
			realURL = resolved
		}
	}

	req, err := http.NewRequest("GET", realURL, nil)
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

func (s *WebpageSearcher) fetchContent(pageURL string, queryKeywords []string) (string, error) {
	realURL := pageURL
	if isSogouRedirect(pageURL) {
		if resolved := resolveSogouURL(s.httpClient, pageURL); resolved != "" {
			realURL = resolved
		}
	}

	req, err := http.NewRequest("GET", realURL, nil)
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

	htmlStr := string(body)
	text := extractTextContent(htmlStr)

	if isSPAShell(text) {
		if s.browserRenderer != nil {
			if rendered := renderWithBrowser(s.browserRenderer, realURL, s.debugLog); rendered != "" {
				if s.debugLog != nil {
					s.debugLog("[网页搜索] SPA浏览器渲染成功 URL=%s html_len=%d rendered_len=%d",
						realURL, len([]rune(text)), len([]rune(rendered)))
				}
				text = rendered
			} else if s.debugLog != nil {
				s.debugLog("[网页搜索] SPA浏览器渲染失败 将回退到摘要 URL=%s html_len=%d",
					realURL, len([]rune(text)))
			}
		} else if s.debugLog != nil {
			s.debugLog("[网页搜索] SPA检测到但浏览器未初始化 URL=%s body_len=%d",
				realURL, len([]rune(text)))
		}
	}

	if len(queryKeywords) > 0 && isThinContent(text) {
		links := extractPageLinks(htmlStr)
		bestURLs := selectBestLinks(links, queryKeywords, realURL, 2)
		for _, detailURL := range bestURLs {
			detailBody, err := s.fetchRawContent(detailURL)
			if err == nil && len(detailBody) > 0 {
				detailText := extractTextContent(detailBody)
				if len([]rune(detailText)) > 100 {
					text = text + "\n\n[详情页内容]\n" + detailText
					if s.debugLog != nil {
						s.debugLog("[网页搜索] 薄页面跟随链接成功 列表页=%s 详情页=%s", realURL, detailURL)
					}
					break // 只取第一个成功的详情页
				}
			}
		}
	}

	return text, nil
}

// fetchRawContent 抓取页面原始HTML（不做链接跟踪）
func (s *WebpageSearcher) fetchRawContent(pageURL string) (string, error) {
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

	htmlStr := string(body)

	if s.browserRenderer != nil {
		text := extractTextContent(htmlStr)
		if isSPAShell(text) {
			if rendered, err := s.browserRenderer.Render(pageURL); err == nil && rendered != "" {
				if s.debugLog != nil {
					s.debugLog("[网页搜索] 详情页SPA浏览器渲染成功 URL=%s", pageURL)
				}
				return rendered, nil
			}
		}
	}

	return htmlStr, nil
}

// isSogouRedirect 检查 URL 是否为搜狗跳转链接
func isSogouRedirect(pageURL string) bool {
	return strings.Contains(pageURL, "sogou.com/link")
}

// resolveSogouURL 从搜狗跳转页面提取真实目标 URL
func resolveSogouURL(client *http.Client, sogouURL string) string {
	req, err := http.NewRequest("GET", sogouURL, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("User-Agent", defaultConfig.HTTP.UserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")

	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<18))
	if err != nil {
		return ""
	}

	htmlStr := string(body)
	lower := strings.ToLower(htmlStr)

	// 方法1：从 meta refresh 中提取 URL（noscript 回退路径）
	// 格式: <meta http-equiv="refresh" content="0;url='REAL_URL'">
	// 注意：URL 可能被引号包裹（单引号或双引号），需要跳过引号再提取
	metaStart := strings.Index(lower, "http-equiv=\"refresh\"")
	if metaStart < 0 {
		metaStart = strings.Index(lower, "http-equiv='refresh'")
	}
	if metaStart >= 0 {
		contentStart := strings.Index(lower[metaStart:], "content=")
		if contentStart >= 0 {
			contentStr := lower[metaStart+contentStart:]
			urlStart := strings.Index(contentStr, "url=")
			if urlStart >= 0 {
				valueStart := urlStart + 4
				// 跳过 url= 后面的引号（单引号或双引号）
				if valueStart < len(contentStr) && (contentStr[valueStart] == '\'' || contentStr[valueStart] == '"') {
					quoteChar := contentStr[valueStart]
					valueStart++ // 跳过开引号
					urlEnd := strings.IndexByte(contentStr[valueStart:], quoteChar)
					if urlEnd >= 0 {
						realURL := strings.TrimSpace(contentStr[valueStart : valueStart+urlEnd])
						if strings.HasPrefix(realURL, "http") {
							return realURL
						}
					}
				} else {
					// 无引号包裹，用分隔符截断
					urlEnd := strings.IndexAny(contentStr[valueStart:], "\"' ;<>")
					if urlEnd < 0 {
						urlEnd = len(contentStr) - valueStart
					}
					realURL := strings.TrimSpace(contentStr[valueStart : valueStart+urlEnd])
					if strings.HasPrefix(realURL, "http") {
						return realURL
					}
				}
			}
		}
	}

	// 方法2：从 JS location.replace / location.href 中提取 URL
	// 格式: window.location.replace("REAL_URL") 或 location.href="REAL_URL"
	locStart := strings.Index(lower, "location.replace")
	if locStart < 0 {
		locStart = strings.Index(lower, "location.href")
	}
	if locStart < 0 {
		locStart = strings.Index(lower, "location.assign")
	}
	if locStart >= 0 {
		quoteStart := strings.IndexAny(lower[locStart:], "'\"")
		if quoteStart >= 0 {
			afterQuote := lower[locStart+quoteStart+1:]
			quoteEnd := strings.IndexAny(afterQuote, "'\"")
			if quoteEnd > 0 {
				realURL := afterQuote[:quoteEnd]
				if strings.HasPrefix(realURL, "http") {
					return realURL
				}
			}
		}
	}

	return ""
}

var dictionaryDomains = []string{
	"hanyuguoxue.com",
	"hao86.com/ciyu",
	"cidianwang.com",
	"zdic.net",
	"hgcha.com/zidian",
	"hanyucool.com",
	"hanyuciyuan.com",
	"ufanv.cn/zidian",
	"gushici.net",
	"chagushici.com/zidian",
	"chaoxing.com",
}

func isDictionarySite(url string) bool {
	for _, domain := range dictionaryDomains {
		if strings.Contains(url, domain) {
			return true
		}
	}
	return false
}

// extractQueryKeywords 从查询字符串中提取关键词用于相关性判定
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

// extractCoreEntity 从查询末尾剥离常见修饰词，提取核心实体
func extractCoreEntity(query string) string {
	// 常见搜索修饰词（出现在查询末尾），按长度降序优先匹配长词
	modifiers := []string{
		"最近更新", "最新更新", "最新版本", "最新动态", "最新消息", "最新资讯",
		"是什么", "怎么了", "怎么样", "多少钱", "什么时候",
		"更新", "最新", "最近", "新闻", "公告", "攻略", "下载", "官网",
		"资讯", "动态", "消息", "版本", "简介", "介绍", "几号",
	}
	entity := query
	changed := true
	for changed {
		changed = false
		for _, mod := range modifiers {
			if strings.HasSuffix(entity, mod) && len([]rune(entity)) > len([]rune(mod)) {
				entity = strings.TrimSuffix(entity, mod)
				entity = strings.TrimRight(entity, " 。，、！？·-_/|~")
				changed = true
				break
			}
		}
	}
	entity = strings.TrimSpace(entity)
	if entity != query && len([]rune(entity)) >= 2 {
		return entity
	}
	return ""
}

// checkContentRelevance 检查内容与查询的相关性，返回匹配关键词数量
func checkContentRelevance(queryKeywords []string, content, title, snippet, url string, isOfficial bool) int {
	if len(queryKeywords) == 0 {
		return 2 // 无法判断时，默认保留
	}

	// 判断是否为官方网站：仅使用搜索引擎标记，不依赖硬编码域名
	isOfficialSite := isOfficial

	// 合并标题和摘要作为轻量级预检
	meta := strings.ToLower(title + " " + snippet)
	contentLower := strings.ToLower(content)

	// 检查完整查询词是否在标题/摘要中（queryKeywords[0] 是完整查询词）
	if len(queryKeywords) > 0 && containsFuzzy(meta, queryKeywords[0]) {
		if isOfficialSite {
			return 3 // 官方网站完整匹配，给予最高评分
		}
		return 2 // 完整查询词匹配，直接判定为相关
	}

	// 提取核心关键词（完整查询词 + 按空格拆分的词，排除2-gram）
	var coreKeywords []string
	coreKeywords = append(coreKeywords, queryKeywords[0]) // 完整查询词
	for _, kw := range queryKeywords[1:] {
		// 2-gram 是两个字符的，跳过它们，只保留完整的词
		if len([]rune(kw)) >= 2 && !isTwoCharGram(kw, queryKeywords[0]) {
			coreKeywords = append(coreKeywords, kw)
		}
	}

	// 统计标题/摘要中匹配的核心关键词数量
	metaMatchCount := 0
	for _, kw := range coreKeywords {
		if containsFuzzy(meta, kw) {
			metaMatchCount++
		}
	}

	// 统计内容中匹配的核心关键词数量
	contentMatchCount := 0
	for _, kw := range coreKeywords {
		if containsFuzzy(contentLower, kw) {
			contentMatchCount++
		}
	}

	// 总匹配数
	totalMatchCount := metaMatchCount + contentMatchCount

	// 标题/摘要匹配到核心关键词 → 搜索引擎已筛选，判定为相关
	if metaMatchCount >= 1 {
		// 对于短查询（核心关键词数量≤2），只要标题/摘要匹配了1个关键词就认为相关
		// 因为搜索引擎已经做了排序，短查询的部分匹配可能是正确结果
		if len(coreKeywords) <= 2 {
			if isOfficialSite {
				return metaMatchCount + 2 // 官方网站加分
			}
			return metaMatchCount + 1 // 加分，鼓励标题匹配
		}
		// 如果匹配了至少一半的核心关键词，直接认为相关
		if metaMatchCount >= (len(coreKeywords)+1)/2 {
			if isOfficialSite {
				return metaMatchCount + 2 // 官方网站加分
			}
			return metaMatchCount + 1 // 加分，鼓励标题匹配
		}
		// 如果只匹配了部分关键词，但内容也匹配了一些，也认为相关
		if totalMatchCount >= (len(coreKeywords)+1)/2 {
			if isOfficialSite {
				return totalMatchCount + 1 // 官方网站加分
			}
			return totalMatchCount
		}
		// 如果只匹配了部分关键词且内容也不匹配，降低评分但不直接跳过
		// 官方网站给予更高宽容度
		if isOfficialSite {
			return 2 // 官方网站即使部分匹配也保留较高评分
		}
		return 1 // 非官方网站保留评分1，让搜索引擎排序来决定优先级
	}

	// 标题/摘要无匹配 → 检查实际内容
	if contentMatchCount >= 1 {
		if isOfficialSite {
			return contentMatchCount + 1 // 官方网站加分
		}
		return contentMatchCount
	}

	// 完全无匹配 → 官方网站给予最低评分1，非官方网站返回0
	if isOfficialSite {
		return 1 // 官方网站即使无关键词匹配也保留，让用户自己判断
	}

	return 0
}

// isTwoCharGram 判断关键词是否是从完整查询词中提取的2-gram（两个字符的组合）
func isTwoCharGram(kw string, fullQuery string) bool {
	if len([]rune(kw)) != 2 {
		return false
	}
	return strings.Contains(strings.ToLower(fullQuery), kw)
}

// containsFuzzy 模糊包含检查：先精确匹配，若失败则去除中英文标点后再匹配
func containsFuzzy(text, keyword string) bool {
	if strings.Contains(text, keyword) {
		return true
	}
	return strings.Contains(normalizeForMatch(text), normalizeForMatch(keyword))
}

// normalizeForMatch 去除中英文标点符号和空格，用于模糊匹配
func normalizeForMatch(s string) string {
	var result []rune
	for _, r := range s {
		if isPunctuation(r) {
			continue
		}
		result = append(result, r)
	}
	return string(result)
}

// isPunctuation 判断是否为中英文标点或空格
func isPunctuation(r rune) bool {
	switch {
	case r == ' ':
		return true
	case r >= 0x2000 && r <= 0x206F: // General Punctuation (包括 … — 等)
		return true
	case r >= 0x3000 && r <= 0x303F: // CJK Symbols and Punctuation (。、〃 etc)
		return true
	case r >= 0xFF00 && r <= 0xFF0F: // Fullwidth forms (！＂＃ etc)
		return true
	case r >= 0xFF1A && r <= 0xFF20: // Fullwidth forms (：；＜ etc)
		return true
	case r >= 0xFF3B && r <= 0xFF40: // Fullwidth forms (［＼］ etc)
		return true
	case r >= 0xFF5B && r <= 0xFF65: // Fullwidth forms (｛｜｝ etc)
		return true
	case r == 0x00B7: // middle dot ·
		return true
	}
	return false
}
