package MutualForum

// repair.go 向拯救者注册互助论坛智能体的模型侧修复能力。
//
// 跨轮上下文由 chain/forum 维护并单独注册；这里只重置最近一次模型轮次诊断信息。

import (
	"LunarSubsystem/LunarHarness/protocol/agentrepair"
)

func init() {
	agentrepair.RegisterResettable("mutual_forum_agent", "互助论坛-模型",
		"互助论坛智能体最近一次模型轮次诊断信息；reset 清除该记录",
		resetForumAgentRun,
		func(int) (any, error) { return exportContext(false) })
}

// resetForumAgentRun 清空最近一次模型轮次诊断信息。
func resetForumAgentRun() string {
	resetLastRun()
	return "已清除互助论坛智能体的最近一次模型轮次记录"
}
