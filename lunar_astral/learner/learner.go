package learner

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
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
// 参数: dialogueJSON (对话历史JSON), unreadJSON (未读消息JSON), hint (意图提示)
// 返回: 报告文本 或 错误
func (l *Learner) Execute(dialogueJSON string, unreadJSON string, hint IntentHint) (string, error) {
	startTime := time.Now()

	// 构建完整上下文
	fullContext := l.buildFullContext(dialogueJSON, unreadJSON)
	if fullContext == "" {
		return "", fmt.Errorf("未能从消息中提取有效上下文")
	}

	// 阶段0: 意图校准
	hint = l.calibrateIntent(fullContext, hint)
	logger.Info("Learner", "开始执行研究，意图: %s，上下文长度: %d 字符", hint, len([]rune(fullContext)))

	// 检查可用性
	if !l.search.IsAvailable() && !l.memory.IsAvailable() {
		return "未能检索到任何有效记忆", nil
	}

	// 阶段1: 预探测
	probe := l.probePhase(fullContext, hint)

	// 阶段2: 策略评估
	plan, err := l.evaluateStrategy(fullContext, hint, probe)
	if err != nil {
		logger.Error("Learner", "策略评估失败: %v，降级为深度研究", err)
		plan = &StrategyPlan{
			Sufficient:     false,
			Intent:         hint,
			SearchStrategy: AgentModeDepth,
			DebateRounds:   MaxDebateRounds,
			MemoryTopK:     MemoryQueryTopK,
		}
	}

	// sufficient=true: 直接返回
	if plan.Sufficient {
		logger.Info("Learner", "预探测结果充分，直接返回")
		elapsed := time.Since(startTime)
		logger.Info("Learner", "研究完成(直接回答): 耗时=%v", elapsed)
		return plan.DirectAnswer, nil
	}

	// 阶段3: 按策略执行
	var report string

	switch plan.SearchStrategy {
	case AgentModeWebpage:
		// Webpage 分支
		logger.Info("Learner", "=== Webpage 分支 ===")
		result, err := l.search.WebpageSearch(fullContext)
		if err != nil {
			logger.Error("Learner", "网页搜索失败: %v", err)
			return "网页搜索失败，未能检索到有效信息", nil
		}
		report = "[研究报告]\n\n" + result
		// 注入预探测的记忆结果到 debate.state，供 updateMemory 比对旧记忆
		l.debate.state = &DebateState{
			OriginalQuery: fullContext,
			MemoryResults: probe.MemoryMatches,
		}
		// 更新记忆
		l.debate.updateMemory(report)

	default: // AgentModeDepth 或降级
		// Depth 分支（辩论）
		logger.Info("Learner", "=== Depth 辩论分支 ===")
		result, err := l.debate.Execute(fullContext, *plan)
		if err != nil {
			logger.Error("Learner", "研究执行失败: %v", err)
			// 错误降级：如果搜索不可用，只用记忆库
			if l.memory.IsAvailable() {
				matches, memErr := l.memory.Query(fullContext, MemoryQueryTopK)
				if memErr == nil && len(matches) > 0 {
					return fmt.Sprintf("[研究报告]\n\n## 研究主题\n%s\n\n## 研究结论\n基于记忆库的有限检索结果。\n\n## 支持证据\n%s",
						fullContext, FormatMemoryResults(matches)), nil
				}
			}
			return "未能检索到任何有效记忆", nil
		}
		report = result.Report

		elapsed := time.Since(startTime)
		logger.Info("Learner", "研究完成: 耗时=%v, 搜索轮次=%d, 辩论轮次=%d",
			elapsed, result.SearchRounds, result.DebateRounds)
	}

	return report, nil
}

// calibrateIntent 校准意图提示（处理 ambiguous 情况）
func (l *Learner) calibrateIntent(query string, hint IntentHint) IntentHint {
	if hint != IntentAmbiguous {
		return hint
	}
	// 预查记忆（第一次查询，仅用于判定意图偏向）
	if !l.memory.IsAvailable() {
		logger.Info("Learner", "意图校准: ambiguous → search (记忆不可用)")
		return IntentSearch
	}
	results, err := l.memory.Query(query, 10)
	if err != nil {
		logger.Info("Learner", "意图校准: ambiguous → search (记忆查询失败)")
		return IntentSearch
	}
	strongCount := 0
	for _, m := range results {
		if m.Similarity >= MemoryStrongMatchThreshold {
			strongCount++
		}
	}
	if strongCount >= MemoryStrongMatchMinCount {
		logger.Info("Learner", "意图校准: ambiguous → memory (强匹配%d条)", strongCount)
		return IntentMemory
	}
	logger.Info("Learner", "意图校准: ambiguous → search (强匹配%d条)", strongCount)
	return IntentSearch
}

// probePhase 执行预探测（并行 Simple 搜索 + 记忆查询）
func (l *Learner) probePhase(query string, hint IntentHint) *ProbeResult {
	probe := &ProbeResult{}
	var wg sync.WaitGroup

	// Simple 搜索
	if l.search.IsAvailable() {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results, err := l.search.SimpleSearchRaw(query)
			if err != nil {
				logger.Warn("Learner", "预探测搜索失败: %v", err)
				return
			}
			probe.SearchItems = results
			logger.Info("Learner", "预探测搜索完成: %d 条结果", len(results))
		}()
	}

	// 记忆查询（按意图调整 topK，这是第二次查询）
	if l.memory.IsAvailable() {
		wg.Add(1)
		go func() {
			defer wg.Done()
			topK := IntentMemoryTopK[hint]
			results, err := l.memory.Query(query, topK)
			if err != nil {
				logger.Warn("Learner", "预探测记忆查询失败: %v", err)
				return
			}
			probe.MemoryMatches = results
			logger.Info("Learner", "预探测记忆完成: %d 条结果", len(results))
		}()
	}

	wg.Wait()
	return probe
}

// evaluateStrategy 阶段2: LLM 评估预探测结果，输出执行计划
func (l *Learner) evaluateStrategy(query string, hint IntentHint, probe *ProbeResult) (*StrategyPlan, error) {
	// 获取策略评估 prompt
	strategyPrompt := l.debate.promptStrategy
	if strategyPrompt == "" {
		strategyPrompt = defaultStrategyPrompt
	}

	contextInfo := l.debate.getRuntimeContext()
	probeText := l.formatProbeResult(probe)

	prompt := fmt.Sprintf(`%s

当前运行时上下文：
%s

用户的原始查询：%s
TS 层意图提示：%s

预探测结果：
%s

请评估预探测结果是否足以直接回答用户问题，并输出策略计划。`,
		strategyPrompt, contextInfo, query, hint, probeText)

	messages := []LLMMessage{
		{Role: "system", Content: prompt},
		{Role: "user", Content: "请评估预探测结果并输出策略 JSON。"},
	}

	resp, err := l.llm.Chat(messages, BudgetStrategyEval)
	if err != nil {
		return nil, fmt.Errorf("策略评估失败: %w", err)
	}

	// 解析 JSON
	jsonStr := extractJSON(strings.TrimSpace(resp.Content))
	var plan StrategyPlan
	if err := json.Unmarshal([]byte(jsonStr), &plan); err != nil {
		logger.Warn("Learner", "策略评估结果解析失败: %v，降级为 depth", err)
		return &StrategyPlan{
			Sufficient:     false,
			Intent:         hint,
			SearchStrategy: AgentModeDepth,
			DebateRounds:   MaxDebateRounds,
			MemoryTopK:     MemoryQueryTopK,
		}, nil
	}

	// 安全边界：clamp 参数
	if plan.DebateRounds > MaxDebateRounds {
		plan.DebateRounds = MaxDebateRounds
	}
	if plan.DebateRounds < 1 {
		plan.DebateRounds = 1
	}
	if plan.MemoryTopK < 5 {
		plan.MemoryTopK = 5
	}
	if plan.MemoryTopK > 50 {
		plan.MemoryTopK = 50
	}

	logger.Info("Learner", "策略评估完成: sufficient=%v, strategy=%s, multiAngle=%v, rounds=%d",
		plan.Sufficient, plan.SearchStrategy, plan.MultiAngleSearch, plan.DebateRounds)

	return &plan, nil
}

// formatProbeResult 格式化预探测结果为 LLM 可读文本
func (l *Learner) formatProbeResult(probe *ProbeResult) string {
	var parts []string

	if len(probe.SearchItems) > 0 {
		parts = append(parts, "## 轻量搜索结果")
		for i, item := range probe.SearchItems {
			parts = append(parts, fmt.Sprintf("[%d] %s\nURL: %s\n摘要: %s",
				i+1, item.Title, item.URL, item.Snippet))
		}
	} else {
		parts = append(parts, "## 轻量搜索结果\n无结果")
	}

	if len(probe.MemoryMatches) > 0 {
		parts = append(parts, "## 记忆库匹配\n"+FormatMemoryResults(probe.MemoryMatches))
	} else {
		parts = append(parts, "## 记忆库匹配\n无匹配")
	}

	return strings.Join(parts, "\n\n")
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

// DumpContext 导出学习者运行时上下文到 JSON 文件（覆写模式）
// 用于调试排查消息重复、上下文异常等问题
func (l *Learner) DumpContext(dialogueJSON string, unreadJSON string, hint IntentHint, outputPath string) (string, error) {
	dump := &DebugContextDump{
		Timestamp:       time.Now().Format("2006-01-02 15:04:05"),
		IntentHint:      string(hint),
		DialogueJSON:    dialogueJSON,
		UnreadJSON:      unreadJSON,
		MemoryAvailable: l.memory.IsAvailable(),
		SearchAvailable: l.search.IsAvailable(),
	}

	// 构建完整上下文（与 Execute 一致）
	dump.FullContext = l.buildFullContext(dialogueJSON, unreadJSON)

	// 执行预探测
	probe := l.probePhase(dump.FullContext, hint)
	dump.ProbeSearch = probe.SearchItems
	dump.ProbeMemory = probe.MemoryMatches

	// 策略评估
	plan, err := l.evaluateStrategy(dump.FullContext, hint, probe)
	if err == nil {
		dump.StrategyPlan = plan
	}

	// 当前辩论系统状态
	if l.debate != nil && l.debate.state != nil {
		dump.DebateState = l.debate.state
	}

	// 序列化为 JSON（带缩进，方便人工阅读）
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
		intentHint := IntentBalanced // 默认均衡

		if len(call.Arguments) >= 1 {
			dialogueJSON, _ = call.Argument(0).Export().(string)
		}
		if len(call.Arguments) >= 2 {
			unreadJSON, _ = call.Argument(1).Export().(string)
		}
		if len(call.Arguments) >= 3 {
			if hint, ok := call.Argument(2).Export().(string); ok && hint != "" {
				intentHint = IntentHint(hint)
			}
		}

		report, err := inst.Execute(dialogueJSON, unreadJSON, intentHint)
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

	// learnerDumpContext 导出学习者上下文到文件（覆写模式）
	// 参数: dialogueJSON, unreadJSON, intentHint, outputPath
	// 返回: [文件路径, error]
	vm.Set("learnerDumpContext", func(call goja.FunctionCall) goja.Value {
		learnerMutex.RLock()
		inst := learnerInstance
		learnerMutex.RUnlock()

		if inst == nil {
			return vm.ToValue([]any{"", fmt.Errorf("学习者未初始化，请先调用 learnerInit")})
		}

		dialogueJSON := ""
		unreadJSON := ""
		intentHint := IntentBalanced
		outputPath := "learner_debug_context.json"

		if len(call.Arguments) >= 1 {
			dialogueJSON, _ = call.Argument(0).Export().(string)
		}
		if len(call.Arguments) >= 2 {
			unreadJSON, _ = call.Argument(1).Export().(string)
		}
		if len(call.Arguments) >= 3 {
			if hint, ok := call.Argument(2).Export().(string); ok && hint != "" {
				intentHint = IntentHint(hint)
			}
		}
		if len(call.Arguments) >= 4 {
			if path, ok := call.Argument(3).Export().(string); ok && path != "" {
				outputPath = path
			}
		}

		path, err := inst.DumpContext(dialogueJSON, unreadJSON, intentHint, outputPath)
		if err != nil {
			logger.Error("Learner", "上下文导出失败: %v", err)
			return vm.ToValue([]any{"", err})
		}

		return vm.ToValue([]any{path, nil})
	})
}
