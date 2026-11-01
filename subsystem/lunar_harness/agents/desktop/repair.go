//go:build windows

package desktop

// repair.go 向拯救者注册桌面操纵的修复能力。
//
// 桌面操纵会占用真实键鼠：任务卡死时 reset 等价于中断，
// 没有运行中的任务时是无副作用的空操作。

import (
	"LunarSubsystem/LunarHarness/engine/agentrepair"
)

func init() {
	agentrepair.RegisterResettable("desktop", "桌面操作",
		"桌面操纵的运行状态（要求、起止时间、结果）；reset 中断正在运行的任务",
		resetDesktop)
}

// resetDesktop 中断当前任务；未运行时直接返回。
func resetDesktop() string {
	if Stop() {
		return "已中断桌面操纵任务"
	}
	return "桌面操纵当前没有运行中的任务"
}
