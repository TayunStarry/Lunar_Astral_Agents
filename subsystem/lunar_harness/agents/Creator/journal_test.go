package Creator

// journal_test.go 两种上下文留存模式的验收：Creator 全量保存，指派模式只留内容级操作记录。

import (
	"strings"
	"testing"

	"LunarSubsystem/LunarHarness/engine/fileflow"
)

// sampleJournal 构造一份含全部规划字段的日志。
func sampleJournal() journal {
	return journal{
		Version:     1,
		Directives:  []string{"用户要求写一篇文章"},
		LastPlans:   []planStep{{Agent: agentWriter, Task: "写文章"}},
		Memos:       []string{"下一轮继续润色"},
		References:  []string{"参考资料 A"},
		Errors:      []string{"上次执行失败"},
		LastSummary: "上一轮收尾",
	}
}

// sampleOps 构造内容级操作记录。
func sampleOps() []fileflow.OpRecord {
	return []fileflow.OpRecord{{
		Seq: 1, Op: fileflow.OpWrite, PathRel: "document/notes/a.md", ReviewStatus: fileflow.ReviewPassed,
		Deltas: []fileflow.Delta{{
			Kind: fileflow.DeltaModify, PathRel: "document/notes/a.md",
			Locator: "替换 原第 2-2 行 → 新第 2-2 行",
			OldText: "旧内容", NewText: "新内容",
		}},
	}}
}

// TestAssignedJournalKeepsOnlyOperations 指派模式不得把规划、备忘、参考与错误带进下一次任务。
func TestAssignedJournalKeepsOnlyOperations(t *testing.T) {
	pruned := pruneJournal(sampleJournal(), fileflow.ModeAssigned)
	if len(pruned.LastPlans) != 0 || len(pruned.Memos) != 0 || len(pruned.References) != 0 ||
		len(pruned.Errors) != 0 || len(pruned.Directives) != 0 || pruned.LastSummary != "" {
		t.Fatalf("指派模式应只保留操作内容差异，实际 %+v", pruned)
	}
	text := formatJournal(pruned, sampleOps())
	if !strings.Contains(text, "内容差异") || !strings.Contains(text, "notes/a.md") {
		t.Fatalf("指派模式上下文应只含内容级操作记录: %q", text)
	}
	if !strings.Contains(text, "旧内容") || !strings.Contains(text, "新内容") {
		t.Fatalf("指派模式应注入替换前后片段: %q", text)
	}
	if strings.Contains(text, "上一轮规划") || strings.Contains(text, "参考资料") {
		t.Fatalf("指派模式不得带入规划与参考资料: %q", text)
	}
}

// TestCreatorJournalKeepsFullRecord Creator 模式必须完整保留规划、备忘、参考与错误，并注入最近内容差异。
func TestCreatorJournalKeepsFullRecord(t *testing.T) {
	pruned := pruneJournal(sampleJournal(), fileflow.ModeCreator)
	if len(pruned.LastPlans) != 1 || len(pruned.Memos) != 1 || len(pruned.References) != 1 ||
		len(pruned.Errors) != 1 || len(pruned.Directives) != 1 || pruned.LastSummary == "" {
		t.Fatalf("Creator 模式应保留完整流程记录，实际 %+v", pruned)
	}
	text := formatJournal(pruned, sampleOps())
	if !strings.Contains(text, "上一轮规划") || !strings.Contains(text, "参考资料") ||
		!strings.Contains(text, "历史操作的内容差异") {
		t.Fatalf("Creator 模式上下文应包含完整章节与内容差异: %q", text)
	}
}

// TestFormatRecentOpsShowsContentAndCaps 提示词侧只取最近若干条，并展示二进制结论。
func TestFormatRecentOpsShowsContentAndCaps(t *testing.T) {
	ops := make([]fileflow.OpRecord, 0, maxContextOperations+5)
	for i := 0; i < maxContextOperations+5; i++ {
		ops = append(ops, fileflow.OpRecord{
			Seq: i + 1, Op: fileflow.OpArchiveWrite, PathRel: "document/pack.zip",
			Deltas: []fileflow.Delta{{
				Kind: fileflow.DeltaArchive, PathRel: "document/pack.zip",
				Binary: true, BinaryNote: "压缩包写入：新增条目 a.txt", OldSize: 10, NewSize: 20,
			}},
		})
	}
	text := formatRecentOps(ops)
	if strings.Count(text, "#") < maxContextOperations {
		t.Fatalf("应注入最近 %d 条，实际 %q", maxContextOperations, text)
	}
	if !strings.Contains(text, "压缩包写入：新增条目 a.txt") {
		t.Fatalf("二进制记录应注入可读差异结论: %q", text)
	}
	// 越过上限的早期记录不进入提示词
	if strings.Contains(text, "#1 ") {
		t.Fatalf("超出上限的旧记录不应注入: %q", text)
	}
}
