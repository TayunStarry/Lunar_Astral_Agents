package Musician

// repair.go 向拯救者注册演奏者的修复能力（清空最近一次演奏记录）。

import (
	"LunarSubsystem/LunarHarness/protocol/agentrepair"
)

func init() {
	agentrepair.RegisterResettable(agentID, "演奏者",
		"演奏者按次执行、不保留跨轮对话；reset 清空最近一次演奏记录",
		resetRun,
		func(int) (any, error) { return exportContext(false) })
}
