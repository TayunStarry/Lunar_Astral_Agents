package agentdispatch

// instruction.go 指令与句柄的无损拼接。
//
// 统一原语把目标句柄单独放在 handle 字段，但部分子智能体只能从 instruction
// 中解析附件与目标；这里把尚未出现在指令里的句柄补到句尾，避免重复。

import "strings"

// WithHandles 把目标句柄补进自然语言指令。
func WithHandles(instruction string, handles []string) string {
	missing := make([]string, 0, len(handles))
	for _, handle := range handles {
		if text := strings.TrimSpace(handle); text != "" && !strings.Contains(instruction, text) {
			missing = append(missing, text)
		}
	}
	if len(missing) == 0 {
		return instruction
	}
	return instruction + "\n（涉及句柄：" + strings.Join(missing, "、") + "）"
}
