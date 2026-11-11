//go:build windows

package execute

// toolresult_test.go 钉住工具结果三态的判定依据，以及实况中「调用 + 结果」的成对形态。

import (
	"strings"
	"testing"

	"LunarSubsystem/LunarHarness/agents/desktop"
	"LunarSubsystem/LunarHarness/protocol/agentbase"
)

func TestJudgeToolOutcome(t *testing.T) {
	cases := []struct {
		name     string
		res      desktop.ToolResult
		captured bool
		changed  bool
		want     agentbase.ToolResultState
	}{
		{"type_text", fail("UIA 写入失败"), false, false, agentbase.ToolNotApplied},
		{"type_text", ok("已键入文本"), true, true, agentbase.ToolApplied},
		{"type_text", ok("已键入文本"), true, false, agentbase.ToolUnknown}, // 画面无变化且无查询核对，不能猜成功
		{"click", ok("已点击"), true, true, agentbase.ToolApplied},
		{"click", ok("已点击"), true, false, agentbase.ToolUnknown},              // 证据不足，不并入失败
		{"click", ok("已点击"), false, false, agentbase.ToolUnknown},             // 没有前后指纹
		{"activate_window", ok("已激活窗口"), false, false, agentbase.ToolApplied}, // 窗口查询确认
		{"uia_click", ok("已点击元素"), false, false, agentbase.ToolApplied},
		{"uia_input", ok("已写入元素"), false, false, agentbase.ToolApplied},
		{"list_windows", ok("窗口列表"), false, false, agentbase.ToolApplied},
		{"launch_program", ok("已启动程序"), false, false, agentbase.ToolUnknown}, // 未查询到窗口前不猜成功
		{"close_window", ok("已关闭窗口"), false, false, agentbase.ToolUnknown},   // 未确认真的关闭
	}
	for _, c := range cases {
		state, _ := judgeToolOutcome(c.name, c.res, c.captured, c.changed)
		if state != c.want {
			t.Errorf("%s(success=%v,captured=%v,changed=%v) = %s，期望 %s", c.name, c.res.Success, c.captured, c.changed, state, c.want)
		}
	}
}

// 一轮多动作的实况形态：每条调用行后面都有一条结果行（含三态与一句人话），
// 未派发的动作也补 Unknown，不出现「上报了却没执行」的悬空行。
func TestToolResultLiveShape(t *testing.T) {
	var lines []string
	agentbase.SetObserver(agentbase.Observer{
		Tool: func(role, name, args string) {
			line := "[" + role + "] 调用 " + name + " " + args
			lines = append(lines, line)
			t.Log(line)
		},
		ToolResult: func(role, name string, state agentbase.ToolResultState, summary string) {
			line := "[" + role + "] " + state.Label() + " " + name + "：" + summary
			lines = append(lines, line)
			t.Log(line)
		},
	})
	defer agentbase.SetObserver(agentbase.Observer{})

	round := []struct {
		args     string
		name     string
		res      desktop.ToolResult
		captured bool
		changed  bool
	}{
		{`{"text":"你好世界"}`, "type_text", ok("已键入文本「你好世界」"), true, true},
		{`{"x":400,"y":300}`, "click", ok("已点击 (400,300)"), true, false},
		{`{"path":"C:\\不存在"}`, "open_folder", fail("路径不存在"), false, false},
	}
	for _, item := range round {
		// 调用行由基座 ReportTools 上报；这里按同样的格式复现，便于核对成对形态。
		lines = append(lines, "["+modelRole+"] 调用 "+item.name+" "+item.args)
		t.Log(lines[len(lines)-1])
		state, summary := judgeToolOutcome(item.name, item.res, item.captured, item.changed)
		agentbase.ReportToolResult(modelRole, item.name, state, summary)
	}
	// 第四个动作因前序失败未派发：基座已上报调用行，结果行必须显式补齐。
	lines = append(lines, "["+modelRole+"] 调用 press_key {\"key\":\"enter\"}")
	t.Log(lines[len(lines)-1])
	reportUnreported([]desktop.ToolCall{{Function: desktop.ToolFunc{Name: "press_key"}}}, "同一回复中的前一个动作已失败")

	if len(lines) != 8 {
		t.Fatalf("实况行数 = %d，期望 8（4 条调用 + 4 条结果）：\n%s", len(lines), strings.Join(lines, "\n"))
	}
	for i := 0; i < len(lines); i += 2 {
		if !strings.Contains(lines[i], "调用") || !strings.Contains(lines[i+1], "：") {
			t.Fatalf("第 %d/%d 行没有成对出现：\n%s\n%s", i+1, i+2, lines[i], lines[i+1])
		}
	}
}
