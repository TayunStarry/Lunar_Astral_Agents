package creation

// repair.go 向维修侧注册自由创作的只读诊断入口。
//
// 自由创作保留跨轮记录（上一轮计划、备忘、参考资料、错误与收尾），
// 维修侧只能读取这些记录用于给出建议，不再具备清空轮次的能力。

import (
	"LunarSubsystem/LunarHarness/protocol/agentrepair"
)

func init() {
	agentrepair.RegisterReadonly("creative", "自由创作",
		"自由创作的跨轮记录（上一轮计划、备忘、参考资料、错误与收尾，只读）",
		func(int) (any, error) { return exportContext(false) })
}
