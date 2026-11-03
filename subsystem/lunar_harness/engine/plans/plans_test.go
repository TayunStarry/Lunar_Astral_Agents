package plans

// plans_test.go 计划表存储与查询验收：时间归一化、增删改查、到期消费与未来摘要。

import (
	"strings"
	"testing"
	"time"

	"LunarSubsystem/LunarHarness/internal/testenv"
)

// setItems 替换内存计划表（测试辅助）。
func setItems(t *testing.T, value []Item) {
	t.Helper()
	mu.Lock()
	items = value
	mu.Unlock()
	t.Cleanup(func() {
		mu.Lock()
		items = nil
		mu.Unlock()
	})
}

// TestNormalizeTime 时间格式归一化。
func TestNormalizeTime(t *testing.T) {
	cases := map[string]string{
		"2026年6月14日 15:30":    "2026-06-14T15:30:00",
		"2026-06-14T15:30:00": "2026-06-14T15:30:00",
		"不是时间":                "",
	}
	for input, want := range cases {
		if got := NormalizeTime(input); got != want {
			t.Fatalf("NormalizeTime(%q) = %q，期望 %q", input, got, want)
		}
	}
}

// TestCreateUpdateDelete 计划表增删改查。
func TestCreateUpdateDelete(t *testing.T) {
	testenv.Isolate(t)
	setItems(t, nil)

	item, err := Create("2026-06-14 15:30", "整理文档")
	if err != nil {
		t.Fatalf("新增失败: %v", err)
	}
	if item.Time != "2026-06-14T15:30:00" || len(List()) != 1 {
		t.Fatalf("新增结果不正确: %+v", item)
	}
	updated, err := Update(item.ID, "2026-06-14 16:00", "整理并归档")
	if err != nil || updated.Content != "整理并归档" {
		t.Fatalf("改写失败: %+v %v", updated, err)
	}
	if _, err := Update("missing", "", "x"); err == nil {
		t.Fatal("改写不存在的计划项应失败")
	}
	deleted, err := Delete(item.ID)
	if err != nil || deleted.ID != item.ID {
		t.Fatalf("删除失败: %+v %v", deleted, err)
	}
	if len(List()) != 0 {
		t.Fatalf("删除后计划表应为空: %+v", List())
	}
	if _, err := Delete(item.ID); err == nil {
		t.Fatal("删除不存在的计划项应失败")
	}
}

// TestQueryByDateAndKeyword 按日期与关键词查询。
func TestQueryByDateAndKeyword(t *testing.T) {
	testenv.Isolate(t)
	setItems(t, []Item{
		{ID: "one", Time: "2026-06-14T15:30:00", Content: "整理文档"},
		{ID: "two", Time: "2026-06-15T09:00:00", Content: "开会"},
	})
	text, err := Query([]string{"文档"}, "2026-06-14", "day")
	if err != nil || !strings.Contains(text, "整理文档") || strings.Contains(text, "开会") {
		t.Fatalf("查询结果不正确: %q %v", text, err)
	}
	text, err = Query(nil, "", "all")
	if err != nil || !strings.Contains(text, "整理文档") || !strings.Contains(text, "开会") {
		t.Fatalf("全部范围查询结果不正确: %q %v", text, err)
	}
}

// TestUpcomingSummary 未来窗口内的计划摘要。
func TestUpcomingSummary(t *testing.T) {
	testenv.Isolate(t)
	now := time.Date(2026, 6, 14, 15, 0, 0, 0, time.Local)
	setItems(t, []Item{{ID: "next", Time: "2026-06-14T15:30:00", Content: "喝水"}})
	summary := Upcoming(now, time.Hour)
	if !strings.Contains(summary, "喝水") || !strings.Contains(summary, "15:30") {
		t.Fatalf("未来摘要不正确: %q", summary)
	}
	setItems(t, nil)
	if summary := Upcoming(now, time.Hour); summary != "" {
		t.Fatalf("空计划表不应有摘要: %q", summary)
	}
}

// TestConsumeDue 到期计划项只消费一次。
func TestConsumeDue(t *testing.T) {
	testenv.Isolate(t)
	now := time.Now()
	setItems(t, []Item{{
		ID: "due", Time: now.Add(-time.Minute).Format("2006-01-02T15:04:05"), Content: "到期任务",
	}})
	due := ConsumeDue()
	if len(due) != 1 || due[0].Content != "到期任务" {
		t.Fatalf("到期消费不正确: %+v", due)
	}
	if again := ConsumeDue(); len(again) != 0 {
		t.Fatalf("到期项不应重复消费: %+v", again)
	}
}
