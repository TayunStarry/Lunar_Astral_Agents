package learner

import (
	"sync"
)

// ==== 学习者全局变量 ====

// learnerInstance 学习者实例
var learnerInstance *Learner

// learnerMutex 学习者实例互斥锁
var learnerMutex sync.RWMutex

// ==== 记忆表名常量 ====

const (
	// TableKnowledge 知识记忆表名 — 存储从网络搜索获取的事实性知识
	TableKnowledge = "learner_knowledge"

	// TableExperience 经验记忆表名 — 存储请求处理策略与搜索指导经验
	TableExperience = "learner_experience"
)

// ==== 工作流常量 ====

const (
	// MaxDeepSearchRounds 深度搜索最大轮次
	MaxDeepSearchRounds = 5

	// MaxSearchSubQuestions 最大子问题数（深度搜索拆解用）
	MaxSearchSubQuestions = 4

	// MemoryQueryTopK 记忆查询默认返回条数
	MemoryQueryTopK = 10

	// ExperienceQueryTopK 经验记忆查询返回条数
	ExperienceQueryTopK = 5

	// SimpleSearchMaxResults 初步网络搜索最大结果数
	SimpleSearchMaxResults = 10

	// SearchResultMaxChars 单次搜索结果压缩后最大字符数
	SearchResultMaxChars = 3000

	// DeepSearchResultMaxChars 深度搜索结果最大字符数
	DeepSearchResultMaxChars = 8000

	// ContextMaxTokens 单次 LLM 调用上下文最大 token 数
	ContextMaxTokens = 16384

	// CharPerToken 中文字符与 token 的估算比率
	CharPerToken = 1.5

	// OutputTokenReserve 输出 token 预留量
	OutputTokenReserve = 4096

	// InputTokenReserve 输入 token 预留量
	InputTokenReserve = ContextMaxTokens - OutputTokenReserve
)

// ==== 降级阈值常量 ====

const (
	// KnowledgeMinSimilarity 知识记忆最低相似度阈值
	// 低于此值的记忆条目被视为与查询不相关
	KnowledgeMinSimilarity = 0.60

	// KnowledgeMinMatchCount 知识记忆最少匹配条数
	// 当知识库可用但记忆查询结果少于该数量时，返回"月华不知道"
	KnowledgeMinMatchCount = 2

	// MemoryUpdateSimilarityThreshold 记忆更新相似度阈值
	// 高于此值的旧记忆条目将被新内容替代
	MemoryUpdateSimilarityThreshold = 0.75
)

// ==== 报告验证常量 ====

const (
	// MinReportLength 报告最小字符数（低于此值视为无效）
	MinReportLength = 10

	// MaxReportLength 报告最大字符数（超出截断）
	MaxReportLength = 8000
)

// ==== Token 预算预设 ====

var (
	// BudgetRefine 查询推理完善预算
	BudgetRefine = TokenBudget{MaxInput: 2000, MaxOutput: 800}

	// BudgetEvaluate 策略评估预算
	BudgetEvaluate = TokenBudget{MaxInput: 8000, MaxOutput: 2000}

	// BudgetSearchEval 搜索内容评估预算
	BudgetSearchEval = TokenBudget{MaxInput: 12000, MaxOutput: 3000}

	// BudgetReport 报告生成预算
	BudgetReport = TokenBudget{MaxInput: 10000, MaxOutput: 4000}

	// BudgetMemoryUpdate 记忆更新预算
	BudgetMemoryUpdate = TokenBudget{MaxInput: 4000, MaxOutput: 800}

	// BudgetSearchCompress 搜索结果压缩预算
	BudgetSearchCompress = TokenBudget{MaxInput: 6000, MaxOutput: 1500}
)
