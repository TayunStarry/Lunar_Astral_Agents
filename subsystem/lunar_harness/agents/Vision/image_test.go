package Vision

// image_test.go 图片描述提示词：与影像观看共用同一份判定标准，只保留详略差别。

import (
	"strings"
	"testing"

	"LunarSubsystem/LunarHarness/engine/multimodal"
	"LunarSubsystem/LunarHarness/protocol/agentbase"
)

// TestImageTaskDefaultIsConcrete 钉住默认档：必须回答「是什么 / 主体 / 关键细节 / 文字」，
// 篇幅短于影像切片，且不再沿用旧的 40 字一句式。
func TestImageTaskDefaultIsConcrete(t *testing.T) {
	prompt := imageTask("")
	for _, want := range []string{"禁止空话套话", "这是什么", "主体", "关键细节", "能看清的文字", "不超过 100 字"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("图片默认描述应要求 %q：%s", want, prompt)
		}
	}
	if strings.Contains(prompt, "不超过 40 字") {
		t.Fatalf("旧的 40 字标准应已移除：%s", prompt)
	}
}

// TestImageTaskQuestionKeepsQuestion 钉住定向档：保留问题、沿用同一标准、篇幅放宽到 150 字。
func TestImageTaskQuestionKeepsQuestion(t *testing.T) {
	prompt := imageTask("画面里出现了哪些文字？")
	for _, want := range []string{"禁止空话套话", "画面里出现了哪些文字？", "不超过 150 字", "图片里看不到"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("图片定向提问应包含 %q：%s", want, prompt)
		}
	}
}

// TestImageAndVideoShareVisualRules 钉住单一标准来源：
// 图片任务与影像任务必须以完全相同的判定标准开头，差异只在各自的字段与字数。
func TestImageAndVideoShareVisualRules(t *testing.T) {
	rules := strings.TrimSpace(agentbase.PromptText("visualFactRules.md"))
	if rules == "" {
		t.Fatal("共用判定标准不应为空")
	}
	image := imageTask("")
	video := WatchTask(1, 1, multimodal.WatchSlice{Start: 0, End: 60, Images: []string{"a"}}, "")
	if !strings.HasPrefix(image, rules) {
		t.Fatalf("图片任务应以共用判定标准开头：%s", image)
	}
	if !strings.HasPrefix(video, rules) {
		t.Fatalf("影像任务应以共用判定标准开头：%s", video)
	}
}
