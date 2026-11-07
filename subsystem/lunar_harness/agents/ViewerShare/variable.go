package ViewerShare

// variable.go 观影者-分享的常量集中管理。

import "LunarSubsystem/LunarHarness/protocol/agentinfo"

const (
	// AgentID 故障归属 ID
	AgentID = "viewer_share"
	// Role 推理实况角色标签
	Role = agentinfo.NameViewerShare
	// promptDynamic 观影动态提示词
	promptDynamic = "viewerShareDynamic.md"
	// promptShare 分享评论提示词
	promptShare = "viewerShareComment.md"
	// attempts 分享类调用的尝试次数
	attempts = 3
	// taskBlock 人设与任务的分隔格式
	taskBlock = "\n\n## 本次任务\n\n"
	// summaryMaxRunes 素材摘要的长度上限
	summaryMaxRunes = 512
)
