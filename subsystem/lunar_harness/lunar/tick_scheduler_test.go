package lunar

import (
	"context"
	"sync"
	"testing"
	"time"

	"LunarSubsystem/LunarHarness/engine/config"
)

type fakeTickSystem struct {
	id       string
	priority int
	ready    bool
	reason   string

	mu      sync.Mutex
	runs    int
	started chan struct{}
	block   chan struct{}
}

func (f *fakeTickSystem) ID() string    { return f.id }
func (f *fakeTickSystem) Priority() int { return f.priority }
func (f *fakeTickSystem) Ready(TickSnapshot) (bool, string) {
	return f.ready, f.reason
}
func (f *fakeTickSystem) Run(context.Context) error {
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

func (f *fakeTickSystem) runCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.runs
}

func newTestTickScheduler(t *testing.T) *tickScheduler {
	t.Helper()
	oldDir := config.LocalDir
	config.LocalDir = t.TempDir()
	t.Cleanup(func() { config.LocalDir = oldDir })

	oldHook := viewingPendingHook
	viewingPendingHook = func() bool { return false }
	oldIdle := notifyIdleHook
	notifyIdleHook = func() {}
	t.Cleanup(func() {
		viewingPendingHook = oldHook
		notifyIdleHook = oldIdle
	})

	unreadMu.Lock()
	unreadContext = nil
	unreadMu.Unlock()

	return &tickScheduler{
		ctx:    context.Background(),
		cancel: func() {},
		state:  TickState{Version: 1},
	}
}

func TestSchedulerPriorityAndIdleReset(t *testing.T) {
	s := newTestTickScheduler(t)
	dialogue := &fakeTickSystem{id: "dialogue", priority: 300, ready: true, reason: "unread"}
	viewing := &fakeTickSystem{id: "viewing", priority: 200, ready: true, reason: "slices"}
	life := &fakeTickSystem{id: "life", priority: 100, ready: true, reason: "cooldown"}
	s.Register(life)
	s.Register(viewing)
	s.Register(dialogue)

	s.state.IdleTicks = 905
	s.Tick()
	if s.state.LastSystem != "dialogue" {
		t.Fatalf("应优先触发对话，实际 %s", s.state.LastSystem)
	}
	if s.state.IdleTicks != 0 {
		t.Fatalf("触发后空闲计数应清零，实际 %d", s.state.IdleTicks)
	}
	s.wg.Wait()
	if dialogue.runCount() != 1 || viewing.runCount() != 0 || life.runCount() != 0 {
		t.Fatal("同一 Tick 只允许一个体系运行")
	}
}

func TestSchedulerBusySkipsTick(t *testing.T) {
	s := newTestTickScheduler(t)
	started := make(chan struct{})
	block := make(chan struct{})
	system := &fakeTickSystem{id: "dialogue", priority: 300, ready: true, reason: "unread", started: started, block: block}
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
	s := newTestTickScheduler(t)
	count := 0
	notifyIdleHook = func() { count++ }
	s.Register(&fakeTickSystem{id: "dialogue", priority: 300, ready: false})
	s.Tick()
	if count != 1 {
		t.Fatalf("空闲 Tick 应通知桥接器 1 次，实际 %d", count)
	}
}

func TestSchedulerDoesNotNotifyBridgeWhenTriggering(t *testing.T) {
	s := newTestTickScheduler(t)
	count := 0
	notifyIdleHook = func() { count++ }
	s.Register(&fakeTickSystem{id: "dialogue", priority: 300, ready: true})
	s.Tick()
	s.wg.Wait()
	if count != 0 {
		t.Fatalf("触发体系的 Tick 不应通知桥接器，实际 %d", count)
	}
}

func TestSchedulerIdleCounterAndThreshold(t *testing.T) {
	s := newTestTickScheduler(t)
	life := &fakeTickSystem{id: "life", priority: 100, ready: false}
	s.Register(&fakeTickSystem{id: "dialogue", priority: 300, ready: false})
	s.Register(&fakeTickSystem{id: "viewing", priority: 200, ready: false})
	s.Register(life)

	for i := 0; i < tickIdleThreshold; i++ {
		s.Tick()
	}
	if s.state.IdleTicks != tickIdleThreshold {
		t.Fatalf("空闲计数应为 %d，实际 %d", tickIdleThreshold, s.state.IdleTicks)
	}
	if life.runCount() != 0 {
		t.Fatal("达到阈值前不应触发生活")
	}
	life.ready = true
	s.Tick()
	if s.state.LastSystem != "life" {
		t.Fatalf("达到阈值后应触发生活，实际 %s", s.state.LastSystem)
	}
	s.wg.Wait()
}

func TestSchedulerRegisterExtension(t *testing.T) {
	s := newTestTickScheduler(t)
	s.Register(&fakeTickSystem{id: "low", priority: 10})
	s.Register(&fakeTickSystem{id: "high", priority: 900})
	s.Register(&fakeTickSystem{id: "mid", priority: 500})
	if len(s.systems) != 3 {
		t.Fatalf("应注册 3 个体系，实际 %d", len(s.systems))
	}
	if s.systems[0].ID() != "high" || s.systems[1].ID() != "mid" || s.systems[2].ID() != "low" {
		t.Fatalf("优先级排序错误: %s / %s / %s", s.systems[0].ID(), s.systems[1].ID(), s.systems[2].ID())
	}
}
