package Vision

// variable.go 视觉理解智能体的常量与工具名（业务逻辑不得写在本文件）

const (
	// roleLabel TUI 实况屏显示的角色标签
	roleLabel = "视觉理解"
	// summaryStoreChars 写回资源目录的摘要最大字符数
	summaryStoreChars = 1000
)

// 视觉理解工具名
const (
	// ToolDescribe 观看图片 / 动图并生成或返回内容描述
	ToolDescribe = "vision_describe"
	// ToolAsk 对图片 / 动图提出定向问题
	ToolAsk = "vision_ask"
	// ToolWatch 解读本机视频地址
	ToolWatch = "vision_watch_video"
)
