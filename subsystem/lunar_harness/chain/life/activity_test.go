package life

// activity_test.go 状态字段覆盖回归：
// 固定三步 / daze / memory 完成后必须写入，分支文案优先于兜底记录，
// 失败与条件不满足的早退都必须写入可区分的文案。

import (
	"context"
	"errors"
	"strings"
	"testing"

	"LunarSubsystem/LunarHarness/agents/MutualForum"
	"LunarSubsystem/LunarHarness/chain/viewer"
)

// recordActivity 用记录器替换状态写入点，返回收集到的文案列表。
func recordActivity(t *testing.T) *[]string {
	t.Helper()
	var texts []string
	old := setActivityHook
	setActivityHook = func(text string) { texts = append(texts, text) }
	t.Cleanup(func() { setActivityHook = old })
	return &texts
}

func lastActivity(texts []string) string {
	if len(texts) == 0 {
		return ""
	}
	return texts[len(texts)-1]
}

// runPendingEvent 以「已选中断点」的方式执行一次事件步骤。
func runPendingEvent(t *testing.T, event Event, run EventRun) error {
	t.Helper()
	mu.Lock()
	state.Active = &ActiveRun{Event: event.ID(), Step: 0}
	mu.Unlock()
	return runOneEventStep(context.Background(), Env{}, event, run, 0)
}

func TestFixedStepsWriteActivity(t *testing.T) {
	resetLifeForTest(t)
	withFixedStepHooks(t)
	texts := recordActivity(t)
	order := make([]string, 0, 3)
	stubFixedSteps(&order)

	if err := Step(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := lastActivity(*texts); !strings.Contains(got, "心情") || !strings.Contains(got, "整理完") {
		t.Fatalf("固定三步完成后应写入整理状态，实际 %q（全部 %v）", got, *texts)
	}
}

func TestFixedStepsFailureWritesPartialActivity(t *testing.T) {
	resetLifeForTest(t)
	withFixedStepHooks(t)
	texts := recordActivity(t)
	moodStepHook = func(context.Context) error { return errors.New("嵌入服务不可用") }
	affectionStepHook = func(context.Context) error { return nil }
	memoryPendingStepHook = func(context.Context) error { return nil }

	if err := Step(context.Background()); err != nil {
		t.Fatal(err)
	}
	got := lastActivity(*texts)
	if !strings.Contains(got, "没整理完") || !strings.Contains(got, "嵌入服务不可用") {
		t.Fatalf("固定步骤失败时应写入可读的部分失败文案，实际 %q", got)
	}
}

func TestDazeCompletedWritesActivity(t *testing.T) {
	resetLifeForTest(t)
	texts := recordActivity(t)
	oldRand := pickRandHook
	t.Cleanup(func() { pickRandHook = oldRand })
	pickRandHook = func(int) int { return 0 } // 命中 daze

	if err := stepWholeRound(t); err != nil {
		t.Fatal(err)
	}
	if got := lastActivity(*texts); !strings.Contains(got, "什么也没做") {
		t.Fatalf("daze 完成后应写入状态，实际 %q（全部 %v）", got, *texts)
	}
}

func TestWatchActivityKeepsBranchText(t *testing.T) {
	resetLifeForTest(t)
	texts := recordActivity(t)
	want := "在看《测试视频》第 1/2 段"
	watchStepHook = func(context.Context, string) (viewer.WatchOutcome, error) {
		return viewer.WatchOutcome{
			Triggered: true, Title: "测试视频", SegmentIndex: 1, SegmentTotal: 2, Activity: want,
		}, nil
	}

	event := newWatchEvent()
	if err := runPendingEvent(t, event, event.NewRun()); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	record := state.LastRecord
	mu.Unlock()
	if got := lastActivity(*texts); got != want {
		t.Fatalf("看视频成功时应保留分支文案 %q，实际 %q", want, got)
	}
	if lastActivity(*texts) == record {
		t.Fatal("兜底文案不应覆盖分支已经写过的具体文案")
	}
}

func TestWatchEarlyExitWritesReason(t *testing.T) {
	resetLifeForTest(t)
	texts := recordActivity(t)
	watchStepHook = func(context.Context, string) (viewer.WatchOutcome, error) {
		return viewer.WatchOutcome{Reason: "待看列表里还没有下载好的片段"}, nil
	}

	event := newWatchEvent()
	if err := runPendingEvent(t, event, event.NewRun()); err != nil {
		t.Fatal(err)
	}
	got := lastActivity(*texts)
	if !strings.Contains(got, "想看点视频") || !strings.Contains(got, "没有下载好的片段") {
		t.Fatalf("看视频无片段时应写入明确早退文案，实际 %q", got)
	}
}

func TestEventFailureReplacesStaleActivity(t *testing.T) {
	resetLifeForTest(t)
	texts := recordActivity(t)
	setActivity("上一次的旧状态")
	watchStepHook = func(context.Context, string) (viewer.WatchOutcome, error) {
		return viewer.WatchOutcome{}, errors.New("网络断了")
	}

	event := newWatchEvent()
	if err := runPendingEvent(t, event, event.NewRun()); err == nil {
		t.Fatal("看视频失败应返回错误")
	}
	got := lastActivity(*texts)
	if !strings.Contains(got, "没做成") || !strings.Contains(got, "网络断了") {
		t.Fatalf("失败时应写入失败文案，实际 %q", got)
	}
	if got == "上一次的旧状态" {
		t.Fatal("失败后不能保留旧状态")
	}
	mu.Lock()
	record := state.LastRecord
	mu.Unlock()
	if record != "看视频没看成（未完成）" {
		t.Fatalf("失败记录应沿用原文案，实际 %q", record)
	}
}

func TestFailureAndSkipActivityDiffer(t *testing.T) {
	skipText := failureActivity("forum", skip("互助论坛未就绪"))
	failText := failureActivity("forum", errors.New("论坛接口 500"))
	if !strings.Contains(skipText, "做不了") || !strings.Contains(skipText, "互助论坛未就绪") {
		t.Fatalf("条件不满足的早退应写入「做不了」文案，实际 %q", skipText)
	}
	if !strings.Contains(failText, "没做成") || !strings.Contains(failText, "论坛接口 500") {
		t.Fatalf("失败应写入「没做成」文案，实际 %q", failText)
	}
	if skipText == failText {
		t.Fatal("早退与失败的状态文案必须可区分")
	}
	if !isSkip(skip("x")) || isSkip(errors.New("x")) {
		t.Fatal("isSkip 应只识别条件不满足的早退")
	}
}

// TestMaintenanceStepsWriteActivity 回合第 2~4 步：成功写这次做了什么，
// 失败写「没做成」，跳过写「做不了」并带原因，与生活事件的措辞保持一致。
func TestMaintenanceStepsWriteActivity(t *testing.T) {
	resetLifeForTest(t)
	texts := recordActivity(t)
	ctx := context.Background()

	ensureWatchlistHook = func(context.Context) (viewer.WatchlistResult, error) {
		return viewer.WatchlistResult{Reason: "待看列表非空"}, nil
	}
	runWatchlistStage(ctx)
	assertActivity(t, *texts, "做不了", "待看列表非空")

	ensureWatchlistHook = func(context.Context) (viewer.WatchlistResult, error) {
		return viewer.WatchlistResult{Triggered: true, Enqueued: 3, Downloaded: 2}, nil
	}
	runWatchlistStage(ctx)
	assertActivity(t, *texts, "待看列表", "3")

	biliAutoRespondHook = func(context.Context) (viewer.AutoRespondResult, error) {
		return viewer.AutoRespondResult{}, errors.New("网络断了")
	}
	runBiliReplyStage(ctx)
	assertActivity(t, *texts, "没做成", "网络断了")

	biliAutoRespondHook = func(context.Context) (viewer.AutoRespondResult, error) {
		return viewer.AutoRespondResult{Triggered: true, Video: viewer.SourceReplyStat{Found: 1, Replied: 1}}, nil
	}
	runBiliReplyStage(ctx)
	assertActivity(t, *texts, "B 站", "1 条")

	forumAutoRespondHook = func(context.Context) (MutualForum.Summary, error) {
		return MutualForum.Summary{PageOpen: true}, nil
	}
	runForumReplyStage(ctx)
	assertActivity(t, *texts, "做不了", "没有未读回应")

	forumAutoRespondHook = func(context.Context) (MutualForum.Summary, error) {
		return MutualForum.Summary{PageOpen: true, Pending: 2, Responded: 2}, nil
	}
	runForumReplyStage(ctx)
	assertActivity(t, *texts, "互助论坛", "2 条")
}

// assertActivity 断言最近一次写入的状态文案包含全部片段。
func assertActivity(t *testing.T, texts []string, wants ...string) {
	t.Helper()
	got := lastActivity(texts)
	for _, want := range wants {
		if !strings.Contains(got, want) {
			t.Fatalf("状态文案应包含 %q，实际 %q（全部 %v）", want, got, texts)
		}
	}
}
