package DialogueBridging

// dump.go 通讯桥接的导出：最近一次多目标投递计划与执行报告。

import (
	"time"

	"LunarSubsystem/LunarHarness/engine/agentdump"
)

func init() {
	agentdump.Register(agentdump.Provider{
		ID:          "dialogue_bridging",
		Label:       "通讯桥接",
		Group:       "Go 智能体",
		Description: "最近一次自然语言需求、多目标投递清单与逐项执行报告",
		Export: func(includeImages bool) (any, error) {
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
		},
	})
}
