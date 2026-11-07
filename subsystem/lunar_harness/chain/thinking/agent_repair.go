package thinking

// agent_repair.go 把对话者的上下文以只读方式交给维修侧诊断。
//
// 维修侧只通过 protocol/agentrepair 注册表读取对话者上下文（未读队列 + 已实例化角色），
// 不再具备裁剪、剔除、压缩、清空等改动上下文的能力，诊断结果仅用于生成维修建议。

import (
	"fmt"

	"LunarSubsystem/LunarHarness/agents/Dialogue"
	"LunarSubsystem/LunarHarness/protocol/agentrepair"
)

func init() {
	agentrepair.RegisterReadonly("dialogue", Dialogue.Role,
		Dialogue.Role+"与月华主上下文（未读队列 + 已实例化角色的消息，只读）",
		inspectCoreContext)
}

// inspectCoreContext 读取对话者上下文；limit<=0 表示全部。
func inspectCoreContext(limit int) (any, error) {
	if !IsReady() {
		return nil, fmt.Errorf("月华智能体尚未就绪，暂时无法读取上下文")
	}
	unreadMu.Lock()
	unread := append([]PostMessage(nil), unreadContext...)
	unreadMu.Unlock()
	out := map[string]any{
		"lunar": map[string]any{
			"label":         "月华主上下文（未读队列）",
			"message_count": len(unread),
			"messages":      dumpMessages(tailMessages(unread, limit), false),
		},
	}
	for _, role := range coreRoleSnapshots() {
		out[role.ID] = map[string]any{
			"label":         role.Label,
			"message_count": len(role.Builder.Messages),
			"messages":      dumpMessages(tailMessages(role.Builder.Messages, limit), false),
		}
	}
	return out, nil
}

// coreRoleSnapshots 返回当前已实例化的对话者角色。
func coreRoleSnapshots() []coreRoleSnapshot {
	roles := make([]coreRoleSnapshot, 0, 1)
	if dialogueRole != nil && dialogueRole.ModelBuilder != nil {
		roles = append(roles, coreRoleSnapshot{ID: "dialogue", Label: Dialogue.Role, Builder: dialogueRole.ModelBuilder})
	}
	return roles
}

// tailMessages 取最后 limit 条；limit<=0 返回原切片。
func tailMessages(messages []PostMessage, limit int) []PostMessage {
	if limit <= 0 || len(messages) <= limit {
		return messages
	}
	return messages[len(messages)-limit:]
}
