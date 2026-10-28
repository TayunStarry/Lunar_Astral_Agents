package BilibiliViewer

// dump.go 哔哩观影的导出：选片偏好、已看记录规模与记忆库条目。

import (
	"time"

	"LunarSubsystem/LunarHarness/engine/agentdump"
)

func init() {
	agentdump.Register(agentdump.Provider{
		ID:          "bilibili",
		Label:       "哔哩观影",
		Group:       "Go 智能体",
		Description: "哔哩观影的选片配置、已看视频规模与统一记忆库中的 [B站视频] 条目",
		Export: func(includeImages bool) (any, error) {
			cfg := LoadConfig()
			entries := MemoryEntries()
			return map[string]any{
				"kind":           "bilibili-viewer-context",
				"generated_at":   time.Now().Format(time.RFC3339),
				"message_count":  0,
				"config":         cfg,
				"seen_videos":    len(seenSnapshot()),
				"memory_entries": entries,
				"memory_prefix":  memoryTagPrefix,
			}, nil
		},
	})
}
