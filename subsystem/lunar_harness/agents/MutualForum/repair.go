package MutualForum

// repair.go 向拯救者注册互助论坛的上下文修复能力。

import (
	"fmt"

	"LunarSubsystem/LunarHarness/engine/agentrepair"
	"LunarSubsystem/LunarHarness/engine/console/logger"
)

func init() {
	agentrepair.Register(agentrepair.Handler{
		ID:          "mutual_forum",
		Label:       "互助论坛",
		Description: "互助论坛独立上下文（每轮需求与答复）",
		Actions:     []string{"trim", "reset"},
		Inspect:     inspectForumContext,
		Repair:      repairForumContext,
	})
}

// inspectForumContext 读取论坛上下文；limit<=0 表示全部。
func inspectForumContext(limit int) (any, error) {
	messages := historySnapshot()
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
		keep := 8
		if value, ok := params["keep_last"].(int); ok && value > 0 {
			keep = value
		}
		historyMu.Lock()
		if len(history) > keep {
			history = append([]chatMessage(nil), history[len(history)-keep:]...)
		}
		remaining := len(history)
		historyMu.Unlock()
		logger.Warn(roleLabel, "[拯救] 论坛上下文已裁剪为最近 %d 条", remaining)
		return fmt.Sprintf("已把互助论坛上下文裁剪为最近 %d 条消息", remaining), nil
	case "reset":
		ResetContext()
		logger.Warn(roleLabel, "[拯救] 论坛上下文已清空")
		return "已清空互助论坛上下文", nil
	default:
		return "", fmt.Errorf("互助论坛不支持动作：%s", action)
	}
}
