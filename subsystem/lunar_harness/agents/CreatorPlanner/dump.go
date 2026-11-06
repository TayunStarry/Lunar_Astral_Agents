package CreatorPlanner

// dump.go 创作规划者的数据导出：最近一次推理循环与登记的工具声明。

import (
	"time"

	"LunarSubsystem/LunarHarness/protocol/agentdump"
	"LunarSubsystem/LunarHarness/protocol/toolreg"
)

// exportContext 创作规划者的上下文快照；dump 导出与智能修复的 Inspect 共用这一份。
func exportContext(includeImages bool) (any, error) {
	_ = includeImages
	schemas := toolreg.Schemas(AgentID)
	tools := make([]map[string]string, 0, len(schemas))
	for _, schema := range schemas {
		tools = append(tools, map[string]string{"name": schema.Name, "description": schema.Description})
	}
	return map[string]any{
		"kind":         "creator-planner-run",
		"generated_at": time.Now().Format(time.RFC3339),
		"last_run":     snapshotLastRun(),
		"tool_policy":  toolPolicySnapshot(),
		"tools":        tools,
	}, nil
}

func init() {
	agentdump.Register(agentdump.Provider{
		ID:          AgentID,
		Label:       "创作规划者",
		Group:       "Go 智能体",
		Description: "创作规划者的最近一次推理循环（角色、轮次、错误）与已登记的工具声明",
		Export:      exportContext,
	})
}
