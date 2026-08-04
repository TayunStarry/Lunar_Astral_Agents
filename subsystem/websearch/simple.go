package websearch

import "fmt"

// NewSimpleSearcher 创建轻量摘要搜索器
func NewSimpleSearcher(cfg Config) *SimpleSearcher {
	return &SimpleSearcher{
		baidu:      NewBaiduSearcher(cfg.HTTP),
		sogou:      NewSogouSearcher(cfg.HTTP),
		bing:       NewBingSearcher(cfg.HTTP),
		ddg:        NewDuckDuckGoSearcher(cfg.HTTP),
		maxResults: cfg.Simple.MaxResults,
		timeRange:  cfg.Simple.TimeRange,
	}
}

// SetMaxResults 设置最大结果数
func (s *SimpleSearcher) SetMaxResults(n int) {
	if n > 0 {
		s.maxResults = n
	}
}

// SetTimeRange 设置时间范围过滤
func (s *SimpleSearcher) SetTimeRange(tr TimeRange) {
	s.timeRange = tr
}

// Search 轻量摘要搜索，Bing → 百度 → 搜狗 → DuckDuckGo 依次尝试
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
		{s.bing, "Bing"},
		{s.baidu, "百度"},
		{s.sogou, "搜狗"},
		{s.ddg, "DuckDuckGo"},
	}

	for _, eng := range engines {
		if eng.searcher == nil {
			continue
		}
		results, err := eng.searcher.SearchWithTimeRange(query, limit, s.timeRange)
		if err == nil && len(results) > 0 {
			ScoreResults(results)
			return formatResults(results), nil
		}
	}

	return fmt.Sprintf("未找到与 %q 相关的搜索结果，所有搜索引擎均无结果。", query), nil
}

// SearchRaw 轻量摘要搜索，返回原始结果列表
func (s *SimpleSearcher) SearchRaw(query string) ([]SearchResult, error) {
	limit := max(s.maxResults, 10)

	results, err := s.trySearchRaw(query, limit)
	if err != nil {
		return nil, fmt.Errorf("轻量摘要搜索全部失败: %w", err)
	}
	ScoreResults(results)
	SortResults(results, query)
	return results, nil
}

// SearchRawNoPrep 跳过预处理的原始搜索（用于回退策略）
func (s *SimpleSearcher) SearchRawNoPrep(query string) ([]SearchResult, error) {
	limit := max(s.maxResults, 10)

	results, err := s.trySearchRawNoPrep(query, limit)
	if err != nil {
		return nil, fmt.Errorf("原始搜索全部失败: %w", err)
	}
	ScoreResults(results)
	SortResults(results, query)
	return results, nil
}

// trySearchRaw 依次尝试各引擎，跳过已降级引擎
func (s *SimpleSearcher) trySearchRaw(query string, limit int) ([]SearchResult, error) {
	engines := []Searcher{s.bing, s.baidu, s.sogou, s.ddg}
	for _, eng := range engines {
		if eng == nil {
			continue
		}
		// 健康检查：跳过已降级引擎
		if s.health != nil && s.health.IsDegraded(eng.Name()) {
			if s.debugLog != nil {
				s.debugLog("[引擎选择] %s 已降级，跳过", eng.Name())
			}
			continue
		}
		results, err := eng.SearchWithTimeRange(query, limit, s.timeRange)
		if s.health != nil {
			s.health.RecordResult(eng.Name(), err == nil && len(results) > 0)
		}
		if err != nil {
			if s.debugLog != nil {
				s.debugLog("[引擎选择] %s 搜索出错 err=%v", eng.Name(), err)
			}
			continue
		}
		if len(results) == 0 {
			if s.debugLog != nil {
				s.debugLog("[引擎选择] %s 返回空结果", eng.Name())
			}
			continue
		}
		if s.debugLog != nil {
			s.debugLog("[引擎选择] %s 命中，返回%d条结果", eng.Name(), len(results))
		}
		return results, nil
	}
	return nil, nil
}

// trySearchRawNoPrep 依次尝试各引擎的原始搜索（跳过预处理）
func (s *SimpleSearcher) trySearchRawNoPrep(query string, limit int) ([]SearchResult, error) {
	engines := []Searcher{s.bing, s.baidu, s.sogou, s.ddg}
	for _, eng := range engines {
		if eng == nil {
			continue
		}
		// 健康检查：跳过已降级引擎
		if s.health != nil && s.health.IsDegraded(eng.Name()) {
			if s.debugLog != nil {
				s.debugLog("[引擎选择] %s 已降级，跳过", eng.Name())
			}
			continue
		}
		results, err := eng.SearchWithTimeRange(query, limit, s.timeRange)
		if s.health != nil {
			s.health.RecordResult(eng.Name(), err == nil && len(results) > 0)
		}
		if err != nil {
			if s.debugLog != nil {
				s.debugLog("[引擎选择] %s 搜索出错 err=%v", eng.Name(), err)
			}
			continue
		}
		if len(results) == 0 {
			if s.debugLog != nil {
				s.debugLog("[引擎选择] %s 返回空结果", eng.Name())
			}
			continue
		}
		if s.debugLog != nil {
			s.debugLog("[引擎选择] %s 命中，返回%d条结果", eng.Name(), len(results))
		}
		return results, nil
	}
	return nil, nil
}
