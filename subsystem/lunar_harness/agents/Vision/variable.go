package Vision

// variable.go 视觉理解智能体的常量与工具名（业务逻辑不得写在本文件）

import (
	"time"

	"LunarSubsystem/LunarHarness/protocol/agentinfo"
)

const (
	// roleLabel TUI 实况屏显示的角色标签
	roleLabel = agentinfo.NameVision
	// summaryStoreChars 写回资源目录的摘要最大字符数
	summaryStoreChars = 1000
	// remoteVideoDownloadTimeout 远程视频回源超时（沿用原有 5 分钟）
	remoteVideoDownloadTimeout = 5 * time.Minute
	// maxRemoteVideoBytes 远程视频回源上限（2 GiB）：
	// 与视频分析（watch / keyframe）上限一致，覆盖 QQ 渠道长视频；
	// 同时避免异常大流被整段读入内存。
	maxRemoteVideoBytes = int64(2 << 30)
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
