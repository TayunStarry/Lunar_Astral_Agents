package SearchPlanner

// variable.go 全局变量与常量集中声明（类型定义见 type.go）。

import "LunarSubsystem/LunarHarness/protocol/agentinfo"

const (
	// AgentID 故障归属与导出 ID
	AgentID = "search_planner"
	// modelRole 推理实况屏显示名
	modelRole = agentinfo.NameSearchPlanner
	// promptName 规划提示词文件
	promptName = "searchPlannerRoute.md"
	// loopRetries 模型输出陷入重复时的额外重试次数
	loopRetries = 1
	// PipelineSearch 联网检索管线（兜底管线）
	PipelineSearch = "search"
	// PipelineWeather 天气查询管线（纯 API）
	PipelineWeather = "weather"
	// routeConfidenceMin 低于该把握时一律回落检索管线
	routeConfidenceMin = 0.6
	// routeQueryMaxRunes 检索词/城市参数的最大字符数
	routeQueryMaxRunes = 512
	// decisionToolName 结构化决策工具名
	decisionToolName = "decide_search_plan"
)

// routePipelines 结构化路由注册表：新增直连能力只需在此加一条，
// 提示词候选列表与合法性校验都从这里生成，不需要额外的正则或分支判断。
var routePipelines = []routePipeline{
	{
		ID:          PipelineSearch,
		Description: "联网检索：查资料、新闻、百科、网页内容或任何实时信息；query 填简洁检索词",
	},
	{
		ID:          PipelineWeather,
		Description: "天气查询：当前天气、气温、降水、预报、空气质量；args.city 填城市名（中文优先），确实不知道城市时留空按 IP 定位",
	},
}
