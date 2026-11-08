package CreatorPlanner

// variable.go 创作规划者的智能体标识、角色名、循环与工具预算常量，以及运行记录状态。

import (
	"sync"
	"time"

	"LunarSubsystem/LunarHarness/protocol/agentinfo"
)

const (
	// AgentID 推理实况、数据导出与失败上报共用的智能体标识。
	AgentID = "creator"
	// PlannerRole 调度者角色名（行为判别条件，必须与链侧传值保持一致，勿改）。
	PlannerRole = "自由创作.调度者"
	// DisplayName 导出、维修与目录展示用的显示名。
	DisplayName = agentinfo.NameCreator

	// AgentWriter 文字创作者角色。
	AgentWriter = "writer"
	// AgentEngineer 工程创作者角色。
	AgentEngineer = "engineer"
	// AgentResearcher 资料查阅者角色。
	AgentResearcher = "researcher"
	// AgentRunner 脚本运行者角色。
	AgentRunner = "runner"
	// AgentPainter 图片绘制者角色：由创作链把绘图需求交给现有图片绘制智能体执行。
	AgentPainter = "painter"

	// MaxPlannerRounds 调度者推理轮次上限。
	//
	// 调度者要做「列目录 → 查资源 → 读文件 → 核对变更 → 提交计划」这类多步取证，
	// 旧值 6 轮会在真正提交计划前就被判定失败；这里给足余量，但仍保留硬上限兜底，
	// 防止模型无意义空转无限消耗模型调用与上下文预算。
	MaxPlannerRounds = 24
	// MaxExecutorRounds 单步智能体推理轮次上限。
	//
	// 单步执行者要覆盖「读文件 → 识别 → 写入 / 改写 → 校验」的完整闭环，
	// 旧值 8 轮对多文件步骤偏紧；放宽到 32 轮，同样保留硬上限。
	MaxExecutorRounds = 32
	// ModelTimeout 单次模型调用超时。
	ModelTimeout = 90 * time.Second

	// ToolMessageMaxRunes 单条工具结果回填模型的最大字符数。
	ToolMessageMaxRunes = 6000
	// ToolBudgetRunes 一次工具循环里全部工具结果的字符预算。
	ToolBudgetRunes = 18000
	// ToolPrunedRunes 收紧较早的工具结果时保留的字符数。
	ToolPrunedRunes = 512

	// FileReadChunkLimit 单次 file_read 允许的最大字符数（工具 schema 上限）。
	FileReadChunkLimit = 8000
	// MaxChangeQuery 单次 change_query 返回的文件变更次数上限（工具 schema 上限）。
	MaxChangeQuery = 10

	// maxRecordedToolCalls 单次循环记录的工具调用条数上限。
	//
	// 只影响诊断序列（失败简报、数据导出），不影响工具能否执行；
	// 与新的轮次上限对齐，避免一轮多调用时记录被截断、排障缺失关键证据。
	maxRecordedToolCalls = 64
)

// ==== 提示词文件 ====

const (
	// promptPlanner 调度者系统提示模板（含 {directives} / {mode} / {journal-hint} / {path-snapshot} 占位）。
	promptPlanner = "creatorPlanner.md"
	// promptExecutor 专属智能体共用系统提示。
	promptExecutor = "creatorExecutor.md"
	// promptWriter 文字创作者附加提示。
	promptWriter = "creatorWriter.md"
	// promptEngineer 工程创作者附加提示。
	promptEngineer = "creatorEngineer.md"
	// promptResearcher 资料查阅者附加提示。
	promptResearcher = "creatorResearcher.md"
	// promptRunner 脚本运行者附加提示。
	promptRunner = "creatorRunner.md"
)

var (
	// lastRunMu 保护最近一次推理记录。
	lastRunMu sync.Mutex
	// lastRun 最近一次推理循环的摘要（供数据导出与修复复位）。
	lastRun = map[string]any{}

	// policyMu 保护本次循环的工具调用记录。
	policyMu sync.Mutex
	// toolCalls 本次（或最近一次）循环的工具调用序列。
	toolCalls []toolCallRecord
	// toolCallCounts 工具名 → 调用次数。
	toolCallCounts = map[string]int{}
)
