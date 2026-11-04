package MutualForum

// dump.go Mutual Forum 智能体的导出：最近一次模型轮次的诊断信息。
//
// 跨轮对话上下文已上移到 chain/forum，由论坛链负责导出；本包只暴露模型侧状态。

import (
	"time"

	"LunarSubsystem/LunarHarness/protocol/agentdump"
)

func init() {
	agentdump.Register(agentdump.Provider{
		ID:          "mutual_forum_agent",
		Label:       "互助论坛-模型",
		Group:       "Go 智能体",
		Description: "互助论坛智能体最近一次模型轮次的轮次、工具调用数与错误（不含内部 api_key）",
		Export: func(includeImages bool) (any, error) {
			return map[string]any{
				"kind":         "mutual-forum-agent-run",
				"generated_at": time.Now().Format(time.RFC3339),
				"last_run":     snapshotLastRun(),
			}, nil
		},
	})
}
