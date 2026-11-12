package StoryWriter

// variable.go 故事编写智能体的标识、角色、提示词与运行参数。

import "time"

const (
	// AgentID 故事创作智能体标识
	AgentID = "story_writer"
	// Role 推理实况中显示的角色名
	Role = "故事创作"
	// DisplayName 控制台工具目录中的显示名
	DisplayName = "故事创作"
	// promptFile 故事创作提示词
	promptFile = "storyWriter.md"
	// maxRounds 一章的建纲、读取、写作与进度更新通常需要多轮工具调用
	maxRounds = 40
	// maxTokens 单次输出上限；整章正文会作为工具参数输出，额度不足会在字符串中途截断，
	// llama-server 随即把不完整的 JSON 判为 500，因此这里给到工具循环允许的最大额度。
	maxTokens = 16384
	// writeTimeout 单次模型调用超时；整章正文生成可能明显长于普通文件操作
	writeTimeout = 5 * time.Minute

	// storyRoot 故事目录的根
	storyRoot = "documents"
	// outlineFile 故事大纲文件名
	outlineFile = "大纲.md"
	// progressFile 故事进度文件名
	progressFile = "进度.md"
)
