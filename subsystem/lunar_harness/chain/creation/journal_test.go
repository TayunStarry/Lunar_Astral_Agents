package creation

// journal_test.go 两种模式的跨轮可见规则：自由创作全量读取，指派模式只注入上一轮步骤与错误。

import (
	"strings"
	"testing"

	"LunarSubsystem/LunarHarness/engine/fileflow"
)

// sampleChanges 构造上一轮的内容级变更。
func sampleChanges() []fileflow.ChangeRecord {
	return []fileflow.ChangeRecord{
		{Seq: 1, Kind: fileflow.DeltaModify, PathRel: "documents/notes/a.md",
			Locator: "替换 原第 2-2 行 → 新第 2-2 行", OldText: "旧内容", NewText: "新内容"},
		{Seq: 2, Kind: fileflow.DeltaArchive, PathRel: "documents/pack.zip",
			Binary: true, BinaryNote: "压缩包写入：新增条目 a.txt", OldSize: 10, NewSize: 20},
	}
}

// sampleJournal 构造一份含全部规划字段的日志视图。
func sampleJournal(mode fileflow.Mode) journal {
	return journal{
		Version:     1,
		Mode:        string(mode),
		Directives:  []string{"用户要求写一篇文章"},
		LastPlans:   []planStep{{Agent: agentWriter, Task: "写文章"}},
		Memos:       []string{"下一轮继续润色"},
		References:  []string{"参考资料 A"},
		Errors:      []string{"上次执行失败"},
		LastSummary: "上一轮收尾",
		Changes:     sampleChanges(),
	}
}

// TestAssignedJournalHidesCreatorFields 指派模式只注入上一轮步骤与错误，其他字段不可见。
func TestAssignedJournalHidesCreatorFields(t *testing.T) {
	text := formatJournal(sampleJournal(fileflow.ModeAssigned))
	if !strings.Contains(text, "上一轮指派任务的步骤") || !strings.Contains(text, "写文章") {
		t.Fatalf("指派模式应注入上一轮步骤: %q", text)
	}
	if !strings.Contains(text, "上次执行失败") {
		t.Fatalf("指派模式应注入上一轮错误: %q", text)
	}
	if !strings.Contains(text, "change_query") {
		t.Fatalf("指派模式应提示可按路径查询变更: %q", text)
	}
	for _, banned := range []string{"参考资料 A", "下一轮继续润色", "上一轮收尾", "旧内容"} {
		if strings.Contains(text, banned) {
			t.Fatalf("指派模式不得带入 %q: %q", banned, text)
		}
	}
}

// TestCreatorJournalShowsFullRecord 自由创作模式注入规划、备忘、参考、错误与上一轮内容差异。
func TestCreatorJournalShowsFullRecord(t *testing.T) {
	text := formatJournal(sampleJournal(fileflow.ModeCreator))
	for _, want := range []string{"上一轮规划", "参考资料 A", "下一轮继续润色", "上次执行失败",
		"上一轮文件变更的内容差异", "documents/notes/a.md", "旧内容", "新内容",
		"压缩包写入：新增条目 a.txt", "上一轮收尾"} {
		if !strings.Contains(text, want) {
			t.Fatalf("自由创作模式应包含 %q: %q", want, text)
		}
	}
}

// TestFormatChangesCapsOldGroups 提示词侧只注入最近 maxContextOperations 次操作。
func TestFormatChangesCapsOldGroups(t *testing.T) {
	records := make([]fileflow.ChangeRecord, 0, maxContextOperations+5)
	for i := 0; i < maxContextOperations+5; i++ {
		records = append(records, fileflow.ChangeRecord{
			Seq: i + 1, Kind: fileflow.DeltaModify, PathRel: "documents/notes/a.md",
			OldText: "旧", NewText: "新",
		})
	}
	text := formatChanges(records)
	if strings.Contains(text, "#1 ") {
		t.Fatalf("超出上限的旧变更不应注入: %q", text)
	}
	if !strings.Contains(text, "#"+itoa(maxContextOperations+5)) {
		t.Fatalf("应保留最新变更: %q", text)
	}
}

// TestJournalFromRoundMapsCall 库快照应正确映射为日志视图。
func TestJournalFromRoundMapsCall(t *testing.T) {
	prev := fileflow.RoundSnapshot{
		Call:       fileflow.CallRecord{CallID: "c1", Mode: fileflow.ModeCreator, Summary: "收尾"},
		Steps:      []fileflow.RoundStep{{Ordinal: 0, Agent: agentWriter, Task: "写文章"}},
		References: []string{"参考"},
		Memos:      []string{"备忘"},
		Errors:     []string{"错误"},
		Changes:    sampleChanges(),
	}
	j := journalFromRound(prev)
	if j.Mode != string(fileflow.ModeCreator) || len(j.LastPlans) != 1 || len(j.References) != 1 ||
		len(j.Memos) != 1 || len(j.Errors) != 1 || len(j.Changes) != 2 || j.LastSummary != "收尾" {
		t.Fatalf("快照映射不完整: %+v", j)
	}
}

// itoa 小工具，避免测试文件引入 strconv 仅用于数字拼接。
func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	out := ""
	for value > 0 {
		out = string(rune('0'+value%10)) + out
		value /= 10
	}
	return out
}
