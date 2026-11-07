package FileEditor

// repair.go 向维修侧注册文件编辑的只读诊断入口。
//
// 文件编辑按次执行（每次从系统提示词 + 本次指令重新开始），不保留跨轮上下文。

import (
	"LunarSubsystem/LunarHarness/protocol/agentrepair"
)

func init() {
	agentrepair.RegisterReadonly("file_editor", Module,
		"文件编辑的最近一次执行记录（按次执行、不保留对话上下文，只读）",
		func(int) (any, error) { return exportContext(false) })
}
