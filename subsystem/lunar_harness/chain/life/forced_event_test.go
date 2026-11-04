package life

// forced_event_test.go 工具触发强制事件：跳过错权选择、携带自然语言要求。

import (
	"context"
	"encoding/json"
	"testing"

	"LunarSubsystem/LunarHarness/engine/config"
)

type forcedTestEvent struct{ lastInstruction string }

func (e *forcedTestEvent) ID() string       { return "forced_test" }
func (e *forcedTestEvent) Weight() int      { return 1 }
func (e *forcedTestEvent) DailyQuota() int  { return 1 }
func (e *forcedTestEvent) NewRun() EventRun { return &forcedTestRun{event: e} }

type forcedTestRun struct {
	event       *forcedTestEvent
	instruction string
}

func (r *forcedTestRun) Start(_ context.Context, env Env) error {
	r.event.lastInstruction = env.Instruction
	return nil
}

func (r *forcedTestRun) Next(_ context.Context, _ Env) (EventStep, error) {
	return EventStep{Done: true, Record: "强制事件完成", Detail: "forced"}, nil
}

func (r *forcedTestRun) Snapshot() json.RawMessage {
	raw, _ := json.Marshal(map[string]string{"instruction": r.event.lastInstruction})
	return raw
}

func (r *forcedTestRun) Restore(raw json.RawMessage) error { return nil }

func resetLifeForTest(t *testing.T) {
	t.Helper()
	oldDir := config.LocalDir
	config.LocalDir = t.TempDir()
	t.Cleanup(func() {
		config.LocalDir = oldDir
		mu.Lock()
		registry = nil
		state = State{Version: 1, Used: map[string]int{}}
		mu.Unlock()
	})
	Init()
}

func TestRequestEventCarriesInstructionAndClears(t *testing.T) {
	resetLifeForTest(t)
	event := &forcedTestEvent{}
	RegisterEvent(event)
	if !RequestEvent("forced_test", "用户想看猫咪视频") {
		t.Fatal("强制事件登记失败")
	}
	if !HasPendingEvent() {
		t.Fatal("登记后应有待执行事件")
	}

	env := Env{}
	mu.Lock()
	taken := takeForcedEventLocked(&env)
	mu.Unlock()
	if taken == nil || taken.ID() != "forced_test" {
		t.Fatalf("应取出强制事件，实际 %v", taken)
	}
	if env.Instruction != "用户想看猫咪视频" {
		t.Fatalf("自然语言要求未传递: %q", env.Instruction)
	}
	if HasPendingEvent() {
		t.Fatal("取出后应清空待执行事件")
	}
}

func TestStepPrefersForcedEventOverWeights(t *testing.T) {
	resetLifeForTest(t)
	event := &forcedTestEvent{}
	RegisterEvent(event)
	if !RequestEvent("forced_test", "写一段关于星空的诗") {
		t.Fatal("强制事件登记失败")
	}
	if err := Step(context.Background()); err != nil {
		t.Fatal(err)
	}
	if event.lastInstruction != "写一段关于星空的诗" {
		t.Fatalf("Step 未把指令交给事件: %q", event.lastInstruction)
	}
	mu.Lock()
	record := state.LastRecord
	active := state.Active
	mu.Unlock()
	if active != nil {
		t.Fatal("强制事件应已完成，不应留下未完成运行")
	}
	if record != "强制事件完成" {
		t.Fatalf("生活记录应为强制事件结果，实际 %q", record)
	}
}

func TestRequestEventRejectsUnknown(t *testing.T) {
	resetLifeForTest(t)
	if RequestEvent("missing", "随便") {
		t.Fatal("未注册事件不应登记成功")
	}
	if HasPendingEvent() {
		t.Fatal("未注册事件不应留下待执行状态")
	}
}
