package lunarloop

import (
	"testing"

	"LunarSubsystem/GeneralConfig"
)

// resetEngineStatusForTest 清空自愈状态与暂存快照（各子测试互不干扰；测试间不并发）
func resetEngineStatusForTest() {
	statusMu.Lock()
	engineStatus = EngineStatus{}
	statusMu.Unlock()
	snapshotMu.Lock()
	pendingSnapshot = ""
	snapshotMu.Unlock()
}

// 覆盖：连续失败计数与"成功即清零"。
//
// 这条计数是自动重载的唯一判据：数错了会有两种坏结果——
// 数得太快（把一次抖动当持续故障）会白白丢掉上下文；
// 数得太慢（或不归零）会让引擎在一次失败后再也不自动重载。
func TestAgentFailureCounter(t *testing.T) {
	resetEngineStatusForTest()
	// 关掉自动重载：本用例只验证计数，不想真的去重建运行时
	savedAuto, savedThreshold := *GeneralConfig.EngineAutoReload, *GeneralConfig.EngineAutoReloadFailures
	*GeneralConfig.EngineAutoReload = false
	defer func() {
		*GeneralConfig.EngineAutoReload = savedAuto
		*GeneralConfig.EngineAutoReloadFailures = savedThreshold
	}()

	NotifyAgentFailure("Could not establish connection")
	NotifyAgentFailure("Could not establish connection")
	state := EngineState()
	if state.ConsecutiveFailures != 2 {
		t.Fatalf("连续失败应为 2，实得 %d", state.ConsecutiveFailures)
	}
	if state.LastFailure != "Could not establish connection" {
		t.Fatalf("最近失败原因应保留，实得 %q", state.LastFailure)
	}
	if state.LastFailureAt.IsZero() {
		t.Fatal("最近失败时刻应被记录（排查「什么时候开始连不上」靠它）")
	}

	NotifyAgentIdle()
	if got := EngineState().ConsecutiveFailures; got != 0 {
		t.Fatalf("成功一次后应清零，实得 %d", got)
	}
}

// 覆盖：重载记账——次数、原因、结果、启动时刻，以及"重载成功清零失败计数"。
func TestRecordReloadUpdatesStatus(t *testing.T) {
	resetEngineStatusForTest()

	recordReload("手动重载（控制台）", true, "运行时就绪；模型服务可用；上下文：已恢复 3 条")
	state := EngineState()
	if state.ReloadCount != 1 {
		t.Fatalf("重载次数应为 1，实得 %d", state.ReloadCount)
	}
	if state.LastReloadReason != "手动重载（控制台）" {
		t.Fatalf("重载原因不符: %q", state.LastReloadReason)
	}
	if state.LastReloadResult == "" {
		t.Fatal("重载结果应被记录（否则出事后无从判断重载是否真的成功）")
	}
	if state.BootedAt.IsZero() {
		t.Fatal("成功重载应更新启动时刻")
	}

	// 先制造"连续失败"，再记录一次成功重载：计数必须归零
	statusMu.Lock()
	engineStatus.ConsecutiveFailures = 3
	statusMu.Unlock()
	recordReload("自动重载", true, "ok")
	if got := EngineState().ConsecutiveFailures; got != 0 {
		t.Fatalf("重载成功应清零连续失败，实得 %d", got)
	}
}

// 覆盖：重载失败也要记账（否则界面可能一直显示上一次的"成功"）。
func TestRecordReloadFailureKeepsBootTime(t *testing.T) {
	resetEngineStatusForTest()
	recordReload("手动重载", true, "ok")
	booted := EngineState().BootedAt
	recordReload("手动重载", false, "加载 agentSystem.js 失败: boom")
	state := EngineState()
	if state.ReloadCount != 2 {
		t.Fatalf("失败也应计入重载次数，实得 %d", state.ReloadCount)
	}
	if !state.BootedAt.Equal(booted) {
		t.Fatal("重载失败不应刷新启动时刻（上一次成功启动仍然有效）")
	}
}

// 覆盖：暂存快照是"一次性"的——取走即清空，避免新运行时被重复回灌旧上下文。
func TestPendingSnapshotIsOneShot(t *testing.T) {
	resetEngineStatusForTest()
	setPendingSnapshot(`{"kind":"lunar-agent-recovery","version":1}`)
	if got := takePendingSnapshot(); got == "" {
		t.Fatal("应取到暂存快照")
	}
	if got := takePendingSnapshot(); got != "" {
		t.Fatalf("快照应只取一次，第二次实得 %q", got)
	}
}
