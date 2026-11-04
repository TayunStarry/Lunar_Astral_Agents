package ViewerScreening

// variable.go 观影者-阅片的常量集中管理。

const (
	// AgentID 故障归属 ID
	AgentID = "viewer_screening"
	// Role 推理实况角色标签
	Role = "观影者-阅片"

	// 提示词文件名
	promptImpress  = "viewerScreeningImpress.md"
	promptQuick    = "viewerScreeningQuick.md"
	promptDirect   = "viewerScreeningDirect.md"
	promptKeywords = "viewerScreeningKeywords.md"

	// impressAttempts 印象整理的尝试次数
	impressAttempts = 3
	// onceAttempts 单次语义的调用（切片观看）不做重试
	onceAttempts = 1
	// summaryMaxRunes 印象摘要的长度上限
	summaryMaxRunes = 512
	// instructionMaxRunes 指令拼进提示词的长度上限
	instructionMaxRunes = 512
)
