package life

// event_creative_test.go 自由创作触发信息必须明确状态归属，并携带当前愿望清单。

import (
	"context"
	"strings"
	"testing"

	"LunarSubsystem/LunarHarness/chain/wishchain"
	"LunarSubsystem/LunarHarness/internal/testenv"
)

func TestCreativeInstructionMarksOwnState(t *testing.T) {
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

// TestCreativeInstructionCarriesWishesAndMoodBranch
// 创作参考不再来自独立的「创作意图」文件，而是直接读取愿望清单；
// 心情正负仍决定延伸愿望还是自行规划。
func TestCreativeInstructionCarriesWishesAndMoodBranch(t *testing.T) {
	testenv.Isolate(t)
	wishchain.Reset()
	t.Cleanup(wishchain.Reset)
	if _, err := wishchain.Add("写一篇关于月亮的散文", "", wishchain.AutoPriority); err != nil {
		t.Fatalf("写入愿望失败: %v", err)
	}

	positive := &creativeRun{}
	if err := positive.Start(context.Background(), Env{MoodLabel: "开心"}); err != nil {
		t.Fatalf("准备正向创作失败: %v", err)
	}
	if !strings.Contains(positive.data.Instruction, "写一篇关于月亮的散文") {
		t.Fatalf("创作指令应包含当前愿望清单: %s", positive.data.Instruction)
	}
	if !strings.Contains(positive.data.Instruction, "沿着上一轮创作思路") {
		t.Fatalf("正向心情应允许延续上一轮思路: %s", positive.data.Instruction)
	}

	negative := &creativeRun{}
	if err := negative.Start(context.Background(), Env{MoodLabel: "伤心"}); err != nil {
		t.Fatalf("准备负向创作失败: %v", err)
	}
	if !strings.Contains(negative.data.Instruction, "自行规划创作方向") {
		t.Fatalf("负向心情应允许自行规划: %s", negative.data.Instruction)
	}
}
