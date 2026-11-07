//go:build windows

package desktop

// repair.go 向维修侧注册桌面操纵智能体（模型侧）的只读诊断入口。
//
// 桌面键鼠任务的中断由 chain/desktop 负责；这里只导出各阶段最近执行结果。

import (
	"LunarSubsystem/LunarHarness/protocol/agentrepair"
)

func init() {
	agentrepair.RegisterReadonly("desktop_agent", DisplayName,
		DisplayName+"各阶段最近一次执行结果（只读）",
		func(int) (any, error) { return exportContext(false) })
}
