package tickloop

// scheduler_test.go 通用 Tick 调度器的行为回归（原月华调度器测试迁移）。

import (
	"context"
	"sync"
	"testing"
	"time"

	"LunarSubsystem/LunarHarness/engine/tickpermit"
)

type testSnapshot struct {
	Unread int
}

type fakeSystem struct {
	id        string
	priority  int
	idleReset int
	keepIdle  bool
	ready     bool
	reason    string

	mu      sync.Mutex
	runs    int
	started chan struct{}
	block   chan struct{}
}

func (f *fakeSystem) ID() string     { return f.id }
func (f *fakeSystem) Priority() int  { return f.priority }
func (f *fakeSystem) IdleReset() int { return f.idleReset }
func (f *fakeSystem) KeepIdle() bool { return f.keepIdle }
func (f *fakeSystem) Ready(testSnapshot) (bool, string) {
	return f.ready, f.reason
}
func (f *fakeSystem) Run(context.Context) error {
	f.mu.Lock()
	f.runs++
	f.mu.Unlock()
	if f.started != nil {
		close(f.started)
	}
	if f.block != nil {
		<-f.block
	}
	return nil
}
func (f *fakeSystem) runCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.runs
}

func newTestScheduler(t *testing.T, notify func()) *Scheduler[testSnapshot] {
	t.Helper()
	return New(Hooks[testSnapshot]{
		StateFile:  "",
		Snapshot:   func(int) testSnapshot { return testSnapshot{} },
		NotifyIdle: notify,
	})
}

func TestSchedulerPriorityAndIdleReset(t *testing.T) {
	s := newTestScheduler(t, nil)
	dialogue := &fakeSystem{id: "dialogue", priority: 300, ready: true, reason: "unread"}
	viewing := &fakeSystem{id: "viewing", priority: 200, ready: true, reason: "slices"}
	lifeSys := &fakeSystem{id: "life", priority: 100, ready: true, reason: "cooldown"}
	s.Register(lifeSys)
	s.Register(viewing)
	s.Register(dialogue)

	s.state.IdleTicks = 600
	s.Tick()
	if s.state.LastSystem != "dialogue" {
		t.Fatalf("应优先触发对话，实际 %s", s.state.LastSystem)
	}
	if s.state.IdleTicks != 0 {
		t.Fatalf("触发后空闲计数应清零，实际 %d", s.state.IdleTicks)
	}
	s.wg.Wait()
	if dialogue.runCount() != 1 || viewing.runCount() != 0 || lifeSys.runCount() != 0 {
		t.Fatal("同一 Tick 只允许一个体系运行")
	}
}

func TestSchedulerPauseBlocksTickAndWaitsIdle(t *testing.T) {
	s := newTestScheduler(t, nil)
	system := &fakeSystem{id: "dialogue", priority: 300, ready: true, reason: "unread"}
	s.Register(system)

	s.Pause()
	s.Tick()
	time.Sleep(20 * time.Millisecond)
	if system.runCount() != 0 {
		t.Fatalf("维护态下不应触发体系，实际运行 %d 次", system.runCount())
	}

	s.Resume()
	s.Tick()
	s.wg.Wait()
	if system.runCount() != 1 {
		t.Fatalf("恢复后应正常触发，实际 %d 次", system.runCount())
	}

	system.started = make(chan struct{})
	system.block = make(chan struct{})
	s.Tick()
	<-system.started
	if s.WaitIdle(50 * time.Millisecond) {
		t.Fatal("运行中不应判定为空闲")
	}
	close(system.block)
	if !s.WaitIdle(time.Second) {
		t.Fatal("运行结束后应判定为空闲")
	}
	s.wg.Wait()
}

func TestSchedulerBusySkipsTick(t *testing.T) {
	s := newTestScheduler(t, nil)
	started := make(chan struct{})
	block := make(chan struct{})
	system := &fakeSystem{id: "dialogue", priority: 300, ready: true, reason: "unread", started: started, block: block}
	s.Register(system)

	s.Tick()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("体系未启动")
	}
	s.Tick()
	if system.runCount() != 1 {
		t.Fatalf("运行中不应再次触发，实际运行 %d 次", system.runCount())
	}
	if s.state.IdleTicks != 0 {
		t.Fatalf("运行中不应增加空闲计数，实际 %d", s.state.IdleTicks)
	}
	close(block)
	s.wg.Wait()
}

func TestSchedulerNotifiesBridgeOnIdleTick(t *testing.T) {
	count := 0
	s := newTestScheduler(t, func() { count++ })
	s.Register(&fakeSystem{id: "dialogue", priority: 300, ready: false})
	s.Tick()
	if count != 1 {
		t.Fatalf("空闲 Tick 应通知 1 次，实际 %d", count)
	}
}

func TestSchedulerDoesNotNotifyBridgeWhenTriggering(t *testing.T) {
	count := 0
	s := newTestScheduler(t, func() { count++ })
	s.Register(&fakeSystem{id: "dialogue", priority: 300, ready: true})
	s.Tick()
	s.wg.Wait()
	if count != 0 {
		t.Fatalf("触发体系的 Tick 不应通知，实际 %d", count)
	}
}

func TestSchedulerIdleCounterAndThreshold(t *testing.T) {
	s := newTestScheduler(t, nil)
	lifeSys := &fakeSystem{id: "life", priority: 100, idleReset: -300, ready: false}
	s.Register(&fakeSystem{id: "dialogue", priority: 300, ready: false})
	s.Register(&fakeSystem{id: "viewing", priority: 200, ready: false})
	s.Register(lifeSys)

	target := 600
	for i := 0; i < target; i++ {
		s.Tick()
	}
	if s.state.IdleTicks != target {
		t.Fatalf("空闲计数应为 %d，实际 %d", target, s.state.IdleTicks)
	}
	if lifeSys.runCount() != 0 {
		t.Fatal("达到阈值前不应触发生活")
	}
	lifeSys.ready = true
	s.Tick()
	if s.state.LastSystem != "life" {
		t.Fatalf("达到阈值后应触发生活，实际 %s", s.state.LastSystem)
	}
	s.wg.Wait()
	if s.state.IdleTicks != -300 {
		t.Fatalf("生活结束后进度应重置为 -300，实际 %d", s.state.IdleTicks)
	}
}

func TestSchedulerUsesPerSystemIdleReset(t *testing.T) {
	cases := []struct {
		id    string
		reset int
	}{
		{"dialogue", 0},
		{"viewing", 300},
		{"life", -300},
	}
	for _, tc := range cases {
		s := newTestScheduler(t, nil)
		system := &fakeSystem{id: tc.id, priority: 300, idleReset: tc.reset, ready: true, reason: "test"}
		s.Register(system)
		s.state.IdleTicks = 900
		s.Tick()
		s.wg.Wait()
		if s.state.IdleTicks != tc.reset {
			t.Fatalf("%s 结束后进度应为 %d，实际 %d", tc.id, tc.reset, s.state.IdleTicks)
		}
	}
}

func TestSchedulerRegisterExtension(t *testing.T) {
	s := newTestScheduler(t, nil)
	s.Register(&fakeSystem{id: "low", priority: 10})
	s.Register(&fakeSystem{id: "high", priority: 900})
	s.Register(&fakeSystem{id: "mid", priority: 500})
	if len(s.systems) != 3 {
		t.Fatalf("应注册 3 个体系，实际 %d", len(s.systems))
	}
	if s.systems[0].ID() != "high" || s.systems[1].ID() != "mid" || s.systems[2].ID() != "low" {
		t.Fatalf("优先级排序错误: %s / %s / %s", s.systems[0].ID(), s.systems[1].ID(), s.systems[2].ID())
	}
}

// TestSchedulerForcedEventOnlyRunsLife 强制事件抢占：即使对话就绪也只允许 life 体系。
func TestSchedulerForcedEventOnlyRunsLife(t *testing.T) {
	s := New(Hooks[testSnapshot]{
		StateFile: "",
		Snapshot:  func(int) testSnapshot { return testSnapshot{} },
		Forced:    func() bool { return true },
	})
	dialogue := &fakeSystem{id: "dialogue", priority: 300, ready: true, reason: "unread"}
	lifeSys := &fakeSystem{id: "life", priority: 100, ready: true, reason: "forced"}
	s.Register(dialogue)
	s.Register(lifeSys)
	s.Tick()
	s.wg.Wait()
	if dialogue.runCount() != 0 || lifeSys.runCount() != 1 {
		t.Fatalf("强制事件下只应运行 life，实际 dialogue=%d life=%d", dialogue.runCount(), lifeSys.runCount())
	}
}

// TestSchedulerResponseRunsWhileMainBusy 主链长步骤在途时，响应链仍可插入执行。
func TestSchedulerResponseRunsWhileMainBusy(t *testing.T) {
	s := New(Hooks[testSnapshot]{
		StateFile:              "",
		Snapshot:               func(int) testSnapshot { return testSnapshot{} },
		AllowResponseWhileBusy: true,
	})
	main := &fakeSystem{id: "life", priority: 100, ready: true, reason: "cooldown", started: make(chan struct{}), block: make(chan struct{})}
	resp := &fakeSystem{id: "response", priority: 260, ready: false, reason: "pending"}
	s.Register(resp)
	s.Register(main)

	s.Tick()
	<-main.started
	resp.ready = true
	s.Tick()
	time.Sleep(20 * time.Millisecond)
	if resp.runCount() != 1 {
		t.Fatalf("主链忙碌时响应链应执行，实际 %d 次", resp.runCount())
	}
	if main.runCount() != 1 {
		t.Fatalf("主链仍应只运行一次，实际 %d 次", main.runCount())
	}
	close(main.block)
	s.wg.Wait()
}

// TestSchedulerResponseBlockedWhileMainBusyByDefault 默认保持旧语义：主链忙碌时不插入响应链。
func TestSchedulerResponseBlockedWhileMainBusyByDefault(t *testing.T) {
	s := newTestScheduler(t, nil)
	main := &fakeSystem{id: "life", priority: 100, ready: true, reason: "cooldown", started: make(chan struct{}), block: make(chan struct{})}
	resp := &fakeSystem{id: "response", priority: 260, ready: false, reason: "pending"}
	s.Register(resp)
	s.Register(main)

	s.Tick()
	<-main.started
	resp.ready = true
	s.Tick()
	time.Sleep(20 * time.Millisecond)
	if resp.runCount() != 0 {
		t.Fatalf("未开启并行时响应链不应执行，实际 %d 次", resp.runCount())
	}
	close(main.block)
	s.wg.Wait()
}

// backgroundSystem 模拟把长步骤放到后台的体系：Run 立即返回，步骤在后台阻塞。
type backgroundSystem struct {
	id       string
	priority int
	ready    bool

	mu      sync.Mutex
	running bool
	runs    int
	started chan struct{}
	block   chan struct{}
}

func (b *backgroundSystem) ID() string     { return b.id }
func (b *backgroundSystem) Priority() int  { return b.priority }
func (b *backgroundSystem) IdleReset() int { return 0 }
func (b *backgroundSystem) KeepIdle() bool { return false }
func (b *backgroundSystem) StepActive() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.running
}

func (b *backgroundSystem) Ready(testSnapshot) (bool, string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.running {
		return false, ""
	}
	return b.ready, "test"
}

func (b *backgroundSystem) Run(context.Context) error {
	b.mu.Lock()
	if b.running {
		b.mu.Unlock()
		return nil
	}
	b.running = true
	b.runs++
	b.mu.Unlock()
	close(b.started)
	go func() {
		<-b.block
		b.mu.Lock()
		b.running = false
		b.mu.Unlock()
	}()
	return nil
}

func (b *backgroundSystem) WaitStep(timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !b.StepActive() {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return !b.StepActive()
}

// TestSchedulerRunsOtherSystemsWhileBackgroundStepActive 后台长步骤在途时不占 busy 标记：
// 其它链照常运行，空闲判定仍视其为忙碌。
func TestSchedulerRunsOtherSystemsWhileBackgroundStepActive(t *testing.T) {
	s := newTestScheduler(t, nil)
	bg := &backgroundSystem{id: "storyboard", priority: 200, ready: true, started: make(chan struct{}), block: make(chan struct{})}
	other := &fakeSystem{id: "viewing", priority: 150, ready: true, reason: "slices"}
	s.Register(bg)
	s.Register(other)

	s.Tick()
	<-bg.started
	s.Tick()
	s.wg.Wait()
	if other.runCount() != 1 {
		t.Fatalf("后台步骤在途时其它链应运行，实际 %d 次", other.runCount())
	}
	if !s.hasActiveStep() {
		t.Fatal("后台步骤应被识别为忙碌")
	}
	if s.WaitIdle(30 * time.Millisecond) {
		t.Fatal("后台步骤未结束时不应判定空闲")
	}
	close(bg.block)
	if !s.WaitIdle(time.Second) {
		t.Fatal("后台步骤结束后应判定空闲")
	}
}

// TestSchedulerDialoguePreemptsViewingAcrossTicks 用户消息到达后，下一个 Tick 由对话抢占，
// 回应结束后观影才继续（观影步骤在安全边界让出，任务保留）。
func TestSchedulerDialoguePreemptsViewingAcrossTicks(t *testing.T) {
	s := newTestScheduler(t, nil)
	dialogue := &fakeSystem{id: "dialogue", priority: 300, ready: false, reason: "unread"}
	viewing := &fakeSystem{id: "viewing", priority: 200, ready: true, reason: "slices"}
	s.Register(viewing)
	s.Register(dialogue)

	s.Tick()
	s.wg.Wait()
	if s.state.LastSystem != "viewing" {
		t.Fatalf("没有用户消息时应继续观影，实际 %s", s.state.LastSystem)
	}

	// 用户消息到达：对话就绪，观影虽然仍有待办，但这一 Tick 必须先回应消息。
	dialogue.ready = true
	s.Tick()
	s.wg.Wait()
	if s.state.LastSystem != "dialogue" {
		t.Fatalf("用户消息到达后应优先对话，实际 %s", s.state.LastSystem)
	}
	if dialogue.runCount() != 1 || viewing.runCount() != 1 {
		t.Fatalf("同一 Tick 只运行对话，实际 dialogue=%d viewing=%d", dialogue.runCount(), viewing.runCount())
	}

	// 回应完毕：观影从让出点继续。
	dialogue.ready = false
	s.Tick()
	s.wg.Wait()
	if s.state.LastSystem != "viewing" || viewing.runCount() != 2 {
		t.Fatalf("回应结束后观影应继续，实际 %s（%d 次）", s.state.LastSystem, viewing.runCount())
	}
}

// TestSchedulerPermitAllowsResponseLane 单循环下的特许响应车道：
// 对话步骤阻塞在工具等待时，只有持有特许才允许响应链并行，且生活链始终不得插入。
func TestSchedulerPermitAllowsResponseLane(t *testing.T) {
	permit := tickpermit.New()
	s := New(Hooks[testSnapshot]{
		StateFile: "",
		Snapshot:  func(int) testSnapshot { return testSnapshot{} },
		Permit:    permit,
	})
	dialogueStarted := make(chan struct{})
	dialogueBlock := make(chan struct{})
	dialogue := &fakeSystem{id: "dialogue", priority: 300, ready: true, reason: "unread", started: dialogueStarted, block: dialogueBlock}
	response := &fakeSystem{id: "response", priority: 260, ready: true, reason: "pending"}
	lifeSys := &fakeSystem{id: "life", priority: 100, ready: true, reason: "cooldown"}
	s.Register(lifeSys)
	s.Register(response)
	s.Register(dialogue)

	s.Tick()
	<-dialogueStarted

	// 没有特许时，对话步骤运行期间不允许任何其他链插入。
	s.Tick()
	if response.runCount() != 0 || lifeSys.runCount() != 0 {
		t.Fatalf("无特许时不应插入其他链，response=%d life=%d", response.runCount(), lifeSys.runCount())
	}

	responseStarted := make(chan struct{})
	response.started = responseStarted
	permit.Begin("dialogue", time.Minute)
	s.Tick()
	select {
	case <-responseStarted:
	case <-time.After(time.Second):
		t.Fatal("特许期间响应链未获得执行机会")
	}
	if lifeSys.runCount() != 0 {
		t.Fatalf("特许期间只允许响应链，生活链却运行了 %d 次", lifeSys.runCount())
	}

	if !permit.End(time.Second) {
		t.Fatal("End 应在响应步骤结束后返回")
	}
	close(dialogueBlock)
	s.wg.Wait()
}

// TestSchedulerKeepIdleAdvancesCounter 接管空闲的体系（KeepIdle）运行后冷却 +1 而非重置。
func TestSchedulerKeepIdleAdvancesCounter(t *testing.T) {
	s := newTestScheduler(t, nil)
	system := &fakeSystem{id: "logistics", priority: 250, idleReset: 0, keepIdle: true, ready: true, reason: "salvage"}
	s.Register(system)
	s.state.IdleTicks = 12
	s.Tick()
	s.wg.Wait()
	if s.state.IdleTicks != 13 {
		t.Fatalf("KeepIdle 体系应把冷却推进到 13，实际 %d", s.state.IdleTicks)
	}
}
