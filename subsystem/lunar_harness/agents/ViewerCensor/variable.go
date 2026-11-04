package ViewerCensor

// variable.go 观影者-审查的常量集中管理。

import "time"

const (
	// AgentID 故障归属 ID
	AgentID = "viewer_censor"
	// Role 推理实况角色标签
	Role = "观影者-审查"
	// promptReview 审核提示词
	promptReview = "viewerCensorReview.md"
	// reviewAttempts 审核尝试次数（首次带封面，其余纯文本兜底）
	reviewAttempts = 3
	// retryBackoff 审核失败重试的基础退避
	retryBackoff = time.Second
)
