package learner

import (
	"encoding/json"
	"fmt"
	"logger"
	"os"
	"strings"
	"time"

	lunar_chromedp "lunar_chromedp"

	"github.com/dop251/goja"
)

// BindLearnerToRuntime 注册学习者智能体函数到 Goja 运行时
// 由 adapters.create.go 中的 registerAdaptersToRuntime 调用
func BindLearnerToRuntime(vm *goja.Runtime) {
	vm.Set("learnerIsReady", func(call goja.FunctionCall) goja.Value {
		runtimeMutex.Lock()
		defer runtimeMutex.Unlock()
		return vm.ToValue(learnerInitialized)
	})

	vm.Set("learnerInit", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 6 {
				return vm.ToValue([]any{false, fmt.Errorf("learnerInit 参数不足, 需至少 6 个: systemURL, systemKey, modelName, embeddingURL, embeddingKey, embeddingName[, memoryDBDir]")})
			}

			systemURL := call.Argument(0).String()
			systemKey := call.Argument(1).String()
			modelName := call.Argument(2).String()
			embeddingURL := call.Argument(3).String()
			embeddingKey := call.Argument(4).String()
			embeddingName := call.Argument(5).String()
			memoryDBDir := "local_data/database/memory" // 默认值
			if len(call.Arguments) >= 7 {
				if dir := call.Argument(6).String(); dir != "" {
					memoryDBDir = dir
				}
			}

		runtimeMutex.Lock()
		defer runtimeMutex.Unlock()

		// 如果已初始化，直接返回成功
		if learnerInitialized {
			return vm.ToValue([]any{true, nil})
		}

		// 构建 lunar_chromedp 搜索配置（MaxContextTokens 用默认值）
			config := lunar_chromedp.SearchConfig{
				MultimodalURL:  systemURL,
				MultimodalName: modelName,
				MultimodalKey:  systemKey,
				EmbeddingURL:   embeddingURL,
				EmbeddingName:  embeddingName,
				EmbeddingKey:   embeddingKey,
				MemoryDBDir:    memoryDBDir,
			}

		// 初始化 lunar_chromedp 搜索智能体（包含记忆库初始化、浏览器启动）
		if err := lunar_chromedp.InitSearch(config); err != nil {
			logger.Error("Learner", "学习者初始化失败: %v", err)
			return vm.ToValue([]any{false, fmt.Errorf("学习者初始化失败: %v", err)})
		}

		learnerInitialized = true
		logger.Info("Learner", "学习者初始化完成, model=%s", modelName)
		return vm.ToValue([]any{true, nil})
	})

	vm.Set("learnerExecute", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return vm.ToValue([]any{nil, fmt.Errorf("learnerExecute 参数不足, 需 1 个: query")})
		}

		query := call.Argument(0).String()
		query = strings.TrimSpace(query)
		if query == "" {
			return vm.ToValue([]any{nil, fmt.Errorf("查询内容为空")})
		}

		runtimeMutex.Lock()
		initialized := learnerInitialized
		runtimeMutex.Unlock()

		if !initialized {
			return vm.ToValue([]any{nil, fmt.Errorf("学习者未初始化，请先调用 learnerInit")})
		}

		// 执行搜索
		report, err := lunar_chromedp.Search(query)
		if err != nil {
			logger.Error("Learner", "搜索执行失败: %v", err)
			return vm.ToValue([]any{nil, err})
		}

		// 格式化为可读报告
		formatted := formatSearchReport(report)
		// 将完整搜索报告打印到终端日志，便于查看搜索产物
		logger.Info("Learner", "==== 搜索报告开始 (query=%q, answer=%d字符, 来源%d个) ====\n%s\n==== 搜索报告结束 ====",
			query, len([]rune(report.Answer)), len(report.UsedSources), formatted)
		logger.Info("Learner", "搜索完成, query=%q, answer=%d字符", query, len([]rune(report.Answer)))
		return vm.ToValue([]any{formatted, nil})
	})

	vm.Set("learnerDumpContext", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 {
			return vm.ToValue([]any{false, fmt.Errorf("learnerDumpContext 参数不足, 需 2 个: query, outputPath")})
		}

		query := call.Argument(0).String()
		outputPath := call.Argument(1).String()

		runtimeMutex.Lock()
		initialized := learnerInitialized
		runtimeMutex.Unlock()

		// 构建快照
		snapshot := map[string]any{
			"timestamp":          time.Now().Format("2006-01-02 15:04:05"),
			"role":               "学习者(Go层)",
			"query":              query,
			"learnerInitialized": initialized,
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

// formatSearchReport 将 SearchReport 格式化为可读的 markdown 报告
func formatSearchReport(report *lunar_chromedp.SearchReport) string {
	if report == nil {
		return "搜索未返回结果。"
	}

	var sb strings.Builder

	// 标题
	if report.FromMemory {
		sb.WriteString("## 记忆库查询结果\n\n")
	} else {
		sb.WriteString("## 网络搜索结果\n\n")
		sb.WriteString(fmt.Sprintf("> 查询: %s | 搜索轮次: %d | 生成时间: %s\n\n",
			report.Query, report.SearchRounds,
			report.GeneratedAt.Format("2006-01-02 15:04:05")))
	}

	// 答案
	if report.Answer != "" {
		sb.WriteString(report.Answer)
		sb.WriteString("\n\n")
	}

	// 引用来源
	if len(report.UsedSources) > 0 {
		sb.WriteString("### 引用来源\n")
		for i, src := range report.UsedSources {
			if i >= 10 {
				sb.WriteString(fmt.Sprintf("... 及其他 %d 个来源\n", len(report.UsedSources)-10))
				break
			}
			sb.WriteString(fmt.Sprintf("- [%s](%s)\n", src, src))
		}
		sb.WriteString("\n")
	}

	if report.FromMemory {
		sb.WriteString("*（以上信息来自记忆库，如需最新网络内容请明确要求搜索）*\n")
	}

	return sb.String()
}
