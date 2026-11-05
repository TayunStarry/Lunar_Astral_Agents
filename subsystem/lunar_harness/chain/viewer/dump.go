package viewer

// dump.go 哔哩观影的导出：选片配置与已看视频规模。
//
// 观影记忆已经迁出统一记忆库，不再出现在 dump 里。

import (
	"time"

	"LunarSubsystem/LunarHarness/protocol/agentdump"
)

func init() {
	agentdump.Register(agentdump.Provider{
		ID:          "bilibili",
		Label:       "哔哩观影",
		Group:       "Go 智能体",
		Description: "哔哩观影的选片配置与已看视频规模",
		Export: func(includeImages bool) (any, error) {
			cfg := LoadConfig()
			return map[string]any{
				"kind":          "bilibili-viewer-context",
				"generated_at":  time.Now().Format(time.RFC3339),
				"message_count": 0,
				"config":        cfg,
				"seen_videos":   len(seenSnapshot()),
			}, nil
		},
	})
}
