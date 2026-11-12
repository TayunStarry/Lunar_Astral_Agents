package ViewerCensor

// type.go 观影者-审查的输入类型。

// ReviewInput 一次合规复核的输入。
type ReviewInput struct {
	// Title 视频标题（只用于日志）
	Title string
	// Material 复核材料（标题 / UP主 / 简介 / 评论节选 / 标签 / 时长）
	Material string
	// Cover 封面 data URL（空表示纯文本复核）
	Cover string
	// Model 模型名
	Model string
}
