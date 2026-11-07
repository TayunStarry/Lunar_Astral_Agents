package SearchSummarizer

// variable.go 全局变量与常量集中声明（类型定义见 type.go）。

import "LunarSubsystem/LunarHarness/protocol/agentinfo"

const (
	// AgentID 故障归属与导出 ID
	AgentID = "search_summarizer"
	// modelRole 推理实况屏显示名
	modelRole = agentinfo.NameSearchSummarizer
	// promptName 摘要提示词文件（`---user---` 分隔系统段与用户模板）
	promptName = "searchSummarizerDigest.md"
	// promptUserMarker 任务型提示词的「系统段 / 用户模板」分隔符
	promptUserMarker = "\n---user---\n"
	// loopRetries 模型输出陷入重复时的额外重试次数
	loopRetries = 1
	// pageDigestMaxRunes 正文兜底摘要的单页字符上限
	pageDigestMaxRunes = 800
)
