package ViewerCritic

// variable.go 观影者-评论的常量集中管理。

const (
	// AgentID 故障归属 ID
	AgentID = "viewer_critic"
	// Role 推理实况角色标签
	Role = "观影者-评论"
	// promptInbox 被动消息回应提示词
	promptInbox = "viewerCriticInbox.md"
	// promptMemory 观后回顾提示词
	promptMemory = "viewerCriticMemory.md"
	// attempts 评论类调用的尝试次数
	attempts = 3
	// taskBlock 人设与任务的分隔格式
	taskBlock = "\n\n## 本次任务\n\n"
)
