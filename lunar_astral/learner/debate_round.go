package learner

import (
	"fmt"
	"strings"

	"logger"
)

// ============================================================
// 辩论轮次
// ============================================================

// debateSubQuestion 对单个子问题进行辩论
func (d *DebateSystem) debateSubQuestion(subQ SubQuestion, memResults []MemoryMatch) ([]DebateRound, error) {
	var rounds []DebateRound
	var previousSummary string

	for roundNum := 1; roundNum <= d.state.MaxRounds; roundNum++ {
		logger.Info("Learner", "辩论轮次 %d/%d", roundNum, d.state.MaxRounds)

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
	debateContext := d.buildDebateContext(subQ, memResults, previousSummary)

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
		converged = roundNum >= d.state.MaxRounds
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
func (d *DebateSystem) buildDebateContext(subQ SubQuestion, memResults []MemoryMatch, previousSummary string) string {
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
