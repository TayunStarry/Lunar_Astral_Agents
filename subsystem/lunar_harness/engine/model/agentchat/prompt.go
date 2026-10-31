package agentchat

// prompt.go 智能体提示词读取与占位符渲染。
//
// 提示词集中存放在 LunarHarness/prompts 内嵌目录；本包只做缓存与替换，
// 让各智能体不必再各维护一份提示词缓存。

import (
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/prompts"
)

// PromptText 读取提示词文本（带进程内缓存；读取失败返回空串）
func PromptText(name string) string {
	promptMu.Lock()
	defer promptMu.Unlock()
	if cached, ok := promptCache[name]; ok {
		return cached
	}
	data, err := prompts.Read(name)
	if err != nil {
		logger.Error("CrystalAstral", "智能体读取提示词 %s 失败: %v", name, err)
		return ""
	}
	text := string(data)
	promptCache[name] = text
	return text
}

// RenderPrompt 单次替换提示词中的 {占位符}（pairs 为「占位符, 值」成对传入）
//
// 单次替换而不是逐项 ReplaceAll：插入的正文本身可能含有 `{xxx}` 字样，
// 逐项替换会把它当占位符二次展开（文件内容里的花括号并不罕见）。
func RenderPrompt(template string, pairs ...string) string {
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
