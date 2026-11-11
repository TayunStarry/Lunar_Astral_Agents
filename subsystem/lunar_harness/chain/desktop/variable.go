//go:build windows

package desktopchain

// variable.go 桌控链的链级常量。
//
// chainScopeID / chainScopeLabel 是「链级故障归属与导出标识」，不是任何智能体的身份：
// 本族没有名为 desktop_operator 的成员，感知 / 规划 / 执行各用自己的成员 ID。
// 与 chain/viewer 的 bilibili（chainScopeID / chainScopeLabel）保持同一形态。

import (
	"LunarSubsystem/LunarHarness/protocol/agentinfo"
)

const (
	// chainScopeID 链级故障归属 ID（智能修复、导出与只读诊断共用）。
	chainScopeID = "desktop_operator"
	// chainScopeLabel 链级实况显示名与导出条目标签。
	chainScopeLabel = agentinfo.NameDesktopOperator
)
