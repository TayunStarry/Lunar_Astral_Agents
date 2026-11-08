package creation

// plan.go 调度者：读取规划日志与触发信息，统一推理并提交有序计划。
// 计划中的每个调度项都必须是自然语言描述，并指定一个专属智能体。

import (
	"context"
	"fmt"
	"strings"

	"LunarSubsystem/LunarHarness/agents/CreatorPlanner"
	"LunarSubsystem/LunarHarness/engine/fileflow"
)

// planRound 调用调度者生成本轮计划；history 为统一库中上一轮调用的可见信息。
func (s *session) planRound(ctx context.Context, instruction string, history journal) (planResult, error) {
	prompt := plannerPrompt(s.mode, history, pathSnapshotText())
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

// plannerPrompt 调度者的系统提示：固定文案与运行模式说明由创作规划者智能体持有，
// 链侧只把本轮模式、创作参考、历史日志规模与已有路径快照传过去。
func plannerPrompt(mode fileflow.Mode, history journal, pathSnapshot string) string {
	return CreatorPlanner.PlannerPrompt(
		mode == fileflow.ModeAssigned,
		history.Directives,
		len(history.LastPlans), len(history.Memos), len(history.Errors), len(history.Changes),
		pathSnapshot,
	)
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
		paint := parsePaint(item["paint"])
		if agent == agentPainter && (paint == nil || strings.TrimSpace(paint.Prompt) == "") {
			return planResult{}, fmt.Errorf("第 %d 步交给图片绘制时必须提供 paint.prompt（画面描述），不能只用 task 文字代替", i+1)
		}
		if agent == agentPainter && paint != nil && len(paint.References) > 0 && strings.TrimSpace(paint.Change) == "" {
			return planResult{}, fmt.Errorf("第 %d 步带参考图时必须提供 paint.change（基于参考图要改什么）", i+1)
		}
		steps = append(steps, planStep{
			Agent:  agent,
			Task:   task,
			Paint:  paint,
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

// parsePaint 解析交给图片绘制智能体的结构化绘图需求；缺省或格式不对时返回 nil。
func parsePaint(raw any) *paintBrief {
	fields, ok := raw.(map[string]any)
	if !ok {
		return nil
	}
	prompt := strings.TrimSpace(argString(fields, "prompt"))
	if prompt == "" {
		return nil
	}
	references := make([]string, 0, 2)
	if single := strings.TrimSpace(argString(fields, "reference")); single != "" {
		references = append(references, single)
	}
	for _, item := range argStringSlice(fields, "references") {
		if reference := strings.TrimSpace(item); reference != "" && !containsString(references, reference) {
			references = append(references, reference)
		}
	}
	if len(references) > maxPaintReferences {
		references = references[:maxPaintReferences]
	}
	return &paintBrief{
		Prompt:     clipAtSentence(prompt, maxPaintPromptRunes),
		Ratio:      clipAtSentence(strings.TrimSpace(argString(fields, "ratio")), maxPaintMetaRunes),
		Style:      clipAtSentence(strings.TrimSpace(argString(fields, "style")), maxPaintMetaRunes),
		References: references,
		Change:     clipAtSentence(strings.TrimSpace(argString(fields, "change")), maxPaintChangeRunes),
		Output:     clipAtSentence(strings.TrimSpace(argString(fields, "output")), maxPaintMetaRunes),
	}
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
