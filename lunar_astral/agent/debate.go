package agent

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"logger"
	"lunar_astral/hierarchy"
)

// DebateSystem 辩论系统
// 4阶段状态机：问题分析 → 并行搜索 → 辩论循环 → 综合报告
type DebateSystem struct {
	state  *DebateState
	llm    *LLMClient
	search *SearchManager
	memory *MemoryManager

	// Prompt 模板缓存
	promptAnalysis string
	promptDebate   string
	promptReport   string
	promptMemory   string
}

// NewDebateSystem 创建辩论系统
func NewDebateSystem(llm *LLMClient, search *SearchManager, memory *MemoryManager) *DebateSystem {
	ds := &DebateSystem{
		llm:    llm,
		search: search,
		memory: memory,
	}

	// 加载 prompt 模板
	ds.loadPrompts()

	return ds
}

// loadPrompts 从嵌入式文件系统加载 prompt 模板
func (d *DebateSystem) loadPrompts() {
	// 尝试从嵌入式文件系统加载
	if data, err := hierarchy.EmbeddedFiles.ReadFile("assets/prompts/learnerAnalysis.md"); err == nil {
		d.promptAnalysis = string(data)
	}
	if data, err := hierarchy.EmbeddedFiles.ReadFile("assets/prompts/learnerDebate.md"); err == nil {
		d.promptDebate = string(data)
	}
	if data, err := hierarchy.EmbeddedFiles.ReadFile("assets/prompts/learnerReport.md"); err == nil {
		d.promptReport = string(data)
	}
	if data, err := hierarchy.EmbeddedFiles.ReadFile("assets/prompts/learnerMemory.md"); err == nil {
		d.promptMemory = string(data)
	}

	// 如果加载失败，使用内置默认模板
	if d.promptAnalysis == "" {
		d.promptAnalysis = defaultAnalysisPrompt
	}
	if d.promptDebate == "" {
		d.promptDebate = defaultDebatePrompt
	}
	if d.promptReport == "" {
		d.promptReport = defaultReportPrompt
	}
	if d.promptMemory == "" {
		d.promptMemory = defaultMemoryPrompt
	}
}

// Execute 执行完整的辩论研究流程
func (d *DebateSystem) Execute(query string) (*LearnerResult, error) {
	d.state = &DebateState{
		OriginalQuery: query,
		MaxRounds:     MaxDebateRounds,
		CurrentPhase:  PhaseAnalyze,
	}

	// 注入运行时上下文（时间 + 位置）
	contextInfo := d.getRuntimeContext()

	// 阶段1: 问题分析
	logger.Info("Learner", "=== 阶段1: 问题分析 ===")
	if err := d.analyzeQuestion(query, contextInfo); err != nil {
		logger.Error("Learner", "问题分析失败: %v", err)
		// 降级：使用原始查询作为唯一子问题
		d.state.SubQuestions = []SubQuestion{
			{Question: query, SearchQuery: query, Source: "降级-原始查询"},
		}
	}

	// 阶段2: 并行搜索
	logger.Info("Learner", "=== 阶段2: 并行搜索 ===")
	if err := d.parallelSearch(); err != nil {
		logger.Error("Learner", "并行搜索失败: %v", err)
	}

	// 阶段3: 辩论循环
	logger.Info("Learner", "=== 阶段3: 辩论循环 ===")
	if err := d.runDebate(); err != nil {
		logger.Error("Learner", "辩论循环失败: %v", err)
	}

	// 阶段4: 综合报告
	logger.Info("Learner", "=== 阶段4: 综合报告 ===")
	report, err := d.synthesizeReport()
	if err != nil {
		logger.Error("Learner", "综合报告失败: %v", err)
		return nil, fmt.Errorf("综合报告生成失败: %w", err)
	}

	// 记忆更新
	logger.Info("Learner", "=== 记忆更新 ===")
	d.updateMemory(report)

	// 构建结果
	result := &LearnerResult{
		Report:       report,
		SearchRounds: len(d.state.SubQuestions),
		DebateRounds: len(d.state.Rounds),
	}

	// 收集信息来源
	for _, sq := range d.state.SubQuestions {
		if sq.Source != "" {
			result.Sources = append(result.Sources, sq.Source)
		}
	}

	return result, nil
}

// ============================================================
// 阶段1: 问题分析
// ============================================================

// analyzeQuestion 分析问题并拆解子问题
// query 参数为完整上下文（包含未读消息和对话历史），LLM 需从中提取真正的搜索意图
func (d *DebateSystem) analyzeQuestion(query string, contextInfo string) error {
	d.state.CurrentPhase = PhaseAnalyze

	// 将完整上下文传给 LLM，指示其从中提取真正的搜索/研究意图
	prompt := fmt.Sprintf(`%s

当前运行时上下文：
%s

以下是用户的对话内容（包含最新消息和历史对话）。
请从中提取用户真正的搜索/研究意图，然后拆解为子问题。

对话内容：
%s`, d.promptAnalysis, contextInfo, query)

	messages := []LLMMessage{
		{Role: "system", Content: prompt},
		{Role: "user", Content: "请分析上述对话内容，提取用户的搜索/研究意图并拆解为子问题。"},
	}

	resp, err := d.llm.Chat(messages, BudgetAnalyze)
	if err != nil {
		return fmt.Errorf("LLM 问题分析失败: %w", err)
	}

	// 解析 JSON 输出
	content := strings.TrimSpace(resp.Content)

	// 提取 JSON 部分（可能被 markdown 代码块包裹）
	jsonStr := extractJSON(content)

	var analysis struct {
		Topic        string   `json:"topic"`
		Complexity   string   `json:"complexity"`
		Dimensions   []string `json:"dimensions"`
		SubQuestions []struct {
			Question    string `json:"question"`
			SearchQuery string `json:"search_query"`
			Dimension   string `json:"dimension"`
		} `json:"sub_questions"`
	}

	if err := json.Unmarshal([]byte(jsonStr), &analysis); err != nil {
		logger.Warn("Learner", "问题分析结果解析失败: %v，使用原始查询", err)
		d.state.SubQuestions = []SubQuestion{
			{Question: query, SearchQuery: query, Source: "分析降级-原始查询"},
		}
		return nil
	}

	// 构建子问题列表
	d.state.SubQuestions = make([]SubQuestion, 0, len(analysis.SubQuestions))
	for i, sq := range analysis.SubQuestions {
		if i >= MaxSearchSubQuestions {
			break
		}
		d.state.SubQuestions = append(d.state.SubQuestions, SubQuestion{
			Question:    sq.Question,
			SearchQuery: sq.SearchQuery,
			Source:      sq.Dimension,
		})
	}

	// 确保至少有一个子问题
	if len(d.state.SubQuestions) == 0 {
		d.state.SubQuestions = []SubQuestion{
			{Question: query, SearchQuery: query, Source: "分析降级-原始查询"},
		}
	}

	logger.Info("Learner", "问题分析完成: 主题=%s, 子问题数=%d", analysis.Topic, len(d.state.SubQuestions))
	return nil
}

// ============================================================
// 阶段2: 并行搜索
// ============================================================

// parallelSearch 并行搜索所有子问题 + 记忆检索
func (d *DebateSystem) parallelSearch() error {
	d.state.CurrentPhase = PhaseSearch

	searchAvailable := d.search.IsAvailable()
	memoryAvailable := d.memory.IsAvailable()

	if !searchAvailable && !memoryAvailable {
		return fmt.Errorf("搜索和记忆均不可用")
	}

	var wg sync.WaitGroup

	// 并行搜索每个子问题
	for i := range d.state.SubQuestions {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			sq := &d.state.SubQuestions[idx]

			if !searchAvailable {
				sq.SearchResult = "网络搜索不可用"
				sq.Source = "无来源（搜索不可用）"
				return
			}

			result, err := d.search.SearchAndCompress(sq.SearchQuery, SearchResultMaxChars, d.llm)
			if err != nil {
				logger.Warn("Learner", "子问题[%d]搜索失败: %v", idx, err)
				sq.SearchResult = fmt.Sprintf("搜索失败: %v", err)
				sq.Source = "搜索失败"
				return
			}

			sq.SearchResult = result
			sq.Source = "网络搜索"
			logger.Info("Learner", "子问题[%d]搜索完成: %d 字符", idx, len([]rune(result)))
		}(i)
	}

	// 记忆检索
	wg.Add(1)
	go func() {
		defer wg.Done()
		if !memoryAvailable {
			d.state.MemoryResults = nil
			return
		}

		results, err := d.memory.Query(d.state.OriginalQuery, MemoryQueryTopK)
		if err != nil {
			logger.Warn("Learner", "记忆检索失败: %v", err)
			d.state.MemoryResults = nil
			return
		}

		d.state.MemoryResults = results
		logger.Info("Learner", "记忆检索完成: %d 条结果", len(results))
	}()

	wg.Wait()

	// 检查是否有任何有效数据
	hasSearchResult := false
	for _, sq := range d.state.SubQuestions {
		if sq.SearchResult != "" && sq.SearchResult != "网络搜索不可用" && !strings.HasPrefix(sq.SearchResult, "搜索失败") {
			hasSearchResult = true
			break
		}
	}

	if !hasSearchResult && len(d.state.MemoryResults) == 0 {
		return fmt.Errorf("未能检索到任何有效记忆")
	}

	return nil
}

// ============================================================
// 阶段3: 辩论循环
// ============================================================

// runDebate 运行辩论循环
// 每个子问题独立辩论，辩论结果摘要压缩后供后续使用
func (d *DebateSystem) runDebate() error {
	d.state.CurrentPhase = PhaseDebate

	// 如果没有搜索结果也没有记忆数据，跳过辩论
	hasData := len(d.state.MemoryResults) > 0
	for _, sq := range d.state.SubQuestions {
		if sq.SearchResult != "" && !strings.HasPrefix(sq.SearchResult, "搜索失败") && sq.SearchResult != "网络搜索不可用" {
			hasData = true
			break
		}
	}
	if !hasData {
		logger.Warn("Learner", "无有效数据，跳过辩论")
		return nil
	}

	// 对每个子问题进行独立辩论
	var allDebateSummaries []string

	for i, sq := range d.state.SubQuestions {
		logger.Info("Learner", "辩论子问题[%d]: %s", i, sq.Question)

		memForSubQ := d.filterMemoryForSubQuestion(sq)
		rounds, err := d.debateSubQuestion(sq, memForSubQ)
		if err != nil {
			logger.Warn("Learner", "子问题[%d]辩论失败: %v", i, err)
			continue
		}

		d.state.Rounds = append(d.state.Rounds, rounds...)

		// 生成该子问题辩论的综合摘要
		subSummary := d.summarizeSubQuestionDebate(sq, rounds)
		allDebateSummaries = append(allDebateSummaries, subSummary)
	}

	// 设置收敛状态
	d.state.Converged = len(d.state.Rounds) > 0

	return nil
}

// debateSubQuestion 对单个子问题进行辩论
func (d *DebateSystem) debateSubQuestion(subQ SubQuestion, memResults []MemoryMatch) ([]DebateRound, error) {
	var rounds []DebateRound
	var previousSummary string

	for roundNum := 1; roundNum <= MaxDebateRounds; roundNum++ {
		logger.Info("Learner", "辩论轮次 %d/%d", roundNum, MaxDebateRounds)

		round, err := d.executeDebateRound(roundNum, subQ, memResults, previousSummary)
		if err != nil {
			logger.Warn("Learner", "辩论轮次 %d 执行失败: %v", roundNum, err)
			break
		}

		rounds = append(rounds, *round)

		// 生成本轮摘要
		summary, err := d.summarizeRound(round)
		if err != nil {
			summary = fmt.Sprintf("轮次%d摘要生成失败", roundNum)
		}
		round.Summary = summary
		previousSummary = summary

		// 检查收敛
		if round.IsConverged {
			logger.Info("Learner", "辩论在第 %d 轮收敛", roundNum)
			break
		}
	}

	return rounds, nil
}

// executeDebateRound 执行一轮辩论（4个角色交替发言）
func (d *DebateSystem) executeDebateRound(roundNum int, subQ SubQuestion, memResults []MemoryMatch, previousSummary string) (*DebateRound, error) {
	round := &DebateRound{RoundNum: roundNum}

	// 构建辩论上下文
	debateContext := d.buildDebateContext(subQ, memResults, previousSummary, roundNum)

	// 网络派发言
	netProArg, err := d.debaterSpeak(RoleNetPro, debateContext, subQ)
	if err != nil {
		netProArg = fmt.Sprintf("网络派发言失败: %v", err)
	}
	round.NetProArg = netProArg

	// 记忆派发言
	memProArg, err := d.debaterSpeak(RoleMemPro, debateContext, subQ)
	if err != nil {
		memProArg = fmt.Sprintf("记忆派发言失败: %v", err)
	}
	round.MemProArg = memProArg

	// 质疑者发言
	debateContext += fmt.Sprintf("\n\n网络派论点：%s\n记忆派论点：%s", netProArg, memProArg)
	skepticArg, err := d.debaterSpeak(RoleSkeptic, debateContext, subQ)
	if err != nil {
		skepticArg = fmt.Sprintf("质疑者发言失败: %v", err)
	}
	round.SkepticArg = skepticArg

	// 裁决者发言
	debateContext += fmt.Sprintf("\n质疑者论点：%s", skepticArg)
	judgeVerdict, converged, err := d.judgeSpeak(debateContext, subQ)
	if err != nil {
		judgeVerdict = fmt.Sprintf("裁决者发言失败: %v", err)
		converged = roundNum >= MaxDebateRounds
	}
	round.JudgeVerdict = judgeVerdict
	round.IsConverged = converged

	return round, nil
}

// debaterSpeak 单个辩论角色发言
func (d *DebateSystem) debaterSpeak(role DebateRole, context string, subQ SubQuestion) (string, error) {
	var rolePrompt string
	switch role {
	case RoleNetPro:
		rolePrompt = "你是网络派，基于网络搜索结果提出论点。只使用搜索结果中的事实和数据，标注来源。控制在300字以内。"
	case RoleMemPro:
		rolePrompt = "你是记忆派，基于记忆库中的历史记录提出论点。只使用记忆数据，标注来源。如果记忆数据为空，请说明'记忆库中无相关记录'。控制在300字以内。"
	case RoleSkeptic:
		rolePrompt = "你是质疑者，专门挑战网络派和记忆派的论点。指出矛盾、来源不可靠、信息过时或逻辑漏洞。提出双方都未涉及的盲点。控制在300字以内。"
	default:
		rolePrompt = "你是辩论参与者。"
	}

	messages := []LLMMessage{
		{Role: "system", Content: d.promptDebate + "\n\n" + rolePrompt},
		{Role: "user", Content: fmt.Sprintf("当前辩论子问题：%s\n\n%s", subQ.Question, context)},
	}

	resp, err := d.llm.Chat(messages, BudgetDebate)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(resp.Content), nil
}

// judgeSpeak 裁决者发言，返回裁决内容和收敛判断
func (d *DebateSystem) judgeSpeak(context string, subQ SubQuestion) (string, bool, error) {
	rolePrompt := `你是裁决者，综合评估网络派和记忆派的论点，以及质疑者的质疑。

你必须：
1. 指出共识点和分歧点
2. 给出当前的综合判断
3. 明确判断"信息是否充分"

输出格式：
- 共识点：...
- 分歧点：...
- 盲点：...
- 信息充分度：充分/不充分
- 综合判断：...

控制在400字以内。`

	messages := []LLMMessage{
		{Role: "system", Content: d.promptDebate + "\n\n" + rolePrompt},
		{Role: "user", Content: fmt.Sprintf("当前辩论子问题：%s\n\n%s", subQ.Question, context)},
	}

	resp, err := d.llm.Chat(messages, BudgetDebate)
	if err != nil {
		return "", false, err
	}

	verdict := strings.TrimSpace(resp.Content)

	// 判断是否收敛：如果裁决者说"充分"则收敛
	converged := strings.Contains(verdict, "信息充分度：充分") || strings.Contains(verdict, "信息充分度:充分")

	return verdict, converged, nil
}

// buildDebateContext 构建辩论上下文
func (d *DebateSystem) buildDebateContext(subQ SubQuestion, memResults []MemoryMatch, previousSummary string, roundNum int) string {
	var parts []string

	// 搜索结果
	if subQ.SearchResult != "" {
		parts = append(parts, fmt.Sprintf("## 网络搜索结果\n%s", subQ.SearchResult))
	}

	// 记忆数据
	if len(memResults) > 0 {
		parts = append(parts, fmt.Sprintf("## 记忆库数据\n%s", FormatMemoryResults(memResults)))
	}

	// 前序辩论摘要
	if previousSummary != "" {
		parts = append(parts, fmt.Sprintf("## 前序辩论摘要\n%s", previousSummary))
	}

	return strings.Join(parts, "\n\n")
}

// filterMemoryForSubQuestion 过滤与子问题相关的记忆
func (d *DebateSystem) filterMemoryForSubQuestion(subQ SubQuestion) []MemoryMatch {
	if len(d.state.MemoryResults) == 0 {
		return nil
	}

	// 对每个子问题额外查询一次记忆，获取更精准的结果
	if d.memory.IsAvailable() {
		results, err := d.memory.Query(subQ.SearchQuery, 5)
		if err == nil && len(results) > 0 {
			return results
		}
	}

	// 降级：使用全局记忆结果
	return d.state.MemoryResults
}

// summarizeRound 生成单轮辩论摘要
func (d *DebateSystem) summarizeRound(round *DebateRound) (string, error) {
	prompt := fmt.Sprintf(`请将以下辩论轮次的内容压缩为不超过 %d 字的精炼摘要，保留关键论点和结论。

轮次 %d：
- 网络派：%s
- 记忆派：%s
- 质疑者：%s
- 裁决者：%s

摘要：`, DebateSummaryMaxChars, round.RoundNum,
		round.NetProArg, round.MemProArg, round.SkepticArg, round.JudgeVerdict)

	messages := []LLMMessage{
		{Role: "system", Content: "你是信息压缩助手，擅长在保留关键信息的前提下精炼文本。"},
		{Role: "user", Content: prompt},
	}

	resp, err := d.llm.Chat(messages, BudgetConvergence)
	if err != nil {
		// 降级：手动拼接摘要
		summary := fmt.Sprintf("轮次%d: 网络派提出论点，记忆派提出论点，裁决者判断信息%s",
			round.RoundNum, map[bool]string{true: "充分", false: "不充分"}[round.IsConverged])
		return truncateText(summary, DebateSummaryMaxChars), nil
	}

	summary := strings.TrimSpace(resp.Content)
	if len([]rune(summary)) > DebateSummaryMaxChars {
		summary = truncateText(summary, DebateSummaryMaxChars)
	}

	return summary, nil
}

// summarizeSubQuestionDebate 生成子问题辩论的综合摘要
func (d *DebateSystem) summarizeSubQuestionDebate(subQ SubQuestion, rounds []DebateRound) string {
	var roundSummaries []string
	for _, r := range rounds {
		if r.Summary != "" {
			roundSummaries = append(roundSummaries, r.Summary)
		}
	}

	return fmt.Sprintf("子问题「%s」辩论摘要：%s", subQ.Question, strings.Join(roundSummaries, " → "))
}

// ============================================================
// 阶段4: 综合报告
// ============================================================

// synthesizeReport 生成综合研究报告（先提纲、再细化）
func (d *DebateSystem) synthesizeReport() (string, error) {
	d.state.CurrentPhase = PhaseSynthesize

	// 第一步：生成报告提纲
	outline, err := d.generateOutline()
	if err != nil {
		logger.Warn("Learner", "提纲生成失败: %v，跳过提纲阶段", err)
		// 降级：直接生成报告
		return d.generateFullReport(nil)
	}

	logger.Info("Learner", "报告提纲: %s, %d 个要点", outline.Topic, len(outline.Sections))

	// 第二步：基于提纲生成完整报告
	report, err := d.generateFullReport(outline)
	if err != nil {
		return "", err
	}

	return report, nil
}

// generateOutline 生成报告提纲
func (d *DebateSystem) generateOutline() (*ReportOutline, error) {
	dataSummary := d.buildDataSummary()

	prompt := fmt.Sprintf(`%s

请基于以下研究数据生成报告提纲。

原始问题：%s

研究数据摘要：
%s

要求：
1. 明确研究主题
2. 给出核心结论方向
3. 列出3-5个提纲要点
4. 标记待解决的疑点
5. 以 JSON 格式输出：{"topic":"...","conclusion":"...","sections":["要点1","要点2","要点3"],"doubts":["疑点1","疑点2"]}`,
		d.promptReport, d.state.OriginalQuery, dataSummary)

	messages := []LLMMessage{
		{Role: "system", Content: d.promptReport},
		{Role: "user", Content: prompt},
	}

	resp, err := d.llm.Chat(messages, BudgetReportOutline)
	if err != nil {
		return nil, err
	}

	// 解析提纲 JSON
	jsonStr := extractJSON(strings.TrimSpace(resp.Content))

	var outline ReportOutline
	if err := json.Unmarshal([]byte(jsonStr), &outline); err != nil {
		return nil, fmt.Errorf("提纲解析失败: %w", err)
	}

	return &outline, nil
}

// generateFullReport 基于提纲生成完整报告
func (d *DebateSystem) generateFullReport(outline *ReportOutline) (string, error) {
	dataSummary := d.buildDataSummary()

	var outlineText string
	if outline != nil {
		outlineText = fmt.Sprintf("主题：%s\n结论方向：%s\n提纲要点：%s\n疑点：%s",
			outline.Topic, outline.Conclusion,
			strings.Join(outline.Sections, "、"),
			strings.Join(outline.Doubts, "、"))
	} else {
		outlineText = "（提纲生成失败，请直接根据数据生成报告）"
	}

	prompt := fmt.Sprintf(`%s

原始问题：%s

报告提纲：
%s

详细研究数据：
%s

请按照 [研究报告] 格式输出完整的研究报告。`,
		d.promptReport, d.state.OriginalQuery, outlineText, dataSummary)

	messages := []LLMMessage{
		{Role: "system", Content: d.promptReport},
		{Role: "user", Content: prompt},
	}

	resp, err := d.llm.Chat(messages, BudgetReportFull)
	if err != nil {
		return "", fmt.Errorf("报告生成失败: %w", err)
	}

	report := strings.TrimSpace(resp.Content)

	// 确保报告以 [研究报告] 开头
	if !strings.HasPrefix(report, "[研究报告]") {
		report = "[研究报告]\n\n" + report
	}

	return report, nil
}

// buildDataSummary 构建研究数据摘要（用于报告生成）
func (d *DebateSystem) buildDataSummary() string {
	var parts []string

	// 子问题搜索结果
	for i, sq := range d.state.SubQuestions {
		parts = append(parts, fmt.Sprintf("### 子问题%d：%s\n搜索结果：%s",
			i+1, sq.Question, truncateText(sq.SearchResult, SearchResultMaxChars)))
	}

	// 记忆数据
	if len(d.state.MemoryResults) > 0 {
		parts = append(parts, fmt.Sprintf("### 记忆库数据\n%s",
			FormatMemoryResults(d.state.MemoryResults)))
	}

	// 辩论摘要
	if len(d.state.Rounds) > 0 {
		var debateParts []string
		for _, r := range d.state.Rounds {
			if r.Summary != "" {
				debateParts = append(debateParts, r.Summary)
			}
		}
		if len(debateParts) > 0 {
			parts = append(parts, fmt.Sprintf("### 辩论过程摘要\n%s",
				strings.Join(debateParts, "\n")))
		}
	}

	return strings.Join(parts, "\n\n")
}

// ============================================================
// 记忆更新
// ============================================================

// updateMemory 基于研究结果更新记忆库
func (d *DebateSystem) updateMemory(report string) {
	if !d.memory.IsAvailable() {
		logger.Warn("Learner", "记忆库不可用，跳过记忆更新")
		return
	}

	// 将研究报告摘要存入记忆
	summary := truncateText(report, 500)
	if _, err := d.memory.Add(summary); err != nil {
		logger.Warn("Learner", "研究报告存入记忆失败: %v", err)
	} else {
		logger.Info("Learner", "研究报告已存入记忆")
	}

	// 检查是否有需要更新的旧记忆
	if len(d.state.MemoryResults) == 0 {
		return
	}

	// 收集高相似度的记忆条目
	var highSimMatches []MemoryMatch
	for _, match := range d.state.MemoryResults {
		if match.Similarity >= MemoryUpdateSimilarityThreshold {
			highSimMatches = append(highSimMatches, match)
		}
	}

	if len(highSimMatches) == 0 {
		return
	}

	// 使用 LLM 判断哪些记忆需要更新
	updates := d.judgeMemoryUpdates(report, highSimMatches)
	if len(updates) == 0 {
		return
	}

	// 执行批量更新
	if err := d.memory.BatchUpdate(updates); err != nil {
		logger.Warn("Learner", "记忆批量更新失败: %v", err)
	}
}

// judgeMemoryUpdates 使用 LLM 判断记忆更新
func (d *DebateSystem) judgeMemoryUpdates(report string, matches []MemoryMatch) []MemoryUpdate {
	var memInfo []string
	for _, m := range matches {
		memInfo = append(memInfo, fmt.Sprintf("ID: %s\n内容: %s\n相似度: %.2f", m.ID, m.Content, m.Similarity))
	}

	prompt := fmt.Sprintf(`%s

研究报告摘要：
%s

需要评估的现有记忆条目：
%s

请判断哪些记忆需要更新，以 JSON 数组格式输出更新指令。`, d.promptMemory, truncateText(report, 1000), strings.Join(memInfo, "\n\n"))

	messages := []LLMMessage{
		{Role: "system", Content: d.promptMemory},
		{Role: "user", Content: prompt},
	}

	resp, err := d.llm.Chat(messages, BudgetMemoryUpdate)
	if err != nil {
		logger.Warn("Learner", "记忆更新判断失败: %v", err)
		return nil
	}

	// 解析 JSON
	jsonStr := extractJSON(strings.TrimSpace(resp.Content))
	if jsonStr == "" || jsonStr == "[]" {
		return nil
	}

	var updates []MemoryUpdate
	if err := json.Unmarshal([]byte(jsonStr), &updates); err != nil {
		logger.Warn("Learner", "记忆更新指令解析失败: %v", err)
		return nil
	}

	return updates
}

// ============================================================
// 工具函数
// ============================================================

// getRuntimeContext 获取运行时上下文（时间 + 位置）
func (d *DebateSystem) getRuntimeContext() string {
	now := time.Now()
	timeStr := now.Format("2006-01-02 15:04:05")
	weekDay := [...]string{"星期日", "星期一", "星期二", "星期三", "星期四", "星期五", "星期六"}[now.Weekday()]

	// 位置信息暂时无法在 Go 层直接获取，由 TS 层注入或使用默认值
	return fmt.Sprintf("当前时间: %s %s", timeStr, weekDay)
}

// extractJSON 从文本中提取 JSON 内容
// 处理 markdown 代码块包裹等情况
func extractJSON(text string) string {
	text = strings.TrimSpace(text)

	// 去除 markdown 代码块包裹
	if strings.HasPrefix(text, "```json") {
		text = strings.TrimPrefix(text, "```json")
		text = strings.TrimSuffix(text, "```")
		text = strings.TrimSpace(text)
	} else if strings.HasPrefix(text, "```") {
		text = strings.TrimPrefix(text, "```")
		text = strings.TrimSuffix(text, "```")
		text = strings.TrimSpace(text)
	}

	// 尝试找到 JSON 数组或对象
	startIdx := -1
	endIdx := -1

	for i, ch := range text {
		if ch == '[' || ch == '{' {
			if startIdx == -1 {
				startIdx = i
			}
		}
		if ch == ']' || ch == '}' {
			endIdx = i
		}
	}

	if startIdx >= 0 && endIdx > startIdx {
		return text[startIdx : endIdx+1]
	}

	return text
}

// ============================================================
// 内置默认 Prompt 模板（当嵌入式文件加载失败时使用）
// ============================================================

const defaultAnalysisPrompt = `你是学习者的分析中枢，负责从用户的对话内容中提取真正的搜索/研究意图，并拆解为可搜索的子问题。

重要：你收到的不是单一"问题"，而是完整对话内容。你必须：
1. 识别最新意图：从对话中找到用户最新提出的搜索/研究需求
2. 忽略闲聊：跳过"你好"、"谢谢"等无搜索意图的对话内容
3. 提取核心查询：将搜索/研究需求提炼为清晰的研究主题

例如，对话内容为"你好呀\n搜索一下原神最新卡池信息"时，应提取"原神最新卡池信息"作为研究主题。

输出 JSON 格式：
{
  "topic": "问题核心主题",
  "complexity": "simple|moderate|complex",
  "sub_questions": [
    {"question": "子问题描述", "search_query": "搜索关键词", "dimension": "聚焦维度"}
  ]
}

拆解原则：互补、可搜索、保留专有名词、2-4个子问题。`

const defaultDebatePrompt = `你是学习者的辩论系统。四角色交替辩论：
- 网络派：基于搜索结果
- 记忆派：基于记忆数据
- 质疑者：挑战双方论点
- 裁决者：综合评估，判断信息充分度

每个角色发言控制在300字以内，基于证据，不编造。`

const defaultReportPrompt = `你是学习者的报告生成系统。输出 [研究报告] 格式：

[研究报告]

## 研究主题
## 研究结论
## 支持证据
## 疑点与未解决问题
## 研究方法说明

所有结论必须有证据支撑，标注来源（网络/记忆/辩论共识）。`

const defaultMemoryPrompt = `你是学习者的记忆管理助手。基于研究结果判断是否需要更新记忆库中的条目。

输出 JSON 数组格式（不需要更新则输出 []）：
[{"old_id": "旧条目ID", "new_content": "完善后的完整内容", "reason": "更新原因"}]

只更新确实需要修正或补充的条目。`
