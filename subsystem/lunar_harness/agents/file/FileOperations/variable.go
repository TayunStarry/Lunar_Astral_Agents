package FileOperations

// variable.go 文件操作智能体的标识、角色与提示词。

const (
	AgentID     = "file_operations"
	Role        = "文件编辑"
	DisplayName = "文件编辑"
	promptFile  = "fileOperations.md"
	// maxRounds 创作工具循环轮次上限
	maxRounds = 32
	// maxRepairRetries 失败后先让创作系统自行修复的重试次数
	maxRepairRetries = 3
)
