package AgentSearch

import (
	"LunarSubsystem/FileManager/module"
	"LunarSubsystem/GeneralConfig"
	"fmt"
	"strings"
	"time"
)

// =============================================================================
// 搜索智能体 — 主控流程编排
// =============================================================================

// =============================================================================
// 公开 API
// =============================================================================

// InitSearch 初始化搜索智能体
// 必须在使用 Search 之前调用
// 模型配置（URL、模型名、API Key）从 config 模块（lunar_config.json）读取，不再通过 SearchConfig 传入
// 初始化失败时返回错误（如模型不可用、配置无效等）
func InitSearch(cfg SearchConfig) error {
	searchAgentMu.Lock()
	defer searchAgentMu.Unlock()

	if searchAgentInit {
		return fmt.Errorf("搜索智能体已初始化，请勿重复调用 InitSearch")
	}

	// 参数校验
	if cfg.MaxContextTokens <= 0 {
		cfg.MaxContextTokens = MaxContextTokensDefault
	}

	// 提前设置全局配置（AI 调用层在连通性测试中需要读取）
	configMutex.Lock()
	activeConfig = &cfg
	configMutex.Unlock()

	// 验证多模态模型连通性（模型配置从 config 模块读取）
	if aiCall != nil {
		testPrompt := "请回复 'OK'"
		resp, err := aiCall("你是一个测试助手。", testPrompt, nil)
		if err != nil {
			return fmt.Errorf("多模态模型连通性测试失败 [%s @ %s]: %w",
				*GeneralConfig.SearchMultimodalModel, *GeneralConfig.SearchMultimodalURL, err)
		}
		if !strings.Contains(strings.ToUpper(resp), "OK") {
			return fmt.Errorf("多模态模型响应异常，未返回预期内容")
		}
		fmt.Printf("[%s] 多模态模型连接验证通过: %s\n", ModuleName, *GeneralConfig.SearchMultimodalModel)
	}

	// 验证嵌入模型连通性 + 初始化 search_memory 集合
	if memoryInitCollection != nil {
		// 初始化 MemoryDB 全局实例（仅首次调用生效，后续调用幂等）
		if cfg.MemoryDBDir != "" {
			module.InitMemoryDB(cfg.MemoryDBDir)
		}
		if err := memoryInitCollection(); err != nil {
			return fmt.Errorf("嵌入模型连通性测试/记忆集合初始化失败 [%s @ %s]: %w",
				*GeneralConfig.SearchEmbeddingModel, *GeneralConfig.SearchEmbeddingURL, err)
		}
		fmt.Printf("[%s] 嵌入模型连接验证通过: %s\n", ModuleName, *GeneralConfig.SearchEmbeddingModel)
	}

	// 启动浏览器
	if err := LaunchBrowser(); err != nil {
		return fmt.Errorf("浏览器启动失败: %w", err)
	}

	searchAgent = &SearchAgent{
		config: cfg,
	}
	searchAgentInit = true

	// 设置全局配置（供 AI 调用层读取）
	configMutex.Lock()
	activeConfig = &cfg
	configMutex.Unlock()

	fmt.Printf("[%s] 搜索智能体初始化完成\n", ModuleName)
	fmt.Printf("[%s]   多模态: %s @ %s\n", ModuleName, *GeneralConfig.SearchMultimodalModel, *GeneralConfig.SearchMultimodalURL)
	fmt.Printf("[%s]   嵌入:   %s @ %s\n", ModuleName, *GeneralConfig.SearchEmbeddingModel, *GeneralConfig.SearchEmbeddingURL)
	fmt.Printf("[%s]   上下文上限: %d tokens\n", ModuleName, cfg.MaxContextTokens)

	return nil
}

// Search 执行搜索查询（阻塞，串行执行）
// 返回结构化的搜索报告，包含 AI 生成的答案和引用来源
// 调用前必须先调用 InitSearch
func Search(query string) (*SearchReport, error) {
	if !searchAgentInit {
		return nil, fmt.Errorf("搜索智能体未初始化，请先调用 InitSearch")
	}

	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("搜索查询不能为空")
	}

	// 串行执行：获取查询锁
	queryMutex.Lock()
	defer queryMutex.Unlock()

	fmt.Printf("\n[%s] ===== 开始搜索: %s =====\n", ModuleName, query)

	// 浏览器健康检查
	if err := ensureBrowserHealthy(); err != nil {
		return nil, err
	}

	// 执行搜索流水线
	return searchAgent.executeSearchPipeline(query)
}

// =============================================================================
// 搜索流水线
// =============================================================================

// executeSearchPipeline 执行完整搜索流水线（统一单一模式）
// 流程：记忆优先匹配 → 不满足（无匹配/时效性/用户明确要求搜索）则进入统一网络搜索
// 网络搜索：原始输入 → 搜索引擎前15条 → 逐页滚动截图(≤10帧) → 逐页视觉摘要 → 逐条判定能否解答
//        → 可解答则生成报告并入库；全部不可解答则进入增强搜索再跑一轮；仍失败则返回"月华不知道"并记录失败经验
func (a *SearchAgent) executeSearchPipeline(query string) (*SearchReport, error) {
	// 重置搜索状态
	a.usedKeywords = nil
	a.accumulatedSummaries = nil
	a.accumulatedSources = nil
	a.memoryHints = nil
	a.attemptedPages = nil
	a.coreEntities = nil
	a.queryEmbedding = nil
	a.initialQuery = ""

	// ---- 步骤1：AI 提取核心实体 + 关键词数组 ----
	// ---- 步骤2：关键词空格拼接成初始查询语句 ----
	initialQuery := ""
	if aiExtractKeywords != nil {
		entities, keywords, err := aiExtractKeywords(query)
		if err == nil && len(keywords) > 0 {
			a.coreEntities = entities
			initialQuery = buildInitialQuery(keywords)
			a.initialQuery = initialQuery
			a.usedKeywords = append(a.usedKeywords, keywords...)
			fmt.Printf("[%s] [关键词提取] 核心实体: %v\n", ModuleName, entities)
			fmt.Printf("[%s] [关键词提取] 初始查询: %s\n", ModuleName, initialQuery)
		} else if err != nil {
			fmt.Printf("[%s] [关键词提取] 失败，回退规则清洗: %v\n", ModuleName, err)
		}
	}
	if strings.TrimSpace(initialQuery) == "" {
		initialQuery = cleanSearchText(query)
		a.initialQuery = initialQuery
	}

	// ---- 步骤3：对初始查询语句做向量嵌入 ----
	if emb, err := callEmbedding(initialQuery); err == nil {
		a.queryEmbedding = emb
	} else {
		fmt.Printf("[%s] [关键词提取] 初始查询嵌入失败: %v\n", ModuleName, err)
	}

	// ---- 步骤4：用初始查询在记忆库检索 ----
	if r := a.phaseMemoryLookup(query, initialQuery); r != nil {
		return r, nil
	}

	// ---- 步骤5-10：统一网络搜索 ----
	return a.phaseNetworkSearch(query, initialQuery, 1, "")
}

// =============================================================================
// 统一网络搜索（单一模式）
// =============================================================================

// phaseNetworkSearch 统一网络搜索
// attempt=1：用用户原始输入（清洗套话后）搜索 → 逐页混合提取 → 摘要 → 逐条判定能否解答
// 全部无法解答且 attempt=1 → 增强搜索（模型推测真实意图产出强化文本）再执行一轮 attempt=2
// 增强轮仍无法解答 → 用"原始完整句"兜底再搜一次（应对"钛宇星光阁"这类专名与常见字冲突的歧义）
// 全部失败 → 返回"月华不知道"，并把失败经验记入记忆库供下次规避相同错误方向
func (a *SearchAgent) phaseNetworkSearch(query, initialQuery string, attempt int, priorExperience string) (*SearchReport, error) {
	type searchCandidate struct {
		text string
	}
	var candidates []searchCandidate
	if attempt > 1 {
		refined, err := a.enhanceSearchText(query, priorExperience)
		if err != nil || strings.TrimSpace(refined) == "" {
			refined = initialQuery
		}
		fmt.Printf("[%s] [网络搜索] 增强搜索文本: %s\n", ModuleName, refined)
		candidates = append(candidates, searchCandidate{text: refined})
		// 专名歧义兜底：强化文本不同于初始查询与原始问题，再加搜一次原始完整句，
		// 应对"钛宇星光阁"这类与常见字冲突、只有完整口语化表达才能被正确识别的昵称。
		if refined != initialQuery && refined != query {
			candidates = append(candidates, searchCandidate{text: query})
		}
	} else {
		candidates = append(candidates, searchCandidate{text: initialQuery})
	}

	var lastAttempted []string
	var sawFallback bool
	for _, cd := range candidates {
		validSums, validSrcs, attemptedSums, fallback := a.searchTextAndJudge(query, cd.text)
		if len(attemptedSums) > 0 {
			lastAttempted = attemptedSums
		}
		if fallback {
			sawFallback = true
			fmt.Printf("[%s] [网络搜索] 检测到 Bing 工具站兜底，跳过本候选，转下一轮\n", ModuleName)
			continue
		}
		if len(validSums) == 0 {
			continue
		}

		// ---- 步骤8：调用 AI 综合判定拼接后的摘要能否回答 ----
		memoryReference := strings.Join(a.memoryHints, "\n")
		answerable := true
		if aiJudgeComprehensive != nil {
			ok, err := aiJudgeComprehensive(query, memoryReference, strings.Join(validSums, "\n\n---\n\n"))
			if err == nil {
				answerable = ok
			} else {
				fmt.Printf("[%s] [网络搜索] 综合判定失败（默认判为可解答）: %v\n", ModuleName, err)
				answerable = true
			}
		}
		if answerable {
			a.accumulatedSummaries = validSums
			a.accumulatedSources = validSrcs
			fmt.Printf("[%s] [网络搜索] 综合判定可解答（%d 份摘要），生成调查报告\n", ModuleName, len(validSums))
			report, err := a.phaseGenerateReport(query, attempt)
			if err != nil {
				return nil, err
			}
			a.phaseStoreToMemory(query, report)
			return report, nil
		}
		fmt.Printf("[%s] [网络搜索] 综合判定不足以解答，尝试下一候选/增强搜索\n", ModuleName)
	}

	// ---- 全部无法解答 → 增强搜索再试一轮 ----
	if attempt < 2 {
		prior := strings.Join(lastAttempted, "\n")
		if sawFallback {
			fmt.Printf("[%s] [网络搜索] 检测到工具站兜底，进入增强搜索换词重试\n", ModuleName)
		} else {
			fmt.Printf("[%s] [网络搜索] 全部摘要均无法解答，进入增强搜索\n", ModuleName)
		}
		return a.phaseNetworkSearch(query, initialQuery, attempt+1, prior)
	}

	// ---- 增强后仍无法解答 → 月华不知道 + 记录失败经验 ----
	return a.phaseUnknownAnswer(query)
}

// searchTextAndJudge 用给定搜索文本执行一轮 搜索→混合提取→摘要+相关性判定
// clean 为 true 时先剥离口语套话；为 false 时按原文直搜（用于专名歧义兜底）
// 判断时参考记忆库 topk（memoryReference），并在摘要阶段顺带判定相关性，无关页面直接跳过。
// 返回: 可用摘要/来源、本轮尝试过的摘要、是否存在可解答项、是否触发工具站兜底
func (a *SearchAgent) searchTextAndJudge(query, searchText string) (validSums, validSrcs, attemptedSums []string, fallback bool) {
	if strings.TrimSpace(searchText) == "" {
		searchText = cleanSearchText(query)
	}
	fmt.Printf("[%s] [网络搜索] 搜索: %s\n", ModuleName, searchText)
	a.usedKeywords = append(a.usedKeywords, searchText)

	results, err := ExecuteSearch(searchText, SingleSearchResults)
	if err != nil {
		fmt.Printf("[%s] [网络搜索] 搜索执行失败: %v\n", ModuleName, err)
		return nil, nil, nil, false
	}

	// 去重（按 URL）
	seenURLs := make(map[string]bool)
	var uniqueResults []SearchResult
	for _, r := range results {
		if !seenURLs[r.URL] {
			seenURLs[r.URL] = true
			uniqueResults = append(uniqueResults, r)
		}
		if len(uniqueResults) >= SingleSearchResults {
			break
		}
	}
	uniqueResults = filterDictionarySites(uniqueResults)
	fmt.Printf("[%s] [网络搜索] 过滤后共 %d 条结果\n", ModuleName, len(uniqueResults))
	if len(uniqueResults) == 0 {
		return nil, nil, nil, false
	}

	// Bing 工具站兜底感知：若大量结果是快递/物流/在线工具/whois/学信网/地图等固定工具站，
	// 判断为触发了工具站兜底——不再逐页提取白费时间，直接标记由上层换词重试。
	if detectSearchFallback(uniqueResults) {
		fmt.Printf("[%s] [网络搜索] ⚠ 检测到 Bing 工具站兜底（多数结果为工具站），跳过本页提取\n", ModuleName)
		return nil, nil, nil, true
	}

	// ---- 步骤6：标题初筛（用核心实体完整名过滤无关标题） ----
	uniqueResults = a.filterByTitleEntities(uniqueResults)
	fmt.Printf("[%s] [网络搜索] 标题初筛后共 %d 条结果\n", ModuleName, len(uniqueResults))
	if len(uniqueResults) == 0 {
		return nil, nil, nil, false
	}

	// ---- 步骤7：逐页混合提取 → 摘要 → 嵌入打分 + 关键词比对 ----
	for i, r := range uniqueResults {
		fmt.Printf("[%s] [网络搜索] 提取 [%d/%d]: %s\n", ModuleName, i+1, len(uniqueResults), r.Title)

		content, extractErr := ExtractPageContent(r.URL)
		if extractErr != nil {
			fmt.Printf("[%s] [网络搜索] 页面打不开，跳过: %s (%v)\n", ModuleName, r.URL, extractErr)
			continue
		}
		if strings.Contains(content.URL, "chrome-error://") {
			fmt.Printf("[%s] [网络搜索] 页面加载失败(错误页)，跳过: %s\n", ModuleName, content.URL)
			continue
		}

		var summary string
		if strings.TrimSpace(content.TextContent) != "" && aiSummarizeContent != nil {
			summary, _ = aiSummarizeContent(content.TextContent, nil)
		} else if content.ContentType == "visual" && aiSummarizeVisualContent != nil {
			var screens [][]byte
			for _, ss := range content.Screenshots {
				screens = append(screens, ss.ImageData)
			}
			if len(screens) > 0 {
				summary, _ = aiSummarizeVisualContent(screens)
			}
		}
		if strings.TrimSpace(summary) == "" {
			fmt.Printf("[%s] [网络搜索] 页面无可用内容，跳过: %s\n", ModuleName, content.URL)
			continue
		}

		a.attemptedPages = append(a.attemptedPages, content.URL)
		attemptedSums = append(attemptedSums, summary)

		// 嵌入打分 + 核心实体关键词比对，满足其一即视为有效信息
		if valid, sim := a.isSummaryValid(summary); valid {
			validSums = append(validSums, summary)
			validSrcs = append(validSrcs, content.URL)
			fmt.Printf("[%s] [网络搜索] 摘要有效(相似度=%.2f)，保留\n", ModuleName, sim)
		} else {
			fmt.Printf("[%s] [网络搜索] 摘要与查询无关(相似度=%.2f)，跳过该页: %s\n", ModuleName, sim, truncateText(summary, 50))
		}
	}

	return validSums, validSrcs, attemptedSums, false
}

// filterByTitleEntities 步骤6：用核心实体完整名过滤标题
// 仅当提取到了核心实体时生效；标题不含任意核心实体完整名的结果直接淘汰。
// 实体为空时放行（回退到仅靠嵌入打分），避免误杀标题碎片化的有效页面。
func (a *SearchAgent) filterByTitleEntities(results []SearchResult) []SearchResult {
	if len(a.coreEntities) == 0 {
		return results
	}
	kept := make([]SearchResult, 0, len(results))
	for _, r := range results {
		if containsAnyEntity(r.Title, a.coreEntities) {
			kept = append(kept, r)
		} else {
			fmt.Printf("[%s] [网络搜索] 标题不含核心实体，初筛跳过: %s\n", ModuleName, truncateText(r.Title, 50))
		}
	}
	return kept
}

// isSummaryValid 步骤7：判定单条摘要是否构成有效信息
// 条件一：摘要包含核心实体完整名 → 直接有效（硬命中，解决搜索引擎歧义）
// 条件二：摘要与初始查询的嵌入余弦相似度过阈值 → 有效
// 返回 (是否有效, 嵌入相似度)
func (a *SearchAgent) isSummaryValid(summary string) (bool, float32) {
	if containsAnyEntity(summary, a.coreEntities) {
		return true, 1.0
	}
	if len(a.queryEmbedding) > 0 {
		if emb, err := callEmbedding(truncateText(summary, 200)); err == nil {
			sim := cosineSimilarity32(a.queryEmbedding, emb)
			if sim >= float32(EmbedRelevanceThreshold) {
				return true, sim
			}
			return false, sim
		}
	}
	return false, 0
}

// containsAnyEntity 判断文本是否包含任一核心实体名
// 做分隔符归一化（如"钛宇·星光阁"/"钛宇-星光阁"与"钛宇星光阁"视为相同），
// 并对含空格的长实体按分隔符拆分为多个 token 逐个匹配，避免长串实体误杀相关标题。
func containsAnyEntity(text string, entities []string) bool {
	normText := normalizeForMatch(text)
	for _, e := range entities {
		e = strings.TrimSpace(e)
		if e == "" {
			continue
		}
		for _, tok := range splitEntityTokens(e) {
			nt := normalizeForMatch(tok)
			if len([]rune(nt)) >= 2 && strings.Contains(normText, nt) {
				return true
			}
		}
	}
	return false
}

// normalizeForMatch 归一化文本用于实体匹配：转小写并移除常见分隔符与空白
func normalizeForMatch(s string) string {
	s = strings.ToLower(s)
	return strings.Map(func(r rune) rune {
		switch r {
		case ' ', '　', '·', '-', '_', '/', '\\', '.', '、', '，', ',', '（', '）', '(', ')', '《', '》', '"', '\'':
			return -1
		}
		return r
	}, s)
}

// splitEntityTokens 将实体按分隔符切分为独立 token
func splitEntityTokens(e string) []string {
	f := func(r rune) bool {
		switch r {
		case ' ', '　', '·', '-', '_', '/', '\\', '.', '、', '，', ',', '（', '）', '(', ')', '《', '》':
			return true
		}
		return false
	}
	return strings.FieldsFunc(e, f)
}

// buildInitialQuery 将关键词数组去重后以空格拼接为初始查询语句
func buildInitialQuery(keywords []string) string {
	seen := make(map[string]bool)
	var parts []string
	for _, k := range keywords {
		k = strings.TrimSpace(k)
		if k == "" || seen[k] {
			continue
		}
		seen[k] = true
		parts = append(parts, k)
	}
	return strings.Join(parts, " ")
}

// detectSearchFallback 检测搜索结果是否为 Bing 工具站兜底
// 当绝大多数（≥2/3）结果是快递/物流/在线工具/whois/学信网/地图等固定工具站时，判定为触发了工具站兜底。
func detectSearchFallback(results []SearchResult) bool {
	if len(results) == 0 {
		return false
	}
	markers := []string{
		"快递", "物流", "运单", "货运", "kuaidi", "ickd", "zto",
		"17track", "17TRACK", "IP地址", "在线工具", "whois", "域名查询",
		"站长工具", "学信网", "chsi", "map.baidu", "爱企查", "工商查询",
	}
	var hit int
	for _, r := range results {
		blob := strings.ToLower(r.Title + " " + r.URL)
		for _, m := range markers {
			if strings.Contains(blob, m) {
				hit++
				break
			}
		}
	}
	return hit*3 >= len(results)*2 // ≥ 2/3 为工具站
}

// enhanceSearchText 基于原始问题推测真实意图，产出强化后的搜索文本
// priorExperience 为上一轮已尝试的页面摘要；memoryHints 为相关的历史记忆提示
func (a *SearchAgent) enhanceSearchText(query string, priorExperience string) (string, error) {
	if aiEnhanceSearchText == nil {
		return query, nil
	}
	return aiEnhanceSearchText(query, priorExperience, strings.Join(a.memoryHints, "\n"))
}

// phaseUnknownAnswer 增强搜索后仍无法解答：返回"月华不知道"，并把失败经验记入记忆库
func (a *SearchAgent) phaseUnknownAnswer(query string) (*SearchReport, error) {
	fmt.Printf("[%s] [网络搜索] 增强搜索仍无法解答，返回月华不知道\n", ModuleName)

	if memoryStore != nil {
		record := MemorySearchRecord{
			Question: query,
			Keywords: a.usedKeywords,
			KeyFindings: fmt.Sprintf("该问题未能得到解答。本次尝试的搜索文本：%s；尝试过的页面：%s",
				strings.Join(a.usedKeywords, "、"), strings.Join(a.attemptedPages, "、")),
			Answer:    "（月华不知道：该问题未得到解答）",
			Timestamp: time.Now().Unix(),
		}
		if err := memoryStore(record); err != nil {
			fmt.Printf("[%s] 失败经验记忆存储失败: %v\n", ModuleName, err)
		} else {
			fmt.Printf("[%s] 已把失败经验记入记忆库\n", ModuleName)
		}
	}

	return &SearchReport{
		Query:       query,
		Answer:      "月华不知道该问题的答案",
		FromMemory:  false,
		GeneratedAt: time.Now(),
	}, nil
}

// reportNotFound 构造"未找到信息"的占位报告
func (a *SearchAgent) reportNotFound(query string) *SearchReport {
	return &SearchReport{
		Query:       query,
		Answer:      "月华找不到你想要的信息",
		FromMemory:  false,
		GeneratedAt: time.Now(),
	}
}

// =============================================================================
// Phase 1: 记忆检索
// =============================================================================

// phaseMemoryLookup 查询记忆库，判定是否可以直接使用历史答案
// 返回非 nil 表示记忆足够，跳过后续搜索直接返回
// 规则：无匹配 → nil；时效性需求 → nil；用户明确要求搜索/网络 → nil；匹配且足够 → 返回记忆答案
func (a *SearchAgent) phaseMemoryLookup(query, initialQuery string) *SearchReport {
	if memoryLookup == nil {
		fmt.Printf("[%s] 记忆库未集成，跳过记忆检索\n", ModuleName)
		return nil
	}

	// 用户明确要求"搜索/网络查询"，跳过记忆直接网络搜索
	if hasExplicitWebIntent(query) {
		fmt.Printf("[%s] 用户明确要求搜索/网络查询，跳过记忆，直接进入网络搜索\n", ModuleName)
		return nil
	}

	fmt.Printf("[%s] [阶段 1/5] 记忆检索中...\n", ModuleName)

	// 用清洗后的初始查询（关键词拼接）检索记忆，去除口语噪声，提升召回质量
	lookupText := initialQuery
	if strings.TrimSpace(lookupText) == "" {
		lookupText = query
	}

	entries, err := memoryLookup(lookupText, 5)
	if err != nil {
		fmt.Printf("[%s] 记忆检索失败: %v，继续网络搜索\n", ModuleName, err)
		return nil
	}

	if len(entries) == 0 {
		fmt.Printf("[%s] 记忆库无相关记录\n", ModuleName)
		return nil
	}

	// 过滤低相似度结果
	var relevantEntries []memoryEntry
	for _, e := range entries {
		if e.Similarity >= float32(MemorySimilarityMin) {
			relevantEntries = append(relevantEntries, e)
		}
	}

	if len(relevantEntries) == 0 {
		fmt.Printf("[%s] 记忆库结果相似度过低（< %.0f%%），继续网络搜索\n", ModuleName, MemorySimilarityMin*100)
		return nil
	}

	// 把相关记忆作为增强搜索的提示（即使不能直接作答，也能帮忙避开错误方向）
	for _, e := range relevantEntries {
		a.memoryHints = append(a.memoryHints, e.Content)
	}

	// 直接复用门槛：只有最佳匹配足够相似才允许用记忆直接作答。
	// 否则（相似但不对应同一实体）误用历史答案会造成张冠李戴，继续走网络搜索更稳妥。
	if relevantEntries[0].Similarity < float32(MemoryDirectAnswerMin) {
		fmt.Printf("[%s] 记忆库最佳匹配相似度 %.0f%% < 直接复用门槛 %.0f%%，继续网络搜索\n",
			ModuleName, relevantEntries[0].Similarity*100, MemoryDirectAnswerMin*100)
		return nil
	}

	// 构建记忆上下文
	var memContext strings.Builder
	memContext.WriteString("以下是历史搜索记录：\n\n")
	for i, e := range relevantEntries {
		memContext.WriteString(fmt.Sprintf("记录 %d [相似度: %.0f%%]:\n%s\n\n",
			i+1, e.Similarity*100, e.Content))
	}

	// AI 判定：记忆是否足够
	if aiJudgeMemory == nil {
		fmt.Printf("[%s] AI 判定未集成，跳过记忆评估\n", ModuleName)
		return nil
	}

	sufficient, timeSensitive, err := aiJudgeMemory(memContext.String(), query)
	if err != nil {
		fmt.Printf("[%s] 记忆评估失败: %v，继续网络搜索\n", ModuleName, err)
		return nil
	}

	if timeSensitive {
		fmt.Printf("[%s] 查询具有时效性要求，跳过记忆结果，执行网络搜索\n", ModuleName)
		return nil
	}

	if sufficient {
		// 从相关记忆中筛选出"非失败记录"作为可复用答案。
		// 历史"未解答/信息不足"的失败记录不应作为答案直接返回（它们只作失败经验提示）。
		bestAnswer := ""
		for _, e := range relevantEntries {
			if !isFailedMemoryContent(e.Content) {
				bestAnswer = e.Content
				break
			}
		}
		if bestAnswer == "" {
			fmt.Printf("[%s] 相关记忆均为失败记录，继续网络搜索\n", ModuleName)
			return nil
		}
		fmt.Printf("[%s] 记忆库内容足够回答，跳过网络搜索\n", ModuleName)

		return &SearchReport{
			Query:       query,
			Answer:      bestAnswer,
			FromMemory:  true,
			GeneratedAt: time.Now(),
		}
	}

	fmt.Printf("[%s] 记忆库内容不足，继续网络搜索\n", ModuleName)
	return nil
}

// isFailedMemoryContent 判断记忆记录内容是否为"未解答/信息不足"的失败记录
// 这类记录不应作为答案直接复用，只能作为失败经验提示（memoryHints）用于避开错误方向。
func isFailedMemoryContent(content string) bool {
	for _, m := range []string{
		"月华不知道该问题的答案",
		"月华找不到你想要的信息",
		"该问题未能得到解答",
		"未能得到解答",
		"不足以确定",
		"信息不足",
		"无法解答",
	} {
		if strings.Contains(content, m) {
			return true
		}
	}
	return false
}

// hasExplicitWebIntent 判断用户是否明确要求走网络搜索/查询，而不是查记忆
func hasExplicitWebIntent(query string) bool {
	for _, kw := range []string{
		"网上搜索", "去网上查", "上网查", "网络搜索", "网络查询",
		"帮我搜索", "实时查询", "在搜索引擎", "搜索一下最新", "查一下最新",
	} {
		if strings.Contains(query, kw) {
			return true
		}
	}
	return false
}

// cleanSearchText 剥离用户口语套话，提取核心实体与关键词用于搜索引擎
// 原因：Bing 对"查询一下…信息/在哪里/是什么"这类完整口语句会返回工具站兜底页，
// 只有把核心实体+限定词直接喂给搜索引擎才能命中真实、有序的网页结果。
func cleanSearchText(raw string) string {
	s := raw
	for _, chatter := range []string{
		"替我查询一下", "帮我查询一下", "请帮我查一下", "帮我搜索一下", "帮我搜一下",
		"查询一下", "帮我查询", "我来查一下", "想问一下", "我想问一下", "麻烦查一下", "帮忙查一下",
		"查询", "帮我查", "帮我搜", "搜索一下", "想知道", "想了解", "请问", "请查询",
		"的信息", "的信息资料", "相关情报", "的情报", "情报资料", "资料介绍",
		"在哪里", "在哪儿", "在什么地方", "是什么", "是什么意思", "是哪里", "有哪些", "哪些",
		"有什么", "有没有", "怎么样", "如何", "情况", "信息",
	} {
		s = strings.ReplaceAll(s, chatter, " ")
	}

	// 清理残留标点与多余空白
	replacer := strings.NewReplacer("，", " ", ",", " ", "。", "", "？", "", "？", "", "！", "", "!", "", "？", "")
	s = replacer.Replace(s)
	s = strings.Join(strings.Fields(s), " ")
	return strings.TrimSpace(s)
}

// =============================================================================
// Phase 4: 报告生成
// =============================================================================

// phaseGenerateReport 基于所有积累的摘要生成最终搜索报告
func (a *SearchAgent) phaseGenerateReport(query string, searchRounds int) (*SearchReport, error) {
	fmt.Printf("[%s] [阶段 4/5] 报告生成中...\n", ModuleName)

	if aiGenerateReport == nil {
		// 无 AI 时返回原始摘要拼接
		rawAnswer := strings.Join(a.accumulatedSummaries, "\n\n---\n\n")
		return &SearchReport{
			Query:        query,
			Answer:       rawAnswer,
			UsedSources:  a.accumulatedSources,
			SearchRounds: searchRounds,
			GeneratedAt:  time.Now(),
		}, nil
	}

	answer, err := aiGenerateReport(query, a.accumulatedSummaries, a.accumulatedSources)
	if err != nil {
		return nil, fmt.Errorf("报告生成失败: %w", err)
	}

	return &SearchReport{
		Query:        query,
		Answer:       answer,
		FromMemory:   false,
		UsedSources:  a.accumulatedSources,
		SearchRounds: searchRounds,
		GeneratedAt:  time.Now(),
	}, nil
}

// =============================================================================
// Phase 5: 记忆存储
// =============================================================================

// phaseStoreToMemory 将搜索结果存入记忆库
func (a *SearchAgent) phaseStoreToMemory(query string, report *SearchReport) {
	if memoryStore == nil || report.FromMemory {
		return
	}

	// 拒绝把"信息不足/未找到"的低质量结果写入记忆库，避免后续查询误复用无效答案
	if isLowQualityAnswer(report) {
		fmt.Printf("[%s] 本次结果为信息不足/未找到，不入记忆库\n", ModuleName)
		return
	}

	fmt.Printf("[%s] [阶段 5/5] 存储搜索结果到记忆库...\n", ModuleName)

	record := MemorySearchRecord{
		Question:    query,
		Keywords:    a.usedKeywords,
		KeyFindings: strings.Join(a.accumulatedSummaries, "\n"),
		Answer:      report.Answer,
		Timestamp:   time.Now().Unix(),
	}

	if err := memoryStore(record); err != nil {
		fmt.Printf("[%s] 记忆存储失败: %v\n", ModuleName, err)
	} else {
		fmt.Printf("[%s] 搜索结果已存入 search_memory\n", ModuleName)
	}
}

// =============================================================================
// 内部辅助方法
// =============================================================================

// isLowQualityAnswer 判断报告是否为"信息不足/未找到"的低质量结果
// 此类结果不应写入记忆库，否则会被后续相似查询误复用
func isLowQualityAnswer(report *SearchReport) bool {
	if report == nil {
		return true
	}

	// 明确的无结果占位
	if strings.Contains(report.Answer, "找不到你想要的信息") {
		return true
	}

	// 信息不足/未包含等失效表述
	for _, marker := range []string{
		"未包含", "未找到", "没有找到", "无法回答", "不足以回答",
	} {
		if strings.Contains(report.Answer, marker) {
			return true
		}
	}

	// 完全无来源且无答案
	if len(report.UsedSources) == 0 && strings.TrimSpace(report.Answer) == "" {
		return true
	}

	return false
}

// filterDictionarySites 过滤字典/词典类网站
// 搜索智能体自身具备字典能力，无需浪费浏览器时间和模型 token 查阅外部字典网站
func filterDictionarySites(results []SearchResult) []SearchResult {
	if len(results) == 0 {
		return results
	}

	filtered := make([]SearchResult, 0, len(results))
	for _, r := range results {
		if isDictionarySite(r.URL, r.Title) {
			fmt.Printf("[%s] 过滤字典网站: %s\n", ModuleName, r.URL)
			continue
		}
		filtered = append(filtered, r)
	}
	return filtered
}

// isDictionarySite 检查 URL 或标题是否包含字典关键词
func isDictionarySite(url, title string) bool {
	combined := strings.ToLower(url + " " + title)
	for _, kw := range dictionaryKeywords {
		if strings.Contains(combined, kw) {
			return true
		}
	}
	return false
}

// =============================================================================
// 浏览器健康管理
// =============================================================================

// ensureBrowserHealthy 确保浏览器处于健康状态
// 浏览器刚启动/重启后首次查询跳过健康检查，避免进程注册延迟导致误判
func ensureBrowserHealthy() error {
	if browserJustLaunched {
		browserJustLaunched = false
		return nil
	}

	if !browserLaunched {
		return tryRestartBrowser()
	}

	if checkBrowserHealth == nil {
		// 无监控时简单检查浏览器是否启动
		if !browserLaunched {
			return tryRestartBrowser()
		}
		return nil
	}

	health := checkBrowserHealth()
	if health.Healthy {
		return nil
	}

	fmt.Printf("[%s] 浏览器不健康 (内存=%dMB CPU=%.1f%%)，尝试重启...\n",
		ModuleName, health.MemMB, health.CPUPercent)

	return tryRestartBrowser()
}

// tryRestartBrowser 尝试重启浏览器，每次清理资源后重试，最多 MaxBrowserRetryAttempts 次
func tryRestartBrowser() error {
	fmt.Printf("[%s] 正在重启浏览器...\n", ModuleName)

	var lastErr error
	for attempt := 1; attempt <= MaxBrowserRetryAttempts; attempt++ {
		CloseBrowser()
		ResetCPUTracking() // 重置 CPU 追踪状态

		if err := LaunchBrowser(); err != nil {
			lastErr = err
			fmt.Printf("[%s] 浏览器重启第 %d/%d 次失败: %v\n",
				ModuleName, attempt, MaxBrowserRetryAttempts, err)
			continue
		}

		fmt.Printf("[%s] 浏览器重启成功\n", ModuleName)
		return nil
	}

	return fmt.Errorf("月华的浏览器重启 %d 次仍失败，找不到你想要的内容: %v",
		MaxBrowserRetryAttempts, lastErr)
}

// =============================================================================
// 通用辅助函数
// =============================================================================

// truncateText 截断文本到指定最大字符数（保留完整 rune）
func truncateText(text string, maxLen int) string {
	runes := []rune(text)
	if len(runes) <= maxLen {
		return text
	}
	return string(runes[:maxLen]) + "\n...(内容已截断)"
}
