package viewer

// submit_test.go 钉住「必填工具提交」的唯一入口：正文不采纳，异常一律报错。

import (
	"strings"
	"testing"

	"LunarSubsystem/LunarHarness/protocol/agentbase"
)

func toolCall(name, arguments string) agentbase.ToolCall {
	return agentbase.ToolCall{Type: "function", Function: agentbase.ToolFunction{Name: name, Arguments: arguments}}
}

// TestPickSubmitCallNormal 正常工具调用原样返回参数。
func TestPickSubmitCallNormal(t *testing.T) {
	call, err := PickSubmitCall("观感", agentbase.Reply{
		Text:      "随便说一句",
		ToolCalls: []agentbase.ToolCall{toolCall("submit_remark", `{"comments":[]}`)},
	}, "submit_remark")
	if err != nil {
		t.Fatalf("正常提交不应报错: %v", err)
	}
	if call.Function.Name != "submit_remark" || call.Function.Arguments != `{"comments":[]}` {
		t.Fatalf("提交调用不符: %+v", call)
	}
}

// TestPickSubmitCallIgnoresTextJSON 正文里有 JSON 但没调工具时不采纳。
func TestPickSubmitCallIgnoresTextJSON(t *testing.T) {
	_, err := PickSubmitCall("观感", agentbase.Reply{
		Text: `{"comments":[{"text":"正文里的 JSON","mention":0}]}`,
	}, "submit_remark")
	if err == nil {
		t.Fatal("正文 JSON 不应被采纳")
	}
	if !strings.Contains(err.Error(), "不予采纳") {
		t.Fatalf("错误应说明正文不采纳: %v", err)
	}
}

// TestPickSubmitCallMissing 没有工具调用且正文为空时也要报错。
func TestPickSubmitCallMissing(t *testing.T) {
	if _, err := PickSubmitCall("审查", agentbase.Reply{}, "submit_review"); err == nil {
		t.Fatal("没有工具调用应返回错误")
	}
}

// TestPickSubmitCallMultiple 同一回复提交多次视为非法。
func TestPickSubmitCallMultiple(t *testing.T) {
	_, err := PickSubmitCall("阅片", agentbase.Reply{
		ToolCalls: []agentbase.ToolCall{
			toolCall("submit_impression", `{"summary":"a"}`),
			toolCall("submit_impression", `{"summary":"b"}`),
		},
	}, "submit_impression")
	if err == nil || !strings.Contains(err.Error(), "多次") {
		t.Fatalf("多次提交应报错: %v", err)
	}
}

// TestPickSubmitCallWrongTool 调用了未提供的工具时不采纳。
func TestPickSubmitCallWrongTool(t *testing.T) {
	_, err := PickSubmitCall("审查", agentbase.Reply{
		ToolCalls: []agentbase.ToolCall{toolCall("review_video", `{"pass":true}`)},
	}, "submit_review")
	if err == nil {
		t.Fatal("非提交工具不应被采纳")
	}
}

// TestPickSubmitCallEmptyArguments 参数为空字符串视为非法。
func TestPickSubmitCallEmptyArguments(t *testing.T) {
	_, err := PickSubmitCall("审查", agentbase.Reply{
		ToolCalls: []agentbase.ToolCall{toolCall("submit_review", "   ")},
	}, "submit_review")
	if err == nil || !strings.Contains(err.Error(), "没有参数") {
		t.Fatalf("空参数应报错: %v", err)
	}
}
