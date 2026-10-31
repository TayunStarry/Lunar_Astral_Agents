package mood

import (
	"encoding/json"
	"time"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/persistence"
)

// loadState 读取持久化心情；不满足当前嵌入模型时返回 false。
func loadState() bool {
	raw, ok := persistence.ReadFileBytes(stateFile)
	migrated := false
	if !ok {
		return false
	}
	var saved State
	if err := json.Unmarshal(raw, &saved); err != nil {
		logger.Warn(moodModule, "[心情] 状态文件损坏，将重新初始化: %v", err)
		return false
	}
	if saved.Model != config.AgentEmbeddingModel || saved.Dim <= 0 || len(saved.Vector) != saved.Dim {
		logger.Info(moodModule, "[心情] 嵌入模型或向量维度已变化，重新初始化")
		return false
	}
	for _, label := range labels {
		if len(saved.Base[label]) != saved.Dim {
			logger.Info(moodModule, "[心情] 基础情绪向量缺失或维度不一致，重新初始化")
			return false
		}
	}
	if len(saved.Weights) > 0 {
		if corrected := dominantLabel(saved.Weights); corrected != saved.Label {
			logger.Info(moodModule, "[心情] 标签与权重不一致，已校准：%s → %s", saved.Label, corrected)
			saved.Label = corrected
			migrated = true
		}
	}
	mu.Lock()
	state = saved
	initialized = true
	if migrated {
		saveStateLocked()
	}
	mu.Unlock()
	return true
}

// saveStateLocked 持久化当前状态；调用方需持有写锁。
func saveStateLocked() {
	state.Version = 1
	state.UpdatedAt = time.Now().Unix()
	raw, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		logger.Warn(moodModule, "[心情] 状态序列化失败: %v", err)
		return
	}
	if err := persistence.WriteFileAtomic(stateFile, raw); err != nil {
		logger.Warn(moodModule, "[心情] 状态写入失败: %v", err)
	}
}
