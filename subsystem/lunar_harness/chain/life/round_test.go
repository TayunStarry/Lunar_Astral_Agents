package life

// round_test.go 生活回合：五步顺序、维护失败不中断、让出点、看视频与工具共用路径、论坛发帖节流。

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"LunarSubsystem/LunarHarness/agents/MutualForum"
	"LunarSubsystem/LunarHarness/chain/viewer"
)

// errTestStage 模拟机会性维护失败。
var errTestStage = errors.New("阶段失败")

// stepWholeRound 连续推进，直到回合结束或事件完成。
func stepWholeRound(t *testing.T) error {
	t.Helper()
	for i := 0; i < 8; i++ {
		if err := Step(context.Background()); err != nil {
			return err
		}
		if !HasActiveRun() {
			return nil
		}
	}
	t.Fatal("回合在 8 步内未结束")
	return nil
}

// stubFixedSteps 用记录器替换固定三步。
func stubFixedSteps(order *[]string) {
	moodStepHook = func(context.Context) error {
		*order = append(*order, "mood")
		return nil
	}
	affectionStepHook = func(context.Context) error {
		*order = append(*order, "affection")
		return nil
	}
	memoryPendingStepHook = func(context.Context) error {
		*order = append(*order, "memory")
		return nil
	}
}

func TestRoundFixedOrder(t *testing.T) {
	resetLifeForTest(t)
	order := make([]string, 0, 8)
	stubFixedSteps(&order)
	ensureWatchlistHook = func(context.Context) (viewer.WatchlistResult, error) {
		order = append(order, "watchlist")
		return viewer.WatchlistResult{Triggered: true, Candidates: 24, Approved: 24, Enqueued: 24, Downloaded: 6}, nil
	}
	biliAutoRespondHook = func(context.Context) (viewer.AutoRespondResult, error) {
		order = append(order, "bili")
		return viewer.AutoRespondResult{Triggered: true}, nil
	}
	forumAutoRespondHook = func(context.Context) (MutualForum.Summary, error) {
		order = append(order, "forum")
		return MutualForum.Summary{PageOpen: true, Pending: 1, Responded: 1}, nil
	}
	watchStepHook = func(_ context.Context, _ string) (viewer.WatchOutcome, error) {
		order = append(order, "branch")
		return viewer.WatchOutcome{Triggered: true, BV: "BV1xx", Title: "测试视频", SegmentIndex: 1, SegmentTotal: 2}, nil
	}
	oldRand := pickRandHook
	t.Cleanup(func() { pickRandHook = oldRand })
	pickRandHook = func(int) int { return 40 } // 命中 watch（daze 40 之后）

	if err := stepWholeRound(t); err != nil {
		t.Fatal(err)
	}

	want := []string{"mood", "affection", "memory", "watchlist", "bili", "forum", "branch"}
	if len(order) != len(want) {
		t.Fatalf("步骤数量应为 %d，实际 %v", len(want), order)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("第 %d 步顺序错误：want %s，实际 %s（完整顺序 %v）", i+1, want[i], order[i], order)
		}
	}

	summary, ok := LastRoundSummary()
	if !ok {
		t.Fatal("回合结束后应有可读摘要")
	}
	if summary.Branch != "watch" {
		t.Fatalf("回合分支应为 watch，实际 %q", summary.Branch)
	}
	if len(summary.Steps) != 5 {
		t.Fatalf("摘要应记录 5 个阶段，实际 %d", len(summary.Steps))
	}
	names := []string{roundStageFixed, roundStageWatchlist, roundStageBiliReply, roundStageForumReply, roundStagePick}
	for i, name := range names {
		if summary.Steps[i].Name != name {
			t.Fatalf("摘要第 %d 阶段应为 %s，实际 %s", i+1, name, summary.Steps[i].Name)
		}
	}
}

func TestRoundMaintenanceFailureDoesNotStop(t *testing.T) {
	resetLifeForTest(t)
	order := make([]string, 0, 8)
	stubFixedSteps(&order)
	ensureWatchlistHook = func(context.Context) (viewer.WatchlistResult, error) {
		order = append(order, "watchlist")
		return viewer.WatchlistResult{}, errTestStage
	}
	biliAutoRespondHook = func(context.Context) (viewer.AutoRespondResult, error) {
		order = append(order, "bili")
		return viewer.AutoRespondResult{}, errTestStage
	}
	forumAutoRespondHook = func(context.Context) (MutualForum.Summary, error) {
		order = append(order, "forum")
		return MutualForum.Summary{PageOpen: true, Pending: 2, Responded: 2}, nil
	}
	oldRand := pickRandHook
	t.Cleanup(func() { pickRandHook = oldRand })
	pickRandHook = func(int) int { return 0 } // 落到发呆，避免依赖未就绪的外部流程

	if err := stepWholeRound(t); err != nil {
		t.Fatal(err)
	}
	if len(order) != 6 || order[3] != "watchlist" || order[4] != "bili" || order[5] != "forum" {
		t.Fatalf("前一步失败不得中断后续维护，实际顺序 %v", order)
	}
	summary, _ := LastRoundSummary()
	if len(summary.Steps) != 5 {
		t.Fatalf("失败也要记录全部阶段，实际 %d", len(summary.Steps))
	}
	if summary.Steps[1].Status != roundStatusFailed || summary.Steps[2].Status != roundStatusFailed {
		t.Fatalf("失败阶段应标记 failed，实际 %+v", summary.Steps)
	}
	if summary.Steps[3].Status != roundStatusOK {
		t.Fatalf("第三步成功应标记 ok，实际 %+v", summary.Steps[3])
	}
}

func TestRoundYieldsBeforeMaintenance(t *testing.T) {
	resetLifeForTest(t)
	order := make([]string, 0, 4)
	stubFixedSteps(&order)
	called := 0
	ensureWatchlistHook = func(context.Context) (viewer.WatchlistResult, error) {
		called++
		return viewer.WatchlistResult{}, nil
	}

	if err := Step(context.Background()); err != nil {
		t.Fatal(err)
	}
	if called != 0 || !RoundInProgress() {
		t.Fatal("固定三步之后应停在让出点")
	}

	pauseHook = func() bool { return true }
	if err := Step(context.Background()); err != nil {
		t.Fatal(err)
	}
	if called != 0 {
		t.Fatal("用户消息到达时不应继续机会性维护")
	}
	if !RoundInProgress() {
		t.Fatal("让出后回合应保持未完成，等下次 Tick 续跑")
	}

	pauseHook = func() bool { return false }
	if err := Step(context.Background()); err != nil {
		t.Fatal(err)
	}
	if called != 1 {
		t.Fatalf("恢复后应从同一步继续，实际调用 %d 次", called)
	}
}

func TestWatchBranchSharesToolPath(t *testing.T) {
	resetLifeForTest(t)
	gotInstruction := ""
	watchStepHook = func(_ context.Context, instruction string) (viewer.WatchOutcome, error) {
		gotInstruction = instruction
		return viewer.WatchOutcome{
			Triggered: true, BV: "BV1tool", Title: "对话者点名的视频",
			SegmentIndex: 1, SegmentTotal: 1, LastSegment: true, CommentPosted: true,
		}, nil
	}
	if !RequestEvent("watch", "对话者想看猫咪视频", nil) {
		t.Fatal("watch 强制事件应可登记")
	}
	if err := stepWholeRound(t); err != nil {
		t.Fatal(err)
	}
	if gotInstruction != "对话者想看猫咪视频" {
		t.Fatalf("工具触发应走同一看视频实现并携带指令，实际 %q", gotInstruction)
	}
	mu.Lock()
	record := state.LastRecord
	mu.Unlock()
	if !strings.Contains(record, "对话者点名的视频") || !strings.Contains(record, "看了") {
		t.Fatalf("生活记录应来自看视频结果，实际 %q", record)
	}
}

func TestForumPostThrottledByCooldown(t *testing.T) {
	resetLifeForTest(t)
	now := time.Unix(1_800_000_000, 0)

	first := &forumRun{}
	if err := first.Start(context.Background(), Env{Now: now}); err != nil {
		t.Fatal(err)
	}
	if !first.data.AllowPost {
		t.Fatal("首轮应允许自主发帖")
	}

	second := &forumRun{}
	if err := second.Start(context.Background(), Env{Now: now.Add(time.Minute)}); err != nil {
		t.Fatal(err)
	}
	if second.data.AllowPost {
		t.Fatal("冷却期内不应再次允许自主发帖")
	}
	if !strings.Contains(second.data.Instruction, "不要发新话题") {
		t.Fatalf("冷却期应明确要求只读和回复，实际 %q", second.data.Instruction)
	}

	third := &forumRun{}
	if err := third.Start(context.Background(), Env{Now: now.Add(forumPostCooldown)}); err != nil {
		t.Fatal(err)
	}
	if !third.data.AllowPost {
		t.Fatal("冷却结束后应重新允许发帖")
	}

	// 对话者通过工具明确要求时不受冷却限制。
	forced := &forumRun{}
	if err := forced.Start(context.Background(), Env{Now: now.Add(time.Minute), Instruction: "帮我在论坛问问进度"}); err != nil {
		t.Fatal(err)
	}
	if !forced.data.AllowPost || forced.data.Instruction != "帮我在论坛问问进度" {
		t.Fatalf("工具触发应直接采用对话者要求，实际 %+v", forced.data)
	}
}
