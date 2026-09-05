package adapters

import (
	"LunarSubsystem/AgentSearch"
	"LunarSubsystem/LoggerGeneral"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/dop251/goja"
)

// searchIsReady 搜索智能体是否初始化
func (class *Runtime) searchIsReady(_ goja.FunctionCall) goja.Value {
	searchRuntimeMutex.Lock()
	defer searchRuntimeMutex.Unlock()
	return class.runtime.ToValue(searchInitialized)
}

// searchInit 初始化搜索智能体
func (class *Runtime) searchInit(call goja.FunctionCall) goja.Value {
	memoryDBDir := "local_data/database/memory"
	if len(call.Arguments) >= 1 {
		if dir := call.Argument(0).String(); dir != "" {
			memoryDBDir = dir
		}
	}

	searchRuntimeMutex.Lock()
	defer searchRuntimeMutex.Unlock()

	// 如果已初始化，直接返回成功
	if searchInitialized {
		return class.runtime.ToValue([]any{true, nil})
	}

	// 构建 lunar_chromedp 搜索配置（模型配置从 config 模块读取）
	config := AgentSearch.SearchConfig{
		MemoryDBDir: memoryDBDir,
	}

	// 初始化 lunar_chromedp 搜索智能体（包含记忆库初始化、浏览器启动）
	if err := AgentSearch.InitSearch(config); err != nil {
		LoggerGeneral.Error("Searcher", "搜索者初始化失败: %v", err)
		return class.runtime.ToValue([]any{false, fmt.Errorf("搜索者初始化失败: %v", err)})
	}

	searchInitialized = true
	LoggerGeneral.Info("Searcher", "搜索者初始化完成 (模型配置从 lunar_config.json 读取)")
	return class.runtime.ToValue([]any{true, nil})
}

// searchExecute 执行搜索
func (class *Runtime) searchExecute(call goja.FunctionCall) goja.Value {
	if len(call.Arguments) < 1 {
		return class.runtime.ToValue([]any{nil, fmt.Errorf("searchExecute 参数不足, 需 1 个: query")})
	}

	query := call.Argument(0).String()
	query = strings.TrimSpace(query)
	if query == "" {
		return class.runtime.ToValue([]any{nil, fmt.Errorf("查询内容为空")})
	}

	searchRuntimeMutex.Lock()
	initialized := searchInitialized
	searchRuntimeMutex.Unlock()

	if !initialized {
		return class.runtime.ToValue([]any{nil, fmt.Errorf("搜索者未初始化，请先调用 searchInit")})
	}

	// 执行搜索
	report, err := AgentSearch.Search(query)
	if err != nil {
		LoggerGeneral.Error("Searcher", "搜索执行失败: %v", err)
		return class.runtime.ToValue([]any{nil, err})
	}

	// 格式化为可读报告
	formatted := formatSearchReport(report)
	// 将完整搜索报告打印到终端日志，便于查看搜索产物
	LoggerGeneral.Info("Searcher", "==== 搜索报告开始 (query=%q, answer=%d字符, 来源%d个) ====\n%s\n==== 搜索报告结束 ====",
		query, len([]rune(report.Answer)), len(report.UsedSources), formatted)
	LoggerGeneral.Info("Searcher", "搜索完成, query=%q, answer=%d字符", query, len([]rune(report.Answer)))
	return class.runtime.ToValue([]any{formatted, nil})
}

// searchDumpContext 导出搜索智能体上下文
func (class *Runtime) searchDumpContext(call goja.FunctionCall) goja.Value {
	if len(call.Arguments) < 2 {
		return class.runtime.ToValue([]any{false, fmt.Errorf("searchDumpContext 参数不足, 需 2 个: query, outputPath")})
	}

	query := call.Argument(0).String()
	outputPath := call.Argument(1).String()

	searchRuntimeMutex.Lock()
	initialized := searchInitialized
	searchRuntimeMutex.Unlock()

	// 构建快照
	snapshot := map[string]any{
		"timestamp":         time.Now().Format("2006-01-02 15:04:05"),
		"role":              "搜索者(Go层)",
		"query":             query,
		"searchInitialized": initialized,
	}

	// 序列化为 JSON
	jsonBytes, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return class.runtime.ToValue([]any{false, fmt.Errorf("序列化快照失败: %v", err)})
	}

	// 写入文件
	if err := os.WriteFile(outputPath, jsonBytes, 0644); err != nil {
		return class.runtime.ToValue([]any{false, fmt.Errorf("写入文件失败: %v", err)})
	}

	LoggerGeneral.Info("Searcher", "Go 层上下文快照已导出: %s", outputPath)
	return class.runtime.ToValue([]any{true, nil})
}

// formatSearchReport 将 SearchReport 格式化为可读的 markdown 报告
func formatSearchReport(report *AgentSearch.SearchReport) string {
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
