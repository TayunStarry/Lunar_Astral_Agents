//go:build windows

package execute

// type.go 「桌面执行」成员的类型定义。
// 只放本成员专属类型；族间共享的交接与决策结构在 agents/desktop。

import (
	"LunarSubsystem/LunarHarness/agents/desktop"
	"LunarSubsystem/LunarHarness/protocol/agentbase"
)

// Intent 一次执行请求的意图，决定本成员向模型暴露哪一组工具。
// 集中在一处是为了避免把 15 个工具一次性丢给模型裸选。
type Intent string

const (
	// IntentLaunch 启动/激活/关闭窗口，或打开文件夹。
	IntentLaunch Intent = "launch"
	// IntentKeyboard 键入文本、发送、按键。
	IntentKeyboard Intent = "keyboard"
	// IntentMouse 点击、拖拽、滚动、移动光标。
	IntentMouse Intent = "mouse"
	// IntentUIA 对 UIA 元素执行点击或写入。
	IntentUIA Intent = "uia"
)

// diffCallOutcome 一次带差异检测的桌面操作结果。
type diffCallOutcome struct {
	res     desktop.ToolResult        // 工具返回
	effect  bool                      // 是否产生可见变化（供编排层判定停滞）
	state   agentbase.ToolResultState // 实况三态：已生效 / 未生效 / 状态未确认
	summary string                    // 一句人话的结果说明
}
