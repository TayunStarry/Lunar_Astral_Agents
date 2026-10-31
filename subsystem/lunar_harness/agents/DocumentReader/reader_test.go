package DocumentReader

// reader_test.go 文献解析智能体的单元测试：格式识别、超大文件分批与读取上限。

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/resource"
)

// TestDetectProfileByContent 扩展名未知时按内容结构识别。
func TestDetectProfileByContent(t *testing.T) {
	cases := []struct {
		name string
		text string
		want string
	}{
		{"笔记.txt", "# 标题\n\n正文段落", "markdown"},
		{"脚本.txt", "package main\n\nfunc main() {}\n", "source"},
		{"数据.txt", "name,age\n张三,18\n李四,20\n", "table"},
		{"配置.txt", "{\"name\": \"lunar\"}", "config"},
		{"小说.txt", "第一章 起点\n他推开门。\n", "prose"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := resource.Resource{Name: tc.name, Ext: ".txt"}
			profile := detectProfile(res, int64(len(tc.text)), tc.text)
			if profile.Kind != tc.want {
				t.Fatalf("%s 识别为 %s，期望 %s", tc.name, profile.Kind, tc.want)
			}
			if profile.Binary {
				t.Fatalf("%s 不应被识别为二进制", tc.name)
			}
		})
	}
}

// TestDetectProfileByExtAndBinary 扩展名与二进制探针优先于内容结构。
func TestDetectProfileByExtAndBinary(t *testing.T) {
	declaration := resource.Resource{Name: "api.d.ts", Ext: ".ts"}
	profile := detectProfile(declaration, 10, "interface Foo {}")
	if profile.Kind != "dts" {
		t.Fatalf(".d.ts 识别为 %s", profile.Kind)
	}

	archive := resource.Resource{Name: "pkg.zip", Ext: ".zip"}
	profile = detectProfile(archive, 10, "PK\x03\x04binary")
	if !profile.Binary || profile.Kind != "archive" {
		t.Fatalf("zip 应识别为二进制压缩包，实际 %+v", profile)
	}

	masked := resource.Resource{Name: "looks.txt", Ext: ".txt"}
	profile = detectProfile(masked, 10, "text\x00more")
	if !profile.Binary {
		t.Fatal("含 NUL 字节的文本必须识别为二进制")
	}
}

// TestBuildChunkPlanCaps 每一片都不能超过 10000 字（单行超长除外）。
func TestBuildChunkPlanCaps(t *testing.T) {
	lines := make([]string, 1000)
	for index := range lines {
		lines[index] = strings.Repeat("字", 50)
	}
	spans := buildChunkPlan(lines)
	if len(spans) < 5 {
		t.Fatalf("51000 字应至少切成 6 片，实际 %d 片", len(spans))
	}
	for _, span := range spans {
		if span.Chars > chunkRunes {
			t.Fatalf("第 %d 片 %d 字，超过单片上限 %d", span.Index, span.Chars, chunkRunes)
		}
		if span.StartLine > span.EndLine {
			t.Fatalf("片区间非法: %+v", span)
		}
	}
}

// TestLoadDocLargeFile 超过 10000 字的单文件必须建立分批计划并标记 Large。
func TestLoadDocLargeFile(t *testing.T) {
	oldDir := config.LocalDir
	config.LocalDir = t.TempDir()
	t.Cleanup(func() { config.LocalDir = oldDir })

	dir := filepath.Join(config.LocalDir, "documents")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	var builder strings.Builder
	for i := 0; i < 500; i++ {
		fmt.Fprintf(&builder, "第 %d 行：%s\n", i, strings.Repeat("内容", 10))
	}
	path := filepath.Join(dir, "big.txt")
	if err := os.WriteFile(path, []byte(builder.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	doc, err := loadDoc(resource.Resource{
		Name: "big.txt", Ext: ".txt", Kind: resource.KindDocument,
		BlobPath: "documents/big.txt", State: resource.StateActive,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !doc.Profile.Large {
		t.Fatalf("应标记为需分批处理，实际 %+v", doc.Profile)
	}
	if len(doc.Chunks) < 2 {
		t.Fatalf("应生成多片计划，实际 %d 片", len(doc.Chunks))
	}
	if doc.Profile.Runes < inlineContentLimit {
		t.Fatalf("样本字符数不足: %d", doc.Profile.Runes)
	}
}

// TestReadLimits 单次读取必须按 300 行 / 10000 字双重限流。
func TestReadLimits(t *testing.T) {
	doc := &docContext{Lines: []string{strings.Repeat("字", maxToolReadChars+5000)}}
	out := readLineRange(doc, 1, 1)
	if len([]rune(out)) > maxToolReadChars+64 {
		t.Fatalf("read_lines 输出超过字符上限: %d", len([]rune(out)))
	}
	out = readChunk(doc, 0)
	if len([]rune(out)) > maxToolReadChars+64 {
		t.Fatalf("read_chunk 输出超过字符上限: %d", len([]rune(out)))
	}

	many := make([]string, 400)
	for index := range many {
		many[index] = "x"
	}
	doc = &docContext{Lines: many}
	out = readLineRange(doc, 1, 400)
	if !strings.HasPrefix(out, "L1-300") {
		t.Fatalf("read_lines 应按 300 行截断，实际开头 %q", out[:12])
	}
}

// TestDetectNovelOutline 小说章节标题必须进入结构大纲。
func TestDetectNovelOutline(t *testing.T) {
	doc := &docContext{
		Lines: []string{"第一章 起点", "他推开门。", "第二章 相遇", "她抬起头。"},
		Profile: formatProfile{
			Ext: ".txt", Kind: "prose", Label: "纯文本",
		},
	}
	items := detectOutline(doc)
	if len(items) != 2 {
		t.Fatalf("应识别 2 个章节，实际 %d", len(items))
	}
	if items[0].Start != 1 || items[1].Start != 3 {
		t.Fatalf("章节行区间错误: %+v", items)
	}
}
