//go:build windows

package desktopchain

// repair.go 向维修侧注册「桌面操作」族的只读诊断入口。
//
// 与 dump.go 共用同一个族级 ID，保证「能导出就能诊断」；
// 桌面键鼠任务的中断由本链负责，这里只导出各成员最近执行结果。

import (
	"LunarSubsystem/LunarHarness/protocol/agentrepair"
)

func init() {
	agentrepair.RegisterReadonly(chainScopeID, chainScopeLabel,
		chainScopeLabel+"各阶段最近一次执行结果（只读）",
		func(int) (any, error) { return familyExportContext(false) })
}
