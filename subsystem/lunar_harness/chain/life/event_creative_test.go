package life

// event_creative_test.go 自由创作触发信息：新作品只下发主题，延续作品携带上次目录与句柄。

import (
	"context"
	"strings"
	"testing"

	"LunarSubsystem/LunarHarness/chain/wishchain"
	"LunarSubsystem/LunarHarness/internal/testenv"
)

func TestCreativeInstructionMarksOwnState(t *testing.T) {
	resetCreationState()
	run := &creativeRun{}
	env := Env{
		MoodLabel:    "开心",
		LastRecord:   "发呆中",
		LastUserText: "继续深化上一轮的创作",
	}
	if err := run.Start(context.Background(), env); err != nil {
		t.Fatalf("准备创作触发信息失败: %v", err)
	}
	instruction := run.data.Instruction
	for _, want := range []string{"月华本人", "不是用户的状态", "开心", "发呆中"} {
		if !strings.Contains(instruction, want) {
			t.Fatalf("创作触发信息缺少状态归属标记 %q: %s", want, instruction)
		}
	}
}

// TestCreativeInstructionKeepsWishText 愿望清单正文保留，只去掉多余的小标题。
func TestCreativeInstructionKeepsWishText(t *testing.T) {
	testenv.Isolate(t)
	resetCreationState()
	wishchain.Reset()
	t.Cleanup(wishchain.Reset)
	if _, err := wishchain.Add("写一篇关于月亮的散文", "", wishchain.AutoPriority); err != nil {
		t.Fatalf("写入愿望失败: %v", err)
	}

	run := &creativeRun{}
	if err := run.Start(context.Background(), Env{MoodLabel: "开心"}); err != nil {
		t.Fatalf("准备创作失败: %v", err)
	}
	instruction := run.data.Instruction
	if !strings.Contains(instruction, "写一篇关于月亮的散文") {
		t.Fatalf("创作指令应保留愿望正文: %s", instruction)
	}
	for _, unwanted := range []string{"当前愿望清单", "输出根目录", "-20"} {
		if strings.Contains(instruction, unwanted) {
			t.Fatalf("创作指令不应包含 %q: %s", unwanted, instruction)
		}
	}
	for _, want := range []string{"只做下面这一个主题", "documents/<标题>/", "这次开始一个新作品"} {
		if !strings.Contains(instruction, want) {
			t.Fatalf("创作指令缺少 %q: %s", want, instruction)
		}
	}
}

// TestCreativeInstructionContinuesPreviousProject 心情正向且有上次作品时，
// 必须延续上次的主题、项目目录与句柄，不能改发随机主题。
func TestCreativeInstructionContinuesPreviousProject(t *testing.T) {
	testenv.Isolate(t)
	resetCreationState()
	t.Cleanup(resetCreationState)
	saveCreationState("写个小说", "documents/雨夜旧书店", []string{"@D.001", "@I.002"}, 1)

	run := &creativeRun{}
	if err := run.Start(context.Background(), Env{MoodLabel: "开心", LastRecord: "发呆中"}); err != nil {
		t.Fatalf("准备创作失败: %v", err)
	}
	instruction := run.data.Instruction
	for _, want := range []string{"延续上一次的创作", "雨夜旧书店", "documents/雨夜旧书店", "@D.001", "@I.002"} {
		if !strings.Contains(instruction, want) {
			t.Fatalf("延续创作应包含 %q: %s", want, instruction)
		}
	}
	for _, unwanted := range []string{"这次开始一个新作品", "随机主题", "documents/<标题>/"} {
		if strings.Contains(instruction, unwanted) {
			t.Fatalf("延续创作不应包含 %q: %s", unwanted, instruction)
		}
	}
	if !run.data.Continuation || run.data.Rounds != 2 {
		t.Fatalf("延续状态不正确: continuation=%v rounds=%d", run.data.Continuation, run.data.Rounds)
	}
}

// TestCreativeInstructionPositiveMoodWithoutPrevious 没有上次作品时，
// 即使心情不错也不能声称延续。
func TestCreativeInstructionPositiveMoodWithoutPrevious(t *testing.T) {
	testenv.Isolate(t)
	resetCreationState()
	t.Cleanup(resetCreationState)

	run := &creativeRun{}
	if err := run.Start(context.Background(), Env{MoodLabel: "开心"}); err != nil {
		t.Fatalf("准备创作失败: %v", err)
	}
	instruction := run.data.Instruction
	if strings.Contains(instruction, "延续上一次") {
		t.Fatalf("没有上次作品时不应声称延续: %s", instruction)
	}
	if !strings.Contains(instruction, "这次开始一个新作品") {
		t.Fatalf("没有上次作品时应开始新作品: %s", instruction)
	}
}

func resetCreationState() {
	mu.Lock()
	defer mu.Unlock()
	state.CreationTopic = ""
	state.CreationFolder = ""
	state.CreationHandles = nil
	state.CreationRounds = 0
}
