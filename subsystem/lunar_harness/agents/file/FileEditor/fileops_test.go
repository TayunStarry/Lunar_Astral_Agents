package FileEditor

// fileops_test.go 文件编辑智能体的核心文件操作验收：写入 / 读取 / 置换 / 检索 /
// 移动 / 属性 / 删除，以及「结构化 JSON 必须带 handles 与 summary」这条输出规范。

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"LunarSubsystem/LunarHarness/internal/testenv"
)

func withTempLocalDir(t *testing.T) {
	t.Helper()
	testenv.Isolate(t)
}

// TestCreateFileRejectsDirectoryPath 目录式路径必须以「只能写具体文件」失败，
// 且不能在文档库根目录留下同名 0 字节文件（历史上就是这样把父级写入全部卡死的）。
func TestCreateFileRejectsDirectoryPath(t *testing.T) {
	withTempLocalDir(t)

	created := CreateFile("项目/", "")
	if created.OK {
		t.Fatalf("目录式路径不应创建成功")
	}
	if !strings.Contains(created.Summary, "目录写法") {
		t.Fatalf("错误提示应指出目录写法: %s", created.Summary)
	}

	listed := FileList("")
	if !listed.OK {
		t.Fatalf("列出根目录失败: %s", listed.Summary)
	}
	entries, _ := listed.Detail["entries"].([]fileEntry)
	for _, entry := range entries {
		if entry.Name == "项目" {
			t.Fatalf("目录式创建不应留下同名文件条目")
		}
	}
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

// TestAppendTextKeepsContentAndReportsIncrement 追加只做增量写入：
// 保留原内容、补齐缺失的末尾换行，并返回追加片段的起始行。
func TestAppendTextKeepsContentAndReportsIncrement(t *testing.T) {
	withTempLocalDir(t)

	cases := []struct {
		name      string
		initial   string
		addition  string
		want      string
		wantStart int64
	}{
		{name: "缺少末尾换行", initial: "a\nb", addition: "c", want: "a\nb\nc\n", wantStart: 3},
		{name: "已有末尾换行", initial: "a\n", addition: "c", want: "a\nc\n", wantStart: 2},
		{name: "空文件", initial: "", addition: "c", want: "c\n", wantStart: 1},
	}
	for index, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rel := fmt.Sprintf("notes/append-%d.md", index)
			if created := CreateFile(rel, tc.initial); !created.OK {
				t.Fatalf("创建失败: %s", created.Summary)
			}
			appended := AppendText(rel, tc.addition)
			if !appended.OK {
				t.Fatalf("追加失败: %s", appended.Summary)
			}
			assertStructured(t, appended, ToolAppend)
			if got := toInt64(appended.Detail["start_line"]); got != tc.wantStart {
				t.Fatalf("起始行 = %d，期望 %d", got, tc.wantStart)
			}
			text, err := ReadText(rel)
			if err != nil || text != tc.want {
				t.Fatalf("追加后内容 = %q，期望 %q（%v）", text, tc.want, err)
			}
		})
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

func TestLibraryRunnerURL(t *testing.T) {
	target := libraryRunnerURL("demo.js", "js")
	if !strings.Contains(target, "/library/?") || !strings.Contains(target, "run=documents%2Fdemo.js") {
		t.Fatalf("资源库自动运行地址不正确: %s", target)
	}
	if !strings.Contains(target, "kind=js") {
		t.Fatalf("地址应带上运行类型: %s", target)
	}
}

// TestDocumentPathAcceptsHandleAndPrefix 句柄与 documents/ 前缀都能定位到同一文件。
func TestDocumentPathAcceptsHandleAndPrefix(t *testing.T) {
	testenv.Isolate(t)

	created := CreateFile("notes/a.md", "hello\n")
	if !created.OK || len(created.Handles) == 0 {
		t.Fatalf("创建文件应登记句柄: %s", created.Summary)
	}
	handle := created.Handles[0]

	text, err := ReadText(handle)
	if err != nil || !strings.Contains(text, "hello") {
		t.Fatalf("句柄路径应解析到文档库文件: %q（%v）", text, err)
	}
	if _, err := ReadText(handle + "|§1"); err != nil {
		t.Fatalf("带锚点的句柄也应可解析: %v", err)
	}
	if _, err := ReadText("documents/notes/a.md"); err != nil {
		t.Fatalf("documents/ 前缀应被兼容: %v", err)
	}
	if _, err := ReadText("documents/../documents/notes/a.md"); err == nil {
		t.Fatal("带前缀的 .. 穿越仍应被拒绝")
	}
}
