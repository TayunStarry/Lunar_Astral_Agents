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
	// maxOutputTokens 单次调用输出上限（含思考）
	// 资料判断本应 1-3 句 + 一行 JSON；设上限是为了让推理模型在空转时尽快收尾，
	// 而不是逐词自问自答几十秒后才被循环监管中断。
	maxOutputTokens = 2048
	// researchTimeout 单次模型调用超时
	researchTimeout = 30 * time.Second
)
