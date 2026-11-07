package CreatorPlanner

// repair.go 向维修侧注册创作规划者的只读诊断入口。

import (
	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/protocol/agentrepair"
)

func init() {
	agentrepair.Register(agentrepair.Handler{
		ID:          AgentID,
		Label:       DisplayName,
		Description: "按次执行、不保留跨轮对话；可读配置、工具序列与最近一次运行记录（只读）",
		Inspect:     func(int) (any, error) { return exportContext(false) },
		Status:      statusSnapshot,
	})
}

// statusSnapshot 返回创作规划者的配置与运行态。
func statusSnapshot() (any, error) {
	return map[string]any{
		"kind":                "creator-planner-status",
		"model":               config.AgentMultimodalModel,
		"max_executor_rounds": MaxExecutorRounds,
		"max_planner_rounds":  MaxPlannerRounds,
		"policy":              toolPolicySnapshot(),
		"last_run":            snapshotLastRun(),
	}, nil
}
