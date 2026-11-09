package wishchain

// state.go 心愿链路单一持久化文件的读写。
//
// 所有愿望、心情、态度与联动记录都写在同一个文件里，避免数据碎片化。

import (
	"encoding/json"
	"time"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/persistence"
)

// Reset 清空内存缓存（测试与维护入口）；不会删除磁盘文件。
func Reset() {
	mu.Lock()
	state = State{}
	loaded = false
	initialized = false
	lastMoodWindowKey = ""
	mu.Unlock()
}

// Load 把磁盘状态载入内存（不计算嵌入锚点）；可重复调用。
func Load() {
	mu.Lock()
	defer mu.Unlock()
	ensureLoadedLocked()
}

// readState 读取统一持久化文件；不存在或损坏时返回 false。
func readState() (State, bool) {
	raw, ok := persistence.ReadFileBytes(stateFile)
	if !ok || len(raw) == 0 {
		return State{}, false
	}
	var saved State
	if err := json.Unmarshal(raw, &saved); err != nil {
		logger.Warn(Module, "[心愿] 状态文件损坏，按空状态继续: %v", err)
		return State{}, false
	}
	return saved, true
}

// ensureLoadedLocked 确保磁盘状态已载入内存；调用方需持有锁。
func ensureLoadedLocked() {
	if loaded {
		return
	}
	loaded = true
	saved, ok := readState()
	if !ok {
		state = State{Version: stateVersion}
		state.Wishes = nil
		return
	}
	if saved.Version == 0 {
		saved.Version = stateVersion
	}
	if saved.Wishes == nil {
		saved.Wishes = []Wish{}
	}
	if saved.Attitude.Users == nil {
		saved.Attitude.Users = map[string]UserAffection{}
	}
	// 标签与权重不一致时按权重校准，避免旧文件把错误标签带进来。
	if len(saved.Mood.Weights) > 0 {
		if corrected := dominantLabel(saved.Mood.Weights, labels, defaultLabel); corrected != saved.Mood.Label {
			saved.Mood.Label = corrected
		}
	}
	state = saved
	if anchorsReady(saved) {
		initialized = true
	}
}

// anchorsReady 判断持久化状态里的心情 / 态度锚点是否可以复用。
func anchorsReady(saved State) bool {
	if saved.Model != config.AgentEmbeddingModel || saved.Dim <= 0 {
		return false
	}
	if len(saved.Mood.Vector) != saved.Dim {
		return false
	}
	for _, label := range labels {
		if len(saved.Mood.Base[label]) != saved.Dim {
			return false
		}
	}
	for _, name := range attitudeAnchorNames() {
		if len(saved.Attitude.Anchors[name]) != saved.Dim {
			return false
		}
	}
	return true
}

// saveLocked 原子写入统一状态；调用方需持有锁。
func saveLocked() {
	state.Version = stateVersion
	state.UpdatedAt = time.Now().Unix()
	raw, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		logger.Warn(Module, "[心愿] 状态序列化失败: %v", err)
		return
	}
	if err := persistence.WriteFileAtomic(stateFile, raw); err != nil {
		logger.Warn(Module, "[心愿] 状态写入失败: %v", err)
	}
}

// attitudeAnchorNames 态度锚点名称集合（正向后负向）。
func attitudeAnchorNames() []string {
	names := make([]string, 0, len(positiveLabels)+len(negativeLabels))
	names = append(names, positiveLabels...)
	names = append(names, negativeLabels...)
	return names
}

// attitudeAnchorTexts 态度锚点对应的语义文本。
func attitudeAnchorTexts(names []string) []string {
	texts := make([]string, 0, len(names))
	for _, name := range names {
		texts = append(texts, anchorTexts[name])
	}
	return texts
}
