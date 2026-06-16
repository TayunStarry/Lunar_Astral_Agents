package websearch

import "fmt"

// NewSimpleSearcher 创建轻量摘要搜索器
func NewSimpleSearcher(cfg Config) *SimpleSearcher {
	return &SimpleSearcher{
		bing:       NewBingSearcher(cfg.HTTP.Timeout, cfg.HTTP.UserAgent),
		ddg:        NewDuckDuckGoSearcher(cfg.HTTP.Timeout, cfg.HTTP.UserAgent),
		maxResults: cfg.Simple.MaxResults,
	}
}

// NewSimpleSearcherWithEngine 使用自定义搜索引擎创建轻量摘要搜索器
func NewSimpleSearcherWithEngine(bing, ddg Searcher, maxResults int) *SimpleSearcher {
	return &SimpleSearcher{
		bing:       bing,
		ddg:        ddg,
		maxResults: maxResults,
	}
}

// SetMaxResults 设置最大结果数
func (s *SimpleSearcher) SetMaxResults(n int) {
	if n > 0 {
		s.maxResults = n
	}
}

// Search 执行轻量摘要搜索，Bing 优先，失败回退到 DuckDuckGo
func (s *SimpleSearcher) Search(query string) (string, error) {
	limit := s.maxResults
	if limit <= 0 {
		limit = 10
	}

	results, err := s.bing.Search(query, limit)
	if err == nil && len(results) > 0 {
		return formatResults(results), nil
	}

	results, err = s.ddg.Search(query, limit)
	if err != nil {
		return "", fmt.Errorf("轻量摘要搜索全部失败: %w", err)
	}

	if len(results) == 0 {
		return fmt.Sprintf("未找到与 %q 相关的搜索结果。", query), nil
	}

	return formatResults(results), nil
}

// SearchRaw 执行轻量摘要搜索，返回原始结果（不格式化）
func (s *SimpleSearcher) SearchRaw(query string) ([]SearchResult, error) {
	// 硬性上限：最少抓取10条
	limit := max(s.maxResults, 10)

	results, err := s.bing.Search(query, limit)
	if err == nil && len(results) > 0 {
		return results, nil
	}

	results, err = s.ddg.Search(query, limit)
	if err != nil {
		return nil, fmt.Errorf("轻量摘要搜索全部失败: %w", err)
	}

	return results, nil
}
