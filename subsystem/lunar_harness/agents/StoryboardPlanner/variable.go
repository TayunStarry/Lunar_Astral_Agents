package StoryboardPlanner

// variable.go 分镜规划-拆解的常量与模型调用参数。

import (
	"time"

	"LunarSubsystem/LunarHarness/protocol/agentinfo"
)

const (
	// AgentID 故障归属与数据导出 ID
	AgentID = "storyboard_planner"
	// modelRole 推理实况屏显示名
	modelRole = agentinfo.NameStoryboardPlan
	// promptName 分镜师提示词文件
	promptName = "storyboardPlanner.md"
	// planDelimiter 推演段与结构化分镜段之间的分隔行
	planDelimiter = "---STORYBOARD---"
	// maxAttempts 无任何可解析分镜时的重试次数
	maxAttempts = 2
	// maxStoryRunes 故事正文上限
	maxStoryRunes = 4000
	// maxPaintPromptRunes 单镜绘制提示词上限
	maxPaintPromptRunes = 1600
	// defaultRatio 未指定画幅时的默认值
	defaultRatio = "16:9"
	// fallbackTitle 模型未给标题时的兜底
	fallbackTitle = "未命名故事"
	// planTimeout 单次模型调用超时
	planTimeout = 3 * time.Minute
)
