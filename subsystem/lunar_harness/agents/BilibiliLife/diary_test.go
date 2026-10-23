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
	origLocal := *config.LocalDir
	origArchive := *config.ArchiveDBPath
	origMedia := *config.MediaDBPath
	*config.LocalDir = tmp
	*config.ArchiveDBPath = filepath.Join(tmp, "database", "archive.db")
	*config.MediaDBPath = filepath.Join(tmp, "database", "media.db")
	t.Cleanup(func() {
		// 先关库再还原配置，否则 Windows 上临时目录无法删除
		_ = persistence.CloseArchiveDB()
		_ = persistence.CloseDomainDBs()
		*config.LocalDir = origLocal
		*config.ArchiveDBPath = origArchive
		*config.MediaDBPath = origMedia
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
	urls := []string{
		"https://www.bilibili.com/video/BV1xx411c7mD",
		"https://www.bilibili.com/video/BV1yy411c7mE",
	}
	brief := "第一个视频的画面很有冲击力。\n评论区吵得挺热闹。"

	handle, lines, err := appendDiaryDocument(title, urls, brief)
	if err != nil {
		t.Fatalf("写入日记失败: %v", err)
	}
	if !strings.HasPrefix(handle, "@D.") {
		t.Fatalf("句柄格式不对: %q", handle)
	}
	if lines != 1+len(urls)+2 {
		t.Fatalf("本段应为 1 标题+%d 条链接+2 行简述，实际 %d 行", len(urls), lines)
	}

	raw, err := os.ReadFile(filepath.Join(*config.LocalDir, filepath.FromSlash(diaryDocRel)))
	if err != nil {
		t.Fatalf("读回日记文档失败: %v", err)
	}
	docText := strings.TrimSuffix(string(raw), "\n")
	// 文档 = "## " + 动态正文（同一份行列表，只在标题前多一个标题标记）
	wantDoc := "## " + buildDynamicText(title, urls, brief)
	if docText != wantDoc {
		t.Fatalf("文档与动态不是同一份文案:\n文档: %q\n期望: %q", docText, wantDoc)
	}
	// 本轮看过的每个视频都要有链接，不只是推荐的那一条
	for _, url := range urls {
		if !strings.Contains(docText, url) {
			t.Fatalf("日记缺少视频链接 %s", url)
		}
	}
}

// TestAppendDiaryDocumentNewestOnTop 钉住「新日记写在旧日记前面、段长按实际行数计算」。
func TestAppendDiaryDocumentNewestOnTop(t *testing.T) {
	setupDiaryTest(t)

	oldTitle := "2000-01-01 00:00 刷B站日记"
	newTitle := "2000-01-02 00:00 刷B站日记"
	urls := []string{"https://www.bilibili.com/video/BV1xx411c7mD"}

	oldHandle, _, err := appendDiaryDocument(oldTitle, urls, "旧日记一行\n第二行")
	if err != nil {
		t.Fatalf("写入旧日记失败: %v", err)
	}
	newHandle, newLines, err := appendDiaryDocument(newTitle, urls, "新日记一行")
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

	raw, err := os.ReadFile(filepath.Join(*config.LocalDir, filepath.FromSlash(diaryDocRel)))
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

// TestBuildDynamicText 钉住动态正文格式：标题 + 全部视频链接 + 简述（不补齐、10 行硬上限）。
func TestBuildDynamicText(t *testing.T) {
	title := "2026-10-22 12:00 刷B站日记"
	urls := []string{
		"https://www.bilibili.com/video/BV1xx411c7mD",
		"https://www.bilibili.com/video/BV1yy411c7mE",
	}

	// 常规简述（3 行）就输出几行，绝不用满 10 行
	short := buildDynamicText(title, urls, "第一个视频的画面很有冲击力。\n评论区吵得挺热闹。\n总之看得很开心。")
	shortLines := strings.Split(short, "\n")
	if len(shortLines) != 1+len(urls)+3 {
		t.Fatalf("应为 1 标题+%d 链接+3 行简述，实际 %d 行：%q", len(urls), len(shortLines), short)
	}
	if shortLines[0] != title || shortLines[1] != urls[0] || shortLines[2] != urls[1] {
		t.Fatalf("标题/链接行不符: %q", shortLines[:3])
	}
	if strings.Contains(short, "\n\n") {
		t.Fatalf("动态不应出现空行: %q", short)
	}

	// 重复链接只列一次，空串跳过
	dup := buildDynamicText(title, []string{urls[0], "", urls[0]}, "一句")
	if got := len(strings.Split(dup, "\n")); got != 3 {
		t.Fatalf("重复/空链接应被去掉，实际 %d 行：%q", got, dup)
	}

	// 简述异常过长时才按 10 行硬截断（上限不是目标）
	var long strings.Builder
	for i := 1; i <= 20; i++ {
		long.WriteString("第")
		long.WriteString(strings.Repeat("一", i))
		long.WriteString("句\n")
	}
	if got := len(strings.Split(buildDynamicText(title, urls, long.String()), "\n")); got != 1+len(urls)+dynamicSummaryMaxLines {
		t.Fatalf("超长简述应截断为 %d 行，实际 %d 行", 1+len(urls)+dynamicSummaryMaxLines, got)
	}

	// 没有视频链接时只剩标题 + 简述
	bare := buildDynamicText(title, nil, "只有一句感受")
	if strings.Split(bare, "\n")[1] != "只有一句感受" {
		t.Fatalf("无链接时第二行应是简述，实际 %q", bare)
	}
}

// TestBuildDynamicTextWithCall 钉住「仅在动态中呼叫用户」的文案规则：
// 呼叫必须单独占最后一行，且没有目标时与普通动态正文完全一致。
func TestBuildDynamicTextWithCall(t *testing.T) {
	title := "2026-10-22 12:00 刷B站日记"
	urls := []string{"https://www.bilibili.com/video/BV1xx411c7mD"}
	brief := "第一个视频的画面很有冲击力。"

	plain := buildDynamicText(title, urls, brief)
	withCall := buildDynamicTextWithCall(title, urls, brief, []string{"甲", " 乙 ", ""})
	lines := strings.Split(withCall, "\n")
	if lines[len(lines)-1] != "@甲 @乙" {
		t.Fatalf("呼叫应单独占最后一行，实际最后一行 %q（全文 %q）", lines[len(lines)-1], withCall)
	}
	if strings.Join(lines[:len(lines)-1], "\n") != plain {
		t.Fatalf("呼叫行不应改动原动态正文:\n got: %q\nwant: %q", withCall, plain)
	}
	if got := buildDynamicTextWithCall(title, urls, brief, nil); got != plain {
		t.Fatalf("没有呼叫目标时应与普通动态一致，实际 %q", got)
	}
}

// TestDiaryReceiptText 钉住固定回执文案（行号按本段实际行数填写）。
func TestDiaryReceiptText(t *testing.T) {
	got := diaryReceiptText("@D.001", 4)
	want := "刚刚刷了一会B站，日记写在 @D.001|L1-4 了，可用于回溯刷了什么视频；想聊就说说刚看了什么"
	if got != want {
		t.Fatalf("回执文案不符:\n got: %s\nwant: %s", got, want)
	}
}
