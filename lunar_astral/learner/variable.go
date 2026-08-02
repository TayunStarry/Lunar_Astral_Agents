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
	SearchResultMaxChars = 1500

	// DeepSearchResultMaxChars 深度搜索结果最大字符数
	DeepSearchResultMaxChars = 3000

	// ContextMaxTokens 单次 LLM 调用上下文最大 token 数
	ContextMaxTokens = 16384

	// CharPerToken 中文字符与 token 的估算比率
	CharPerToken = 1.5

	// OutputTokenReserve 输出 token 预留量
	OutputTokenReserve = 4096

	// InputTokenReserve 输入 token 预留量
	InputTokenReserve = ContextMaxTokens - OutputTokenReserve

	// DialogueHistoryLimit 对话历史读取条数
	DialogueHistoryLimit = 15

	// UnreadCheckCount 未读消息检查条数
	UnreadCheckCount = 10
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
	BudgetSearchEval = TokenBudget{MaxInput: 6000, MaxOutput: 1500}

	// BudgetReport 报告生成预算
	BudgetReport = TokenBudget{MaxInput: 10000, MaxOutput: 4000}

	// BudgetMemoryUpdate 记忆更新预算
	BudgetMemoryUpdate = TokenBudget{MaxInput: 4000, MaxOutput: 800}

	// BudgetSearchCompress 搜索结果压缩预算
	BudgetSearchCompress = TokenBudget{MaxInput: 6000, MaxOutput: 1500}
)

// ==== 默认 Prompt 模板 ====
// 当嵌入式文件系统加载失败时使用

const defaultRefinePrompt = `你是学习者的查询推理引擎。请将用户的原始请求完善为结构化、信息充分的查询语句。

原始请求可能简短、模糊或包含隐含意图。你需要：
1. 补全隐含的上下文和意图
2. 提取关键要点
3. 生成适合网络搜索的关键词

输出 JSON 格式（只输出 JSON，不要其他内容）：
{
  "refined": "完善后的完整查询语句",
  "key_points": ["关键要点1", "关键要点2", "关键要点3"],
  "search_terms": ["搜索词1", "搜索词2", "搜索词3"]
}

要求：refined 字段应是一段完整的、信息充分的查询描述；key_points 列出用户关心的核心问题；search_terms 列出适合搜索引擎的关键词组合。`

const defaultEvaluatePrompt = `你是学习者的策略评估中枢。你需要基于已收集的信息（知识记忆、经验记忆、网络搜索摘要），判断信息是否足以回答用户问题，并决定是否需要深度搜索。

评估标准：
1. 信息是否覆盖用户问题的核心要点？
2. 信息来源是否可靠、时效性是否足够？
3. 是否存在明显的知识盲区？

输出 JSON 格式（只输出 JSON，不要其他内容）：
{
  "sufficient": true或false,
  "summary": "sufficient=true时的阶段性摘要，以[研究报告]开头",
  "need_deep_search": true或false,
  "deep_search_query": "sufficient=false时的深度搜索查询词",
  "reasoning": "评估理由简述"
}

约束：sufficient=true时summary必须包含实质性内容；sufficient=false时deep_search_query必填。`

const defaultSearchEvalPrompt = `你是学习者的搜索内容评估器。请评估新获取的深度搜索内容是否足以回答用户问题。

输出 JSON 格式（只输出 JSON，不要其他内容）：
{
  "sufficient": true或false,
  "summary": "相关信息摘要",
  "supplementary_query": "补充搜索词（sufficient=false时必填）",
  "reasoning": "评估理由简述"
}

约束：sufficient=true时summary必须包含从搜索内容中提取的关键信息；sufficient=false时supplementary_query必填。`

const defaultMemoryPrompt = `你是学习者的记忆管理助手。基于本次研究结果，请生成记忆更新指令。

需要处理两种记忆：
1. 知识记忆：从网络搜索中获取的新事实、数据、信息
2. 经验记忆：本次请求的处理策略，包括采用了什么搜索方式、效果如何

输出 JSON 格式（只输出 JSON，不要其他内容）：
{
  "knowledge_items": [{"content": "知识摘要内容"}],
  "experience_item": "本次请求处理策略描述，包括：查询类型、采用的搜索策略、搜索轮次、信息充足度评估"
}

约束：knowledge_items 可为空数组；experience_item 必须包含策略描述。`

const defaultReportPrompt = `你是学习者的报告生成系统。请基于所有收集到的信息，生成一份完整的研究报告。

输出格式：以 [研究报告] 开头，包含以下结构：
- 研究主题
- 研究结论
- 支持证据（标注来源：网络/记忆）
- 疑点与未解决问题（如有）
- 研究方法说明

所有结论必须有证据支撑，标注来源。`

const defaultInsufficientReport = `[研究报告]

## 研究主题
%s

## 研究结论
月华已经尽力搜索了相关信息，但经过多轮搜索后，仍未能找到足够的信息来全面回答您的问题。

## 已获取的部分信息
%s

## 疑点与未解决问题
经过%d轮深度搜索，以下问题仍需进一步信息：
- 部分关键信息在网络公开资源中覆盖不足
- 建议尝试更具体的查询方向或等待相关信息更新

## 研究方法说明
本研究采用了查询推理完善 → 记忆库检索 → 网络搜索 → 多轮深度搜索的研究流程。`