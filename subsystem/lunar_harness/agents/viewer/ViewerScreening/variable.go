package ViewerScreening

// variable.go 观影者-阅片的常量集中管理。

import (
	"LunarSubsystem/LunarHarness/protocol/agentbase"
	"LunarSubsystem/LunarHarness/protocol/agentinfo"
)

const (
	// AgentID 故障归属 ID
	AgentID = "viewer_screening"
	// Role 推理实况角色标签
	Role = agentinfo.NameViewerScreening

	// 提示词文件名
	promptImpress = "viewerScreeningImpress.md"
	promptQuick   = "viewerScreeningQuick.md"
	promptDirect  = "viewerScreeningDirect.md"
	// promptKeywords 选片关键词提示词
	promptKeywords = "viewerScreeningKeywords.md"
	// personaFile 人设提示词文件名（切片观看的人设）
	personaFile = "lifeRole.md"

	// impressAttempts 印象整理的尝试次数
	impressAttempts = 3
	// onceAttempts 单次语义的调用（切片观看）不做重试
	onceAttempts = 1
	// summaryMaxRunes 印象摘要的长度上限
	summaryMaxRunes = 512
	// instructionMaxRunes 指令拼进提示词的长度上限
	instructionMaxRunes = 512
)

// impressionTool 视频印象的唯一提交通道：必填工具调用，正文不解析。
// 它只是模型与链之间的提交约定，没有执行器，因此不登记到 toolreg。
var impressionTool = agentbase.ToolDefinition{
	Type: "function",
	Function: agentbase.ToolFunctionDef{
		Name:        "submit_impression",
		Description: "提交视频印象（唯一输出通道）。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"summary": map[string]any{
					"type":        "string",
					"description": "一两句话概括视频内容与这一分钟的画面，≤120 字",
				},
				"topics": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"minItems":    2,
					"maxItems":    5,
					"description": "2-5 个主题词，优先复用材料里已有的标签",
				},
				"tone": map[string]any{
					"type":        "string",
					"description": "观众整体的情绪或氛围，1-12 字",
				},
			},
			"required":             []string{"summary", "topics", "tone"},
			"additionalProperties": false,
		},
	},
}
