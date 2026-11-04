package LifeMemorizer

// variable.go 生活记忆整理者的标识、整理阈值、提示词文件与运行记录状态。

import (
	"sync"
)

const (
	// AgentID 推理实况、数据导出与失败上报共用的智能体标识。
	AgentID = "life_memorizer"
	// RoleName 实况角色标签（记忆整理按次执行、静默调用）。
	RoleName = "生活记忆整理者"

	// MaxTextRunes 单条整理后记忆的最大字符数（超出时在句读处结束，不硬切半句）。
	MaxTextRunes = 512
	// KeepPercent 整理后保留下限占输入条数的百分比（防止模型把整批压成寥寥几条）。
	KeepPercent = 25
	// KeepFloor 保留下限的最小条数（小批次也要至少保留几条）。
	KeepFloor = 4
	// KeepCeiling 保留下限的最大条数（大批次不要求输出过多条目）。
	KeepCeiling = 48
	// ModelRetry 模型输出解析失败后的额外重试次数。
	ModelRetry = 1

	// promptRole 生活体系人格提示词。
	promptRole = "lifeRole.md"
	// promptTask 记忆条目化整理任务模板（含 {item-count} / {min-keep} 占位）。
	promptTask = "lifeMemoryTask.md"
)

var (
	// lastRunMu 保护最近一次整理记录。
	lastRunMu sync.Mutex
	// lastRun 最近一次整理任务的摘要（供数据导出与修复复位）。
	lastRun = map[string]any{}
)
