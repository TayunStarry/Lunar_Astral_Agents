package Descriptor

// prompts.go 提示词加载（统一从 LunarHarness/prompts 内嵌目录读取）

import (
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/prompts"
)

// promptText 读取提示词文本（带进程内缓存；读取失败返回空串）
func promptText(name string) string {
	promptMu.Lock()
	defer promptMu.Unlock()
	if cached, ok := promptCache[name]; ok {
		return cached
	}
	data, err := prompts.Read(name)
	if err != nil {
		logger.Error("CrystalAstral", "通用描述读取提示词 %s 失败: %v", name, err)
		return ""
	}
	text := string(data)
	promptCache[name] = text
	return text
}

// renderPrompt 单次替换提示词中的 {占位符}（pairs 为「占位符, 值」成对传入）
//
// 单次替换而不是逐项 ReplaceAll：插入的正文本身可能含有 `{xxx}` 字样，
// 逐项替换会把它当占位符二次展开（文件内容里的花括号并不罕见）。
func renderPrompt(template string, pairs ...string) string {
	values := make(map[string]string, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		values[pairs[i]] = pairs[i+1]
	}
	return placeholderPattern.ReplaceAllStringFunc(template, func(key string) string {
		if value, ok := values[key]; ok {
			return value
		}
		return key
	})
}
