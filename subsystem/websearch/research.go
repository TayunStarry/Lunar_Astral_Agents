package websearch

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

// NewResearchSearcher 创建研究搜索器
func NewResearchSearcher(shallowSearcher *ShallowSearcher, llmProvider Provider, cfg ResearchConfig) *ResearchSearcher {
	return &ResearchSearcher{
		shallow:     shallowSearcher,
		llmProvider: llmProvider,
		cfg:         cfg,
	}
}

// Search 执行研究搜索：拆解子问题 → 并行搜索 → 综合报告
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

	// 第二步：并行搜索每个子问题
	var wg sync.WaitGroup
	resultCh := make(chan subResult, len(subQueries))

	for _, sq := range subQueries {
		wg.Add(1)
		go func(q string) {
			defer wg.Done()
			res, err := s.shallow.Search(q)
			resultCh <- subResult{Query: q, Result: res, Error: err}
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

func (s *ResearchSearcher) generateReport(originalQuery string, results []subResult) (string, error) {
	var parts []string
	for i, r := range results {
		if r.Error != nil {
			parts = append(parts, fmt.Sprintf("## 子问题%d：%s\n搜索失败：%v", i+1, r.Query, r.Error))
		} else {
			parts = append(parts, fmt.Sprintf("## 子问题%d：%s\n%s", i+1, r.Query, r.Result))
		}
	}

	allResults := strings.Join(parts, "\n\n")

	if s.llmProvider == nil {
		return fmt.Sprintf("# 研究搜索报告\n\n原始问题：%s\n\n%s", originalQuery, allResults), nil
	}

	prompt := fmt.Sprintf(`你是一个研究助手。请基于以下多个子问题的搜索结果，生成一份结构化的研究报告。

原始问题：%s

各子问题搜索结果：
%s

要求：
1. 以"# 研究搜索报告"开头
2. 包含"核心发现"章节（3-5点总结）
3. 包含"详细分析"章节，按维度分点阐述
4. 包含"信息来源"章节，列出所有引用的来源
5. 保持专业、客观的语气
6. 用markdown格式输出`, originalQuery, allResults)

	messages := []ChatMessage{
		{Role: "user", Content: prompt},
	}

	response, err := s.llmProvider.Chat(messages)
	if err != nil {
		return fmt.Sprintf("# 研究搜索报告\n\n原始问题：%s\n\n%s", originalQuery, allResults), nil
	}

	return response, nil
}
