package LifeMemorizer

// variable.go 生活记忆整理者的标识、整理阈值、提示词文件与运行记录状态。

import (
	"sync"

	"LunarSubsystem/LunarHarness/protocol/agentinfo"
)

const (
	// AgentID 推理实况、数据导出与失败上报共用的智能体标识。
	AgentID = "life_memorizer"
	// RoleName 实况角色标签（记忆整理按次执行，每次模型调用各自一条实况）。
	RoleName = agentinfo.NameLifeMemorizer

	// MaxTextRunes 单条整理后记忆的最大字符数（超出时在句读处结束，不硬切半句）。
	MaxTextRunes = 512
	// KeepPercent 整理后保留下限占输入条数的百分比（防止模型把整批压成寥寥几条）。
	// 从 25 提高到 50：整理只允许温和合并，宁可条目多、每条短，也不丢事实。
	KeepPercent = 50
	// KeepFloor 保留下限的最小条数（小批次也要至少保留几条）。
	KeepFloor = 8
	// MinRetainPercent 整批条目化（Consolidate）输出正文总字数占输入总字数的下限。
	// 只用于「把一批旧记忆重新条目化」这条通道，不用于相似簇合并（见 MergeMinRetainPercent）。
	MinRetainPercent = 20
	// MergeMinRetainPercent 相似簇合并结果正文总字数占输入总字数的下限。
	// 合并的输入本身就是高度重复的记忆，正常结果必然远短于输入总长，
	// 因此这里只拦「几乎什么都没留下」的情况，取值远低于 MinRetainPercent。
	MergeMinRetainPercent = 5
	// MergeMinRunes 相似簇合并结果正文总量的最小绝对字数（非空白字符）。
	// 小簇合并时不按比例，只要求留下这么多个字；实际使用时不超过最长单条输入，
	// 避免 8 条完全相同的短记忆合成 1 条时被固定下限误判。
	MergeMinRunes = 12
	// KeepCeiling 保留下限的最大条数（大批次不要求输出过多条目）。
	KeepCeiling = 48
	// ModelRetry 模型输出解析失败后的额外重试次数。
	ModelRetry = 1

	// promptRole 生活体系人格提示词。
	promptRole = "lifeRole.md"
	// promptTask 记忆条目化整理任务模板（含 {item-count} / {min-keep} 占位）。
	promptTask = "lifeMemoryTask.md"

	// maxRecordedInputs 复跑验证时保留的上次输入条数。
	maxRecordedInputs = 12
	// maxRecordedInputRunes 复跑验证保留的输入总字数上限。
	maxRecordedInputRunes = 8000
)

var (
	// lastRunMu 保护最近一次整理记录。
	lastRunMu sync.Mutex
	// lastRun 最近一次整理任务的摘要（供数据导出与修复复位）。
	lastRun = map[string]any{}

	// inputsMu 保护最近一次合并的输入（仅用于修复后复跑验证）。
	inputsMu sync.Mutex
	// lastInputs 最近一次合并的输入条目（有界副本）。
	lastInputs []string
)
