package websearch

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// NewDeepSearcher 创建深层搜索器
func NewDeepSearcher(shallowSearcher *ShallowSearcher, llmProvider Provider, cfg DeepConfig, httpCfg HTTPConfig) *DeepSearcher {
	return &DeepSearcher{
		shallow:     shallowSearcher,
		llmProvider: llmProvider,
		cfg:         cfg,
		httpClient:  &http.Client{Timeout: time.Duration(cfg.FetchTimeout) * time.Second},
	}
}

// Search 执行深层搜索
func (s *DeepSearcher) Search(query string) (string, error) {
	limit := s.cfg.MaxResults
	if limit <= 0 {
		limit = 30
	}

	// 第一步：搜索
	results, err := s.shallow.SearchRaw(query)
	if err != nil {
		return "", fmt.Errorf("深层搜索失败: %w", err)
	}

	if len(results) == 0 {
		return fmt.Sprintf("未找到与 %q 相关的搜索结果。", query), nil
	}
	if len(results) > limit {
		results = results[:limit]
	}

	// 第二步：抓取网页内容
	contentParts := make([]string, 0, len(results))
	maxContentLen := s.cfg.MaxContentLength
	if maxContentLen <= 0 {
		maxContentLen = 2000
	}

	for i, r := range results {
		if s.cfg.FetchContent {
			body, fetchErr := s.fetchContent(r.URL)
			if fetchErr == nil && len(body) > 0 {
				body = truncateText(body, maxContentLen)
				contentParts = append(contentParts, fmt.Sprintf(
					"[来源%d] %s\nURL: %s\n内容:\n%s",
					i+1, r.Title, r.URL, body,
				))
			} else {
				contentParts = append(contentParts, fmt.Sprintf(
					"[来源%d] %s\n摘要: %s\nURL: %s",
					i+1, r.Title, r.Snippet, r.URL,
				))
			}
		} else {
			contentParts = append(contentParts, fmt.Sprintf(
				"[来源%d] %s\n摘要: %s\nURL: %s",
				i+1, r.Title, r.Snippet, r.URL,
			))
		}
	}

	// 第三步：LLM 总结
	if s.llmProvider == nil {
		return formatDeepResultsFallback(query, contentParts), nil
	}

	result, err := s.summarizeWithLLM(query, contentParts)
	if err != nil {
		return formatDeepResultsFallback(query, contentParts), nil
	}

	return result, nil
}

func (s *DeepSearcher) summarizeWithLLM(query string, contentParts []string) (string, error) {
	prompt := fmt.Sprintf(`请基于以下搜索结果，对用户问题"%s"进行综合分析回答。

要求：
1. 先给出一个简洁的总结（2-3句话）
2. 然后分点列出关键信息，每个要点标注来源编号
3. 最后列出所有引用来源的URL

搜索结果：
%s`, query, strings.Join(contentParts, "\n\n---\n\n"))

	messages := []ChatMessage{
		{Role: "user", Content: prompt},
	}

	response, err := s.llmProvider.Chat(messages)
	if err != nil {
		return "", err
	}

	return response, nil
}

func (s *DeepSearcher) fetchContent(pageURL string) (string, error) {
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
