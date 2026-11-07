package Musician

// dump_test.go 演奏者的导出与诊断登记：Inspect 必须读自己的上下文。

import (
	"strings"
	"testing"

	"LunarSubsystem/LunarHarness/protocol/agentrepair"
)

// TestRepairInspectUsesOwnContext 注册后 Inspect 返回演奏者自己的上下文。
func TestRepairInspectUsesOwnContext(t *testing.T) {
	recordRun("写一段温柔的钢琴小曲", Result{
		Title:       "月光练习曲",
		Description: "钢琴 · 100 BPM · C",
		Duration:    42.5,
		AudioURL:    "/music/test.mp3",
		Summary:     "月华演奏了《月光练习曲》",
	}, nil)

	out, err := agentrepair.Inspect(agentID, 0)
	if err != nil {
		t.Fatalf("演奏者应已登记修复目标: %v", err)
	}
	if !strings.Contains(out, `"kind": "musician-context"`) {
		t.Fatalf("Inspect 未返回演奏者自己的上下文: %s", out)
	}
	if !strings.Contains(out, "月光练习曲") {
		t.Fatalf("Inspect 应包含本轮曲目: %s", out)
	}
	if strings.Contains(out, `"lunar"`) {
		t.Fatalf("Inspect 不应回落到对话者上下文: %s", out)
	}
}
