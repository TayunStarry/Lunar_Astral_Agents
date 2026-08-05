package websearch

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"storage/module"
)

// ── 智能搜索学习器 ──

// SearchLearner 智能搜索学习器，编排 refine → memory → search → evaluate → deep → store 工作流
// 向量存储已迁移至 storage 模块统一管理，通过 module.MemoryDB 操作
type SearchLearner struct {
	memoryDB  *module.MemoryDB
	simple    *SimpleSearcher
	depth     *DepthSearcher
	llm       Provider
	knowledge *SearchKnowledge
	cfg       KnowledgeVectorConfig
	depthCfg  DepthConfig
	debugLog  func(format string, args ...interface{})
}

// NewSearchLearner 创建学习器
func NewSearchLearner(
	memoryDB *module.MemoryDB,
	simple *SimpleSearcher,
	depth *DepthSearcher,
	llm Provider,
	knowledge *SearchKnowledge,
	cfg KnowledgeVectorConfig,
	depthCfg DepthConfig,
	debugLog func(format string, args ...interface{}),
) *SearchLearner {
	return &SearchLearner{
		memoryDB:  memoryDB,
		simple:    simple,
		depth:     depth,
		llm:       llm,
		knowledge: knowledge,
		cfg:       cfg,
		depthCfg:  depthCfg,
		debugLog:  debugLog,
	}
}

// ── 工作流常量 ──

const (
	learnerTopK             = 10   // 向量检索 topK
	learnerMinSimilarity    = 0.60 // 知识匹配最低相似度
	learnerSupersedeSim     = 0.85 // 替代旧知识相似度阈值
	learnerSimpleMaxResults = 10   // 初步搜索最大结果数
	learnerDeepResultChars  = 8000 // 深度搜索结果最大字符数
	learnerMaxReportChars   = 8000 // 最终报告最大字符数
)

// 向量知识库集合名常量
const (
	collectionKnowledge  = "search_knowledge"
	collectionExperience = "search_experience"
)

// ── 主入口 ──

// LearnAndSearch 执行完整学习工作流
func (l *SearchLearner) LearnAndSearch(ctx context.Context, query string) (string, error) {
	startTime := time.Now()

	// 步骤 a: AI 完善查询
	refined := l.refineQuery(query)

	// 步骤 b: 查询向量知识库（storage 模块）
	knowledgeMem, experienceMem := l.queryMemory(ctx, refined.Refined)

	// 步骤 d: 初步网络搜索
	searchItems := l.simpleWebSearch(refined.SearchTerms, refined.Refined)

	// 步骤 e: AI 评估是否充足
	eval := l.evaluateAndDecide(refined, knowledgeMem, experienceMem, searchItems)

	// 步骤 g/h: 深度搜索循环（使用 DepthSearcher 的完整功能：查询拆解、并行子问题、内容抓取、SPA检测、浏览器渲染、域名发现）
	var searchRounds []searchRound
	if eval.NeedDeepSearch {
		searchRounds = l.deepSearchLoop(ctx, refined, eval)
	}

	// 步骤 i: 生成报告 + 存储记忆
	report := l.finalize(ctx, refined, knowledgeMem, experienceMem, searchItems, searchRounds, eval)

	if l.debugLog != nil {
		l.debugLog("[智能搜索] 工作流完成 query=%q 耗时=%s 报告=%d字符",
			query, time.Since(startTime).Round(time.Millisecond), len([]rune(report)))
	}

	return report, nil
}

// ── 步骤 a: AI 完善查询 ──

// refinedQuery 完善后的查询
type refinedQuery struct {
	Original    string   `json:"original"`
	Refined     string   `json:"refined"`
	KeyPoints   []string `json:"key_points"`
	SearchTerms []string `json:"search_terms"`
}

func (l *SearchLearner) refineQuery(rawQuery string) *refinedQuery {
	prompt := `你是一个搜索查询优化专家。你的任务是将用户的原始查询转化为一组高效的搜索引擎查询词，最大程度提升搜索结果的相关性和命中率。

## 规则

### 1. 查询长度分析
- 如果原始查询少于 5 个词 → 补充上下文和同义词，扩展查询范围
- 如果原始查询超过 10 个词 → 精简为最核心的实体和关键词，去掉修饰词
- 其他情况 → 保持核心实体不变，适当补充

### 2. 实体保留（最重要）
- 专有名词、游戏名、模组名、人名、产品名、地名等专有实体必须完整保留，绝对不能拆分

### 3. 上下文保留
- 每个搜索词必须包含足够的上下文（如游戏名、产品名、品牌名等），确保搜索引擎能正确理解专有名词的所属领域
- 例如：搜索某个游戏的模组时，每个搜索词都必须包含游戏名，否则搜索引擎可能误认为是其他领域的同名概念

### 4. 搜索词组成
- 每个搜索词应该是 2-4 个关键词的合理组合，用空格分隔
- 每个搜索词从不同角度切入问题（如：一个侧重"是什么"，一个侧重"怎么用"，一个侧重"最新消息"）
- 搜索词之间要有明显的差异，避免重复

### 5. 输出格式
只输出 JSON，不要包含任何其他文字：
{
  "refined": "完善后的完整查询描述（补充必要的上下文和背景，使查询意图更清晰）",
  "key_points": ["关键要点1", "关键要点2"],
  "search_terms": ["搜索词1", "搜索词2", "搜索词3"]
}`

	messages := []ChatMessage{
		{Role: "system", Content: prompt},
		{Role: "user", Content: "请完善以下查询：" + rawQuery},
	}

	resp, err := l.llm.Chat(messages)
	if err != nil {
		if l.debugLog != nil {
			l.debugLog("[智能搜索] 查询完善失败: %v，使用原始查询", err)
		}
		return &refinedQuery{Original: rawQuery, Refined: rawQuery, KeyPoints: []string{rawQuery}, SearchTerms: []string{rawQuery}}
	}

	var r refinedQuery
	if err := json.Unmarshal([]byte(extractJSON(resp)), &r); err != nil {
		if l.debugLog != nil {
			l.debugLog("[智能搜索] 查询完善解析失败: %v，使用原始查询", err)
		}
		return &refinedQuery{Original: rawQuery, Refined: rawQuery, KeyPoints: []string{rawQuery}, SearchTerms: []string{rawQuery}}
	}

	r.Original = rawQuery
	if r.Refined == "" {
		r.Refined = rawQuery
	}
	if len(r.SearchTerms) == 0 {
		r.SearchTerms = []string{rawQuery}
	}

	if l.debugLog != nil {
		l.debugLog("[智能搜索] 查询完善: refined=%q search_terms=%v", r.Refined, r.SearchTerms)
	}
	return &r
}

// ── 步骤 b: 查询向量知识库（storage 模块） ──

func (l *SearchLearner) queryMemory(ctx context.Context, query string) ([]module.MemoryQueryResult, []module.MemoryQueryResult) {
	if l.memoryDB == nil || !l.memoryDB.IsMemoryInitialized() {
		return nil, nil
	}

	knowledge, _ := l.memoryDB.MemoryQueryMessagesWithContent(ctx, collectionKnowledge, query, learnerTopK)
	experience, _ := l.memoryDB.MemoryQueryMessagesWithContent(ctx, collectionExperience, query, learnerTopK)

	// 过滤低相似度结果
	knowledge = filterBySimilarity(knowledge, learnerMinSimilarity)
	experience = filterBySimilarity(experience, learnerMinSimilarity)

	if l.debugLog != nil {
		l.debugLog("[智能搜索] 记忆查询: 知识=%d条 经验=%d条", len(knowledge), len(experience))
	}
	return knowledge, experience
}

func filterBySimilarity(matches []module.MemoryQueryResult, threshold float64) []module.MemoryQueryResult {
	filtered := make([]module.MemoryQueryResult, 0, len(matches))
	for _, m := range matches {
		if float64(m.Similarity) >= threshold {
			filtered = append(filtered, m)
		}
	}
	return filtered
}

// ── 步骤 d: 初步网络搜索 ──

type searchItemPreview struct {
	Title   string
	URL     string
	Snippet string
}

func (l *SearchLearner) simpleWebSearch(searchTerms []string, fallbackQuery string) []searchItemPreview {
	terms := searchTerms
	if len(terms) == 0 {
		terms = []string{fallbackQuery}
	}
	if len(terms) > 4 {
		terms = terms[:4]
	}

	seen := make(map[string]bool)
	var all []searchItemPreview

	for _, term := range terms {
		results, err := l.simple.SearchRaw(term)
		if err != nil {
			if l.debugLog != nil {
				l.debugLog("[智能搜索] 初步搜索词=%q 失败: %v", term, err)
			}
			continue
		}
		for _, r := range results {
			url := strings.TrimRight(r.URL, "/")
			if seen[url] {
				continue
			}
			seen[url] = true
			all = append(all, searchItemPreview{Title: r.Title, URL: r.URL, Snippet: r.Snippet})
		}
	}

	limit := learnerSimpleMaxResults * 2
	if len(all) > limit {
		all = all[:limit]
	}

	if l.debugLog != nil {
		l.debugLog("[智能搜索] 初步搜索完成: %d条结果", len(all))
	}
	return all
}

// ── 步骤 e: AI 评估 ──

// evaluationResult 评估结果
type evaluationResult struct {
	Sufficient      bool   `json:"sufficient"`
	Summary         string `json:"summary,omitempty"`
	NeedDeepSearch  bool   `json:"need_deep_search"`
	DeepSearchQuery string `json:"deep_search_query,omitempty"`
	Reasoning       string `json:"reasoning"`
}

func (l *SearchLearner) evaluateAndDecide(
	refined *refinedQuery,
	knowledgeMem, experienceMem []module.MemoryQueryResult,
	searchItems []searchItemPreview,
) *evaluationResult {
	// 构建 prompt
	var sb strings.Builder
	sb.WriteString("你是一个信息评估助手。根据以下信息判断是否足以回答用户问题。\n\n")
	sb.WriteString("输出 JSON 格式：\n")
	sb.WriteString(`{"sufficient": true/false, "summary": "阶段性摘要", "need_deep_search": true/false, "deep_search_query": "深度搜索词", "reasoning": "评估理由"}`)
	sb.WriteString("\n")
	sb.WriteString("【核心原则】\n")
	sb.WriteString("- 宁可信不足，不可信过剩。**不确定时务必设置 need_deep_search: true**，不要冒险判断为充足\n")
	sb.WriteString("- 有相关标题不等于有足够信息，标题可能只是诱饵，关键是摘要中是否包含具体、可验证的事实\n")
	sb.WriteString("- 搜索结果数量太少（<3条有效结果）时，信息量通常不足，应进入深度搜索\n")
	sb.WriteString("\n")
	sb.WriteString("【判断标准】\n")
	sb.WriteString("- 设置 sufficient: true 的条件（必须同时满足）：\n")
	sb.WriteString("  1) 搜索结果摘要中包含**可直接回答用户问题的完整具体事实信息**（如明确的名称、时间、数据、定义等）\n")
	sb.WriteString("  2) 信息覆盖了问题的**核心方面**，而非仅部分边角\n")
	sb.WriteString("  3) 至少有多个可靠来源（非单一论坛、百科条目）\n")
	sb.WriteString("- 以下情况必须设置 need_deep_search: true：\n")
	sb.WriteString("  1) 搜索结果只有标题没有具体内容，或摘要信息太笼统/模糊\n")
	sb.WriteString("  2) 搜索结果只涉及问题的部分方面，需要更多补充信息\n")
	sb.WriteString("  3) 搜索结果主要来自百科、词典、论坛等非权威来源\n")
	sb.WriteString("  4) 搜索结果数量太少（<3条），信息量不足以做出判断\n")
	sb.WriteString("\n\n--- 用户查询 ---\n")
	sb.WriteString(refined.Refined)

	if len(knowledgeMem) > 0 {
		sb.WriteString("\n\n--- 知识记忆 ---\n")
		for i, m := range knowledgeMem {
			sb.WriteString(fmt.Sprintf("[%d] 相关度:%.0f%% %s\n", i+1, m.Similarity*100, truncateStr(m.Content, 300)))
		}
	}
	if len(experienceMem) > 0 {
		sb.WriteString("\n\n--- 经验记忆 ---\n")
		for i, m := range experienceMem {
			sb.WriteString(fmt.Sprintf("[%d] 相关度:%.0f%% %s\n", i+1, m.Similarity*100, truncateStr(m.Content, 300)))
		}
	}
	if len(searchItems) > 0 {
		sb.WriteString("\n\n--- 网络搜索摘要 ---\n")
		for i, item := range searchItems {
			sb.WriteString(fmt.Sprintf("[%d] %s\n    %s\n", i+1, item.Title, truncateStr(item.Snippet, 200)))
		}
	} else {
		sb.WriteString("\n\n--- 网络搜索摘要 ---\n（无结果）\n")
	}

	messages := []ChatMessage{
		{Role: "system", Content: sb.String()},
		{Role: "user", Content: "请评估信息是否足以回答用户问题。"},
	}

	resp, err := l.llm.Chat(messages)
	if err != nil {
		if l.debugLog != nil {
			l.debugLog("[智能搜索] 评估失败: %v，降级为深度搜索", err)
		}
		return &evaluationResult{NeedDeepSearch: true, DeepSearchQuery: refined.Refined, Reasoning: "评估失败，降级"}
	}

	var eval evaluationResult
	if err := json.Unmarshal([]byte(extractJSON(resp)), &eval); err != nil {
		if l.debugLog != nil {
			l.debugLog("[智能搜索] 评估解析失败: %v，降级为深度搜索", err)
		}
		return &evaluationResult{NeedDeepSearch: true, DeepSearchQuery: refined.Refined, Reasoning: "解析失败，降级"}
	}

	if l.debugLog != nil {
		l.debugLog("[智能搜索] 评估完成: sufficient=%v need_deep=%v reasoning=%s", eval.Sufficient, eval.NeedDeepSearch, eval.Reasoning)
	}
	return &eval
}

// ── 步骤 g/h: 深度搜索循环 ──

type searchRound struct {
	RoundNum   int
	Query      string
	Result     string
	Evaluation string
	Sufficient bool
}

type searchEvaluation struct {
	Sufficient         bool   `json:"sufficient"`
	Summary            string `json:"summary,omitempty"`
	SupplementaryQuery string `json:"supplementary_query,omitempty"`
	Reasoning          string `json:"reasoning"`
}

func (l *SearchLearner) deepSearchLoop(ctx context.Context, refined *refinedQuery, eval *evaluationResult) []searchRound {
	maxRounds := l.depthCfg.MaxGapRounds
	if maxRounds <= 0 {
		maxRounds = 5
	}

	// 构建搜索词队列
	searchQueue := make([]string, 0, len(refined.SearchTerms)+1)
	searchQueue = append(searchQueue, refined.SearchTerms...)
	if eval.DeepSearchQuery != "" && eval.DeepSearchQuery != refined.Refined {
		searchQueue = append(searchQueue, eval.DeepSearchQuery)
	}
	if len(searchQueue) == 0 {
		searchQueue = []string{refined.Refined}
	}

	var rounds []searchRound
	var previousQueries []string
	var allPreviousSummaries string
	queueIdx := 0

	for roundNum := 1; roundNum <= maxRounds; roundNum++ {
		var searchQuery string
		for queueIdx < len(searchQueue) {
			q := searchQueue[queueIdx]
			queueIdx++
			if !isDuplicateQuery(q, previousQueries) {
				searchQuery = q
				break
			}
		}
		if searchQuery == "" {
			if l.debugLog != nil {
				l.debugLog("[智能搜索] 深度搜索第%d轮无有效搜索词，终止", roundNum)
			}
			break
		}

		previousQueries = append(previousQueries, searchQuery)

		if l.debugLog != nil {
			l.debugLog("[智能搜索] 深度搜索轮次 %d/%d: %s", roundNum, maxRounds, searchQuery)
		}

		// 深度搜索（使用 DepthSearcher 的完整功能：查询拆解、并行子问题、内容抓取、SPA检测、浏览器渲染、域名发现）
		searchResult, _, err := l.depth.SearchWithResults(ctx, searchQuery)
		if err != nil {
			if l.debugLog != nil {
				l.debugLog("[智能搜索] 深度搜索第%d轮失败: %v", roundNum, err)
			}
			searchResult = fmt.Sprintf("搜索失败: %v", err)
		}

		compressed := truncateStr(searchResult, learnerDeepResultChars)

		// AI 评估本轮结果
		roundEval := l.evaluateSearchRound(refined.Refined, compressed, roundNum, allPreviousSummaries, previousQueries)
		allPreviousSummaries += fmt.Sprintf("\n轮次%d(%s): %s", roundNum, searchQuery, roundEval.Summary)

		rounds = append(rounds, searchRound{
			RoundNum:   roundNum,
			Query:      searchQuery,
			Result:     compressed,
			Evaluation: roundEval.Reasoning,
			Sufficient: roundEval.Sufficient,
		})

		if roundEval.Sufficient {
			if l.debugLog != nil {
				l.debugLog("[智能搜索] 深度搜索第%d轮评估充足，结束循环", roundNum)
			}
			break
		}

		// 补充搜索词
		if roundEval.SupplementaryQuery != "" && !isDuplicateQuery(roundEval.SupplementaryQuery, previousQueries) {
			searchQueue = append(searchQueue, roundEval.SupplementaryQuery)
		}
	}

	return rounds
}

func (l *SearchLearner) evaluateSearchRound(refinedQuery, searchResult string, roundNum int, allPrevious string, previousQueries []string) searchEvaluation {
	var sb strings.Builder
	sb.WriteString("你是一个搜索质量评估助手。评估当前搜索结果是否足以回答用户问题。\n\n")
	sb.WriteString("输出 JSON 格式：\n")
	sb.WriteString(`{"sufficient": true/false, "summary": "本轮搜索结果摘要", "supplementary_query": "补充搜索词（不充足时提供，必须是全新角度）", "reasoning": "评估理由"}`)
	sb.WriteString("\n")
	sb.WriteString("【核心原则】\n")
	sb.WriteString("- 宁可信不足，不可信过剩。**不确定时优先补充搜索**，不要冒险判断为充足\n")
	sb.WriteString("- 前序搜索摘要已包含在此上下文中，请综合判断所有轮次的信息，不要仅看当前轮次\n")
	sb.WriteString("\n")
	sb.WriteString("【判断标准】\n")
	sb.WriteString("- 设置 sufficient: true 的条件：当前及前序搜索结果中包含了**可直接回答用户问题的完整具体信息**，且覆盖了问题核心方面\n")
	sb.WriteString("- 如果信息不足或只覆盖了部分方面，请提供 supplementary_query 补充搜索，并说明具体缺失什么信息\n")
	sb.WriteString("- 不要重复已搜索过的内容\n")
	sb.WriteString(fmt.Sprintf("- 这是第%d轮搜索，注意：前序搜索结果已包含在前面的摘要中，请综合判断\n", roundNum))
	sb.WriteString("\n\n--- 用户查询 ---\n")
	sb.WriteString(refinedQuery)
	sb.WriteString(fmt.Sprintf("\n\n--- 第%d轮搜索结果 ---\n", roundNum))
	sb.WriteString(searchResult)

	if allPrevious != "" {
		sb.WriteString("\n\n--- 前序搜索摘要 ---\n")
		sb.WriteString(allPrevious)
	}
	if len(previousQueries) > 0 {
		sb.WriteString("\n\n--- 已搜索词（禁止重复）---\n")
		sb.WriteString(strings.Join(previousQueries, "、"))
	}

	messages := []ChatMessage{
		{Role: "system", Content: sb.String()},
		{Role: "user", Content: "请评估搜索结果。"},
	}

	resp, err := l.llm.Chat(messages)
	if err != nil {
		return searchEvaluation{Sufficient: false, Reasoning: "评估失败", Summary: truncateStr(searchResult, 200)}
	}

	var eval searchEvaluation
	if err := json.Unmarshal([]byte(extractJSON(resp)), &eval); err != nil {
		return searchEvaluation{Sufficient: false, Reasoning: "评估解析失败", Summary: truncateStr(searchResult, 200)}
	}
	if l.debugLog != nil {
		l.debugLog("[智能搜索] 轮次评估: round=%d sufficient=%v reasoning=%s", roundNum, eval.Sufficient, eval.Reasoning)
	}
	return eval
}

// ── 步骤 i: 最终处理 ──

func (l *SearchLearner) finalize(
	ctx context.Context,
	refined *refinedQuery,
	knowledgeMem, experienceMem []module.MemoryQueryResult,
	searchItems []searchItemPreview,
	searchRounds []searchRound,
	eval *evaluationResult,
) string {
	// 生成报告
	report := l.generateReport(refined, knowledgeMem, searchRounds, searchItems, eval)

	// 使用 LLM 提取结构化知识条目（失败时回退到简单存储）
	knowledgeItems := l.extractKnowledgeWithLLM(refined, report, searchRounds)
	if len(knowledgeItems) > 0 {
		// 批量存储知识条目
		storedCount := 0
		for _, item := range knowledgeItems {
			if l.memoryDB != nil && l.memoryDB.IsMemoryInitialized() {
				if err := l.memoryDB.MemoryAddMessageSilent(ctx, collectionKnowledge, "knowledge", item); err == nil {
					storedCount++
				}
			}
		}
		if l.debugLog != nil {
			l.debugLog("[智能搜索] 知识记忆存储: LLM提取%d条，成功存储%d条", len(knowledgeItems), storedCount)
		}
	} else {
		// 回退：直接存报告摘要
		l.storeKnowledgeMemory(ctx, refined.Refined, report)
	}

	// 使用 LLM 生成经验条目（失败时回退到模板生成）
	experienceItem := l.generateExperienceWithLLM(refined, report, searchRounds)
	if experienceItem != "" {
		if l.memoryDB != nil && l.memoryDB.IsMemoryInitialized() {
			if err := l.memoryDB.MemoryAddMessageSilent(ctx, collectionExperience, "experience", experienceItem); err != nil && l.debugLog != nil {
				l.debugLog("[智能搜索] 经验记忆存储失败: %v", err)
			}
		}
	} else {
		// 回退：模板生成
		l.storeExperienceMemory(ctx, refined, searchRounds)
	}

	// 检查并替代高相似度的旧知识记忆
	l.supersedeOldKnowledge(ctx, report, knowledgeMem)

	return report
}

// ── LLM 引导的知识条目提取 ──

// buildMemoryUpdatePrompt 构建记忆更新 prompt（参考 analog 项目的 learnerMemory.md）
func (l *SearchLearner) buildMemoryUpdatePrompt(
	originalQuery, refinedQuery, finalReport string,
	searchRounds []searchRound,
	knowledgeCount int,
) string {
	var sb strings.Builder
	sb.WriteString("你是一个学习者的记忆管理助手，负责基于本次研究结果生成记忆更新指令。\n\n")
	sb.WriteString("## 需要处理的两种记忆\n\n")
	sb.WriteString("### 1. 知识记忆（knowledge_items）\n")
	sb.WriteString("从网络搜索中获取的新事实、数据、信息。每条知识记忆应：\n")
	sb.WriteString("- 包含具体的、可验证的事实信息\n")
	sb.WriteString("- 标注信息来源（网络搜索）\n")
	sb.WriteString("- 内容简洁，聚焦单个知识点\n")
	sb.WriteString("- 至少 10 个字\n\n")
	sb.WriteString("### 2. 经验记忆（experience_item）\n")
	sb.WriteString("本次请求的处理策略，包括：\n")
	sb.WriteString("- 查询类型（简单查询/复杂查询）\n")
	sb.WriteString("- 采用的搜索策略（直接回答/深度搜索 N 轮）\n")
	sb.WriteString("- 搜索轮次和每轮的效果\n")
	sb.WriteString("- 信息充足度评估\n")
	sb.WriteString("- 对未来类似请求的指导建议\n\n")
	sb.WriteString("## 输出格式\n\n")
	sb.WriteString("你必须以 JSON 格式输出，不要输出其他内容：\n")
	sb.WriteString("```json\n")
	sb.WriteString("{\n")
	sb.WriteString("  \"knowledge_items\": [\n")
	sb.WriteString("    {\"content\": \"知识摘要内容1\"},\n")
	sb.WriteString("    {\"content\": \"知识摘要内容2\"}\n")
	sb.WriteString("  ],\n")
	sb.WriteString("  \"experience_item\": \"本次请求处理策略描述，包括：查询类型、采用的搜索策略、搜索轮次、信息充足度评估\"\n")
	sb.WriteString("}\n")
	sb.WriteString("```\n\n")
	sb.WriteString("## 更新内容编写规则\n\n")
	sb.WriteString("1. **完整性**：knowledge_items 应包含从搜索结果中提取的关键事实\n")
	sb.WriteString("2. **准确性**：基于搜索结果的事实，不编造\n")
	sb.WriteString("3. **简洁性**：每条 knowledge_item 去除冗余，保留关键信息，不超过 500 字\n")
	sb.WriteString("4. **保守原则**：当不确定信息是否准确时，倾向于不添加\n")
	sb.WriteString("5. **策略记录**：experience_item 应包含足够的信息以便未来类似请求能复用策略\n\n")
	sb.WriteString("## 约束\n\n")
	sb.WriteString("- knowledge_items 可为空数组（当没有值得记录的新知识时）\n")
	sb.WriteString("- experience_item 必须包含策略描述，不能为空\n")
	sb.WriteString("- 只输出 JSON，不要输出其他内容\n")
	sb.WriteString("- 每条 knowledge_item.content 至少 10 个字\n\n")
	sb.WriteString("---\n\n")
	sb.WriteString(fmt.Sprintf("原始查询：%s\n", originalQuery))
	sb.WriteString(fmt.Sprintf("完善后查询：%s\n", refinedQuery))
	sb.WriteString(fmt.Sprintf("\n最终研究报告：\n%s\n\n", truncateStr(finalReport, 2000)))
	sb.WriteString("搜索策略摘要：\n")
	if len(searchRounds) == 0 {
		sb.WriteString("查询类型：简单查询\n")
		sb.WriteString("搜索策略：无需深度搜索，信息已充足\n")
	} else {
		sb.WriteString(fmt.Sprintf("查询类型：复杂查询\n"))
		sb.WriteString(fmt.Sprintf("搜索策略：深度搜索 %d 轮\n", len(searchRounds)))
		for _, r := range searchRounds {
			sb.WriteString(fmt.Sprintf("  轮次%d: 查询=%s, 充足=%v\n", r.RoundNum, r.Query, r.Sufficient))
		}
		lastRound := searchRounds[len(searchRounds)-1]
		if lastRound.Sufficient {
			sb.WriteString(fmt.Sprintf("效果：%d轮搜索后信息充足\n", len(searchRounds)))
		} else {
			sb.WriteString(fmt.Sprintf("效果：%d轮搜索后信息仍不充足\n", len(searchRounds)))
		}
	}
	sb.WriteString(fmt.Sprintf("\n现有知识记忆条目数：%d\n", knowledgeCount))

	return sb.String()
}

// extractKnowledgeWithLLM 使用 LLM 生成结构化知识条目，失败时回退到简单提取
func (l *SearchLearner) extractKnowledgeWithLLM(
	refined *refinedQuery,
	finalReport string,
	searchRounds []searchRound,
) []string {
	if l.llm == nil {
		return nil
	}

	prompt := l.buildMemoryUpdatePrompt(refined.Original, refined.Refined, finalReport, searchRounds, 0)
	messages := []ChatMessage{
		{Role: "system", Content: prompt},
		{Role: "user", Content: "请基于研究结果生成记忆更新指令 JSON。"},
	}

	resp, err := l.llm.Chat(messages)
	if err != nil {
		if l.debugLog != nil {
			l.debugLog("[智能搜索] LLM 知识提取失败: %v，回退到简单提取", err)
		}
		return nil
	}

	jsonStr := extractJSON(strings.TrimSpace(resp))
	var result struct {
		KnowledgeItems []struct {
			Content string `json:"content"`
		} `json:"knowledge_items"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		if l.debugLog != nil {
			l.debugLog("[智能搜索] 解析知识条目失败: %v，回退到简单提取", err)
		}
		return nil
	}

	var items []string
	for _, item := range result.KnowledgeItems {
		content := strings.TrimSpace(item.Content)
		if len([]rune(content)) >= 10 {
			items = append(items, content)
		}
	}

	if len(items) == 0 {
		if l.debugLog != nil {
			l.debugLog("[智能搜索] LLM 未生成有效知识条目，回退到简单提取")
		}
		return nil
	}

	if l.debugLog != nil {
		l.debugLog("[智能搜索] LLM 生成知识条目: %d 条", len(items))
	}
	return items
}

// generateExperienceWithLLM 使用 LLM 生成经验条目，失败时回退到模板生成
func (l *SearchLearner) generateExperienceWithLLM(
	refined *refinedQuery,
	finalReport string,
	searchRounds []searchRound,
) string {
	if l.llm == nil {
		return ""
	}

	prompt := l.buildMemoryUpdatePrompt(refined.Original, refined.Refined, finalReport, searchRounds, 0)
	messages := []ChatMessage{
		{Role: "system", Content: prompt},
		{Role: "user", Content: "请生成经验记忆条目 JSON。"},
	}

	resp, err := l.llm.Chat(messages)
	if err != nil {
		if l.debugLog != nil {
			l.debugLog("[智能搜索] LLM 经验生成失败: %v，回退到模板生成", err)
		}
		return ""
	}

	jsonStr := extractJSON(strings.TrimSpace(resp))
	// 兼容两种格式：字符串或数组
	var strResult struct {
		ExperienceItem string `json:"experience_item"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &strResult); err == nil && strings.TrimSpace(strResult.ExperienceItem) != "" {
		return strResult.ExperienceItem
	}

	var arrResult struct {
		ExperienceItem []string `json:"experience_item"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &arrResult); err == nil && len(arrResult.ExperienceItem) > 0 {
		return strings.Join(arrResult.ExperienceItem, "\n")
	}

	if l.debugLog != nil {
		l.debugLog("[智能搜索] LLM 未生成经验条目，回退到模板生成")
	}
	return ""
}

// supersedeOldKnowledge 查找并替代高相似度的旧知识记忆（通过 storage 模块查询）
func (l *SearchLearner) supersedeOldKnowledge(ctx context.Context, newReport string, existingMem []module.MemoryQueryResult) {
	if l.memoryDB == nil || !l.memoryDB.IsMemoryInitialized() || len(existingMem) == 0 {
		return
	}

	// 通过 storage 模块查询与新报告高相似度的旧知识条目
	superseded, err := l.memoryDB.MemoryQueryMessagesWithContent(ctx, collectionKnowledge, newReport, 10)
	if err != nil {
		if l.debugLog != nil {
			l.debugLog("[智能搜索] 查找旧知识记忆失败: %v", err)
		}
		return
	}

	for _, old := range superseded {
		if old.ID == "" || float64(old.Similarity) < learnerSupersedeSim {
			continue
		}
		// 使用 storage 模块的 DeleteMessage 删除旧条目
		if err := l.memoryDB.MemoryDeleteMessage(ctx, collectionKnowledge, old.ID); err != nil {
			if l.debugLog != nil {
				l.debugLog("[智能搜索] 删除旧知识记忆失败 id=%s: %v", old.ID, err)
			}
		} else {
			if l.debugLog != nil {
				l.debugLog("[智能搜索] 已替代旧知识记忆: id=%s, 相似度=%.1f%%", old.ID, old.Similarity*100)
			}
		}
	}
}

func (l *SearchLearner) generateReport(
	refined *refinedQuery,
	knowledgeMem []module.MemoryQueryResult,
	searchRounds []searchRound,
	searchItems []searchItemPreview,
	eval *evaluationResult,
) string {
	// 如果评估已充足且没有深度搜索，直接用评估摘要
	if eval.Sufficient && !eval.NeedDeepSearch && eval.Summary != "" {
		return eval.Summary
	}

	var sb strings.Builder
	sb.WriteString("你是一个研究报告生成助手。根据所有收集的信息，生成一份完整的研究报告。\n\n")
	sb.WriteString("要求：\n")
	sb.WriteString("- 用中文回答\n")
	sb.WriteString("- 如果信息不足，诚实说明，并给出已有信息的摘要\n")
	sb.WriteString("- 信息充足时，用结构化格式（要点、分类）呈现\n")
	sb.WriteString(fmt.Sprintf("- 不超过%d字\n", learnerMaxReportChars/2))

	sb.WriteString("\n\n--- 用户查询 ---\n")
	sb.WriteString(refined.Refined)

	if len(knowledgeMem) > 0 {
		sb.WriteString("\n\n--- 知识记忆 ---\n")
		for i, m := range knowledgeMem {
			sb.WriteString(fmt.Sprintf("[%d] %s\n", i+1, truncateStr(m.Content, 500)))
		}
	}

	if len(searchItems) > 0 {
		sb.WriteString("\n\n--- 初步搜索摘要 ---\n")
		for i, item := range searchItems {
			sb.WriteString(fmt.Sprintf("[%d] %s\n    %s\n", i+1, item.Title, truncateStr(item.Snippet, 200)))
		}
	}

	if len(searchRounds) > 0 {
		sb.WriteString("\n\n--- 深度搜索结果 ---\n")
		for _, round := range searchRounds {
			sb.WriteString(fmt.Sprintf("\n### 轮次%d: %s\n%s\n", round.RoundNum, round.Query, truncateStr(round.Result, 1500)))
		}
	}

	messages := []ChatMessage{
		{Role: "system", Content: sb.String()},
		{Role: "user", Content: "请生成研究报告。"},
	}

	resp, err := l.llm.Chat(messages)
	if err != nil {
		// 降级：返回搜索摘要
		return l.buildFallbackReport(searchItems, searchRounds)
	}

	report := strings.TrimSpace(resp)
	if len([]rune(report)) < 10 {
		return l.buildFallbackReport(searchItems, searchRounds)
	}

	return truncateStr(report, learnerMaxReportChars)
}

func (l *SearchLearner) buildFallbackReport(searchItems []searchItemPreview, searchRounds []searchRound) string {
	var sb strings.Builder
	sb.WriteString("搜索完成，以下是找到的信息摘要：\n\n")

	if len(searchItems) > 0 {
		sb.WriteString("### 初步搜索结果\n")
		for i, item := range searchItems {
			if i >= 5 {
				break
			}
			sb.WriteString(fmt.Sprintf("- **%s**\n  %s\n  %s\n", item.Title, truncateStr(item.Snippet, 200), item.URL))
		}
	}

	if len(searchRounds) > 0 {
		sb.WriteString("\n### 深度搜索摘要\n")
		for _, round := range searchRounds {
			sb.WriteString(fmt.Sprintf("- 轮次%d(%s): %s\n", round.RoundNum, round.Query, truncateStr(round.Evaluation, 200)))
		}
	}

	if len(searchItems) == 0 && len(searchRounds) == 0 {
		sb.WriteString("抱歉，没有找到相关信息。")
	}

	return sb.String()
}

// ── 记忆存储（通过 storage 模块） ──

func (l *SearchLearner) storeKnowledgeMemory(ctx context.Context, query, report string) {
	if l.memoryDB == nil || !l.memoryDB.IsMemoryInitialized() {
		return
	}
	// 只存储报告的核心结论部分（前800字符），避免整篇报告做向量化导致精度损失
	// 格式：查询 + 核心结论
	reportRunes := []rune(report)
	summaryLen := 800
	if len(reportRunes) < summaryLen {
		summaryLen = len(reportRunes)
	}
	content := fmt.Sprintf("查询：%s\n结论：%s", query, string(reportRunes[:summaryLen]))
	if err := l.memoryDB.MemoryAddMessageSilent(ctx, collectionKnowledge, "knowledge", content); err != nil && l.debugLog != nil {
		l.debugLog("[智能搜索] 存储知识记忆失败: %v", err)
	}
}

func (l *SearchLearner) storeExperienceMemory(ctx context.Context, refined *refinedQuery, rounds []searchRound) {
	if l.memoryDB == nil || !l.memoryDB.IsMemoryInitialized() {
		return
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("查询类型：%s\n", classifyQueryTypeStr(refined.Original)))
	sb.WriteString(fmt.Sprintf("原始查询：%s\n", truncateStr(refined.Original, 100)))
	sb.WriteString(fmt.Sprintf("完善后查询：%s\n", truncateStr(refined.Refined, 200)))

	if len(rounds) == 0 {
		sb.WriteString("策略：信息充足，无需深度搜索\n")
		sb.WriteString("效果：直接返回，延迟低\n")
		sb.WriteString("指导建议：此类查询通过记忆库和初步搜索即可满足，建议优先查询记忆库。\n")
	} else {
		sb.WriteString(fmt.Sprintf("策略：深度搜索%d轮\n", len(rounds)))
		for _, r := range rounds {
			sb.WriteString(fmt.Sprintf("  轮次%d: %s → 充足=%v\n", r.RoundNum, r.Query, r.Sufficient))
		}
		lastSufficient := rounds[len(rounds)-1].Sufficient
		if lastSufficient {
			sb.WriteString(fmt.Sprintf("效果：%d轮搜索后信息充足，查询词覆盖了问题核心方面\n", len(rounds)))
		} else {
			sb.WriteString(fmt.Sprintf("效果：%d轮搜索后信息仍不充足，可能需要更精确的搜索词\n", len(rounds)))
		}
		// 指导建议
		if len(rounds) <= 2 {
			sb.WriteString("指导建议：此类查询需要少量深度搜索，建议使用标准搜索策略。\n")
		} else {
			sb.WriteString("指导建议：此类查询需要多轮深度搜索，建议启用多角度搜索策略。\n")
		}
	}

	if err := l.memoryDB.MemoryAddMessageSilent(ctx, collectionExperience, "experience", sb.String()); err != nil && l.debugLog != nil {
		l.debugLog("[智能搜索] 存储经验记忆失败: %v", err)
	}
}

// ── 辅助函数 ──

// extractJSON 从 LLM 响应中提取 JSON（处理 markdown 代码块包裹）
func extractJSON(s string) string {
	s = strings.TrimSpace(s)
	// 去掉 markdown 代码块标记
	if strings.HasPrefix(s, "```") {
		if idx := strings.Index(s, "\n"); idx != -1 {
			s = s[idx+1:]
		}
		if idx := strings.LastIndex(s, "```"); idx != -1 {
			s = s[:idx]
		}
	}
	return strings.TrimSpace(s)
}

func truncateStr(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}

func isDuplicateQuery(query string, previous []string) bool {
	q := strings.TrimSpace(strings.ToLower(query))
	for _, p := range previous {
		if strings.TrimSpace(strings.ToLower(p)) == q {
			return true
		}
	}
	return false
}

func classifyQueryTypeStr(query string) string {
	if len([]rune(query)) > 50 {
		return "复杂查询"
	}
	return "简单查询"
}