//go:build windows

package desktop

// 编排层：跨轮尝试记录与停滞判定依赖的数据结构。

import (
	"strings"
	"testing"
)

func TestAppendAttemptKeepsRecent(t *testing.T) {
	var list []ActionAttempt
	for i := 1; i <= autoMaxAttempts+3; i++ {
		list = appendAttempt(list, ActionAttempt{Round: i, Decision: "uia"})
	}
	if len(list) != autoMaxAttempts {
		t.Fatalf("应只保留最近 %d 条，实得 %d", autoMaxAttempts, len(list))
	}
	if list[0].Round != 4 {
		t.Fatalf("保留窗口起始轮次错误: %d", list[0].Round)
	}
}

func TestStringifyHandoffShowsAttempts(t *testing.T) {
	h := HandoffRecord{
		Foreground:        "无标题 - Notepad",
		ForegroundProcess: "notepad.exe",
		TargetApp:         "记事本",
		Progress:          "等待输入",
		Attempts: []ActionAttempt{
			{Round: 1, Decision: "uia", Tool: "uia_click", Result: "已点击文本编辑器", Success: true, Changed: false},
			{Round: 2, Decision: "keyboard", Tool: "type_text", Result: "已键入文本", Success: true, Changed: true},
		},
	}
	text := stringifyHandoff(h)
	for _, want := range []string{"最近尝试", "第1轮 uia/uia_click", "无变化", "第2轮 keyboard/type_text", "有变化", "notepad.exe", "记事本"} {
		if !strings.Contains(text, want) {
			t.Fatalf("交接摘要缺少 %q:\n%s", want, text)
		}
	}
}

func TestStringifyProgressOmitsAttempts(t *testing.T) {
	h := HandoffRecord{
		Foreground: "Notepad",
		Attempts:   []ActionAttempt{{Round: 1, Decision: "uia"}},
	}
	if strings.Contains(stringifyProgress(h), "最近尝试") {
		t.Fatal("视觉/书记用的进度摘要不应包含历史尝试")
	}
}
