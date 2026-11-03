package Creator

// repair.go 向拯救者注册自由创作的上下文修复能力。

import (
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/fileflow"
	"LunarSubsystem/LunarHarness/protocol/agentrepair"
)

func init() {
	agentrepair.RegisterResettable("creative", "自由创作",
		"自由创作的规划日志（上一轮计划、备忘、参考资料、错误与收尾）；reset 清空日志",
		resetJournal)
}

// resetJournal 清空规划日志，让下一轮调度从空白计划开始。
func resetJournal() string {
	saveJournal(journalPath(fileflow.ModeCreator), journal{Version: 1})
	saveJournal(journalPath(fileflow.ModeAssigned), journal{Version: 1})
	logger.Warn(Module, "[拯救] 自由创作规划日志已清空")
	return "已清空自由创作的规划日志"
}
