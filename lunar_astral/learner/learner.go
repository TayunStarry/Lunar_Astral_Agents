package agent

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"logger"

	"github.com/dop251/goja"
)

// Learner 学习者智能体
type Learner struct {
	config LearnerConfig
	llm    *LLMClient
	search *SearchManager
	memory *MemoryManager
	debate *DebateSystem
}

// NewLearner 创建学习者实例
func NewLearner(cfg LearnerConfig) *Learner {
	return &Learner{config: cfg}
}

// Init 初始化学习者子系统
func (l *Learner) Init() error {
	// 初始化 LLM 客户端
	l.llm = NewLLMClient(l.config)

	// 初始化搜索管理器
	l.search = NewSearchManager()
	if err := l.search.Init(l.config); err != nil {
		logger.Warn("Learner", "搜索子系统初始化失败: %v（将仅使用记忆库）", err)
	}

	// 初始化记忆管理器
	l.memory = NewMemoryManager()
	if err := l.memory.Init(l.config); err != nil {
		logger.Warn("Learner", "记忆子系统初始化失败: %v", err)
	}

	// 初始化辩论系统
	l.debate = NewDebateSystem(l.llm, l.search, l.memory)

	logger.Info("Learner", "学习者智能体初始化完成 (搜索=%v, 记忆=%v)",
		l.search.IsAvailable(), l.memory.IsAvailable())

	return nil
}

// Execute 执行学习者研究流程
// 参数: dialogueJSON (对话历史JSON), unreadJSON (未读消息JSON)
// 返回: 报告文本 或 错误
func (l *Learner) Execute(dialogueJSON string, unreadJSON string) (string, error) {
	startTime := time.Now()

	// 构建完整上下文（不再只提取单条查询，而是传递所有相关消息）
	fullContext := l.buildFullContext(dialogueJSON, unreadJSON)
	if fullContext == "" {
		return "", fmt.Errorf("未能从消息中提取有效上下文")
	}

	logger.Info("Learner", "开始执行研究，上下文长度: %d 字符", len([]rune(fullContext)))

	// 检查可用性
	if !l.search.IsAvailable() && !l.memory.IsAvailable() {
		return "未能检索到任何有效记忆", nil
	}

	// 执行辩论研究（传入完整上下文，由 LLM 分析阶段提取真正的搜索意图）
	result, err := l.debate.Execute(fullContext)
	if err != nil {
		logger.Error("Learner", "研究执行失败: %v", err)

		// 错误降级：如果搜索不可用，只用记忆库
		if !l.search.IsAvailable() && l.memory.IsAvailable() {
			matches, memErr := l.memory.Query(fullContext, MemoryQueryTopK)
			if memErr == nil && len(matches) > 0 {
				return fmt.Sprintf("[研究报告]\n\n## 研究主题\n%s\n\n## 研究结论\n基于记忆库的有限检索结果，网络搜索不可用。\n\n## 支持证据\n%s\n\n## 疑点与未解决问题\n网络搜索不可用，无法交叉验证记忆中的信息。",
					fullContext, FormatMemoryResults(matches)), nil
			}
		}

		return "未能检索到任何有效记忆", nil
	}

	elapsed := time.Since(startTime)
	logger.Info("Learner", "研究完成: 耗时=%v, 搜索轮次=%d, 辩论轮次=%d, 记忆操作=%d",
		elapsed, result.SearchRounds, result.DebateRounds, result.MemoryOps)

	return result.Report, nil
}

// buildFullContext 从 TS 层传入的消息构建完整上下文
// 策略：将所有用户消息拼接为完整上下文，交由 LLM 分析阶段提取真正的搜索意图
// 关键：未读消息在前（包含最新触发搜索的消息），对话历史在后（提供背景）
func (l *Learner) buildFullContext(dialogueJSON string, unreadJSON string) string {
	var parts []string

	// 第一步：提取未读消息（优先级最高，包含触发搜索的消息）
	if unreadJSON != "" {
		var unreadMessages []PostMessage
		if err := json.Unmarshal([]byte(unreadJSON), &unreadMessages); err == nil {
			for _, msg := range unreadMessages {
				text := extractTextFromPostMessage(msg)
				if strings.TrimSpace(text) != "" {
					parts = append(parts, text)
				}
			}
		}
	}

	// 第二步：提取对话历史中的用户消息（提供背景上下文）
	if dialogueJSON != "" {
		var dialogueMessages []PostMessage
		if err := json.Unmarshal([]byte(dialogueJSON), &dialogueMessages); err == nil {
			// 只取最近的几条
			start := 0
			if len(dialogueMessages) > DialogueHistoryLimit {
				start = len(dialogueMessages) - DialogueHistoryLimit
			}
			for _, msg := range dialogueMessages[start:] {
				if msg.Role == "user" {
					text := extractTextFromPostMessage(msg)
					if strings.TrimSpace(text) != "" {
						parts = append(parts, text)
					}
				}
			}
		}
	}

	if len(parts) == 0 {
		return ""
	}

	return strings.Join(parts, "\n")
}

// extractTextFromPostMessage 从 PostMessage 中提取文本内容
func extractTextFromPostMessage(msg PostMessage) string {
	switch v := msg.Content.(type) {
	case string:
		return v
	case []interface{}:
		var parts []string
		for _, item := range v {
			if m, ok := item.(map[string]interface{}); ok {
				if t, ok := m["type"].(string); ok && t == "text" {
					if text, ok := m["text"].(string); ok {
						parts = append(parts, text)
					}
				}
			}
		}
		return strings.Join(parts, " ")
	default:
		return fmt.Sprintf("%v", msg.Content)
	}
}

// ============================================================
// Goja 绑定函数 — 通过 adapters 包的 Runtime 方法模式注册
// ============================================================

// BindLearnerToRuntime 将学习者 Goja 绑定注册到指定运行时
// 由 adapters/create.go 调用，传入 goja.Runtime 实例
func BindLearnerToRuntime(vm *goja.Runtime) {
	vm.Set("learnerInit", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 8 {
			return vm.ToValue([]any{false, fmt.Errorf("learnerInit 参数不足，需要 baseURL, apiKey, model, maxTokens, temperature, embeddingURL, embeddingKey, embeddingModel")})
		}

		baseURL, _ := call.Argument(0).Export().(string)
		apiKey, _ := call.Argument(1).Export().(string)
		model, _ := call.Argument(2).Export().(string)
		maxTokens := int(call.Argument(3).ToInteger())
		temperature := call.Argument(4).ToFloat()
		embeddingURL, _ := call.Argument(5).Export().(string)
		embeddingKey, _ := call.Argument(6).Export().(string)
		embeddingModel, _ := call.Argument(7).Export().(string)

		cfg := LearnerConfig{
			BaseURL:        baseURL,
			APIKey:         apiKey,
			Model:          model,
			MaxTokens:      maxTokens,
			Temperature:    temperature,
			EmbeddingURL:   embeddingURL,
			EmbeddingKey:   embeddingKey,
			EmbeddingModel: embeddingModel,
		}

		learnerMutex.Lock()
		defer learnerMutex.Unlock()

		// 如果已初始化，先关闭旧的
		if learnerInstance != nil {
			logger.Info("Learner", "学习者实例已存在，重新初始化")
		}

		learnerInstance = NewLearner(cfg)
		if err := learnerInstance.Init(); err != nil {
			logger.Error("Learner", "学习者初始化失败: %v", err)
			return vm.ToValue([]any{false, err})
		}

		return vm.ToValue([]any{true, nil})
	})

	vm.Set("learnerExecute", func(call goja.FunctionCall) goja.Value {
		learnerMutex.RLock()
		inst := learnerInstance
		learnerMutex.RUnlock()

		if inst == nil {
			return vm.ToValue([]any{"", fmt.Errorf("学习者未初始化，请先调用 learnerInit")})
		}

		dialogueJSON := ""
		unreadJSON := ""

		if len(call.Arguments) >= 1 {
			dialogueJSON, _ = call.Argument(0).Export().(string)
		}
		if len(call.Arguments) >= 2 {
			unreadJSON, _ = call.Argument(1).Export().(string)
		}

		report, err := inst.Execute(dialogueJSON, unreadJSON)
		if err != nil {
			logger.Error("Learner", "学习者执行失败: %v", err)
			return vm.ToValue([]any{"", err})
		}

		return vm.ToValue([]any{report, nil})
	})

	vm.Set("learnerIsReady", func(call goja.FunctionCall) goja.Value {
		learnerMutex.RLock()
		defer learnerMutex.RUnlock()
		return vm.ToValue(learnerInstance != nil)
	})
}
