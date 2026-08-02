package adapters

import (
	"context"
	"fmt"
	"logger"
	"websearch"

	"github.com/dop251/goja"
)

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

	provider := websearch.NewOpenAIProvider(baseURL, apiKey, model, maxTokens, temperature)
	webSearchSystem = websearch.NewWithLLM(cfg, provider)

	if webSearchSystem.HasLLM() {
		logger.Info("LunarCore", "网络检索子系统初始化成功，LLM 已配置: %s", model)
	} else {
		logger.Warn("LunarCore", "网络检索子系统初始化成功，但 LLM 未配置，网页搜索将降级为轻量摘要")
	}

	return class.runtime.ToValue([]any{true, nil})
}

// webSearchWebpage 执行网页搜索
// 参数: query
// 返回值: [string, error]
func (class *Runtime) webSearchWebpage(call goja.FunctionCall) goja.Value {
	if webSearchSystem == nil {
		return class.runtime.ToValue([]any{"", fmt.Errorf("网络检索子系统未初始化，请先调用 webSearchInit")})
	}

	if len(call.Arguments) < 1 {
		return class.runtime.ToValue([]any{"", fmt.Errorf("webSearchWebpage 参数不足，需要 query")})
	}

	query, ok := call.Argument(0).Export().(string)
	if !ok {
		return class.runtime.ToValue([]any{"", fmt.Errorf("query 必须是字符串")})
	}

	logger.Info("LunarCore", "执行网页搜索: %s", query)

	result, err := webSearchSystem.WebpageSearch(query)
	if err != nil {
		logger.Error("LunarCore", "网页搜索失败: %v", err)
		return class.runtime.ToValue([]any{"", err})
	}

	return class.runtime.ToValue([]any{result, nil})
}

// webSearchSimple 执行轻量摘要搜索
// 参数: query
// 返回值: [string, error]
func (class *Runtime) webSearchSimple(call goja.FunctionCall) goja.Value {
	if webSearchSystem == nil {
		return class.runtime.ToValue([]any{"", fmt.Errorf("网络检索子系统未初始化，请先调用 webSearchInit")})
	}

	if len(call.Arguments) < 1 {
		return class.runtime.ToValue([]any{"", fmt.Errorf("webSearchSimple 参数不足，需要 query")})
	}

	query, ok := call.Argument(0).Export().(string)
	if !ok {
		return class.runtime.ToValue([]any{"", fmt.Errorf("query 必须是字符串")})
	}

	logger.Info("LunarCore", "执行轻量摘要搜索: %s", query)

	result, err := webSearchSystem.SimpleSearch(query)
	if err != nil {
		logger.Error("LunarCore", "轻量摘要搜索失败: %v", err)
		return class.runtime.ToValue([]any{"", err})
	}

	return class.runtime.ToValue([]any{result, nil})
}

// webSearchDepth 执行深度研究
// 参数: query
// 返回值: [string, error]
func (class *Runtime) webSearchDepth(call goja.FunctionCall) goja.Value {
	if webSearchSystem == nil {
		return class.runtime.ToValue([]any{"", fmt.Errorf("网络检索子系统未初始化，请先调用 webSearchInit")})
	}

	if len(call.Arguments) < 1 {
		return class.runtime.ToValue([]any{"", fmt.Errorf("webSearchDepth 参数不足，需要 query")})
	}

	query, ok := call.Argument(0).Export().(string)
	if !ok {
		return class.runtime.ToValue([]any{"", fmt.Errorf("query 必须是字符串")})
	}

	logger.Info("LunarCore", "执行深度研究: %s", query)

	result, err := webSearchSystem.DepthSearch(query)
	if err != nil {
		logger.Error("LunarCore", "深度研究失败: %v", err)
		return class.runtime.ToValue([]any{"", err})
	}

	return class.runtime.ToValue([]any{result, nil})
}

// webSearchIsReady 检查网络检索子系统是否已初始化
// 返回值: boolean
func (class *Runtime) webSearchIsReady(call goja.FunctionCall) goja.Value {
	return class.runtime.ToValue(webSearchSystem != nil)
}

// webSearchAssembly 执行大会辩论式深度研究
// 参数: query
// 返回值: [string, error]
func (class *Runtime) webSearchAssembly(call goja.FunctionCall) goja.Value {
	if webSearchSystem == nil {
		return class.runtime.ToValue([]any{"", fmt.Errorf("网络检索子系统未初始化，请先调用 webSearchInit")})
	}

	if len(call.Arguments) < 1 {
		return class.runtime.ToValue([]any{"", fmt.Errorf("webSearchAssembly 参数不足，需要 query")})
	}

	query, ok := call.Argument(0).Export().(string)
	if !ok {
		return class.runtime.ToValue([]any{"", fmt.Errorf("query 必须是字符串")})
	}

	logger.Info("LunarCore", "执行大会辩论式深度研究: %s", query)

	result, err := webSearchSystem.Search(context.Background(), query, websearch.ModeDepth)
	if err != nil {
		logger.Error("LunarCore", "大会辩论式深度研究失败: %v", err)
		return class.runtime.ToValue([]any{"", err})
	}

	return class.runtime.ToValue([]any{result, nil})
}

// webSearchProcessLinks 处理消息中的链接，提取URL并替换为摘要
// 参数: query
// 返回值: [replacedText: string, descriptions: string[], error]
func (class *Runtime) webSearchProcessLinks(call goja.FunctionCall) goja.Value {
	if webSearchSystem == nil {
		return class.runtime.ToValue([]any{"", []string{}, fmt.Errorf("网络检索子系统未初始化，请先调用 webSearchInit")})
	}

	if len(call.Arguments) < 1 {
		return class.runtime.ToValue([]any{"", []string{}, fmt.Errorf("webSearchProcessLinks 参数不足，需要 query")})
	}

	query, ok := call.Argument(0).Export().(string)
	if !ok {
		return class.runtime.ToValue([]any{"", []string{}, fmt.Errorf("query 必须是字符串")})
	}

	logger.Info("LunarCore", "处理消息中的链接: %s", query)

	replacedText, descriptions := webSearchSystem.ProcessLinks(query)
	return class.runtime.ToValue([]any{replacedText, descriptions, nil})
}

// webSearchSetMemoryProvider 设置记忆库提供者（供大会辩论的守旧派使用）
// 参数: (无参数，自动使用内置记忆库)
// 返回值: [boolean, error]
func (class *Runtime) webSearchSetMemoryProvider(call goja.FunctionCall) goja.Value {
	if webSearchSystem == nil {
		return class.runtime.ToValue([]any{false, fmt.Errorf("网络检索子系统未初始化，请先调用 webSearchInit")})
	}

	if memorySystem == nil {
		return class.runtime.ToValue([]any{false, fmt.Errorf("记忆库未初始化，无法设置 MemoryProvider")})
	}

	webSearchSystem.SetMemoryProvider(memorySystem)
	logger.Info("LunarCore", "已为网络检索子系统设置记忆库提供者（大会辩论模式就绪）")

	return class.runtime.ToValue([]any{true, nil})
}

// webSearchSetDownloadFunc 设置下载回调函数
// 参数: downloadDir (下载目标目录), groupID (下载目标群组ID)
// 返回值: [boolean, error]
func (class *Runtime) webSearchSetDownloadFunc(call goja.FunctionCall) goja.Value {
	if webSearchSystem == nil {
		return class.runtime.ToValue([]any{false, fmt.Errorf("网络检索子系统未初始化，请先调用 webSearchInit")})
	}

	if len(call.Arguments) < 2 {
		return class.runtime.ToValue([]any{false, fmt.Errorf("webSearchSetDownloadFunc 参数不足，需要 downloadDir, groupID")})
	}

	downloadDir, ok := call.Argument(0).Export().(string)
	if !ok {
		return class.runtime.ToValue([]any{false, fmt.Errorf("downloadDir 必须是字符串")})
	}

	groupID, ok := call.Argument(1).Export().(string)
	if !ok {
		return class.runtime.ToValue([]any{false, fmt.Errorf("groupID 必须是字符串")})
	}

	// 创建下载回调函数
	fn := func(url string, gid string) (string, error) {
		return downloadFile(url, downloadDir, gid)
	}

	webSearchSystem.SetDownloadFunc(fn)
	webSearchSystem.SetDownloadGroupID(groupID)

	logger.Info("LunarCore", "已设置下载回调函数，目标目录: %s, 群组ID: %s", downloadDir, groupID)

	return class.runtime.ToValue([]any{true, nil})
}
