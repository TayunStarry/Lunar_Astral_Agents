package life

// event_watch_test.go 观影结果回写愿望：整支视频看完 → 完成；观看失败 → 逾期；
// 用户消息优先导致的暂停让出不算失败。

import (
	"context"
	"testing"

	"LunarSubsystem/LunarHarness/chain/viewer"
	"LunarSubsystem/LunarHarness/engine/wishchain"
	"LunarSubsystem/LunarHarness/internal/testenv"
)

func withWatchHook(t *testing.T, outcome viewer.WatchOutcome, err error) {
	t.Helper()
	old := watchStepHook
	watchStepHook = func(context.Context, string) (viewer.WatchOutcome, error) {
		return outcome, err
	}
	t.Cleanup(func() { watchStepHook = old })
}

func TestWatchRunCompletesWish(t *testing.T) {
	testenv.Isolate(t)
	wishchain.Reset()
	t.Cleanup(wishchain.Reset)
	wish, err := wishchain.Add("看《流浪地球》", "", wishchain.AutoPriority)
	if err != nil {
		t.Fatal(err)
	}
	withWatchHook(t, viewer.WatchOutcome{
		Triggered: true, LastSegment: true, Title: "流浪地球", Topics: []string{"科幻"},
	}, nil)

	run := &watchRun{}
	if err := run.Start(context.Background(), Env{}); err != nil {
		t.Fatal(err)
	}
	if _, err := run.Next(context.Background(), Env{}); err != nil {
		t.Fatal(err)
	}
	item, ok := wishchain.Get(wish.ID)
	if !ok || item.Status != wishchain.StatusCompleted {
		t.Fatalf("整支视频看完应完成对应愿望，实际 %+v（%v）", item, ok)
	}
}

func TestWatchRunFailureKeepsWishPending(t *testing.T) {
	testenv.Isolate(t)
	wishchain.Reset()
	t.Cleanup(wishchain.Reset)
	wish, err := wishchain.Add("看《流浪地球》", "", wishchain.AutoPriority)
	if err != nil {
		t.Fatal(err)
	}
	withWatchHook(t, viewer.WatchOutcome{
		Triggered: false, Failed: true, Reason: "待看列表里还没有下载好的片段",
	}, nil)

	run := &watchRun{}
	if err := run.Start(context.Background(), Env{Instruction: "看《流浪地球》"}); err != nil {
		t.Fatal(err)
	}
	if _, err := run.Next(context.Background(), Env{}); err != nil {
		t.Fatal(err)
	}
	item, ok := wishchain.Get(wish.ID)
	if !ok || item.Status != wishchain.StatusPending {
		t.Fatalf("观看失败不应改变愿望状态，实际 %+v（%v）", item, ok)
	}
	if !hasLink(wishchain.LinkFailure) {
		t.Fatal("观看失败应写入类似逾期的失望联动")
	}
}

func TestWatchRunPauseDoesNotMarkOverdue(t *testing.T) {
	testenv.Isolate(t)
	wishchain.Reset()
	t.Cleanup(wishchain.Reset)
	wish, err := wishchain.Add("看《流浪地球》", "", wishchain.AutoPriority)
	if err != nil {
		t.Fatal(err)
	}
	withWatchHook(t, viewer.WatchOutcome{
		Triggered: false, Reason: "用户消息优先，这一片段留到下次再看",
	}, nil)

	run := &watchRun{}
	if err := run.Start(context.Background(), Env{Instruction: "看《流浪地球》"}); err != nil {
		t.Fatal(err)
	}
	if _, err := run.Next(context.Background(), Env{}); err != nil {
		t.Fatal(err)
	}
	item, ok := wishchain.Get(wish.ID)
	if !ok || item.Status != wishchain.StatusPending {
		t.Fatalf("暂停让出不应改变愿望状态，实际 %+v（%v）", item, ok)
	}
	if hasLink(wishchain.LinkFailure) {
		t.Fatal("暂停让出不应产生失败联动")
	}
}

// hasLink 检查最近联动记录里是否存在指定类型。
func hasLink(kind string) bool {
	for _, link := range wishchain.RecentLinks(16) {
		if link.Kind == kind {
			return true
		}
	}
	return false
}
