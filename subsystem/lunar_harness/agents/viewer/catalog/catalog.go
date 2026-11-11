// Package catalog 观影族的工具目录汇总入口。
//
// 族根不 import 成员，调用方也不应散落 import 四个成员包：控制台与观影链
// 需要一次性取到全部成员的阶段能力时，统一从这里取。
package catalog

import (
	"LunarSubsystem/LunarHarness/agents/viewer/ViewerCensor"
	"LunarSubsystem/LunarHarness/agents/viewer/ViewerCritic"
	"LunarSubsystem/LunarHarness/agents/viewer/ViewerScreening"
	"LunarSubsystem/LunarHarness/agents/viewer/ViewerShare"
	"LunarSubsystem/LunarHarness/protocol/agentinfo"
)

// AgentIDs 返回在 protocol/toolreg 登记过阶段能力的成员 ID（顺序：阅片 / 审查 / 评论 / 分享）。
func AgentIDs() []string {
	return []string{
		ViewerScreening.AgentID,
		ViewerCensor.AgentID,
		ViewerCritic.AgentID,
		ViewerShare.AgentID,
	}
}

// ToolCatalogs 返回族内各成员的工具目录（顺序与 AgentIDs 一致）。
func ToolCatalogs() []agentinfo.Agent {
	out := make([]agentinfo.Agent, 0, 4)
	out = append(out, ViewerScreening.ToolCatalog()...)
	out = append(out, ViewerCensor.ToolCatalog()...)
	out = append(out, ViewerCritic.ToolCatalog()...)
	out = append(out, ViewerShare.ToolCatalog()...)
	return out
}
