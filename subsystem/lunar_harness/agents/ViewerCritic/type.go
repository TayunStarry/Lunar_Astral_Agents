package ViewerCritic

// type.go 观影者-评论的输入类型。

// InboxInput 一条被动消息的回应输入。
type InboxInput struct {
	// Persona 人设提示词
	Persona string
	// Scene 互动发生的场景（如「《标题》的视频评论区」）
	Scene string
	// Material 对方消息与上下文材料
	Material string
	// Model 模型名
	Model string
}

// MemoryInput 一轮观影回顾的输入。
type MemoryInput struct {
	// Persona 人设提示词
	Persona string
	// Material 本轮观影感受与互动材料
	Material string
	// Model 模型名
	Model string
}
