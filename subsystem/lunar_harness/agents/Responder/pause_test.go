package Responder

// pause_test.go 维护态：Drain 不得在暂停期间取走队列中的请求（否则会拉起模型）。

import (
	"context"
	"encoding/json"
	"testing"
)

func TestDrainPausedKeepsQueue(t *testing.T) {
	// 清理可能残留的队列项，避免测试间相互影响
	for len(inbox) > 0 {
		<-inbox
	}
	Pause()
	defer Resume()
	defer func() {
		for len(inbox) > 0 {
			<-inbox
		}
	}()

	request := &Request{
		Messages: json.RawMessage(`[{"role":"user","content":"hello"}]`),
		Reply:    make(chan Result, 1),
	}
	if err := Enqueue(request); err != nil {
		t.Fatalf("入队失败: %v", err)
	}
	Drain(context.Background())
	if got := Pending(); got != 1 {
		t.Fatalf("维护态下 Drain 不应消费请求，期望排队 1 条，实得 %d", got)
	}
}
