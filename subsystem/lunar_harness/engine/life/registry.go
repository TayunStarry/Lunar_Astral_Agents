package life

import (
	"math/rand"
	"time"

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

// SetLastUserNameProvider 注入最新用户发言的发送者名读取能力（由 lunar 包提供）。
func SetLastUserNameProvider(provider func() string) {
	mu.Lock()
	defer mu.Unlock()
	lastUserNameHook = provider
}

// SetLastReplyTextProvider 注入上一轮月华回应读取能力（由 lunar 包提供）。
func SetLastReplyTextProvider(provider func() string) {
	mu.Lock()
	defer mu.Unlock()
	lastReplyTextHook = provider
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
		newForumEvent(),
		newWatchEvent(),
		newMemoryEvent(),
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
func pickLocked() Event {
	available := make([]Event, 0, len(registry))
	for _, event := range registry {
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
	r := rand.Intn(total)
	for _, event := range available {
		r -= event.Weight()
		if r < 0 {
			return event
		}
	}
	return available[len(available)-1]
}

func envFromStateLocked(now time.Time) Env {
	env := Env{Now: now, LastRecord: state.LastRecord, MoodLabel: mood.Snapshot().Label}
	if lastUserTextHook != nil {
		env.LastUserText = lastUserTextHook()
	}
	return env
}
