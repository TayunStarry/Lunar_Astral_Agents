package Creator

// plan.go 调度者：读取规划日志与触发信息，统一推理并提交有序计划。
// 计划中的每个调度项都必须是自然语言描述，并指定一个专属智能体。

import (
	"context"
	"fmt"
	"strings"
)

// planRound 调用调度者生成本轮计划。
func (s *session) planRound(ctx context.Context, instruction string, history journal) (planResult, error) {
	prompt := plannerPrompt(history)
	user := instruction + "\n\n" + formatJournal(history)
	handler := func(name string, args map[string]any) (toolOutcome, bool, error) {
		if name == toolSubmitPlan {
			parsed, err := parsePlan(args)
			if err != nil {
				return toolOutcome{
					JSON:    jsonText(map[string]any{"error": err.Error()}),
					Summary: "计划被拒绝：" + err.Error(),
				}, false, nil
			}
			s.plan = parsed
			s.planReady = true
			for _, memo := range parsed.Memos {
				s.addMemo(memo)
			}
			return toolOutcome{
				JSON:    jsonText(map[string]any{"accepted": true, "steps": len(parsed.Steps)}),
				Summary: fmt.Sprintf("计划已接收（%d 步）", len(parsed.Steps)),
			}, true, nil
		}
		return s.executeTool(ctx, name, args), false, nil
	}
	if _, err := s.runToolLoop(ctx, plannerLabel, prompt, user, definitionsFor(plannerTools), handler); err != nil {
		return planResult{}, err
	}
	if !s.planReady {
		return planResult{}, fmt.Errorf("调度者没有提交有效计划")
	}
	return s.plan, nil
}

// plannerPrompt 调度者的系统提示。
func plannerPrompt(history journal) string {
	return `你是「自由创作.调度者」，负责为月华的本轮创作统一推理与编排。

工作方式：
1. 先读懂规划日志里的上一轮规划、遗留备忘、参考资料与错误；需要补充事实时调用工具（联网搜索、记忆库、句柄、文件、代码索引、DTS 文档）。
2. 编排完成后必须调用 submit_plan 提交计划，不能只回复文字。
3. steps 必须按严格顺序执行，后一步可以依赖前一步的产物，但不可并列或乱序。
4. 每个步骤用自然语言详细写清执行目标与操作要求，并指定专属智能体：
   - writer：文字创作与文档维护（个人文档、小说、故事、诗歌等）
   - engineer：JavaScript 项目创建与推进、代码文件维护
   - researcher：资料查阅、开发文档研究、目录与文件浏览
   - runner：脚本运行与结果查看、内容调度
5. 若上一轮有未完成或遗留事项，优先延续之前的创作思路；也可以发起全新创作。
6. 所有文件路径一律写相对 /document 的路径。
7. 不要臆造工具结果；没有查到的资料就明确留待下一步查证。
8. 把需要下一轮继续的期望写进 memos。

历史日志已附在用户消息中。` + journalHint(history)
}

// journalHint 对历史日志做一句概括，帮助调度者抓重点。
func journalHint(history journal) string {
	if len(history.LastPlans) == 0 && len(history.Memos) == 0 {
		return "\n\n这是第一轮创作，没有历史日志。"
	}
	return fmt.Sprintf("\n\n历史概况：上一轮 %d 步规划、%d 条备忘、%d 条错误。",
		len(history.LastPlans), len(history.Memos), len(history.Errors))
}

// parsePlan 解析并校验 submit_plan 参数。
func parsePlan(args map[string]any) (planResult, error) {
	rawSteps, ok := args["steps"].([]any)
	if !ok || len(rawSteps) == 0 {
		return planResult{}, fmt.Errorf("steps 必须是非空数组")
	}
	if len(rawSteps) > 12 {
		return planResult{}, fmt.Errorf("单轮计划最多 12 步，请合并同类操作")
	}
	steps := make([]planStep, 0, len(rawSteps))
	for i, raw := range rawSteps {
		item, ok := raw.(map[string]any)
		if !ok {
			return planResult{}, fmt.Errorf("第 %d 步格式不正确", i+1)
		}
		agent := strings.TrimSpace(argString(item, "agent"))
		if !validAgent(agent) {
			return planResult{}, fmt.Errorf("第 %d 步的 agent「%s」不合法", i+1, agent)
		}
		task := strings.TrimSpace(argString(item, "task"))
		if task == "" {
			return planResult{}, fmt.Errorf("第 %d 步缺少 task 描述", i+1)
		}
		steps = append(steps, planStep{Agent: agent, Task: task})
	}

	memos := make([]string, 0, maxMemos)
	if rawMemos, ok := args["memos"].([]any); ok {
		for _, raw := range rawMemos {
			if memo, ok := raw.(string); ok && strings.TrimSpace(memo) != "" {
				memos = append(memos, clipAtSentence(memo, maxMemoRunes))
				if len(memos) >= maxMemos {
					break
				}
			}
		}
	}
	return planResult{Steps: steps, Memos: memos}, nil
}
