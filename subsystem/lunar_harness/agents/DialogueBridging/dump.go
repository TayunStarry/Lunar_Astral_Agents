package DialogueBridging

// dump.go 通讯桥接的导出：最近一次多目标投递计划与执行报告。

import (
	"time"

	"LunarSubsystem/LunarHarness/protocol/agentdump"
)

// exportContext 通讯桥接的上下文快照；dump 导出与智能修复的 Inspect 共用这一份。
func exportContext(includeImages bool) (any, error) {
	lastMu.Lock()
	record := lastDelivery
	lastMu.Unlock()
	if record == nil {
		return map[string]any{
			"kind":          "dialogue-bridging-context",
			"generated_at":  time.Now().Format(time.RFC3339),
			"message_count": 0,
			"note":          "本次进程内还没有执行过通讯桥接投递",
		}, nil
	}
	return map[string]any{
		"kind":          "dialogue-bridging-context",
		"generated_at":  time.Now().Format(time.RFC3339),
		"message_count": len(record.Deliveries),
		"last_delivery": record,
	}, nil
}

func init() {
	agentdump.Register(agentdump.Provider{
		ID:          "dialogue_bridging",
		Label:       roleLabel,
		Group:       "Go 智能体",
		Description: "最近一次自然语言需求、多目标投递清单与逐项执行报告",
		Export:      exportContext,
	})
}
