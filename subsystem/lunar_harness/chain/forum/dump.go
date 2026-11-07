package forum

// dump.go 论坛链上下文的导出与只读诊断登记。
//
// 跨轮上下文归论坛链所有，因此 agentdump / agentrepair 也在这里注册，
// 保持原标识 mutual_forum，控制台与维修侧无需感知分层变化。

import (
	"time"

	"LunarSubsystem/LunarHarness/protocol/agentdump"
	"LunarSubsystem/LunarHarness/protocol/agentrepair"
)

func init() {
	agentdump.Register(agentdump.Provider{
		ID:          "mutual_forum",
		Label:       "论坛协作",
		Group:       "逻辑链",
		Description: "互助论坛链的最近需求/答复上下文（不含内部 api_key）",
		Export: func(includeImages bool) (any, error) {
			messages := HistorySnapshot()
			return map[string]any{
				"kind":          "mutual-forum-context",
				"generated_at":  time.Now().Format(time.RFC3339),
				"message_count": len(messages),
				"messages":      messages,
			}, nil
		},
	})
	agentrepair.Register(agentrepair.Handler{
		ID:          "mutual_forum",
		Label:       "论坛协作",
		Description: "互助论坛链的独立上下文（每轮需求与答复，只读）",
		Inspect:     inspectForumContext,
	})
}

// inspectForumContext 读取论坛上下文；limit<=0 表示全部。
func inspectForumContext(limit int) (any, error) {
	messages := HistorySnapshot()
	if limit > 0 && len(messages) > limit {
		messages = messages[len(messages)-limit:]
	}
	return map[string]any{
		"message_count": len(messages),
		"messages":      messages,
	}, nil
}
