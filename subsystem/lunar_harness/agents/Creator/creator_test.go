package Creator

import (
	"strings"
	"testing"

	"LunarSubsystem/LunarHarness/engine/config"
)

func TestDocumentPathRejectsEscape(t *testing.T) {
	config.LocalDir = t.TempDir()
	for _, rel := range []string{"../secret.txt", "a/../../b.txt", "/etc/passwd", "C:/Windows/win.ini"} {
		if _, _, err := documentPath(rel); err == nil {
			t.Fatalf("路径 %q 应被拒绝", rel)
		}
	}
	if _, _, err := documentPath("notes/a.md"); err != nil {
		t.Fatalf("合法路径不应被拒绝: %v", err)
	}
}

func TestDocumentFileLifecycle(t *testing.T) {
	oldDir := config.LocalDir
	config.LocalDir = t.TempDir()
	t.Cleanup(func() { config.LocalDir = oldDir })

	if _, err := createDocument("notes/a.md", "hello world", true); err != nil {
		t.Fatal(err)
	}
	content, err := readDocument("notes/a.md")
	if err != nil {
		t.Fatal(err)
	}
	if content != "hello world" {
		t.Fatalf("读取内容不符: %q", content)
	}
	if _, err := createDocument("notes/a.md", "重复创建", true); err == nil {
		t.Fatal("已存在的文件应拒绝 file_create")
	}
	if _, err := replaceDocument("notes/a.md", "world", "月华", false); err != nil {
		t.Fatal(err)
	}
	if content, _ := readDocument("notes/a.md"); content != "hello 月华" {
		t.Fatalf("替换结果不符: %q", content)
	}
	if _, err := deleteDocumentText("notes/a.md", "hello ", false); err != nil {
		t.Fatal(err)
	}
	if err := moveDocument("notes/a.md", "archive/a.md"); err != nil {
		t.Fatal(err)
	}
	if content, err := readDocument("archive/a.md"); err != nil || content != "月华" {
		t.Fatalf("移动后读取失败: %q, %v", content, err)
	}
	if err := removeDocumentContent("archive/a.md"); err != nil {
		t.Fatal(err)
	}
	if content, _ := readDocument("archive/a.md"); content != "" {
		t.Fatalf("清空后应为空，实际 %q", content)
	}
	if err := deleteDocument("archive/a.md"); err != nil {
		t.Fatal(err)
	}
}

func TestBuildCodeIndex(t *testing.T) {
	source := `// 全局配置
const config = { mode: "ltp9" };

/**
 * 求两数之和
 */
function add(a, b) {
  var local = a + b;
  return local;
}

/* 事件处理器 */
const onMessage = (event) => { return event; };

// 创作画布
class Canvas {
  draw() {}
}
`
	index := buildCodeIndex(source)
	if len(index.Variables) != 1 || index.Variables[0].Name != "config" || index.Variables[0].Comment != "全局配置" {
		t.Fatalf("全局变量索引异常: %#v", index.Variables)
	}
	if len(index.Functions) != 2 {
		t.Fatalf("函数索引数量异常: %#v", index.Functions)
	}
	if index.Functions[0].Name != "add" || !strings.Contains(index.Functions[0].Comment, "求两数之和") {
		t.Fatalf("函数签名或注释异常: %#v", index.Functions[0])
	}
	if len(index.Classes) != 1 || index.Classes[0].Name != "Canvas" || index.Classes[0].Comment != "创作画布" {
		t.Fatalf("类索引异常: %#v", index.Classes)
	}
}

func TestParsePlanValidation(t *testing.T) {
	valid := map[string]any{
		"steps": []any{
			map[string]any{"agent": agentResearcher, "task": "查阅 DTS 文档"},
			map[string]any{"agent": agentWriter, "task": "写一段故事"},
		},
		"memos": []any{"下一轮继续润色"},
	}
	plan, err := parsePlan(valid)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 2 || plan.Steps[0].Agent != agentResearcher || len(plan.Memos) != 1 {
		t.Fatalf("计划解析异常: %#v", plan)
	}

	if _, err := parsePlan(map[string]any{"steps": []any{}}); err == nil {
		t.Fatal("空计划应被拒绝")
	}
	if _, err := parsePlan(map[string]any{"steps": []any{
		map[string]any{"agent": "unknown", "task": "做事"},
	}}); err == nil {
		t.Fatal("非法智能体应被拒绝")
	}
	if _, err := parsePlan(map[string]any{"steps": []any{
		map[string]any{"agent": agentWriter, "task": "   "},
	}}); err == nil {
		t.Fatal("空任务描述应被拒绝")
	}
}

func TestJournalReferencesAreCapped(t *testing.T) {
	s := &session{}
	for i := 0; i < maxReferences+5; i++ {
		s.addReference(strings.Repeat("参考", i+1))
	}
	if len(s.refs) != maxReferences {
		t.Fatalf("参考资料应限制为 %d 条，实际 %d", maxReferences, len(s.refs))
	}
	for _, ref := range s.refs {
		if len([]rune(ref)) > maxReferenceRunes+1 {
			t.Fatalf("参考资料未按长度截断: %d", len([]rune(ref)))
		}
	}
}
