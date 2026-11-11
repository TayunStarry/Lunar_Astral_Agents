package file

// loop_test.go 校验输出额度不足时的工具调用截断识别。

import (
	"testing"

	"LunarSubsystem/LunarHarness/protocol/agentbase"
)

func TestHasIncompleteToolCalls(t *testing.T) {
	cases := []struct {
		name  string
		calls []agentbase.ToolCall
		want  bool
	}{
		{
			name: "合法参数",
			calls: []agentbase.ToolCall{{
				Function: agentbase.ToolFunction{Name: "file_append", Arguments: `{"path":"a.md","content":"x"}`},
			}},
			want: false,
		},
		{
			name: "截断的参数",
			calls: []agentbase.ToolCall{{
				Function: agentbase.ToolFunction{Name: "file_append", Arguments: `{"path":"a.md","content":"`},
			}},
			want: true,
		},
		{
			name: "缺少工具名",
			calls: []agentbase.ToolCall{{
				Function: agentbase.ToolFunction{Arguments: `{}`},
			}},
			want: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := hasIncompleteToolCalls(tc.calls); got != tc.want {
				t.Fatalf("hasIncompleteToolCalls() = %v，期望 %v", got, tc.want)
			}
		})
	}
}
