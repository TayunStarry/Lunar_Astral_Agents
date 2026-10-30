package Responder

// dump.go 通用应答（Responder）的数据导出：队列与运行标记。
//
// 通用应答按请求即时推理、不保存会话上下文，故导出的是调度状态而非消息历史。

import (
	"time"

	"LunarSubsystem/LunarHarness/engine/agentdump"
)

func init() {
	agentdump.Register(agentdump.Provider{
		ID:          "responder",
		Label:       "通用应答",
		Group:       "Go 智能体",
		Description: "通用应答的队列与运行状态（待处理数、特许运行、忙碌标记；不保留上下文）",
		Export: func(includeImages bool) (any, error) {
			_ = includeImages
			pending := Pending()
			return map[string]any{
				"kind":           "responder-context",
				"generated_at":   time.Now().Format(time.RFC3339),
				"queue_pending":  pending,
				"queue_capacity": queueCapacity,
				"permitted":      Permitted(),
				"busy":           Busy(),
				"message_count":  pending,
			}, nil
		},
	})
}
