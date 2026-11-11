package agentbase

import (
	"strings"
	"testing"
)

// 三态与回调契约：Applied / NotApplied / Unknown 原样透传；
// 未知既不当成功也不当失败，非法值归一为 Unknown。
func TestReportToolResultStates(t *testing.T) {
	var got []string
	SetObserver(Observer{ToolResult: func(role, name string, state ToolResultState, summary string) {
		got = append(got, role+"|"+name+"|"+string(state)+"|"+state.Label()+"|"+summary)
	}})
	defer SetObserver(Observer{})

	ReportToolResult("桌面执行", "type_text", ToolApplied, "前后画面指纹有变化，动作已生效")
	ReportToolResult("桌面执行", "click", ToolNotApplied, "工具明确失败：目标不存在")
	ReportToolResult("桌面执行", "click", ToolUnknown, "执行超时，状态未确认")
	ReportToolResult("桌面执行", "click", "bogus", "非法状态归一")

	want := []string{
		"桌面执行|type_text|applied|已生效|前后画面指纹有变化，动作已生效",
		"桌面执行|click|not_applied|未生效|工具明确失败：目标不存在",
		"桌面执行|click|unknown|⚠ 状态未确认|执行超时，状态未确认",
		"桌面执行|click|unknown|⚠ 状态未确认|非法状态归一",
	}
	if len(got) != len(want) {
		t.Fatalf("回调次数 = %d，期望 %d：%v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("第 %d 条 = %q，期望 %q", i+1, got[i], want[i])
		}
	}
	if label := ToolUnknown.Label(); !strings.Contains(label, "未确认") {
		t.Fatalf("Unknown 标签应明确标注未确认，实际 %q", label)
	}
}

// 未注入回调时行为不变：不 panic、不产生副作用。
func TestReportToolResultWithoutObserver(t *testing.T) {
	SetObserver(Observer{})
	ReportToolResult("桌面执行", "click", ToolUnknown, "无观察者")
}
