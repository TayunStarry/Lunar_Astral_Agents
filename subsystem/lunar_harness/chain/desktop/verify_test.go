//go:build windows

package desktopchain

import (
	"context"
	"strings"
	"testing"

	"LunarSubsystem/LunarHarness/agents/desktop"
	"LunarSubsystem/LunarHarness/protocol/agentbase"
)

// TestJudgeVerify 钉住三态判定：只有真实证据才给 Applied / NotApplied，
// 取消、读取失败、找不到文本、窗口不在前台一律 Unknown。
func TestJudgeVerify(t *testing.T) {
	cases := []struct {
		name string
		f    verifyFacts
		want agentbase.ToolResultState
	}{
		{
			name: "取消只能是未知",
			f:    verifyFacts{canceled: true},
			want: agentbase.ToolUnknown,
		},
		{
			name: "产出路径已存在",
			f: verifyFacts{
				produceIntent: true, producePath: `D:\out\a.txt`, pathExists: true,
			},
			want: agentbase.ToolApplied,
		},
		{
			name: "产出路径不存在",
			f: verifyFacts{
				produceIntent: true, producePath: `D:\out\a.txt`,
			},
			want: agentbase.ToolNotApplied,
		},
		{
			name: "界面已包含最后输入",
			f: verifyFacts{
				lastInput: "你好世界", uiaOK: true, uiaTree: "编辑 你好世界",
			},
			want: agentbase.ToolApplied,
		},
		{
			name: "界面没有最后输入",
			f: verifyFacts{
				lastInput: "你好世界", uiaOK: true, uiaTree: "编辑 ",
			},
			want: agentbase.ToolUnknown,
		},
		{
			name: "界面读取失败",
			f: verifyFacts{
				lastInput: "你好世界", uiaErr: "读取 UIA 树超时",
			},
			want: agentbase.ToolUnknown,
		},
		{
			name: "打开类任务且目标在前台",
			f: verifyFacts{
				openIntent: true, targetApp: "notepad", targetForeground: true,
				targetWindow: true, foreground: "无标题 - 记事本",
			},
			want: agentbase.ToolApplied,
		},
		{
			name: "目标窗口存在但不在前台",
			f: verifyFacts{
				openIntent: true, targetApp: "notepad", targetWindow: true,
				foreground: "桌面操纵控制台",
			},
			want: agentbase.ToolUnknown,
		},
		{
			name: "找不到目标窗口",
			f: verifyFacts{
				openIntent: true, targetApp: "notepad",
			},
			want: agentbase.ToolUnknown,
		},
		{
			name: "没有任何可核对证据",
			f:    verifyFacts{foreground: "桌面"},
			want: agentbase.ToolUnknown,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, summary := judgeVerify(tc.f)
			if got != tc.want {
				t.Fatalf("状态 = %s，期望 %s（%s）", got, tc.want, summary)
			}
			if strings.TrimSpace(summary) == "" {
				t.Fatal("结果摘要不能为空")
			}
		})
	}
}

// TestDeclaredProducePath 只有「创建/保存 … <路径>」才算产出意图，普通引用路径不算。
func TestDeclaredProducePath(t *testing.T) {
	if got := declaredProducePath("把内容保存到 D:\\out\\a.txt"); got != `D:\out\a.txt` {
		t.Fatalf("产出路径 = %q", got)
	}
	if got := declaredProducePath("打开 C:\\in\\a.docx 看看"); got != "" {
		t.Fatalf("普通引用不应算产出路径，实际 %q", got)
	}
}

// TestVerifyLivePair 一次核对只产生一对「调用 + 结果」，且三态文案来自基座。
func TestVerifyLivePair(t *testing.T) {
	var lines []string
	agentbase.SetObserver(agentbase.Observer{
		Tool: func(role, name, args string) {
			lines = append(lines, "["+role+"] 调用 "+name+" "+args)
		},
		ToolResult: func(role, name string, state agentbase.ToolResultState, summary string) {
			lines = append(lines, "["+role+"] "+state.Label()+" "+name+"："+summary)
		},
	})
	t.Cleanup(func() { agentbase.SetObserver(agentbase.Observer{}) })

	reportVerify(`{"target_app":"notepad"}`, agentbase.ToolUnknown, "界面元素读取失败，输入结果未确认")

	if len(lines) != 2 {
		t.Fatalf("应为一对调用/结果，实际 %d 行：%v", len(lines), lines)
	}
	if !strings.Contains(lines[0], "调用 "+verifyToolName) {
		t.Fatalf("第一行应为调用行：%s", lines[0])
	}
	if !strings.Contains(lines[1], "⚠ 状态未确认 "+verifyToolName) {
		t.Fatalf("第二行应为未知态结果行：%s", lines[1])
	}
	t.Log(strings.Join(lines, "\n"))
}

// TestVerifyOutcomeWithoutEvidence 真实执行一次核对：没有任务与动作记录时只能是未知。
func TestVerifyOutcomeWithoutEvidence(t *testing.T) {
	var calls, results int
	agentbase.SetObserver(agentbase.Observer{
		Tool: func(role, name, args string) { calls++ },
		ToolResult: func(role, name string, state agentbase.ToolResultState, summary string) {
			results++
			if state != agentbase.ToolUnknown {
				t.Fatalf("无证据却判成 %s：%s", state, summary)
			}
		},
	})
	t.Cleanup(func() { agentbase.SetObserver(agentbase.Observer{}) })

	state, summary := verifyOutcome(context.Background(), desktop.EditorTask{}, desktop.HandoffRecord{})
	if state != agentbase.ToolUnknown {
		t.Fatalf("状态 = %s，期望 unknown", state)
	}
	if calls != 1 || results != 1 {
		t.Fatalf("调用/结果应各 1 行，实际 %d/%d", calls, results)
	}
	role := chainScopeLabel
	if role == "" {
		t.Fatal("族名不能为空")
	}
	if !strings.Contains(summary, "状态未确认") && !strings.Contains(summary, "无法确认") && !strings.Contains(summary, "没有可核对") {
		t.Fatalf("摘要应说明证据不足：%s", summary)
	}
}
