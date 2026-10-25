//go:build windows

package desktop

// ==== 规划阶段：任务规划者 ====
// 规划者只拿到同一轮感知快照、目标窗口状态与最近尝试，输出可立即执行的一步决策。

import (
	"context"
	"encoding/json"
	"strings"
)

// plannerPhase 任务规划者：综合视觉/UIA 情报决策下一步，返回决策结构。
// 输入里显式给出「目标窗口是否在前台」与「最近尝试」，让规划者不必反复试探焦点。
func plannerPhase(ctx context.Context, plan editorTask, handoff HandoffRecord, treeSummary string, elements []string) planDecision {
	var sb strings.Builder
	sb.WriteString(buildPlanContext(plan))
	sb.WriteString("\n")
	sb.WriteString("【进度】")
	sb.WriteString(stringifyHandoff(handoff))
	sb.WriteString("\n")
	sb.WriteString("【窗口状态】前台=")
	sb.WriteString(handoff.Foreground)
	sb.WriteString("（进程 ")
	sb.WriteString(handoff.ForegroundProcess)
	sb.WriteString("）；目标应用=")
	sb.WriteString(handoff.TargetApp)
	sb.WriteString("\n")
	sb.WriteString("【UIA 情报】树摘要:")
	sb.WriteString(treeSummary)
	sb.WriteString("\n元素:")
	sb.WriteString(strings.Join(elements, "；"))
	sb.WriteString("\n")
	sb.WriteString("请决策下一步（mouse/keyboard/uia/complete），输出 JSON。")
	text, err := autoChatText(ctx, autoRolePlanner, []chatMessage{{Role: "system", Content: promptPlanner}, {Role: "user", Content: sb.String()}})
	if err != nil {
		return planDecision{Decision: "complete", Action: "（规划失败，结束）"}
	}
	var d planDecision
	if json.Unmarshal(extractJSON(text), &d) != nil {
		return planDecision{Decision: "complete", Action: "（规划输出无法解析，结束）"}
	}
	return d
}
