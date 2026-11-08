package tickpermit

// controller_test.go 特许运行的关键不变量：撤销与响应车道的互斥、End 的有界屏障。

import (
	"testing"
	"time"
)

// TestEndWaitsForResponseAndBlocksNewResponse End 必须先撤销许可、等待在途响应结束，
// 且在撤销之后不再允许启动新的响应步骤。
func TestEndWaitsForResponseAndBlocksNewResponse(t *testing.T) {
	controller := New()
	controller.Begin("dialogue", time.Minute)
	if !controller.TryBeginResponse() {
		t.Fatal("特许有效时应允许启动响应步骤")
	}

	done := make(chan bool, 1)
	go func() { done <- controller.End(time.Second) }()

	select {
	case <-done:
		t.Fatal("响应仍在途时 End 不应返回")
	case <-time.After(50 * time.Millisecond):
	}
	if controller.TryBeginResponse() {
		t.Fatal("许可撤销后不应再允许新的响应步骤")
	}
	if controller.Active() {
		t.Fatal("End 开始后许可应立即失效")
	}

	controller.MarkResponse(false)
	select {
	case ok := <-done:
		if !ok {
			t.Fatal("响应结束后 End 应返回 true")
		}
	case <-time.After(time.Second):
		t.Fatal("响应结束后 End 未返回")
	}
}

// TestEndTimeoutStillClearsPermit 等待超时也必须撤销许可，避免响应链卡死对话。
func TestEndTimeoutStillClearsPermit(t *testing.T) {
	controller := New()
	controller.Begin("dialogue", time.Minute)
	controller.TryBeginResponse()

	if controller.End(20 * time.Millisecond) {
		t.Fatal("响应未结束时应超时返回 false")
	}
	if controller.Active() {
		t.Fatal("超时后许可仍应被撤销")
	}
	controller.MarkResponse(false)
}

// TestExpireIfNeededRecoversLeakedPermit 泄漏的特许超时后被回收。
func TestExpireIfNeededRecoversLeakedPermit(t *testing.T) {
	controller := New()
	controller.Begin("dialogue", time.Millisecond)

	expired, owner := controller.ExpireIfNeeded(time.Now().Add(time.Second))
	if !expired || owner != "dialogue" {
		t.Fatalf("应回收超时特许，实际 expired=%v owner=%s", expired, owner)
	}
	if controller.Active() {
		t.Fatal("回收后不应仍处于特许状态")
	}
	if controller.TryBeginResponse() {
		t.Fatal("无特许时不应允许启动响应步骤")
	}
}
