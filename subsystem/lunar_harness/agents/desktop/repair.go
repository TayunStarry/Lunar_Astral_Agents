//go:build windows

package desktop

// repair.go 向拯救者注册桌面操纵智能体（模型侧）的修复能力。
//
// 桌面键鼠任务的中断由 chain/desktop 负责；这里只清空各阶段最近执行结果。

import (
	"LunarSubsystem/LunarHarness/protocol/agentrepair"
)

func init() {
	agentrepair.RegisterResettable("desktop_agent", "桌面操作-模型",
		"桌面操纵智能体各阶段最近一次执行结果；reset 清除该记录",
		resetDesktopPhases)
}

// resetDesktopPhases 清空各阶段最近执行结果。
func resetDesktopPhases() string {
	resetPhases()
	return "已清除桌面操纵智能体的阶段执行记录"
}
