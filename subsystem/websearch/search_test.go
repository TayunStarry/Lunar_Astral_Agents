package websearch

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// ============================================================
// Mock LLM Provider
// ============================================================

type mockLLM struct {
	chatFunc func(messages []ChatMessage) (string, error)
}

func (m *mockLLM) Chat(messages []ChatMessage) (string, error) {
	return m.chatFunc(messages)
}

// ============================================================
// Mock 搜索引擎
// ============================================================

type simpleMockSearcher struct {
	results []SearchResult
	err     error
}

func (s *simpleMockSearcher) Search(query string, limit int) ([]SearchResult, error) {
	if s.err != nil {
		return nil, s.err
	}
	if len(s.results) > limit {
		return s.results[:limit], nil
	}
	return s.results, nil
}

func (s *simpleMockSearcher) Name() string { return "SimpleMock" }

// ============================================================
// 工具函数测试
// ============================================================

func TestTruncateText(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		maxLen int
		expect string
	}{
		{"短文本不截断", "hello", 100, "hello"},
		{"长文本截断", "hello world this is a long text", 10, "hello worl..."},
		{"中文截断", "你好世界这是一个很长的文本", 5, "你好世界这..."},
		{"精确长度", "abc", 3, "abc"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateText(tt.input, tt.maxLen)
			if got != tt.expect {
				t.Errorf("truncateText(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.expect)
			}
		})
	}
}

func TestExtractTextContent(t *testing.T) {
	html := `<!DOCTYPE html><html><head><script>var x = 1;</script><style>body{}</style></head>
<body><nav>导航</nav><article><p>这是正文内容</p><p>第二段</p></article><footer>页脚</footer></body></html>`

	text := extractTextContent(html)

	if !strings.Contains(text, "这是正文内容") {
		t.Errorf("应包含正文内容，实际: %s", text)
	}
	if strings.Contains(text, "var x = 1") {
		t.Errorf("不应包含 script 内容，实际: %s", text)
	}
	if strings.Contains(text, "导航") {
		t.Errorf("不应包含 nav 内容，实际: %s", text)
	}
}

// ============================================================
// 格式化测试
// ============================================================

func TestFormatResults(t *testing.T) {
	results := []SearchResult{
		{Title: "标题1", Snippet: "摘要1"},
		{Title: "标题2", Snippet: ""},
		{Title: "标题3", Snippet: "摘要3"},
	}

	output := formatResults(results)

	if !strings.Contains(output, "「标题1」：摘要1") {
		t.Errorf("期望包含「标题1」：摘要1，实际: %s", output)
	}
	if !strings.Contains(output, "「标题2」") {
		t.Errorf("期望包含「标题2」，实际: %s", output)
	}
}

func TestFormatResultsForLLM(t *testing.T) {
	results := []SearchResult{
		{Title: "测试标题", URL: "https://example.com", Snippet: "测试摘要"},
	}

	output := formatResultsForLLM(results)

	if !strings.Contains(output, "**测试标题**") {
		t.Errorf("期望包含 **测试标题**，实际: %s", output)
	}
	if !strings.Contains(output, "来源: https://example.com") {
		t.Errorf("期望包含来源 URL，实际: %s", output)
	}
}

// ============================================================
// 浅层搜索测试
// ============================================================

func TestShallowSearch_WithMockEngines(t *testing.T) {
	bing := &simpleMockSearcher{
		results: []SearchResult{
			{Title: "结果1", URL: "https://example.com/1", Snippet: "摘要1"},
			{Title: "结果2", URL: "https://example.com/2", Snippet: "摘要2"},
		},
	}
	ddg := &simpleMockSearcher{
		results: []SearchResult{
			{Title: "DDG结果1", URL: "https://example.com/d1", Snippet: "DDG摘要1"},
		},
	}

	sh := NewShallowSearcherWithEngine(bing, ddg, 10)
	result, err := sh.Search("测试查询")
	if err != nil {
		t.Fatalf("浅层搜索失败: %v", err)
	}
	if !strings.Contains(result, "结果1") {
		t.Errorf("应包含「结果1」，实际: %s", result)
	}
}

func TestShallowSearch_BingFailFallbackToDDG(t *testing.T) {
	bing := &simpleMockSearcher{err: fmt.Errorf("Bing 不可用")}
	ddg := &simpleMockSearcher{
		results: []SearchResult{
			{Title: "DDG结果1", URL: "https://example.com/d1", Snippet: "DDG摘要1"},
		},
	}

	sh := NewShallowSearcherWithEngine(bing, ddg, 10)
	result, err := sh.Search("fallback测试")
	if err != nil {
		t.Fatalf("回退搜索失败: %v", err)
	}
	if !strings.Contains(result, "DDG结果1") {
		t.Errorf("应包含 DDG结果1，实际: %s", result)
	}
}

func TestShallowSearch_BothFail(t *testing.T) {
	bing := &simpleMockSearcher{err: fmt.Errorf("Bing 不可用")}
	ddg := &simpleMockSearcher{err: fmt.Errorf("DDG 不可用")}

	sh := NewShallowSearcherWithEngine(bing, ddg, 10)
	_, err := sh.Search("全部失败测试")
	if err == nil {
		t.Error("期望返回错误，但成功了")
	}
}

func TestShallowSearch_BothEmpty(t *testing.T) {
	bing := &simpleMockSearcher{results: []SearchResult{}}
	ddg := &simpleMockSearcher{results: []SearchResult{}}

	sh := NewShallowSearcherWithEngine(bing, ddg, 10)
	result, err := sh.Search("空结果查询")
	if err != nil {
		t.Fatalf("空结果不应返回错误: %v", err)
	}
	if !strings.Contains(result, "未找到") {
		t.Errorf("应包含「未找到」，实际: %s", result)
	}
}

// ============================================================
// 深层搜索测试
// ============================================================

func TestDeepSearch_LLMUnavailableFallback(t *testing.T) {
	bing := &simpleMockSearcher{
		results: []SearchResult{
			{Title: "搜索结果A", URL: "https://example.com/a", Snippet: "摘要A"},
			{Title: "搜索结果B", URL: "https://example.com/b", Snippet: "摘要B"},
		},
	}
	ddg := &simpleMockSearcher{results: []SearchResult{}}
	sh := NewShallowSearcherWithEngine(bing, ddg, 10)

	deepCfg := DeepConfig{MaxResults: 30, FetchContent: false, FetchTimeout: 5, MaxContentLength: 2000}
	httpCfg := HTTPConfig{Timeout: 5 * time.Second, UserAgent: "test"}
	ds := NewDeepSearcher(sh, nil, deepCfg, httpCfg)

	result, err := ds.Search("测试查询")
	if err != nil {
		t.Fatalf("无 LLM 的深层搜索失败: %v", err)
	}
	if !strings.Contains(result, "搜索结果A") {
		t.Errorf("回退结果应包含搜索结果，实际: %s", result)
	}
}

func TestDeepSearch_NoResults(t *testing.T) {
	bing := &simpleMockSearcher{results: []SearchResult{}}
	ddg := &simpleMockSearcher{results: []SearchResult{}}
	sh := NewShallowSearcherWithEngine(bing, ddg, 10)

	deepCfg := DeepConfig{MaxResults: 30, FetchContent: false, FetchTimeout: 5, MaxContentLength: 2000}
	httpCfg := HTTPConfig{Timeout: 5 * time.Second, UserAgent: "test"}
	ds := NewDeepSearcher(sh, nil, deepCfg, httpCfg)

	result, err := ds.Search("无结果查询")
	if err != nil {
		t.Fatalf("无结果不应报错: %v", err)
	}
	if !strings.Contains(result, "未找到") {
		t.Errorf("应包含「未找到」，实际: %s", result)
	}
}

func TestDeepSearch_WithMockLLM(t *testing.T) {
	bing := &simpleMockSearcher{
		results: []SearchResult{
			{Title: "深度学习论文", URL: "https://example.com/1", Snippet: "深度学习摘要"},
			{Title: "Transformer架构", URL: "https://example.com/2", Snippet: "Transformer摘要"},
		},
	}
	ddg := &simpleMockSearcher{results: []SearchResult{}}
	sh := NewShallowSearcherWithEngine(bing, ddg, 10)

	mockLLMProvider := &mockLLM{
		chatFunc: func(messages []ChatMessage) (string, error) {
			return "## 深度学习总结\n\n深度学习是机器学习的重要分支...\n\n来源：\n- https://example.com/1", nil
		},
	}

	deepCfg := DeepConfig{MaxResults: 30, FetchContent: false, FetchTimeout: 5, MaxContentLength: 2000}
	httpCfg := HTTPConfig{Timeout: 5 * time.Second, UserAgent: "test"}
	ds := NewDeepSearcher(sh, mockLLMProvider, deepCfg, httpCfg)

	result, err := ds.Search("深度学习的核心概念")
	if err != nil {
		t.Fatalf("深层搜索失败: %v", err)
	}
	if !strings.Contains(result, "深度学习总结") {
		t.Errorf("LLM 总结应包含标题，实际: %s", result)
	}
}

// ============================================================
// 研究搜索测试
// ============================================================

func TestResearchSearch_LLMUnavailable(t *testing.T) {
	bing := &simpleMockSearcher{
		results: []SearchResult{
			{Title: "无LLM结果1", URL: "https://example.com/1", Snippet: "摘要1"},
			{Title: "无LLM结果2", URL: "https://example.com/2", Snippet: "摘要2"},
		},
	}
	ddg := &simpleMockSearcher{results: []SearchResult{}}
	sh := NewShallowSearcherWithEngine(bing, ddg, 10)

	researchCfg := ResearchConfig{MaxResults: 10, MaxSubQueries: 6}
	rs := NewResearchSearcher(sh, nil, researchCfg)

	result, err := rs.Search("无LLM的研究搜索")
	if err != nil {
		t.Fatalf("无LLM研究搜索失败: %v", err)
	}
	if !strings.Contains(result, "研究搜索报告") {
		t.Errorf("应包含原始报告，实际: %s", result)
	}
}

func TestResearchSearch_DecomposeFail(t *testing.T) {
	bing := &simpleMockSearcher{
		results: []SearchResult{
			{Title: "单问题结果1", URL: "https://example.com/1", Snippet: "摘要1"},
		},
	}
	ddg := &simpleMockSearcher{results: []SearchResult{}}
	sh := NewShallowSearcherWithEngine(bing, ddg, 10)

	mockLLMProvider := &mockLLM{
		chatFunc: func(messages []ChatMessage) (string, error) {
			content := messages[len(messages)-1].Content
			if strings.Contains(content, "拆解") {
				return "", fmt.Errorf("LLM 超时")
			}
			return "# 降级报告\n\n降级为单问题搜索。", nil
		},
	}

	researchCfg := ResearchConfig{MaxResults: 10, MaxSubQueries: 6}
	rs := NewResearchSearcher(sh, mockLLMProvider, researchCfg)

	result, err := rs.Search("什么是量子计算")
	if err != nil {
		t.Fatalf("降级搜索失败: %v", err)
	}
	if !strings.Contains(result, "降级报告") {
		t.Errorf("降级后应生成报告，实际: %s", result)
	}
}

func TestResearchSearch_WithMockLLM(t *testing.T) {
	bing := &simpleMockSearcher{
		results: []SearchResult{
			{Title: "搜索结果", URL: "https://example.com/1", Snippet: "摘要"},
		},
	}
	ddg := &simpleMockSearcher{results: []SearchResult{}}
	sh := NewShallowSearcherWithEngine(bing, ddg, 10)

	mockLLMProvider := &mockLLM{
		chatFunc: func(messages []ChatMessage) (string, error) {
			content := messages[len(messages)-1].Content
			if strings.Contains(content, "拆解") {
				return `["AI的定义和历史", "AI的主要技术分支"]`, nil
			}
			return "# 研究搜索报告\n\n## 核心发现\n1. AI技术发展迅速\n\n## 信息来源\n- https://example.com", nil
		},
	}

	researchCfg := ResearchConfig{MaxResults: 10, MaxSubQueries: 6}
	rs := NewResearchSearcher(sh, mockLLMProvider, researchCfg)

	result, err := rs.Search("人工智能的发展现状")
	if err != nil {
		t.Fatalf("研究搜索失败: %v", err)
	}
	if !strings.Contains(result, "研究搜索报告") {
		t.Errorf("应包含报告标题，实际: %s", result)
	}
}

// ============================================================
// 子系统整体测试
// ============================================================

func TestSystem_NoLLM(t *testing.T) {
	cfg := DefaultConfig()
	sys := NewWithConfig(cfg)
	if sys.HasLLM() {
		t.Error("未配置 LLM 时 HasLLM 应返回 false")
	}
}

func TestSystem_WithLLM(t *testing.T) {
	cfg := DefaultConfig()
	cfg.LLM.APIKey = "test-key"
	sys := NewWithConfig(cfg)
	if !sys.HasLLM() {
		t.Error("配置了 API Key 后 HasLLM 应返回 true")
	}
}

func TestSystem_WithCustomLLM(t *testing.T) {
	cfg := DefaultConfig()
	provider := &mockLLM{
		chatFunc: func(messages []ChatMessage) (string, error) {
			return "测试响应", nil
		},
	}
	sys := NewWithLLM(cfg, provider)
	if !sys.HasLLM() {
		t.Error("配置了自定义 LLM 后 HasLLM 应返回 true")
	}
}

func TestSystem_SearchMode(t *testing.T) {
	if ModeShallow != "shallow" {
		t.Errorf("ModeShallow 应为 'shallow'，实际: %s", ModeShallow)
	}
	if ModeDeep != "deep" {
		t.Errorf("ModeDeep 应为 'deep'，实际: %s", ModeDeep)
	}
	if ModeResearch != "research" {
		t.Errorf("ModeResearch 应为 'research'，实际: %s", ModeResearch)
	}
}

func TestSystem_SetShallowMaxResults(t *testing.T) {
	cfg := DefaultConfig()
	sys := NewWithConfig(cfg)
	sys.SetShallowMaxResults(20)
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Shallow.MaxResults != 10 {
		t.Errorf("默认浅层搜索结果数应为 10，实际: %d", cfg.Shallow.MaxResults)
	}
	if cfg.Deep.MaxResults != 30 {
		t.Errorf("默认深层搜索结果数应为 30，实际: %d", cfg.Deep.MaxResults)
	}
	if cfg.Research.MaxSubQueries != 6 {
		t.Errorf("默认最大子问题数应为 6，实际: %d", cfg.Research.MaxSubQueries)
	}
}
