package FileOperations

// tools_test.go 校验步骤状态的记录与失败识别。

import (
	"fmt"
	"strings"
	"testing"
)

func TestHandlesFromTextAreMergedAndDeduped(t *testing.T) {
	handles := handlesFromText("请改写 @D.078，别碰 @I.002，也不要重复 @D.078")
	if len(handles) != 2 || handles[0] != "@D.078" || handles[1] != "@I.002" {
		t.Fatalf("文本句柄提取不正确: %v", handles)
	}
	merged := mergeHandles([]string{"@D.001", "@D.078"}, handles)
	want := []string{"@D.001", "@D.078", "@I.002"}
	if len(merged) != len(want) {
		t.Fatalf("合并后句柄数量不正确: %v", merged)
	}
	for i, handle := range want {
		if merged[i] != handle {
			t.Fatalf("合并后句柄顺序不正确: %v", merged)
		}
	}
}

func TestProjectFolderFromChangedPaths(t *testing.T) {
	cases := []struct {
		name  string
		paths []string
		want  string
	}{
		{"同一项目", []string{"documents/雨夜旧书店/正文.md", "documents/雨夜旧书店/设定.md"}, "documents/雨夜旧书店"},
		{"Windows 反斜杠", []string{`documents\雨夜旧书店\正文.md`}, "documents/雨夜旧书店"},
		{"多个项目", []string{"documents/甲/正文.md", "documents/乙/正文.md"}, ""},
		{"直接写在 documents 下", []string{"documents/正文.md"}, ""},
		{"项目外文件", []string{"notes/a.md"}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := projectFolder(tc.paths); got != tc.want {
				t.Fatalf("projectFolder(%v)=%q，期望 %q", tc.paths, got, tc.want)
			}
		})
	}
}

func TestDropFailedStepsKeepsSuccessfulWork(t *testing.T) {
	c := &collector{changedPaths: []string{"documents/作品/index.html"}}
	c.steps = []stepResult{
		{Type: "manipulate_files", OK: true, Summary: "已创建 index.html"},
		{Type: "manipulate_files", Summary: "写入失败", Error: "行号越界"},
	}
	c.dropFailedSteps()
	if c.hasFailure() {
		t.Fatal("重试前应清掉上一轮的失败步骤")
	}
	if len(c.steps) != 1 || c.steps[0].Summary != "已创建 index.html" {
		t.Fatalf("成功步骤应保留，实际 %+v", c.steps)
	}
	if len(c.changed()) != 1 {
		t.Fatalf("已修改文件应保留，实际 %v", c.changed())
	}
}

func TestRepairInstructionCarriesFailureAndChangedFiles(t *testing.T) {
	c := &collector{changedPaths: []string{"documents/作品/index.html"}}
	got := repairInstruction("原始任务", fmt.Errorf("工具循环达到轮次上限"), c)
	for _, want := range []string{"原始任务", "工具循环达到轮次上限", "documents/作品/index.html", "只修复未完成或出错的部分"} {
		if !strings.Contains(got, want) {
			t.Fatalf("修复指令缺少 %q：%s", want, got)
		}
	}
}

func TestCollectorTracksStepStatus(t *testing.T) {
	c := &collector{}
	c.steps = []stepResult{
		{Type: "manipulate_files", OK: true, Summary: "已追加内容", Changed: []string{"documents/a.md"}},
	}
	if c.hasFailure() {
		t.Fatal("成功步骤不应被标记为失败")
	}
	text := c.operationsText()
	if !strings.Contains(text, "成功") || strings.Contains(text, "失败") {
		t.Fatalf("操作记录应显示成功: %s", text)
	}

	c.steps = append(c.steps, stepResult{
		Type: "manipulate_files", Summary: "写入失败", Error: "行号越界",
	})
	if !c.hasFailure() {
		t.Fatal("失败步骤应被识别")
	}
	if got := c.failureText(); got != "行号越界" {
		t.Fatalf("失败原因不正确: %q", got)
	}
}
