package forum

// forum_test.go 钉住论坛链的上下文推进：一轮需求→答复必须落库，并在下一轮作为历史传入模型。

import (
	"context"
	"testing"

	"LunarSubsystem/LunarHarness/protocol/agentbase"
)

func TestRunRecordsHistoryAndReplaysIt(t *testing.T) {
	oldRun := runRound
	defer func() { runRound = oldRun }()

	var gotHistory []agentbase.Message
	runRound = func(_ context.Context, instruction string, history []agentbase.Message) (string, error) {
		gotHistory = history
		return "论坛答复：" + instruction, nil
	}

	ResetContext()
	if _, err := Run("先发一个求助"); err != nil {
		t.Fatalf("首轮 Run 失败: %v", err)
	}
	if got := len(HistorySnapshot()); got != 2 {
		t.Fatalf("首轮后上下文应记录 2 条消息，实际 %d", got)
	}

	if _, err := Run("再回一条"); err != nil {
		t.Fatalf("次轮 Run 失败: %v", err)
	}
	if len(gotHistory) != 2 {
		t.Fatalf("次轮应收到首轮的 2 条历史，实际 %d", len(gotHistory))
	}
	if gotHistory[0].Role != "user" || gotHistory[1].Role != "assistant" {
		t.Fatalf("历史角色顺序错误: %#v", gotHistory)
	}

	ResetContext()
	if got := len(HistorySnapshot()); got != 0 {
		t.Fatalf("ResetContext 后应为空，实际 %d", got)
	}
}

func TestRunRejectsEmptyInstruction(t *testing.T) {
	if _, err := Run("   "); err == nil {
		t.Fatal("空指令应被拒绝")
	}
}
