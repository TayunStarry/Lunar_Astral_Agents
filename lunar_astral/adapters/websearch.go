package adapters

import (
	"fmt"
	"logger"
	"time"
	"websearch"

	"github.com/dop251/goja"
)

// webSearchSystem 网络检索子系统实例
var webSearchSystem *websearch.System

// webSearchInit 初始化网络检索子系统
// 参数: baseURL, apiKey, model, maxTokens, temperature
// 返回值: [boolean, error]
func (class *Runtime) webSearchInit(call goja.FunctionCall) goja.Value {
	if len(call.Arguments) < 5 {
		return class.runtime.ToValue([]any{false, fmt.Errorf("webSearchInit 参数不足，需要 baseURL, apiKey, model, maxTokens, temperature")})
	}

	baseURL, ok := call.Argument(0).Export().(string)
	if !ok {
		return class.runtime.ToValue([]any{false, fmt.Errorf("baseURL 必须是字符串")})
	}

	apiKey, ok := call.Argument(1).Export().(string)
	if !ok {
		return class.runtime.ToValue([]any{false, fmt.Errorf("apiKey 必须是字符串")})
	}

	model, ok := call.Argument(2).Export().(string)
	if !ok {
		return class.runtime.ToValue([]any{false, fmt.Errorf("model 必须是字符串")})
	}

	maxTokens := int(call.Argument(3).ToInteger())

	temperature := call.Argument(4).ToFloat()

	cfg := websearch.DefaultConfig()
	cfg.LLM.BaseURL = baseURL
	cfg.LLM.APIKey = apiKey
	cfg.LLM.Model = model
	cfg.LLM.MaxTokens = maxTokens
	cfg.LLM.Temperature = temperature

	webSearchSystem = websearch.NewWithConfig(cfg)

	if webSearchSystem.HasLLM() {
		logger.Info("LunarCore", "网络检索子系统初始化成功，LLM 已配置: %s", model)
	} else {
		logger.Warn("LunarCore", "网络检索子系统初始化成功，但 LLM 未配置，深层搜索将降级为浅层搜索")
	}

	return class.runtime.ToValue([]any{true, nil})
}

// webSearchDeep 执行深层搜索
// 参数: query
// 返回值: [string, error]
func (class *Runtime) webSearchDeep(call goja.FunctionCall) goja.Value {
	if webSearchSystem == nil {
		return class.runtime.ToValue([]any{"", fmt.Errorf("网络检索子系统未初始化，请先调用 webSearchInit")})
	}

	if len(call.Arguments) < 1 {
		return class.runtime.ToValue([]any{"", fmt.Errorf("webSearchDeep 参数不足，需要 query")})
	}

	query, ok := call.Argument(0).Export().(string)
	if !ok {
		return class.runtime.ToValue([]any{"", fmt.Errorf("query 必须是字符串")})
	}

	logger.Info("LunarCore", "执行深层搜索: %s", query)

	result, err := webSearchSystem.DeepSearch(query)
	if err != nil {
		logger.Error("LunarCore", "深层搜索失败: %v", err)
		return class.runtime.ToValue([]any{"", err})
	}

	return class.runtime.ToValue([]any{result, nil})
}

// webSearchShallow 执行浅层搜索
// 参数: query
// 返回值: [string, error]
func (class *Runtime) webSearchShallow(call goja.FunctionCall) goja.Value {
	if webSearchSystem == nil {
		return class.runtime.ToValue([]any{"", fmt.Errorf("网络检索子系统未初始化，请先调用 webSearchInit")})
	}

	if len(call.Arguments) < 1 {
		return class.runtime.ToValue([]any{"", fmt.Errorf("webSearchShallow 参数不足，需要 query")})
	}

	query, ok := call.Argument(0).Export().(string)
	if !ok {
		return class.runtime.ToValue([]any{"", fmt.Errorf("query 必须是字符串")})
	}

	logger.Info("LunarCore", "执行浅层搜索: %s", query)

	result, err := webSearchSystem.ShallowSearch(query)
	if err != nil {
		logger.Error("LunarCore", "浅层搜索失败: %v", err)
		return class.runtime.ToValue([]any{"", err})
	}

	return class.runtime.ToValue([]any{result, nil})
}

// webSearchResearch 执行研究搜索
// 参数: query
// 返回值: [string, error]
func (class *Runtime) webSearchResearch(call goja.FunctionCall) goja.Value {
	if webSearchSystem == nil {
		return class.runtime.ToValue([]any{"", fmt.Errorf("网络检索子系统未初始化，请先调用 webSearchInit")})
	}

	if len(call.Arguments) < 1 {
		return class.runtime.ToValue([]any{"", fmt.Errorf("webSearchResearch 参数不足，需要 query")})
	}

	query, ok := call.Argument(0).Export().(string)
	if !ok {
		return class.runtime.ToValue([]any{"", fmt.Errorf("query 必须是字符串")})
	}

	logger.Info("LunarCore", "执行研究搜索: %s", query)

	result, err := webSearchSystem.ResearchSearch(query)
	if err != nil {
		logger.Error("LunarCore", "研究搜索失败: %v", err)
		return class.runtime.ToValue([]any{"", err})
	}

	return class.runtime.ToValue([]any{result, nil})
}

// webSearchIsReady 检查网络检索子系统是否已初始化
// 返回值: boolean
func (class *Runtime) webSearchIsReady(call goja.FunctionCall) goja.Value {
	return class.runtime.ToValue(webSearchSystem != nil)
}

// webSearchHasLLM 检查网络检索子系统是否配置了 LLM
// 返回值: boolean
func (class *Runtime) webSearchHasLLM(call goja.FunctionCall) goja.Value {
	if webSearchSystem == nil {
		return class.runtime.ToValue(false)
	}
	return class.runtime.ToValue(webSearchSystem.HasLLM())
}

// initWebSearchWithTimeout 带超时的初始化（内部使用）
func initWebSearchWithTimeout(baseURL, apiKey, model string, maxTokens int, temperature float64, _ time.Duration) error {
	cfg := websearch.DefaultConfig()
	cfg.LLM.BaseURL = baseURL
	cfg.LLM.APIKey = apiKey
	cfg.LLM.Model = model
	cfg.LLM.MaxTokens = maxTokens
	cfg.LLM.Temperature = temperature

	webSearchSystem = websearch.NewWithConfig(cfg)

	if webSearchSystem == nil {
		return fmt.Errorf("网络检索子系统创建失败")
	}

	return nil
}
