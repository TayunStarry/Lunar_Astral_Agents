package websearch

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

// ============================================================
// 大会辩论编排器
// ============================================================

// NewAssembly 创建大会辩论系统
func NewAssembly(simple *SimpleSearcher, llmProvider Provider, memProvider MemoryProvider, cfg DepthConfig) *Assembly {
	return &Assembly{
		simple:      simple,
		llmProvider: llmProvider,
		memProvider: memProvider,
		cfg:         cfg,
	}
}

// Search 执行大会辩论式深度搜索
func (a *Assembly) Search(query string) (string, error) {
	if a.llmProvider == nil {
		return a.fallbackSearch(query)
	}

	state := &AssemblyState{
		OriginalQuery: query,
	}

	// 阶段1: 议题拆解
	topics, err := a.decomposeTopics(query)
	if err != nil || len(topics) == 0 {
		topics = []string{query}
	}
	state.Topics = topics

	// 阶段2: 独立调研（并行）
	if err := a.phaseResearch(state); err != nil {
		return a.fallbackSearch(query)
	}

	// 检查搜索结果是否全部为空
	hasSearchResults := false
	for _, results := range state.SearchResults {
		if len(results) > 0 {
			hasSearchResults = true
			break
		}
	}
	hasMemoryResults := len(state.MemoryResults) > 0

	// 搜索结果和记忆都为空，跳过辩论直接返回
	if !hasSearchResults && !hasMemoryResults {
		return fmt.Sprintf("# 深度研究报告\n\n原始问题：%s\n\n## 核心结论\n\n抱歉，网络搜索和记忆库均未找到与「%s」相关的信息。\n\n建议：\n- 尝试更换搜索关键词\n- 使用简易搜索或常规搜索模式\n- 提供更多上下文信息以便更精准地搜索", query, query), nil
	}

	// 搜索结果为空但记忆有结果时，跳过辩论直接用记忆结果
	if !hasSearchResults && hasMemoryResults {
		return a.phaseSynthesizeFromMemory(state)
	}

	// 阶段3: 辩论回合
	a.phaseDebate(state)

	// 阶段4: 综合报告
	return a.phaseSynthesize(state)
}

// decomposeTopics 拆解议题
func (a *Assembly) decomposeTopics(query string) ([]string, error) {
	// 专有名词/短查询不拆解，避免 LLM 幻觉子问题
	if isProperNounQuery(query) {
		return []string{query}, nil
	}

	if a.llmProvider == nil {
		return []string{query}, nil
	}

	prompt := buildDecomposePrompt(query)
	messages := []ChatMessage{
		{Role: "system", Content: synthesizerSystemPrompt()},
		{Role: "user", Content: prompt},
	}

	response, err := a.llmProvider.Chat(messages)
	if err != nil {
		return nil, err
	}

	response = strings.TrimSpace(response)
	if idx := strings.Index(response, "["); idx >= 0 {
		if endIdx := strings.LastIndex(response, "]"); endIdx > idx {
			response = response[idx : endIdx+1]
		}
	}

	var topics []string
	if err := json.Unmarshal([]byte(response), &topics); err != nil {
		return []string{query}, nil
	}

	maxSub := a.cfg.MaxSubQueries
	if maxSub <= 0 {
		maxSub = 4
	}
	if len(topics) > maxSub {
		topics = topics[:maxSub]
	}

	// 始终把原始查询放在第一位
	hasOriginal := false
	for _, t := range topics {
		if t == query {
			hasOriginal = true
			break
		}
	}
	if !hasOriginal {
		topics = append([]string{query}, topics...)
		if len(topics) > maxSub {
			topics = topics[:maxSub]
		}
	}

	return topics, nil
}

// phaseResearch 独立调研阶段（并行搜索每个子题 + 检索记忆 + 相关性过滤 + 多策略回退）
func (a *Assembly) phaseResearch(state *AssemblyState) error {
	state.SearchResults = make(map[string][]SearchResult)
	state.MemoryResults = make(map[string]string)

	var wg sync.WaitGroup

	maxResults := a.cfg.MaxResults
	if maxResults <= 0 {
		maxResults = 10
	}

	// 对每个子题做网络搜索（并行）
	seenQueries := make(map[string]bool)
	for _, topic := range state.Topics {
		if seenQueries[topic] {
			continue
		}
		seenQueries[topic] = true
		wg.Add(1)
		go func(t string) {
			defer wg.Done()
			keywords := extractQueryKeywords(t)

			// 策略1：标准搜索（Bing 带预处理）
			results, _ := a.simple.SearchRaw(t)
			filtered := filterByKeywords(results, keywords, maxResults)

			// 策略2：标准搜索全被过滤 → 尝试替代查询（去引号、去分隔符、DDG）
			if len(filtered) == 0 {
				for _, altQ := range buildAltQueries(t) {
					altResults, altErr := a.simple.SearchRawNoPrep(altQ)
					if altErr != nil {
						continue
					}
					for _, r := range altResults {
						if len(filtered) >= maxResults {
							break
						}
						if checkContentRelevance(keywords, "", r.Title, r.Snippet) >= 1 {
							filtered = append(filtered, r)
						}
					}
					if len(filtered) > 0 {
						break
					}
				}
			}

			state.SearchResults[t] = filtered
		}(topic)
	}

	// 对原始查询做记忆检索
	if a.memProvider != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			memResult, memErr := a.memProvider.Query(state.OriginalQuery)
			if memErr == nil {
				state.MemoryResults[state.OriginalQuery] = memResult
			}
		}()
	}

	wg.Wait()
	return nil
}

// phaseDebate 辩论回合
func (a *Assembly) phaseDebate(state *AssemblyState) {
	maxRounds := a.cfg.MaxRounds
	if maxRounds <= 0 {
		maxRounds = 3
	}
	if maxRounds > 5 {
		maxRounds = 5
	}

	for round := 1; round <= maxRounds; round++ {
		state.CurrentRound = round

		debateRound := a.executeDebateRound(state)
		state.Rounds = append(state.Rounds, debateRound)

		if debateRound.Converged {
			state.Converged = true
			break
		}
	}
}

// executeDebateRound 执行一轮辩论
func (a *Assembly) executeDebateRound(state *AssemblyState) DebateRound {
	round := DebateRound{Round: state.CurrentRound}

	// 构建辩论历史
	debateHistory := a.buildDebateHistory(state)
	searchResultsStr := a.formatSearchResults(state)
	memoryResultsStr := a.formatMemoryResults(state)

	var wg sync.WaitGroup

	// 组1: ReformistA + ConservativeA
	wg.Add(2)
	go func() {
		defer wg.Done()
		round.ReformistA = a.callDelegate(RoleReformist, "维新派A", reformistASystemPrompt(),
			debateHistory, searchResultsStr, memoryResultsStr)
	}()
	go func() {
		defer wg.Done()
		round.ConservativeA = a.callDelegate(RoleConservative, "守旧派A", conservativeASystemPrompt(),
			debateHistory, searchResultsStr, memoryResultsStr)
	}()
	wg.Wait()

	// 组2: Supporter + Opponent
	wg.Add(2)
	go func() {
		defer wg.Done()
		round.Supporter = a.callDelegate(RoleSupporter, "赞同者", supporterSystemPrompt(),
			debateHistory+"\n\n维新派A: "+round.ReformistA+"\n守旧派A: "+round.ConservativeA,
			searchResultsStr, memoryResultsStr)
	}()
	go func() {
		defer wg.Done()
		round.Opponent = a.callDelegate(RoleOpponent, "反对者", opponentSystemPrompt(),
			debateHistory+"\n\n维新派A: "+round.ReformistA+"\n守旧派A: "+round.ConservativeA,
			searchResultsStr, memoryResultsStr)
	}()
	wg.Wait()

	// 组3: ReformistB + ConservativeB
	wg.Add(2)
	go func() {
		defer wg.Done()
		round.ReformistB = a.callDelegate(RoleReformist, "维新派B", reformistBSystemPrompt(),
			debateHistory+fmt.Sprintf("\n\n维新派A: %s\n守旧派A: %s\n赞同者: %s\n反对者: %s",
				round.ReformistA, round.ConservativeA, round.Supporter, round.Opponent),
			searchResultsStr, memoryResultsStr)
	}()
	go func() {
		defer wg.Done()
		round.ConservativeB = a.callDelegate(RoleConservative, "守旧派B", conservativeBSystemPrompt(),
			debateHistory+fmt.Sprintf("\n\n维新派A: %s\n守旧派A: %s\n赞同者: %s\n反对者: %s",
				round.ReformistA, round.ConservativeA, round.Supporter, round.Opponent),
			searchResultsStr, memoryResultsStr)
	}()
	wg.Wait()

	// 组4: Synthesizer 判断收敛
	round.Verdict = a.callDelegate(RoleSynthesizer, "整合者", synthesizerSystemPrompt(),
		debateHistory+fmt.Sprintf("\n\n维新派A: %s\n守旧派A: %s\n赞同者: %s\n反对者: %s\n维新派B: %s\n守旧派B: %s",
			round.ReformistA, round.ConservativeA, round.Supporter, round.Opponent,
			round.ReformistB, round.ConservativeB),
		searchResultsStr, memoryResultsStr)

	round.Converged = strings.Contains(strings.ToUpper(round.Verdict), "CONVERGED")

	return round
}

// callDelegate 调用一位代表
func (a *Assembly) callDelegate(role DelegateRole, name string, systemPrompt string, debateHistory string, searchResults string, memoryResults string) string {
	userPrompt := buildDebateRoundPrompt(role, name, debateHistory, searchResults, memoryResults)

	messages := []ChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}

	response, err := a.llmProvider.Chat(messages)
	if err != nil {
		return fmt.Sprintf("[%s发言失败: %v]", name, err)
	}

	return strings.TrimSpace(response)
}

// buildDebateHistory 构建辩论历史文本
func (a *Assembly) buildDebateHistory(state *AssemblyState) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("原始问题: %s\n", state.OriginalQuery))
	sb.WriteString(fmt.Sprintf("议题: %s\n\n", strings.Join(state.Topics, "、")))

	for _, r := range state.Rounds {
		sb.WriteString(fmt.Sprintf("--- 第%d轮辩论 ---\n", r.Round))
		if r.ReformistA != "" {
			sb.WriteString(fmt.Sprintf("维新派A: %s\n", truncateText(r.ReformistA, 300)))
		}
		if r.ConservativeA != "" {
			sb.WriteString(fmt.Sprintf("守旧派A: %s\n", truncateText(r.ConservativeA, 300)))
		}
		if r.Supporter != "" {
			sb.WriteString(fmt.Sprintf("赞同者: %s\n", truncateText(r.Supporter, 300)))
		}
		if r.Opponent != "" {
			sb.WriteString(fmt.Sprintf("反对者: %s\n", truncateText(r.Opponent, 300)))
		}
		if r.ReformistB != "" {
			sb.WriteString(fmt.Sprintf("维新派B: %s\n", truncateText(r.ReformistB, 300)))
		}
		if r.ConservativeB != "" {
			sb.WriteString(fmt.Sprintf("守旧派B: %s\n", truncateText(r.ConservativeB, 300)))
		}
		if r.Verdict != "" {
			sb.WriteString(fmt.Sprintf("整合者判断: %s\n", truncateText(r.Verdict, 300)))
		}
	}

	return sb.String()
}

// formatSearchResults 格式化搜索结果
func (a *Assembly) formatSearchResults(state *AssemblyState) string {
	if len(state.SearchResults) == 0 {
		return ""
	}

	var sb strings.Builder
	for query, results := range state.SearchResults {
		sb.WriteString(fmt.Sprintf("查询: %s\n", query))
		for i, r := range results {
			if i >= 10 {
				break
			}
			snippet := r.Snippet
			if len([]rune(snippet)) > 200 {
				snippet = string([]rune(snippet)[:200]) + "..."
			}
			sb.WriteString(fmt.Sprintf("  [%d] %s\n  URL: %s\n  摘要: %s\n", i+1, r.Title, r.URL, snippet))
		}
	}
	return sb.String()
}

// formatMemoryResults 格式化记忆检索结果
func (a *Assembly) formatMemoryResults(state *AssemblyState) string {
	if len(state.MemoryResults) == 0 {
		return ""
	}

	var sb strings.Builder
	for query, result := range state.MemoryResults {
		sb.WriteString(fmt.Sprintf("查询: %s\n%s\n", query, result))
	}
	return sb.String()
}

// phaseSynthesize 综合报告阶段
func (a *Assembly) phaseSynthesize(state *AssemblyState) (string, error) {
	debateHistory := a.buildFullDebateHistory(state)
	prompt := buildFinalReportPrompt(state.OriginalQuery, debateHistory)

	messages := []ChatMessage{
		{Role: "system", Content: synthesizerSystemPrompt()},
		{Role: "user", Content: prompt},
	}

	response, err := a.llmProvider.Chat(messages)
	if err != nil {
		return a.fallbackSearch(state.OriginalQuery)
	}

	// 输出截断保护
	responseRunes := []rune(response)
	if len(responseRunes) > 3000 {
		response = string(responseRunes[:3000]) + "\n\n[报告已截断]"
	}

	return response, nil
}

// buildFullDebateHistory 构建完整辩论历史（用于最终报告）
func (a *Assembly) buildFullDebateHistory(state *AssemblyState) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("原始问题: %s\n", state.OriginalQuery))
	sb.WriteString(fmt.Sprintf("议题: %s\n\n", strings.Join(state.Topics, "、")))

	// 搜索结果摘要
	sb.WriteString("=== 网络搜索结果 ===\n")
	sb.WriteString(a.formatSearchResults(state))
	sb.WriteString("\n=== 记忆检索结果 ===\n")
	sb.WriteString(a.formatMemoryResults(state))
	sb.WriteString("\n")

	for _, r := range state.Rounds {
		sb.WriteString(fmt.Sprintf("=== 第%d轮辩论 ===\n", r.Round))
		sb.WriteString(fmt.Sprintf("维新派A: %s\n\n", r.ReformistA))
		sb.WriteString(fmt.Sprintf("守旧派A: %s\n\n", r.ConservativeA))
		sb.WriteString(fmt.Sprintf("赞同者: %s\n\n", r.Supporter))
		sb.WriteString(fmt.Sprintf("反对者: %s\n\n", r.Opponent))
		sb.WriteString(fmt.Sprintf("维新派B: %s\n\n", r.ReformistB))
		sb.WriteString(fmt.Sprintf("守旧派B: %s\n\n", r.ConservativeB))
		sb.WriteString(fmt.Sprintf("整合者判断: %s\n\n", r.Verdict))
	}

	return sb.String()
}

// fallbackSearch 降级到简易搜索
func (a *Assembly) fallbackSearch(query string) (string, error) {
	result, err := a.simple.Search(query)
	if err != nil {
		return "", err
	}
	return "# 深度搜索（降级）\n\nLLM不可用，使用简易搜索：\n\n" + result, nil
}

// phaseSynthesizeFromMemory 仅用记忆结果生成报告（无搜索结果时跳过辩论）
func (a *Assembly) phaseSynthesizeFromMemory(state *AssemblyState) (string, error) {
	var memText strings.Builder
	for query, result := range state.MemoryResults {
		memText.WriteString(fmt.Sprintf("查询: %s\n%s\n", query, result))
	}

	prompt := fmt.Sprintf(`请基于以下记忆库信息，生成一份结构化的深度研究报告。

原始问题：%s

记忆库信息：
%s

要求：
1. 以"# 深度研究报告"开头
2. 包含"核心结论"章节（基于记忆信息总结，3-5点）
3. 包含"信息来源"章节（标注信息来自记忆库）
4. 说明网络搜索暂未找到相关信息，当前结果仅基于记忆
5. 用markdown格式输出，保持专业客观`, state.OriginalQuery, memText.String())

	messages := []ChatMessage{
		{Role: "system", Content: synthesizerSystemPrompt()},
		{Role: "user", Content: prompt},
	}

	response, err := a.llmProvider.Chat(messages)
	if err != nil {
		return a.fallbackSearch(state.OriginalQuery)
	}

	responseRunes := []rune(response)
	if len(responseRunes) > 3000 {
		response = string(responseRunes[:3000]) + "\n\n[报告已截断]"
	}

	return response, nil
}

// filterByKeywords 基于关键词过滤搜索结果，保留相关度 >= 1 的条目
func filterByKeywords(results []SearchResult, keywords []string, maxResults int) []SearchResult {
	filtered := make([]SearchResult, 0, maxResults)
	for _, r := range results {
		if len(filtered) >= maxResults {
			break
		}
		if checkContentRelevance(keywords, "", r.Title, r.Snippet) >= 1 {
			filtered = append(filtered, r)
		}
	}
	return filtered
}

// buildAltQueries 为专有名词查询生成替代搜索词（去分隔符变体）
func buildAltQueries(query string) []string {
	// 替换特殊分隔符为空格
	withSpace := query
	hasSpecial := false
	for _, old := range []string{"·", "-", "/", "|", "•", "\u2014", "～", "~", "、"} {
		if strings.Contains(withSpace, old) {
			withSpace = strings.ReplaceAll(withSpace, old, " ")
			hasSpecial = true
		}
	}
	// 移除特殊分隔符
	noSep := query
	for _, old := range []string{"·", "-", "/", "|", "•", "\u2014", "～", "~", "、"} {
		noSep = strings.ReplaceAll(noSep, old, "")
	}

	if !hasSpecial {
		return nil
	}

	alt := []string{}
	withSpace = strings.TrimSpace(withSpace)
	noSep = strings.TrimSpace(noSep)
	if withSpace != query {
		alt = append(alt, withSpace)
	}
	if noSep != query && noSep != withSpace {
		alt = append(alt, noSep)
	}
	return alt
}
