package life

// event_watch_test.go 观影结果回写愿望：整支视频看完 → 完成；观看失败 → 逾期；
// 用户消息优先导致的暂停让出不算失败。

import (
	"context"
	"testing"

	"LunarSubsystem/LunarHarness/chain/viewer"
	"LunarSubsystem/LunarHarness/chain/wishchain"
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

func TestWatchRunMarksWishOverdue(t *testing.T) {
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
	if !ok || !item.ForcedOverdue {
		t.Fatalf("观看失败应把对应愿望标记为逾期，实际 %+v（%v）", item, ok)
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
	if !ok || item.ForcedOverdue || item.Status != wishchain.StatusPending {
		t.Fatalf("暂停让出不应标记逾期，实际 %+v（%v）", item, ok)
	}
}
