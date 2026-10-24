package BilibiliLife

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

// TestNormalizeBriefNoPaddingAndCap 钉住简述规则：去空行、不补齐、10 行只是硬上限。
func TestNormalizeBriefNoPaddingAndCap(t *testing.T) {
	if got := normalizeBrief("只有一句"); len(got) != 1 {
		t.Fatalf("单行简述不应被补齐，实际 %d 行", len(got))
	}
	if got := normalizeBrief("  第一句  \n\n\n第二句\n"); len(got) != 2 || got[0] != "第一句" || got[1] != "第二句" {
		t.Fatalf("空行应剔除、内容不应错位，实际 %q", got)
	}

	var long strings.Builder
	for i := 0; i < 20; i++ {
		long.WriteString("第几行\n")
	}
	if got := normalizeBrief(long.String()); len(got) != dynamicSummaryMaxLines {
		t.Fatalf("超长简述应截断为 %d 行，实际 %d 行", dynamicSummaryMaxLines, len(got))
	}
}

// TestDiaryAndDynamicShareSameCopy 钉住「日记与动态是同一份文案」：
// 文档段落去掉 ## 前缀后必须与动态正文逐行一致，行数也一致。
func TestDiaryAndDynamicShareSameCopy(t *testing.T) {
	setupDiaryTest(t)

	title := "2000-01-02 00:00 刷B站日记"
	videos := []diaryVideo{
		{Title: "标题甲", URL: "https://www.bilibili.com/video/BV1xx411c7mD"},
		{Title: "标题乙", URL: "https://www.bilibili.com/video/BV1yy411c7mE"},
	}
	brief := "第一个视频的画面很有冲击力。\n评论区吵得挺热闹。"

	handle, lines, err := appendDiaryDocument(title, videos, brief)
	if err != nil {
		t.Fatalf("写入日记失败: %v", err)
	}
	if !strings.HasPrefix(handle, "@D.") {
		t.Fatalf("句柄格式不对: %q", handle)
	}
	if lines != 1+len(videos)+2 {
		t.Fatalf("本段应为 1 标题+%d 条视频+2 行简述，实际 %d 行", len(videos), lines)
	}

	raw, err := os.ReadFile(filepath.Join(config.LocalDir, filepath.FromSlash(diaryDocRel)))
	if err != nil {
		t.Fatalf("读回日记文档失败: %v", err)
	}
	docText := strings.TrimSuffix(string(raw), "\n")
	// 文档 = "## " + 动态正文（同一份行列表，只在标题前多一个标题标记）
	wantDoc := "## " + buildDynamicText(title, videos, brief)
	if docText != wantDoc {
		t.Fatalf("文档与动态不是同一份文案:\n文档: %q\n期望: %q", docText, wantDoc)
	}
	// 每个视频的标题与 URL 必须写在同一行，且本轮看过的视频都要出现
	for _, video := range videos {
		want := "《" + video.Title + "》 " + video.URL
		if !strings.Contains(docText, want) {
			t.Fatalf("日记缺少标题与 URL 同行的一行 %q", want)
		}
	}
}

// TestAppendDiaryDocumentNewestOnTop 钉住「新日记写在旧日记前面、段长按实际行数计算」。
func TestAppendDiaryDocumentNewestOnTop(t *testing.T) {
	setupDiaryTest(t)

	oldTitle := "2000-01-01 00:00 刷B站日记"
	newTitle := "2000-01-02 00:00 刷B站日记"
	videos := []diaryVideo{{Title: "标题甲", URL: "https://www.bilibili.com/video/BV1xx411c7mD"}}

	oldHandle, _, err := appendDiaryDocument(oldTitle, videos, "旧日记一行\n第二行")
	if err != nil {
		t.Fatalf("写入旧日记失败: %v", err)
	}
	newHandle, newLines, err := appendDiaryDocument(newTitle, videos, "新日记一行")
	if err != nil {
		t.Fatalf("写入新日记失败: %v", err)
	}
	// 同名文档复用同一句柄：@D.xxx 不随内容变化，回执长期有效
	if oldHandle != newHandle {
		t.Fatalf("同名文档应复用同一句柄，实际 %q / %q", oldHandle, newHandle)
	}
	// 新段 1 标题 + 1 链接 + 1 行简述
	if newLines != 3 {
		t.Fatalf("新段应为 3 行，实际 %d 行", newLines)
	}

	raw, err := os.ReadFile(filepath.Join(config.LocalDir, filepath.FromSlash(diaryDocRel)))
	if err != nil {
		t.Fatalf("读回日记文档失败: %v", err)
	}
	content := string(raw)
	if !strings.HasPrefix(content, "## "+newTitle) {
		t.Fatalf("新日记未置顶，文件开头: %q", truncate(content, 60))
	}
	if strings.Index(content, "## "+newTitle) > strings.Index(content, "## "+oldTitle) {
		t.Fatalf("新日记应写在旧日记前面")
	}
	// 旧段标题应紧跟在新段（3 行）之后，即第 4 行
	oldLine := len(strings.Split(content[:strings.Index(content, "## "+oldTitle)], "\n"))
	if oldLine != newLines+1 {
		t.Fatalf("旧日记标题应位于第 %d 行，实际第 %d 行", newLines+1, oldLine)
	}
}

// TestBuildDynamicText 钉住动态正文格式：标题 + 每条视频《标题》 URL 同行 + 简述（不补齐、10 行硬上限）。
func TestBuildDynamicText(t *testing.T) {
	title := "2026-10-22 12:00 刷B站日记"
	videos := []diaryVideo{
		{Title: "标题甲", URL: "https://www.bilibili.com/video/BV1xx411c7mD"},
		{Title: "标题乙", URL: "https://www.bilibili.com/video/BV1yy411c7mE"},
	}

	// 常规简述（3 行）就输出几行，绝不用满 10 行
	short := buildDynamicText(title, videos, "第一个视频的画面很有冲击力。\n评论区吵得挺热闹。\n总之看得很开心。")
	shortLines := strings.Split(short, "\n")
	if len(shortLines) != 1+len(videos)+3 {
		t.Fatalf("应为 1 标题+%d 条视频+3 行简述，实际 %d 行：%q", len(videos), len(shortLines), short)
	}
	wantFirst := "《" + videos[0].Title + "》 " + videos[0].URL
	wantSecond := "《" + videos[1].Title + "》 " + videos[1].URL
	if shortLines[0] != title || shortLines[1] != wantFirst || shortLines[2] != wantSecond {
		t.Fatalf("标题/视频行不符: %q", shortLines[:3])
	}
	if strings.Contains(short, "\n\n") {
		t.Fatalf("动态不应出现空行: %q", short)
	}

	// 重复 URL 只列一次，空标题也保留 URL
	dup := buildDynamicText(title, []diaryVideo{
		{Title: "标题甲", URL: videos[0].URL},
		{},
		{Title: "标题甲", URL: videos[0].URL},
	}, "一句")
	if got := len(strings.Split(dup, "\n")); got != 3 {
		t.Fatalf("重复/空视频应被去掉，实际 %d 行：%q", got, dup)
	}

	// 简述异常过长时才按 10 行硬截断（上限不是目标）
	var long strings.Builder
	for i := 1; i <= 20; i++ {
		long.WriteString("第")
		long.WriteString(strings.Repeat("一", i))
		long.WriteString("句\n")
	}
	if got := len(strings.Split(buildDynamicText(title, videos, long.String()), "\n")); got != 1+len(videos)+dynamicSummaryMaxLines {
		t.Fatalf("超长简述应截断为 %d 行，实际 %d 行", 1+len(videos)+dynamicSummaryMaxLines, got)
	}

	// 没有视频时只剩标题 + 简述
	bare := buildDynamicText(title, nil, "只有一句感受")
	if strings.Split(bare, "\n")[1] != "只有一句感受" {
		t.Fatalf("无视频时第二行应是简述，实际 %q", bare)
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
