package plans

// due.go 计划到期消费与未来窗口摘要，仅供生活体系调用。

import (
	"sort"
	"strconv"
	"strings"
	"time"
)

// ConsumeDue 取出到期计划项并推进计划表；返回本次触发的计划项。
func ConsumeDue() []Item {
	mu.Lock()
	defer mu.Unlock()
	if len(items) == 0 {
		return nil
	}
	now := time.Now()
	today := formatDate(now)
	kept := make([]Item, 0, len(items))
	removed := make([]Item, 0)
	type candidate struct {
		item    *Item
		dueTime time.Time
	}
	candidates := make([]candidate, 0)
	for index := range items {
		item := &items[index]
		if item.Type == "daily" {
			due, ok := dailyTaskTime(*item, now)
			if ok && !now.Before(due) && item.CompletedDate != today {
				if now.Sub(due) > 30*time.Minute {
					item.CompletedDate = today
				} else {
					candidates = append(candidates, candidate{item: item, dueTime: due})
				}
			}
			kept = append(kept, *item)
			continue
		}
		due, err := time.ParseInLocation("2006-01-02T15:04:05", item.Time, time.Local)
		if err != nil {
			kept = append(kept, *item)
			continue
		}
		if now.Before(due) {
			kept = append(kept, *item)
			continue
		}
		if now.Sub(due) > 30*time.Minute {
			removed = append(removed, *item)
			continue
		}
		candidates = append(candidates, candidate{item: item, dueTime: due})
	}
	executed := make([]Item, 0)
	if len(candidates) > 0 {
		best := 0
		for i := 1; i < len(candidates); i++ {
			a := absDuration(now.Sub(candidates[i].dueTime))
			b := absDuration(now.Sub(candidates[best].dueTime))
			if a < b {
				best = i
			}
		}
		chosen := candidates[best].item
		if chosen.Type == "daily" {
			chosen.CompletedDate = today
			for i := range kept {
				if kept[i].ID == chosen.ID {
					kept[i] = *chosen
				}
			}
		} else {
			removed = append(removed, *chosen)
		}
		executed = append(executed, *chosen)
	}
	if len(executed) > 0 || len(removed) > 0 {
		items = kept
		_ = saveLocked()
	}
	return executed
}

// Upcoming 未来 window 内即将触发的计划摘要；没有则返回空串。
func Upcoming(now time.Time, window time.Duration) string {
	mu.Lock()
	snapshot := append([]Item(nil), items...)
	mu.Unlock()
	type upcoming struct {
		due     time.Time
		content string
	}
	list := make([]upcoming, 0, len(snapshot))
	for _, item := range snapshot {
		due, ok := nextPlanTime(item, now)
		if !ok {
			continue
		}
		if !due.After(now) || due.After(now.Add(window)) {
			continue
		}
		list = append(list, upcoming{due: due, content: strings.TrimSpace(item.Content)})
	}
	if len(list) == 0 {
		return ""
	}
	sort.SliceStable(list, func(i, j int) bool { return list[i].due.Before(list[j].due) })
	parts := make([]string, 0, len(list))
	for _, item := range list {
		parts = append(parts, item.due.Format("15:04")+" "+truncateRunes(item.content, 40))
	}
	return strings.Join(parts, "；")
}

// nextPlanTime 计算计划项的下一次触发时刻；无法计算或已过期返回 false。
func nextPlanTime(item Item, now time.Time) (time.Time, bool) {
	if item.Type == "daily" {
		due, ok := dailyTaskTime(item, now)
		if !ok {
			return time.Time{}, false
		}
		if item.CompletedDate == formatDate(now) || !due.After(now) {
			due = due.Add(24 * time.Hour)
		}
		return due, true
	}
	due, err := time.ParseInLocation("2006-01-02T15:04:05", item.Time, time.Local)
	if err != nil || !due.After(now) {
		return time.Time{}, false
	}
	return due, true
}

// dailyTaskTime 每日计划的当日触发时刻。
func dailyTaskTime(item Item, now time.Time) (time.Time, bool) {
	match := dailyRegex.FindStringSubmatch(strings.TrimSpace(item.Time))
	if match == nil {
		return time.Time{}, false
	}
	hour, _ := strconv.Atoi(match[1])
	minute, _ := strconv.Atoi(match[2])
	return time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, time.Local), true
}

func absDuration(value time.Duration) time.Duration {
	if value < 0 {
		return -value
	}
	return value
}

// truncateRunes 按字符截断并带省略号。
func truncateRunes(text string, limit int) string {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) <= limit {
		return string(runes)
	}
	return string(runes[:limit]) + "…"
}
