package DocumentReader

// repair.go 向维修侧注册文献解析的只读诊断入口。
//
// 文献解析按次执行（每次独立读文件、按需分批），不保留跨轮上下文；
// 失败通常来自文件本身，维修侧只读取最近一次解析记录。

import (
	"LunarSubsystem/LunarHarness/protocol/agentrepair"
)

func init() {
	agentrepair.RegisterReadonly("document_reader", roleLabel,
		"文献解析的最近一次解析记录（按次执行、不保留对话上下文，只读）",
		func(int) (any, error) { return exportContext(false) })
}
