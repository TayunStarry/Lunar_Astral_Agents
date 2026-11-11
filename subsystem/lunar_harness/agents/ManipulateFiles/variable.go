package ManipulateFiles

// variable.go 文件操纵智能体的标识、角色与提示词。

const (
	// AgentID 推理实况与失败上报使用的智能体标识
	AgentID = "manipulate_files"
	// Role 推理实况中显示的角色名
	Role = "文件操纵"
	// DisplayName 控制台与目录展示用的显示名
	DisplayName = "文件操纵"
	// promptFile 系统提示词文件
	promptFile = "manipulateFiles.md"
	// maxRounds 工具循环轮次上限
	maxRounds = 32
)
