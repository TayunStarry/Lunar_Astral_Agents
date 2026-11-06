package life

import (
	"time"

	"LunarSubsystem/LunarHarness/engine/affection"
	"LunarSubsystem/LunarHarness/engine/mood"
)

// SetPlanHooks 注入计划表判定能力（由 lunar 包提供，避免包循环依赖）。
func SetPlanHooks(due func() int, upcoming func(time.Time, time.Duration) string) {
	mu.Lock()
	defer mu.Unlock()
	planDueHook = due
	planUpcomingHook = upcoming
}

// SetLastUserTextProvider 注入最新用户发言读取能力（由 lunar 包提供）。
func SetLastUserTextProvider(provider func() string) {
	mu.Lock()
	defer mu.Unlock()
	lastUserTextHook = provider
}

// SetDialogueRoundsProvider 注入各角色最近若干轮对话读取能力（由 lunar 包提供）。
func SetDialogueRoundsProvider(provider func() []affection.Round) {
	mu.Lock()
	defer mu.Unlock()
	dialogueRoundsHook = provider
}

// RegisterEvent 注册生活事件；重复 ID 忽略。
func RegisterEvent(event Event) {
	if event == nil || event.ID() == "" {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	for _, item := range registry {
		if item.ID() == event.ID() {
			return
		}
	}
	registry = append(registry, event)
}

func registerBuiltinLocked() {
	builtins := []Event{
		newDazeEvent(),
		newWatchEvent(),
		newForumEvent(),
		newCreativeEvent(),
	}
	for _, event := range builtins {
		if eventByIDLocked(event.ID()) != nil {
			continue
		}
		registry = append(registry, event)
	}
}

func eventByID(id string) Event {
	mu.Lock()
	defer mu.Unlock()
	return eventByIDLocked(id)
}

func eventByIDLocked(id string) Event {
	for _, event := range registry {
		if event.ID() == id {
			return event
		}
	}
	return nil
}

// pickLocked 按权重选择事件；调用方需持有锁。
//
// 权重 <= 0 的事件视为「自动触发已关闭」：不参与随机挑选，但仍可通过
// RequestEvent 手动/强制触发。记忆整理不在此列，它是固定步骤，不注册为事件。
func pickLocked() Event {
	available := make([]Event, 0, len(registry))
	for _, event := range registry {
		if event.Weight() <= 0 {
			continue
		}
		if usedLocked(event.ID()) < event.DailyQuota() {
			available = append(available, event)
		}
	}
	if len(available) == 0 {
		return eventByIDLocked("daze")
	}
	total := 0
	for _, event := range available {
		total += event.Weight()
	}
	if total <= 0 {
		return available[0]
	}
	r := pickRandHook(total)
	for _, event := range available {
		r -= event.Weight()
		if r < 0 {
			return event
		}
	}
	return available[len(available)-1]
}

func envFromStateLocked(now time.Time) Env {
	// 注意：这里不再注入最新用户发言。生活事件（尤其创作）需要对话内容时，
	// 统一通过 life_planning 写入的创作参考字符串获取，不从对话历史截取。
	return Env{Now: now, LastRecord: state.LastRecord, MoodLabel: mood.Snapshot().Label}
}
