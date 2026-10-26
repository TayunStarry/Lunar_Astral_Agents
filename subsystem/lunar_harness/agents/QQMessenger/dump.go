package QQMessenger

// dump.go 专项通讯的导出：最近一次多目标投递计划与执行报告。

import (
	"time"

	"LunarSubsystem/LunarHarness/engine/agentdump"
)

func init() {
	agentdump.Register(agentdump.Provider{
		ID:          "qq",
		Label:       "专项通讯",
		Group:       "Go 智能体",
		Description: "最近一次自然语言需求、多目标投递清单与逐项执行报告",
		Export: func(includeImages bool) (any, error) {
			lastMu.Lock()
			record := lastDelivery
			lastMu.Unlock()
			if record == nil {
				return map[string]any{
					"kind":          "qq-messenger-context",
					"generated_at":  time.Now().Format(time.RFC3339),
					"message_count": 0,
					"note":          "本次进程内还没有执行过专项通讯投递",
				}, nil
			}
			return map[string]any{
				"kind":          "qq-messenger-context",
				"generated_at":  time.Now().Format(time.RFC3339),
				"message_count": len(record.Deliveries),
				"last_delivery": record,
			}, nil
		},
	})
}
