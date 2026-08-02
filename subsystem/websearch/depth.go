package websearch

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode"
)

// NewDepthSearcher 创建 深度研究 搜索器
func NewDepthSearcher(simpleSearcher *SimpleSearcher, llmProvider Provider, cfg DepthConfig, httpCfg HTTPConfig) *DepthSearcher {
	return &DepthSearcher{
		simple:      simpleSearcher,
		llmProvider: llmProvider,
		cfg:         cfg,
		httpClient:  &http.Client{Timeout: time.Duration(httpCfg.Timeout) * time.Second},
		userAgent:   httpCfg.UserAgent,
	}
}

// Search 执行深度研究
func (s *DepthSearcher) Search(ctx context.Context, query string) (string, error) {
	// 第一步：判断是否需要拆解子问题
	var subQueries []string
	if isProperNounQuery(query) {
		// 短查询/专有名词不拆解，直接搜索
		subQueries = []string{query}
	} else {
		decomposed, err := s.decomposeQuery(ctx, query)
		if err != nil || len(decomposed) == 0 {
			subQueries = []string{query}
		} else {
			subQueries = decomposed
			// 确保原始查询在子问题中（保持原始搜索意图）
			if !slices.Contains(subQueries, query) {
				subQueries = append([]string{query}, subQueries...)
			}
		}
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

	// 并发限流（固定为3，避免对搜索引擎造成过大压力）
	sem := make(chan struct{}, 3)

	for _, sq := range subQueries {
		wg.Add(1)
		go func(q string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			results, searchErr := s.simple.SearchRaw(q)
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

	// 第三步：抓取网页内容
	s.fetchContentForResults(allResults, query)

	// 第四步：汇总生成报告
	return s.generateReport(query, allResults)
}

// CollectData 采集结构化研究数据，供大会辩论使用
// 执行子问题拆解、并行搜索、网页内容抓取、URL去重，但不生成最终报告
func (s *DepthSearcher) CollectData(ctx context.Context, query string) (*ResearchData, error) {
	// 第一步：判断是否需要拆解子问题
	var subQueries []string
	if isProperNounQuery(query) {
		subQueries = []string{query}
	} else {
		decomposed, err := s.decomposeQuery(ctx, query)
		if err != nil || len(decomposed) == 0 {
			subQueries = []string{query}
		} else {
			subQueries = decomposed
			if !slices.Contains(subQueries, query) {
				subQueries = append([]string{query}, subQueries...)
			}
		}
	}

	maxSub := s.cfg.MaxSubQueries
	if maxSub <= 0 {
		maxSub = 6
	}
	if len(subQueries) > maxSub {
		subQueries = subQueries[:maxSub]
	}

	// 第二步：并行搜索每个子问题
	var wg sync.WaitGroup
	resultCh := make(chan subResult, len(subQueries))

	// 并发限流（固定为3，避免对搜索引擎造成过大压力）
	sem := make(chan struct{}, 3)

	for _, sq := range subQueries {
		wg.Add(1)
		go func(q string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			results, searchErr := s.simple.SearchRaw(q)
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

	// URL去重
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

	// 第三步：抓取网页内容（填充到 Snippet 字段）
	s.fetchContentForResults(allResults, query)

	// 转换为 ResearchData
	data := &ResearchData{
		OriginalQuery: query,
		SubQueries:    make([]SubQueryResult, len(allResults)),
	}
	for i, r := range allResults {
		data.SubQueries[i] = SubQueryResult{
			Query:   r.Query,
			Results: r.Results,
			Error:   r.Error,
		}
	}

	return data, nil
}

// fetchContentForResults 为搜索结果抓取网页内容
func (s *DepthSearcher) fetchContentForResults(results []subResult, query string) {
	if len(results) == 0 {
		return
	}

	// 阶段1：HTTP 抓取所有结果，收集 SPA 索引
	type fetchItem struct {
		subIdx int
		resIdx int
		body   string
		isSPA  bool
	}
	var allItems []fetchItem
	var spaItems []int // 指向 allItems 的索引

	for subIdx := range results {
		topResult := &results[subIdx]
		fetchLimit := min(5, len(topResult.Results))

		for i := range fetchLimit {
			r := &topResult.Results[i]
			if r.URL == "" {
				continue
			}
			body, err := s.fetchContentHTTPOnly(r.URL)
			if err != nil || len(body) == 0 {
				continue
			}
			isSPA := isSPAShell(body)
			itemIdx := len(allItems)
			allItems = append(allItems, fetchItem{subIdx, i, body, isSPA})
			if isSPA {
				spaItems = append(spaItems, itemIdx)
			}
		}
	}

	// 阶段2：并行浏览器渲染所有 SPA 页面
	if s.browserRenderer != nil && len(spaItems) > 0 {
		if s.debugLog != nil {
			s.debugLog("[深度搜索] 开始并行渲染 %d 个SPA页面", len(spaItems))
		}
		sem := make(chan struct{}, 3) // 最多3并发
		var wg sync.WaitGroup
		for _, itemIdx := range spaItems {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				item := &allItems[idx]
				url := results[item.subIdx].Results[item.resIdx].URL
				if rendered := renderWithBrowser(s.browserRenderer, url, s.debugLog, query); rendered != "" {
					item.body = rendered
					item.isSPA = false
				}
			}(itemIdx)
		}
		wg.Wait()
	}

	// 阶段3：回写结果
	for _, item := range allItems {
		r := &results[item.subIdx].Results[item.resIdx]
		body := truncateText(item.body, depthMaxFetchedContentLen)
		if item.isSPA && r.Snippet != "" {
			body = r.Snippet
		}
		r.Snippet = body
	}
}

// fetchContentHTTPOnly 仅 HTTP 抓取 + 文本提取，不触发浏览器渲染
func (s *DepthSearcher) fetchContentHTTPOnly(pageURL string) (string, error) {
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
	req.Header.Set("User-Agent", s.userAgent)
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

func (s *DepthSearcher) fetchContent(pageURL string, queryKeywords []string) (string, error) {
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
	req.Header.Set("User-Agent", s.userAgent)
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
				text = rendered
			} else if s.debugLog != nil {
				s.debugLog("[深度搜索] SPA浏览器渲染失败 将回退到摘要 URL=%s body_len=%d",
					realURL, len([]rune(text)))
			}
		} else if s.debugLog != nil {
			s.debugLog("[深度搜索] SPA检测到但浏览器未初始化 URL=%s body_len=%d",
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
					break
				}
			}
		}
	}

	return text, nil
}

// fetchRawContent 抓取页面原始HTML（不做链接跟踪）
func (s *DepthSearcher) fetchRawContent(pageURL string) (string, error) {
	req, err := http.NewRequest("GET", pageURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", s.userAgent)
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
				return rendered, nil
			}
		}
	}

	return htmlStr, nil
}

// isProperNounQuery 判断查询是否为专有名词或短查询
func isProperNounQuery(query string) bool {
	runes := []rune(query)

	// 包含疑问词的查询应拆解，不视为专有名词（优先判断）
	questionWords := []string{"什么", "怎么", "如何", "为什么", "哪", "吗", "呢", "多少", "几", "是否", "有哪些", "哪些", "谁", "何时", "何地"}
	for _, w := range questionWords {
		if strings.Contains(query, w) {
			return false
		}
	}

	// 包含特殊分隔符的查询视为专有名词/人名/品牌名
	for _, r := range runes {
		if r == '·' || r == '-' || r == '—' || r == '～' || r == '~' || r == '|' || r == '•' || r == '/' {
			return true
		}
	}

	// 极短查询（≤4个字符）不拆解，如单个词"超自然"
	if len(runes) <= 4 {
		return true
	}

	// 无空格的中文字符串（可能是专有名词）
	hasSpace := strings.Contains(query, " ") || strings.Contains(query, "　")
	if !hasSpace {
		chineseCount := 0
		for _, r := range runes {
			if unicode.Is(unicode.Han, r) {
				chineseCount++
			}
		}
		// 纯中文且≤4字，可能是专有名词（如人名、地名、游戏名）
		// 超过4字的纯中文查询（如"最近的大新闻"）应拆解
		if chineseCount == len(runes) && len(runes) <= 4 {
			return true
		}
	}

	return false
}

func (s *DepthSearcher) decomposeQuery(ctx context.Context, query string) ([]string, error) {
	if s.llmProvider == nil {
		return []string{query}, nil
	}

	prompt := fmt.Sprintf(`你是一个研究助手。请将以下用户问题拆解为2-5个更具体的子问题，以便进行全面深入的网络搜索。

用户问题：%s

要求：
1. 保留原始问题中的专有名词、产品名、人名等完整词语，不要将其拆分
2. 每个子问题应聚焦于问题的不同方面或维度
3. 子问题之间应有互补性，避免重复
4. 用JSON数组格式输出，只输出数组，不要其他内容

示例输出：["子问题1", "子问题2", "子问题3"]`, query)

	messages := []ChatMessage{
		{Role: "user", Content: prompt},
	}

	// 检查上下文是否已取消
	if ctx.Err() != nil {
		return nil, ctx.Err()
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
func (s *DepthSearcher) generateReport(originalQuery string, results []subResult) (string, error) {
	// 格式化子问题结果，控制总注入量
	var parts []string
	totalChars := 0

	for i, r := range results {
		var part string
		if r.Error != nil {
			part = fmt.Sprintf("## 子问题%d：%s\n搜索失败：%v", i+1, r.Query, r.Error)
		} else {
			// 使用截断格式化
			formatted := formatResultsTruncated(r.Results, depthMaxSnippetLen)
			part = fmt.Sprintf("## 子问题%d：%s\n%s", i+1, r.Query, formatted)
		}

		partLen := len([]rune(part))
		// 预算检查：超过上限则截断
		if totalChars+partLen > depthMaxSubResultsChars {
			remaining := depthMaxSubResultsChars - totalChars
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
		fallback := fmt.Sprintf("# 深度研究报告\n\n原始问题：%s\n\n%s", originalQuery, allResults)
		// 回退也截断
		fallbackRunes := []rune(fallback)
		if len(fallbackRunes) > depthMaxOutputChars {
			fallback = string(fallbackRunes[:depthMaxOutputChars]) + "\n\n[报告已截断]"
		}
		return fallback, nil
	}

	// 构建prompt，带预算控制
	promptTemplate := `你是一个研究助手。请基于以下多个子问题的搜索结果，生成一份结构化的研究报告。

原始问题：%s

各子问题搜索结果：
%s

要求：
1. 以"# 深度研究报告"开头
2. 包含"核心发现"章节（3-5点总结）
3. 包含"详细分析"章节，按维度分点阐述
4. 包含"信息来源"章节，列出所有引用的来源
5. 直接呈现搜索结果中的信息，不要对信息来源做"可靠/不可靠"的主观判断，由用户自行判断
6. 用markdown格式输出`

	prompt := fmt.Sprintf(promptTemplate, originalQuery, allResults)

	// prompt预算控制
	promptRunes := []rune(prompt)
	if len(promptRunes) > depthMaxPromptChars {
		// 计算需要从allResults中砍掉多少
		overhead := len([]rune(fmt.Sprintf(promptTemplate, originalQuery, "")))
		available := depthMaxPromptChars - overhead
		available = max(available, 500)
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
		fallback := fmt.Sprintf("# 深度研究报告\n\n原始问题：%s\n\n%s", originalQuery, allResults)
		fallbackRunes := []rune(fallback)
		if len(fallbackRunes) > depthMaxOutputChars {
			fallback = string(fallbackRunes[:depthMaxOutputChars]) + "\n\n[报告已截断]"
		}
		return fallback, nil
	}

	// 输出截断
	responseRunes := []rune(response)
	if len(responseRunes) > depthMaxOutputChars {
		response = string(responseRunes[:depthMaxOutputChars]) + "\n\n[报告已截断]"
	}

	return response, nil
}

// SupplementarySearch 补充搜索（供大会辩论中途使用）
func (s *DepthSearcher) SupplementarySearch(query string) ([]SearchResult, error) {
	results, err := s.simple.SearchRaw(query)
	if err != nil {
		return nil, err
	}
	// 为结果抓取网页内容
	for i := range results {
		if results[i].URL != "" {
			content, fetchErr := s.fetchContent(results[i].URL, nil)
			if fetchErr == nil && content != "" {
				results[i].Snippet = content
			}
		}
	}
	return results, nil
}
