package websearch

import "fmt"

// NewShallowSearcher 创建浅层搜索器
func NewShallowSearcher(cfg Config) *ShallowSearcher {
	return &ShallowSearcher{
		bing:       NewBingSearcher(cfg.HTTP.Timeout, cfg.HTTP.UserAgent),
		ddg:        NewDuckDuckGoSearcher(cfg.HTTP.Timeout, cfg.HTTP.UserAgent),
		maxResults: cfg.Shallow.MaxResults,
	}
}

// NewShallowSearcherWithEngine 使用自定义搜索引擎创建浅层搜索器
func NewShallowSearcherWithEngine(bing, ddg Searcher, maxResults int) *ShallowSearcher {
	return &ShallowSearcher{
		bing:       bing,
		ddg:        ddg,
		maxResults: maxResults,
	}
}

// SetMaxResults 设置最大结果数
func (s *ShallowSearcher) SetMaxResults(n int) {
	if n > 0 {
		s.maxResults = n
	}
}

// Search 执行浅层搜索，Bing 优先，失败回退到 DuckDuckGo
func (s *ShallowSearcher) Search(query string) (string, error) {
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
		return "", fmt.Errorf("浅层搜索全部失败: %w", err)
	}

	if len(results) == 0 {
		return fmt.Sprintf("未找到与 %q 相关的搜索结果。", query), nil
	}

	return formatResults(results), nil
}

// SearchRaw 执行浅层搜索，返回原始结果（不格式化）
func (s *ShallowSearcher) SearchRaw(query string) ([]SearchResult, error) {
	// 硬性上限：最少抓取10条
	limit := max(s.maxResults, 10)

	results, err := s.bing.Search(query, limit)
	if err == nil && len(results) > 0 {
		return results, nil
	}

	results, err = s.ddg.Search(query, limit)
	if err != nil {
		return nil, fmt.Errorf("浅层搜索全部失败: %w", err)
	}

	return results, nil
}
