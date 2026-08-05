package learner

import (
	"context"
	"encoding/json"
	"fmt"
	"logger"
	"os"
	"strings"
	"time"
	"websearch"

	"storage/module"

	"github.com/dop251/goja"
)

// BindLearnerToRuntime 注册学习者智能体函数到 Goja 运行时
// 由 adapters.create.go 中的 registerAdaptersToRuntime 调用
func BindLearnerToRuntime(vm *goja.Runtime) {
	vm.Set("learnerIsReady", func(call goja.FunctionCall) goja.Value {
		runtimeMutex.Lock()
		defer runtimeMutex.Unlock()

		if runtime == nil || runtime.system == nil {
			return vm.ToValue(false)
		}
		return vm.ToValue(runtime.system.HasLearner())
	})

	vm.Set("learnerInit", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 8 {
			return vm.ToValue([]any{false, fmt.Errorf("learnerInit 参数不足, 需 8 个参数")})
		}

		systemURL := call.Argument(0).String()
		systemKey := call.Argument(1).String()
		modelName := call.Argument(2).String()
		maxTokens := int(call.Argument(3).ToInteger())
		temperature := call.Argument(4).ToFloat()
		_ = call.Argument(5).String() // embeddingURL（当前未使用，storage 模块自行管理）
		_ = call.Argument(6).String() // embeddingKey（当前未使用）
		_ = call.Argument(7).String() // embeddingName（当前未使用，collection 创建时指定）

		runtimeMutex.Lock()
		defer runtimeMutex.Unlock()

		// 如果已初始化，直接返回成功
		if runtime != nil && runtime.system != nil && runtime.system.HasLearner() {
			return vm.ToValue([]any{true, nil})
		}

		// 创建 LLM 提供者
		provider := websearch.NewOpenAIProvider(systemURL, systemKey, modelName, maxTokens, temperature)

		// 创建网络检索配置（开启智能学习模式）
		cfg := defaultWebSearchConfig()

		// 创建带调试日志回调的子系统入口
		system := websearch.NewWithLLM(cfg, provider, func(format string, args ...interface{}) {
			logger.Info("Learner", format, args...)
		})

		runtime = &LearnerRuntime{system: system}

		if !system.HasLearner() {
			return vm.ToValue([]any{false, fmt.Errorf("学习者初始化失败: storage 模块记忆库未就绪，请确保记忆库已初始化")})
		}

		logger.Info("Learner", "学习者初始化完成, model=%s", modelName)
		return vm.ToValue([]any{true, nil})
	})

	vm.Set("learnerExecute", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 {
			return vm.ToValue([]any{nil, fmt.Errorf("learnerExecute 参数不足, 需 2 个: unreadTexts, mode")})
		}

		// 解析未读文本数组
		unreadTexts := extractStringArray(call.Argument(0))
		mode := call.Argument(1).String()

		if len(unreadTexts) == 0 {
			return vm.ToValue([]any{nil, fmt.Errorf("未读文本为空")})
		}

		// 合并所有文本为查询字符串
		query := strings.Join(unreadTexts, "\n")

		runtimeMutex.Lock()
		sys := runtime
		runtimeMutex.Unlock()

		if sys == nil || sys.system == nil || !sys.system.HasLearner() {
			return vm.ToValue([]any{nil, fmt.Errorf("学习者未初始化，请先调用 learnerInit")})
		}

		ctx := context.Background()

		switch mode {
		case "recall":
			// 回忆模式：仅查询记忆库，不进行网络搜索
			report, err := recallFromMemory(ctx, query)
			if err != nil {
				logger.Error("Learner", "回忆模式查询失败: %v", err)
				return vm.ToValue([]any{nil, err})
			}
			logger.Info("Learner", "回忆模式完成, query=%q", query)
			return vm.ToValue([]any{report, nil})

		case "full":
			// 完整模式：执行完整学习工作流（refine→memory→search→evaluate→deep→store）
			report, err := sys.system.LearnerSearch(ctx, query)
			if err != nil {
				logger.Error("Learner", "完整搜索执行失败: %v", err)
				return vm.ToValue([]any{nil, err})
			}
			logger.Info("Learner", "完整搜索完成, query=%q, report=%d字符", query, len([]rune(report)))
			return vm.ToValue([]any{report, nil})

		default:
			return vm.ToValue([]any{nil, fmt.Errorf("未知模式: %s，支持 recall 或 full", mode)})
		}
	})

	vm.Set("learnerDumpContext", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 3 {
			return vm.ToValue([]any{false, fmt.Errorf("learnerDumpContext 参数不足, 需 3 个: unreadTexts, mode, outputPath")})
		}

		unreadTexts := extractStringArray(call.Argument(0))
		mode := call.Argument(1).String()
		outputPath := call.Argument(2).String()

		runtimeMutex.Lock()
		sys := runtime
		runtimeMutex.Unlock()

		// 构建快照
		snapshot := map[string]any{
			"timestamp": time.Now().Format("2006-01-02 15:04:05"),
			"role":      "学习者(Go层)",
			"mode":      mode,
			"unreadTexts": func() []map[string]any {
				items := make([]map[string]any, 0, len(unreadTexts))
				for i, t := range unreadTexts {
					preview := t
					if len([]rune(preview)) > 300 {
						preview = string([]rune(preview)[:300]) + "..."
					}
					items = append(items, map[string]any{
						"index":          i,
						"contentPreview": preview,
						"contentLength":  len([]rune(t)),
					})
				}
				return items
			}(),
			"learnerInitialized":        sys != nil && sys.system != nil && sys.system.HasLearner(),
			"storageInitialized":        module.MemoryDatabase != nil && module.MemoryDatabase.IsMemoryInitialized(),
			"knowledgeCollectionCount":  module.GetCollectionCount("search_knowledge"),
			"experienceCollectionCount": module.GetCollectionCount("search_experience"),
		}

		// 序列化为 JSON
		jsonBytes, err := json.MarshalIndent(snapshot, "", "  ")
		if err != nil {
			return vm.ToValue([]any{false, fmt.Errorf("序列化快照失败: %v", err)})
		}

		// 写入文件
		if err := os.WriteFile(outputPath, jsonBytes, 0644); err != nil {
			return vm.ToValue([]any{false, fmt.Errorf("写入文件失败: %v", err)})
		}

		logger.Info("Learner", "Go 层上下文快照已导出: %s", outputPath)
		return vm.ToValue([]any{true, nil})
	})
}

// ── 辅助函数 ──

// extractStringArray 从 Goja 值中提取字符串数组
func extractStringArray(val goja.Value) []string {
	exported := val.Export()
	switch arr := exported.(type) {
	case []string:
		return arr
	case []interface{}:
		result := make([]string, 0, len(arr))
		for _, item := range arr {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
		return result
	default:
		return nil
	}
}

// recallFromMemory 回忆模式：仅查询记忆库，不进行网络搜索
func recallFromMemory(ctx context.Context, query string) (string, error) {
	if module.MemoryDatabase == nil || !module.MemoryDatabase.IsMemoryInitialized() {
		return "", fmt.Errorf("记忆库未初始化")
	}

	const recallTopK = 10
	const recallMinSimilarity = 0.60

	knowledge, err := module.MemoryDatabase.MemoryQueryMessagesWithContent(ctx, "search_knowledge", query, recallTopK)
	if err != nil {
		return "", fmt.Errorf("查询知识记忆失败: %w", err)
	}

	experience, err := module.MemoryDatabase.MemoryQueryMessagesWithContent(ctx, "search_experience", query, recallTopK)
	if err != nil {
		return "", fmt.Errorf("查询经验记忆失败: %w", err)
	}

	// 过滤低相似度结果
	knowledge = filterBySimilarity(knowledge, recallMinSimilarity)
	experience = filterBySimilarity(experience, recallMinSimilarity)

	// 构建报告
	var sb strings.Builder
	sb.WriteString("【记忆库查询结果】\n\n")

	if len(knowledge) > 0 {
		sb.WriteString("### 知识记忆\n")
		for i, m := range knowledge {
			sb.WriteString(fmt.Sprintf("%d. 相关度:%.0f%% | %s\n", i+1, m.Similarity*100, truncateStr(m.Content, 500)))
		}
		sb.WriteString("\n")
	}

	if len(experience) > 0 {
		sb.WriteString("### 经验记忆\n")
		for i, m := range experience {
			sb.WriteString(fmt.Sprintf("%d. 相关度:%.0f%% | %s\n", i+1, m.Similarity*100, truncateStr(m.Content, 500)))
		}
		sb.WriteString("\n")
	}

	if len(knowledge) == 0 && len(experience) == 0 {
		sb.WriteString("记忆库中未找到相关信息。建议使用完整搜索模式获取最新信息。\n")
	}

	return sb.String(), nil
}

// filterBySimilarity 按相似度阈值过滤记忆查询结果
func filterBySimilarity(matches []module.MemoryQueryResult, threshold float64) []module.MemoryQueryResult {
	filtered := make([]module.MemoryQueryResult, 0, len(matches))
	for _, m := range matches {
		if float64(m.Similarity) >= threshold {
			filtered = append(filtered, m)
		}
	}
	return filtered
}

// truncateStr 截断字符串到指定字符数（rune 级别）
func truncateStr(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}