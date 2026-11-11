package ManipulateFiles

// tools_test.go 校验按物理行替换：行号指向真实行，而不是镜头编号等业务编号。

import (
	"os"
	"testing"

	"LunarSubsystem/LunarHarness/agents/file/filesupport"
	"LunarSubsystem/LunarHarness/internal/testenv"
)

func TestReplaceByPhysicalLines(t *testing.T) {
	testenv.Isolate(t)
	t.Cleanup(func() { _ = filesupport.Close() })
	if err := os.MkdirAll(filesupport.Abs("documents"), 0o755); err != nil {
		t.Fatal(err)
	}
	rel := "documents/story.md"
	if err := os.WriteFile(filesupport.Abs(rel), []byte("第一行\n第二行\n第三行\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	c := &collector{root: "documents"}
	outcome := replace(map[string]any{
		"path":       rel,
		"start_line": float64(2),
		"end_line":   float64(2),
		"content":    "替换第二行",
	}, c)
	if !outcome.Mutated {
		t.Fatalf("替换应标记为已修改: %+v", outcome)
	}
	got, err := os.ReadFile(filesupport.Abs(rel))
	if err != nil {
		t.Fatal(err)
	}
	want := "第一行\n替换第二行\n第三行\n"
	if string(got) != want {
		t.Fatalf("物理行替换结果不正确：\n got=%q\nwant=%q", got, want)
	}
	if versions, _ := filesupport.List(rel); len(versions) != 1 {
		t.Fatalf("修改前应保存 1 个版本，实际 %d", len(versions))
	}
}

func TestReplaceAllLines(t *testing.T) {
	testenv.Isolate(t)
	t.Cleanup(func() { _ = filesupport.Close() })
	if err := os.MkdirAll(filesupport.Abs("documents"), 0o755); err != nil {
		t.Fatal(err)
	}
	rel := "documents/story.md"
	if err := os.WriteFile(filesupport.Abs(rel), []byte("旧内容\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	c := &collector{root: "documents"}
	outcome := replace(map[string]any{
		"path":    rel,
		"content": "全部替换\n",
		"all":     true,
	}, c)
	if !outcome.Mutated {
		t.Fatalf("全量替换应标记为已修改: %+v", outcome)
	}
	got, err := os.ReadFile(filesupport.Abs(rel))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "全部替换" {
		t.Fatalf("全量替换结果不正确: %q", got)
	}
}
