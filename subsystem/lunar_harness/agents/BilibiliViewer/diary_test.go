package BilibiliViewer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/persistence"
)

// setupDiaryTest 把 LocalDir 与资源目录数据库指到临时目录，
// 避免测试把日记文档与句柄写进真实 local_data。
func setupDiaryTest(t *testing.T) {
	t.Helper()
	tmp := t.TempDir()
	origLocal := config.LocalDir
	origArchive := config.ArchiveDBPath
	origMedia := config.MediaDBPath
	config.LocalDir = tmp
	config.ArchiveDBPath = filepath.Join(tmp, "database", "archive.db")
	config.MediaDBPath = filepath.Join(tmp, "database", "media.db")
	t.Cleanup(func() {
		// 先关库再还原配置，否则 Windows 上临时目录无法删除
		_ = persistence.CloseArchiveDB()
		_ = persistence.CloseDomainDBs()
		config.LocalDir = origLocal
		config.ArchiveDBPath = origArchive
		config.MediaDBPath = origMedia
	})
}

// TestBuildVideoLines 钉住视频段格式：- 标题 / * 链接，条目之间加分隔线，重复链接只留一条。
func TestBuildVideoLines(t *testing.T) {
	videos := []diaryVideo{
		{Title: "标题甲", URL: "https://www.bilibili.com/video/BV1xx411c7mD"},
		{Title: "标题乙", URL: "https://www.bilibili.com/video/BV1yy411c7mE"},
		{Title: "标题甲", URL: "https://www.bilibili.com/video/BV1xx411c7mD"},
	}
	lines := buildVideoLines(videos)
	want := []string{
		"- 《标题甲》", "* https://www.bilibili.com/video/BV1xx411c7mD",
		diaryVideoSeparator,
		"- 《标题乙》", "* https://www.bilibili.com/video/BV1yy411c7mE",
	}
	if strings.Join(lines, "\n") != strings.Join(want, "\n") {
		t.Fatalf("视频段格式不符:\n实际: %q\n期望: %q", lines, want)
	}
}

// TestNormalizeDynamic 钉住动态排版：去掉 @、压缩连续空行、最多 10 行。
func TestNormalizeDynamic(t *testing.T) {
	got := normalizeDynamic("\n\n第一句 @某人\n\n\n第二句\n第三句\n")
	if got != "第一句 某人\n\n第二句\n第三句" {
		t.Fatalf("动态规整不符: %q", got)
	}
	var long strings.Builder
	for i := 0; i < 20; i++ {
		long.WriteString("第几行\n")
	}
	lines := strings.Split(normalizeDynamic(long.String()), "\n")
	if len(lines) != dynamicSummaryMaxLines {
		t.Fatalf("超长动态应截断为 %d 行，实际 %d 行", dynamicSummaryMaxLines, len(lines))
	}
}

// TestWriteDiaryFileSectionsAndRange 钉住三段模板与句柄区间：
// 句柄区间从「刚刚观看的视频」开始，到「动态日记」最后一行结束。
func TestWriteDiaryFileSectionsAndRange(t *testing.T) {
	setupDiaryTest(t)

	videos := []diaryVideo{
		{Title: "标题甲", URL: "https://www.bilibili.com/video/BV1xx411c7mD"},
		{Title: "标题乙", URL: "https://www.bilibili.com/video/BV1yy411c7mE"},
	}
	handle, start, end, err := writeDiaryFile(videos, nil, "今天看了两个视频。\n\n都很喜欢。", "我记得和甲聊过两句。")
	if err != nil {
		t.Fatalf("写入日记失败: %v", err)
	}
	if !strings.HasPrefix(handle, "@D.") {
		t.Fatalf("句柄格式不对: %q", handle)
	}

	raw, err := os.ReadFile(filepath.Join(config.LocalDir, filepath.FromSlash(diaryDocRel)))
	if err != nil {
		t.Fatalf("读回日记失败: %v", err)
	}
	content := string(raw)
	for _, heading := range []string{diaryFileTitle, diaryVideosHeading, diaryDynamicHeading, diaryMemoryHeading} {
		if !strings.Contains(content, heading) {
			t.Fatalf("日记缺少标题 %q:\n%s", heading, content)
		}
	}
	if !strings.Contains(content, "* https://www.bilibili.com/video/BV1yy411c7mE") {
		t.Fatalf("日记缺少视频链接:\n%s", content)
	}

	lines := strings.Split(strings.TrimSuffix(content, "\n"), "\n")
	if start < 1 || end > len(lines) || start > end {
		t.Fatalf("句柄区间越界: start=%d end=%d total=%d", start, end, len(lines))
	}
	if lines[start-1] != diaryVideosHeading {
		t.Fatalf("区间起点应为视频段标题，实际 %q", lines[start-1])
	}
	if lines[end-1] != "都很喜欢。" {
		t.Fatalf("区间终点应为动态最后一行，实际 %q", lines[end-1])
	}
	if strings.Contains(strings.Join(lines[start-1:end], "\n"), "我记得和甲聊过两句。") {
		t.Fatalf("历史记忆不应落在句柄区间内")
	}

	sections := loadDiarySections()
	if len(sections.Videos) == 0 || !strings.Contains(strings.Join(sections.Videos, "\n"), "标题甲") {
		t.Fatalf("视频段解析失败: %q", sections.Videos)
	}
	if sections.Dynamic != "今天看了两个视频。\n\n都很喜欢。" {
		t.Fatalf("动态段解析失败: %q", sections.Dynamic)
	}
	if sections.Memory != "我记得和甲聊过两句。" {
		t.Fatalf("记忆段解析失败: %q", sections.Memory)
	}
}

// TestWriteDiaryFileKeepsOldSections 钉住「没看视频/没发动态就沿用上一版」：
// 传入空视频与上一版视频行时，视频段保持；动态由调用方传入上一版时保持。
func TestWriteDiaryFileKeepsOldSections(t *testing.T) {
	setupDiaryTest(t)

	keptVideos := []string{"- 《旧视频》", "* https://www.bilibili.com/video/BV1old411c7mD"}
	if _, _, _, err := writeDiaryFile(nil, keptVideos, "上一版动态", "上一版记忆"); err != nil {
		t.Fatalf("写入日记失败: %v", err)
	}
	sections := loadDiarySections()
	if strings.Join(sections.Videos, "\n") != strings.Join(keptVideos, "\n") {
		t.Fatalf("没看视频时应保留上一版视频段，实际 %q", sections.Videos)
	}
	if sections.Dynamic != "上一版动态" {
		t.Fatalf("动态段应保留上一版，实际 %q", sections.Dynamic)
	}
}

// TestLoadDiaryLegacy 钉住旧模板迁移：没有三段标题时整篇收进 Legacy。
func TestLoadDiaryLegacy(t *testing.T) {
	setupDiaryTest(t)

	abs := filepath.Join(config.LocalDir, filepath.FromSlash(diaryDocRel))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatalf("建目录失败: %v", err)
	}
	legacy := "## 2026-10-01 12:00 刷B站日记\n《旧视频》 https://www.bilibili.com/video/BV1old411c7mD\n很好看的。"
	if err := os.WriteFile(abs, []byte(legacy), 0o644); err != nil {
		t.Fatalf("写入旧日记失败: %v", err)
	}
	sections := loadDiarySections()
	if sections.Legacy == "" || sections.Memory != "" {
		t.Fatalf("旧模板应整体收进 Legacy，实际 %#v", sections)
	}
}

// TestClampMemory 钉住历史记忆上限：超长时截断。
func TestClampMemory(t *testing.T) {
	long := strings.Repeat("记", memoryMaxRunes+100)
	got := clampMemory(long)
	if runes := []rune(got); len(runes) > memoryMaxRunes+1 {
		t.Fatalf("记忆应截断到 %d 字左右，实际 %d", memoryMaxRunes, len(runes))
	}
}

// TestMentionAtFields 钉住评论 @ 字段的规则：
// at_name/at_mid 只收录配置了 UID 的目标（真实 @ 关系）。
func TestMentionAtFields(t *testing.T) {
	targets := []MentionTarget{
		{Name: "甲", UID: 1001},
		{Name: "乙", UID: 0},
	}
	names, mids := mentionAtFields(targets)
	if names != "甲" || mids != "1001" {
		t.Fatalf("只有带 UID 的目标才进 at 字段，实际 %q / %q", names, mids)
	}
	if names, mids := mentionAtFields(nil); names != "" || mids != "" {
		t.Fatalf("没有目标时不应产生 at 字段，实际 %q / %q", names, mids)
	}
}
