package ViewerCensor

// type.go 观影者-审查的输入类型。

// ReviewInput 一次合规复核的输入。
type ReviewInput struct {
	// Title 视频标题（只用于日志）
	Title string
	// Material 复核材料（标题 / UP主 / 简介 / 评论节选 / 视频印象 / 时长）
	Material string
	// Cover 封面 data URL（空表示纯文本复核）
	Cover string
	// Model 模型名
	Model string
	// DefaultSlices 拿不准时的默认切片数
	DefaultSlices int
	// MinSlices / MaxSlices 模型可选的切片数范围
	MinSlices int
	MaxSlices int
}
