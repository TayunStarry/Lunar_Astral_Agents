package thinking

// wish_view_test.go 运行时心愿话术：执念聚合与即将到期摘要。

import (
	"strings"
	"testing"
	"time"

	"LunarSubsystem/LunarHarness/engine/wishchain"
	"LunarSubsystem/LunarHarness/internal/testenv"
)

// TestDueWishMessageKeepsFormat 到期提醒文本保持「XX愿望到期」。
func TestDueWishMessageKeepsFormat(t *testing.T) {
	text := dueWishMessage(wishchain.DueNotice{Wish: wishchain.Wish{Content: "看科幻电影"}})
	if text != "看科幻电影愿望到期" {
		t.Fatalf("提醒格式应为「XX愿望到期」，实际 %q", text)
	}
}

// TestObsessionTextAggregatesOverdueWishes 逾期愿望按优先级整合为唯一执念。
func TestObsessionTextAggregatesOverdueWishes(t *testing.T) {
	testenv.Isolate(t)
	wishchain.Reset()
	t.Cleanup(wishchain.Reset)
	past := time.Now().Add(-wishchain.GraceDuration - time.Hour).Format("2006-01-02 15:04:05")
	if _, err := wishchain.Add("写一篇关于月亮的散文", past, 3); err != nil {
		t.Fatal(err)
	}
	if _, err := wishchain.Add("看《流浪地球》", past, 0); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(24 * time.Hour).Format("2006-01-02 15:04:05")
	if _, err := wishchain.Add("下个月去海边", future, 1); err != nil {
		t.Fatal(err)
	}
	obsession := obsessionText()
	if !strings.Contains(obsession, "写一篇关于月亮的散文") || !strings.Contains(obsession, "看《流浪地球》") {
		t.Fatalf("执念应整合全部逾期愿望: %s", obsession)
	}
	if strings.Contains(obsession, "下个月去海边") {
		t.Fatalf("未逾期愿望不应进入执念: %s", obsession)
	}
	if strings.Contains(obsession, "\n") {
		t.Fatalf("执念只能有一条: %s", obsession)
	}
	if strings.Index(obsession, "看《流浪地球》") > strings.Index(obsession, "写一篇关于月亮的散文") {
		t.Fatalf("执念应按优先级从高到低整合: %s", obsession)
	}
}

// TestUpcomingWishTextWindow 即将到期摘要只包含窗口内的愿望。
func TestUpcomingWishTextWindow(t *testing.T) {
	testenv.Isolate(t)
	wishchain.Reset()
	t.Cleanup(wishchain.Reset)
	now := time.Now()
	near := now.Add(30 * time.Minute)
	far := now.Add(2 * time.Hour)
	if _, err := wishchain.Add("看电影", near.Format("2006-01-02 15:04:05"), wishchain.AutoPriority); err != nil {
		t.Fatal(err)
	}
	if _, err := wishchain.Add("去海边", far.Format("2006-01-02 15:04:05"), wishchain.AutoPriority); err != nil {
		t.Fatal(err)
	}
	text := upcomingWishText(now, time.Hour)
	if !strings.Contains(text, near.Format("15:04")+" 看电影") {
		t.Fatalf("窗口内愿望应进入摘要: %s", text)
	}
	if strings.Contains(text, "去海边") {
		t.Fatalf("窗口外愿望不应进入摘要: %s", text)
	}
}
