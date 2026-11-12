package StoryWriter

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"LunarSubsystem/LunarHarness/agents/file/FileSupport"
	"LunarSubsystem/LunarHarness/internal/testenv"
)

func TestListStoriesOnlyIncludesOutlineFolders(t *testing.T) {
	tmp := testenv.Isolate(t)

	root := filepath.Join(tmp, storyRoot)
	mustWrite(t, filepath.Join(root, "雨夜旧书店", outlineFile), "# 《雨夜旧书店》\n")
	mustWrite(t, filepath.Join(root, "雨夜旧书店", progressFile), "# 进度\n")
	mustWrite(t, filepath.Join(root, "雨夜旧书店", "001-开门.md"), "正文")
	mustWrite(t, filepath.Join(root, "普通文档", "笔记.md"), "不是故事")
	mustWrite(t, filepath.Join(root, "README.md"), "根目录文件")

	stories, err := ListStories()
	if err != nil {
		t.Fatalf("ListStories 失败：%v", err)
	}
	if len(stories) != 1 {
		t.Fatalf("应只识别 1 个故事，实际 %d：%+v", len(stories), stories)
	}
	if stories[0].Name != "雨夜旧书店" || stories[0].Chapters != 1 || stories[0].LastChapter != 1 {
		t.Fatalf("故事摘要不符合预期：%+v", stories[0])
	}
}

func TestToolHandlerCreatesAndAppendsStoryFiles(t *testing.T) {
	tmp := testenv.Isolate(t)
	defer func() { _ = FileSupport.Close() }()

	c := &collector{root: storyRoot}
	handler := toolHandler(c, Intent{})

	outcome, _, err := handler(toolCreate, map[string]any{
		"path":    "测试故事/大纲.md",
		"content": "# 《测试故事》\n\n## 章节纲要\n",
	})
	if err != nil || !outcome.Mutated {
		t.Fatalf("创建大纲失败：%v / %+v", err, outcome)
	}
	outcome, _, err = handler(toolAppend, map[string]any{
		"path":    "测试故事/大纲.md",
		"content": "- 001｜开端｜主角收到一封信｜信件出现｜信末没有署名\n",
	})
	if err != nil || !outcome.Mutated {
		t.Fatalf("追加章节纲要失败：%v / %+v", err, outcome)
	}

	data, readErr := os.ReadFile(filepath.Join(tmp, storyRoot, "测试故事", "大纲.md"))
	if readErr != nil {
		t.Fatalf("读取大纲失败：%v", readErr)
	}
	if !strings.Contains(string(data), "001｜开端") {
		t.Fatalf("追加内容未落盘：%s", string(data))
	}
	if got := storyFolderFrom(c.changed()); got != "documents/测试故事" {
		t.Fatalf("故事目录推断错误：%s", got)
	}
}

func TestCreateStoryAndWriteChapter(t *testing.T) {
	tmp := testenv.Isolate(t)
	defer func() { _ = FileSupport.Close() }()

	c := &collector{root: storyRoot}
	handler := toolHandler(c, Intent{Mode: ModeNew, Story: "雾港灯塔"})

	// 标题由系统判定时，模型传入的标题必须被覆盖。
	outcome, _, err := handler(toolNew, map[string]any{
		"title":   "别的名字",
		"outline": "## 背景设定\n- 时代与地点：雾港\n\n## 人物信息\n- 林默｜调查员\n\n## 章节纲要\n- 001｜无人小船｜……\n",
	})
	if err != nil || !outcome.Mutated {
		t.Fatalf("创建故事失败：%v / %+v", err, outcome)
	}
	if _, statErr := os.Stat(filepath.Join(tmp, storyRoot, "雾港灯塔", outlineFile)); statErr != nil {
		t.Fatalf("大纲未按系统标题落盘：%v", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(tmp, storyRoot, "别的名字")); statErr == nil {
		t.Fatalf("不应使用模型自拟标题建目录")
	}
	if _, statErr := os.Stat(filepath.Join(tmp, storyRoot, "雾港灯塔", progressFile)); statErr != nil {
		t.Fatalf("进度文件未自动创建：%v", statErr)
	}

	// 章节路径与编号由 Go 侧生成，模型不需要拼路径。
	outcome, _, err = handler(toolChapter, map[string]any{
		"title":   "无人小船",
		"content": strings.Repeat("雾", 400),
		"summary": "守塔人发现一艘无人小船漂进港湾",
		"changes": "无",
	})
	if err != nil || !outcome.Mutated {
		t.Fatalf("写入章节失败：%v / %+v", err, outcome)
	}
	chapterPath := filepath.Join(tmp, storyRoot, "雾港灯塔", "001-无人小船.md")
	if data, readErr := os.ReadFile(chapterPath); readErr != nil || !strings.Contains(string(data), "雾") {
		t.Fatalf("章节正文未落盘：%v / %s", readErr, string(data))
	}
	progress, readErr := os.ReadFile(filepath.Join(tmp, storyRoot, "雾港灯塔", progressFile))
	if readErr != nil || !strings.Contains(string(progress), "## 001 实际发生") {
		t.Fatalf("进度未自动追加：%v / %s", readErr, string(progress))
	}

	// 不传编号时自动取下一号。
	outcome, _, err = handler(toolChapter, map[string]any{
		"title":   "灯塔守望",
		"content": strings.Repeat("灯", 400),
		"summary": "继续调查",
	})
	if err != nil || !outcome.Mutated {
		t.Fatalf("写入第二章失败：%v / %+v", err, outcome)
	}
	if _, statErr := os.Stat(filepath.Join(tmp, storyRoot, "雾港灯塔", "002-灯塔守望.md")); statErr != nil {
		t.Fatalf("第二章编号未自动递增：%v", statErr)
	}

	// 章节与进度有专用工具，普通文件工具要拒绝，避免模型再次写错路径或只写进度。
	if outcome, _, _ = handler(toolCreate, map[string]any{"path": "雾港灯塔/003-越界.md", "content": "正文"}); outcome.Mutated {
		t.Fatalf("file_create 不应允许写章节")
	}
	if outcome, _, _ = handler(toolAppend, map[string]any{"path": "雾港灯塔/进度.md", "content": "手动进度"}); outcome.Mutated {
		t.Fatalf("file_append 不应允许手写进度")
	}

	if got := storyFolderFrom(c.changed()); got != "documents/雾港灯塔" {
		t.Fatalf("故事目录推断错误：%s", got)
	}
}

func TestStoryFolderFromChangedPaths(t *testing.T) {
	got := storyFolderFrom([]string{"documents/雨夜旧书店/001-开门.md", "documents/雨夜旧书店/进度.md"})
	if got != "documents/雨夜旧书店" {
		t.Fatalf("故事目录推断错误：%s", got)
	}
	if got := storyFolderFrom([]string{"documents/根文件.md"}); got != "" {
		t.Fatalf("根目录文件不应推断出故事目录：%s", got)
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("创建目录失败：%v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("写入文件失败：%v", err)
	}
}
