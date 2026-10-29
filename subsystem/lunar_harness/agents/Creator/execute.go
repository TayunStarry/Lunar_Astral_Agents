package Creator

// execute.go 按调度顺序依次调用专属智能体执行计划。
// 单步出现循环输出或 500 类异常时自动重试一次，仍失败则跳过并继续后续步骤。

import (
	"context"
	"fmt"
	"strings"
)

// executePlan 顺序执行计划并返回每步结果。
func (s *session) executePlan(ctx context.Context, plan planResult) []stepResult {
	results := make([]stepResult, 0, len(plan.Steps))
	prior := "（无）"
	for i, step := range plan.Steps {
		label := agentLabels[step.Agent]
		instruction := fmt.Sprintf(
			"本轮第 %d/%d 步。\n专属智能体：%s\n任务：%s\n前面步骤的结果：%s\n"+
				"请用工具完成本步骤；完成后用中文简要说明产出（文件路径与关键内容），不要臆造结果。",
			i+1, len(plan.Steps), label, step.Task, prior,
		)

		text, err := s.runStep(ctx, step.Agent, instruction)
		if retryable(err) {
			s.addError(fmt.Errorf("第 %d 步首次执行失败: %w", i+1, err))
			retryInstruction := instruction + "\n\n注意：上一次执行失败（" +
				previewRunes(err.Error(), 160) + "）。请换一种方式完成，避免重复相同的工具调用与相同输出。"
			text, err = s.runStep(ctx, step.Agent, retryInstruction)
		}
		if err != nil {
			s.addError(fmt.Errorf("第 %d 步[%s]失败并跳过: %w", i+1, label, err))
			s.addMemo("未完成：" + step.Task)
			results = append(results, stepResult{
				Agent:   step.Agent,
				Task:    step.Task,
				Skipped: true,
				Error:   err.Error(),
			})
			prior = fmt.Sprintf("第 %d 步（%s）被跳过：%s", i+1, label, previewRunes(err.Error(), 100))
			continue
		}
		text = strings.TrimSpace(text)
		if text == "" {
			text = "已完成本步骤的工具操作"
		}
		results = append(results, stepResult{Agent: step.Agent, Task: step.Task, Result: text})
		prior = fmt.Sprintf("第 %d 步（%s）完成：%s", i+1, label, previewRunes(text, 200))
	}
	return results
}

// runStep 用指定专属智能体执行一步。
func (s *session) runStep(ctx context.Context, agent, instruction string) (string, error) {
	prompt := executorPrompt(agent)
	tools := definitionsFor(toolsFor(agent))
	handler := func(name string, args map[string]any) (toolOutcome, bool, error) {
		return s.executeTool(ctx, name, args), false, nil
	}
	return s.runToolLoop(ctx, agentLabels[agent], prompt, instruction, tools, handler)
}

// executorPrompt 返回专属智能体的系统提示。
func executorPrompt(agent string) string {
	base := `你在执行「月华.生活.创作」管线中的一个步骤。只能通过工具完成操作，不要臆造结果。
所有文件路径一律相对 /document；需要外部资料时可随时联网搜索或查询记忆库。
完成后用中文简要说明产出（文件路径与关键内容）。`
	switch agent {
	case agentWriter:
		return base + `

你是文字创作者：负责编写、更新与维护个人文档、小说、故事、诗歌等文字内容。
写作时保持连贯的文风与人物设定；修改已有内容前先读取文件，尽量增量更新而不是整篇覆盖。
需要交付时可用 send_group_text / send_group_file 调度内容。`
	case agentEngineer:
		return base + `

你是工程创作者：负责创建或推进 JavaScript / LTP9 项目，维护代码文件。
运行环境限定为 LTP9 引擎；写脚本前可先用 read_dts 查阅开发文档，用 code_index 了解现有代码结构。
脚本写好后用 run_ltp9_script 加载运行（会自动签发授权），并用 script_status 监控状态。`
	case agentResearcher:
		return base + `

你是资料查阅者：负责查阅资料、研究 DTS 开发文档、浏览 /document 目录与代码索引。
只做检索与阅读，不修改文件；把关键结论、出处与可直接使用的素材整理清楚，供后续步骤使用。`
	case agentRunner:
		return base + `

你是脚本运行者：负责运行 LTP9 脚本并实时监控状态。
运行前确认脚本文件已存在；用 run_ltp9_script 传入一个或多个脚本（按顺序拼接加载），
随后用 script_status 查看是否已加载、是否报错、是否导出了可调用结果。`
	default:
		return base
	}
}

// summarizeSteps 汇总本轮执行结果，作为生活事件记录。
func summarizeSteps(results []stepResult, plan planResult) string {
	if len(results) == 0 {
		return "创作没有产生可执行的步骤"
	}
	done := 0
	var b strings.Builder
	for i, result := range results {
		if result.Skipped {
			fmt.Fprintf(&b, "%d.[%s] 跳过：%s\n", i+1, agentLabels[result.Agent], previewRunes(result.Error, 80))
			continue
		}
		done++
		fmt.Fprintf(&b, "%d.[%s] %s\n", i+1, agentLabels[result.Agent], previewRunes(result.Result, 160))
	}
	prefix := fmt.Sprintf("本轮创作 %d 步，完成 %d 步", len(plan.Steps), done)
	return previewRunes(prefix+"：\n"+b.String(), 1200)
}

// mergeMemos 合并计划备忘与执行遗留备忘，去重并限长。
func mergeMemos(plan planResult, extra []string) []string {
	out := make([]string, 0, maxMemos)
	seen := map[string]bool{}
	for _, memo := range append(append([]string{}, plan.Memos...), extra...) {
		memo = strings.TrimSpace(memo)
		if memo == "" || seen[memo] {
			continue
		}
		seen[memo] = true
		out = append(out, previewRunes(memo, 300))
		if len(out) >= maxMemos {
			break
		}
	}
	return out
}
