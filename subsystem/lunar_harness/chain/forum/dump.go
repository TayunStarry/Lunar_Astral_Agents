package forum

// dump.go 互助论坛的导出与只读诊断登记：链与智能体共用唯一目标 mutual_forum。
//
// 故障归属只认一个 ID：智能体在 agents/MutualForum 以 agentbase.ReportRun("mutual_forum", …)
// 上报，装配层 app/tui_hooks.go 的 roleAgentIDs 把「互助论坛」映射到 mutual_forum，
// protocol/agentinfo.DisplayNames 与控制台导出（13 号项）也按同一 ID 取数。
// 因此两侧数据放在同一目标里，不再单列 mutual_forum_agent：
//   - 链侧：最近若干轮「需求 + 答复」上下文（本包持有）；
//   - 智能体侧：最近一次模型轮次的轮次 / 工具调用数 / 错误（MutualForum.LastRunSnapshot）。

import (
	"time"

	"LunarSubsystem/LunarHarness/agents/MutualForum"
	"LunarSubsystem/LunarHarness/protocol/agentdump"
	"LunarSubsystem/LunarHarness/protocol/agentrepair"
)

func init() {
	agentdump.Register(agentdump.Provider{
		ID:          "mutual_forum",
		Label:       "论坛协作",
		Group:       "逻辑链",
		Description: "互助论坛链的需求/答复上下文与智能体最近一次模型轮次（不含内部 api_key）",
		Export: func(includeImages bool) (any, error) {
			return forumSnapshot(0), nil
		},
	})
	agentrepair.Register(agentrepair.Handler{
		ID:          "mutual_forum",
		Label:       "论坛协作",
		Description: "互助论坛链的独立上下文与智能体最近一次模型轮次（只读）",
		Inspect:     inspectForumContext,
	})
}

// forumSnapshot 汇总链侧上下文与智能体侧最近一次模型轮次；导出与 Inspect 共用这一份。
// limit<=0 表示保留全部消息。
func forumSnapshot(limit int) map[string]any {
	messages := HistorySnapshot()
	if limit > 0 && len(messages) > limit {
		messages = messages[len(messages)-limit:]
	}
	return map[string]any{
		"kind":          "mutual-forum-context",
		"generated_at":  time.Now().Format(time.RFC3339),
		"message_count": len(messages),
		"messages":      messages,
		"last_run":      MutualForum.LastRunSnapshot(),
	}
}

// inspectForumContext 读取论坛上下文；limit<=0 表示全部。
func inspectForumContext(limit int) (any, error) {
	return forumSnapshot(limit), nil
}
