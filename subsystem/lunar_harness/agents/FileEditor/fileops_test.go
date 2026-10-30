package FileEditor

// fileops_test.go 文件编辑智能体的核心文件操作验收：写入 / 读取 / 置换 / 检索 /
// 移动 / 属性 / 删除，以及「结构化 JSON 必须带 handles 与 summary」这条输出规范。

import (
	"encoding/json"
	"strings"
	"testing"

	"LunarSubsystem/LunarHarness/engine/config"
)

func withTempLocalDir(t *testing.T) {
	t.Helper()
	old := config.LocalDir
	config.LocalDir = t.TempDir()
	t.Cleanup(func() { config.LocalDir = old })
}

func TestFileEditorFileLifecycle(t *testing.T) {
	withTempLocalDir(t)

	created := CreateFile("notes/a.md", "hello\nworld\n")
	if !created.OK {
		t.Fatalf("创建失败: %s", created.Summary)
	}
	assertStructured(t, created, ToolCreate)

	read := ReadFile("notes/a.md", 0, 0)
	if !read.OK {
		t.Fatalf("读取失败: %s", read.Summary)
	}
	if content, _ := read.Detail["content"].(string); !strings.Contains(content, "world") {
		t.Fatalf("读取内容不含 world: %q", content)
	}
	assertStructured(t, read, ToolRead)

	replaced := ReplaceAt("notes/a.md", 2, 2, "月华")
	if !replaced.OK {
		t.Fatalf("置换失败: %s", replaced.Summary)
	}
	assertStructured(t, replaced, ToolReplace)
	if text, err := ReadText("notes/a.md"); err != nil || !strings.Contains(text, "月华") {
		t.Fatalf("置换后内容不符: %q (%v)", text, err)
	}

	hits := SearchContent("notes/a.md", "月华")
	if !hits.OK {
		t.Fatalf("检索失败: %s", hits.Summary)
	}
	if count := toInt64(hits.Detail["count"]); count < 1 {
		t.Fatalf("检索应命中至少 1 行，实得 %d", count)
	}

	info := FileAttr("notes/a.md")
	if !info.OK {
		t.Fatalf("属性读取失败: %s", info.Summary)
	}
	assertStructured(t, info, ToolInfo)

	moved := MoveFile("notes/a.md", "archive/a.md")
	if !moved.OK {
		t.Fatalf("移动失败: %s", moved.Summary)
	}
	if _, err := ReadText("archive/a.md"); err != nil {
		t.Fatalf("移动后目标不可读: %v", err)
	}

	deleted := DeleteFile("archive/a.md")
	if !deleted.OK {
		t.Fatalf("删除失败: %s", deleted.Summary)
	}
}

// assertStructured 校验所有工具返回值都是「带 handles / summary 的 JSON 字符串」。
func assertStructured(t *testing.T, result OpResult, wantOp string) {
	t.Helper()
	raw := result.JSON()
	var parsed map[string]any
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		t.Fatalf("%s 返回值不是合法 JSON: %v（%s）", wantOp, err, raw)
	}
	if _, ok := parsed["handles"]; !ok {
		t.Fatalf("%s 返回值缺少 handles: %s", wantOp, raw)
	}
	if _, ok := parsed["summary"].(string); !ok {
		t.Fatalf("%s 返回值缺少 summary: %s", wantOp, raw)
	}
	if op, _ := parsed["op"].(string); op != wantOp {
		t.Fatalf("%s 返回值 op 字段为 %q: %s", wantOp, op, raw)
	}
}

func TestHarnessMarkDetection(t *testing.T) {
	if !hasHarnessMark("\ufeff  \n//Engine: Lunar Harness\nconsole.log(1)") {
		t.Fatal("应识别文件头部的 Lunar Harness 标识")
	}
	if hasHarnessMark("// 普通脚本\nconsole.log(1)") {
		t.Fatal("无标识的脚本不应被判为 Lunar Harness")
	}
}

func TestJSRunnerTemplateContainsCode(t *testing.T) {
	page := jsRunnerTemplate("demo.js", `console.log("hi")`)
	if !strings.Contains(page, `console.log("hi")`) {
		t.Fatal("模板网页必须内联原始脚本，否则纯 js 无法运行")
	}
	if !strings.Contains(page, "console") {
		t.Fatal("模板网页必须接管 console 输出以便展示运行结果")
	}
}
