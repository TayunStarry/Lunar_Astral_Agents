package creation

// round_test.go 创作链路从统一库读取上一轮的验收：自由创作可见全量，指派模式只见步骤与错误。

import (
	"strings"
	"testing"

	"LunarSubsystem/LunarHarness/engine/fileflow"
	"LunarSubsystem/LunarHarness/internal/testenv"
)

// TestLoadRoundJournalFreeMode 自由创作下一轮读取 references / memos / 上一轮步骤与错误。
func TestLoadRoundJournalFreeMode(t *testing.T) {
	testenv.Isolate(t)
	t.Cleanup(func() { _ = fileflow.CloseDeltaStore() })

	callID, err := fileflow.BeginCall(fileflow.ModeCreator)
	if err != nil {
		t.Fatalf("开始轮次失败: %v", err)
	}
	if err := fileflow.FinishRound(callID, fileflow.ModeCreator, fileflow.CallStatusDone, "收尾",
		[]fileflow.RoundStep{{Agent: agentWriter, Task: "写文章"}},
		[]string{"参考"}, []string{"备忘"}, []string{"错误"}); err != nil {
		t.Fatalf("归档轮次失败: %v", err)
	}
	if _, err := fileflow.BeginCall(fileflow.ModeCreator); err != nil {
		t.Fatalf("开始下一轮失败: %v", err)
	}

	history := loadRoundJournal(fileflow.ModeCreator)
	if len(history.LastPlans) != 1 || len(history.References) != 1 ||
		len(history.Memos) != 1 || len(history.Errors) != 1 || history.LastSummary != "收尾" {
		t.Fatalf("自由创作应读取全量跨轮信息: %+v", history)
	}
	text := formatJournal(history)
	for _, want := range []string{"上一轮规划", "参考资料", "备忘", "写文章"} {
		if !strings.Contains(text, want) {
			t.Fatalf("自由创作上下文应包含 %q: %q", want, text)
		}
	}
}

// TestLoadRoundJournalAssignedMode 指派模式下一轮只读步骤与错误，参考 / 备忘不落库。
func TestLoadRoundJournalAssignedMode(t *testing.T) {
	testenv.Isolate(t)
	t.Cleanup(func() { _ = fileflow.CloseDeltaStore() })

	callID, err := fileflow.BeginCall(fileflow.ModeAssigned)
	if err != nil {
		t.Fatalf("开始轮次失败: %v", err)
	}
	if err := fileflow.FinishRound(callID, fileflow.ModeAssigned, fileflow.CallStatusDone, "不该保存的收尾",
		[]fileflow.RoundStep{{Agent: agentEngineer, Task: "实现功能"}},
		[]string{"参考"}, []string{"备忘"}, []string{"错误"}); err != nil {
		t.Fatalf("归档轮次失败: %v", err)
	}
	if _, err := fileflow.BeginCall(fileflow.ModeAssigned); err != nil {
		t.Fatalf("开始下一轮失败: %v", err)
	}

	history := loadRoundJournal(fileflow.ModeAssigned)
	if len(history.References) != 0 || len(history.Memos) != 0 || history.LastSummary != "" {
		t.Fatalf("指派模式不得读取参考 / 备忘 / 收尾: %+v", history)
	}
	if len(history.LastPlans) != 1 || len(history.Errors) != 1 {
		t.Fatalf("指派模式应读取步骤与错误: %+v", history)
	}
	text := formatJournal(history)
	if !strings.Contains(text, "上一轮指派任务的步骤") || !strings.Contains(text, "实现功能") {
		t.Fatalf("指派模式上下文应包含步骤: %q", text)
	}
	if !strings.Contains(text, "change_query") {
		t.Fatalf("指派模式应提示可查询文件变更: %q", text)
	}
	if strings.Contains(text, "参考") || strings.Contains(text, "备忘") {
		t.Fatalf("指派模式不得带入参考 / 备忘: %q", text)
	}
}
