package ViewerCensor

// variable.go 观影者-审查的常量集中管理。

import (
	"time"

	"LunarSubsystem/LunarHarness/protocol/agentbase"
	"LunarSubsystem/LunarHarness/protocol/agentinfo"
)

const (
	// AgentID 故障归属 ID
	AgentID = "viewer_censor"
	// Role 推理实况角色标签
	Role = agentinfo.NameViewerCensor
	// promptReview 审核提示词
	promptReview = "viewerCensorReview.md"
	// reviewAttempts 审核尝试次数（首次带封面，其余纯文本兜底）
	reviewAttempts = 3
	// retryBackoff 审核失败重试的基础退避
	retryBackoff = time.Second
)

// reviewTool 审查结论的唯一提交通道：必填工具调用，正文不解析。
// 它只是模型与链之间的提交约定，没有执行器，因此不登记到 toolreg。
var reviewTool = agentbase.ToolDefinition{
	Type: "function",
	Function: agentbase.ToolFunctionDef{
		Name:        "submit_review",
		Description: "提交合规审查结论（唯一输出通道）。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"pass": map[string]any{
					"type":        "boolean",
					"description": "true 表示适合公开讨论；false 表示不适合",
				},
				"reason": map[string]any{
					"type":        "string",
					"description": "pass=false 时的具体原因；pass=true 时填空字符串",
				},
			},
			"required":             []string{"pass", "reason"},
			"additionalProperties": false,
		},
	},
}
