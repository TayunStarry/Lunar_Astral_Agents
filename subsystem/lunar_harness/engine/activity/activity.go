package activity

// activity.go 月华「刚刚在做的事情」状态：唯一状态源、线程安全、JSON 持久化。
//
// 业务链条只需一行调用即可写入，例如：
//
//	activity.Set("在整理今天的观影笔记")
//
// 运行时消息通过 Line() 读取，固定渲染为「我刚刚在干的事情：……」。

import (
	"encoding/json"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/persistence"
)

const module = "Activity"

// Set 写入当前活动描述并立即持久化；空白描述回退为默认描述。
// 典型用法：生活/观影/论坛/创作链条进入某个阶段时调用 activity.Set("在看《xxx》")。
func Set(text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		text = defaultText
	}
	ensureLoaded()
	mu.Lock()
	state.Text = text
	state.Version = stateVersion
	state.UpdatedAt = time.Now().Unix()
	saveLocked()
	mu.Unlock()
}

// Current 返回当前活动描述；未设置时返回默认描述。
func Current() string {
	ensureLoaded()
	mu.RLock()
	defer mu.RUnlock()
	if strings.TrimSpace(state.Text) == "" {
		return defaultText
	}
	return state.Text
}

// Line 渲染运行时消息里的固定字段；每轮调用即可拿到最新值。
func Line() string {
	return fieldPrefix + Current()
}

// ensureLoaded 首次访问时从磁盘恢复；并发首访由二次检查兜住。
func ensureLoaded() {
	mu.RLock()
	ready := loaded
	mu.RUnlock()
	if ready {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	if loaded {
		return
	}
	loadLocked()
	loaded = true
}

// loadLocked 读取持久化状态；调用方需持有写锁。
func loadLocked() {
	raw, ok := persistence.ReadFileBytes(stateFile)
	if !ok {
		return
	}
	var saved State
	if err := json.Unmarshal(raw, &saved); err != nil {
		logger.Warn(module, "[活动] 状态文件损坏，使用默认描述: %v", err)
		return
	}
	state = saved
}

// saveLocked 持久化当前状态；调用方需持有写锁。
func saveLocked() {
	raw, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		logger.Warn(module, "[活动] 状态序列化失败: %v", err)
		return
	}
	if err := persistence.WriteFileAtomic(stateFile, raw); err != nil {
		logger.Warn(module, "[活动] 状态写入失败: %v", err)
	}
}
