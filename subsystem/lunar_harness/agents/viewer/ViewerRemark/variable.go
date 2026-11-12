package ViewerRemark

// variable.go 观影者-观感的常量集中管理。

import "LunarSubsystem/LunarHarness/protocol/agentinfo"

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
