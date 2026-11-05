package creation

// repair.go 向拯救者注册自由创作的上下文修复能力。

import (
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/fileflow"
	"LunarSubsystem/LunarHarness/protocol/agentrepair"
)

func init() {
	agentrepair.RegisterResettable("creative", "自由创作",
		"自由创作的跨轮记录（上一轮计划、备忘、参考资料、错误与收尾）；reset 清空这些记录",
		resetJournal,
		func(int) (any, error) { return exportContext(false) })
}

// resetJournal 清空两种模式的调用轮次记录，让下一轮调度从空白计划开始；文件变更与回退记录保留。
func resetJournal() string {
	if err := fileflow.ResetRounds(fileflow.ModeCreator); err != nil {
		logger.Warn(Module, "[拯救] 清空自由创作记录失败: %v", err)
	}
	if err := fileflow.ResetRounds(fileflow.ModeAssigned); err != nil {
		logger.Warn(Module, "[拯救] 清空指派模式记录失败: %v", err)
	}
	logger.Warn(Module, "[拯救] 自由创作跨轮记录已清空")
	return "已清空自由创作的跨轮记录"
}
