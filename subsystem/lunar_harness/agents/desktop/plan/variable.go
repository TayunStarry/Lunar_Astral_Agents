//go:build windows

package plan

// variable.go 「桌面规划」成员的身份、提示词与工具边界。

import (
	"time"

	"LunarSubsystem/LunarHarness/agents/desktop"
	"LunarSubsystem/LunarHarness/protocol/agentinfo"
)

// AgentID 成员唯一标识：推理实况、故障修复、数据导出与工具目录共用这一个 ID。
const AgentID = "desktop_plan"

// modelRole 实况显示名（四字，取自 protocol/agentinfo 统一约定）。
const modelRole = agentinfo.NameDesktopPlan

// autoLoopMaxRetries 模型输出命中重复后，单次模型调用的额外重试次数。
const autoLoopMaxRetries = 2

// modelTimeout 单次模型调用的最长等待时间。
const modelTimeout = 120 * time.Second

// maxRounds 单次模型调用的最大工具轮次（决策一次提交即结束）。
const maxRounds = 1

// 提示词：编纂把口语请求收敛成结构化任务，规划只决定下一步动作。
var (
	promptEditor  = desktop.MustPrompt("desktopEditor.md")
	promptPlanner = desktop.MustPrompt("desktopPlanner.md")
)

// DecisionDone 规划者判断「没有下一步可执行的动作」时的显式决策值。
// 它只表示规划侧无动作，不等于任务已达成——验收由链级收尾核对负责。
const DecisionDone = "no_action"

// decisionTool 规划决策的唯一提交通道：必填工具调用，杜绝正文 JSON 解析失败被当作完成。
var decisionTool = desktop.ToolDef{
	Type: "function",
	Function: desktop.FuncDef{
		Name:        "submit_decision",
		Description: "提交下一步决策（唯一输出通道）。决定这一步做什么，不判断任务是否成功。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"decision": map[string]any{
					"type": "string", "enum": []string{"keyboard", "uia", "mouse", DecisionDone},
					"description": "keyboard/uia/mouse 为要执行的动作类型；no_action 表示没有可执行的下一步",
				},
				"action": map[string]any{"type": "string", "description": "要执行的具体动作（含目标参数）；no_action 时填写结束原因"},
				"via":    map[string]any{"type": "string", "description": "选择该决策的理由"},
			},
			"required": []string{"decision", "action", "via"},
		},
	},
}
