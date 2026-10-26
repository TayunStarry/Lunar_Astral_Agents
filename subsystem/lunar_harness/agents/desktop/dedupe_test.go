//go:build windows

package desktop

// dedupe_test.go 钉住「相同文本已输入且窗口标题已包含时不再重复输入」的判定。

import "testing"

func TestRepeatedInputGuard(t *testing.T) {
	handoff := HandoffRecord{
		Foreground: "12345 - Notepad",
		Attempts: []ActionAttempt{
			{Success: true, InputText: "12345"},
		},
	}
	if !repeatedInput(handoff, "12345") {
		t.Fatal("窗口标题已包含且历史成功输入相同文本时应拦截重复输入")
	}
	if repeatedInput(handoff, "67890") {
		t.Fatal("不同文本不应被拦截")
	}

	notInTitle := HandoffRecord{
		Foreground: "新建文本文档 - Notepad",
		Attempts:   []ActionAttempt{{Success: true, InputText: "12345"}},
	}
	if repeatedInput(notInTitle, "12345") {
		t.Fatal("窗口标题尚未证实包含该文本时不应拦截（可能确实需要重试）")
	}

	failedOnly := HandoffRecord{
		Foreground: "12345 - Notepad",
		Attempts:   []ActionAttempt{{Success: false, InputText: "12345"}},
	}
	if repeatedInput(failedOnly, "12345") {
		t.Fatal("上一次输入失败时不应按重复输入拦截")
	}
}
