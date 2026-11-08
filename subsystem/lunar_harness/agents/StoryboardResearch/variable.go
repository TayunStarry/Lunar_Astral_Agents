package StoryboardResearch

// variable.go 分镜规划-资料的常量与模型调用参数。

import (
	"time"

	"LunarSubsystem/LunarHarness/protocol/agentinfo"
)

const (
	// AgentID 故障归属与数据导出 ID
	AgentID = "storyboard_research"
	// modelRole 推理实况屏显示名
	modelRole = agentinfo.NameStoryboardResearch
	// promptName 资料员提示词文件
	promptName = "storyboardResearch.md"
	// queryDelimiter 可读判断与结构化检索意图之间的分隔行
	queryDelimiter = "---QUERIES---"
	// maxQueries 单个任务最多执行的检索条数
	maxQueries = 4
	// maxQueryRunes 单条检索词上限
	maxQueryRunes = 120
	// maxStoryRunes 故事正文上限（与演绎链输入约束一致）
	maxStoryRunes = 4000
	// researchTimeout 单次模型调用超时
	researchTimeout = 90 * time.Second
)
