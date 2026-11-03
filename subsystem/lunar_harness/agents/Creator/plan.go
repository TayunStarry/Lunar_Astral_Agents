package Creator

// plan.go 调度者：读取规划日志与触发信息，统一推理并提交有序计划。
// 计划中的每个调度项都必须是自然语言描述，并指定一个专属智能体。

import (
	"context"
	"fmt"
	"strings"

	"LunarSubsystem/LunarHarness/engine/fileflow"
)

// planRound 调用调度者生成本轮计划；recent 为增量库中的最近内容级操作记录。
func (s *session) planRound(ctx context.Context, instruction string, history journal, recent []fileflow.OpRecord) (planResult, error) {
	prompt := plannerPrompt(s.mode, history, len(recent))
	user := instruction + "\n\n" + formatJournal(history, recent)
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
		if !plannerToolAllowed(name) {
			return failure(name, fmt.Errorf("调度阶段只允许只读工具，写操作请写进计划步骤")), false, nil
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

// plannerToolAllowed 判断工具是否属于调度阶段的只读能力集。
func plannerToolAllowed(name string) bool {
	for _, tool := range plannerTools {
		if tool == name {
			return true
		}
	}
	return false
}

// plannerPrompt 调度者的系统提示。
func plannerPrompt(mode fileflow.Mode, history journal, recentOps int) string {
	return `你是「自由创作.调度者」，负责统一推理与编排本轮任务。` + directiveBlock(mode, history) + modeBlock(mode) + `

工作方式：
1. 先读懂规划日志里的历史信息；需要补充事实时调用工具（联网搜索、记忆库、句柄、文件、代码索引、DTS 文档）。
2. 编排完成后必须调用 submit_plan 提交计划，不能只回复文字。
   你只负责调度与节点管理：不要直接写文件、删除文件或运行脚本，所有写操作必须落到计划步骤里。
3. steps 必须按严格顺序执行，后一步可以依赖前一步的产物，但不可并列或乱序。
4. 每个步骤用自然语言详细写清执行目标与操作要求，并指定专属智能体：
   - writer：文字创作与文档维护（个人文档、小说、故事、诗歌等）
   - engineer：JavaScript 项目创建与推进、代码文件维护
   - researcher：资料查阅、开发文档研究、目录与文件浏览
   - runner：脚本运行与结果查看、内容调度
5. 优先级：本轮创作参考中的明确要求 > 上一轮遗留事项 > 延续上一轮的创作思路。
   与本轮指令或用户要求冲突时，不要继续上一轮的题材、文体或篇幅；用户没有新要求时，才延续遗留事项。
6. 所有文件路径一律写相对 /document 的路径。
7. 不要臆造工具结果；没有查到的资料就明确留待下一步查证。
8. 把需要下一轮继续的期望写进 memos。
9. 状态归属：本轮指令与规划日志中的「心情 / 生活记录 / 刚才在…」都是月华本人的状态，
   不是用户的状态；不要写成「用户处于…」。历史日志里若出现把月华状态记到用户身上的
   表述，视为笔误，先纠正为月华本人状态再编排。
10. 自动审查：每个步骤可写 expect 数组，声明该步骤完成后应满足的客观事实，
    系统会在每次文件操作后逐条比对，不一致立即回滚：
    - path：目标路径（相对 /document 或完整路径）
    - exists：期望操作后是否存在（写文件/创建填 true，删除填 false）
    - contains / not_contains：期望包含 / 禁止包含的文本片段
    - min_bytes / max_bytes：期望的字节数范围
    写文件至少声明「目标存在 + 关键内容片段」；删除声明 exists=false。
11. 文件操作会按目标实际内容自动路由：压缩包走归档智能体，大型目录 / 长文档走文献解析，
    普通文本走文件编辑。你只需在 task 中写准目标路径，不要自行指定承接智能体。

历史日志已附在用户消息中。` + journalHint(history, recentOps)
}

// modeBlock 返回当前运行模式的编排补充说明。
func modeBlock(mode fileflow.Mode) string {
	if mode == fileflow.ModeAssigned {
		return `

【本次为对话者指派的任务】
- 只完成本轮指令要求的事，不要主动延续上一轮的创作题材或篇幅。
- 历史信息里只有文件操作记录：用它避免重复操作、衔接已有产物；不得把记录之外的内容当作本轮上下文。
- 任务集中在文件操作时，步骤要写清目标路径与预期结果；不要拆分出与任务无关的创作步骤。`
	}
	return `

【本次为自由创作模式】
- 在完成本轮指令的同时，可以延续上一轮的创作思路；完整规划与操作记录会全程保存。`
}

// directiveBlock 把本轮创作参考放到系统提示开头，明确其最高优先级。
func directiveBlock(mode fileflow.Mode, history journal) string {
	if mode == fileflow.ModeAssigned {
		return "\n\n【本轮创作参考】\n（本次是指派任务，没有额外的创作参考，按本轮指令执行）"
	}
	if len(history.Directives) == 0 {
		return "\n\n【本轮创作参考】\n（本轮没有新的创作参考，按上一轮思路或自由发挥）"
	}
	var b strings.Builder
	b.WriteString("\n\n【本轮创作参考（来自对话者给出的期望，优先级最高；与遗留事项冲突时以此为准）】\n")
	for _, item := range history.Directives {
		b.WriteString("- ")
		b.WriteString(item)
		b.WriteByte('\n')
	}
	return b.String()
}

// journalHint 对历史日志做一句概括，帮助调度者抓重点。
func journalHint(history journal, recentOps int) string {
	if len(history.LastPlans) == 0 && len(history.Memos) == 0 && recentOps == 0 {
		return "\n\n这是第一轮，没有历史日志。"
	}
	if recentOps > 0 && len(history.LastPlans) == 0 {
		return fmt.Sprintf("\n\n历史概况：%d 条内容级操作记录。", recentOps)
	}
	return fmt.Sprintf("\n\n历史概况：上一轮 %d 步规划、%d 条备忘、%d 条错误、%d 条内容级操作记录。",
		len(history.LastPlans), len(history.Memos), len(history.Errors), recentOps)
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
		steps = append(steps, planStep{
			Agent:  agent,
			Task:   task,
			Expect: parseExpects(item["expect"]),
		})
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

// parseExpects 解析步骤声明的规划预期。
func parseExpects(raw any) []fileflow.Expect {
	items, ok := raw.([]any)
	if !ok || len(items) == 0 {
		return nil
	}
	out := make([]fileflow.Expect, 0, len(items))
	for _, item := range items {
		fields, ok := item.(map[string]any)
		if !ok {
			continue
		}
		path := strings.TrimSpace(argString(fields, "path"))
		if path == "" {
			continue
		}
		expect := fileflow.Expect{
			Path:        path,
			Contains:    argStringSlice(fields, "contains"),
			NotContains: argStringSlice(fields, "not_contains"),
		}
		if value, ok := fields["exists"].(bool); ok {
			exists := value
			expect.Exists = &exists
		}
		if value, ok := argInt64(fields, "min_bytes"); ok {
			expect.MinBytes = &value
		}
		if value, ok := argInt64(fields, "max_bytes"); ok {
			expect.MaxBytes = &value
		}
		out = append(out, expect)
		if len(out) >= maxExpectsPerStep {
			break
		}
	}
	return out
}

// argStringSlice 读取字符串数组（兼容单个字符串）。
func argStringSlice(args map[string]any, key string) []string {
	switch value := args[key].(type) {
	case []any:
		out := make([]string, 0, len(value))
		for _, item := range value {
			if text, ok := item.(string); ok && strings.TrimSpace(text) != "" {
				out = append(out, text)
				if len(out) >= maxExpectFragments {
					break
				}
			}
		}
		return out
	case string:
		if strings.TrimSpace(value) == "" {
			return nil
		}
		return []string{value}
	}
	return nil
}

// argInt64 读取整数参数（兼容 JSON 数字与字符串）。
func argInt64(args map[string]any, key string) (int64, bool) {
	switch value := args[key].(type) {
	case float64:
		return int64(value), true
	case int:
		return int64(value), true
	case int64:
		return value, true
	}
	return 0, false
}
