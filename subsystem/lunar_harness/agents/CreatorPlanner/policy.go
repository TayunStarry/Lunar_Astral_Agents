package CreatorPlanner

// policy.go 创作规划者的工具循环修复策略：记录调用序列，并可复位已卡死的在途记录。
//
// 智能维修只允许修复已经产生的状态（最近推理记录、工具调用序列），
// 不得临时禁用工具或改写后续判定策略——那会约束智能体未来的行为。

import (
	"context"

	"LunarSubsystem/LunarHarness/protocol/agentbase"
)

// beginToolRun 开始新一轮工具循环：清空上一次的调用记录。
func beginToolRun() {
	policyMu.Lock()
	toolCalls = nil
	toolCallCounts = map[string]int{}
	policyMu.Unlock()
}

// recordToolCall 记录一次工具调用（有界）。
func recordToolCall(name, args, note string) {
	policyMu.Lock()
	defer policyMu.Unlock()
	toolCallCounts[name]++
	if len(toolCalls) < maxRecordedToolCalls {
		toolCalls = append(toolCalls, toolCallRecord{
			Name: name,
			Args: agentbase.TruncateRunes(args, 240),
			Note: note,
		})
	}
}

// dominantTool 返回本次循环调用次数最多的工具及其示例参数。
func dominantTool() (string, string, int) {
	policyMu.Lock()
	defer policyMu.Unlock()
	name := ""
	count := 0
	for tool, times := range toolCallCounts {
		if times > count {
			name, count = tool, times
		}
	}
	args := ""
	for _, call := range toolCalls {
		if call.Name == name {
			args = call.Args
			break
		}
	}
	return name, args, count
}

// toolPolicySnapshot 返回本次循环调用序列的摘要。
func toolPolicySnapshot() map[string]any {
	policyMu.Lock()
	defer policyMu.Unlock()
	return map[string]any{
		"recorded_calls": len(toolCalls),
		"call_counts":    cloneCounts(toolCallCounts),
	}
}

// cloneCounts 复制调用计数，避免导出后与运行时共享 map。
func cloneCounts(counts map[string]int) map[string]int {
	out := make(map[string]int, len(counts))
	for key, value := range counts {
		out[key] = value
	}
	return out
}

// toolRunRecords 返回最近一次循环的工具调用序列。
func toolRunRecords() []map[string]any {
	policyMu.Lock()
	defer policyMu.Unlock()
	out := make([]map[string]any, 0, len(toolCalls))
	for _, call := range toolCalls {
		out = append(out, map[string]any{
			"name": call.Name,
			"args": call.Args,
			"note": call.Note,
		})
	}
	return out
}

// resetRepairState 清空最近推理记录与调用序列（只复位已产生的状态）。
func resetRepairState() string {
	beginToolRun()
	return resetLastRun() + "，并清空工具调用序列"
}

// verifyResetState 验证 reset 是否真的清掉了在途记录；不涉及任何后续行为约束。
func verifyResetState(_ context.Context, _ map[string]any) (any, error) {
	policyMu.Lock()
	recorded := len(toolCalls)
	policyMu.Unlock()
	lastRunCleared := len(snapshotLastRun()) == 0
	if recorded == 0 && lastRunCleared {
		return map[string]any{
			"verified":       true,
			"mode":           "state_check",
			"detail":         "在途工具调用序列与最近推理记录均已清空；未改动后续工具可用性",
			"recorded_calls": 0,
		}, nil
	}
	return map[string]any{
		"verified":       false,
		"mode":           "state_check",
		"recorded_calls": recorded,
		"last_run_clear": lastRunCleared,
		"reason":         "在途记录尚未清空，请重新执行 reset",
	}, nil
}
