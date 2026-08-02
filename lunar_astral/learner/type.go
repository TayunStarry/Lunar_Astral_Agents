package learner

// ============================================================
// 配置与状态
// ============================================================

// LearnerConfig 学习者初始化配置
type LearnerConfig struct {
	BaseURL        string  // LLM API 地址
	APIKey         string  // API 密钥
	Model          string  // 模型名称
	MaxTokens      int     // 最大输出 token
	Temperature    float64 // 温度
	EmbeddingURL   string  // 嵌入服务地址
	EmbeddingKey   string  // 嵌入服务密钥
	EmbeddingModel string  // 嵌入模型名称
}

// LearnerState 学习者运行时状态
type LearnerState struct {
	Initialized bool
	SearchReady bool
	MemoryReady bool
	Config      LearnerConfig
}

// LearnerResult 学习者执行结果
type LearnerResult struct {
	Report        string   // 研究报告全文
	Sources       []string // 信息来源列表
	KnowledgeOps  int      // 知识记忆操作次数
	ExperienceOps int      // 经验记忆操作次数
	SearchRounds  int      // 深度搜索轮次
}

// ============================================================
// 工作流阶段
// ============================================================

// WorkflowPhase 工作流阶段
type WorkflowPhase int

const (
	PhaseRefine       WorkflowPhase = iota // a: AI 推理完善请求
	PhaseMemoryQuery                       // b: 查询记忆库
	PhaseWebSearch                         // d: 初步网络搜索
	PhaseEvaluate                          // e: AI 总结评估 + 决策
	PhaseDeepSearch                        // g/h: 深度搜索循环
	PhaseFinalize                          // i: 统一处理 + 记忆更新
)

// ============================================================
// 步骤 a: 查询推理完善
// ============================================================

// RefinedQuery AI 推理完善后的查询
type RefinedQuery struct {
	Original    string   `json:"original"`     // 原始请求
	Refined     string   `json:"refined"`      // 完善后的完整查询
	KeyPoints   []string `json:"key_points"`   // 关键要点
	SearchTerms []string `json:"search_terms"` // 建议搜索词
}

// ============================================================
// 步骤 e: 评估与决策
// ============================================================

// EvaluationResult AI 评估结果
type EvaluationResult struct {
	Sufficient      bool   `json:"sufficient"`        // 信息是否充足
	Summary         string `json:"summary,omitempty"`  // 阶段性摘要（充足时）
	NeedDeepSearch  bool   `json:"need_deep_search"`   // 是否需要深度搜索
	DeepSearchQuery string `json:"deep_search_query,omitempty"` // 深度搜索查询词
	Reasoning       string `json:"reasoning"`          // 评估理由
}

// ============================================================
// 步骤 g/h: 深度搜索
// ============================================================

// SearchRound 深度搜索单轮记录
type SearchRound struct {
	RoundNum   int    // 轮次编号
	Query      string // 搜索查询词
	Result     string // 搜索结果（压缩后）
	Evaluation string // AI 对结果的评估
	Sufficient bool   // 本轮信息是否充足
}

// SearchEvaluation AI 对搜索内容的评估
type SearchEvaluation struct {
	Sufficient         bool   `json:"sufficient"`           // 信息是否充足
	Summary            string `json:"summary,omitempty"`     // 相关信息摘要
	SupplementaryQuery string `json:"supplementary_query,omitempty"` // 补充搜索词
	Reasoning          string `json:"reasoning"`             // 评估理由
}

// ============================================================
// 工作流状态
// ============================================================

// WorkflowState 工作流运行时状态
type WorkflowState struct {
	OriginalQuery string              // 原始用户请求
	RefinedQuery  *RefinedQuery       // 完善后的查询
	KnowledgeMem  []MemoryMatch       // 知识记忆检索结果
	ExperienceMem []MemoryMatch       // 经验记忆检索结果
	SimpleSearch  []SearchItemPreview // 初步网络搜索摘要
	Evaluation    *EvaluationResult   // 评估与决策
	SearchRounds  []SearchRound       // 深度搜索轮次
	FinalReport   string              // 最终报告
	CurrentPhase  WorkflowPhase       // 当前阶段
}

// ============================================================
// 记忆相关类型
// ============================================================

// MemoryMatch 记忆匹配结果
type MemoryMatch struct {
	ID         string  // 文档 ID
	Content    string  // 内容
	Similarity float32 // 相似度
	Table      string  // 来源表名（knowledge / experience）
}

// MemoryUpdate 记忆更新指令
type MemoryUpdate struct {
	OldID      string // 原条目 ID（空表示新增）
	NewContent string // 新内容
	Reason     string // 更新原因
}

// MemoryAddRequest 记忆添加请求
type MemoryAddRequest struct {
	Table   string // 目标表名
	Content string // 内容
}

// MemoryBatchResult 记忆批量操作结果
type MemoryBatchResult struct {
	KnowledgeAdded  int // 知识记忆新增条数
	KnowledgeUpdated int // 知识记忆更新条数
	ExperienceAdded int // 经验记忆新增条数
}

// ============================================================
// LLM 相关类型
// ============================================================

// LLMMessage LLM 消息
type LLMMessage struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

// ToolCall 工具调用
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

// FunctionCall 函数调用
type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ToolDefinition 工具定义
type ToolDefinition struct {
	Type     string      `json:"type"`
	Function FunctionDef `json:"function"`
}

// FunctionDef 函数定义
type FunctionDef struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

// LLMResponse LLM 响应
type LLMResponse struct {
	Content      string     `json:"content,omitempty"`
	ToolCalls    []ToolCall `json:"tool_calls,omitempty"`
	FinishReason string     `json:"finish_reason,omitempty"`
}

// ============================================================
// Token 预算
// ============================================================

// TokenBudget 单次 LLM 调用的 token 预算
type TokenBudget struct {
	MaxInput  int // 输入 token 上限
	MaxOutput int // 输出 token 上限
}

// ============================================================
// LLM 请求/响应结构体
// ============================================================

// chatCompletionRequest OpenAI v1 Chat Completion 请求
type chatCompletionRequest struct {
	Model       string           `json:"model"`
	Messages    []LLMMessage     `json:"messages"`
	MaxTokens   int              `json:"max_tokens,omitempty"`
	Temperature float64          `json:"temperature,omitempty"`
	Tools       []ToolDefinition `json:"tools,omitempty"`
	ToolChoice  string           `json:"tool_choice,omitempty"`
}

// chatCompletionResponse OpenAI v1 Chat Completion 响应
type chatCompletionResponse struct {
	ID      string `json:"id"`
	Choices []struct {
		Message struct {
			Role      string     `json:"role"`
			Content   string     `json:"content,omitempty"`
			ToolCalls []ToolCall `json:"tool_calls,omitempty"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

// chatCompletionError OpenAI v1 错误响应
type chatCompletionError struct {
	Error struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	} `json:"error"`
}

// ============================================================
// TS 层传入的消息结构
// ============================================================

// PostMessage TS 层消息结构
type PostMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

// TextContent 文本内容
type TextContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// ============================================================
// 搜索相关类型
// ============================================================

// SearchItemPreview 初步搜索摘要条目
type SearchItemPreview struct {
	Title   string
	URL     string
	Snippet string
}

// AgentSearchMode 智能体搜索模式
type AgentSearchMode string

const (
	AgentModeSimple  AgentSearchMode = "simple"  // 轻量摘要
	AgentModeWebpage AgentSearchMode = "webpage" // 网页搜索
	AgentModeDepth   AgentSearchMode = "depth"   // 深度研究
)

// ============================================================
// 调试导出
// ============================================================

// DebugContextDump 调试用上下文快照
type DebugContextDump struct {
	Timestamp      string              `json:"timestamp"`
	DialogueJSON   string              `json:"dialogue_json"`
	UnreadJSON     string              `json:"unread_json"`
	FullContext    string              `json:"full_context"`
	RefinedQuery   *RefinedQuery       `json:"refined_query,omitempty"`
	KnowledgeMem   []MemoryMatch       `json:"knowledge_memory"`
	ExperienceMem  []MemoryMatch       `json:"experience_memory"`
	SimpleSearch   []SearchItemPreview `json:"simple_search"`
	Evaluation     *EvaluationResult   `json:"evaluation,omitempty"`
	SearchRounds   []SearchRound       `json:"search_rounds"`
	FinalReport    string              `json:"final_report,omitempty"`
	MemoryReady    bool                `json:"memory_ready"`
	SearchReady    bool                `json:"search_ready"`
}