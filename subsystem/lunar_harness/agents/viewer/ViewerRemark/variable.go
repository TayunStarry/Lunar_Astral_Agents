package ViewerRemark

// variable.go 观影者-观感的常量集中管理。

import (
	"LunarSubsystem/LunarHarness/protocol/agentbase"
	"LunarSubsystem/LunarHarness/protocol/agentinfo"
)

const (
	// AgentID 故障归属 ID
	AgentID = "viewer_remark"
	// Role 推理实况角色标签
	Role = agentinfo.NameViewerRemark
	// promptRemark 观感评论提示词
	promptRemark = "viewerRemark.md"
	// personaFile 人设提示词文件名
	personaFile = "lifeRole.md"
	// attempts 观感评论的尝试次数：失败可重试，避免一条评论都留不下
	attempts = 3
	// maxComments 一次最多生成 2 条评论
	maxComments = 2
	// commentMaxRunes 单条评论的长度上限
	commentMaxRunes = 120
	// mentionMax 一次最多 @ 3 个特别关注
	mentionMax = 3
	// taskBlock 人设与任务的分隔格式
	taskBlock = "\n\n## 本次任务\n\n"
)

// remarkTool 观感评论的唯一提交通道：必填工具调用，正文不解析。
// 它只是模型与链之间的提交约定，没有执行器，因此不登记到 toolreg。
var remarkTool = agentbase.ToolDefinition{
	Type: "function",
	Function: agentbase.ToolFunctionDef{
		Name:        "submit_remark",
		Description: "提交 1-2 条观感评论（唯一输出通道）。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"comments": map[string]any{
					"type":     "array",
					"minItems": 1,
					"maxItems": maxComments,
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"text": map[string]any{
								"type":        "string",
								"description": "可直接发布的评论正文，60 字以内",
							},
							"mention": map[string]any{
								"type":        "integer",
								"minimum":     0,
								"maximum":     mentionMax,
								"description": "这条评论想 @ 的特别关注人数：0 表示不 @，1-3 表示想 @ 几个人",
							},
							"at": map[string]any{
								"type":        "array",
								"maxItems":    mentionMax,
								"description": "候选清单里的序号或名字，只能从清单里选",
								"items": map[string]any{
									"anyOf": []any{
										map[string]any{"type": "string"},
										map[string]any{"type": "integer"},
									},
								},
							},
						},
						"required":             []string{"text", "mention"},
						"additionalProperties": false,
					},
				},
			},
			"required":             []string{"comments"},
			"additionalProperties": false,
		},
	},
}
