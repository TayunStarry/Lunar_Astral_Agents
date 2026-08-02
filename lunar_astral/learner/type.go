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
	Report       string   // 研究报告全文
	Sources      []string // 信息来源列表
	MemoryOps    int      // 记忆操作次数
	SearchRounds int      // 搜索轮次
	DebateRounds int      // 辩论轮次
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

// ToolDefinition 工具定义（传给 LLM 的 JSON 格式）
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
	Content     string     `json:"content,omitempty"`
	ToolCalls   []ToolCall `json:"tool_calls,omitempty"`
	FinishReason string    `json:"finish_reason,omitempty"`
}

// ============================================================
// 辩论系统类型
// ============================================================

// DebatePhase 辩论阶段
type DebatePhase int

const (
	PhaseAnalyze   DebatePhase = iota // 阶段1: 问题分析
	PhaseSearch                       // 阶段2: 并行搜索
	PhaseDebate                       // 阶段3: 辩论循环
	PhaseSynthesize                   // 阶段4: 综合报告
)

// SubQuestion 子问题
type SubQuestion struct {
	Question     string // 子问题文本
	SearchQuery  string // 对应的搜索查询
	SearchResult string // 搜索结果（压缩后）
	Source       string // 来源标记
}

// DebateRole 辩论角色
type DebateRole string

const (
	RoleModerator DebateRole = "moderator" // 主持人
	RoleNetPro    DebateRole = "net_pro"   // 网络派
	RoleMemPro    DebateRole = "mem_pro"   // 记忆派
	RoleSkeptic   DebateRole = "skeptic"   // 质疑者
	RoleJudge     DebateRole = "judge"     // 裁决者
)

// DebateRound 辩论轮次记录
type DebateRound struct {
	RoundNum     int    // 轮次编号
	NetProArg    string // 网络派论点
	MemProArg    string // 记忆派论点
	SkepticArg   string // 质疑者论点
	JudgeVerdict string // 裁决者判断
	IsConverged  bool   // 是否收敛
	Summary      string // 本轮摘要（压缩后，供下一轮使用）
}

// DebateState 辩论状态
type DebateState struct {
	OriginalQuery string        // 原始查询
	SubQuestions  []SubQuestion // 子问题列表
	MemoryResults []MemoryMatch // 记忆检索结果
	Rounds        []DebateRound // 辩论轮次
	CurrentPhase  DebatePhase   // 当前阶段
	Converged     bool          // 是否已收敛
	MaxRounds     int           // 最大辩论轮次
}

// ============================================================
// 记忆相关类型
// ============================================================

// MemoryMatch 记忆匹配结果
type MemoryMatch struct {
	ID         string  // 文档 ID
	Content    string  // 内容
	Similarity float32 // 相似度
	Superseded bool    // 是否已被新条目替代（标记删除用）
}

// MemoryUpdate 记忆更新指令
type MemoryUpdate struct {
	OldID      string // 原条目 ID
	NewContent string // 完善后的内容
	Reason     string // 更新原因
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
// 报告类型
// ============================================================

// ReportOutline 报告提纲
type ReportOutline struct {
	Topic      string   // 研究主题
	Conclusion string   // 核心结论方向
	Sections   []string // 提纲要点列表
	Doubts     []string // 待解决的疑点
}

// ReportSection 报告段落
type ReportSection struct {
	Title   string // 段落标题
	Content string // 段落内容
	Source  string // 信息来源
}

// ============================================================
// LLM 请求/响应结构体
// ============================================================

// chatCompletionRequest OpenAI v1 Chat Completion 请求
type chatCompletionRequest struct {
	Model       string        `json:"model"`
	Messages    []LLMMessage  `json:"messages"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
	Temperature float64       `json:"temperature,omitempty"`
	Tools       []ToolDefinition `json:"tools,omitempty"`
	ToolChoice  string        `json:"tool_choice,omitempty"`
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

// PostMessage TS 层消息结构（与 adapters/type.go 保持兼容）
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
// 意图与策略
// ============================================================

// IntentHint TS 层传入的意图提示
type IntentHint string

const (
	IntentMemory    IntentHint = "memory"    // 记忆偏向
	IntentSearch    IntentHint = "search"    // 搜索偏向
	IntentBalanced  IntentHint = "balanced"  // 均衡
	IntentAmbiguous IntentHint = "ambiguous" // 模糊，需预查记忆判定
)

// AgentSearchMode 智能体搜索模式（与 websearch.SearchMode 对齐）
type AgentSearchMode string

const (
	AgentModeSimple  AgentSearchMode = "simple"  // 轻量摘要
	AgentModeWebpage AgentSearchMode = "webpage" // 网页搜索
	AgentModeDepth   AgentSearchMode = "depth"   // 深度研究
)

// StrategyPlan LLM 策略评估输出（阶段2）
type StrategyPlan struct {
	Sufficient       bool            `json:"sufficient"`
	DirectAnswer     string          `json:"direct_answer,omitempty"`
	Intent           IntentHint      `json:"intent"`
	SearchStrategy   AgentSearchMode `json:"search_strategy,omitempty"`
	MultiAngleSearch bool            `json:"multi_angle_search"`
	DebateRounds     int             `json:"debate_rounds"`
	SubQuestions     []SubQuestion   `json:"sub_questions,omitempty"`
	MemoryTopK       int             `json:"memory_top_k"`
}

// ProbeResult 预探测结果（阶段1）
type ProbeResult struct {
	SearchItems   []SearchItemPreview // Simple 搜索结果
	MemoryMatches []MemoryMatch       // 记忆查询结果
}

// SearchItemPreview Simple 搜索结果预览条目
type SearchItemPreview struct {
	Title   string
	URL     string
	Snippet string
}

// ============================================================
// 调试导出
// ============================================================

// DebugContextDump 调试用上下文快照（导出到文件供排查问题）
type DebugContextDump struct {
	Timestamp       string              `json:"timestamp"`
	IntentHint      string              `json:"intent_hint"`
	DialogueJSON    string              `json:"dialogue_json"`
	UnreadJSON      string              `json:"unread_json"`
	FullContext     string              `json:"full_context"`
	ProbeSearch     []SearchItemPreview `json:"probe_search_items"`
	ProbeMemory     []MemoryMatch       `json:"probe_memory_matches"`
	StrategyPlan    *StrategyPlan       `json:"strategy_plan,omitempty"`
	DebateState     *DebateState        `json:"debate_state,omitempty"`
	MemoryAvailable bool                `json:"memory_available"`
	SearchAvailable bool                `json:"search_available"`
}
