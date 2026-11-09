package storyboard

// watchdog_test.go 超时判定规则：只认「无状态更新」，不认「运行总时长」。

import (
	"context"
	"strings"
	"testing"
	"time"
)

// newWatchTask 构造一个可推进的任务状态（默认处于绘制阶段且仍在心跳）。
func newWatchTask(t *testing.T, taskID string) *taskState {
	t.Helper()
	now := time.Now()
	state := newTestTask(t, taskID)
	taskMu.Lock()
	state.stage = stagePainting
	state.view.Status = statusPainting
	state.view.Stage = "正在绘制第 3 / 44 镜（1-3）"
	state.startedAt = now
	state.lastBeat = now
	state.budgetAt = now.Add(runBudget)
	state.nextWarn = now.Add(runWarnTimeout)
	taskMu.Unlock()
	return state
}

func watchVerdict(t *testing.T, state *taskState, now time.Time) watchdogAction {
	t.Helper()
	taskMu.Lock()
	defer taskMu.Unlock()
	return watchTaskLocked(state, now)
}

// TestLongRunningActiveTaskIsNotFailed 运行远超旧 90 分钟阈值的任务，
// 只要还在持续产出状态更新，就不得判失败，只允许顺延预算。
func TestLongRunningActiveTaskIsNotFailed(t *testing.T) {
	state := newWatchTask(t, "watch-long-active")
	now := time.Now()
	taskMu.Lock()
	state.startedAt = now.Add(-3 * time.Hour) // 已经跑了 3 小时
	state.budgetAt = now.Add(-time.Second)    // 预算已到期
	state.lastBeat = now.Add(-30 * time.Second)
	taskMu.Unlock()

	if verdict := watchVerdict(t, state, now); verdict != watchdogRenew {
		t.Fatalf("长耗时但仍在推进的任务应顺延预算，实际处置 = %d", verdict)
	}

	renewBudget("watch-long-active", state.view.Stage)

	taskMu.Lock()
	defer taskMu.Unlock()
	if state.view.Status == statusFailed {
		t.Fatalf("仍在推进的长任务不得判失败，实际原因 %q", state.view.Error)
	}
	if !state.budgetAt.After(now.Add(runBudget - time.Minute)) {
		t.Fatalf("预算未被顺延，当前到期时间 %v", state.budgetAt)
	}
	consumePendingLocked(state)
}

// TestBudgetExpiryChecksHealthBeforeFailing 预算到期且确实长时间无更新时才判失败。
func TestBudgetExpiryChecksHealthBeforeFailing(t *testing.T) {
	state := newWatchTask(t, "watch-budget-stalled")
	now := time.Now()
	taskMu.Lock()
	state.startedAt = now.Add(-3 * time.Hour)
	state.budgetAt = now.Add(-time.Minute)
	state.lastBeat = now.Add(-runStallTimeout - time.Minute)
	taskMu.Unlock()

	renewBudget("watch-budget-stalled", state.view.Stage)

	taskMu.Lock()
	defer taskMu.Unlock()
	if state.view.Status != statusFailed {
		t.Fatalf("预算到期且无响应应判失败，实际 %s", state.view.Status)
	}
	if !strings.Contains(state.view.Error, "没有任何状态更新") {
		t.Fatalf("失败原因应说明静默过久，实际 %q", state.view.Error)
	}
}

// TestWarnBeforeBudget 接近预算只提醒，不做任何终止。
func TestWarnBeforeBudget(t *testing.T) {
	state := newWatchTask(t, "watch-warn")
	now := time.Now()
	taskMu.Lock()
	state.nextWarn = now.Add(-time.Second)
	state.budgetAt = now.Add(10 * time.Minute)
	taskMu.Unlock()

	if verdict := watchVerdict(t, state, now); verdict != watchdogWarn {
		t.Fatalf("接近预算应只提醒，实际处置 = %d", verdict)
	}
}

// TestPausedAndFinishedTasksAreIgnored 用户等待态与终态不参与卡死判定。
func TestPausedAndFinishedTasksAreIgnored(t *testing.T) {
	state := newWatchTask(t, "watch-paused")
	now := time.Now()
	taskMu.Lock()
	state.lastBeat = now.Add(-time.Hour)
	state.paused = true
	taskMu.Unlock()
	if verdict := watchVerdict(t, state, now); verdict != watchdogIdle {
		t.Fatalf("暂停中的任务不应被判定，实际处置 = %d", verdict)
	}

	taskMu.Lock()
	state.paused = false
	state.stage = stageDone
	taskMu.Unlock()
	if verdict := watchVerdict(t, state, now); verdict != watchdogIdle {
		t.Fatalf("已结束的任务不应被判定，实际处置 = %d", verdict)
	}
}

// TestRenewBudgetEmitsNotice 预算顺延必须推送提醒，且不改变任务状态。
func TestRenewBudgetEmitsNotice(t *testing.T) {
	state := newWatchTask(t, "watch-renew")
	now := time.Now()
	taskMu.Lock()
	state.startedAt = now.Add(-2 * time.Hour)
	state.budgetAt = now.Add(-time.Minute)
	taskMu.Unlock()

	renewBudget("watch-renew", "正在绘制第 3 / 44 镜（1-3）")

	taskMu.Lock()
	defer taskMu.Unlock()
	if state.view.Status != statusPainting {
		t.Fatalf("预算顺延不得改变任务状态，实际 %s", state.view.Status)
	}
	if state.view.Event != eventLongRunning {
		t.Fatalf("应推送长耗时提醒事件，实际 %s", state.view.Event)
	}
	if !strings.Contains(state.view.Summary, "顺延") {
		t.Fatalf("提醒文案应说明预算已顺延，实际 %q", state.view.Summary)
	}
	if !state.budgetAt.After(now.Add(runBudget - time.Minute)) {
		t.Fatalf("预算未被顺延，当前到期时间 %v", state.budgetAt)
	}
	consumePendingLocked(state)
}

// TestStalledStepFails 单步超时后仍无任何状态更新，才判卡死。
func TestStalledStepFails(t *testing.T) {
	state := newWatchTask(t, "watch-step")
	now := time.Now()
	taskMu.Lock()
	state.lastBeat = now.Add(-20 * time.Minute)
	taskMu.Unlock()

	failStalledStep("watch-step", stagePainting, 10*time.Minute)

	taskMu.Lock()
	defer taskMu.Unlock()
	if state.view.Status != statusFailed {
		t.Fatalf("超时且无更新的步骤应判失败，实际 %s", state.view.Status)
	}
	if !strings.Contains(state.view.Error, "没有任何状态更新") {
		t.Fatalf("失败原因应说明是静默过久，实际 %q", state.view.Error)
	}
	if !strings.Contains(state.view.Error, "正在绘制第 3 / 44 镜") {
		t.Fatalf("失败原因应指出卡在哪个阶段，实际 %q", state.view.Error)
	}
}

// TestStalledStepIgnoresProgress 步骤期间仍有状态更新时不得误判。
func TestStalledStepIgnoresProgress(t *testing.T) {
	state := newWatchTask(t, "watch-step-active")
	failStalledStep("watch-step-active", stagePainting, 10*time.Minute)

	taskMu.Lock()
	defer taskMu.Unlock()
	if state.view.Status != statusPainting {
		t.Fatalf("仍在产出进度的步骤不得判失败，实际 %s", state.view.Status)
	}
}

// TestStalledStepIgnoresMovedStage 步骤已结束（阶段变化）时兜底不得误伤。
func TestStalledStepIgnoresMovedStage(t *testing.T) {
	state := newWatchTask(t, "watch-step-moved")
	now := time.Now()
	taskMu.Lock()
	state.stage = stageWriting
	state.lastBeat = now.Add(-20 * time.Minute)
	taskMu.Unlock()

	failStalledStep("watch-step-moved", stagePainting, 10*time.Minute)

	taskMu.Lock()
	defer taskMu.Unlock()
	if state.view.Status == statusFailed {
		t.Fatalf("阶段已推进的旧兜底不得判失败，实际原因 %q", state.view.Error)
	}
}

// TestAdvanceKeepsLongRunningQueuedTask 超长运行但仍有心跳的任务，
// 调度入口不得把它判失败，而是继续推进。
func TestAdvanceKeepsLongRunningQueuedTask(t *testing.T) {
	state := newTestTask(t, "watch-advance-long")
	now := time.Now()
	taskMu.Lock()
	state.stage = stageQueued
	state.view.Status = statusQueued
	state.view.Stage = "已受理，等待演绎队列"
	state.req = SubmitRequest{Text: strings.Repeat("星月", 20)}
	state.startedAt = now.Add(-4 * time.Hour)
	state.budgetAt = now.Add(-time.Minute)
	state.lastBeat = now.Add(-10 * time.Second)
	taskMu.Unlock()

	if err := Advance(context.Background()); err != nil {
		t.Fatalf("调度入口不应返回错误: %v", err)
	}

	taskMu.Lock()
	defer taskMu.Unlock()
	if state.view.Status == statusFailed {
		t.Fatalf("仍在推进的长任务不得判失败，实际原因 %q", state.view.Error)
	}
	if state.stage != stageResearching {
		t.Fatalf("任务应继续推进到资料准备，实际阶段 %s", state.stage)
	}
	consumePendingLocked(state)
}

// TestStallReasonIsReadable 失败文案必须说清阶段与静默时长，不能只给原始 duration。
func TestStallReasonIsReadable(t *testing.T) {
	message := stallReason("正在绘制第 3 / 44 镜（1-3）", 21*time.Minute).Error()
	if !strings.Contains(message, "正在绘制第 3 / 44 镜") || !strings.Contains(message, "21 分钟") {
		t.Fatalf("判定文案不完整: %q", message)
	}
	if strings.Contains(message, "h") && strings.Contains(message, "m0s") {
		t.Fatalf("判定文案不应出现原始 duration 写法: %q", message)
	}
}

// TestHumanDuration 时长格式化。
func TestHumanDuration(t *testing.T) {
	cases := map[time.Duration]string{
		90 * time.Second:            "1 分钟",
		21 * time.Minute:            "21 分钟",
		90 * time.Minute:            "1 小时 30 分",
		2 * time.Hour:               "2 小时 0 分",
		3*time.Hour + 5*time.Minute: "3 小时 5 分",
	}
	for input, want := range cases {
		if got := humanDuration(input); got != want {
			t.Fatalf("humanDuration(%v) = %q，期望 %q", input, got, want)
		}
	}
}
