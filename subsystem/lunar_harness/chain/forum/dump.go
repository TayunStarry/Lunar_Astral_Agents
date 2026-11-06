package forum

// dump.go 论坛链上下文的导出与修复。
//
// 跨轮上下文现在归论坛链所有，因此 agentdump / agentrepair 也在这里注册，
// 保持原标识 mutual_forum，控制台与拯救者无需感知分层变化。

import (
	"context"
	"fmt"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/protocol/agentbase"
	"LunarSubsystem/LunarHarness/protocol/agentdump"
	"LunarSubsystem/LunarHarness/protocol/agentrepair"
)

func init() {
	agentdump.Register(agentdump.Provider{
		ID:          "mutual_forum",
		Label:       "互助论坛",
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
		Label:       "互助论坛",
		Description: "互助论坛链的独立上下文（每轮需求与答复）",
		Actions:     []string{"trim", "summarize", "reset"},
		Inspect:     inspectForumContext,
		Verify:      verifyForumContext,
		Repair:      repairForumContext,
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

// repairForumContext 执行一次修复动作。
func repairForumContext(action string, params map[string]any) (string, error) {
	switch action {
	case "trim":
		keep := agentrepair.IntParam(params, "keep_last", 8, 1, 200)
		historyMu.Lock()
		if len(history) > keep {
			history = append([]agentbase.Message(nil), history[len(history)-keep:]...)
		}
		remaining := len(history)
		historyMu.Unlock()
		logger.Warn("互助论坛", "[拯救] 论坛上下文已裁剪为最近 %d 条", remaining)
		return fmt.Sprintf("已把互助论坛上下文裁剪为最近 %d 条消息", remaining), nil
	case "summarize":
		keep := agentrepair.IntParam(params, "keep_last", 8, 2, 40)
		target := agentrepair.IntParam(params, "target_runes", 2000, 500, 8000)
		return summarizeForumContext(keep, target)
	case "reset":
		ResetContext()
		logger.Warn("互助论坛", "[拯救] 论坛上下文已清空")
		return "已清空互助论坛上下文", nil
	default:
		return "", fmt.Errorf("互助论坛不支持动作：%s", action)
	}
}

// verifyForumContext 读回论坛上下文状态，确认修复后仍可用。
func verifyForumContext(_ context.Context, params map[string]any) (any, error) {
	_ = params
	messages := HistorySnapshot()
	hasSummary := len(messages) > 0 && strings.Contains(messageText(messages[0]), "[论坛历史摘要]")
	return map[string]any{
		"verified":      true,
		"mode":          "state_readback",
		"message_count": len(messages),
		"has_summary":   hasSummary,
		"reason":        "已读回互助论坛上下文，历史可正常读取",
	}, nil
}
