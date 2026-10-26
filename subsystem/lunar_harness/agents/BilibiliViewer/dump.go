package BilibiliViewer

// dump.go 哔哩观影的导出：选片偏好、已看记录规模与当前日记记忆。

import (
	"time"

	"LunarSubsystem/LunarHarness/engine/agentdump"
)

func init() {
	agentdump.Register(agentdump.Provider{
		ID:          "bilibili",
		Label:       "哔哩观影",
		Group:       "Go 智能体",
		Description: "哔哩观影的选片配置、已看视频规模与日记/历史记忆",
		Export: func(includeImages bool) (any, error) {
			cfg := LoadConfig()
			diary := loadDiarySections()
			seen := seenSnapshot()
			return map[string]any{
				"kind":          "bilibili-viewer-context",
				"generated_at":  time.Now().Format(time.RFC3339),
				"message_count": 0,
				"config":        cfg,
				"seen_videos":   len(seen),
				"diary": map[string]any{
					"dynamic": diary.Dynamic,
					"memory":  diary.Memory,
					"videos":  diary.Videos,
					"legacy":  diary.Legacy,
				},
			}, nil
		},
	})
}
