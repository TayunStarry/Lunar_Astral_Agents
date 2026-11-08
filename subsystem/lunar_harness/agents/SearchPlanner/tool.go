package SearchPlanner

// tool.go 决策工具的 schema：参数结构由管线注册表生成，新增管线无需改这里。

import "LunarSubsystem/LunarHarness/protocol/agentbase"

// decisionTool 暴露给模型的唯一工具：提交搜索规划决策。
func decisionTool() agentbase.ToolDefinition {
	return agentbase.ToolDefinition{
		Type: "function",
		Function: agentbase.ToolFunctionDef{
			Name:        decisionToolName,
			Description: "提交搜索规划决策。必须调用本工具，禁止把参数写在正文里。",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"pipeline": map[string]any{
						"type":        "string",
						"enum":        pipelineIDs(),
						"description": "命中的处理管线，必须取自候选列表",
					},
					"query": map[string]any{
						"type":        "string",
						"description": "检索词或展示词；检索管线填简洁关键词，天气管线可为空",
					},
					"args": map[string]any{
						"type":        "object",
						"description": "管线附加参数（可选）：天气管线填 city；其他管线可携带过滤条件、提示词等扩展参数",
						"properties": map[string]any{
							"city": map[string]any{
								"type":        "string",
								"description": "城市名（中文优先），确实不知道时留空按 IP 定位",
							},
						},
						"additionalProperties": true,
					},
					"confidence": map[string]any{
						"type":        "number",
						"minimum":     0,
						"maximum":     1,
						"description": "判断把握（0 到 1），低于 0.6 会回落联网检索",
					},
				},
				"required":             []string{"pipeline", "query"},
				"additionalProperties": false,
			},
		},
	}
}

// pipelineIDs 返回注册表中的全部管线 id，作为工具枚举值。
func pipelineIDs() []string {
	ids := make([]string, 0, len(routePipelines))
	for _, pipeline := range routePipelines {
		ids = append(ids, pipeline.ID)
	}
	return ids
}
