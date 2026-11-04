package CreatorPlanner

// variable.go 创作规划者的智能体标识、角色名、循环与工具预算常量，以及运行记录状态。

import (
	"sync"
	"time"
)

const (
	// AgentID 推理实况、数据导出与失败上报共用的智能体标识。
	AgentID = "creator"
	// PlannerRole 调度者角色名（与迁移前的提示词角色保持一致）。
	PlannerRole = "自由创作.调度者"

	// AgentWriter 文字创作者角色。
	AgentWriter = "writer"
	// AgentEngineer 工程创作者角色。
	AgentEngineer = "engineer"
	// AgentResearcher 资料查阅者角色。
	AgentResearcher = "researcher"
	// AgentRunner 脚本运行者角色。
	AgentRunner = "runner"

	// MaxPlannerRounds 调度者推理轮次上限。
	MaxPlannerRounds = 6
	// MaxExecutorRounds 单步智能体推理轮次上限。
	MaxExecutorRounds = 8
	// MaxRepeats 同一输出/工具调用重复达到该次数即判定循环输出。
	MaxRepeats = 3
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
)

// ==== 提示词文件 ====

const (
	// promptPlanner 调度者系统提示模板（含 {directives} / {mode} / {journal-hint} 占位）。
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
)
