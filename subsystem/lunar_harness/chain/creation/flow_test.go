package creation

// flow_test.go 创作管线写操作的审查与回滚验收：规划预期命中时通过，不命中时回滚。

import (
	"os"
	"path/filepath"
	"testing"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/fileflow"
	"LunarSubsystem/LunarHarness/executors/ArchiveReader"
	"LunarSubsystem/LunarHarness/internal/testenv"
)

// TestMutationReviewPassAndRollback 写操作执行后自动审查：通过时保留，不通过时回滚到操作前版本。
func TestMutationReviewPassAndRollback(t *testing.T) {
	testenv.Isolate(t)
	t.Cleanup(func() { _ = fileflow.CloseDeltaStore() })
	flowID := fileflow.NewFlowID()
	s := &session{
		mode:      fileflow.ModeAssigned,
		flowID:    flowID,
		flow:      fileflow.NewContext(fileflow.ModeAssigned, flowID),
		stepIndex: 0,
	}

	created := decodeFlowOutcome(t, s.fileCreate(map[string]any{"path": "notes/review.md", "content": "初稿"}))
	if ok, _ := created["ok"].(bool); !ok {
		t.Fatalf("创建文件失败: %#v", created)
	}

	// 规划预期：操作后必须包含「预期内容」
	s.current = &planStep{Expect: []fileflow.Expect{{
		Path:     "/document/notes/review.md",
		Contains: []string{"预期内容"},
	}}}

	passed := decodeFlowOutcome(t, s.fileRewrite(map[string]any{"path": "notes/review.md", "content": "预期内容"}))
	review, _ := passed["review"].(map[string]any)
	if ok, _ := review["ok"].(bool); !ok {
		t.Fatalf("符合预期时审查应通过: %#v", passed)
	}

	failed := decodeFlowOutcome(t, s.fileRewrite(map[string]any{"path": "notes/review.md", "content": "错误内容"}))
	review, _ = failed["review"].(map[string]any)
	if ok, _ := review["ok"].(bool); ok {
		t.Fatalf("不符合预期时审查必须失败: %#v", failed)
	}
	if rolledBack, _ := review["rolled_back"].(bool); !rolledBack {
		t.Fatalf("审查失败后必须回滚: %#v", review)
	}
	content, err := os.ReadFile(filepath.Join(config.LocalDir, "documents", "notes", "review.md"))
	if err != nil {
		t.Fatalf("读取回滚后的文件失败: %v", err)
	}
	if string(content) != "预期内容" {
		t.Fatalf("回滚应恢复到操作前版本，实际 %q", string(content))
	}

	// 内容级增量必须落库：通过的一次保留修改片段，失败的一次标记为已回滚
	ops, err := fileflow.FlowOps(flowID)
	if err != nil {
		t.Fatalf("读取增量记录失败: %v", err)
	}
	if len(ops) != 3 {
		t.Fatalf("三次写操作都应落库，实际 %d 条", len(ops))
	}
	passedOp := ops[1]
	if passedOp.DeltaKind != fileflow.DeltaModify || passedOp.ReviewStatus != fileflow.ReviewPassed {
		t.Fatalf("通过的操作应记录为 modify 增量: %+v", passedOp)
	}
	replaced := false
	for _, delta := range passedOp.Deltas {
		if delta.NewText == "预期内容" {
			replaced = true
		}
	}
	if !replaced {
		t.Fatalf("增量记录应包含替换后的内容片段: %+v", passedOp.Deltas)
	}
	failedOp := ops[2]
	if failedOp.ReviewStatus != fileflow.ReviewRolledBack || !failedOp.RollbackOK {
		t.Fatalf("回滚的操作应记录回滚结果: %+v", failedOp)
	}
}

// TestArchiveWriteReviewRollback 压缩包写入同样走快照与审查，不通过时回滚整个压缩包。
func TestArchiveWriteReviewRollback(t *testing.T) {
	prepareRouteDir(t)
	t.Cleanup(func() { _ = fileflow.CloseDeltaStore() })
	flowID := fileflow.NewFlowID()
	s := &session{
		mode:      fileflow.ModeAssigned,
		flowID:    flowID,
		flow:      fileflow.NewContext(fileflow.ModeAssigned, flowID),
		stepIndex: 0,
	}
	exists := true
	s.current = &planStep{Expect: []fileflow.Expect{{
		Path:     "/document/pkg.zip",
		Exists:   &exists,
		Contains: []string{"压缩包里不该出现的内容"},
	}}}
	outcome := decodeFlowOutcome(t, s.archiveWrite(map[string]any{
		"action": "add", "target": "/document/pkg.zip", "entry": "note.txt", "content": "临时内容",
	}))
	review, _ := outcome["review"].(map[string]any)
	if ok, _ := review["ok"].(bool); ok {
		t.Fatalf("压缩包内容不满足预期时审查必须失败: %#v", outcome)
	}
	if rolledBack, _ := review["rolled_back"].(bool); !rolledBack {
		t.Fatalf("压缩包写入审查失败后必须回滚: %#v", review)
	}
	entries, err := ArchiveReader.ListEntries("/document/pkg.zip")
	if err != nil {
		t.Fatalf("回滚后压缩包不可读: %v", err)
	}
	for _, entry := range entries {
		if entry.Name == "note.txt" {
			t.Fatalf("回滚后不应保留新增条目: %+v", entries)
		}
	}
}
