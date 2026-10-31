package LunarPersonality

import (
	"strings"
	"testing"
	"time"
)

func TestUpcomingPlanSummaryWithinOneHour(t *testing.T) {
	scheduleMu.Lock()
	old := scheduleCache
	scheduleCache = []ScheduleItem{
		{ID: "soon", Type: "daily", Time: "10:30", Content: "喝水"},
		{ID: "later", Type: "daily", Time: "12:00", Content: "午饭"},
		{ID: "done", Type: "daily", Time: "10:10", Content: "已做", CompletedDate: "2026-10-28"},
	}
	scheduleMu.Unlock()
	t.Cleanup(func() {
		scheduleMu.Lock()
		scheduleCache = old
		scheduleMu.Unlock()
	})

	now := time.Date(2026, 10, 28, 10, 0, 0, 0, time.Local)
	summary := upcomingPlanSummary(now, time.Hour)
	if !strings.Contains(summary, "10:30 喝水") {
		t.Fatalf("应包含未来一小时内计划，实际 %q", summary)
	}
	if strings.Contains(summary, "午饭") || strings.Contains(summary, "已做") {
		t.Fatalf("不应包含超过窗口或已完成计划，实际 %q", summary)
	}
}

func TestUpcomingPlanSummaryEmpty(t *testing.T) {
	scheduleMu.Lock()
	old := scheduleCache
	scheduleCache = nil
	scheduleMu.Unlock()
	t.Cleanup(func() {
		scheduleMu.Lock()
		scheduleCache = old
		scheduleMu.Unlock()
	})
	if summary := upcomingPlanSummary(time.Now(), time.Hour); summary != "" {
		t.Fatalf("没有计划时应返回空串，实际 %q", summary)
	}
}
