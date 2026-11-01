package FileEditor

// pathfind_test.go 文件编辑的路径定位验收：三种路径形式 + 句柄 + 歧义候选。

import (
	"strings"
	"testing"

	"LunarSubsystem/LunarHarness/engine/resource"
)

// TestReadByThreePathForms 完整路径 / 相对路径 / 纯文件名都能读到同一文件。
func TestReadByThreePathForms(t *testing.T) {
	prepareScheduleDir(t)

	created := CreateFile("notes/a.md", "alpha")
	if !created.OK {
		t.Fatalf("创建文件失败: %s", created.Summary)
	}
	for _, form := range []string{"documents/notes/a.md", "notes/a.md", "a.md", "A.MD"} {
		text, err := ReadText(form)
		if err != nil || !strings.Contains(text, "alpha") {
			t.Fatalf("%q 读取失败: %q（%v）", form, text, err)
		}
	}
	_, handle, err := resource.RegisterPath("documents/notes/a.md")
	if err != nil || handle == "" {
		t.Fatalf("登记句柄失败: %v / %q", err, handle)
	}
	if text, err := ReadText(handle); err != nil || !strings.Contains(text, "alpha") {
		t.Fatalf("句柄读取失败: %q（%v）", text, err)
	}
}

// TestReadAmbiguousFileName 同名文件必须返回候选列表，而不是随便挑一个。
func TestReadAmbiguousFileName(t *testing.T) {
	prepareScheduleDir(t)

	for _, path := range []string{"notes/a.md", "archive/a.md"} {
		if result := CreateFile(path, "content"); !result.OK {
			t.Fatalf("创建 %s 失败: %s", path, result.Summary)
		}
	}
	_, err := ReadText("a.md")
	if err == nil {
		t.Fatal("同名文件应返回歧义错误")
	}
	message := err.Error()
	for _, candidate := range []string{"documents/notes/a.md", "documents/archive/a.md"} {
		if !strings.Contains(message, candidate) {
			t.Fatalf("歧义提示缺少候选 %s：%s", candidate, message)
		}
	}
}
