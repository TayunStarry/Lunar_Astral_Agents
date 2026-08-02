package learner

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"logger"

	"github.com/dop251/goja"
)

// Learner 学习者智能体
// 采用双记忆架构（知识记忆 + 经验记忆），9 步工作流
type Learner struct {
	config  LearnerConfig
	llm     *LLMClient
	search  *SearchManager
	memory  *MemoryManager
	prompts *PromptTemplates
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

	// 初始化记忆管理器（知识 + 经验双表）
	l.memory = NewMemoryManager()
	if err := l.memory.Init(l.config); err != nil {
		logger.Warn("Learner", "记忆子系统初始化失败: %v", err)
	}

	// 加载 Prompt 模板
	l.prompts = loadPrompts()

	logger.Info("Learner", "学习者智能体初始化完成 (搜索=%v, 记忆=%v)",
		l.search.IsAvailable(), l.memory.IsAvailable())

	return nil
}

// Execute 执行学习者研究流程
// 参数: dialogueJSON (对话历史JSON), unreadJSON (未读消息JSON), hint (意图提示，保留兼容)
// 返回: 报告文本 或 错误
func (l *Learner) Execute(dialogueJSON string, unreadJSON string, hint string) (string, error) {
	startTime := time.Now()

	// 构建完整上下文
	fullContext := l.buildFullContext(dialogueJSON, unreadJSON)
	if fullContext == "" {
		return "月华不知道呢，请提供更具体的问题吧~", nil
	}

	logger.Info("Learner", "开始执行研究，上下文长度: %d 字符", len([]rune(fullContext)))

	// 检查知识库是否可用
	if !l.memory.IsAvailable() {
		logger.Warn("Learner", "知识库不可用")
		return "月华不知道呢，知识库暂时无法访问~", nil
	}

	// 创建并运行工作流
	runner := NewWorkflowRunner(l.llm, l.search, l.memory, l.prompts)
	report, state, err := runner.Run(fullContext)
	if err != nil {
		logger.Error("Learner", "研究执行失败: %v", err)

		// 降级策略：知识库可用但其他错误 → 基于相似度决定
		if state != nil && l.memory.IsAvailable() {
			knowledgeMem := state.KnowledgeMem
			if l.memory.HasKnowledgeMatches(knowledgeMem) {
				logger.Info("Learner", "降级：知识库有足够匹配，返回知识库数据")
				fallbackReport := buildKnowledgeFallbackReport(fullContext, knowledgeMem)
				elapsed := time.Since(startTime)
				logger.Info("Learner", "研究完成(降级-知识库): 耗时=%v", elapsed)
				return fallbackReport, nil
			}
		}

		return "月华不知道呢，处理过程中遇到了问题~", nil
	}

	// TS 层验证：检查最小长度和乱码
	if !isValidReport(report) {
		logger.Warn("Learner", "报告验证失败，长度=%d", len([]rune(report)))

		// 降级：尝试使用知识库数据
		if state != nil && l.memory.HasKnowledgeMatches(state.KnowledgeMem) {
			report = buildKnowledgeFallbackReport(fullContext, state.KnowledgeMem)
		} else {
			return "月华不知道呢，生成的内容似乎有问题~", nil
		}
	}

	elapsed := time.Since(startTime)
	logger.Info("Learner", "研究完成: 耗时=%v", elapsed)

	return report, nil
}

// buildFullContext 从 TS 层传入的消息构建完整上下文
// 策略：未读消息在前（包含最新触发搜索的消息），对话历史在后（提供背景）
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

// buildKnowledgeFallbackReport 基于知识库构建降级报告
func buildKnowledgeFallbackReport(query string, matches []MemoryMatch) string {
	var parts []string
	parts = append(parts, fmt.Sprintf("[研究报告]\n\n## 研究主题\n%s\n\n## 研究结论\n基于记忆库的检索结果，以下是相关信息：\n", query))

	parts = append(parts, FormatMemoryResults(matches))

	parts = append(parts, "\n## 疑点与未解决问题\n网络搜索未能完成，信息可能不完整，建议稍后重试。")

	return strings.Join(parts, "\n")
}

// DumpContext 导出学习者运行时上下文到 JSON 文件（覆写模式）
// 用于调试排查问题
func (l *Learner) DumpContext(dialogueJSON string, unreadJSON string, outputPath string) (string, error) {
	dump := &DebugContextDump{
		Timestamp:    time.Now().Format("2006-01-02 15:04:05"),
		DialogueJSON: dialogueJSON,
		UnreadJSON:   unreadJSON,
		MemoryReady:  l.memory.IsAvailable(),
		SearchReady:  l.search.IsAvailable(),
	}

	// 构建完整上下文
	dump.FullContext = l.buildFullContext(dialogueJSON, unreadJSON)

	// 执行工作流
	if l.memory.IsAvailable() {
		runner := NewWorkflowRunner(l.llm, l.search, l.memory, l.prompts)
		_, state, err := runner.Run(dump.FullContext)
		if err == nil && state != nil {
			dump.RefinedQuery = state.RefinedQuery
			dump.KnowledgeMem = state.KnowledgeMem
			dump.ExperienceMem = state.ExperienceMem
			dump.SimpleSearch = state.SimpleSearch
			dump.Evaluation = state.Evaluation
			dump.SearchRounds = state.SearchRounds
			dump.FinalReport = state.FinalReport
		}
	}

	// 序列化为 JSON
	data, err := json.MarshalIndent(dump, "", "  ")
	if err != nil {
		return "", fmt.Errorf("序列化上下文快照失败: %w", err)
	}

	// 确保目录存在
	dir := filepath.Dir(outputPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("创建目录失败: %w", err)
	}

	// 覆写写入文件
	if err := os.WriteFile(outputPath, data, 0644); err != nil {
		return "", fmt.Errorf("写入文件失败: %w", err)
	}

	logger.Info("Learner", "上下文快照已导出: %s (%d 字节)", outputPath, len(data))
	return outputPath, nil
}

// ============================================================
// Goja 绑定函数
// ============================================================

// BindLearnerToRuntime 将学习者 Goja 绑定注册到指定运行时
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
			return vm.ToValue([]any{"月华不知道呢，学习者还没准备好~", fmt.Errorf("学习者未初始化，请先调用 learnerInit")})
		}

		dialogueJSON := ""
		unreadJSON := ""

		if len(call.Arguments) >= 1 {
			dialogueJSON, _ = call.Argument(0).Export().(string)
		}
		if len(call.Arguments) >= 2 {
			unreadJSON, _ = call.Argument(1).Export().(string)
		}

		// 第三个参数 hint 保留兼容性，但新系统不依赖它
		hint := ""
		if len(call.Arguments) >= 3 {
			hint, _ = call.Argument(2).Export().(string)
		}

		report, err := inst.Execute(dialogueJSON, unreadJSON, hint)
		if err != nil {
			logger.Error("Learner", "学习者执行失败: %v", err)
			return vm.ToValue([]any{"月华不知道呢，处理过程中遇到了问题~", err})
		}

		return vm.ToValue([]any{report, nil})
	})

	vm.Set("learnerIsReady", func(call goja.FunctionCall) goja.Value {
		learnerMutex.RLock()
		defer learnerMutex.RUnlock()
		return vm.ToValue(learnerInstance != nil)
	})

	// learnerDumpContext 导出学习者上下文到文件（覆写模式）
	vm.Set("learnerDumpContext", func(call goja.FunctionCall) goja.Value {
		learnerMutex.RLock()
		inst := learnerInstance
		learnerMutex.RUnlock()

		if inst == nil {
			return vm.ToValue([]any{"", fmt.Errorf("学习者未初始化，请先调用 learnerInit")})
		}

		dialogueJSON := ""
		unreadJSON := ""
		outputPath := "learner_debug_context.json"

		if len(call.Arguments) >= 1 {
			dialogueJSON, _ = call.Argument(0).Export().(string)
		}
		if len(call.Arguments) >= 2 {
			unreadJSON, _ = call.Argument(1).Export().(string)
		}
		if len(call.Arguments) >= 3 {
			if path, ok := call.Argument(2).Export().(string); ok && path != "" {
				outputPath = path
			}
		}

		path, err := inst.DumpContext(dialogueJSON, unreadJSON, outputPath)
		if err != nil {
			logger.Error("Learner", "上下文导出失败: %v", err)
			return vm.ToValue([]any{"", err})
		}

		return vm.ToValue([]any{path, nil})
	})
}