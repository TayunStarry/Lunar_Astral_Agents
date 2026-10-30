package console

// editshow_test.go「编辑显示」上报状态的行为验收。

import (
	"testing"
	"time"
)

func TestPublishFileEditKeepsSnippetOnRead(t *testing.T) {
	ClearFileEdits()
	t.Cleanup(ClearFileEdits)

	now := time.Now()
	PublishFileEdit(FileEditNotice{
		Path: "documents/notes/a.md", Op: "replace", Snippet: "月华", StartLine: 2, SnippetLines: 1, At: now,
	})
	touched, _, _ := editSnapshot()
	status, ok := touched["notes/a.md"]
	if !ok {
		t.Fatalf("上报路径未规整到 documents 相对路径: %v", touched)
	}
	if status.Snippet != "月华" || status.StartLine != 2 {
		t.Fatalf("改动片段未记录: %+v", status)
	}

	// 后续的读取类操作不应抹掉绿色高亮片段
	PublishFileEdit(FileEditNotice{Path: "notes/a.md", Op: "read", At: now})
	touched, _, _ = editSnapshot()
	if got := touched["notes/a.md"]; got.Snippet != "月华" || got.Op != "read" {
		t.Fatalf("读取操作应保留改动片段且更新操作名: %+v", got)
	}
}

func TestEditTouchedSurvivesAcrossFiles(t *testing.T) {
	ClearFileEdits()
	t.Cleanup(ClearFileEdits)

	PublishFileEdit(FileEditNotice{Path: "a.js", Op: "write", Snippet: "x", StartLine: 1, SnippetLines: 1})
	PublishFileEdit(FileEditNotice{Path: "b.js", Op: "read"})
	touched, _, _ := editSnapshot()
	if len(touched) != 2 {
		t.Fatalf("本次运行周期内的触碰文件应全部保留，实得 %d", len(touched))
	}
	if touched["a.js"].Snippet != "x" {
		t.Fatalf("切换文件后首个文件的改动片段被抹掉了: %+v", touched["a.js"])
	}
}

// TestRunFailureMarksRedUntilNextSuccessfulRun 钉住「编辑显示」文件名着色：
// 运行失败 → 红色；之后的读取/写入不清除红色；再次运行成功才恢复。
func TestRunFailureMarksRedUntilNextSuccessfulRun(t *testing.T) {
	ClearFileEdits()
	t.Cleanup(ClearFileEdits)

	PublishFileEdit(FileEditNotice{Path: "demo.js", Op: "run", Failed: true})
	if got := editTouched["demo.js"]; !got.Failed {
		t.Fatalf("运行失败应标记 Failed: %+v", got)
	}

	PublishFileEdit(FileEditNotice{Path: "demo.js", Op: "read"})
	if got := editTouched["demo.js"]; !got.Failed {
		t.Fatalf("后续读取不应清除运行失败标记: %+v", got)
	}

	PublishFileEdit(FileEditNotice{Path: "demo.js", Op: "run", Failed: false})
	if got := editTouched["demo.js"]; got.Failed {
		t.Fatalf("运行成功后应清除失败标记: %+v", got)
	}
}
