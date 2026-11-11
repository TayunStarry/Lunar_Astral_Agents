//go:build windows

package plan

// type.go 「桌面规划」成员的类型定义。

import "LunarSubsystem/LunarHarness/agents/desktop"

// plannerContext 一次规划决策所需的全部输入，集中传递避免参数散落。
type plannerContext struct {
	plan        desktop.EditorTask // 结构化任务
	handoff     desktop.HandoffRecord
	treeSummary string   // UIA 树摘要
	elements    []string // 与任务相关的元素清单
}
