package life

// event_creative_test.go 自由创作触发信息必须明确状态归属：心情/生活记录属于月华本人。

import (
	"context"
	"strings"
	"testing"

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

// TestCreativeReferenceAndMoodBranch 创作参考来自字符串变量，心情正负决定延伸或自行规划。
func TestCreativeReferenceAndMoodBranch(t *testing.T) {
	testenv.Isolate(t)
	if err := SetCreativeIntent("写一篇关于月亮的散文"); err != nil {
		t.Fatalf("写入创作参考失败: %v", err)
	}
	positive := &creativeRun{}
	if err := positive.Start(context.Background(), Env{MoodLabel: "开心"}); err != nil {
		t.Fatalf("准备正向创作失败: %v", err)
	}
	if !strings.Contains(positive.data.Instruction, "写一篇关于月亮的散文") {
		t.Fatalf("创作指令应包含参考字符串: %s", positive.data.Instruction)
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
	if positive.data.Reference != "写一篇关于月亮的散文" {
		t.Fatalf("参考字符串未落入运行数据: %+v", positive.data)
	}
}

// TestCreativeIntentConditionalReset 创作参考读后按值重置，不覆盖运行期间新写入的意图。
func TestCreativeIntentConditionalReset(t *testing.T) {
	testenv.Isolate(t)
	if err := SetCreativeIntent("第一轮创作"); err != nil {
		t.Fatal(err)
	}
	if got := CreativeIntent(); got != "第一轮创作" {
		t.Fatalf("读取创作参考失败: %q", got)
	}
	ResetCreativeIntentIf("第一轮创作")
	if got := CreativeIntent(); got != "" {
		t.Fatalf("一轮结束后应重置为空: %q", got)
	}
	if err := SetCreativeIntent("新写入的意图"); err != nil {
		t.Fatal(err)
	}
	ResetCreativeIntentIf("第一轮创作")
	if got := CreativeIntent(); got != "新写入的意图" {
		t.Fatalf("旧轮次的重置不应覆盖新意图: %q", got)
	}
}
