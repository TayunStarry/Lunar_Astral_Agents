package CreatorPlanner

// repair.go 向拯救者注册创作规划者的修复能力。

import (
	"fmt"
	"time"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/protocol/agentrepair"
)

func init() {
	agentrepair.Register(agentrepair.Handler{
		ID:          AgentID,
		Label:       "创作规划者",
		Description: "按次执行、不保留跨轮对话；可临时禁用反复调用或参数非法的工具，避免下一轮继续空转到轮次上限",
		Actions:     []string{"deny_tool", "clear_deny", "reset"},
		Inspect:     func(int) (any, error) { return exportContext(false) },
		Status:      statusSnapshot,
		Verify:      verifyToolPolicy,
		Repair:      applyRepair,
	})
}

// statusSnapshot 返回创作规划者的配置与运行态。
func statusSnapshot() (any, error) {
	return map[string]any{
		"kind":                "creator-planner-status",
		"model":               config.AgentMultimodalModel,
		"max_executor_rounds": MaxExecutorRounds,
		"max_planner_rounds":  MaxPlannerRounds,
		"max_repeats":         MaxRepeats,
		"policy":              toolPolicySnapshot(),
		"last_run":            snapshotLastRun(),
	}, nil
}

// applyRepair 执行创作规划者的修复动作。
func applyRepair(action string, params map[string]any) (string, error) {
	switch action {
	case "deny_tool":
		name := agentrepair.StringParam(params, "tool")
		if name == "" {
			name, _, _ = dominantTool()
		}
		if name == "" {
			return "", fmt.Errorf("没有指定工具，且本次循环没有记录到可定位的工具调用")
		}
		reason := agentrepair.StringParam(params, "reason")
		if reason == "" {
			reason = "重复调用或参数非法，已在智能修复中禁用"
		}
		until, err := denyTool(name, reason, toolTTLFrom(params))
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("已临时禁用工具 %s（到期 %s）；下一轮调用会直接返回禁用错误", name, until.Format("15:04")), nil
	case "clear_deny":
		count := clearDeniedTools()
		return fmt.Sprintf("已解除 %d 个工具的临时禁用", count), nil
	case "reset":
		return resetRepairState(), nil
	default:
		return "", fmt.Errorf("创作规划者不支持动作：%s", action)
	}
}

// toolTTLFrom 读取禁用有效期（分钟），夹在 [5, 120]。
func toolTTLFrom(params map[string]any) time.Duration {
	minutes := agentrepair.IntParam(params, "ttl_minutes", int(denyDefaultTTL/time.Minute),
		int(5), int(denyMaxTTL/time.Minute))
	return time.Duration(minutes) * time.Minute
}
