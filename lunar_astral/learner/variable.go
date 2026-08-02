package agent

import (
	"sync"
)

// ==== 学习者全局变量 ====

// learnerInstance 学习者实例
var learnerInstance *Learner

// learnerMutex 学习者实例互斥锁
var learnerMutex sync.RWMutex

// ==== 常量 ====

const (
	// LearnerMemoryTable 学习者专用记忆表名
	LearnerMemoryTable = "online_learning"

	// MaxDebateRounds 最大辩论轮次
	MaxDebateRounds = 5

	// MaxSearchSubQuestions 最大子问题数
	MaxSearchSubQuestions = 4

	// MemoryUpdateSimilarityThreshold 记忆更新相似度阈值（余弦相似度）
	MemoryUpdateSimilarityThreshold = 0.75

	// MemoryQueryTopK 记忆查询默认返回条数
	MemoryQueryTopK = 10

	// DefaultMaxToolCallRounds 默认工具调用最大轮次
	DefaultMaxToolCallRounds = 8

	// DebateSummaryMaxChars 辩论摘要最大字符数
	DebateSummaryMaxChars = 300

	// SearchResultMaxChars 单次搜索结果压缩后最大字符数
	SearchResultMaxChars = 1500

	// TotalSearchResultMaxChars 搜索结果总量最大字符数（所有子问题合计）
	TotalSearchResultMaxChars = 4500

	// ContextMaxTokens 单次 LLM 调用上下文最大 token 数
	ContextMaxTokens = 20480

	// CharPerToken 中文字符与 token 的估算比率（1 token ≈ 1.5 中文字符）
	CharPerToken = 1.5

	// OutputTokenReserve 输出 token 预留量
	OutputTokenReserve = 4096

	// InputTokenReserve 输入 token 预留量（= ContextMaxTokens - OutputTokenReserve）
	InputTokenReserve = ContextMaxTokens - OutputTokenReserve

	// DialogueHistoryLimit 对话历史读取条数
	DialogueHistoryLimit = 15

	// UnreadCheckCount 未读消息检查条数
	UnreadCheckCount = 10
)

// ==== Token 预算预设 ====

var (
	// BudgetAnalyze 问题分析阶段预算
	BudgetAnalyze = TokenBudget{MaxInput: 4000, MaxOutput: 1000}

	// BudgetSearchCompress 搜索结果压缩预算
	BudgetSearchCompress = TokenBudget{MaxInput: 6000, MaxOutput: 1500}

	// BudgetDebate 辩论阶段预算
	BudgetDebate = TokenBudget{MaxInput: 8000, MaxOutput: 2500}

	// BudgetConvergence 收敛判断预算
	BudgetConvergence = TokenBudget{MaxInput: 3000, MaxOutput: 300}

	// BudgetReportOutline 报告提纲预算
	BudgetReportOutline = TokenBudget{MaxInput: 10000, MaxOutput: 1500}

	// BudgetReportFull 完整报告预算
	BudgetReportFull = TokenBudget{MaxInput: 12000, MaxOutput: 4000}

	// BudgetMemoryUpdate 记忆更新预算
	BudgetMemoryUpdate = TokenBudget{MaxInput: 4000, MaxOutput: 800}
)
