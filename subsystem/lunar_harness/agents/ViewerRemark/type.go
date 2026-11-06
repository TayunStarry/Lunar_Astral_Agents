package ViewerRemark

// type.go 观影者-观感的输入类型。

// RemarkInput 一条观感评论的输入：画面理解 + 视频印象 + 对话者的附加要求。
type RemarkInput struct {
	// Persona 人设提示词
	Persona string
	// Material 画面理解、视频印象与片段位置等材料
	Material string
	// Model 模型名
	Model string
}
