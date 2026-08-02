package learner

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

// ==== 意图校准 ====

// MemoryStrongMatchThreshold 记忆强匹配相似度阈值
const MemoryStrongMatchThreshold = 0.75

// MemoryStrongMatchMinCount 判定记忆偏向的强匹配最少条目数
const MemoryStrongMatchMinCount = 3

// IntentMemoryTopK 意图→记忆查询 topK 映射
var IntentMemoryTopK = map[IntentHint]int{
	IntentMemory:   25,
	IntentSearch:   5,
	IntentBalanced: 10,
}

// ==== 策略评估预算 ====

// BudgetStrategyEval 阶段2策略评估预算
var BudgetStrategyEval = TokenBudget{MaxInput: 8000, MaxOutput: 2000}

// BudgetWebpageReport Webpage 分支报告预算
var BudgetWebpageReport = TokenBudget{MaxInput: 10000, MaxOutput: 3000}

// ==== 辩论系统默认 Prompt 模板 ====
// 当嵌入式文件系统加载失败时使用

const defaultDebatePrompt = `你是学习者的辩论系统。四角色交替辩论：
- 网络派：基于搜索结果
- 记忆派：基于记忆数据
- 质疑者：挑战双方论点
- 裁决者：综合评估，判断信息充分度

每个角色发言控制在300字以内，基于证据，不编造。`

const defaultReportPrompt = `你是学习者的报告生成系统。输出 [研究报告] 格式：

[研究报告]

## 研究主题
## 研究结论
## 支持证据
## 疑点与未解决问题
## 研究方法说明

所有结论必须有证据支撑，标注来源（网络/记忆/辩论共识）。`

const defaultMemoryPrompt = `你是学习者的记忆管理助手。基于研究结果判断是否需要更新记忆库中的条目。

输出 JSON 数组格式（不需要更新则输出 []）：
[{"old_id": "旧条目ID", "new_content": "完善后的完整内容", "reason": "更新原因"}]

只更新确实需要修正或补充的条目。`

// defaultStrategyPrompt 策略评估的内置默认 prompt
const defaultStrategyPrompt = `你是学习者的"策略评估中枢"。你需要基于预探测结果（轻量搜索摘要 + 记忆库匹配），判断信息是否足以直接回答用户问题，并决定下一步研究策略。

输出 JSON 格式：
{
  "sufficient": true或false,
  "direct_answer": "sufficient=true时的直接回答（以[研究报告]开头）",
  "intent": "memory或search或balanced",
  "search_strategy": "webpage或depth（sufficient=false时必填）",
  "multi_angle_search": true或false,
  "debate_rounds": 2到5的整数（仅depth策略时）,
  "sub_questions": [{"question": "子问题", "search_query": "搜索关键词", "dimension": "维度"}],
  "memory_top_k": 5到25的整数
}

约束：sufficient=true时direct_answer以[研究报告]开头；sufficient=false时search_strategy必填；debate_rounds不超过5；sub_questions不超过4个；只输出JSON。`
