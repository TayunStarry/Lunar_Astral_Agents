package CreatorPlanner

// policy.go 创作规划者的工具循环修复策略：记录调用序列、临时禁用问题工具、验证拦截是否生效。
//
// 禁用表是「下一次运行真实生效」的修复手段：被禁用的工具在 Execute 入口直接返回业务错误，
// 让模型改用其他工具或结束，而不是继续空转到轮次上限。

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/protocol/agentbase"
)

// beginToolRun 开始新一轮工具循环：清空上一次的调用记录，保留禁用表。
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

// toolDeniedReason 判断工具是否在禁用期内，返回禁用原因。
func toolDeniedReason(name string) (string, bool) {
	now := time.Now()
	policyMu.Lock()
	defer policyMu.Unlock()
	for tool, denied := range deniedTools {
		if now.After(denied.Until) {
			delete(deniedTools, tool)
		}
	}
	denied, ok := deniedTools[name]
	if !ok || now.After(denied.Until) {
		return "", false
	}
	return denied.Reason, true
}

// denyTool 临时禁用工具，返回到期时间。
func denyTool(name, reason string, ttl time.Duration) (time.Time, error) {
	name = agentbase.TruncateRunes(name, 120)
	if name == "" {
		return time.Time{}, fmt.Errorf("缺少要禁用的工具名")
	}
	if ttl <= 0 {
		ttl = denyDefaultTTL
	}
	if ttl > denyMaxTTL {
		ttl = denyMaxTTL
	}
	until := time.Now().Add(ttl)
	policyMu.Lock()
	deniedTools[name] = deniedTool{Reason: reason, Until: until}
	for len(deniedTools) > maxDeniedTools {
		oldestName := ""
		var oldest time.Time
		for tool, denied := range deniedTools {
			if oldestName == "" || denied.Until.Before(oldest) {
				oldestName, oldest = tool, denied.Until
			}
		}
		delete(deniedTools, oldestName)
	}
	policyMu.Unlock()
	return until, nil
}

// clearDeniedTools 解除全部工具禁用。
func clearDeniedTools() int {
	policyMu.Lock()
	defer policyMu.Unlock()
	count := len(deniedTools)
	deniedTools = map[string]deniedTool{}
	return count
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

// toolPolicySnapshot 返回禁用表与调用序列的摘要。
func toolPolicySnapshot() map[string]any {
	now := time.Now()
	policyMu.Lock()
	defer policyMu.Unlock()
	denied := make([]map[string]any, 0, len(deniedTools))
	for tool, item := range deniedTools {
		if now.After(item.Until) {
			continue
		}
		denied = append(denied, map[string]any{
			"tool":             tool,
			"reason":           item.Reason,
			"remaining_minute": int(item.Until.Sub(now).Minutes()),
		})
	}
	sort.Slice(denied, func(i, j int) bool {
		return denied[i]["tool"].(string) < denied[j]["tool"].(string)
	})
	return map[string]any{
		"denied_tools":   denied,
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

// resetRepairState 清空最近推理记录与调用序列；禁用表保留（那是修复手段）。
func resetRepairState() string {
	beginToolRun()
	return resetLastRun() + "，并清空工具调用序列（已生效的工具禁用保留）"
}

// verifyToolPolicy 检查本次循环里调用最多的问题工具是否已被禁用。
func verifyToolPolicy(_ context.Context, params map[string]any) (any, error) {
	name, args, count := dominantTool()
	if provided, ok := params["tool"].(string); ok && strings.TrimSpace(provided) != "" {
		name = strings.TrimSpace(provided)
	}
	if name == "" {
		return map[string]any{
			"verified": false,
			"mode":     "policy_check",
			"reason":   "没有记录到工具调用序列，无法确认禁用是否命中；请读取运行态人工核对",
		}, nil
	}
	reason, denied := toolDeniedReason(name)
	if !denied {
		return map[string]any{
			"verified":     false,
			"mode":         "policy_check",
			"tool":         name,
			"repeat_count": count,
			"reason":       "问题工具尚未被禁用，下一轮仍可能重复调用",
		}, nil
	}
	return map[string]any{
		"verified":     true,
		"mode":         "policy_check",
		"tool":         name,
		"tool_args":    args,
		"repeat_count": count,
		"deny_reason":  reason,
		"detail":       "下一轮调用该工具会直接返回禁用错误，模型需改用其他工具或结束",
	}, nil
}
