package life

// event_creative_test.go 自由创作触发信息必须明确状态归属：心情/生活记录属于月华本人。

import (
	"context"
	"strings"
	"testing"
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
