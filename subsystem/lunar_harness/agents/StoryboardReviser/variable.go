package StoryboardReviser

// variable.go 分镜规划-修订的常量与模型调用参数。

import (
	"time"

	"LunarSubsystem/LunarHarness/protocol/agentinfo"
)

const (
	// AgentID 故障归属与数据导出 ID
	AgentID = "storyboard_revise"
	// modelRole 推理实况屏显示名
	modelRole = agentinfo.NameStoryboardRevise
	// promptName 修订师提示词文件
	promptName = "storyboardReview.md"
	// maxShots 单次修订的镜头上限（与审查上限一致）
	maxShots = 200
	// defaultRatio 未指定画幅时的默认值
	defaultRatio = "16:9"
	// reviseTimeout 单次模型调用超时
	reviseTimeout = 3 * time.Minute
)
