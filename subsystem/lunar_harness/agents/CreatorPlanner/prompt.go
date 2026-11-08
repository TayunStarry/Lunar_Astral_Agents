package CreatorPlanner

// prompt.go 创作规划者的系统提示组装：固定文案放在 prompts/，按轮次注入运行模式、
// 创作参考与历史概况；提示词文本与迁移前逐字一致。

import (
	"fmt"
	"strings"

	"LunarSubsystem/LunarHarness/protocol/agentbase"
)

// PlannerPrompt 调度者系统提示。
// assigned 为 true 表示对话者指派的单次任务；directives 为本轮创作参考；
// 其余四个计数为上一轮规划日志的规模（计划、备忘、错误、内容级变更）；
// pathSnapshot 为已有路径树快照，只注入规划阶段，执行体不携带。
func PlannerPrompt(assigned bool, directives []string, lastPlans, memos, errors, changes int, pathSnapshot string) string {
	return agentbase.RenderPrompt(
		promptText(promptPlanner),
		"{directives}", directiveBlock(assigned, directives),
		"{mode}", modeBlock(assigned),
		"{journal-hint}", journalHint(assigned, lastPlans, memos, errors, changes),
		"{path-snapshot}", pathSnapshotBlock(pathSnapshot),
	)
}

// ExecutorPrompt 返回专属智能体的系统提示。
func ExecutorPrompt(agent string) string {
	base := promptText(promptExecutor)
	switch agent {
	case AgentWriter:
		return joinPrompt(base, promptWriter)
	case AgentEngineer:
		return joinPrompt(base, promptEngineer)
	case AgentResearcher:
		return joinPrompt(base, promptResearcher)
	case AgentRunner:
		return joinPrompt(base, promptRunner)
	default:
		return base
	}
}

// directiveBlock 把本轮创作参考放到系统提示开头，明确其最高优先级。
func directiveBlock(assigned bool, directives []string) string {
	if assigned {
		return "\n\n【本轮创作参考】\n（本次是指派任务，没有额外的创作参考，按本轮指令执行）"
	}
	if len(directives) == 0 {
		return "\n\n【本轮创作参考】\n（本轮没有新的创作参考，按上一轮思路或自由发挥）"
	}
	var b strings.Builder
	b.WriteString("\n\n【本轮创作参考（来自对话者给出的期望，优先级最高；与遗留事项冲突时以此为准）】\n")
	for _, item := range directives {
		b.WriteString("- ")
		b.WriteString(item)
		b.WriteByte('\n')
	}
	return b.String()
}

// modeBlock 返回当前运行模式的编排补充说明。
func modeBlock(assigned bool) string {
	if assigned {
		return `

【本次为对话者指派的任务】
- 只完成本轮指令要求的事，不要主动延续上一轮的创作题材或篇幅。
- 历史信息里只有上一轮指派的步骤、错误，以及可按路径查询的文件变更；
  用它避免重复操作、衔接已有产物；不得把记录之外的内容当作本轮上下文。
- 任务集中在文件操作时，步骤要写清目标路径与预期结果；不要拆分出与任务无关的创作步骤。`
	}
	return `

【本次为自由创作模式】
- 在完成本轮指令的同时，可以延续上一轮的创作思路；完整规划与操作记录会全程保存。`
}

// journalHint 对历史日志做一句概括，帮助调度者抓重点。
func journalHint(assigned bool, lastPlans, memos, errors, changes int) string {
	if assigned {
		if lastPlans == 0 && errors == 0 {
			return "\n\n这是第一次指派任务，没有历史步骤与错误；可用 change_query 查看已有文件的变更。"
		}
		return fmt.Sprintf("\n\n历史概况：上一轮指派 %d 步、%d 条错误；可用 change_query 查看文件变更。",
			lastPlans, errors)
	}
	if lastPlans == 0 && memos == 0 && changes == 0 {
		return "\n\n这是第一轮，没有历史日志。"
	}
	return fmt.Sprintf("\n\n历史概况：上一轮 %d 步规划、%d 条备忘、%d 条错误、%d 条内容级变更。",
		lastPlans, memos, errors, changes)
}

// pathSnapshotBlock 注入规划阶段的已有路径树快照；为空时不占用提示体积。
func pathSnapshotBlock(snapshot string) string {
	snapshot = strings.TrimSpace(snapshot)
	if snapshot == "" {
		return ""
	}
	return "\n\n【已有文件路径快照（/document 与 unknown；路径可直接用作工具参数，不含大小、时间、正文与句柄）】\n" + snapshot
}

// joinPrompt 拼接主提示与角色附加提示，避免空段落。
func joinPrompt(base, name string) string {
	extra := promptText(name)
	if extra == "" {
		return base
	}
	if base == "" {
		return extra
	}
	return base + "\n\n" + extra
}

// promptText 读取内嵌提示词并去掉文件末尾换行（保持与迁移前文案逐字一致）。
func promptText(name string) string {
	return strings.TrimRight(agentbase.PromptText(name), "\r\n")
}
