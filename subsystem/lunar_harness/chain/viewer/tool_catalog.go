package viewer

// tool_catalog.go 哔哩观影的能力目录（供控制台「工具列表」屏展示）。
//
// 目录内容不再在链侧硬编码：由观影族成员的汇总入口派生，
// 链侧只依赖族根下的 catalog，不直接 import 各成员包。

import (
	viewercatalog "LunarSubsystem/LunarHarness/agents/viewer/catalog"
	"LunarSubsystem/LunarHarness/protocol/agentinfo"
)

// ToolCatalog 返回哔哩观影的内部能力。
//
// 哔哩观影是「真实浏览器 + Tick 状态机」智能体，不使用 function calling；
// 这里列出四个观影者在各阶段实际承担的模型能力：启动阶段一次性完成，观看阶段每个 Tick 一步。
func ToolCatalog() []agentinfo.Agent {
	return []agentinfo.Agent{{
		ID: "bilibili_viewer", Name: "哔哩观影", Kind: agentinfo.KindChain,
		Note:  "观影智能体：选片、观看、评论与观影记忆整理。",
		Tools: agentinfo.ToolsFromRegistry(viewercatalog.AgentIDs()...),
	}}
}
