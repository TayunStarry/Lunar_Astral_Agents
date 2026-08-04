package websearch

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// NewWithLLM 使用工具模型池 LLM 提供者创建网络检索子系统
func NewWithLLM(cfg Config, provider Provider, debugLog func(format string, args ...interface{})) *System {
	s := &System{
		cfg:         cfg,
		llmProvider: provider,
		DebugLog:    debugLog,
	}

	s.health = NewEngineHealth()
	s.simple = NewSimpleSearcher(cfg)
	s.simple.health = s.health
	s.simple.debugLog = debugLog

	// 先初始化搜索知识库（SQLite）——所有模式都初始化，简易模式也需要查询
	var knowledge *SearchKnowledge
	sk, err := NewSearchKnowledge("data/search_knowledge.db", debugLog)
	if err != nil {
		// 初始化失败时不能静默吞掉，否则后续所有知识库操作都被跳过
		if debugLog != nil {
			debugLog("[搜索知识库] 初始化失败: %v", err)
		} else {
			fmt.Printf("  ⚠ [搜索知识库] 初始化失败: %v\n", err)
		}
	} else {
		knowledge = sk
		s.knowledge = sk
	}

	s.webpage = NewWebpageSearcher(s.simple, s.llmProvider, cfg.Webpage, cfg.HTTP, debugLog, knowledge)
	s.depth = NewDepthSearcher(s.simple, s.llmProvider, cfg.Depth, cfg.HTTP, knowledge)
	s.depth.debugLog = debugLog

	return s
}

// SetDebugLogFunc 设置诊断日志回调
func (s *System) SetDebugLogFunc(fn func(format string, args ...interface{})) {
	s.DebugLog = fn
	s.webpage.debugLog = fn
	s.simple.debugLog = fn
	s.depth.debugLog = fn
	if s.knowledge != nil {
		s.knowledge.debugLog = fn
	}

	if s.DebugLog != nil {
		s.DebugLog("[浏览器渲染] 懒加载模式已就绪，将在首次搜索时初始化浏览器")
	}
}

// SetMemoryProvider 设置记忆提供者（供深度搜索使用）
func (s *System) SetMemoryProvider(mp MemoryProvider) {
	s.memProvider = mp
}

// SetVisionProvider 设置图片识别提供者
func (s *System) SetVisionProvider(vp VisionProvider) {
	s.visionProvider = vp
}

// SetRerankProvider 设置重排序提供者（Embedding模型池）
// 用于 LLM Rerank：对搜索结果按余弦相似度重排序
// 仅在网页搜索和深度搜索中生效，简易搜索不调用
func (s *System) SetRerankProvider(embedding EmbeddingProvider) {
	s.reranker = NewReranker(embedding, s.DebugLog)
}

// SetDownloadFunc 设置下载回调函数
func (s *System) SetDownloadFunc(fn DownloadFunc) {
	s.downloadFunc = fn
}

// SetDownloadGroupID 设置下载目标群组ID
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
// forceRefresh 为 true 时跳过知识库缓存，强制联网搜索
func (s *System) Search(ctx context.Context, query string, mode SearchMode, forceRefresh ...bool) (string, error) {
	urls := extractURLs(query)
	if len(urls) > 0 {
		searchText := stripURLs(query, urls)
		searchText = strings.TrimSpace(searchText)
		if searchText == "" {
			return "查询中仅包含链接，链接内容请使用 fetch_url 工具单独获取。", nil
		}
		return s.doSearch(ctx, searchText, mode, forceRefresh...)
	}
	return s.doSearch(ctx, query, mode, forceRefresh...)
}

// doSearch 按模式路由搜索，集成知识库缓存
func (s *System) doSearch(ctx context.Context, query string, mode SearchMode, forceRefresh ...bool) (string, error) {
	skipCache := len(forceRefresh) > 0 && forceRefresh[0]

	// 简易模式：查知识库，未命中/过期则轻量联网搜索
	if mode == ModeSimple {
		if skipCache {
			return s.simpleSearch(query)
		}
		return s.cachedOnlySearch(query)
	}

	// 常规/深度模式：先查知识库
	isUpdate := isUpdateQuery(query)
	if !skipCache && s.knowledge != nil {
		cached, err := s.knowledge.LookupQuery(query)
		if err != nil {
			if s.DebugLog != nil {
				s.DebugLog("[搜索知识库] 查询失败 query=%q err=%v", query, err)
			}
		} else if cached != nil && len(cached.Results) > 0 {
			// 定义类问题（非更新类）：永久有效，直接返回
			if !isUpdate {
				if s.DebugLog != nil {
					s.DebugLog("[搜索知识库] 定义类查询命中 query=%q 结果数=%d", query, len(cached.Results))
				}
				// 返回存储的最终LLM总结结果，而非重新格式化原始结果
				if cached.ResultText != "" {
					return cached.ResultText, nil
				}
				return formatResults(cached.Results), nil
			}
			// 更新类问题：1小时内有效
			if time.Since(cached.SearchedAt) < 1*time.Hour {
				if s.DebugLog != nil {
					s.DebugLog("[搜索知识库] 更新类查询数据仍新鲜 query=%q 时间=%s", query, cached.SearchedAt.Format("01-02 15:04"))
				}
				if cached.ResultText != "" {
					return cached.ResultText, nil
				}
				return formatResults(cached.Results), nil
			}
			if s.DebugLog != nil {
				s.DebugLog("[搜索知识库] 更新类查询数据已过期 query=%q 将重新搜索", query)
			}
		}
	}

	// 联网搜索
	result, rawResults, err := s.realSearch(ctx, query, mode, skipCache)
	if err != nil {
		return result, err
	}

	// 常规/深度模式：存储搜索结果到知识库（包含最终LLM总结结果）
	if s.knowledge != nil && mode != ModeSimple {
		// 使用realSearch返回的原始结果，不再重复调用SearchRaw
		if len(rawResults) > 0 {
			if storeErr := s.knowledge.StoreQuery(query, result, rawResults); storeErr != nil && s.DebugLog != nil {
				s.DebugLog("[搜索知识库] 存储失败 query=%q err=%v", query, storeErr)
			}
		}
	}

	return result, nil
}

// cachedOnlySearch 简易模式：优先查知识库，未命中或过期则回退到轻量联网搜索
func (s *System) cachedOnlySearch(query string) (string, error) {
	if s.knowledge == nil {
		if s.DebugLog != nil {
			s.DebugLog("[简易搜索] 知识库未初始化，回退到轻量联网搜索")
		}
		return s.simpleSearch(query)
	}
	cached, err := s.knowledge.LookupQuery(query)
	if err != nil {
		if s.DebugLog != nil {
			s.DebugLog("[简易搜索] 查询知识库失败: %v，回退到轻量联网搜索", err)
		}
		return s.simpleSearch(query)
	}
	if cached == nil || len(cached.Results) == 0 {
		if s.DebugLog != nil {
			s.DebugLog("[简易搜索] 知识库未命中 query=%q，回退到轻量联网搜索", query)
		}
		return s.simpleSearch(query)
	}

	// 数据过期判断（与常规/深度模式一致）
	if isUpdateQuery(query) && time.Since(cached.SearchedAt) >= 1*time.Hour {
		if s.DebugLog != nil {
			s.DebugLog("[简易搜索] 知识库中更新类查询已过期 query=%q 时间=%s，重新搜索", query, cached.SearchedAt.Format("01-02 15:04"))
		}
		return s.simpleSearch(query)
	}

	if s.DebugLog != nil {
		s.DebugLog("[简易搜索] 知识库命中 query=%q 结果数=%d", query, len(cached.Results))
	}
	// 返回存储的最终LLM总结结果，而非重新格式化原始结果
	if cached.ResultText != "" {
		return cached.ResultText, nil
	}
	return formatResults(cached.Results), nil
}

// realSearch 执行真正的联网搜索（返回结果文本和原始搜索结果）
// skipURLCache: true 时跳过 URL 缓存
func (s *System) realSearch(ctx context.Context, query string, mode SearchMode, skipURLCache ...bool) (string, []SearchResult, error) {
	force := len(skipURLCache) > 0 && skipURLCache[0]
	switch mode {
	case ModeWebpage:
		return s.webpageSearchWithResults(query)
	case ModeDepth:
		return s.depthSearchWithResults(ctx, query, force)
	default:
		result, err := s.simpleSearch(query)
		return result, nil, err
	}
}

// simpleSearch 执行轻量摘要搜索
func (s *System) simpleSearch(query string) (string, error) {
	return s.simple.Search(query)
}

// SimpleSearchRaw 执行轻量摘要搜索，返回原始结果列表（而非格式化字符串）
// 供外部调用方自行处理搜索结果结构
func (s *System) SimpleSearchRaw(query string) ([]SearchResult, error) {
	return s.simple.SearchRaw(query)
}

// webpageSearch 执行网页搜索
func (s *System) webpageSearch(query string) (string, error) {
	result, _, err := s.webpage.SearchWithResults(query)
	return result, err
}

// webpageSearchWithResults 执行网页搜索并返回原始结果
func (s *System) webpageSearchWithResults(query string) (string, []SearchResult, error) {
	result, rawResults, err := s.webpage.SearchWithResults(query)
	if err != nil {
		return result, rawResults, err
	}
	// 应用 Rerank 重排序（不影响已格式化的 LLM 结果，仅重排原始结果列表）
	if s.reranker != nil && len(rawResults) > 0 {
		rawResults = applyRerank(s.reranker, query, rawResults, s.DebugLog)
	}
	return result, rawResults, nil
}

// depthSearch 执行深度研究
func (s *System) depthSearch(ctx context.Context, query string, forceRefresh ...bool) (string, error) {
	result, _, err := s.depthSearchWithResults(ctx, query, forceRefresh...)
	return result, err
}

// depthSearchWithResults 执行深度研究并返回原始搜索结果
func (s *System) depthSearchWithResults(ctx context.Context, query string, forceRefresh ...bool) (string, []SearchResult, error) {
	force := len(forceRefresh) > 0 && forceRefresh[0]
	if s.llmProvider == nil {
		result, rawResults, err := s.depth.SearchWithResults(ctx, query, force)
		if err != nil {
			return result, rawResults, err
		}
		if s.reranker != nil && len(rawResults) > 0 {
			rawResults = applyRerank(s.reranker, query, rawResults, s.DebugLog)
		}
		return result, rawResults, nil
	}
	cfg := GapCheckConfig{
		MaxRounds:     s.cfg.Depth.MaxGapRounds,
		MaxSubQueries: s.cfg.Depth.MaxSubQueries,
	}
	// runGapCheckSearch 返回 (string, error)
	result, err := s.depth.runGapCheckSearch(ctx, query, s.webpage, s.memProvider, cfg, force)
	if err != nil {
		return "", nil, err
	}
	// 获取原始搜索结果用于知识库存储（只获取一次，不重复搜索）
	rawResults, _ := s.simple.SearchRaw(query)
	if s.reranker != nil && len(rawResults) > 0 {
		rawResults = applyRerank(s.reranker, query, rawResults, s.DebugLog)
	}
	return result, rawResults, nil
}

// isUpdateQuery 判断查询是否有时效性（"最新/卡池/新闻"等需要实时数据的查询）
func isUpdateQuery(query string) bool {
	updateWords := []string{
		"更新", "最新", "最近", "新闻", "动态", "现在", "今天",
		"近日", "近期", "新出", "新版本", "公告", "活动",
		"有什么新", "出了什么", "最近有", "现在有",
		// 时间/日期相关
		"下个", "下期", "下个版本", "本月", "下月", "这周", "下周",
		"今年", "明年", "即将", "未来", "之后", "接下来",
		// 游戏/娱乐时效性
		"卡池", "新角色", "新卡池", "新皮肤", "新活动", "联动",
		"排期", "上线", "什么时候出", "什么时候开",
		// 时政/经济时效性
		"股价", "汇率", "天气", "限行", "油价", "金价",
		"确诊病例", "疫情", "实时",
	}
	for _, w := range updateWords {
		if strings.Contains(query, w) {
			return true
		}
	}
	// 包含年份（如2026）或月份（如7月）的查询，通常带时效性
	if yearMonthPattern.MatchString(query) {
		return true
	}
	return false
}

var yearMonthPattern = regexp.MustCompile(`\d{4}年|\d{1,2}月|\d{4}-\d{2}-\d{2}`)

// setSimpleMaxResults 设置轻量摘要搜索最大结果数
func (s *System) setSimpleMaxResults(n int) {
	s.simple.SetMaxResults(n)
}

// getConfig 获取当前配置
func (s *System) getConfig() Config {
	return s.cfg
}

// HasLLM 检查是否配置了 LLM
func (s *System) HasLLM() bool {
	return s.llmProvider != nil
}

// EnsureBrowser 确保浏览器渲染器可用（懒加载）
func (s *System) EnsureBrowser() {
	s.browserMu.Lock()
	defer s.browserMu.Unlock()

	if s.browserRenderer != nil {
		return
	}
	cr := NewChromeRenderer(60*time.Second, s.DebugLog)
	if cr == nil {
		if s.DebugLog != nil {
			s.DebugLog("[浏览器渲染] 初始化失败，本次搜索将不使用浏览器渲染")
		}
		return
	}
	cr.SetDebugLog(s.DebugLog)
	s.browserRenderer = cr
	s.webpage.browserRenderer = cr
	s.depth.browserRenderer = cr
	if baidu, ok := s.simple.baidu.(*BaiduSearcher); ok {
		baidu.browserRenderer = cr
	}
}

// CloseBrowser 关闭浏览器并清理所有进程
func (s *System) CloseBrowser() {
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

// Close 释放子系统资源
func (s *System) Close() {
	s.CloseBrowser()
	if s.knowledge != nil {
		s.knowledge.Close()
	}
}
