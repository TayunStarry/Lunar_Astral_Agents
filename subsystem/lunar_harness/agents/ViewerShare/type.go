package ViewerShare

// type.go 观影者-分享的输入类型。

// DynamicInput 观影动态的输入。
type DynamicInput struct {
	// Persona 人设提示词
	Persona string
	// Material 本轮视频印象、观看感受与互动材料
	Material string
	// Model 模型名
	Model string
}

// ShareInput 分享评论的输入。
type ShareInput struct {
	// Title 视频标题
	Title string
	// Owner UP 主
	Owner string
	// Summary 这一轮的整体感受或初步印象
	Summary string
	// Model 模型名
	Model string
}
