package StoryWriter

import (
	"path/filepath"
	"strings"
	"testing"

	"LunarSubsystem/LunarHarness/engine/resource"
	"LunarSubsystem/LunarHarness/internal/testenv"
)

func TestResolveIntent(t *testing.T) {
	stories := []StoryInfo{
		{Name: "雨夜旧书店", Chapters: 3, LastChapter: 3, UpdatedUnix: 200},
		{Name: "星港信使", Chapters: 1, LastChapter: 1, UpdatedUnix: 100},
	}
	cases := []struct {
		name        string
		mode        string
		story       string
		instruction string
		wantMode    string
		wantStory   string
	}{
		{"明确新建", ModeAuto, "", "写个新故事，关于机器人", ModeNew, ""},
		{"换个故事按新建", ModeAuto, "", "换个故事吧，写点轻松的", ModeNew, ""},
		{"点名并续写", ModeAuto, "", "续写《星港信使》下一章", ModeContinue, "星港信使"},
		{"点名加一章", ModeAuto, "", "给《雨夜旧书店》加一章", ModeContinue, "雨夜旧书店"},
		{"续写不点名取最近", ModeAuto, "", "继续写下一章", ModeContinue, "雨夜旧书店"},
		{"再写一章", ModeAuto, "", "再写一章吧", ModeContinue, "雨夜旧书店"},
		{"普通写故事默认新建", ModeAuto, "", "写个关于机器人的故事", ModeNew, ""},
		{"否定续写后新建", ModeAuto, "", "别续写《星港信使》了，写个新故事", ModeNew, ""},
		{"否定另起后延续", ModeAuto, "", "不要另起炉灶，继续写《雨夜旧书店》", ModeContinue, "雨夜旧书店"},
		{"生活延续标记", ModeAuto, "", "【故事模式】延续\n这是延续上一次的创作", ModeContinue, "雨夜旧书店"},
		{"生活新建标记", ModeAuto, "", "【故事模式】新建\n这次开始一个新作品", ModeNew, ""},
		{"标记优先于续写词", ModeAuto, "", "【故事模式】新建\n继续写《雨夜旧书店》", ModeNew, ""},
		{"上级指定延续取最近", ModeContinue, "", "随便写点", ModeContinue, "雨夜旧书店"},
		{"上级指定延续并点名", ModeContinue, "星港信使", "继续", ModeContinue, "星港信使"},
		{"上级指定新建覆盖续写词", ModeNew, "", "继续写《雨夜旧书店》", ModeNew, ""},
		{"中文模式值", "延续", "", "随便", ModeContinue, "雨夜旧书店"},
		{"未知模式按自动", "whatever", "", "写个新故事", ModeNew, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ResolveIntent(tc.mode, tc.story, tc.instruction, nil, stories, 0)
			if got.Mode != tc.wantMode || got.Story != tc.wantStory {
				t.Fatalf("判定不符合预期：got=%+v want=(%s,%s)", got, tc.wantMode, tc.wantStory)
			}
		})
	}

	if got := ResolveIntent(ModeContinue, "", "续写下一章", nil, nil, 0); got.Mode != ModeNew {
		t.Fatalf("没有故事时应回落为新建：%+v", got)
	}
}

func TestResolveIntentUsesHandleStory(t *testing.T) {
	tmp := testenv.Isolate(t)
	mustWrite(t, filepath.Join(tmp, storyRoot, "星港信使", "001-启航.md"), "正文")
	_, handle, err := resource.RegisterPath(storyRoot + "/星港信使/001-启航.md")
	if err != nil || handle == "" {
		t.Fatalf("登记句柄失败：%v / %q", err, handle)
	}
	stories := []StoryInfo{
		{Name: "雨夜旧书店", Chapters: 3, LastChapter: 3, UpdatedUnix: 200},
		{Name: "星港信使", Chapters: 1, LastChapter: 1, UpdatedUnix: 100},
	}
	got := ResolveIntent(ModeAuto, "", "接着上次继续", []string{handle}, stories, 0)
	if got.Mode != ModeContinue || got.Story != "星港信使" {
		t.Fatalf("应按句柄延续《星港信使》：%+v", got)
	}
}

func TestResolveIntentKeepsExplicitTitle(t *testing.T) {
	stories := []StoryInfo{
		{Name: "雨夜旧书店", Chapters: 3, LastChapter: 3, UpdatedUnix: 200},
		{Name: "星港信使", Chapters: 1, LastChapter: 1, UpdatedUnix: 100},
	}
	got := ResolveIntent(ModeNew, "", "写一个新故事，标题叫《雾港灯塔》", nil, stories, 0)
	if got.Mode != ModeNew || got.Story != "雾港灯塔" {
		t.Fatalf("应强制使用指令标题：%+v", got)
	}
	got = ResolveIntent(ModeAuto, "", "写个新故事《雾港灯塔》", nil, stories, 0)
	if got.Mode != ModeNew || got.Story != "雾港灯塔" {
		t.Fatalf("应识别新建标题：%+v", got)
	}
	// 标题已存在时改为延续，避免重复建目录。
	got = ResolveIntent(ModeNew, "", "写个新故事《星港信使》", nil, stories, 0)
	if got.Mode != ModeContinue || got.Story != "星港信使" {
		t.Fatalf("同名故事应改为延续：%+v", got)
	}
	// 被否定 / 续写引出的旧标题不能当作新标题。
	got = ResolveIntent(ModeAuto, "", "别续写《星港信使》了，写个新故事", nil, stories, 0)
	if got.Mode != ModeNew || got.Story != "" {
		t.Fatalf("否定句里的旧标题不应复用：%+v", got)
	}
	// 章节名不是故事标题。
	got = ResolveIntent(ModeNew, "", "写个新故事，第一章叫《无人小船》", nil, stories, 0)
	if got.Mode != ModeNew || got.Story != "" {
		t.Fatalf("章节名不应被当成故事标题：%+v", got)
	}
}

func TestResolveIntentReusesStoryCreatedThisTask(t *testing.T) {
	stories := []StoryInfo{
		{Name: "雨夜旧书店", Chapters: 3, LastChapter: 3, UpdatedUnix: 900},
		{Name: "刚建好的故事", Chapters: 0, LastChapter: 0, UpdatedUnix: 1100},
	}
	got := ResolveIntent(ModeNew, "", "写个关于灯塔的故事", nil, stories, 1000)
	if got.Mode != ModeContinue || got.Story != "刚建好的故事" {
		t.Fatalf("重试时应复用本次新建的故事：%+v", got)
	}
	got = ResolveIntent(ModeNew, "", "写个关于灯塔的故事", nil, stories, 0)
	if got.Mode != ModeNew {
		t.Fatalf("没有任务时间时不应自动延续：%+v", got)
	}
}

func TestTaskBlockState(t *testing.T) {
	stories := []StoryInfo{{Name: "雨夜旧书店", Chapters: 3, LastChapter: 3}}
	block := taskBlock(Intent{Mode: ModeContinue, Story: "雨夜旧书店", Reason: "测试"}, stories)
	if !strings.Contains(block, "【任务判定】延续故事《雨夜旧书店》") {
		t.Fatalf("延续判定块缺少故事名：%s", block)
	}
	if !strings.Contains(block, "至少 004") {
		t.Fatalf("延续判定块缺少下一章编号：%s", block)
	}
	newBlock := taskBlock(Intent{Mode: ModeNew, Reason: "测试"}, stories)
	if !strings.Contains(newBlock, "不得续写已有故事") {
		t.Fatalf("新建判定块缺少约束：%s", newBlock)
	}
	titleBlock := taskBlock(Intent{Mode: ModeNew, Story: "雾港灯塔", Reason: "测试"}, stories)
	if !strings.Contains(titleBlock, "《雾港灯塔》") || !strings.Contains(titleBlock, "不得改名") {
		t.Fatalf("新建判定块缺少强制标题：%s", titleBlock)
	}
}
