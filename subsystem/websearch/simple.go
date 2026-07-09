package websearch

import "fmt"

// NewSimpleSearcher 创建轻量摘要搜索器
func NewSimpleSearcher(cfg Config) *SimpleSearcher {
	return &SimpleSearcher{
		baidu:      NewBaiduSearcher(cfg.HTTP.Timeout, cfg.HTTP.UserAgent),
		sogou:      NewSogouSearcher(cfg.HTTP.Timeout, cfg.HTTP.UserAgent),
		bing:       NewBingSearcher(cfg.HTTP.Timeout, cfg.HTTP.UserAgent),
		ddg:        NewDuckDuckGoSearcher(cfg.HTTP.Timeout, cfg.HTTP.UserAgent),
		maxResults: cfg.Simple.MaxResults,
	}
}

// SetMaxResults 设置最大结果数
func (s *SimpleSearcher) SetMaxResults(n int) {
	if n > 0 {
		s.maxResults = n
	}
}

// Search 执行轻量摘要搜索，百度 → 搜狗 → Bing → DuckDuckGo 级联回退
func (s *SimpleSearcher) Search(query string) (string, error) {
	limit := s.maxResults
	if limit <= 0 {
		limit = 10
	}

	// 顺序尝试各引擎
	engines := []struct {
		searcher Searcher
		name     string
	}{
		{s.baidu, "百度"},
		{s.sogou, "搜狗"},
		{s.bing, "Bing"},
		{s.ddg, "DuckDuckGo"},
	}

	for _, eng := range engines {
		if eng.searcher == nil {
			continue
		}
		results, err := eng.searcher.Search(query, limit)
		if err == nil && len(results) > 0 {
			return formatResults(results), nil
		}
	}

	return fmt.Sprintf("未找到与 %q 相关的搜索结果，所有搜索引擎均无结果。", query), nil
}

// SearchRaw 执行轻量摘要搜索，返回原始结果（百度 → 搜狗 → Bing → DDG）
func (s *SimpleSearcher) SearchRaw(query string) ([]SearchResult, error) {
	limit := max(s.maxResults, 10)

	results, err := s.trySearchRaw(query, limit)
	if err != nil {
		return nil, fmt.Errorf("轻量摘要搜索全部失败: %w", err)
	}
	return results, nil
}

// SearchRawNoPrep 跳过预处理的原始搜索（用于回退策略），百度 → 搜狗 → Bing → DDG
func (s *SimpleSearcher) SearchRawNoPrep(query string) ([]SearchResult, error) {
	limit := max(s.maxResults, 10)

	results, err := s.trySearchRawNoPrep(query, limit)
	if err != nil {
		return nil, fmt.Errorf("原始搜索全部失败: %w", err)
	}
	return results, nil
}

// trySearchRaw 依次尝试各引擎的标准搜索（带预处理）
func (s *SimpleSearcher) trySearchRaw(query string, limit int) ([]SearchResult, error) {
	engines := []Searcher{s.baidu, s.sogou, s.bing, s.ddg}
	for _, eng := range engines {
		if eng == nil {
			continue
		}
		results, err := eng.Search(query, limit)
		if err == nil && len(results) > 0 {
			return results, nil
		}
	}
	return nil, nil
}

// trySearchRawNoPrep 依次尝试各引擎的原始搜索（跳过预处理）
func (s *SimpleSearcher) trySearchRawNoPrep(query string, limit int) ([]SearchResult, error) {
	engines := []Searcher{s.baidu, s.sogou, s.bing, s.ddg}
	for _, eng := range engines {
		if eng == nil {
			continue
		}
		results, err := eng.SearchRaw(query, limit)
		if err == nil && len(results) > 0 {
			return results, nil
		}
	}
	return nil, nil
}
