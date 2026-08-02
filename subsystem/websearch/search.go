package websearch

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// NewWithLLM 使用工具模型池 LLM 提供者创建网络检索子系统
func NewWithLLM(cfg Config, provider Provider) *System {
	s := &System{
		cfg:         cfg,
		llmProvider: provider,
	}

	s.health = NewEngineHealth()
	s.simple = NewSimpleSearcher(cfg)
	s.simple.health = s.health

	// 浏览器懒加载，不在启动时创建

	s.webpage = NewWebpageSearcher(s.simple, s.llmProvider, cfg.Webpage, cfg.HTTP, nil)
	s.depth = NewDepthSearcher(s.simple, s.llmProvider, cfg.Depth, cfg.HTTP)

	// 大会辩论：深度搜索的顶层编排者
	if cfg.Depth.Enabled && s.llmProvider != nil {
		s.assembly = NewAssembly(s.depth, s.webpage, s.llmProvider, nil, cfg.Depth)
		if s.DebugLog != nil {
			s.DebugLog("[深度搜索] 大会辩论系统已创建 MaxRounds=%d MaxSubQueries=%d",
				cfg.Depth.MaxRounds, cfg.Depth.MaxSubQueries)
		}
	} else if cfg.Depth.Enabled && s.llmProvider == nil {
		if s.DebugLog != nil {
			s.DebugLog("[深度搜索] 大会辩论未创建：LLM Provider为nil")
		}
	}

	return s
}

// SetDebugLogFunc 设置诊断日志回调
func (s *System) SetDebugLogFunc(fn func(format string, args ...interface{})) {
	s.DebugLog = fn
	s.webpage.debugLog = fn
	s.simple.debugLog = fn
	s.depth.debugLog = fn
	if s.assembly != nil {
		s.assembly.debugLog = fn
	}

	if s.DebugLog != nil {
		s.DebugLog("[浏览器渲染] 懒加载模式已就绪，将在首次搜索时初始化浏览器")
	}
}

// SetMemoryProvider 设置记忆提供者（供大会辩论使用）
func (s *System) SetMemoryProvider(mp MemoryProvider) {
	s.memProvider = mp
	if s.assembly != nil {
		s.assembly.memProvider = mp
	}
}

// SetVisionProvider 设置图片识别提供者（使用已有的模型池实例）
func (s *System) SetVisionProvider(vp VisionProvider) {
	s.visionProvider = vp
}

// SetDownloadFunc 设置下载回调函数（由调用方注入下载管理器）
func (s *System) SetDownloadFunc(fn DownloadFunc) {
	s.downloadFunc = fn
}

// SetDownloadGroupID 设置下载目标群组ID（每次处理前由调用方设置）
func (s *System) SetDownloadGroupID(groupID string) {
	s.downloadGroupID = groupID
}

// ProcessLinks 检测并替换消息中的链接为摘要
func (s *System) ProcessLinks(query string) (string, []string) {
	urls := extractURLs(query)
	if len(urls) == 0 {
		return query, nil
	}
	replacedQuery, linkMap := s.processAndReplaceLinks(query)

	// 构建链接描述列表
	descriptions := linkMapToDescriptions(linkMap)
	return replacedQuery, descriptions
}

// linkMapToDescriptions 将链接映射转换为描述列表
func linkMapToDescriptions(linkMap map[string]string) []string {
	if len(linkMap) == 0 {
		return nil
	}
	descs := make([]string, 0, len(linkMap))
	for i := 1; i <= len(linkMap); i++ {
		label := fmt.Sprintf("[链接%d]", i)
		if content, ok := linkMap[label]; ok {
			// content 格式为 "url：摘要"，提取摘要部分
			parts := strings.SplitN(content, "：", 2)
			summary := content
			if len(parts) == 2 {
				summary = parts[1]
			}
			descs = append(descs, fmt.Sprintf("链接%d: %s", i, summary))
		}
	}
	return descs
}

// Search 执行搜索（根据模式自动选择搜索策略）
func (s *System) Search(ctx context.Context, query string, mode SearchMode) (string, error) {
	// 检测并剥离链接（链接内容由规划器通过 fetch_url 工具单独处理）
	urls := extractURLs(query)
	if len(urls) > 0 {
		searchText := stripURLs(query, urls)
		searchText = strings.TrimSpace(searchText)

		if searchText == "" {
			// 纯链接消息，提示规划器使用 fetch_url
			return "查询中仅包含链接，链接内容请使用 fetch_url 工具单独获取。", nil
		}

		// 对剩余文字执行搜索
		return s.doSearch(ctx, searchText, mode)
	}

	return s.doSearch(ctx, query, mode)
}

// doSearch 按模式路由搜索
func (s *System) doSearch(ctx context.Context, query string, mode SearchMode) (string, error) {
	var result string
	var err error
	switch mode {
	case ModeWebpage:
		result, err = s.webpageSearch(query)
	case ModeDepth:
		if s.assembly != nil {
			result, err = s.assembly.Search(ctx, query)
		} else {
			// 无LLM时降级到普通深度搜索
			result, err = s.depthSearch(ctx, query)
		}
	default:
		result, err = s.simpleSearch(query)
	}

	return result, err
}

// simpleSearch 执行轻量摘要搜索
func (s *System) simpleSearch(query string) (string, error) {
	return s.simple.Search(query)
}

// webpageSearch 执行网页搜索
func (s *System) webpageSearch(query string) (string, error) {
	return s.webpage.Search(query)
}

// depthSearch 执行深度研究
func (s *System) depthSearch(ctx context.Context, query string) (string, error) {
	return s.depth.Search(ctx, query)
}

// setSimpleMaxResults 设置轻量摘要搜索最大结果数
func (s *System) setSimpleMaxResults(n int) {
	s.simple.SetMaxResults(n)
}

// getConfig 获取当前配置
func (s *System) getConfig() Config {
	return s.cfg
}

// hasLLM 检查是否配置了 LLM
func (s *System) hasLLM() bool {
	return s.llmProvider != nil
}

// EnsureBrowser 确保浏览器渲染器可用（懒加载），每次搜索前调用
// 使用 browserMu 保护并发创建：多个 goroutine 同时调用时，只有第一个创建浏览器，其余等待复用
func (s *System) EnsureBrowser() {
	s.browserMu.Lock()
	defer s.browserMu.Unlock()

	if s.browserRenderer != nil {
		return
	}
	// 用局部变量接收，避免 Go 接口 nil 陷阱：
	// nil *ChromeRenderer 赋值给 BrowserRenderer 接口后 ≠ nil
	cr := NewChromeRenderer(60*time.Second, s.DebugLog)
	if cr == nil {
		if s.DebugLog != nil {
			s.DebugLog("[浏览器渲染] 初始化失败，本次搜索将不使用浏览器渲染")
		}
		return
	}
	cr.SetDebugLog(s.DebugLog)
	s.browserRenderer = cr
	// 同步到子模块
	s.webpage.browserRenderer = cr
	s.depth.browserRenderer = cr
	if baidu, ok := s.simple.baidu.(*BaiduSearcher); ok {
		baidu.browserRenderer = cr
	}
}

// CloseBrowser 关闭浏览器释放资源，每次搜索完成后调用（现在改为空实现，浏览器复用）
// 浏览器不再每次搜索后关闭，改为应用退出时统一释放
func (s *System) CloseBrowser() {
}

// Close 释放子系统资源（包括无头浏览器实例），应用退出时调用
func (s *System) Close() {
	s.browserMu.Lock()
	defer s.browserMu.Unlock()

	if s.browserRenderer != nil {
		s.browserRenderer.Close()
		s.browserRenderer = nil
	}
	s.webpage.browserRenderer = nil
	s.depth.browserRenderer = nil
	if baidu, ok := s.simple.baidu.(*BaiduSearcher); ok {
		baidu.browserRenderer = nil
	}
}

// ============================================================
// 外部调用者导出的便捷方法
// ============================================================

// DefaultConfig 返回默认配置
func DefaultConfig() Config {
	return defaultConfig
}

// HasLLM 检查是否配置了 LLM 提供者
func (s *System) HasLLM() bool {
	return s.hasLLM()
}

// SimpleSearchRaw 执行轻量摘要搜索，返回原始搜索结果（不格式化）
func (s *System) SimpleSearchRaw(query string) ([]SearchResult, error) {
	limit := s.cfg.Simple.MaxResults
	if limit <= 0 {
		limit = 10
	}

	// 依次尝试各搜索引擎
	engines := []Searcher{s.simple.baidu, s.simple.sogou, s.simple.bing, s.simple.ddg}
	for _, engine := range engines {
		results, err := engine.SearchRaw(query, limit)
		if err == nil && len(results) > 0 {
			return results, nil
		}
	}
	return nil, fmt.Errorf("所有搜索引擎均无结果")
}

// DepthSearch 执行深度搜索（导出包装）
func (s *System) DepthSearch(query string) (string, error) {
	ctx := context.Background()
	return s.depthSearch(ctx, query)
}

// WebpageSearch 执行网页搜索（导出包装）
func (s *System) WebpageSearch(query string) (string, error) {
	return s.webpageSearch(query)
}

// SimpleSearch 执行轻量摘要搜索（导出包装，返回格式化文本）
func (s *System) SimpleSearch(query string) (string, error) {
	return s.simpleSearch(query)
}
