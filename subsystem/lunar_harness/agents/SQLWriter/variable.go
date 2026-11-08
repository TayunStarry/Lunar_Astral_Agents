package SQLWriter

// variable.go 全局变量与常量集中声明（类型定义见 type.go）。

import (
	"time"

	"LunarSubsystem/LunarHarness/protocol/agentinfo"
)

const (
	// AgentID 故障归属与导出 ID
	AgentID = "sql_writer"
	// modelRole 推理实况屏显示名
	modelRole = agentinfo.NameSQLWriter
	// promptName 语句生成提示词文件（相对 prompts/）
	promptName = "sqlWriter.md"
	// module 日志模块名
	module = "SQLWriter"
	// loopRetries 模型输出陷入重复时的额外重试次数
	loopRetries = 1
	// requestTimeout 单次生成请求超时
	requestTimeout = 120 * time.Second
)
